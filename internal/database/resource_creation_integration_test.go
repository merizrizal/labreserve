package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"labreserve.local/labreserve/internal/bookings"
	"labreserve.local/labreserve/internal/resources"
)

func TestResourceCreationPersistsCanonicalValuesAndBothInitialStates(t *testing.T) {
	adminPool, appPool, fixtures := resourceCreationSetup(t)
	now := time.Date(2040, time.January, 2, 3, 4, 5, 123456789, time.UTC)
	actor := resources.Actor{AccountID: fixtures.coordinator}
	service := resources.NewService(NewStore(appPool), func() time.Time { return now })
	store := NewStore(appPool)

	for _, test := range []struct {
		name        string
		active      bool
		description string
		wantDesc    string
	}{
		{name: "active with canonical Unicode text", active: true,
			description: "\u2003Cafe\u0301  inside\tvalue\nend\u00a0", wantDesc: "Cafe\u0301  inside\tvalue\nend"},
		{name: "inactive with empty description", active: false,
			description: " \u2003\u00a0", wantDesc: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			suffix := strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", "")
			code := "MiX/Resource-" + suffix
			result, err := service.Create(t.Context(), actor, resources.CreateRequest{
				Code: "\u2003" + code + "\t\u00a0", Name: "\u00a0Test\t Resource  West\n\u2003",
				Description: test.description, Active: test.active,
			})
			if err != nil {
				t.Fatalf("create Resource: %v", err)
			}
			fixtures.trackResource(result.Resource.ID)
			wantCode := code
			if result.Resource.Code != wantCode || result.Resource.Name != "Test\t Resource  West" ||
				result.Resource.Description != test.wantDesc || result.Resource.Active != test.active ||
				!result.Resource.CreatedAt.Equal(now.Truncate(time.Microsecond)) ||
				!result.Resource.UpdatedAt.Equal(result.Resource.CreatedAt) {
				t.Fatalf("created Resource = %+v", result.Resource)
			}
			assertResourceCreationActivity(t, adminPool, actor.AccountID, result.Resource)

			byID, found, err := store.ResourceByID(t.Context(), result.Resource.ID)
			if err != nil || !found {
				t.Fatalf("existing Resource read boundary returned found=%v error=%v", found, err)
			}
			if byID.ID != result.Resource.ID || byID.Code != wantCode || byID.Name != "Test\t Resource  West" ||
				byID.Description != test.wantDesc || byID.Active != test.active {
				t.Fatalf("Resource read by ID = %+v", byID)
			}
			listed, err := store.ListResources(t.Context())
			if err != nil {
				t.Fatalf("list Resources through existing read boundary: %v", err)
			}
			if !containsResource(listed, result.Resource.ID) {
				t.Fatalf("created Resource %s not visible in existing Resource list", result.Resource.ID)
			}
			for _, listedResource := range listed {
				if listedResource.ID == result.Resource.ID && listedResource.Active != test.active {
					t.Fatalf("existing Resource list reports active=%v, want explicit initial state %v", listedResource.Active, test.active)
				}
			}
		})
	}
}

func TestResourceCreationPersistsLongTextWithoutProductLengthCaps(t *testing.T) {
	adminPool, appPool, fixtures := resourceCreationSetup(t)
	service := resources.NewService(NewStore(appPool), func() time.Time {
		return time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC)
	})
	code := "Long-" + strings.Repeat("C", 256)
	name := strings.Repeat("界", 4096)
	description := strings.Repeat("long description\tinside\n", 4096) + "end"
	created, err := service.Create(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.CreateRequest{
		Code: code, Name: name, Description: description, Active: true,
	})
	if err != nil {
		t.Fatalf("create Resource with long valid text: %v", err)
	}
	fixtures.trackResource(created.Resource.ID)
	persisted, found, err := NewStore(appPool).ResourceByID(t.Context(), created.Resource.ID)
	if err != nil || !found || persisted.Code != code || persisted.Name != name || persisted.Description != description {
		t.Fatalf("long Resource text was not persisted exactly: found=%v code-length=%d name-length=%d description-length=%d error=%v",
			found, len(persisted.Code), len(persisted.Name), len(persisted.Description), err)
	}
	assertResourceCreationActivity(t, adminPool, fixtures.coordinator, created.Resource)
}

