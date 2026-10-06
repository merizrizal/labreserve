package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"labreserve.local/labreserve/internal/config"
	"labreserve.local/labreserve/internal/database"
)

func TestResourceScheduleIntersectionPaginationAndDerivedStates(t *testing.T) {
	runtimePool := webTestPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	migrationPool := webTestPool(t, "LABRESERVE_TEST_DATABASE_URL")
	var resourceID, ownerID string
	if err := migrationPool.QueryRow(t.Context(), `SELECT id::text FROM resources WHERE lower(code) = 'net-01'`).Scan(&resourceID); err != nil {
		t.Fatal(err)
	}
	if err := migrationPool.QueryRow(t.Context(), `SELECT id::text FROM accounts WHERE login = 'alex@example.test'`).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}

	jakarta, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	dayStart := time.Date(2040, time.February, 3, 0, 0, 0, 0, jakarta)
	dayEnd := dayStart.AddDate(0, 0, 1)
	now := dayStart.Add(7 * time.Hour)
	fixturePrefix := fmt.Sprintf("task-2c-schedule-%d", time.Now().UnixNano())
	defer func() {
		_, _ = migrationPool.Exec(t.Context(), "DELETE FROM bookings WHERE purpose LIKE $1", fixturePrefix+"-%")
	}()

	fixtureIDs := make([]string, 0, 29)
	insertFixture := func(suffix string, start, end time.Time, cancelled bool) string {
		t.Helper()
		var id string
		purpose := fixturePrefix + "-" + suffix
		if cancelled {
			err = migrationPool.QueryRow(t.Context(), `
				INSERT INTO bookings (
					id, resource_id, owner_account_id, request_id, start_at, end_at,
					purpose, state, created_at, cancelled_by_account_id, cancelled_at
				) VALUES (gen_random_uuid(), $1, $2, gen_random_uuid(), $3, $4, $5, 'cancelled', $6, $2, $6)
				RETURNING id::text`, resourceID, ownerID, start, end, purpose, now).Scan(&id)
		} else {
			err = migrationPool.QueryRow(t.Context(), `
				INSERT INTO bookings (
					id, resource_id, owner_account_id, request_id, start_at, end_at, purpose, state, created_at
				) VALUES (gen_random_uuid(), $1, $2, gen_random_uuid(), $3, $4, $5, 'confirmed', $6)
				RETURNING id::text`, resourceID, ownerID, start, end, purpose, now).Scan(&id)
		}
		if err != nil {
			t.Fatalf("insert %s schedule fixture: %v", suffix, err)
		}
		fixtureIDs = append(fixtureIDs, id)
		return id
	}

	for index := 0; index < 26; index++ {
		start := dayStart.Add(time.Duration(index*30+30) * time.Minute)
		insertFixture(fmt.Sprintf("%02d", index), start, start.Add(30*time.Minute), index == 1)
	}
	crossMidnightID := insertFixture("cross-midnight", dayStart.Add(-30*time.Minute), dayStart.Add(30*time.Minute), false)
	endsAtMidnightID := insertFixture("ends-at-midnight", dayStart.Add(-time.Hour), dayStart, true)
	insertFixture("ends-at-next-midnight", dayEnd.Add(-30*time.Minute), dayEnd, false)
	startsAtNextMidnightID := insertFixture("starts-at-next-midnight", dayEnd, dayEnd.Add(30*time.Minute), false)

	store := database.NewStore(runtimePool)
	application, err := New(store, nil, config.Config{PublicOrigin: "http://labreserve.test"}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	requestFor := func(date, page string) *httptest.ResponseRecorder {
		t.Helper()
		target := "/resources/" + resourceID + "?date=" + date
		if page != "" {
			target += "&page=" + page
		}
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.SetPathValue("id", resourceID)
		request = request.WithContext(withState(request, requestState{
			hasSession: true,
			session: database.Session{
				Identity:  &database.Identity{ID: ownerID, DisplayName: "Alex Morgan", Role: database.RoleEngineer},
				CSRFToken: []byte("01234567890123456789012345678901"),
			},
		}))
		response := httptest.NewRecorder()
		application.resourceDetail(response, request)
		return response
	}

	firstPage := requestFor("2040-02-03", "")
	firstBody := firstPage.Body.String()
	if firstPage.Code != http.StatusOK {
		t.Fatalf("first schedule page status = %d, body = %s", firstPage.Code, firstBody)
	}
	if strings.Count(firstBody, `class="booking-row `) != 25 || !strings.Contains(firstBody, "Next page") || !strings.Contains(firstBody, "Page 1") {
		t.Fatalf("first schedule page does not contain 25 ordered rows and next navigation: %s", firstBody)
	}
	for _, status := range []string{"Upcoming", "In use", "Past", "Cancelled"} {
		if !strings.Contains(firstBody, status) {
			t.Errorf("first schedule page does not show %s status", status)
		}
	}
	if !strings.Contains(firstBody, "Cancelled bookings do not occupy the resource.") {
		t.Fatal("cancelled schedule entry is not visually distinguished as non-occupying")
	}
	crossMidnightPosition := strings.Index(firstBody, "Booking "+crossMidnightID)
	firstRegularPosition := strings.Index(firstBody, "Booking "+fixtureIDs[0])
	if crossMidnightPosition < 0 || firstRegularPosition < 0 || crossMidnightPosition >= firstRegularPosition {
		t.Fatalf("cross-midnight booking was not listed before same-day bookings: %s", firstBody)
	}
	if strings.Contains(firstBody, "Booking "+endsAtMidnightID) {
		t.Fatal("booking ending exactly at the selected midnight appeared on the selected day")
	}
	if strings.Contains(firstBody, "Booking "+startsAtNextMidnightID) {
		t.Fatal("booking starting exactly at the following midnight appeared on the selected day")
	}

	secondPage := requestFor("2040-02-03", "2")
	secondBody := secondPage.Body.String()
	if secondPage.Code != http.StatusOK || strings.Count(secondBody, `class="booking-row `) != 3 || !strings.Contains(secondBody, "Previous page") || strings.Contains(secondBody, "Next page") {
		t.Fatalf("second schedule page is not a stable 25-item continuation: status=%d body=%s", secondPage.Code, secondBody)
	}
	if !strings.Contains(secondBody, "Booking "+fixtureIDs[25]) {
		t.Fatal("second page omitted the last same-day booking")
	}
	if strings.Contains(secondBody, "Booking "+crossMidnightID) {
		t.Fatal("second page repeated a record from the first page")
	}

	invalidPage := requestFor("2040-02-03", "0")
	if invalidPage.Code != http.StatusBadRequest {
		t.Fatalf("page=0 status = %d, want 400", invalidPage.Code)
	}
	invalidDate := requestFor("2040-02-30", "")
	if invalidDate.Code != http.StatusBadRequest {
		t.Fatalf("invalid date status = %d, want 400", invalidDate.Code)
	}

	nextDay, err := store.ListResourceBookingViews(t.Context(), resourceID, dayEnd, dayEnd.AddDate(0, 0, 1), 25, 0)
	if err != nil {
		t.Fatalf("query following Jakarta day: %v", err)
	}
	if len(nextDay) != 1 || nextDay[0].ID != startsAtNextMidnightID {
		t.Fatalf("following-day bookings = %+v, want only booking starting at midnight", nextDay)
	}
	if strings.Contains(firstBody, "Booking "+crossMidnightID) && !strings.Contains(firstBody, "Past") {
		t.Fatal("cross-midnight booking did not display its derived time status")
	}
}
