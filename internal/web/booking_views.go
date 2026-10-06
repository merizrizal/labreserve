package web

import (
	"crypto/rand"
	"encoding/hex"
	"math"
	"net/url"
	"strconv"
	"time"

	"labreserve.local/labreserve/internal/database"
)

const (
	bookingPageSize       int64 = 25
	bookingQueryPageLimit int64 = bookingPageSize + 1
	jakartaDateLayout           = "2006-01-02"
	jakartaDateTimeLayout       = "2006-01-02T15:04"
)

type bookingFormValues struct {
	RequestID string
	StartAt   string
	EndAt     string
	Purpose   string
}

type bookingScheduleRow struct {
	ID               string
	OwnerDisplayName string
	StartDisplay     string
	EndDisplay       string
	StartInstant     string
	EndInstant       string
	Purpose          string
	Status           string
	StatusClass      string
	Cancelled        bool
}

type bookingDetailView struct {
	ID               string
	ResourceID       string
	ResourceCode     string
	ResourceName     string
	OwnerDisplayName string
	StartDisplay     string
	EndDisplay       string
	StartInstant     string
	EndInstant       string
	Purpose          string
	Status           string
	StatusClass      string
}

func newBookingRequestID() (string, error) {
	var identifier [16]byte
	if _, err := rand.Read(identifier[:]); err != nil {
		return "", err
	}
	identifier[6] = identifier[6]&0x0f | 0x40
	identifier[8] = identifier[8]&0x3f | 0x80
	encoded := hex.EncodeToString(identifier[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}

func parseJakartaDate(value string, jakarta *time.Location) (time.Time, bool) {
	parsed, err := time.ParseInLocation(jakartaDateLayout, value, jakarta)
	if err != nil || parsed.Format(jakartaDateLayout) != value {
		return time.Time{}, false
	}
	return parsed, true
}

func parseJakartaDateTime(value string, jakarta *time.Location) (time.Time, bool) {
	parsed, err := time.ParseInLocation(jakartaDateTimeLayout, value, jakarta)
	if err != nil || parsed.Format(jakartaDateTimeLayout) != value {
		return time.Time{}, false
	}
	return parsed, true
}

func bookingPageOffset(query url.Values) (int64, int64, bool) {
	values, present := query["page"]
	if !present {
		return 1, 0, true
	}
	if len(values) != 1 || values[0] == "" {
		return 0, 0, false
	}
	page, err := strconv.ParseInt(values[0], 10, 64)
	if err != nil || page < 1 || page-1 > math.MaxInt64/bookingPageSize {
		return 0, 0, false
	}
	return page, (page - 1) * bookingPageSize, true
}

func bookingTimeStatus(state string, startAt, endAt, now time.Time) (string, string, bool) {
	if state == "cancelled" {
		return "Cancelled", "cancelled", true
	}
	if now.Before(startAt) {
		return "Upcoming", "upcoming", false
	}
	if now.Before(endAt) {
		return "In use", "in-use", false
	}
	return "Past", "past", false
}

func scheduleRows(views []database.BookingView, now time.Time, jakarta *time.Location) []bookingScheduleRow {
	rows := make([]bookingScheduleRow, 0, len(views))
	for _, view := range views {
		status, className, cancelled := bookingTimeStatus(view.State, view.StartAt, view.EndAt, now)
		rows = append(rows, bookingScheduleRow{
			ID: view.ID, OwnerDisplayName: view.OwnerDisplayName,
			StartDisplay: view.StartAt.In(jakarta).Format("02 Jan 2006 15:04"),
			EndDisplay:   view.EndAt.In(jakarta).Format("02 Jan 2006 15:04"),
			StartInstant: view.StartAt.UTC().Format(time.RFC3339Nano),
			EndInstant:   view.EndAt.UTC().Format(time.RFC3339Nano),
			Purpose:      view.Purpose, Status: status, StatusClass: className, Cancelled: cancelled,
		})
	}
	return rows
}

func bookingDetail(view database.BookingView, now time.Time, jakarta *time.Location) bookingDetailView {
	status, className, _ := bookingTimeStatus(view.State, view.StartAt, view.EndAt, now)
	return bookingDetailView{
		ID: view.ID, ResourceID: view.ResourceID, ResourceCode: view.ResourceCode,
		ResourceName: view.ResourceName, OwnerDisplayName: view.OwnerDisplayName,
		StartDisplay: view.StartAt.In(jakarta).Format("02 Jan 2006 15:04"),
		EndDisplay:   view.EndAt.In(jakarta).Format("02 Jan 2006 15:04"),
		StartInstant: view.StartAt.UTC().Format(time.RFC3339Nano),
		EndInstant:   view.EndAt.UTC().Format(time.RFC3339Nano),
		Purpose:      view.Purpose, Status: status, StatusClass: className,
	}
}
