-- Add a per-user token_version used to revoke all outstanding JWTs at once.
-- Every issued access/refresh token embeds the user's current token_version as
-- a claim; logout increments this column, after which any token carrying the
-- old version is rejected on refresh. This closes the gap where a stolen
-- refresh token stayed valid for its full 30-day TTL after logout
-- (security review Finding 2 / LMS-6513).
--
-- Default 0 so that tokens minted before this migration (which carry no tver
-- claim, decoded as 0) still match their user's version and keep working until
-- they expire or the user next logs out. This avoids forcing every active
-- session to re-authenticate at deploy. From the first logout onward the column
-- is bumped and revocation takes effect.
--
-- If you instead want to invalidate ALL currently-outstanding tokens at deploy
-- (e.g. as a precaution against already-leaked refresh tokens), follow this
-- migration with: UPDATE users SET token_version = 1;
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS token_version INTEGER NOT NULL DEFAULT 0;
