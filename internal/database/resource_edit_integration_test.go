package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"labreserve.local/labreserve/internal/bookings"
	"labreserve.local/labreserve/internal/resources"
)

func TestResourceEditingPersistsDescriptiveValuesAndPreservesBookings(t *testing.T) {
	adminPool, appPool, fixtures := resourceCreationSetup(t)
	operationTime := time.Date(2040, time.January, 2, 3, 4, 5, 123456789, time.UTC)
	resourceService := resources.NewService(NewStore(appPool), func() time.Time { return operationTime })
	created, err := resourceService.Create(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.CreateRequest{
		Code: "Edit-" + strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", ""),
		Name: "Original Lab", Description: "Original description", Active: true,
	})
	if err != nil {
		t.Fatalf("create edit fixture: %v", err)
	}
	fixtures.trackResource(created.Resource.ID)

	bookingService := bookings.NewService(NewStore(appPool), func() time.Time { return operationTime })
	bookingRequest := bookings.CreateRequest{
		ResourceID: created.Resource.ID, RequestID: testDatabaseUUID(t, adminPool),
		StartAt: operationTime.Add(time.Hour), EndAt: operationTime.Add(2 * time.Hour),
		Purpose: "\u2003retained canonical request\u00a0",
	}
	bookingResult, err := bookingService.Create(t.Context(), bookings.Actor{AccountID: fixtures.coordinator}, bookingRequest)
	if err != nil {
		t.Fatalf("create retained Booking: %v", err)
	}
	bookingID := bookingResult.Booking.ID
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := adminPool.Exec(ctx, "DELETE FROM public.activity_events WHERE booking_id = $1::uuid", bookingID); err != nil {
			t.Errorf("delete retained Booking Activity fixture: %v", err)
		}
		if _, err := adminPool.Exec(ctx, "DELETE FROM public.bookings WHERE id = $1::uuid", bookingID); err != nil {
			t.Errorf("delete retained Booking fixture: %v", err)
		}
	})
	if bookingResult.Booking.Purpose != "retained canonical request" {
		t.Fatalf("Booking purpose was not canonical: %+v", bookingResult.Booking)
	}

	initial := created.Resource
	editCases := []struct {
		name        string
		description string
		at          time.Time
	}{
		{name: "Renamed Lab", description: initial.Description, at: operationTime.Add(time.Minute)},
		{name: "Renamed Lab", description: "Updated description", at: operationTime.Add(2 * time.Minute)},
		{name: "Renamed Lab", description: "", at: operationTime.Add(3 * time.Minute)},
		{name: "renamed Lab", description: "", at: operationTime.Add(4 * time.Minute)},
		{name: "renamed  Lab", description: "", at: operationTime.Add(5 * time.Minute)},
		{name: "renamed  Lab", description: "Cafe\u0301", at: operationTime.Add(6 * time.Minute)},
		{name: "renamed  Lab", description: "Café", at: operationTime.Add(7 * time.Minute)},
	}
	for _, test := range editCases {
		operationTime = test.at
		result, err := resourceService.Edit(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.EditRequest{
			ResourceID: initial.ID, Code: "\u2003" + initial.Code + "\u00a0",
			Name: "\u00a0" + test.name + "\u2003", Description: "\u2003" + test.description + "\u00a0",
		})
		if err != nil {
			t.Fatalf("edit Resource to name=%q description=%q: %v", test.name, test.description, err)
		}
		if !result.Changed || result.Resource.ID != initial.ID || result.Resource.Code != initial.Code ||
			result.Resource.Name != test.name || result.Resource.Description != test.description || !result.Resource.Active ||
			!result.Resource.CreatedAt.Equal(initial.CreatedAt) || !result.Resource.UpdatedAt.Equal(test.at.UTC().Truncate(time.Microsecond)) {
			t.Fatalf("descriptive edit result = %+v", result)
		}
	}

	persisted, found, err := NewStore(appPool).ResourceByID(t.Context(), initial.ID)
	if err != nil || !found || persisted.Code != initial.Code || persisted.Name != "renamed  Lab" ||
		persisted.Description != "Café" || !persisted.Active {
		t.Fatalf("updated Resource read by ID = %+v found=%v error=%v", persisted, found, err)
	}
	listed, err := NewStore(appPool).ListResources(t.Context())
	if err != nil || !containsResource(listed, initial.ID) {
		t.Fatalf("updated Resource missing from existing list read: found=%v error=%v", containsResource(listed, initial.ID), err)
	}

	operationTime = operationTime.Add(time.Hour)
	replayed, err := bookingService.Create(t.Context(), bookings.Actor{AccountID: fixtures.coordinator}, bookingRequest)
	if err != nil || !replayed.Replayed {
		t.Fatalf("replay retained Booking after Resource edit: result=%+v error=%v", replayed, err)
	}
	if replayed.Booking.ID != bookingResult.Booking.ID || replayed.Booking.ResourceID != bookingResult.Booking.ResourceID ||
		replayed.Booking.OwnerAccountID != bookingResult.Booking.OwnerAccountID || replayed.Booking.RequestID != bookingResult.Booking.RequestID ||
		!replayed.Booking.StartAt.Equal(bookingResult.Booking.StartAt) || !replayed.Booking.EndAt.Equal(bookingResult.Booking.EndAt) ||
		replayed.Booking.Purpose != bookingResult.Booking.Purpose || replayed.Booking.State != bookingResult.Booking.State ||
		!replayed.Booking.CreatedAt.Equal(bookingResult.Booking.CreatedAt) || replayed.Booking.CancelledByAccountID != nil ||
		replayed.Booking.CancelledAt != nil {
		t.Fatalf("Resource edit changed retained Booking request data: before=%+v after=%+v", bookingResult.Booking, replayed.Booking)
	}

	events := readResourceCapabilityEvents(t, adminPool, initial.ID)
	if len(events) != len(editCases)+1 || events[0].ActorID != fixtures.coordinator || events[0].Action != "resource.created" ||
		!events[0].OccurredAt.Equal(initial.CreatedAt) || events[0].Details["name"] != initial.Name ||
		events[0].Details["description"] != initial.Description {
		t.Fatalf("Resource creation/edit Activity events = %+v", events)
	}
	wantBefore := []map[string]string{
		{"name": "Original Lab", "description": "Original description"},
		{"name": "Renamed Lab", "description": "Original description"},
		{"name": "Renamed Lab", "description": "Updated description"},
		{"name": "Renamed Lab", "description": ""},
		{"name": "renamed Lab", "description": ""},
		{"name": "renamed  Lab", "description": ""},
		{"name": "renamed  Lab", "description": "Cafe\u0301"},
	}
	wantAfter := []map[string]string{
		{"name": "Renamed Lab", "description": "Original description"},
		{"name": "Renamed Lab", "description": "Updated description"},
		{"name": "Renamed Lab", "description": ""},
		{"name": "renamed Lab", "description": ""},
		{"name": "renamed  Lab", "description": ""},
		{"name": "renamed  Lab", "description": "Cafe\u0301"},
		{"name": "renamed  Lab", "description": "Café"},
	}
	for index := 1; index < len(events); index++ {
		event := events[index]
		before, beforeOK := event.Details["before"].(map[string]any)
		after, afterOK := event.Details["after"].(map[string]any)
		if !beforeOK || !afterOK || event.ActorID != fixtures.coordinator || event.Action != "resource.descriptive_edited" ||
			!event.BookingIsNil || !event.OccurredAt.Equal(editCases[index-1].at.UTC().Truncate(time.Microsecond)) ||
			before["name"] != wantBefore[index-1]["name"] || before["description"] != wantBefore[index-1]["description"] ||
			after["name"] != wantAfter[index-1]["name"] || after["description"] != wantAfter[index-1]["description"] {
			t.Fatalf("edit Activity %d does not match authoritative before/after values: %+v", index, event)
		}
	}
	assertBookingRequestCounts(t, adminPool, fixtures.coordinator, bookingRequest.RequestID, 1, 1)
}

