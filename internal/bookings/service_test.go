package bookings

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	testOwnerID    = "11111111-1111-4111-8111-111111111111"
	testResourceID = "22222222-2222-4222-8222-222222222222"
	testRequestID  = "33333333-3333-4333-8333-333333333333"
)

func TestCreatePersistsCanonicalBookingForAuthenticatedActor(t *testing.T) {
	now := time.Date(2040, time.January, 2, 10, 0, 0, 123456000, time.UTC)
	repository := &fakeRepository{resource: Resource{ID: testResourceID, Active: true}}
	clockCalls := 0
	service := NewService(repository, func() time.Time {
		clockCalls++
		return now
	})
	start := now.Add(5 * time.Minute).Add(999 * time.Nanosecond)
	result, err := service.Create(context.Background(), Actor{AccountID: strings.ToUpper(testOwnerID)}, CreateRequest{
		ResourceID: strings.ToUpper(testResourceID), RequestID: strings.ToUpper(testRequestID),
		StartAt: start, EndAt: start.Add(time.Hour), Purpose: "\u2003Network test\u00a0",
	})
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}
	if result.Replayed || result.Booking.ID == "" || result.Booking.OwnerAccountID != testOwnerID {
		t.Fatalf("creation result = %+v, want new booking owned by authenticated actor", result)
	}
	if result.Booking.ResourceID != testResourceID || result.Booking.RequestID != testRequestID ||
		result.Booking.Purpose != "Network test" || !result.Booking.StartAt.Equal(now.Add(5*time.Minute)) ||
		!result.Booking.CreatedAt.Equal(now) || result.Booking.State != StateConfirmed {
		t.Fatalf("created booking was not canonical: %+v", result.Booking)
	}
	if clockCalls != 1 {
		t.Fatalf("authoritative clock called %d times, want once after locks", clockCalls)
	}
	if len(repository.bookings) != 1 || len(repository.events) != 1 {
		t.Fatalf("committed bookings/events = %d/%d, want exactly one each", len(repository.bookings), len(repository.events))
	}
	event := repository.events[0]
	if event.ActorAccountID != testOwnerID || event.Booking.ID != result.Booking.ID || !event.OccurredAt.Equal(now) {
		t.Fatalf("creation event does not identify actor, booking, and operation time: %+v", event)
	}
	if !reflect.DeepEqual(repository.transactions[0].trace, []string{"account", "lookup", "resource", "insert", "event", "commit"}) {
		t.Fatalf("transaction order = %v", repository.transactions[0].trace)
	}
}

func TestCreateReplayDoesNotRecheckCurrentEligibility(t *testing.T) {
	createdAt := time.Date(2040, time.January, 2, 10, 0, 0, 0, time.UTC)
	existing := Booking{
		ID: "booking-1", ResourceID: testResourceID, OwnerAccountID: testOwnerID,
		RequestID: testRequestID, StartAt: createdAt.Add(5 * time.Minute),
		EndAt: createdAt.Add(65 * time.Minute), Purpose: "original", State: StateCancelled,
		CreatedAt: createdAt,
	}
	repository := &fakeRepository{
		bookings: []Booking{existing}, resource: Resource{ID: testResourceID, Active: false},
	}
	clockCalls := 0
	service := NewService(repository, func() time.Time {
		clockCalls++
		return createdAt.Add(90 * time.Minute)
	})
	result, err := service.Create(context.Background(), Actor{AccountID: testOwnerID}, CreateRequest{
		ResourceID: testResourceID, RequestID: testRequestID,
		StartAt: existing.StartAt, EndAt: existing.EndAt, Purpose: "\u2003original ",
	})
	if err != nil {
		t.Fatalf("replay retained cancelled booking: %v", err)
	}
	if !result.Replayed || result.Booking.ID != existing.ID || clockCalls != 0 {
		t.Fatalf("replay = %+v, clock calls = %d; want existing booking without current eligibility checks", result, clockCalls)
	}
	if !reflect.DeepEqual(repository.transactions[0].trace, []string{"account", "lookup"}) {
		t.Fatalf("replay transaction continued beyond lookup: %v", repository.transactions[0].trace)
	}
	if len(repository.events) != 0 {
		t.Fatalf("replay created %d activity events, want none", len(repository.events))
	}
}

