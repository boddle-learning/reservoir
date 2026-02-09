// Load Testing Script for Boddle Auth Gateway
// Tool: k6 (https://k6.io/)
//
// Installation:
//   brew install k6  (macOS)
//   or visit https://k6.io/docs/getting-started/installation/
//
// Usage:
//   k6 run load-test.js
//
// Customize:
//   k6 run --vus 100 --duration 5m load-test.js

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// Custom metrics
const loginSuccessRate = new Rate('login_success');
const loginDuration = new Trend('login_duration');
const tokenValidationDuration = new Trend('token_validation_duration');

// Configuration
const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const RAILS_URL = __ENV.RAILS_URL || 'http://localhost:3000';

// Load test stages
export const options = {
  stages: [
    { duration: '1m', target: 50 },   // Ramp up to 50 users
    { duration: '3m', target: 50 },   // Stay at 50 users
    { duration: '1m', target: 200 },  // Ramp up to 200 users
    { duration: '3m', target: 200 },  // Stay at 200 users
    { duration: '1m', target: 500 },  // Spike to 500 users
    { duration: '2m', target: 500 },  // Stay at 500 users
    { duration: '2m', target: 0 },    // Ramp down to 0 users
  ],
  thresholds: {
    http_req_duration: ['p(95)<500', 'p(99)<1000'], // 95% < 500ms, 99% < 1s
    http_req_failed: ['rate<0.01'],                  // Error rate < 1%
    login_success: ['rate>0.99'],                    // Login success rate > 99%
    login_duration: ['p(95)<500'],                   // Login p95 < 500ms
    token_validation_duration: ['p(95)<100'],        // Token validation p95 < 100ms
  },
};

// Test data
const TEST_USERS = [
  { email: 'teacher1@example.com', password: 'password123' },
  { email: 'teacher2@example.com', password: 'password123' },
  { email: 'student1@student.student', password: 'password123' },
  { email: 'student2@student.student', password: 'password123' },
  { email: 'parent1@example.com', password: 'password123' },
];

export function setup() {
  console.log('🚀 Starting load test...');
  console.log(`   Base URL: ${BASE_URL}`);
  console.log(`   Rails URL: ${RAILS_URL}`);
  console.log(`   Test users: ${TEST_USERS.length}`);
  return { startTime: Date.now() };
}

export default function () {
  // Select a random user
  const user = TEST_USERS[Math.floor(Math.random() * TEST_USERS.length)];

  // Test 1: Email/Password Login
  testEmailPasswordLogin(user);

  sleep(1);

  // Test 2: Login Token Authentication
  testLoginToken();

  sleep(1);

  // Test 3: OAuth Flow (Google)
  testOAuthFlow();

  sleep(2);
}

function testEmailPasswordLogin(user) {
  const payload = JSON.stringify({
    email: user.email,
    password: user.password,
  });

  const params = {
    headers: { 'Content-Type': 'application/json' },
    tags: { name: 'EmailPasswordLogin' },
  };

  const startTime = Date.now();
  const response = http.post(`${BASE_URL}/auth/login`, payload, params);
  const duration = Date.now() - startTime;

  const success = check(response, {
    'login status is 200': (r) => r.status === 200,
    'login response has token': (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.success && body.data && body.data.token;
      } catch (e) {
        return false;
      }
    },
  });

  loginSuccessRate.add(success);
  loginDuration.add(duration);

  if (success) {
    const token = JSON.parse(response.body).data.token.access_token;

    // Test token validation by calling /auth/me
    testTokenValidation(token);

    // Test Rails JWT validation
    testRailsJwtValidation(token);
  }
}

function testTokenValidation(token) {
  const params = {
    headers: { 'Authorization': `Bearer ${token}` },
    tags: { name: 'TokenValidation' },
  };

  const startTime = Date.now();
  const response = http.get(`${BASE_URL}/auth/me`, params);
  const duration = Date.now() - startTime;

  check(response, {
    'me endpoint status is 200': (r) => r.status === 200,
    'me endpoint returns user': (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.success && body.data && body.data.user;
      } catch (e) {
        return false;
      }
    },
  });

  tokenValidationDuration.add(duration);
}

function testRailsJwtValidation(token) {
  const params = {
    headers: { 'Authorization': `Bearer ${token}` },
    tags: { name: 'RailsJwtValidation' },
  };

  const response = http.get(`${RAILS_URL}/api/v1/classrooms`, params);

  check(response, {
    'rails jwt validation works': (r) => r.status === 200 || r.status === 404, // 404 is ok if endpoint doesn't exist
    'rails jwt not unauthorized': (r) => r.status !== 401,
  });
}

function testLoginToken() {
  // Note: This requires a valid login token to exist
  // In a real test, you would generate tokens via Rails or use test fixtures
  const testToken = __ENV.TEST_LOGIN_TOKEN;

  if (!testToken) {
    // Skip if no test token provided
    return;
  }

  const params = {
    tags: { name: 'LoginToken' },
  };

  const response = http.get(`${BASE_URL}/auth/token?token=${testToken}`, params);

  check(response, {
    'login token status is 200': (r) => r.status === 200,
    'login token response has jwt': (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.success && body.data && body.data.token;
      } catch (e) {
        return false;
      }
    },
  });
}

function testOAuthFlow() {
  // Note: OAuth flow is difficult to test with k6 as it requires browser redirects
  // This test just checks that the OAuth endpoints are responsive

  const params = {
    tags: { name: 'OAuth' },
    redirects: 0, // Don't follow redirects
  };

  const googleResponse = http.get(`${BASE_URL}/auth/google?redirect_url=/dashboard`, params);
  check(googleResponse, {
    'google oauth endpoint redirects': (r) => r.status === 307,
  });

  const cleverResponse = http.get(`${BASE_URL}/auth/clever?redirect_url=/dashboard`, params);
  check(cleverResponse, {
    'clever oauth endpoint redirects': (r) => r.status === 307,
  });

  const icloudResponse = http.get(`${BASE_URL}/auth/icloud?redirect_url=/dashboard`, params);
  check(icloudResponse, {
    'icloud oauth endpoint redirects': (r) => r.status === 307,
  });
}

export function teardown(data) {
  const duration = (Date.now() - data.startTime) / 1000 / 60;
  console.log(`✅ Load test completed in ${duration.toFixed(2)} minutes`);
  console.log('📊 Check the summary above for detailed metrics');
}

// Example run commands:
//
// Standard load test:
//   k6 run load-test.js
//
// Quick smoke test:
//   k6 run --vus 10 --duration 30s load-test.js
//
// Stress test:
//   k6 run --vus 1000 --duration 5m load-test.js
//
// With custom URLs:
//   k6 run --env BASE_URL=https://auth.boddle.com load-test.js
//
// With login token:
//   k6 run --env TEST_LOGIN_TOKEN=abc123 load-test.js