func TestResourceEditingPersistsLongDescriptiveTextWithoutProductLengthCaps(t *testing.T) {
	adminPool, appPool, fixtures := resourceCreationSetup(t)
	now := time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC)
	service := resources.NewService(NewStore(appPool), func() time.Time { return now })
	created, err := service.Create(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.CreateRequest{
		Code: "Long-edit-" + strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", ""), Name: "Before edit", Active: true,
	})
	if err != nil {
		t.Fatalf("create long-edit fixture: %v", err)
	}
	fixtures.trackResource(created.Resource.ID)
	longName := strings.Repeat("界", 4096)
	longDescription := strings.Repeat("description\tinside\n", 4096) + "end"
	now = now.Add(time.Minute)
	edited, err := service.Edit(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.EditRequest{
		ResourceID: created.Resource.ID, Code: created.Resource.Code, Name: longName, Description: longDescription,
	})
	if err != nil || !edited.Changed || edited.Resource.Name != longName || edited.Resource.Description != longDescription {
		t.Fatalf("edit Resource with long valid descriptive text: result lengths=%d/%d error=%v",
			len(edited.Resource.Name), len(edited.Resource.Description), err)
	}
	persisted, found, err := NewStore(appPool).ResourceByID(t.Context(), created.Resource.ID)
	if err != nil || !found || persisted.Name != longName || persisted.Description != longDescription {
		t.Fatalf("long descriptive text was not persisted exactly: found=%v lengths=%d/%d error=%v",
			found, len(persisted.Name), len(persisted.Description), err)
	}
	events := readResourceCapabilityEvents(t, adminPool, created.Resource.ID)
	if len(events) != 2 || events[1].Action != "resource.descriptive_edited" ||
		events[1].Details["after"].(map[string]any)["name"] != longName ||
		events[1].Details["after"].(map[string]any)["description"] != longDescription {
		t.Fatalf("long edit Activity details do not match canonical values: event count=%d", len(events))
	}
}