func TestResourceCreationRejectsEngineerInvalidInputAndCaseInsensitiveDuplicates(t *testing.T) {
	adminPool, appPool, fixtures := resourceCreationSetup(t)
	now := time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC)
	coordinator := resources.Actor{AccountID: fixtures.coordinator}
	service := resources.NewService(NewStore(appPool), func() time.Time { return now })

	validCode := "Service-" + strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", "")
	for index, engineerID := range []string{fixtures.engineer, fixtures.secondEngineer} {
		engineer := resources.Actor{AccountID: engineerID}
		code := fmt.Sprintf("%s-engineer-%d", validCode, index)
		if _, err := service.Create(t.Context(), engineer, resources.CreateRequest{Code: code, Name: "Engineer attempt"}); !errors.Is(err, resources.ErrForbidden) {
			t.Fatalf("Engineer %d creation error = %v, want forbidden", index+1, err)
		}
		assertResourceCreationAbsent(t, adminPool, code)
	}
	unknown, err := service.Create(t.Context(), resources.Actor{AccountID: fixtures.unknown}, resources.CreateRequest{
		Code: validCode + "-unknown-actor", Name: "Unknown actor attempt",
	})
	if unknown.Resource.ID != "" || !errors.Is(err, resources.ErrUnauthenticated) {
		t.Fatalf("unknown actor creation result=%+v error=%v, want unauthenticated", unknown, err)
	}
	assertResourceCreationAbsent(t, adminPool, validCode+"-unknown-actor")

	for _, input := range []resources.CreateRequest{
		{Code: "", Name: "Name"},
		{Code: "\u2003\u00a0", Name: "Name"},
		{Code: validCode + "-missing-name", Name: ""},
		{Code: validCode + "-blank-name", Name: "\u2003\u00a0"},
	} {
		_, err := service.Create(t.Context(), coordinator, input)
		if !errors.Is(err, resources.ErrInvalidInput) {
			t.Fatalf("invalid Resource input %+v returned %v", input, err)
		}
		assertResourceCreationAbsent(t, adminPool, input.Code)
	}

	original, err := service.Create(t.Context(), coordinator, resources.CreateRequest{
		Code: validCode, Name: "Original Name", Description: "Original description", Active: true,
	})
	if err != nil {
		t.Fatalf("create duplicate-code fixture: %v", err)
	}
	fixtures.trackResource(original.Resource.ID)
	bookingRequest := bookings.CreateRequest{
		ResourceID: original.Resource.ID, RequestID: testDatabaseUUID(t, adminPool),
		StartAt: now.Add(time.Hour), EndAt: now.Add(2 * time.Hour), Purpose: "must survive duplicate Resource creation",
	}
	bookingResult, err := bookings.NewService(NewStore(appPool), func() time.Time { return now }).Create(
		t.Context(), bookings.Actor{AccountID: fixtures.coordinator}, bookingRequest)
	if err != nil {
		t.Fatalf("create Booking retained by duplicate-code fixture: %v", err)
	}
	cleanupResourceBookingFixture(t, adminPool, bookingResult.Booking.ID)
	bookingsBefore, bookingEventsBefore := resourceBookingAndEventSnapshot(t, adminPool, original.Resource.ID)
	for _, duplicateCode := range []string{
		validCode,
		" \u2003" + strings.ToLower(validCode) + "\u00a0",
	} {
		duplicate, err := service.Create(t.Context(), coordinator, resources.CreateRequest{
			Code: duplicateCode, Name: "Replacement", Description: "must not overwrite", Active: false,
		})
		if duplicate.Resource.ID != "" || !errors.Is(err, resources.ErrDuplicateCode) {
			t.Fatalf("duplicate code %q result=%+v error=%v", duplicateCode, duplicate, err)
		}
	}
	persisted, found, err := NewStore(appPool).ResourceByID(t.Context(), original.Resource.ID)
	if err != nil || !found || persisted.Name != "Original Name" || persisted.Description != "Original description" || !persisted.Active {
		t.Fatalf("duplicate changed existing Resource: found=%v resource=%+v error=%v", found, persisted, err)
	}
	var retainedCode, retainedName, retainedDescription string
	var retainedActive bool
	var retainedCreatedAt, retainedUpdatedAt time.Time
	if err := adminPool.QueryRow(t.Context(), `
		SELECT code, name, description, active, created_at, updated_at
		FROM public.resources WHERE id = $1::uuid`, original.Resource.ID).Scan(
		&retainedCode, &retainedName, &retainedDescription, &retainedActive, &retainedCreatedAt, &retainedUpdatedAt); err != nil {
		t.Fatalf("inspect Resource timestamps after duplicate requests: %v", err)
	}
	if retainedCode != original.Resource.Code || retainedName != original.Resource.Name || retainedDescription != original.Resource.Description ||
		retainedActive != original.Resource.Active || !retainedCreatedAt.Equal(original.Resource.CreatedAt) || !retainedUpdatedAt.Equal(original.Resource.UpdatedAt) {
		t.Fatalf("duplicate creation changed Resource metadata/timestamps: code=%q name=%q description=%q active=%v created=%s updated=%s",
			retainedCode, retainedName, retainedDescription, retainedActive, retainedCreatedAt, retainedUpdatedAt)
	}
	assertResourceCreationCounts(t, adminPool, validCode, 1, 1)
	assertResourceCreationActivity(t, adminPool, coordinator.AccountID, original.Resource)
	bookingsAfter, bookingEventsAfter := resourceBookingAndEventSnapshot(t, adminPool, original.Resource.ID)
	if bookingsBefore != bookingsAfter || bookingEventsBefore != bookingEventsAfter {
		t.Fatalf("duplicate Resource creation changed retained Booking/history:\nbookings before=%s\nafter=%s\nevents before=%s\nafter=%s",
			bookingsBefore, bookingsAfter, bookingEventsBefore, bookingEventsAfter)
	}
}

