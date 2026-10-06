package web

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"labreserve.local/labreserve/internal/auth"
	"labreserve.local/labreserve/internal/bookings"
	"labreserve.local/labreserve/internal/config"
	"labreserve.local/labreserve/internal/database"
)

var bookingRequestIDPattern = regexp.MustCompile(`name="request_id" value="([^"]+)"`)

func TestBookingHTTPOutcomesPreserveTask2BMutationBoundary(t *testing.T) {
	runtimePool := webTestPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	migrationPool := webTestPool(t, "LABRESERVE_TEST_DATABASE_URL")
	resourceID := resourceIDForCode(t, migrationPool, "net-01")
	otherResourceID := resourceIDForCode(t, migrationPool, "k8s-01")
	demoResourceID := resourceIDForCode(t, migrationPool, "demo-01")
	alexID := accountIDForLogin(t, migrationPool, "alex@example.test")
	samID := accountIDForLogin(t, migrationPool, "sam@example.test")
	jordanID := accountIDForLogin(t, migrationPool, "jordan@example.test")

	fixedNow := time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC)
	now := func() time.Time { return fixedNow }
	store := database.NewStore(runtimePool)
	authentication := auth.NewService(store, now)
	application, err := New(store, authentication, config.Config{
		DatabaseURL: "postgres://test", ListenAddr: "127.0.0.1:8080", PublicOrigin: "http://labreserve.test",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(application.Handler())
	defer server.Close()

	fixturePrefix := fmt.Sprintf("task2c-http-%d-", time.Now().UnixNano())
	inactiveCode := fmt.Sprintf("T2C-INACTIVE-%d", time.Now().UnixNano())
	var inactiveResourceID string
	if err := migrationPool.QueryRow(t.Context(), `
		INSERT INTO resources (id, code, name, description, active)
		VALUES (gen_random_uuid(), $1, 'Inactive Task 2C Fixture', '', FALSE)
		RETURNING id::text`, inactiveCode).Scan(&inactiveResourceID); err != nil {
		t.Fatalf("insert inactive Resource fixture: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = migrationPool.Exec(cleanupCtx, `
			DELETE FROM activity_events e USING bookings b
			WHERE e.booking_id = b.id AND b.purpose LIKE $1`, fixturePrefix+"%")
		_, _ = migrationPool.Exec(cleanupCtx, "DELETE FROM bookings WHERE purpose LIKE $1", fixturePrefix+"%")
		_, _ = migrationPool.Exec(cleanupCtx, "DELETE FROM resources WHERE id = $1", inactiveResourceID)
	})

	anonymousJar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	anonymousClient := &http.Client{Jar: anonymousJar, CheckRedirect: noRedirect}
	protected := mustGet(t, anonymousClient, server.URL+"/resources/"+resourceID+"/bookings/new")
	protected.Body.Close()
	if protected.StatusCode != http.StatusSeeOther || protected.Header.Get("Location") != "/login" {
		t.Fatalf("unauthenticated booking form: status=%d location=%q", protected.StatusCode, protected.Header.Get("Location"))
	}

	alexClient := loginBookingHTTPUser(t, server.URL, "alex@example.test", requiredTestPassword(t, "SEED_ALEX_PASSWORD"))
	formURL := server.URL + "/resources/" + resourceID + "/bookings/new"
	formStatus, formBody, csrf, requestID := getBookingHTTPForm(t, alexClient, formURL)
	if formStatus != http.StatusOK || !strings.Contains(formBody, "Start time (Asia/Jakarta)") {
		t.Fatalf("authenticated Engineer booking form: status=%d body=%s", formStatus, formBody)
	}

	jakarta, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2040, time.January, 4, 12, 0, 0, 0, jakarta)
	end := start.Add(time.Hour)
	startValue := start.Format(jakartaDateTimeLayout)
	endValue := end.Format(jakartaDateTimeLayout)
	purpose := fixturePrefix + `<img src=x onerror="alert(1)">`
	action := server.URL + "/resources/" + resourceID + "/bookings"
	values := bookingHTTPValues(csrf, requestID, startValue, endValue, purpose)

	missingCSRF := bookingHTTPPost(t, alexClient, action, "http://labreserve.test", withoutFormField(values, "_csrf"))
	if missingCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("missing booking CSRF status = %d", missingCSRF.StatusCode)
	}
	missingCSRF.Body.Close()
	assertBookingRequestCounts(t, migrationPool, alexID, requestID, 0, 0)

	invalidCSRFValues := cloneFormValues(values)
	invalidCSRFValues.Set("_csrf", "invalid-csrf-token")
	invalidCSRF := bookingHTTPPost(t, alexClient, action, "http://labreserve.test", invalidCSRFValues)
	if invalidCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("invalid booking CSRF status = %d", invalidCSRF.StatusCode)
	}
	invalidCSRF.Body.Close()
	assertBookingRequestCounts(t, migrationPool, alexID, requestID, 0, 0)

	wrongOrigin := bookingHTTPPost(t, alexClient, action, "https://attacker.example", values)
	if wrongOrigin.StatusCode != http.StatusForbidden {
		t.Fatalf("unexpected booking Origin status = %d", wrongOrigin.StatusCode)
	}
	wrongOrigin.Body.Close()
	assertBookingRequestCounts(t, migrationPool, alexID, requestID, 0, 0)

	malformed := cloneFormValues(values)
	malformed.Set("start_jakarta", "2040-01-04 12:00")
	malformedResponse := bookingHTTPPost(t, alexClient, action, "http://labreserve.test", malformed)
	malformedBody := readAndClose(t, malformedResponse)
	if malformedResponse.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(malformedBody, "Enter valid start and end date/time values in the Asia/Jakarta format.") {
		t.Fatalf("malformed Jakarta timestamp: status=%d body=%s", malformedResponse.StatusCode, malformedBody)
	}
	assertBookingRequestCounts(t, migrationPool, alexID, requestID, 0, 0)

	invalidBusiness := cloneFormValues(values)
	invalidBusiness.Set("end_jakarta", start.Add(15*time.Minute).Format(jakartaDateTimeLayout))
	invalidBusinessResponse := bookingHTTPPost(t, alexClient, action, "http://labreserve.test", invalidBusiness)
	invalidBusinessBody := readAndClose(t, invalidBusinessResponse)
	if invalidBusinessResponse.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(invalidBusinessBody, "does not meet the time, duration, or purpose rules") {
		t.Fatalf("Task 2B validation mapping: status=%d body=%s", invalidBusinessResponse.StatusCode, invalidBusinessBody)
	}
	assertBookingRequestCounts(t, migrationPool, alexID, requestID, 0, 0)

	badIdentifier := cloneFormValues(values)
	badIdentifier.Set("request_id", "not-a-uuid")
	badIdentifierResponse := bookingHTTPPost(t, alexClient, action, "http://labreserve.test", badIdentifier)
	badIdentifierBody := readAndClose(t, badIdentifierResponse)
	if badIdentifierResponse.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(badIdentifierBody, "request identifier is invalid") {
		t.Fatalf("malformed request identifier: status=%d body=%s", badIdentifierResponse.StatusCode, badIdentifierBody)
	}
	assertNoTask2CPurpose(t, migrationPool, fixturePrefix)

	unknownRequestID := newBookingHTTPRequestID(t)
	unknownAction := server.URL + "/resources/44444444-4444-4444-8444-444444444444/bookings"
	unknownValues := bookingHTTPValues(csrf, unknownRequestID, startValue, endValue, fixturePrefix+"unknown Resource")
	unknown := bookingHTTPPost(t, alexClient, unknownAction, "http://labreserve.test", unknownValues)
	unknown.Body.Close()
	if unknown.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown Resource status = %d", unknown.StatusCode)
	}
	assertBookingRequestCounts(t, migrationPool, alexID, unknownRequestID, 0, 0)

	inactiveRequestID := newBookingHTTPRequestID(t)
	inactiveAction := server.URL + "/resources/" + inactiveResourceID + "/bookings"
	inactiveValues := bookingHTTPValues(csrf, inactiveRequestID, startValue, endValue, fixturePrefix+"inactive Resource")
	inactive := bookingHTTPPost(t, alexClient, inactiveAction, "http://labreserve.test", inactiveValues)
	inactiveBody := readAndClose(t, inactive)
	if inactive.StatusCode != http.StatusConflict || !strings.Contains(inactiveBody, "resource is inactive") {
		t.Fatalf("inactive Resource mapping: status=%d body=%s", inactive.StatusCode, inactiveBody)
	}
	assertBookingRequestCounts(t, migrationPool, alexID, inactiveRequestID, 0, 0)

	forgedValues := cloneFormValues(values)
	forgedValues.Set("owner_id", samID)
	forgedValues.Set("role", "coordinator")
	created := bookingHTTPPost(t, alexClient, action, "http://labreserve.test", forgedValues)
	location := created.Header.Get("Location")
	created.Body.Close()
	if created.StatusCode != http.StatusSeeOther || !strings.HasSuffix(location, "?result=created") {
		t.Fatalf("successful booking did not use PRG: status=%d location=%q", created.StatusCode, location)
	}
	bookingID := strings.TrimSuffix(strings.TrimPrefix(strings.Split(location, "?")[0], "/bookings/"), "/")
	if !uuidPattern.MatchString(bookingID) {
		t.Fatalf("successful PRG location has no Booking identifier: %q", location)
	}
	assertBookingRequestCounts(t, migrationPool, alexID, requestID, 1, 1)
	storedID := assertStoredHTTPBooking(t, migrationPool, alexID, resourceID, requestID, start, end, purpose)
	if bookingID != storedID {
		t.Fatalf("PRG Booking ID %q differs from persisted ID %q", bookingID, storedID)
	}

	detailResponse := mustGet(t, alexClient, server.URL+location)
	detailBody := readAndClose(t, detailResponse)
	if detailResponse.StatusCode != http.StatusOK || !strings.Contains(detailBody, "Booking created successfully") || !strings.Contains(detailBody, "Alex Morgan") {
		t.Fatalf("created Booking detail: status=%d body=%s", detailResponse.StatusCode, detailBody)
	}
	if strings.Contains(detailBody, `<img src=x`) || !strings.Contains(detailBody, html.EscapeString(purpose)) || !strings.Contains(detailBody, `datetime="`+start.UTC().Format(time.RFC3339Nano)+`"`) {
		t.Fatalf("Booking purpose or persisted instant was not safely represented: %s", detailBody)
	}

	replay := bookingHTTPPost(t, alexClient, action, "http://labreserve.test", values)
	replayLocation := replay.Header.Get("Location")
	replay.Body.Close()
	if replay.StatusCode != http.StatusSeeOther || replayLocation != "/bookings/"+bookingID+"?result=replayed" {
		t.Fatalf("identical submission did not return the original Booking: status=%d location=%q", replay.StatusCode, replayLocation)
	}
	assertBookingRequestCounts(t, migrationPool, alexID, requestID, 1, 1)
	replayDetail := mustGet(t, alexClient, server.URL+replayLocation)
	replayBody := readAndClose(t, replayDetail)
	if replayDetail.StatusCode != http.StatusOK || !strings.Contains(replayBody, "not a second creation") {
		t.Fatalf("replay result was presented as a new creation: status=%d body=%s", replayDetail.StatusCode, replayBody)
	}

	changed := cloneFormValues(values)
	changed.Set("purpose", fixturePrefix+"changed request data")
	changedResponse := bookingHTTPPost(t, alexClient, action, "http://labreserve.test", changed)
	changedBody := readAndClose(t, changedResponse)
	if changedResponse.StatusCode != http.StatusConflict || !strings.Contains(changedBody, "request identifier already belongs to different booking data") {
		t.Fatalf("changed request-id reuse mapping: status=%d body=%s", changedResponse.StatusCode, changedBody)
	}
	assertBookingRequestCounts(t, migrationPool, alexID, requestID, 1, 1)

	samClient := loginBookingHTTPUser(t, server.URL, "sam@example.test", requiredTestPassword(t, "SEED_SAM_PASSWORD"))
	_, _, samCSRF, samRequestID := getBookingHTTPForm(t, samClient, server.URL+"/resources/"+resourceID+"/bookings/new")
	samAction := server.URL + "/resources/" + resourceID + "/bookings"
	samValues := bookingHTTPValues(samCSRF, samRequestID, startValue, endValue, fixturePrefix+"Engineer B overlap")
	conflict := bookingHTTPPost(t, samClient, samAction, "http://labreserve.test", samValues)
	conflictBody := readAndClose(t, conflict)
	if conflict.StatusCode != http.StatusConflict || !strings.Contains(conflictBody, "selected interval is no longer available") || !strings.Contains(conflictBody, `name="request_id" value="`+samRequestID+`"`) {
		t.Fatalf("overlapping Engineer B request: status=%d body=%s", conflict.StatusCode, conflictBody)
	}
	assertBookingRequestCounts(t, migrationPool, samID, samRequestID, 0, 0)
	assertHTTPIntervalCounts(t, migrationPool, resourceID, start, end, 1, 1)

	adjacentValues := cloneFormValues(samValues)
	adjacentValues.Set("start_jakarta", endValue)
	adjacentValues.Set("end_jakarta", end.Add(time.Hour).Format(jakartaDateTimeLayout))
	adjacentValues.Set("purpose", fixturePrefix+"adjacent corrected interval")
	adjacent := bookingHTTPPost(t, samClient, samAction, "http://labreserve.test", adjacentValues)
	adjacentLocation := adjacent.Header.Get("Location")
	adjacent.Body.Close()
	if adjacent.StatusCode != http.StatusSeeOther || !strings.HasSuffix(adjacentLocation, "?result=created") {
		t.Fatalf("adjacent corrected booking: status=%d location=%q", adjacent.StatusCode, adjacentLocation)
	}
	assertBookingRequestCounts(t, migrationPool, samID, samRequestID, 1, 1)
	assertHTTPIntervalCounts(t, migrationPool, resourceID, start, end, 1, 1)

	_, _, otherResourceCSRF, otherResourceRequestID := getBookingHTTPForm(t, alexClient, server.URL+"/resources/"+otherResourceID+"/bookings/new")
	otherValues := bookingHTTPValues(otherResourceCSRF, otherResourceRequestID, startValue, endValue, fixturePrefix+"same interval different Resource")
	otherAction := server.URL + "/resources/" + otherResourceID + "/bookings"
	otherResource := bookingHTTPPost(t, alexClient, otherAction, "http://labreserve.test", otherValues)
	otherResource.Body.Close()
	if otherResource.StatusCode != http.StatusSeeOther {
		t.Fatalf("identical interval on a different Resource status = %d", otherResource.StatusCode)
	}
	assertBookingRequestCounts(t, migrationPool, alexID, otherResourceRequestID, 1, 1)
	assertHTTPIntervalCounts(t, migrationPool, otherResourceID, start, end, 1, 1)

	jordanClient := loginBookingHTTPUser(t, server.URL, "jordan@example.test", requiredTestPassword(t, "SEED_JORDAN_PASSWORD"))
	_, _, jordanCSRF, jordanRequestID := getBookingHTTPForm(t, jordanClient, server.URL+"/resources/"+demoResourceID+"/bookings/new")
	jordanStart := end.Add(2 * time.Hour)
	jordanValues := bookingHTTPValues(jordanCSRF, jordanRequestID, jordanStart.Format(jakartaDateTimeLayout), jordanStart.Add(time.Hour).Format(jakartaDateTimeLayout), fixturePrefix+"coordinator self-owned")
	jordanValues.Set("owner_id", alexID)
	jordanValues.Set("role", "engineer")
	jordanAction := server.URL + "/resources/" + demoResourceID + "/bookings"
	coordinatorBooking := bookingHTTPPost(t, jordanClient, jordanAction, "http://labreserve.test", jordanValues)
	coordinatorBooking.Body.Close()
	if coordinatorBooking.StatusCode != http.StatusSeeOther {
		t.Fatalf("Coordinator self-booking status = %d", coordinatorBooking.StatusCode)
	}
	assertBookingRequestCounts(t, migrationPool, jordanID, jordanRequestID, 1, 1)
	assertStoredHTTPBooking(t, migrationPool, jordanID, demoResourceID, jordanRequestID, jordanStart, jordanStart.Add(time.Hour), fixturePrefix+"coordinator self-owned")

	operationalRequestID := newBookingHTTPRequestID(t)
	operationalValues := bookingHTTPValues(csrf, operationalRequestID, startValue, endValue, fixturePrefix+"operational diagnostic must not leak")
	application.bookingService = bookings.NewService(operationalBookingFailure{cause: errors.New("SQLSTATE 08006: password=do-not-leak")}, now)
	operational := bookingHTTPPost(t, alexClient, action, "http://labreserve.test", operationalValues)
	operationalBody := readAndClose(t, operational)
	if operational.StatusCode != http.StatusServiceUnavailable || strings.Contains(operationalBody, "SQLSTATE") || strings.Contains(operationalBody, "do-not-leak") || strings.Contains(operationalBody, "password=") {
		t.Fatalf("operational failure was not safely mapped: status=%d body=%s", operational.StatusCode, operationalBody)
	}
	assertBookingRequestCounts(t, migrationPool, alexID, operationalRequestID, 0, 0)
}

func noRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

func loginBookingHTTPUser(t *testing.T, baseURL, login, password string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, CheckRedirect: noRedirect}
	loginPage := mustGet(t, client, baseURL+"/login")
	csrf := csrfFrom(t, readAndClose(t, loginPage))
	response := submitForm(t, client, baseURL+"/login", "http://labreserve.test", url.Values{
		"_csrf": {csrf}, "login": {login}, "password": {password},
	})
	response.Body.Close()
	if response.StatusCode != http.StatusSeeOther || response.Header.Get("Location") != "/resources" {
		t.Fatalf("sign in %s: status=%d location=%q", login, response.StatusCode, response.Header.Get("Location"))
	}
	return client
}

func getBookingHTTPForm(t *testing.T, client *http.Client, target string) (int, string, string, string) {
	t.Helper()
	response := mustGet(t, client, target)
	body := readAndClose(t, response)
	csrf := csrfFrom(t, body)
	match := bookingRequestIDPattern.FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("booking form has no stable request identifier: %s", body)
	}
	return response.StatusCode, body, csrf, match[1]
}

func bookingHTTPValues(csrf, requestID, start, end, purpose string) url.Values {
	return url.Values{
		"_csrf": {csrf}, "request_id": {requestID},
		"start_jakarta": {start}, "end_jakarta": {end}, "purpose": {purpose},
	}
}

func bookingHTTPPost(t *testing.T, client *http.Client, target, origin string, values url.Values) *http.Response {
	t.Helper()
	return submitForm(t, client, target, origin, values)
}

func cloneFormValues(values url.Values) url.Values {
	copy := make(url.Values, len(values))
	for key, entries := range values {
		copy[key] = append([]string(nil), entries...)
	}
	return copy
}

func withoutFormField(values url.Values, field string) url.Values {
	copy := cloneFormValues(values)
	delete(copy, field)
	return copy
}

func newBookingHTTPRequestID(t *testing.T) string {
	t.Helper()
	requestID, err := newBookingRequestID()
	if err != nil {
		t.Fatal(err)
	}
	return requestID
}

func resourceIDForCode(t *testing.T, pool *pgxpool.Pool, code string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(t.Context(), "SELECT id::text FROM resources WHERE lower(code) = $1", code).Scan(&id); err != nil {
		t.Fatalf("find Resource %s: %v", code, err)
	}
	return id
}

func accountIDForLogin(t *testing.T, pool *pgxpool.Pool, login string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(t.Context(), "SELECT id::text FROM accounts WHERE login = $1", login).Scan(&id); err != nil {
		t.Fatalf("find account %s: %v", login, err)
	}
	return id
}

func assertBookingRequestCounts(t *testing.T, pool *pgxpool.Pool, ownerID, requestID string, wantBookings, wantEvents int) {
	t.Helper()
	var bookingsCount, eventsCount int
	err := pool.QueryRow(t.Context(), `
		SELECT count(*)::int,
		       (SELECT count(*)::int FROM activity_events e JOIN bookings b ON b.id = e.booking_id
		        WHERE b.owner_account_id = $1 AND b.request_id = $2 AND e.action = 'booking.created')
		FROM bookings WHERE owner_account_id = $1 AND request_id = $2`, ownerID, requestID).Scan(&bookingsCount, &eventsCount)
	if err != nil {
		t.Fatal(err)
	}
	if bookingsCount != wantBookings || eventsCount != wantEvents {
		t.Fatalf("owner/request %s/%s has bookings/events %d/%d, want %d/%d", ownerID, requestID, bookingsCount, eventsCount, wantBookings, wantEvents)
	}
}

func assertNoTask2CPurpose(t *testing.T, pool *pgxpool.Pool, prefix string) {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(), "SELECT count(*)::int FROM bookings WHERE purpose LIKE $1", prefix+"%").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rejected Task 2C submissions retained %d bookings", count)
	}
}

func assertStoredHTTPBooking(t *testing.T, pool *pgxpool.Pool, ownerID, resourceID, requestID string, start, end time.Time, purpose string) string {
	t.Helper()
	var id, storedOwner, storedResource, storedPurpose string
	var storedStart, storedEnd time.Time
	err := pool.QueryRow(t.Context(), `
		SELECT id::text, owner_account_id::text, resource_id::text, start_at, end_at, purpose
		FROM bookings WHERE owner_account_id = $1 AND request_id = $2`, ownerID, requestID).Scan(
		&id, &storedOwner, &storedResource, &storedStart, &storedEnd, &storedPurpose)
	if err != nil {
		t.Fatalf("load persisted booking: %v", err)
	}
	if storedOwner != ownerID || storedResource != resourceID || !storedStart.Equal(start) || !storedEnd.Equal(end) || storedPurpose != strings.TrimSpace(purpose) {
		t.Fatalf("persisted booking differs from authenticated Jakarta request: owner=%s resource=%s interval=%v–%v purpose=%q", storedOwner, storedResource, storedStart, storedEnd, storedPurpose)
	}
	var eventCount int
	if err := pool.QueryRow(t.Context(), `
		SELECT count(*)::int FROM activity_events
		WHERE booking_id = $1 AND actor_account_id = $2 AND action = 'booking.created'`, id, ownerID).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 {
		t.Fatalf("persisted Booking %s has %d matching creation events, want exactly one", id, eventCount)
	}
	return id
}

func assertHTTPIntervalCounts(t *testing.T, pool *pgxpool.Pool, resourceID string, start, end time.Time, wantBookings, wantEvents int) {
	t.Helper()
	var bookingCount, eventCount int
	if err := pool.QueryRow(t.Context(), `
		SELECT count(*)::int,
		       (SELECT count(*)::int FROM activity_events e JOIN bookings eb ON eb.id = e.booking_id
		        WHERE eb.resource_id = $1 AND eb.start_at = $2 AND eb.end_at = $3 AND e.action = 'booking.created')
		FROM bookings b WHERE b.resource_id = $1 AND b.start_at = $2 AND b.end_at = $3 AND b.state = 'confirmed'`,
		resourceID, start, end).Scan(&bookingCount, &eventCount); err != nil {
		t.Fatal(err)
	}
	if bookingCount != wantBookings || eventCount != wantEvents {
		t.Fatalf("Resource/interval %s/%v–%v has bookings/events %d/%d, want %d/%d", resourceID, start, end, bookingCount, eventCount, wantBookings, wantEvents)
	}
}

type operationalBookingFailure struct{ cause error }

func (r operationalBookingFailure) BeginBookingCreation(context.Context) (bookings.Transaction, error) {
	return nil, r.cause
}