func TestResourceEditingAllowsInactiveResourceChangesWithRetainedBooking(t *testing.T) {
	adminPool, appPool, fixtures := resourceCreationSetup(t)
	operationTime := time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC)
	service := resources.NewService(NewStore(appPool), func() time.Time { return operationTime })
	created, err := service.Create(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.CreateRequest{
		Code: "Inactive-" + strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", ""),
		Name: "Prepared Lab", Description: "Before", Active: true,
	})
	if err != nil {
		t.Fatalf("create inactive edit fixture: %v", err)
	}
	fixtures.trackResource(created.Resource.ID)

	request := bookings.CreateRequest{
		ResourceID: created.Resource.ID, RequestID: testDatabaseUUID(t, adminPool),
		StartAt: operationTime.Add(4 * time.Hour), EndAt: operationTime.Add(5 * time.Hour), Purpose: "retained on inactive Resource",
	}
	booking, err := bookings.NewService(NewStore(appPool), func() time.Time { return operationTime }).Create(
		t.Context(), bookings.Actor{AccountID: fixtures.coordinator}, request)
	if err != nil {
		t.Fatalf("create retained Booking before making fixture Resource inactive: %v", err)
	}
	cleanupResourceBookingFixture(t, adminPool, booking.Booking.ID)
	if _, err := adminPool.Exec(t.Context(), "UPDATE public.resources SET active = FALSE WHERE id = $1::uuid", created.Resource.ID); err != nil {
		t.Fatalf("prepare inactive Resource with retained Booking: %v", err)
	}
	bookingsBefore, bookingEventsBefore := resourceBookingAndEventSnapshot(t, adminPool, created.Resource.ID)

	operationTime = operationTime.Add(time.Minute)
	result, err := service.Edit(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.EditRequest{
		ResourceID: created.Resource.ID, Code: created.Resource.Code, Name: "Edited Prepared Lab", Description: "After",
	})
	if err != nil {
		t.Fatalf("edit inactive Resource with retained Booking: %v", err)
	}
	if !result.Changed || result.Resource.ID != created.Resource.ID || result.Resource.Code != created.Resource.Code ||
		result.Resource.Name != "Edited Prepared Lab" || result.Resource.Description != "After" || result.Resource.Active ||
		!result.Resource.CreatedAt.Equal(created.Resource.CreatedAt) || !result.Resource.UpdatedAt.Equal(operationTime) {
		t.Fatalf("inactive Resource edit result = %+v", result)
	}
	bookingsAfter, bookingEventsAfter := resourceBookingAndEventSnapshot(t, adminPool, created.Resource.ID)
	if bookingsBefore != bookingsAfter || bookingEventsBefore != bookingEventsAfter {
		t.Fatalf("inactive Resource edit changed retained Booking/history:\nbookings before=%s\nafter=%s\nevents before=%s\nafter=%s",
			bookingsBefore, bookingsAfter, bookingEventsBefore, bookingEventsAfter)
	}
	events := readResourceCapabilityEvents(t, adminPool, created.Resource.ID)
	if len(events) != 2 || events[1].ActorID != fixtures.coordinator || events[1].Action != "resource.descriptive_edited" ||
		!events[1].OccurredAt.Equal(result.Resource.UpdatedAt) || events[1].Details["before"].(map[string]any)["name"] != "Prepared Lab" ||
		events[1].Details["after"].(map[string]any)["name"] != "Edited Prepared Lab" {
		t.Fatalf("inactive Resource edit Activity = %+v", events)
	}
}

