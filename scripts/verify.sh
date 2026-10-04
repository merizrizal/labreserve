#!/bin/sh
set -eu

PROJECT=labreserve-verify
compose() {
  docker compose --project-name "$PROJECT" --file compose.yaml --profile verify "$@"
}
cleanup() {
  compose down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

printf '%s\n' 'Building pinned application, PostgreSQL, Go-test, and Playwright images...'
compose build verify-migrate verify-seed verify-go verify-app e2e
printf '%s\n' 'Checking Go formatting...'
compose run --rm --no-deps verify-go sh -ec '
  unformatted=$(gofmt -l $(find cmd internal -type f -name "*.go"))
  if [ -n "$unformatted" ]; then
    printf "Go files require gofmt:\n%s\n" "$unformatted" >&2
    exit 1
  fi
'
printf '%s\n' 'Starting isolated disposable PostgreSQL verification database...'
compose up --wait -d verify-db
compose run --rm verify-migrate
compose run --rm verify-seed

assert_seed_state() {
  state=$(compose exec -T verify-db psql -X -qAt -U postgres -d labreserve_test -c "SELECT (SELECT count(*) FROM accounts WHERE login IN ('alex@example.test', 'sam@example.test', 'jordan@example.test'))::text || '|' || (SELECT count(*) FROM resources WHERE lower(code) IN ('net-01', 'k8s-01', 'demo-01'))::text || '|' || (SELECT count(*) FROM sessions)::text || '|' || (SELECT count(*) FROM schema_migrations)::text")
  if [ "$state" != "3|3|0|2" ]; then
    printf 'Unexpected seeded database state: %s\n' "$state" >&2
    return 1
  fi
}

assert_seed_state
printf '%s\n' 'Restarting PostgreSQL and confirming committed foundation data remains...'
compose restart verify-db
ready=false
attempt=0
while [ "$attempt" -lt 30 ]; do
  if compose exec -T verify-db pg_isready -U postgres -d labreserve_test >/dev/null 2>&1; then
    ready=true
    break
  fi
  attempt=$((attempt + 1))
  sleep 1
done
if [ "$ready" != true ]; then
  printf '%s\n' 'Verification PostgreSQL did not become ready after restart.' >&2
  exit 1
fi
compose run --rm verify-seed
assert_seed_state

printf '%s\n' 'Running Go unit and real-PostgreSQL integration tests...'
compose run --rm verify-go
printf '%s\n' 'Starting the app with the restricted runtime database role...'
compose up --wait -d verify-app
printf '%s\n' 'Running Chromium browser verification in America/New_York...'
compose run --rm e2e

sessions_before=$(compose exec -T verify-db psql -X -qAt -U postgres -d labreserve_test -c "SELECT count(*) FROM sessions")
if [ "$sessions_before" -lt 1 ]; then
  printf '%s\n' 'Expected browser verification to leave a server-side session for restart verification.' >&2
  exit 1
fi
printf '%s\n' 'Restarting PostgreSQL and the app; checking retained sessions and seeded data...'
compose restart verify-db
compose up --wait -d verify-db
compose restart verify-app
compose up --wait -d verify-app
state=$(compose exec -T verify-db psql -X -qAt -U postgres -d labreserve_test -c "SELECT (SELECT count(*) FROM accounts WHERE login IN ('alex@example.test', 'sam@example.test', 'jordan@example.test'))::text || '|' || (SELECT count(*) FROM resources WHERE lower(code) IN ('net-01', 'k8s-01', 'demo-01'))::text || '|' || (SELECT count(*) FROM sessions)::text || '|' || (SELECT count(*) FROM schema_migrations)::text")
expected_state="3|3|$sessions_before|2"
if [ "$state" != "$expected_state" ]; then
  printf 'Application/database restart changed retained state: %s (expected %s)\n' "$state" "$expected_state" >&2
  exit 1
fi
printf '%s\n' 'Repository verification passed.'
