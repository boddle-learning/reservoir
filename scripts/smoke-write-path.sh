#!/usr/bin/env bash
#
# smoke-write-path.sh — confirm Reservoir's auth path can write to the DB
# before flipping LMS traffic to it.
#
# Addresses PIR 2026-05-19 action item #11. The 2026-05-19 incident was
# caused by Reservoir's DB_HOST resolving to a read-only endpoint. The
# in-process write probe (PIR #6, internal/database/postgres.go,
# VerifyWritable) catches this at task startup. This script is the
# external complement: from outside the task, confirm a real auth
# request actually advanced last_logged_on for a known test account
# and that no DB write errors are visible on /metrics.
#
# Usage:
#   ENVIRONMENT=staging \
#   RESERVOIR_BASE_URL=https://reservoir.staging.env.boddlelearning.com \
#   SMOKE_TEST_EMAIL=smoke-test@boddlelearning.com \
#   SMOKE_TEST_PASSWORD=$(aws ssm get-parameter \
#     --name /boddle/staging/reservoir-smoke/PASSWORD \
#     --with-decryption --query Parameter.Value --output text) \
#   ./scripts/smoke-write-path.sh
#
# Pass criteria (all must be true):
#   1. POST /auth/login returns 200 with a valid token pair.
#   2. The smoke-test user's last_logged_on column advanced within
#      ~10 seconds of the login call (queried via LMS or a side channel).
#   3. /metrics shows reservoir_auth_db_write_errors_total flat
#      (no increase across the smoke run).
#
# Exits 0 on all pass, 1 on any fail.

set -euo pipefail

: "${ENVIRONMENT:?ENVIRONMENT must be set, e.g. staging or prod1}"
: "${RESERVOIR_BASE_URL:?RESERVOIR_BASE_URL must be set}"
: "${SMOKE_TEST_EMAIL:?SMOKE_TEST_EMAIL must be set}"
: "${SMOKE_TEST_PASSWORD:?SMOKE_TEST_PASSWORD must be set}"

# Timeout for individual HTTP calls.
HTTP_TIMEOUT_SECONDS="${HTTP_TIMEOUT_SECONDS:-10}"

# How long to wait for last_logged_on to advance after the login call.
# Reservoir's async LastLoginWriter flushes every 5 seconds.
WRITE_VISIBILITY_DEADLINE_SECONDS="${WRITE_VISIBILITY_DEADLINE_SECONDS:-15}"

# Where to read last_logged_on from. The script does not query Postgres
# directly to avoid baking DB creds into the smoke-test runner. Two
# supported strategies:
#   - LMS_API: hit an LMS endpoint that returns the user's last_logged_on
#     (recommended; same trust boundary as production traffic).
#   - SKIP: don't verify the write actually landed (degraded mode; only
#     useful if the LMS endpoint isn't reachable from the runner).
LAST_LOGIN_CHECK_METHOD="${LAST_LOGIN_CHECK_METHOD:-LMS_API}"

red()    { printf "\e[31m%s\e[0m\n" "$*"; }
green()  { printf "\e[32m%s\e[0m\n" "$*"; }
yellow() { printf "\e[33m%s\e[0m\n" "$*"; }

fail() {
  red "FAIL: $*"
  exit 1
}

note() {
  yellow "  $*"
}

echo "Reservoir write-path smoke test"
echo "  Environment: ${ENVIRONMENT}"
echo "  Reservoir:   ${RESERVOIR_BASE_URL}"
echo "  Account:     ${SMOKE_TEST_EMAIL}"
echo

# --- Step 1: capture the pre-test value of the write-error counter -----------

echo "1. Reading pre-test metrics from ${RESERVOIR_BASE_URL}/metrics"
PRE_METRICS=$(curl -fsS --max-time "${HTTP_TIMEOUT_SECONDS}" \
  "${RESERVOIR_BASE_URL}/metrics") \
  || fail "Could not fetch /metrics. Is Reservoir reachable?"

# Sum all label permutations of reservoir_auth_db_write_errors_total.
# The metric is labelled by operation, so multiple lines exist.
PRE_ERRORS=$(echo "${PRE_METRICS}" \
  | awk '/^reservoir_auth_db_write_errors_total{/ { gsub("[^0-9.]", "", $2); sum += $2 } END { print sum+0 }')
note "reservoir_auth_db_write_errors_total (pre): ${PRE_ERRORS}"

# --- Step 2: capture the pre-test last_logged_on value -----------------------