func TestResourceEditingPreservesAllRetainedBookingStatesAndHistory(t *testing.T) {
	adminPool, appPool, fixtures := resourceCreationSetup(t)
	base := bookingCreationNow()
	now := base
	actorID := fixtures.coordinator
	resourceService := resources.NewService(NewStore(appPool), func() time.Time { return now })
	created, err := resourceService.Create(t.Context(), resources.Actor{AccountID: actorID}, resources.CreateRequest{
		Code: "Retained-states-" + strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", ""),
		Name: "Retained State Lab", Active: true,
	})
	if err != nil {
		t.Fatalf("create retained-state Resource: %v", err)
	}
	fixtures.trackResource(created.Resource.ID)
	bookingService := bookings.NewService(NewStore(appPool), func() time.Time { return now })
	cancellationService := bookings.NewCancellationService(NewStore(appPool), func() time.Time { return now })
	offsets := []time.Duration{6 * time.Hour, 7 * time.Hour, 8 * time.Hour, 9 * time.Hour}
	requests := make([]bookings.CreateRequest, len(offsets))
	bookingIDs := make([]string, len(offsets))
	for index, offset := range offsets {
		requests[index] = bookings.CreateRequest{
			ResourceID: created.Resource.ID, RequestID: testDatabaseUUID(t, adminPool),
			StartAt: base.Add(offset), EndAt: base.Add(offset + 30*time.Minute),
			Purpose: fmt.Sprintf("retained state fixture %d", index),
		}
		result, err := bookingService.Create(t.Context(), bookings.Actor{AccountID: actorID}, requests[index])
		if err != nil {
			t.Fatalf("create retained Booking state fixture %d: %v", index, err)
		}
		bookingIDs[index] = result.Booking.ID
		cleanupResourceBookingFixture(t, adminPool, result.Booking.ID)
	}
	now = base.Add(time.Minute)
	cancelled, err := cancellationService.Cancel(t.Context(), bookings.Actor{AccountID: actorID}, bookingIDs[3])
	if err != nil || !cancelled.Changed {
		t.Fatalf("cancel retained-state fixture: result=%+v error=%v", cancelled, err)
	}

	now = base.Add(7*time.Hour + 15*time.Minute)
	var pastCount, inUseCount, upcomingCount, cancelledCount int
	if err := adminPool.QueryRow(t.Context(), `
		SELECT count(*) FILTER (WHERE state = 'confirmed' AND end_at <= $2),
		       count(*) FILTER (WHERE state = 'confirmed' AND start_at <= $2 AND end_at > $2),
		       count(*) FILTER (WHERE state = 'confirmed' AND start_at > $2),
		       count(*) FILTER (WHERE state = 'cancelled')
		FROM public.bookings WHERE resource_id = $1::uuid`, created.Resource.ID, now).Scan(
		&pastCount, &inUseCount, &upcomingCount, &cancelledCount); err != nil {
		t.Fatalf("classify retained Booking states before Resource edit: %v", err)
	}
	if pastCount != 1 || inUseCount != 1 || upcomingCount != 1 || cancelledCount != 1 {
		t.Fatalf("retained Booking state fixtures = past/in-use/upcoming/cancelled %d/%d/%d/%d, want one each",
			pastCount, inUseCount, upcomingCount, cancelledCount)
	}
	bookingsBefore, bookingEventsBefore := resourceBookingAndEventSnapshot(t, adminPool, created.Resource.ID)
	edited, err := resourceService.Edit(t.Context(), resources.Actor{AccountID: actorID}, resources.EditRequest{
		ResourceID: created.Resource.ID, Code: created.Resource.Code,
		Name: "Edited Retained State Lab", Description: "all booking states retained",
	})
	if err != nil || !edited.Changed || edited.Resource.ID != created.Resource.ID || edited.Resource.Code != created.Resource.Code ||
		!edited.Resource.Active || !edited.Resource.CreatedAt.Equal(created.Resource.CreatedAt) || !edited.Resource.UpdatedAt.Equal(now) {
		t.Fatalf("edit Resource with retained Booking states: result=%+v error=%v", edited, err)
	}
	bookingsAfter, bookingEventsAfter := resourceBookingAndEventSnapshot(t, adminPool, created.Resource.ID)
	if bookingsBefore != bookingsAfter || bookingEventsBefore != bookingEventsAfter {
		t.Fatalf("Resource edit changed retained Booking/history:\nbookings before=%s\nafter=%s\nevents before=%s\nafter=%s",
			bookingsBefore, bookingsAfter, bookingEventsBefore, bookingEventsAfter)
	}

	now = base.Add(10 * time.Hour)
	replayed, err := bookingService.Create(t.Context(), bookings.Actor{AccountID: actorID}, requests[3])
	if err != nil || !replayed.Replayed || replayed.Booking.ID != bookingIDs[3] || replayed.Booking.State != bookings.StateCancelled {
		t.Fatalf("cancelled Booking creation replay after Resource edit/start: result=%+v error=%v", replayed, err)
	}
	bookingsAfterReplay, bookingEventsAfterReplay := resourceBookingAndEventSnapshot(t, adminPool, created.Resource.ID)
	if bookingsBefore != bookingsAfterReplay || bookingEventsBefore != bookingEventsAfterReplay {
		t.Fatal("cancelled Booking creation replay after Resource edit changed retained Booking/history")
	}
}