func TestCreateChangedRequestIsRejectedBeforeResourceEligibility(t *testing.T) {
	now := time.Date(2040, time.January, 2, 10, 0, 0, 0, time.UTC)
	original := Booking{
		ID: "booking-1", ResourceID: testResourceID, OwnerAccountID: testOwnerID,
		RequestID: testRequestID, StartAt: now.Add(5 * time.Minute),
		EndAt: now.Add(65 * time.Minute), Purpose: "original", State: StateConfirmed,
		CreatedAt: now,
	}
	mutations := []struct {
		name   string
		change func(*CreateRequest)
	}{
		{name: "resource", change: func(request *CreateRequest) { request.ResourceID = "44444444-4444-4444-8444-444444444444" }},
		{name: "start", change: func(request *CreateRequest) { request.StartAt = request.StartAt.Add(time.Microsecond) }},
		{name: "end", change: func(request *CreateRequest) { request.EndAt = request.EndAt.Add(time.Microsecond) }},
		{name: "purpose", change: func(request *CreateRequest) { request.Purpose = "changed" }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			repository := &fakeRepository{
				bookings: []Booking{original}, resource: Resource{ID: original.ResourceID, Active: false},
			}
			service := NewService(repository, func() time.Time { return now })
			request := CreateRequest{
				ResourceID: original.ResourceID, RequestID: original.RequestID,
				StartAt: original.StartAt, EndAt: original.EndAt, Purpose: original.Purpose,
			}
			mutation.change(&request)
			_, err := service.Create(context.Background(), Actor{AccountID: testOwnerID}, request)
			if !errors.Is(err, ErrRequestIDReuse) {
				t.Fatalf("changed request error = %v, want request-identifier reuse", err)
			}
			if !reflect.DeepEqual(repository.transactions[0].trace, []string{"account", "lookup"}) {
				t.Fatalf("changed request checked Resource or clock before reuse: %v", repository.transactions[0].trace)
			}
		})
	}
}

func TestCreateUsesClockAfterResourceLock(t *testing.T) {
	now := time.Date(2040, time.January, 2, 10, 0, 0, 0, time.UTC)
	later := now.Add(time.Microsecond)
	repository := &fakeRepository{resource: Resource{ID: testResourceID, Active: true}}
	repository.onResourceLock = func() { now = later }
	service := NewService(repository, func() time.Time { return now })
	request := validCreateRequest(now)
	request.StartAt = now.Add(5 * time.Minute)
	request.EndAt = request.StartAt.Add(time.Hour)
	_, err := service.Create(context.Background(), Actor{AccountID: testOwnerID}, request)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("creation after clock crossed minimum horizon returned %v, want invalid input", err)
	}
	if len(repository.bookings) != 0 || len(repository.events) != 0 {
		t.Fatalf("stale-time booking committed: bookings=%d events=%d", len(repository.bookings), len(repository.events))
	}
	if !reflect.DeepEqual(repository.transactions[0].trace, []string{"account", "lookup", "resource"}) {
		t.Fatalf("validation order = %v", repository.transactions[0].trace)
	}
}

func TestCreateRollsBackWhenActivityAppendFailsAndDoesNotConsumeRequestID(t *testing.T) {
	now := time.Date(2040, time.January, 2, 10, 0, 0, 0, time.UTC)
	repository := &fakeRepository{resource: Resource{ID: testResourceID, Active: true}, eventError: errors.New("injected failure")}
	service := NewService(repository, func() time.Time { return now })
	request := validCreateRequest(now)
	if _, err := service.Create(context.Background(), Actor{AccountID: testOwnerID}, request); !errors.Is(err, ErrOperational) {
		t.Fatalf("event append error = %v, want operational failure", err)
	}
	if len(repository.bookings) != 0 || len(repository.events) != 0 {
		t.Fatalf("failed event append retained state: bookings=%d events=%d", len(repository.bookings), len(repository.events))
	}
	repository.eventError = nil
	result, err := service.Create(context.Background(), Actor{AccountID: testOwnerID}, request)
	if err != nil || result.Replayed {
		t.Fatalf("retry after rolled-back creation = %+v, %v; want new creation", result, err)
	}
	if len(repository.bookings) != 1 || len(repository.events) != 1 {
		t.Fatalf("successful retry retained bookings/events = %d/%d, want one each", len(repository.bookings), len(repository.events))
	}
}