if [[ "${LAST_LOGIN_CHECK_METHOD}" == "LMS_API" ]]; then
  : "${LMS_BASE_URL:?LMS_BASE_URL must be set when LAST_LOGIN_CHECK_METHOD=LMS_API}"
  : "${LMS_SMOKE_TOKEN:?LMS_SMOKE_TOKEN must be set (read-only token for the smoke account)}"

  echo "2. Reading pre-test last_logged_on for ${SMOKE_TEST_EMAIL} via LMS"
  PRE_LAST_LOGGED_ON=$(curl -fsS --max-time "${HTTP_TIMEOUT_SECONDS}" \
    -H "Authorization: Bearer ${LMS_SMOKE_TOKEN}" \
    "${LMS_BASE_URL}/api/users/me/last_logged_on") \
    || fail "Could not read last_logged_on from LMS"
  note "last_logged_on (pre): ${PRE_LAST_LOGGED_ON}"
else
  echo "2. Skipping last_logged_on verification (LAST_LOGIN_CHECK_METHOD=SKIP)"
  PRE_LAST_LOGGED_ON=""
fi

# --- Step 3: hit POST /auth/login --------------------------------------------

echo "3. POST ${RESERVOIR_BASE_URL}/auth/login"
LOGIN_HTTP_CODE=$(curl -sS -o /tmp/smoke-login-body --write-out "%{http_code}" \
  --max-time "${HTTP_TIMEOUT_SECONDS}" \
  -H 'Content-Type: application/json' \
  -X POST \
  -d "{\"email\":\"${SMOKE_TEST_EMAIL}\",\"password\":\"${SMOKE_TEST_PASSWORD}\"}" \
  "${RESERVOIR_BASE_URL}/auth/login")

if [[ "${LOGIN_HTTP_CODE}" != "200" ]]; then
  red "Login returned HTTP ${LOGIN_HTTP_CODE}"
  cat /tmp/smoke-login-body
  fail "Login did not return 200"
fi

# Confirm the body shape is what we expect; a 200 with the wrong shape
# usually means something replaced the handler.
if ! grep -q 'access_token\|"token"' /tmp/smoke-login-body; then
  cat /tmp/smoke-login-body
  fail "Login response shape doesn't include an access token field"
fi
note "Login returned 200 with a token field"

# --- Step 4: wait for the async writer to flush ------------------------------

echo "4. Waiting up to ${WRITE_VISIBILITY_DEADLINE_SECONDS}s for last_logged_on to advance"

if [[ "${LAST_LOGIN_CHECK_METHOD}" == "LMS_API" ]]; then
  ELAPSED=0
  ADVANCED=false
  while (( ELAPSED < WRITE_VISIBILITY_DEADLINE_SECONDS )); do
    sleep 2
    ELAPSED=$((ELAPSED + 2))
    POST_LAST_LOGGED_ON=$(curl -fsS --max-time "${HTTP_TIMEOUT_SECONDS}" \
      -H "Authorization: Bearer ${LMS_SMOKE_TOKEN}" \
      "${LMS_BASE_URL}/api/users/me/last_logged_on") || continue

    if [[ "${POST_LAST_LOGGED_ON}" != "${PRE_LAST_LOGGED_ON}" ]]; then
      ADVANCED=true
      note "last_logged_on advanced from ${PRE_LAST_LOGGED_ON} to ${POST_LAST_LOGGED_ON} at t+${ELAPSED}s"
      break
    fi
  done

  if [[ "${ADVANCED}" != "true" ]]; then
    fail "last_logged_on did NOT advance within ${WRITE_VISIBILITY_DEADLINE_SECONDS}s. Either the async writer is broken or the DB connection is read-only."
  fi
else
  sleep 8
  note "Sleep elapsed; write-visibility check skipped"
fi

# --- Step 5: confirm the write-error counter didn't increase -----------------

echo "5. Re-reading /metrics to confirm reservoir_auth_db_write_errors_total is flat"
POST_METRICS=$(curl -fsS --max-time "${HTTP_TIMEOUT_SECONDS}" \
  "${RESERVOIR_BASE_URL}/metrics") \
  || fail "Could not re-fetch /metrics"

POST_ERRORS=$(echo "${POST_METRICS}" \
  | awk '/^reservoir_auth_db_write_errors_total{/ { gsub("[^0-9.]", "", $2); sum += $2 } END { print sum+0 }')
note "reservoir_auth_db_write_errors_total (post): ${POST_ERRORS}"

if (( $(echo "${POST_ERRORS} > ${PRE_ERRORS}" | bc -l) )); then
  red "reservoir_auth_db_write_errors_total increased: ${PRE_ERRORS} -> ${POST_ERRORS}"
  fail "Write errors observed during smoke. DO NOT enable Reservoir in the LMS auth path."
fi

# --- Done --------------------------------------------------------------------

green "PASS: write path is healthy in ${ENVIRONMENT}"
exit 0