func TestResourceCreationActivityFailureRollsBackResource(t *testing.T) {
	adminPool, appPool, fixtures := resourceCreationSetup(t)
	now := time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC)
	actor := resources.Actor{AccountID: fixtures.coordinator}
	service := resources.NewService(NewStore(appPool), func() time.Time { return now })
	code := "Activity-Failure-" + strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", "")
	removeFailure := installResourceActivityFailureTrigger(t, adminPool, actor.AccountID, "resource.created")

	_, err := service.Create(t.Context(), actor, resources.CreateRequest{Code: code, Name: "Must roll back", Active: false})
	if !errors.Is(err, resources.ErrOperational) {
		t.Fatalf("Activity insertion failure = %v, want operational failure", err)
	}
	assertResourceCreationAbsent(t, adminPool, code)
	var actorEvents int
	if err := adminPool.QueryRow(t.Context(), `
		SELECT count(*) FROM public.activity_events
		WHERE actor_account_id = $1::uuid AND action = 'resource.created'`, actor.AccountID).Scan(&actorEvents); err != nil {
		t.Fatalf("inspect creation events after failed transaction: %v", err)
	}
	if actorEvents != 0 {
		t.Fatalf("failed Resource creation left %d successful creation events", actorEvents)
	}
	removeFailure()
}

func resourceCreationSetup(t *testing.T) (*pgxpool.Pool, *pgxpool.Pool, *resourceCapabilityFixtures) {
	t.Helper()
	adminPool := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	if err := Migrate(t.Context(), adminPool); err != nil {
		t.Fatalf("apply Resource creation schema: %v", err)
	}
	appPool := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	fixtures := newResourceCapabilityFixtures(t, adminPool)
	return adminPool, appPool, fixtures
}

