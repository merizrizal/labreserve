package web

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"labreserve.local/labreserve/internal/config"
	"labreserve.local/labreserve/internal/database"
)

func TestMyBookingsUsesAuthenticatedOwnerAndRendersRetainedRecordsReadOnly(t *testing.T) {
	runtimePool := webTestPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	migrationPool := webTestPool(t, "LABRESERVE_TEST_DATABASE_URL")
	fixturePrefix := fmt.Sprintf("task3a-web-%d", time.Now().UnixNano())
	var accountIDs []string
	var resourceIDs []string
	insertAccount := func(login, displayName, role string) string {
		t.Helper()
		var id string
		err := migrationPool.QueryRow(t.Context(), `
			INSERT INTO accounts (id, login, display_name, password_hash, role)
			VALUES (gen_random_uuid(), $1, $2, 'unused-test-hash', $3) RETURNING id::text`,
			fixturePrefix+"-"+login, displayName, role).Scan(&id)
		if err != nil {
			t.Fatalf("insert Task 3A account fixture: %v", err)
		}
		accountIDs = append(accountIDs, id)
		return id
	}
	insertResource := func(index int, active bool) string {
		t.Helper()
		var id string
		err := migrationPool.QueryRow(t.Context(), `
			INSERT INTO resources (id, code, name, description, active)
			VALUES (gen_random_uuid(), $1, $2, '', $3) RETURNING id::text`,
			fmt.Sprintf("T3A-%d-%d", time.Now().UnixNano(), index), fmt.Sprintf("Task 3A Resource %d", index), active).Scan(&id)
		if err != nil {
			t.Fatalf("insert Task 3A resource fixture: %v", err)
		}
		resourceIDs = append(resourceIDs, id)
		return id
	}
	alexID := insertAccount("alex@example.test", "Task 3A Engineer A", "engineer")
	samID := insertAccount("sam@example.test", "Task 3A Engineer B", "engineer")
	jordanID := insertAccount("jordan@example.test", "Task 3A Coordinator", "coordinator")
	emptyID := insertAccount("empty@example.test", "Task 3A Empty Engineer", "engineer")
	pastResource := insertResource(0, true)
	inUseResource := insertResource(1, true)
	upcomingResource := insertResource(2, true)
	inactiveResource := insertResource(3, false)
	samResource := insertResource(4, true)
	jordanResource := insertResource(5, true)

	fixedNow := time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC)
	insertBooking := func(ownerID, resourceID string, startAt time.Time, state, purpose string) string {
		t.Helper()
		var cancelledBy, cancelledAt any
		if state == "cancelled" {
			cancelledBy, cancelledAt = ownerID, startAt
		}
		var id string
		err := migrationPool.QueryRow(t.Context(), `
			INSERT INTO bookings (
				id, resource_id, owner_account_id, request_id, start_at, end_at,
				purpose, state, created_at, cancelled_by_account_id, cancelled_at
			) VALUES (
				gen_random_uuid(), $1, $2, gen_random_uuid(), $3, $4, $5, $6, $3, $7, $8
			) RETURNING id::text`,
			resourceID, ownerID, startAt, startAt.Add(30*time.Minute), purpose, state, cancelledBy, cancelledAt).Scan(&id)
		if err != nil {
			t.Fatalf("insert Task 3A booking fixture: %v", err)
		}
		return id
	}
	insertBooking(alexID, pastResource, fixedNow.Add(-2*time.Hour), "confirmed", fixturePrefix+" past")
	insertBooking(alexID, inUseResource, fixedNow.Add(-5*time.Minute), "confirmed", fixturePrefix+" in use")
	upcomingPurpose := fixturePrefix + ` <script>window.task3aXss = true</script>`
	upcomingID := insertBooking(alexID, upcomingResource, fixedNow.Add(5*time.Minute), "confirmed", upcomingPurpose)
	insertBooking(alexID, inactiveResource, fixedNow.Add(-48*time.Hour), "cancelled", fixturePrefix+" cancelled inactive resource")
	insertBooking(samID, samResource, fixedNow.Add(5*time.Minute), "confirmed", fixturePrefix+" Sam only")
	insertBooking(jordanID, jordanResource, fixedNow.Add(5*time.Minute), "confirmed", fixturePrefix+" coordinator only")

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := migrationPool.Exec(ctx, `DELETE FROM activity_events WHERE booking_id IN (SELECT id FROM bookings WHERE purpose LIKE $1)`, fixturePrefix+"%"); err != nil {
			t.Errorf("remove Task 3A fixture events: %v", err)
		}
		if _, err := migrationPool.Exec(ctx, "DELETE FROM bookings WHERE purpose LIKE $1", fixturePrefix+"%"); err != nil {
			t.Errorf("remove Task 3A fixture bookings: %v", err)
		}
		for _, id := range resourceIDs {
			if _, err := migrationPool.Exec(ctx, "DELETE FROM resources WHERE id = $1", id); err != nil {
				t.Errorf("remove Task 3A fixture resource: %v", err)
			}
		}
		for _, id := range accountIDs {
			if _, err := migrationPool.Exec(ctx, "DELETE FROM accounts WHERE id = $1", id); err != nil {
				t.Errorf("remove Task 3A fixture account: %v", err)
			}
		}
	})

	clockReads := 0
	now := func() time.Time {
		clockReads++
		return fixedNow
	}
	application, err := New(database.NewStore(runtimePool), nil, config.Config{PublicOrigin: "http://labreserve.test"}, now)
	if err != nil {
		t.Fatal(err)
	}
	serveMyBookings := func(request *http.Request, identity *database.Identity) *httptest.ResponseRecorder {
		t.Helper()
		if identity != nil {
			request = request.WithContext(withState(request, requestState{
				hasSession: true,
				session:    database.Session{Identity: identity, CSRFToken: []byte("01234567890123456789012345678901")},
			}))
		}
		response := httptest.NewRecorder()
		application.myBookings(response, request)
		return response
	}
	requestFor := func(path string, identity *database.Identity) *httptest.ResponseRecorder {
		t.Helper()
		return serveMyBookings(httptest.NewRequest(http.MethodGet, path, nil), identity)
	}
	requestForRawQuery := func(rawQuery string, identity *database.Identity) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/my-bookings", nil)
		request.URL.RawQuery = rawQuery
		return serveMyBookings(request, identity)
	}
	identity := func(id, name string, role database.Role) *database.Identity {
		return &database.Identity{ID: id, DisplayName: name, Role: role}
	}

	unauthenticated := requestFor("/my-bookings", nil)
	if unauthenticated.Code != http.StatusSeeOther || unauthenticated.Header().Get("Location") != "/login" {
		t.Fatalf("unauthenticated My Bookings response = %d %q; want redirect to login", unauthenticated.Code, unauthenticated.Header().Get("Location"))
	}

	var beforeBookings, beforeEvents int64
	if err := migrationPool.QueryRow(t.Context(), "SELECT count(*) FROM bookings").Scan(&beforeBookings); err != nil {
		t.Fatal(err)
	}
	if err := migrationPool.QueryRow(t.Context(), "SELECT count(*) FROM activity_events").Scan(&beforeEvents); err != nil {
		t.Fatal(err)
	}
	readsBefore := clockReads
	alex := requestFor("/my-bookings?owner_id="+samID+"&account_id="+samID, identity(alexID, "Task 3A Engineer A", database.RoleEngineer))
	alexBody := alex.Body.String()
	if alex.Code != http.StatusOK || clockReads != readsBefore+1 {
		t.Fatalf("My Bookings response = %d with %d clock reads; want 200 and one clock read", alex.Code, clockReads-readsBefore)
	}
	if strings.Count(alexBody, `class="booking-row `) != 4 {
		t.Fatalf("Engineer A received %d booking rows, want exactly 4: %s", strings.Count(alexBody, `class="booking-row `), alexBody)
	}
	for _, expected := range []string{
		"Upcoming", "In use", "Past", "Cancelled",
		"Task 3A Resource 0", "Task 3A Resource 1", "Task 3A Resource 2", "Task 3A Resource 3",
		fixturePrefix + " past", fixturePrefix + " in use", fixturePrefix + " cancelled inactive resource",
		"Asia/Jakarta", "/bookings/" + upcomingID,
	} {
		if !strings.Contains(alexBody, expected) {
			t.Errorf("Engineer A's My Bookings page omitted %q", expected)
		}
	}
	if strings.Contains(alexBody, fixturePrefix+" Sam only") || !strings.Contains(alexBody, html.EscapeString(upcomingPurpose)) || strings.Contains(alexBody, upcomingPurpose) {
		t.Fatalf("owner filtering or escaped Booking purpose failed: %s", alexBody)
	}
	if strings.Contains(alexBody, "Cancel booking") || strings.Contains(alexBody, "Cancel this booking") {
		t.Fatal("read-only My Bookings view introduced cancellation controls")
	}
	if !strings.Contains(alexBody, `datetime="`+fixedNow.Add(5*time.Minute).UTC().Format(time.RFC3339Nano)+`"`) {
		t.Fatalf("My Bookings did not preserve the UTC instant in its time attribute: %s", alexBody)
	}

	readsBefore = clockReads
	sam := requestFor("/my-bookings", identity(samID, "Task 3A Engineer B", database.RoleEngineer))
	if sam.Code != http.StatusOK || clockReads != readsBefore+1 || !strings.Contains(sam.Body.String(), fixturePrefix+" Sam only") || strings.Contains(sam.Body.String(), fixturePrefix+" past") {
		t.Fatalf("Engineer B My Bookings was not restricted to their own record: status=%d body=%s", sam.Code, sam.Body.String())
	}
	readsBefore = clockReads
	coordinator := requestFor("/my-bookings", identity(jordanID, "Task 3A Coordinator", database.RoleCoordinator))
	if coordinator.Code != http.StatusOK || clockReads != readsBefore+1 || strings.Count(coordinator.Body.String(), `class="booking-row `) != 1 || !strings.Contains(coordinator.Body.String(), fixturePrefix+" coordinator only") || strings.Contains(coordinator.Body.String(), fixturePrefix+" Sam only") {
		t.Fatalf("Coordinator My Bookings did not remain self-scoped: status=%d body=%s", coordinator.Code, coordinator.Body.String())
	}
	readsBefore = clockReads
	empty := requestFor("/my-bookings", identity(emptyID, "Task 3A Empty Engineer", database.RoleEngineer))
	if empty.Code != http.StatusOK || clockReads != readsBefore+1 || !strings.Contains(empty.Body.String(), "You don't have any bookings yet.") || !strings.Contains(empty.Body.String(), `href="/resources"`) {
		t.Fatalf("empty-state response is not useful: status=%d body=%s", empty.Code, empty.Body.String())
	}
	var paginationBookingsBefore, paginationEventsBefore int64
	if err := migrationPool.QueryRow(t.Context(), "SELECT count(*) FROM bookings").Scan(&paginationBookingsBefore); err != nil {
		t.Fatal(err)
	}
	if err := migrationPool.QueryRow(t.Context(), "SELECT count(*) FROM activity_events").Scan(&paginationEventsBefore); err != nil {
		t.Fatal(err)
	}
	paginationCases := []struct {
		name       string
		rawQuery   string
		wantStatus int
	}{
		{name: "malformed percent encoding", rawQuery: "page=%ZZ", wantStatus: http.StatusBadRequest},
		{name: "malformed repeated page parameter", rawQuery: "page=1&page=%ZZ", wantStatus: http.StatusBadRequest},
		{name: "valid page", rawQuery: "page=1", wantStatus: http.StatusOK},
		{name: "repeated valid page parameters", rawQuery: "page=1&page=2", wantStatus: http.StatusBadRequest},
		{name: "zero page", rawQuery: "page=0", wantStatus: http.StatusBadRequest},
		{name: "negative page", rawQuery: "page=-1", wantStatus: http.StatusBadRequest},
		{name: "empty page", rawQuery: "page=", wantStatus: http.StatusBadRequest},
		{name: "non-numeric page", rawQuery: "page=abc", wantStatus: http.StatusBadRequest},
		{name: "parse-overflowing page", rawQuery: "page=999999999999999999999999", wantStatus: http.StatusBadRequest},
		{name: "offset-overflowing page", rawQuery: "page=9223372036854775807", wantStatus: http.StatusBadRequest},
	}
	for _, test := range paginationCases {
		response := requestForRawQuery(test.rawQuery, identity(alexID, "Task 3A Engineer A", database.RoleEngineer))
		if response.Code != test.wantStatus || (test.wantStatus == http.StatusBadRequest && strings.Contains(response.Body.String(), "SQLSTATE")) {
			t.Errorf("%s response = %d body=%s; want safe status %d", test.name, response.Code, response.Body.String(), test.wantStatus)
		}
	}
	var paginationBookingsAfter, paginationEventsAfter int64
	if err := migrationPool.QueryRow(t.Context(), "SELECT count(*) FROM bookings").Scan(&paginationBookingsAfter); err != nil {
		t.Fatal(err)
	}
	if err := migrationPool.QueryRow(t.Context(), "SELECT count(*) FROM activity_events").Scan(&paginationEventsAfter); err != nil {
		t.Fatal(err)
	}
	if paginationBookingsBefore != paginationBookingsAfter || paginationEventsBefore != paginationEventsAfter {
		t.Fatalf("pagination requests changed business records: bookings %d->%d, events %d->%d", paginationBookingsBefore, paginationBookingsAfter, paginationEventsBefore, paginationEventsAfter)
	}

	detailRequest := httptest.NewRequest(http.MethodGet, "/bookings/"+upcomingID, nil)
	detailRequest.SetPathValue("id", upcomingID)
	detailRequest = detailRequest.WithContext(withState(detailRequest, requestState{
		hasSession: true,
		session:    database.Session{Identity: identity(alexID, "Task 3A Engineer A", database.RoleEngineer)},
	}))
	detailResponse := httptest.NewRecorder()
	application.bookingDetailPage(detailResponse, detailRequest)
	if detailResponse.Code != http.StatusOK || !strings.Contains(detailResponse.Body.String(), "Booking details") || !strings.Contains(detailResponse.Body.String(), html.EscapeString(upcomingPurpose)) {
		t.Fatalf("existing Booking detail view failed: status=%d body=%s", detailResponse.Code, detailResponse.Body.String())
	}

	var afterBookings, afterEvents int64
	if err := migrationPool.QueryRow(t.Context(), "SELECT count(*) FROM bookings").Scan(&afterBookings); err != nil {
		t.Fatal(err)
	}
	if err := migrationPool.QueryRow(t.Context(), "SELECT count(*) FROM activity_events").Scan(&afterEvents); err != nil {
		t.Fatal(err)
	}
	if beforeBookings != afterBookings || beforeEvents != afterEvents {
		t.Fatalf("read-only My Bookings/detail changed database state: bookings %d->%d, events %d->%d", beforeBookings, afterBookings, beforeEvents, afterEvents)
	}
}