func TestCanonicalPurposeAndInstants(t *testing.T) {
	if got := strings.TrimSpace("\u2003 purpose \u00a0"); got != "purpose" {
		t.Fatalf("Unicode-trimmed purpose = %q", got)
	}
	utc := time.Date(2040, time.January, 2, 10, 0, 0, 123456000, time.UTC)
	plusSeven := time.FixedZone("UTC+7", 7*60*60)
	equivalent := time.Date(2040, time.January, 2, 17, 0, 0, 123456999, plusSeven)
	if !canonicalInstant(utc).Equal(canonicalInstant(equivalent)) {
		t.Fatalf("equivalent offset/microsecond instants differ: %v vs %v", canonicalInstant(utc), canonicalInstant(equivalent))
	}

	original := Booking{
		ResourceID: testResourceID, StartAt: utc, EndAt: utc.Add(time.Hour), Purpose: "Case é",
	}
	baseRequest := CreateRequest{
		ResourceID: testResourceID, StartAt: equivalent, EndAt: equivalent.Add(time.Hour), Purpose: "Case é",
	}
	if !sameCanonicalRequest(original, baseRequest) {
		t.Fatal("equivalent offsets and microsecond-equivalent instants were not canonical-equal")
	}
	for _, purpose := range []string{"case é", "Case e\u0301"} {
		changed := baseRequest
		changed.Purpose = purpose
		if sameCanonicalRequest(original, changed) {
			t.Fatalf("canonical purpose comparison rewrote distinct purpose %q", purpose)
		}
	}
}

func TestValidateNewBookingBoundaries(t *testing.T) {
	now := time.Date(2040, time.January, 2, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		start     time.Time
		end       time.Time
		purpose   string
		wantError bool
	}{
		{name: "minimum start lead inclusive", start: now.Add(minimumStartLead), end: now.Add(minimumStartLead + minimumDuration), purpose: "x"},
		{name: "start below minimum by one microsecond", start: now.Add(minimumStartLead - time.Microsecond), end: now.Add(minimumStartLead + minimumDuration - time.Microsecond), purpose: "x", wantError: true},
		{name: "maximum start lead inclusive", start: now.Add(maximumStartLead), end: now.Add(maximumStartLead + minimumDuration), purpose: "x"},
		{name: "start beyond maximum by one microsecond", start: now.Add(maximumStartLead + time.Microsecond), end: now.Add(maximumStartLead + time.Microsecond + minimumDuration), purpose: "x", wantError: true},
		{name: "minimum duration inclusive", start: now.Add(time.Hour), end: now.Add(time.Hour + minimumDuration), purpose: "x"},
		{name: "duration below minimum", start: now.Add(time.Hour), end: now.Add(time.Hour + minimumDuration - time.Microsecond), purpose: "x", wantError: true},
		{name: "maximum duration inclusive", start: now.Add(time.Hour), end: now.Add(time.Hour + maximumDuration), purpose: "x"},
		{name: "duration above maximum", start: now.Add(time.Hour), end: now.Add(time.Hour + maximumDuration + time.Microsecond), purpose: "x", wantError: true},
		{name: "end equals start", start: now.Add(time.Hour), end: now.Add(time.Hour), purpose: "x", wantError: true},
		{name: "end before start", start: now.Add(time.Hour), end: now.Add(time.Hour - time.Microsecond), purpose: "x", wantError: true},
		{name: "cross midnight", start: time.Date(2040, time.January, 2, 23, 45, 0, 0, time.UTC), end: time.Date(2040, time.January, 3, 0, 15, 0, 0, time.UTC), purpose: "x"},
		{name: "one Unicode code point", start: now.Add(time.Hour), end: now.Add(time.Hour + minimumDuration), purpose: "🙂"},
		{name: "200 Unicode code points", start: now.Add(time.Hour), end: now.Add(time.Hour + minimumDuration), purpose: strings.Repeat("界", 200)},
		{name: "201 Unicode code points", start: now.Add(time.Hour), end: now.Add(time.Hour + minimumDuration), purpose: strings.Repeat("界", 201), wantError: true},
		{name: "empty purpose", start: now.Add(time.Hour), end: now.Add(time.Hour + minimumDuration), purpose: "", wantError: true},
		{name: "whitespace purpose", start: now.Add(time.Hour), end: now.Add(time.Hour + minimumDuration), purpose: " \u2003 ", wantError: true},
		{name: "invalid UTF-8 purpose", start: now.Add(time.Hour), end: now.Add(time.Hour + minimumDuration), purpose: string([]byte{0xff}), wantError: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := validateNewBooking(CreateRequest{
				StartAt: canonicalInstant(test.start), EndAt: canonicalInstant(test.end),
				Purpose: strings.TrimSpace(test.purpose),
			}, now)
			if test.wantError && !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("validation error = %v, want invalid input", err)
			}
			if !test.wantError && err != nil {
				t.Fatalf("valid boundary rejected: %v", err)
			}
		})
	}
}