func resourceBookingAndEventSnapshot(t *testing.T, pool *pgxpool.Pool, resourceID string) (string, string) {
	t.Helper()
	var bookingsJSON, eventsJSON string
	if err := pool.QueryRow(t.Context(), `
		SELECT COALESCE(jsonb_agg(to_jsonb(booking) ORDER BY booking.id), '[]'::jsonb)::text
		FROM public.bookings AS booking WHERE booking.resource_id = $1::uuid`, resourceID).Scan(&bookingsJSON); err != nil {
		t.Fatalf("snapshot retained Resource Bookings: %v", err)
	}
	if err := pool.QueryRow(t.Context(), `
		SELECT COALESCE(jsonb_agg(to_jsonb(event) ORDER BY event.id), '[]'::jsonb)::text
		FROM public.activity_events AS event
		WHERE event.booking_id IN (SELECT id FROM public.bookings WHERE resource_id = $1::uuid)`, resourceID).Scan(&eventsJSON); err != nil {
		t.Fatalf("snapshot retained Booking Activity: %v", err)
	}
	return bookingsJSON, eventsJSON
}

func TestResourceEditingOuterRollbackRestoresResourceAndActivity(t *testing.T) {
	adminPool, appPool, fixtures := resourceCreationSetup(t)
	createdAt := time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC)
	service := resources.NewService(NewStore(appPool), func() time.Time { return createdAt })
	created, err := service.Create(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.CreateRequest{
		Code: "Outer-Rollback-" + strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", ""),
		Name: "Original Lab", Description: "Original description", Active: false,
	})
	if err != nil {
		t.Fatalf("create outer-rollback edit fixture: %v", err)
	}
	fixtures.trackResource(created.Resource.ID)

	tx, err := appPool.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin outer edit transaction: %v", err)
	}
	defer tx.Rollback(context.Background())
	updatedAt := createdAt.Add(time.Minute)
	edited, err := callEditResourceCapability(t.Context(), tx, fixtures.coordinator, created.Resource.ID,
		"Uncommitted Lab", "Uncommitted description", updatedAt)
	if err != nil {
		t.Fatalf("invoke guarded edit in outer transaction: %v", err)
	}
	if !edited.Changed || edited.Name != "Uncommitted Lab" || edited.Description != "Uncommitted description" ||
		!edited.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("uncommitted edit result = %+v", edited)
	}
	var transactionEvents int
	if err := tx.QueryRow(t.Context(), `
		SELECT count(*) FROM public.activity_events WHERE resource_id = $1::uuid`, created.Resource.ID).Scan(&transactionEvents); err != nil {
		t.Fatalf("inspect edit event before outer rollback: %v", err)
	}
	if transactionEvents != 2 {
		t.Fatalf("outer transaction sees %d Resource events before rollback, want creation and edit", transactionEvents)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatalf("roll back outer edit transaction: %v", err)
	}

	var name, description string
	var storedUpdatedAt time.Time
	if err := adminPool.QueryRow(t.Context(), `
		SELECT name, description, updated_at FROM public.resources WHERE id = $1::uuid`, created.Resource.ID).Scan(
		&name, &description, &storedUpdatedAt); err != nil {
		t.Fatalf("read Resource after outer rollback: %v", err)
	}
	events := readResourceCapabilityEvents(t, adminPool, created.Resource.ID)
	if name != created.Resource.Name || description != created.Resource.Description ||
		!storedUpdatedAt.Equal(created.Resource.UpdatedAt) || len(events) != 1 || events[0].Action != "resource.created" {
		t.Fatalf("outer rollback retained edit state: name=%q description=%q updated=%s events=%+v",
			name, description, storedUpdatedAt, events)
	}
}

