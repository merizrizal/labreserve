package database

import (
	"sort"
	"testing"
	"time"
)

func TestListMyBookingViewsFiltersBeforePaginationAndRetainsRecords(t *testing.T) {
	migrationPool := bookingTestPool(t)
	runtimePool := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	fixtures := newBookingTestFixtures(t, migrationPool, 3, 3)
	store := NewStore(runtimePool)

	if _, err := migrationPool.Exec(t.Context(), "UPDATE resources SET active = FALSE WHERE id = $1", fixtures.resources[2]); err != nil {
		t.Fatalf("deactivate fixture resource: %v", err)
	}

	start := time.Date(2040, time.February, 3, 10, 0, 0, 0, time.UTC)
	type expectedBooking struct {
		id      string
		startAt time.Time
	}
	var expected []expectedBooking
	insertFixture := func(ownerIndex, resourceIndex int, startAt time.Time, state string) string {
		t.Helper()
		ownerID := fixtures.accounts[ownerIndex]
		requestID := testDatabaseUUID(t, migrationPool)
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
				gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $4, $8, $9
			) RETURNING id::text`,
			fixtures.resources[resourceIndex], ownerID, requestID, startAt, startAt.Add(30*time.Minute),
			"Task 3A retained fixture", state, cancelledBy, cancelledAt).Scan(&id)
		if err != nil {
			t.Fatalf("insert %s booking fixture: %v", state, err)
		}
		if ownerIndex == 0 {
			expected = append(expected, expectedBooking{id: id, startAt: startAt})
		}
		return id
	}

	if got, err := store.ListMyBookingViews(t.Context(), fixtures.accounts[0], 26, 0); err != nil || len(got) != 0 {
		t.Fatalf("empty owner query = %d rows, error %v; want no rows", len(got), err)
	}
	inactiveCancelledID := insertFixture(0, 2, start.Add(-24*time.Hour), "cancelled")
	if got, err := store.ListMyBookingViews(t.Context(), fixtures.accounts[0], 26, 0); err != nil || len(got) != 1 {
		t.Fatalf("one-booking query = %d rows, error %v; want one row", len(got), err)
	}

	otherOwnerID := insertFixture(1, 0, start.Add(-48*time.Hour), "confirmed")
	for index := 0; index < 24; index++ {
		insertFixture(0, index%2, start.Add(time.Duration(index/2)*30*time.Minute), "confirmed")
	}
	if got, err := store.ListMyBookingViews(t.Context(), fixtures.accounts[0], 26, 0); err != nil || len(got) != 25 {
		t.Fatalf("exactly-25 query = %d rows, error %v; want 25 rows", len(got), err)
	}

	insertFixture(0, 0, start.Add(12*time.Hour), "confirmed")
	if got, err := store.ListMyBookingViews(t.Context(), fixtures.accounts[0], 26, 0); err != nil || len(got) != 26 {
		t.Fatalf("26-booking query = %d rows, error %v; want 26 rows", len(got), err)
	}
	for index := 25; index < 53; index++ {
		insertFixture(0, index%2, start.Add(13*time.Hour+time.Duration(index/2-12)*30*time.Minute), "confirmed")
	}
	if len(expected) != 54 {
		t.Fatalf("prepared %d owner bookings, want 54", len(expected))
	}

	sort.Slice(expected, func(i, j int) bool {
		if expected[i].startAt.Equal(expected[j].startAt) {
			return expected[i].id < expected[j].id
		}
		return expected[i].startAt.Before(expected[j].startAt)
	})
	var listed []BookingView
	for offset := int64(0); offset < int64(len(expected)); offset += 25 {
		page, err := store.ListMyBookingViews(t.Context(), fixtures.accounts[0], 25, offset)
		if err != nil {
			t.Fatalf("load owner page at offset %d: %v", offset, err)
		}
		want := 25
		if remaining := len(expected) - int(offset); remaining < want {
			want = remaining
		}
		if len(page) != want {
			t.Fatalf("owner page at offset %d has %d rows, want %d", offset, len(page), want)
		}
		listed = append(listed, page...)
	}
	for index, view := range listed {
		if view.ID != expected[index].id || view.OwnerAccountID != fixtures.accounts[0] {
			t.Fatalf("owner result %d = booking %s owned by %s; want %s owned by %s", index, view.ID, view.OwnerAccountID, expected[index].id, fixtures.accounts[0])
		}
	}
	for index := 1; index < len(listed); index++ {
		previous, current := listed[index-1], listed[index]
		if previous.StartAt.After(current.StartAt) || (previous.StartAt.Equal(current.StartAt) && previous.ID >= current.ID) {
			t.Fatalf("results are not ordered by start_at ASC, id ASC at positions %d and %d", index-1, index)
		}
	}

	var foundInactiveCancelled bool
	for _, view := range listed {
		if view.ID == inactiveCancelledID {
			foundInactiveCancelled = view.State == "cancelled" && view.ResourceID == fixtures.resources[2]
		}
		if view.ID == otherOwnerID {
			t.Fatalf("another owner's booking %s influenced the current owner's page", view.ID)
		}
	}
	if !foundInactiveCancelled {
		t.Fatal("cancelled booking for an inactive resource was not retained in My Bookings results")
	}
	if _, err := store.ListMyBookingViews(t.Context(), fixtures.accounts[0], 0, 0); err == nil {
		t.Fatal("zero page size was accepted")
	}
	if _, err := store.ListMyBookingViews(t.Context(), fixtures.accounts[0], 25, -1); err == nil {
		t.Fatal("negative page offset was accepted")
	}
}
