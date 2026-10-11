package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"labreserve.local/labreserve/internal/auth"
	"labreserve.local/labreserve/internal/config"
	"labreserve.local/labreserve/internal/database"
)

const cancellationHTTPOrigin = "http://labreserve.test"

type cancellationHTTPApp struct {
	server        *httptest.Server
	migrationDB   *pgxpool.Pool
	resourceID    string
	fixturePrefix string
	fixedNow      time.Time
	setNow        func(time.Time)
	jakarta       *time.Location
}

type cancellationHTTPBooking struct {
	id    string
	start time.Time
	end   time.Time
}

type cancellationHTTPSnapshot struct {
	state                   string
	cancelledBy             string
	hasCancelledAt          bool
	cancelledAt             time.Time
	creationEvents          int
	cancellationEvents      int
	actorCancellationEvents int
}

func newCancellationHTTPApp(t *testing.T) *cancellationHTTPApp {
	t.Helper()
	runtimePool := webTestPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	migrationPool := webTestPool(t, "LABRESERVE_TEST_DATABASE_URL")
	jakarta, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	fixedNow := time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC)
	var clockMu sync.RWMutex
	clockValue := fixedNow
	now := func() time.Time {
		clockMu.RLock()
		defer clockMu.RUnlock()
		return clockValue
	}
	setNow := func(value time.Time) {
		clockMu.Lock()
		clockValue = value
		clockMu.Unlock()
	}
	store := database.NewStore(runtimePool)
	application, err := New(store, auth.NewService(store, now), config.Config{
		DatabaseURL: "postgres://test", ListenAddr: "127.0.0.1:8080", PublicOrigin: cancellationHTTPOrigin,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(application.Handler())
	fixturePrefix := fmt.Sprintf("task3c-http-%d", time.Now().UnixNano())
	resourceCode := fmt.Sprintf("T3C-%d", time.Now().UnixNano())
	var resourceID string
	if err := migrationPool.QueryRow(t.Context(), `
		INSERT INTO resources (id, code, name, description)
		VALUES (gen_random_uuid(), $1, 'Task 3C Cancellation Test Resource', '')
		RETURNING id::text`, resourceCode).Scan(&resourceID); err != nil {
		server.Close()
		t.Fatalf("insert Task 3C Resource fixture: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := migrationPool.Exec(cleanupCtx, `
			DELETE FROM activity_events WHERE booking_id IN (
				SELECT id FROM bookings WHERE purpose LIKE $1
			)`, fixturePrefix+"%"); err != nil {
			t.Errorf("remove Task 3C Activity fixtures: %v", err)
		}
		if _, err := migrationPool.Exec(cleanupCtx, "DELETE FROM bookings WHERE purpose LIKE $1", fixturePrefix+"%"); err != nil {
			t.Errorf("remove Task 3C Booking fixtures: %v", err)
		}
		if _, err := migrationPool.Exec(cleanupCtx, "DELETE FROM resources WHERE id = $1", resourceID); err != nil {
			t.Errorf("remove Task 3C Resource fixture: %v", err)
		}
	})
	t.Cleanup(server.Close)
	return &cancellationHTTPApp{
		server: server, migrationDB: migrationPool, resourceID: resourceID,
		fixturePrefix: fixturePrefix, fixedNow: fixedNow, setNow: setNow, jakarta: jakarta,
	}
}

func (app *cancellationHTTPApp) createBooking(t *testing.T, client *http.Client, start time.Time, suffix string) cancellationHTTPBooking {
	t.Helper()
	formURL := app.server.URL + "/resources/" + app.resourceID + "/bookings/new"
	status, _, csrf, requestID := getBookingHTTPForm(t, client, formURL)
	if status != http.StatusOK {
		t.Fatalf("Task 3C booking form status = %d", status)
	}
	end := start.Add(30 * time.Minute)
	values := bookingHTTPValues(
		csrf, requestID,
		start.In(app.jakarta).Format(jakartaDateTimeLayout),
		end.In(app.jakarta).Format(jakartaDateTimeLayout),
		app.fixturePrefix+" "+suffix,
	)
	response := bookingHTTPPost(t, client, app.server.URL+"/resources/"+app.resourceID+"/bookings", cancellationHTTPOrigin, values)
	location := response.Header.Get("Location")
	response.Body.Close()
	if response.StatusCode != http.StatusSeeOther || !strings.HasPrefix(location, "/bookings/") || !strings.HasSuffix(location, "?result=created") {
		t.Fatalf("create Task 3C fixture via existing workflow: status=%d location=%q", response.StatusCode, location)
	}
	bookingID := strings.TrimSuffix(strings.TrimPrefix(location, "/bookings/"), "?result=created")
	if !uuidPattern.MatchString(bookingID) {
		t.Fatalf("fixture creation redirected to invalid Booking ID %q", location)
	}
	return cancellationHTTPBooking{id: bookingID, start: start, end: end}
}

func cancellationHTTPFormToken(t *testing.T, app *cancellationHTTPApp, client *http.Client) string {
	t.Helper()
	status, _, csrf, _ := getBookingHTTPForm(t, client,
		app.server.URL+"/resources/"+app.resourceID+"/bookings/new")
	if status != http.StatusOK {
		t.Fatalf("authenticated form for CSRF token status = %d", status)
	}
	return csrf
}

func cancellationHTTPSnapshotFor(t *testing.T, pool *pgxpool.Pool, bookingID string) cancellationHTTPSnapshot {
	t.Helper()
	var snapshot cancellationHTTPSnapshot
	err := pool.QueryRow(t.Context(), `
		SELECT b.state, COALESCE(b.cancelled_by_account_id::text, ''),
		       b.cancelled_at IS NOT NULL, COALESCE(b.cancelled_at, 'epoch'::timestamptz),
		       (SELECT count(*)::int FROM activity_events e WHERE e.booking_id = b.id AND e.action = 'booking.created'),
		       (SELECT count(*)::int FROM activity_events e WHERE e.booking_id = b.id AND e.action = 'booking.cancelled'),
		       (SELECT count(*)::int FROM activity_events e
		        WHERE e.booking_id = b.id AND e.action = 'booking.cancelled' AND e.actor_account_id = b.cancelled_by_account_id)
		FROM bookings b WHERE b.id = $1`, bookingID).Scan(
		&snapshot.state, &snapshot.cancelledBy, &snapshot.hasCancelledAt, &snapshot.cancelledAt,
		&snapshot.creationEvents, &snapshot.cancellationEvents, &snapshot.actorCancellationEvents)
	if err != nil {
		t.Fatalf("read Task 3C cancellation state for %s: %v", bookingID, err)
	}
	return snapshot
}

func assertCancellationHTTPSnapshot(t *testing.T, pool *pgxpool.Pool, bookingID, state, actorID string, creationEvents, cancellationEvents int) cancellationHTTPSnapshot {
	t.Helper()
	snapshot := cancellationHTTPSnapshotFor(t, pool, bookingID)
	if snapshot.state != state || snapshot.cancelledBy != actorID || snapshot.hasCancelledAt != (state == "cancelled") ||
		snapshot.creationEvents != creationEvents || snapshot.cancellationEvents != cancellationEvents ||
		snapshot.actorCancellationEvents != cancellationEvents {
		t.Fatalf("Task 3C Booking snapshot = %+v; want state=%s actor=%s events=%d/%d",
			snapshot, state, actorID, creationEvents, cancellationEvents)
	}
	return snapshot
}

func TestOwnerCancellationHTTPWorkflowAuthorizationAndReplay(t *testing.T) {
	app := newCancellationHTTPApp(t)
	alexID := accountIDForLogin(t, app.migrationDB, "alex@example.test")
	jordanID := accountIDForLogin(t, app.migrationDB, "jordan@example.test")
	alexClient := loginBookingHTTPUser(t, app.server.URL, "alex@example.test", requiredTestPassword(t, "SEED_ALEX_PASSWORD"))
	samClient := loginBookingHTTPUser(t, app.server.URL, "sam@example.test", requiredTestPassword(t, "SEED_SAM_PASSWORD"))
	jordanClient := loginBookingHTTPUser(t, app.server.URL, "jordan@example.test", requiredTestPassword(t, "SEED_JORDAN_PASSWORD"))
	alexCSRF := cancellationHTTPFormToken(t, app, alexClient)
	samCSRF := cancellationHTTPFormToken(t, app, samClient)
	jordanCSRF := cancellationHTTPFormToken(t, app, jordanClient)
	baseStart := app.fixedNow.Add(time.Hour).Truncate(time.Minute)
	mainBooking := app.createBooking(t, alexClient, baseStart, "owner cancellation")
	jordanBooking := app.createBooking(t, jordanClient, baseStart.Add(time.Hour), "Coordinator self-cancellation")
	inactiveBooking := app.createBooking(t, alexClient, baseStart.Add(2*time.Hour), "inactive Resource cancellation")

	mainAction := "/bookings/" + mainBooking.id + "/cancel"
	ownerDetail := mustGet(t, alexClient, app.server.URL+"/bookings/"+mainBooking.id)
	ownerDetailBody := readAndClose(t, ownerDetail)
	cancelFormStart := strings.Index(ownerDetailBody, `<form action="`+mainAction+`"`)
	cancelForm := ""
	if cancelFormStart >= 0 {
		if cancelFormEnd := strings.Index(ownerDetailBody[cancelFormStart:], "</form>"); cancelFormEnd >= 0 {
			cancelForm = ownerDetailBody[cancelFormStart : cancelFormStart+cancelFormEnd]
		}
	}
	if ownerDetail.StatusCode != http.StatusOK || cancelForm == "" || !strings.Contains(cancelForm, "Cancel booking") ||
		!strings.Contains(cancelForm, `name="_csrf" value="`+alexCSRF+`"`) {
		t.Fatalf("future owner detail did not render a CSRF cancellation form: status=%d body=%s", ownerDetail.StatusCode, ownerDetailBody)
	}
	nonOwnerDetail := mustGet(t, samClient, app.server.URL+"/bookings/"+mainBooking.id)
	nonOwnerDetailBody := readAndClose(t, nonOwnerDetail)
	if nonOwnerDetail.StatusCode != http.StatusOK || !strings.Contains(nonOwnerDetailBody, "Booking details") || strings.Contains(nonOwnerDetailBody, mainAction) || strings.Contains(nonOwnerDetailBody, "Cancel booking") {
		t.Fatalf("shared Booking detail visibility or owner-only action failed: status=%d body=%s", nonOwnerDetail.StatusCode, nonOwnerDetailBody)
	}

	for _, test := range []struct {
		name   string
		origin string
		values url.Values
	}{
		{name: "missing CSRF", origin: cancellationHTTPOrigin, values: url.Values{}},
		{name: "invalid CSRF", origin: cancellationHTTPOrigin, values: url.Values{"_csrf": {"invalid-csrf-token"}}},
		{name: "unexpected Origin", origin: "https://attacker.example", values: url.Values{"_csrf": {alexCSRF}}},
	} {
		response := bookingHTTPPost(t, alexClient, app.server.URL+mainAction, test.origin, test.values)
		body := readAndClose(t, response)
		if response.StatusCode != http.StatusForbidden {
			t.Errorf("%s cancellation status = %d body=%s; want 403", test.name, response.StatusCode, body)
		}
		assertCancellationHTTPSnapshot(t, app.migrationDB, mainBooking.id, "confirmed", "", 1, 0)
	}

	forgedValues := url.Values{
		"_csrf": {samCSRF}, "owner_id": {alexID}, "account_id": {alexID}, "role": {"coordinator"},
		"state": {"cancelled"}, "cancelled_by_account_id": {alexID}, "cancelled_at": {app.fixedNow.Format(time.RFC3339Nano)},
	}
	unauthorized := bookingHTTPPost(t, samClient, app.server.URL+mainAction, cancellationHTTPOrigin, forgedValues)
	unauthorizedBody := readAndClose(t, unauthorized)
	if unauthorized.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-owner forged cancellation status = %d body=%s; want 404", unauthorized.StatusCode, unauthorizedBody)
	}
	coordinatorForgery := url.Values{
		"_csrf": {jordanCSRF}, "owner_id": {jordanID}, "role": {"coordinator"},
	}
	coordinatorUnauthorized := bookingHTTPPost(t, jordanClient, app.server.URL+mainAction, cancellationHTTPOrigin, coordinatorForgery)
	coordinatorBody := readAndClose(t, coordinatorUnauthorized)
	if coordinatorUnauthorized.StatusCode != http.StatusNotFound || coordinatorBody != unauthorizedBody {
		t.Fatalf("Coordinator cross-owner outcome = %d %q; want indistinguishable 404 %q", coordinatorUnauthorized.StatusCode, coordinatorBody, unauthorizedBody)
	}
	unknown := bookingHTTPPost(t, alexClient, app.server.URL+"/bookings/44444444-4444-4444-8444-444444444444/cancel",
		cancellationHTTPOrigin, url.Values{"_csrf": {alexCSRF}})
	unknownBody := readAndClose(t, unknown)
	if unknown.StatusCode != http.StatusNotFound || unknownBody != unauthorizedBody {
		t.Fatalf("unknown Booking outcome = %d %q; want indistinguishable 404 %q", unknown.StatusCode, unknownBody, unauthorizedBody)
	}
	assertCancellationHTTPSnapshot(t, app.migrationDB, mainBooking.id, "confirmed", "", 1, 0)

	readOnlyTargets := []struct {
		name   string
		target string
		want   int
	}{
		{name: "Booking detail", target: "/bookings/" + mainBooking.id + "?cancel=1", want: http.StatusOK},
		{name: "My Bookings", target: "/my-bookings?cancel=" + mainBooking.id, want: http.StatusOK},
		{name: "Resource schedule", target: "/resources/" + app.resourceID + "?date=" + mainBooking.start.In(app.jakarta).Format(jakartaDateLayout) + "&cancel=" + mainBooking.id, want: http.StatusOK},
		{name: "GET cancellation path", target: mainAction, want: http.StatusMethodNotAllowed},
	}
	for _, test := range readOnlyTargets {
		response := mustGet(t, alexClient, app.server.URL+test.target)
		body := readAndClose(t, response)
		if response.StatusCode != test.want {
			t.Errorf("%s GET status = %d body=%s; want %d", test.name, response.StatusCode, body, test.want)
		}
		assertCancellationHTTPSnapshot(t, app.migrationDB, mainBooking.id, "confirmed", "", 1, 0)
	}

	firstCancel := bookingHTTPPost(t, alexClient, app.server.URL+mainAction, cancellationHTTPOrigin, url.Values{"_csrf": {alexCSRF}})
	firstLocation := firstCancel.Header.Get("Location")
	firstCancel.Body.Close()
	if firstCancel.StatusCode != http.StatusSeeOther || firstLocation != "/bookings/"+mainBooking.id {
		t.Fatalf("first cancellation did not use successful PRG: status=%d location=%q", firstCancel.StatusCode, firstLocation)
	}
	firstSnapshot := assertCancellationHTTPSnapshot(t, app.migrationDB, mainBooking.id, "cancelled", alexID, 1, 1)
	var eventActor string
	if err := app.migrationDB.QueryRow(t.Context(), `
		SELECT actor_account_id::text FROM activity_events
		WHERE booking_id = $1 AND action = 'booking.cancelled'`, mainBooking.id).Scan(&eventActor); err != nil || eventActor != alexID {
		t.Fatalf("cancellation event actor = %q, %v; want authenticated owner %s", eventActor, err, alexID)
	}
	cancelledDetail := mustGet(t, alexClient, app.server.URL+firstLocation)
	cancelledDetailBody := readAndClose(t, cancelledDetail)
	if cancelledDetail.StatusCode != http.StatusOK || !strings.Contains(cancelledDetailBody, "Cancelled") || strings.Contains(cancelledDetailBody, mainAction) || strings.Contains(cancelledDetailBody, "Cancel booking") {
		t.Fatalf("retained cancelled detail or action visibility failed: status=%d body=%s", cancelledDetail.StatusCode, cancelledDetailBody)
	}
	myBookings := mustGet(t, alexClient, app.server.URL+"/my-bookings")
	myBookingsBody := readAndClose(t, myBookings)
	if myBookings.StatusCode != http.StatusOK || !strings.Contains(myBookingsBody, mainBooking.id) || !strings.Contains(myBookingsBody, "Cancelled") || strings.Contains(myBookingsBody, "Cancel booking") {
		t.Fatalf("My Bookings did not retain cancellation read-only: status=%d body=%s", myBookings.StatusCode, myBookingsBody)
	}
	schedule := mustGet(t, alexClient, app.server.URL+"/resources/"+app.resourceID+"?date="+mainBooking.start.In(app.jakarta).Format(jakartaDateLayout))
	scheduleBody := readAndClose(t, schedule)
	if schedule.StatusCode != http.StatusOK || !strings.Contains(scheduleBody, mainBooking.id) || !strings.Contains(scheduleBody, "Cancelled bookings do not occupy the resource.") {
		t.Fatalf("Resource schedule did not retain Cancelled history: status=%d body=%s", schedule.StatusCode, scheduleBody)
	}

	replacement := app.createBooking(t, samClient, mainBooking.start, "replacement after owner cancellation")
	if replacement.start != mainBooking.start || replacement.end != mainBooking.end {
		t.Fatalf("replacement interval = %v–%v; want released interval %v–%v", replacement.start, replacement.end, mainBooking.start, mainBooking.end)
	}
	assertHTTPIntervalCounts(t, app.migrationDB, app.resourceID, mainBooking.start, mainBooking.end, 1, 2)

	app.setNow(mainBooking.start.Add(time.Microsecond))
	replay := bookingHTTPPost(t, alexClient, app.server.URL+mainAction, cancellationHTTPOrigin, url.Values{"_csrf": {alexCSRF}})
	replayLocation := replay.Header.Get("Location")
	replay.Body.Close()
	if replay.StatusCode != http.StatusSeeOther || replayLocation != "/bookings/"+mainBooking.id {
		t.Fatalf("authorized post-start cancellation replay = %d %q; want successful PRG", replay.StatusCode, replayLocation)
	}
	replaySnapshot := assertCancellationHTTPSnapshot(t, app.migrationDB, mainBooking.id, "cancelled", alexID, 1, 1)
	if replaySnapshot.cancelledAt != firstSnapshot.cancelledAt || replaySnapshot.cancelledBy != firstSnapshot.cancelledBy {
		t.Fatalf("authorized replay changed cancellation metadata: before=%+v after=%+v", firstSnapshot, replaySnapshot)
	}
	app.setNow(app.fixedNow)

	coordinatorCancel := bookingHTTPPost(t, jordanClient, app.server.URL+"/bookings/"+jordanBooking.id+"/cancel",
		cancellationHTTPOrigin, url.Values{"_csrf": {jordanCSRF}})
	coordinatorLocation := coordinatorCancel.Header.Get("Location")
	coordinatorCancel.Body.Close()
	if coordinatorCancel.StatusCode != http.StatusSeeOther || coordinatorLocation != "/bookings/"+jordanBooking.id {
		t.Fatalf("Coordinator self-cancellation did not use PRG: status=%d location=%q", coordinatorCancel.StatusCode, coordinatorLocation)
	}
	assertCancellationHTTPSnapshot(t, app.migrationDB, jordanBooking.id, "cancelled", jordanID, 1, 1)

	if _, err := app.migrationDB.Exec(t.Context(), "UPDATE resources SET active = FALSE WHERE id = $1", app.resourceID); err != nil {
		t.Fatalf("deactivate test Resource: %v", err)
	}
	inactiveDetail := mustGet(t, alexClient, app.server.URL+"/bookings/"+inactiveBooking.id)
	inactiveDetailBody := readAndClose(t, inactiveDetail)
	if inactiveDetail.StatusCode != http.StatusOK || !strings.Contains(inactiveDetailBody, "/bookings/"+inactiveBooking.id+"/cancel") {
		t.Fatalf("future Booking on an inactive Resource did not retain cancellation action: status=%d body=%s", inactiveDetail.StatusCode, inactiveDetailBody)
	}
	inactiveCancel := bookingHTTPPost(t, alexClient, app.server.URL+"/bookings/"+inactiveBooking.id+"/cancel",
		cancellationHTTPOrigin, url.Values{"_csrf": {alexCSRF}})
	inactiveLocation := inactiveCancel.Header.Get("Location")
	inactiveCancel.Body.Close()
	if inactiveCancel.StatusCode != http.StatusSeeOther || inactiveLocation != "/bookings/"+inactiveBooking.id {
		t.Fatalf("future Booking on inactive Resource cancellation = %d %q", inactiveCancel.StatusCode, inactiveLocation)
	}
	assertCancellationHTTPSnapshot(t, app.migrationDB, inactiveBooking.id, "cancelled", alexID, 1, 1)
}

func TestOwnerCancellationHTTPRejectsStartedBookingsAndHidesOperationalFailure(t *testing.T) {
	app := newCancellationHTTPApp(t)
	alexClient := loginBookingHTTPUser(t, app.server.URL, "alex@example.test", requiredTestPassword(t, "SEED_ALEX_PASSWORD"))
	csrf := cancellationHTTPFormToken(t, app, alexClient)
	baseStart := app.fixedNow.Add(time.Hour).Truncate(time.Minute)
	fixtures := []cancellationHTTPBooking{
		app.createBooking(t, alexClient, baseStart, "exact-start rejection"),
		app.createBooking(t, alexClient, baseStart.Add(time.Hour), "started rejection"),
		app.createBooking(t, alexClient, baseStart.Add(2*time.Hour), "past rejection"),
		app.createBooking(t, alexClient, baseStart.Add(3*time.Hour), "event-failure rollback"),
	}
	exactStartFixture := fixtures[0]
	app.setNow(exactStartFixture.start.Add(-time.Microsecond))
	eligibleDetail := mustGet(t, alexClient, app.server.URL+"/bookings/"+exactStartFixture.id)
	eligibleDetailBody := readAndClose(t, eligibleDetail)
	cancelAction := "/bookings/" + exactStartFixture.id + "/cancel"
	formStart := strings.Index(eligibleDetailBody, `<form action="`+cancelAction+`"`)
	if eligibleDetail.StatusCode != http.StatusOK || formStart < 0 {
		t.Fatalf("owner detail did not render cancellation form one microsecond before start: status=%d body=%s", eligibleDetail.StatusCode, eligibleDetailBody)
	}
	formEnd := strings.Index(eligibleDetailBody[formStart:], "</form>")
	if formEnd < 0 {
		t.Fatalf("rendered cancellation form is incomplete: %s", eligibleDetailBody)
	}
	cancellationForm := eligibleDetailBody[formStart : formStart+formEnd]
	if !strings.Contains(cancellationForm, "Cancel booking") || !strings.Contains(cancellationForm, `name="_csrf"`) {
		t.Fatalf("rendered cancellation form lacks its action or CSRF credential: %s", cancellationForm)
	}
	staleFormCSRF := csrfFrom(t, cancellationForm)
	assertCancellationHTTPSnapshot(t, app.migrationDB, exactStartFixture.id, "confirmed", "", 1, 0)
	readCreationEvent := func() string {
		var event string
		if err := app.migrationDB.QueryRow(t.Context(), `
			SELECT to_jsonb(e)::text FROM activity_events e
			WHERE e.booking_id = $1 AND e.action = 'booking.created'`, exactStartFixture.id).Scan(&event); err != nil {
			t.Fatalf("read original creation Activity event: %v", err)
		}
		return event
	}
	creationEventBefore := readCreationEvent()

	app.setNow(exactStartFixture.start)
	staleResponse := bookingHTTPPost(t, alexClient, app.server.URL+cancelAction,
		cancellationHTTPOrigin, url.Values{"_csrf": {staleFormCSRF}})
	staleBody := readAndClose(t, staleResponse)
	if staleResponse.StatusCode != http.StatusConflict || !strings.Contains(staleBody, "can no longer be cancelled") ||
		!strings.Contains(staleBody, `href="/bookings/`+exactStartFixture.id+`"`) || strings.Contains(staleBody, "ErrCancellationStarted") {
		t.Fatalf("previously eligible form POSTed at the exact start = %d body=%s; want safe 409 and no cancellation", staleResponse.StatusCode, staleBody)
	}
	assertCancellationHTTPSnapshot(t, app.migrationDB, exactStartFixture.id, "confirmed", "", 1, 0)
	if creationEventAfter := readCreationEvent(); creationEventAfter != creationEventBefore {
		t.Fatalf("stale cancellation changed the original creation Activity event: before=%s after=%s", creationEventBefore, creationEventAfter)
	}
	atStartDetail := mustGet(t, alexClient, app.server.URL+"/bookings/"+exactStartFixture.id)
	atStartDetailBody := readAndClose(t, atStartDetail)
	if atStartDetail.StatusCode != http.StatusOK || strings.Contains(atStartDetailBody, cancelAction) || strings.Contains(atStartDetailBody, "Cancel booking") {
		t.Fatalf("detail at the exact start still exposed cancellation: status=%d body=%s", atStartDetail.StatusCode, atStartDetailBody)
	}

	timeCases := []struct {
		name  string
		index int
		now   time.Time
	}{
		{name: "after start", index: 1, now: fixtures[1].start.Add(time.Microsecond)},
		{name: "past end", index: 2, now: fixtures[2].end.Add(time.Microsecond)},
	}
	for _, test := range timeCases {
		fixture := fixtures[test.index]
		app.setNow(test.now)
		detail := mustGet(t, alexClient, app.server.URL+"/bookings/"+fixture.id)
		detailBody := readAndClose(t, detail)
		if detail.StatusCode != http.StatusOK || strings.Contains(detailBody, "/bookings/"+fixture.id+"/cancel") || strings.Contains(detailBody, "Cancel booking") {
			t.Errorf("%s detail exposed a cancellation action: status=%d body=%s", test.name, detail.StatusCode, detailBody)
		}
		response := bookingHTTPPost(t, alexClient, app.server.URL+"/bookings/"+fixture.id+"/cancel",
			cancellationHTTPOrigin, url.Values{"_csrf": {csrf}})
		body := readAndClose(t, response)
		if response.StatusCode != http.StatusConflict || !strings.Contains(body, "can no longer be cancelled") ||
			!strings.Contains(body, `href="/bookings/`+fixture.id+`"`) || strings.Contains(body, "ErrCancellationStarted") {
			t.Errorf("%s HTTP outcome = %d body=%s; want safe 409 with detail link", test.name, response.StatusCode, body)
		}
		assertCancellationHTTPSnapshot(t, app.migrationDB, fixture.id, "confirmed", "", 1, 0)
	}

	app.setNow(app.fixedNow)
	failureFixture := fixtures[3]
	functionName := fmt.Sprintf("task3c_cancel_failure_%d", time.Now().UnixNano())
	triggerName := functionName + "_trigger"
	functionIdentifier := pgx.Identifier{"public", functionName}.Sanitize()
	triggerIdentifier := pgx.Identifier{triggerName}.Sanitize()
	if _, err := app.migrationDB.Exec(t.Context(), fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $body$
		BEGIN
			IF NEW.action = 'booking.cancelled' THEN
				RAISE EXCEPTION 'injected cancellation event failure SQLSTATE 08006 secret test marker';
			END IF;
			RETURN NEW;
		END;
		$body$`, functionIdentifier)); err != nil {
		t.Fatalf("create cancellation failure trigger function: %v", err)
	}
	if _, err := app.migrationDB.Exec(t.Context(), fmt.Sprintf(`
		CREATE TRIGGER %s BEFORE INSERT ON activity_events
		FOR EACH ROW EXECUTE FUNCTION %s()`, triggerIdentifier, functionIdentifier)); err != nil {
		_, _ = app.migrationDB.Exec(t.Context(), "DROP FUNCTION "+functionIdentifier+"()")
		t.Fatalf("create cancellation failure trigger: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := app.migrationDB.Exec(cleanupCtx, "DROP TRIGGER IF EXISTS "+triggerIdentifier+" ON activity_events"); err != nil {
			t.Errorf("drop cancellation failure trigger: %v", err)
		}
		if _, err := app.migrationDB.Exec(cleanupCtx, "DROP FUNCTION IF EXISTS "+functionIdentifier+"()"); err != nil {
			t.Errorf("drop cancellation failure function: %v", err)
		}
	})

	response := bookingHTTPPost(t, alexClient, app.server.URL+"/bookings/"+failureFixture.id+"/cancel",
		cancellationHTTPOrigin, url.Values{"_csrf": {csrf}})
	body := readAndClose(t, response)
	if response.StatusCode != http.StatusServiceUnavailable || !strings.Contains(body, "Cancellation could not be confirmed") ||
		!strings.Contains(body, `href="/bookings/`+failureFixture.id+`"`) || strings.Contains(body, "SQLSTATE") ||
		strings.Contains(body, "secret test marker") || strings.Contains(body, "password=") || strings.Contains(body, "rolled back") {
		t.Fatalf("operational cancellation failure leaked details or claimed a definite outcome: status=%d body=%s", response.StatusCode, body)
	}
	assertCancellationHTTPSnapshot(t, app.migrationDB, failureFixture.id, "confirmed", "", 1, 0)
	assertHTTPIntervalCounts(t, app.migrationDB, app.resourceID, failureFixture.start, failureFixture.end, 1, 1)
}