func TestResourceEditingRejectsUnauthorizedInvalidAndImmutableChanges(t *testing.T) {
	adminPool, appPool, fixtures := resourceCreationSetup(t)
	operationTime := time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC)
	resourceService := resources.NewService(NewStore(appPool), func() time.Time { return operationTime })
	create := func(code, name string) resources.Resource {
		t.Helper()
		result, err := resourceService.Create(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.CreateRequest{
			Code: code, Name: name, Description: "Original description", Active: false,
		})
		if err != nil {
			t.Fatalf("create edit rejection fixture: %v", err)
		}
		fixtures.trackResource(result.Resource.ID)
		return result.Resource
	}
	resource := create("Immutable-"+strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", ""), "Inactive Lab")
	other := create("Other-"+strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", ""), "Other Lab")
	edit := func(actorID, resourceID, code, name, description string) (resources.EditResult, error) {
		t.Helper()
		return resourceService.Edit(t.Context(), resources.Actor{AccountID: actorID}, resources.EditRequest{
			ResourceID: resourceID, Code: code, Name: name, Description: description,
		})
	}

	for index, engineerID := range []string{fixtures.engineer, fixtures.secondEngineer} {
		if _, err := edit(engineerID, resource.ID, resource.Code, resource.Name, resource.Description); !errors.Is(err, resources.ErrForbidden) {
			t.Fatalf("Engineer %d unchanged edit error = %v, want forbidden", index+1, err)
		}
		if _, err := edit(engineerID, resource.ID, strings.ToLower(resource.Code), "Unauthorized rename", "Unauthorized description"); !errors.Is(err, resources.ErrForbidden) {
			t.Fatalf("Engineer %d code-change attempt error = %v, want forbidden", index+1, err)
		}
	}
	if _, err := edit(fixtures.unknown, resource.ID, resource.Code, resource.Name, resource.Description); !errors.Is(err, resources.ErrUnauthenticated) {
		t.Fatalf("unknown actor unchanged edit error = %v, want unauthenticated", err)
	}
	if _, err := edit(fixtures.coordinator, resource.ID, resource.Code, "\u2003\u00a0", "Changed"); !errors.Is(err, resources.ErrInvalidInput) {
		t.Fatalf("empty canonical name error = %v, want invalid input", err)
	}
	for _, code := range []string{"A-free-code", other.Code, strings.ToLower(resource.Code)} {
		if _, err := edit(fixtures.coordinator, resource.ID, code, "Must not change", "Must not change"); !errors.Is(err, resources.ErrImmutableCode) {
			t.Fatalf("immutable code assertion %q returned %v", code, err)
		}
	}

	assertUnchanged := func(wantEvents int) {
		t.Helper()
		var code, name, description string
		var active bool
		var createdAt, updatedAt time.Time
		if err := adminPool.QueryRow(t.Context(), `
			SELECT code, name, description, active, created_at, updated_at
			FROM public.resources WHERE id = $1::uuid`, resource.ID).Scan(
			&code, &name, &description, &active, &createdAt, &updatedAt); err != nil {
			t.Fatalf("read Resource after rejected/no-op edit: %v", err)
		}
		if code != resource.Code || name != resource.Name || description != resource.Description || active ||
			!createdAt.Equal(resource.CreatedAt) || !updatedAt.Equal(resource.UpdatedAt) {
			t.Fatalf("rejected/no-op edit changed Resource: code=%q name=%q description=%q active=%v created=%s updated=%s",
				code, name, description, active, createdAt, updatedAt)
		}
		if count := len(readResourceCapabilityEvents(t, adminPool, resource.ID)); count != wantEvents {
			t.Fatalf("Resource has %d Activity events, want %d", count, wantEvents)
		}
	}
	assertUnchanged(1)

	removeFailure := installResourceActivityFailureTrigger(t, adminPool, fixtures.coordinator, "resource.descriptive_edited")
	operationTime = operationTime.Add(time.Minute)
	if _, err := edit(fixtures.coordinator, resource.ID, resource.Code, "Failed edit", "Failed description"); !errors.Is(err, resources.ErrOperational) {
		t.Fatalf("Activity insertion failure = %v, want operational failure", err)
	}
	removeFailure()
	assertUnchanged(1)

	installResourceUpdateFailureTrigger(t, adminPool, resource.ID)
	removeNoOpEventFailure := installResourceActivityFailureTrigger(t, adminPool, fixtures.coordinator, "resource.descriptive_edited")
	operationTime = operationTime.Add(time.Hour)
	result, err := edit(fixtures.coordinator, resource.ID, "\u2003"+resource.Code+"\u00a0", "\u00a0Inactive Lab\u2003", "\u2003Original description\u00a0")
	if err != nil || result.Changed || result.Resource.ID != resource.ID || result.Resource.Code != resource.Code ||
		result.Resource.Active || !result.Resource.UpdatedAt.Equal(resource.UpdatedAt) {
		t.Fatalf("canonical unchanged edit result=%+v error=%v", result, err)
	}
	removeNoOpEventFailure()
	assertUnchanged(1)

	unknownID := testDatabaseUUID(t, adminPool)
	if _, err := edit(fixtures.coordinator, unknownID, resource.Code, resource.Name, resource.Description); !errors.Is(err, resources.ErrUnknownResource) {
		t.Fatalf("unknown Resource edit error = %v, want unknown Resource", err)
	}
	for index, engineerID := range []string{fixtures.engineer, fixtures.secondEngineer} {
		if _, err := edit(engineerID, unknownID, resource.Code, resource.Name, resource.Description); !errors.Is(err, resources.ErrForbidden) {
			t.Fatalf("Engineer %d inaccessible Resource edit error = %v, want forbidden", index+1, err)
		}
	}
}
