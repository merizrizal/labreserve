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
  if [ "$state" != "3|3|0|5" ]; then
    printf 'Unexpected seeded database state: %s\n' "$state" >&2
    return 1
  fi
}

assert_cancellation_restart_state() {
  state=$(compose exec -T verify-db psql -X -qAt -U postgres -d labreserve_test -c "SELECT (SELECT count(*) FROM bookings b JOIN resources r ON r.id = b.resource_id WHERE r.code = 'TASK-3B-RESTART' AND b.state = 'cancelled')::text || '|' || (SELECT count(*) FROM bookings b JOIN resources r ON r.id = b.resource_id WHERE r.code = 'TASK-3B-RESTART' AND b.cancelled_by_account_id IS NOT NULL AND b.cancelled_at IS NOT NULL)::text || '|' || (SELECT count(*) FROM activity_events e JOIN bookings b ON b.id = e.booking_id JOIN resources r ON r.id = b.resource_id WHERE r.code = 'TASK-3B-RESTART' AND e.action = 'booking.created')::text || '|' || (SELECT count(*) FROM activity_events e JOIN bookings b ON b.id = e.booking_id JOIN resources r ON r.id = b.resource_id WHERE r.code = 'TASK-3B-RESTART' AND e.action = 'booking.cancelled')::text")
  if [ "$state" != "1|1|1|1" ]; then
    printf 'Unexpected retained owner-cancellation state: %s\n' "$state" >&2
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
printf '%s\n' 'Preparing Resource and owner-cancellation records for database restart verification...'
compose run --rm -e LABRESERVE_TEST_RESOURCE_RESTART_FIXTURE=1 verify-go go test -run '^TestResourceMutationRestartFixture$' ./internal/database
compose run --rm -e LABRESERVE_TEST_CANCELLATION_RESTART_FIXTURE=1 verify-go go test -run '^TestOwnerCancellationRestartFixture$' ./internal/database
assert_cancellation_restart_state
compose run --rm verify-seed
printf '%s\n' 'Restarting PostgreSQL and the app; checking retained sessions, seeded data, Resource mutations, and cancellation history...'
compose restart verify-db
compose up --wait -d verify-db
compose run --rm verify-seed
compose restart verify-app
compose up --wait -d verify-app
state=$(compose exec -T verify-db psql -X -qAt -U postgres -d labreserve_test -c "SELECT (SELECT count(*) FROM accounts WHERE login IN ('alex@example.test', 'sam@example.test', 'jordan@example.test'))::text || '|' || (SELECT count(*) FROM resources WHERE lower(code) IN ('net-01', 'k8s-01', 'demo-01'))::text || '|' || (SELECT count(*) FROM sessions)::text || '|' || (SELECT count(*) FROM schema_migrations)::text")
expected_state="3|3|$sessions_before|5"
if [ "$state" != "$expected_state" ]; then
  printf 'Application/database restart changed retained state: %s (expected %s)\n' "$state" "$expected_state" >&2
  exit 1
fi
printf '%s\n' 'Verifying Resource durability, replay, no-op, duplicate rejection, and runtime capabilities after restart...'
compose run --rm -e LABRESERVE_TEST_RESOURCE_RESTART_REPLAY=1 verify-go go test -run '^TestResourceMutationRestartReplay$' ./internal/database
printf '%s\n' 'Repeating authorized cancellation after database and app restart...'
compose run --rm -e LABRESERVE_TEST_CANCELLATION_RESTART_REPLAY=1 verify-go go test -run '^TestOwnerCancellationRestartReplay$' ./internal/database
assert_cancellation_restart_state
printf '%s\n' 'Repository verification passed.'