func assertResourceCreationActivity(t *testing.T, pool *pgxpool.Pool, actorID string, resource resources.Resource) {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(), `
		SELECT count(*) FROM public.activity_events WHERE resource_id = $1::uuid`, resource.ID).Scan(&count); err != nil {
		t.Fatalf("count Resource creation Activity events: %v", err)
	}
	if count != 1 {
		t.Fatalf("successful Resource creation has %d Activity events, want exactly one", count)
	}
	var eventActor, action, targetID, detailsJSON string
	var bookingIsNull bool
	var occurredAt time.Time
	if err := pool.QueryRow(t.Context(), `
		SELECT actor_account_id::text, action, resource_id::text, booking_id IS NULL, occurred_at, details::text
		FROM public.activity_events WHERE resource_id = $1::uuid`, resource.ID).Scan(
		&eventActor, &action, &targetID, &bookingIsNull, &occurredAt, &detailsJSON); err != nil {
		t.Fatalf("read Resource creation Activity event: %v", err)
	}
	var details map[string]any
	if err := json.Unmarshal([]byte(detailsJSON), &details); err != nil {
		t.Fatalf("decode Resource creation Activity details: %v", err)
	}
	if eventActor != actorID || action != "resource.created" || targetID != resource.ID || !bookingIsNull ||
		!occurredAt.Equal(resource.CreatedAt) || !occurredAt.Equal(resource.UpdatedAt) ||
		details["code"] != resource.Code || details["name"] != resource.Name ||
		details["description"] != resource.Description || details["active"] != resource.Active {
		t.Fatalf("Resource creation Activity does not match persisted Resource: actor=%s action=%s target=%s booking-null=%v at=%s details=%v resource=%+v",
			eventActor, action, targetID, bookingIsNull, occurredAt, details, resource)
	}
}

func assertResourceCreationAbsent(t *testing.T, pool *pgxpool.Pool, code string) {
	t.Helper()
	canonicalCode := strings.TrimSpace(code)
	if canonicalCode == "" {
		canonicalCode = code
	}
	var resourcesCount, eventCount int
	if err := pool.QueryRow(t.Context(), `
		SELECT count(*),
		       (SELECT count(*) FROM public.activity_events e JOIN public.resources r ON r.id = e.resource_id
		        WHERE lower(r.code) = lower($1))
		FROM public.resources WHERE lower(code) = lower($1)`, canonicalCode).Scan(&resourcesCount, &eventCount); err != nil {
		t.Fatalf("inspect rejected Resource creation: %v", err)
	}
	if resourcesCount != 0 || eventCount != 0 {
		t.Fatalf("rejected Resource creation persisted rows/events %d/%d for code %q", resourcesCount, eventCount, code)
	}
}

func assertResourceCreationCounts(t *testing.T, pool *pgxpool.Pool, code string, wantResources, wantEvents int) {
	t.Helper()
	var resourceCount, eventCount int
	if err := pool.QueryRow(t.Context(), `
		SELECT count(*),
		       (SELECT count(*) FROM public.activity_events e JOIN public.resources r ON r.id = e.resource_id
	        WHERE lower(r.code) = lower($1))
		FROM public.resources WHERE lower(code) = lower($1)`, code).Scan(&resourceCount, &eventCount); err != nil {
		t.Fatalf("inspect Resource creation counts: %v", err)
	}
	if resourceCount != wantResources || eventCount != wantEvents {
		t.Fatalf("Resource code %q persisted resources/events %d/%d, want %d/%d", code, resourceCount, eventCount, wantResources, wantEvents)
	}
}

func containsResource(resourcesList []Resource, id string) bool {
	for _, resource := range resourcesList {
		if resource.ID == id {
			return true
		}
	}
	return false
}
