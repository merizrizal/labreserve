package web

import (
	"net/url"
	"testing"
	"time"
)

func TestBookingRequestIDsAreFreshUUIDs(t *testing.T) {
	first, err := newBookingRequestID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := newBookingRequestID()
	if err != nil {
		t.Fatal(err)
	}
	if !uuidPattern.MatchString(first) || !uuidPattern.MatchString(second) || first == second {
		t.Fatalf("generated request identifiers = %q and %q; want distinct UUIDs", first, second)
	}
	if first[14] != '4' || (first[19] != '8' && first[19] != '9' && first[19] != 'a' && first[19] != 'b') {
		t.Fatalf("request identifier %q is not a UUID version 4", first)
	}
}

func TestJakartaDateAndDateTimeParsing(t *testing.T) {
	jakarta, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	date, valid := parseJakartaDate("2040-03-02", jakarta)
	if !valid || date.Location() != jakarta || date.Format(jakartaDateLayout) != "2040-03-02" {
		t.Fatalf("parsed Jakarta date = %v, valid = %v", date, valid)
	}
	for _, invalid := range []string{"2040-02-30", "2040-3-02", "2040-03-02T00:00", ""} {
		if _, valid := parseJakartaDate(invalid, jakarta); valid {
			t.Errorf("accepted invalid Jakarta date %q", invalid)
		}
	}

	local, valid := parseJakartaDateTime("2040-03-02T15:04", jakarta)
	expected := time.Date(2040, time.March, 2, 15, 4, 0, 0, jakarta)
	if !valid || !local.Equal(expected) || local.Location() != jakarta {
		t.Fatalf("parsed Jakarta date/time = %v, valid = %v; want %v", local, valid, expected)
	}
	for _, invalid := range []string{"2040-02-30T15:04", "2040-03-02T15:04Z", "2040-03-02T15:04:00", "2040-03-02 15:04", ""} {
		if _, valid := parseJakartaDateTime(invalid, jakarta); valid {
			t.Errorf("accepted invalid Jakarta date/time %q", invalid)
		}
	}
}

func TestBookingTimeStatusBoundaries(t *testing.T) {
	start := time.Date(2040, time.January, 2, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	cases := []struct {
		name  string
		state string
		now   time.Time
		want  string
	}{
		{name: "before start", state: "confirmed", now: start.Add(-time.Nanosecond), want: "Upcoming"},
		{name: "at start", state: "confirmed", now: start, want: "In use"},
		{name: "before end", state: "confirmed", now: end.Add(-time.Nanosecond), want: "In use"},
		{name: "at end", state: "confirmed", now: end, want: "Past"},
		{name: "cancelled", state: "cancelled", now: start, want: "Cancelled"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, _, _ := bookingTimeStatus(test.state, start, end, test.now)
			if got != test.want {
				t.Fatalf("booking status = %q; want %q", got, test.want)
			}
		})
	}
}

func TestBookingPageOffsetValidation(t *testing.T) {
	cases := []struct {
		query      url.Values
		wantPage   int64
		wantOffset int64
		valid      bool
	}{
		{query: url.Values{}, wantPage: 1, wantOffset: 0, valid: true},
		{query: url.Values{"page": {"1"}}, wantPage: 1, wantOffset: 0, valid: true},
		{query: url.Values{"page": {"2"}}, wantPage: 2, wantOffset: 25, valid: true},
		{query: url.Values{"page": {"0"}}, valid: false},
		{query: url.Values{"page": {"-1"}}, valid: false},
		{query: url.Values{"page": {"1", "2"}}, valid: false},
		{query: url.Values{"page": {"not-a-page"}}, valid: false},
		{query: url.Values{"page": {"999999999999999999999999"}}, valid: false},
	}
	for _, test := range cases {
		page, offset, valid := bookingPageOffset(test.query)
		if page != test.wantPage || offset != test.wantOffset || valid != test.valid {
			t.Errorf("bookingPageOffset(%v) = (%d, %d, %v); want (%d, %d, %v)", test.query, page, offset, valid, test.wantPage, test.wantOffset, test.valid)
		}
	}
}