func validCreateRequest(now time.Time) CreateRequest {
	start := canonicalInstant(now.Add(time.Hour))
	return CreateRequest{
		ResourceID: testResourceID, RequestID: testRequestID,
		StartAt: start, EndAt: start.Add(time.Hour), Purpose: "purpose",
	}
}

type fakeRepository struct {
	bookings       []Booking
	resource       Resource
	events         []BookingCreatedEvent
	transactions   []*fakeTransaction
	eventError     error
	onResourceLock func()
	nextID         int
}

func (r *fakeRepository) BeginBookingCreation(context.Context) (Transaction, error) {
	tx := &fakeTransaction{repository: r}
	r.transactions = append(r.transactions, tx)
	return tx, nil
}

type fakeTransaction struct {
	repository *fakeRepository
	trace      []string
	pending    *Booking
	pendingEvt *BookingCreatedEvent
}

func (tx *fakeTransaction) LockAccount(context.Context, string) (bool, error) {
	tx.trace = append(tx.trace, "account")
	return true, nil
}

func (tx *fakeTransaction) FindBookingByRequest(_ context.Context, ownerID, requestID string) (Booking, bool, error) {
	tx.trace = append(tx.trace, "lookup")
	for _, booking := range tx.repository.bookings {
		if booking.OwnerAccountID == ownerID && booking.RequestID == requestID {
			return booking, true, nil
		}
	}
	return Booking{}, false, nil
}

func (tx *fakeTransaction) LockResource(_ context.Context, resourceID string) (Resource, bool, error) {
	tx.trace = append(tx.trace, "resource")
	if tx.repository.onResourceLock != nil {
		tx.repository.onResourceLock()
	}
	return tx.repository.resource, tx.repository.resource.ID == resourceID, nil
}

func (tx *fakeTransaction) InsertBooking(_ context.Context, booking Booking) (Booking, error) {
	tx.trace = append(tx.trace, "insert")
	tx.repository.nextID++
	booking.ID = "booking-" + strconv.Itoa(tx.repository.nextID)
	tx.pending = &booking
	return booking, nil
}

func (tx *fakeTransaction) AppendBookingCreatedEvent(_ context.Context, event BookingCreatedEvent) error {
	tx.trace = append(tx.trace, "event")
	if tx.repository.eventError != nil {
		return tx.repository.eventError
	}
	tx.pendingEvt = &event
	return nil
}

func (tx *fakeTransaction) Commit(context.Context) error {
	tx.trace = append(tx.trace, "commit")
	if tx.pending != nil {
		tx.repository.bookings = append(tx.repository.bookings, *tx.pending)
	}
	if tx.pendingEvt != nil {
		tx.repository.events = append(tx.repository.events, *tx.pendingEvt)
	}
	return nil
}

func (tx *fakeTransaction) Rollback(context.Context) error { return nil }
