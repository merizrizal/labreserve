package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"labreserve.local/labreserve/internal/auth"
	"labreserve.local/labreserve/internal/bookings"
)

const sessionCookieName = "labreserve_session"

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	state, _ := requestStateFrom(r)
	if state.hasSession && state.session.Identity != nil {
		http.Redirect(w, r, "/resources", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	state, _ := requestStateFrom(r)
	if state.hasSession && state.session.Identity != nil {
		http.Redirect(w, r, "/resources", http.StatusSeeOther)
		return
	}
	if !state.hasSession {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		token, session, err := s.auth.StartAnonymous(ctx)
		cancel()
		if err != nil {
			http.Error(w, "Authentication service is temporarily unavailable.", http.StatusServiceUnavailable)
			return
		}
		s.setSessionCookie(w, token, session.ExpiresAt)
		state = requestState{session: session, hasSession: true, cookieToken: token}
		r = r.WithContext(context.WithValue(r.Context(), requestStateKey{}, state))
	}
	s.render(w, r, "login.html", http.StatusOK, PageData{Title: "Sign in"})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	state, _ := requestStateFrom(r)
	if state.session.Identity != nil {
		http.Redirect(w, r, "/resources", http.StatusSeeOther)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	token, session, err := s.auth.Login(ctx, state.cookieToken, r.PostForm.Get("login"), r.PostForm.Get("password"))
	cancel()
	if errors.Is(err, auth.ErrInvalidCredentials) {
		s.render(w, r, "login.html", http.StatusUnauthorized, PageData{
			Title: "Sign in", Error: "The login or password was not recognized.", Login: r.PostForm.Get("login"),
		})
		return
	}
	if err != nil {
		http.Error(w, "Authentication service is temporarily unavailable.", http.StatusServiceUnavailable)
		return
	}
	s.setSessionCookie(w, token, session.ExpiresAt)
	http.Redirect(w, r, "/resources", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	state, ok := requestStateFrom(r)
	if !ok || !state.hasSession || state.session.Identity == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	err := s.auth.Logout(ctx, state.cookieToken)
	cancel()
	if err != nil {
		http.Error(w, "Unable to sign out right now.", http.StatusServiceUnavailable)
		return
	}
	clearSessionCookie(w, s.secureCookie)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) resourceList(w http.ResponseWriter, r *http.Request) {
	if !s.requireIdentity(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	resources, err := s.store.ListResources(ctx)
	cancel()
	if err != nil {
		http.Error(w, "Resources are temporarily unavailable.", http.StatusServiceUnavailable)
		return
	}
	s.render(w, r, "resources.html", http.StatusOK, PageData{Title: "Resources", Resources: resources})
}

func (s *Server) resourceDetail(w http.ResponseWriter, r *http.Request) {
	if !s.requireIdentity(w, r) {
		return
	}
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	resource, found, err := s.store.ResourceByID(ctx, id)
	if err != nil {
		http.Error(w, "The resource is temporarily unavailable.", http.StatusServiceUnavailable)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	query := r.URL.Query()
	page, offset, validPage := bookingPageOffset(query)
	pageNow := s.now()
	date := pageNow.In(s.jakarta).Format(jakartaDateLayout)
	if values, present := query["date"]; present {
		if len(values) != 1 || values[0] == "" {
			http.Error(w, "Choose a valid schedule date.", http.StatusBadRequest)
			return
		}
		date = values[0]
	}
	dayStart, validDate := parseJakartaDate(date, s.jakarta)
	if !validPage || !validDate {
		http.Error(w, "Choose a valid schedule date and page.", http.StatusBadRequest)
		return
	}
	dayEnd := dayStart.AddDate(0, 0, 1)
	views, err := s.store.ListResourceBookingViews(ctx, resource.ID, dayStart, dayEnd, bookingQueryPageLimit, offset)
	if err != nil {
		http.Error(w, "The resource schedule is temporarily unavailable.", http.StatusServiceUnavailable)
		return
	}
	hasNext := int64(len(views)) > bookingPageSize
	if hasNext {
		views = views[:bookingPageSize]
	}
	rows := scheduleRows(views, pageNow, s.jakarta)
	s.render(w, r, "resource.html", http.StatusOK, PageData{
		Title: resource.Code + " schedule", Resource: resource, Date: date,
		Schedule: rows, HasSchedule: len(rows) != 0, Page: page,
		HasPreviousPage: page > 1, HasNextPage: hasNext,
		PreviousPage: page - 1, NextPage: page + 1,
	})
}

func (s *Server) bookingFormPage(w http.ResponseWriter, r *http.Request) {
	if !s.requireIdentity(w, r) {
		return
	}
	resourceID := r.PathValue("id")
	if !uuidPattern.MatchString(resourceID) {
		http.NotFound(w, r)
		return
	}
	requestID, err := newBookingRequestID()
	if err != nil {
		http.Error(w, "Unable to prepare a booking form.", http.StatusInternalServerError)
		return
	}
	s.renderBookingForm(w, r, http.StatusOK, resourceID, bookingFormValues{RequestID: requestID}, "")
}

func (s *Server) createBooking(w http.ResponseWriter, r *http.Request) {
	if !s.requireIdentity(w, r) {
		return
	}
	resourceID := r.PathValue("id")
	if !uuidPattern.MatchString(resourceID) {
		http.NotFound(w, r)
		return
	}
	form := bookingFormValues{
		RequestID: r.PostForm.Get("request_id"),
		StartAt:   r.PostForm.Get("start_jakarta"),
		EndAt:     r.PostForm.Get("end_jakarta"),
		Purpose:   r.PostForm.Get("purpose"),
	}
	for _, field := range []string{"request_id", "start_jakarta", "end_jakarta", "purpose"} {
		if _, ok := singlePostValue(r, field); !ok {
			s.renderBookingForm(w, r, http.StatusUnprocessableEntity, resourceID, form, "Complete each booking field and submit the form again.")
			return
		}
	}
	if !uuidPattern.MatchString(form.RequestID) {
		s.renderBookingForm(w, r, http.StatusUnprocessableEntity, resourceID, form, "The request identifier is invalid. Start a new booking form to obtain a fresh identifier.")
		return
	}
	startAt, startOK := parseJakartaDateTime(form.StartAt, s.jakarta)
	endAt, endOK := parseJakartaDateTime(form.EndAt, s.jakarta)
	if !startOK || !endOK {
		s.renderBookingForm(w, r, http.StatusUnprocessableEntity, resourceID, form, "Enter valid start and end date/time values in the Asia/Jakarta format.")
		return
	}
	state, _ := requestStateFrom(r)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	result, err := s.bookingService.Create(ctx, bookings.Actor{AccountID: state.session.Identity.ID}, bookings.CreateRequest{
		ResourceID: resourceID, RequestID: form.RequestID,
		StartAt: startAt, EndAt: endAt, Purpose: form.Purpose,
	})
	cancel()
	if err == nil {
		outcome := "created"
		if result.Replayed {
			outcome = "replayed"
		}
		http.Redirect(w, r, fmt.Sprintf("/bookings/%s?result=%s", result.Booking.ID, outcome), http.StatusSeeOther)
		return
	}
	switch {
	case errors.Is(err, bookings.ErrInvalidInput):
		s.renderBookingForm(w, r, http.StatusUnprocessableEntity, resourceID, form, "The booking does not meet the time, duration, or purpose rules. Check the values and try again.")
	case errors.Is(err, bookings.ErrUnknownResource):
		http.NotFound(w, r)
	case errors.Is(err, bookings.ErrInactiveResource):
		s.renderBookingForm(w, r, http.StatusConflict, resourceID, form, "This resource is inactive and cannot currently be booked.")
	case errors.Is(err, bookings.ErrConflict):
		s.renderBookingForm(w, r, http.StatusConflict, resourceID, form, "The selected interval is no longer available. Choose another interval and retry this form.")
	case errors.Is(err, bookings.ErrRequestIDReuse):
		s.renderBookingForm(w, r, http.StatusConflict, resourceID, form, "This request identifier already belongs to different booking data. Do not change it to retry; start a new booking form for a new request.")
	case errors.Is(err, bookings.ErrOperational):
		http.Error(w, "Booking is temporarily unavailable. Retry the same submitted form.", http.StatusServiceUnavailable)
	default:
		http.Error(w, "Unable to process this booking request.", http.StatusInternalServerError)
	}
}

func (s *Server) renderBookingForm(w http.ResponseWriter, r *http.Request, status int, resourceID string, form bookingFormValues, errorMessage string) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	resource, found, err := s.store.ResourceByID(ctx, resourceID)
	if err != nil {
		http.Error(w, "The resource is temporarily unavailable.", http.StatusServiceUnavailable)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	s.render(w, r, "booking_form.html", status, PageData{
		Title: "Book " + resource.Code, Resource: resource,
		BookingForm: form, Error: errorMessage,
	})
}

func singlePostValue(r *http.Request, name string) (string, bool) {
	values, present := r.PostForm[name]
	if !present || len(values) != 1 {
		return "", false
	}
	return values[0], true
}

func (s *Server) myBookings(w http.ResponseWriter, r *http.Request) {
	if !s.requireIdentity(w, r) {
		return
	}
	state, _ := requestStateFrom(r)
	identity := state.session.Identity
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		http.Error(w, "Choose a valid bookings page.", http.StatusBadRequest)
		return
	}
	page, offset, validPage := bookingPageOffset(query)
	if !validPage {
		http.Error(w, "Choose a valid bookings page.", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	now := s.now()
	views, err := s.store.ListMyBookingViews(ctx, identity.ID, bookingQueryPageLimit, offset)
	if err != nil {
		http.Error(w, "Your bookings are temporarily unavailable.", http.StatusServiceUnavailable)
		return
	}
	hasNext := int64(len(views)) > bookingPageSize
	if hasNext {
		views = views[:bookingPageSize]
	}
	rows := myBookingRows(views, now, s.jakarta)
	s.render(w, r, "my_bookings.html", http.StatusOK, PageData{
		Title: "My Bookings", MyBookings: rows, HasMyBookings: len(rows) != 0,
		Page: page, HasPreviousPage: page > 1, HasNextPage: hasNext,
		PreviousPage: page - 1, NextPage: page + 1,
	})
}

func (s *Server) bookingDetailPage(w http.ResponseWriter, r *http.Request) {
	if !s.requireIdentity(w, r) {
		return
	}
	bookingID := r.PathValue("id")
	if !uuidPattern.MatchString(bookingID) {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	view, found, err := s.store.BookingViewByID(ctx, bookingID)
	cancel()
	if err != nil {
		http.Error(w, "The booking is temporarily unavailable.", http.StatusServiceUnavailable)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	result := r.URL.Query().Get("result")
	if result != "created" && result != "replayed" {
		result = ""
	}
	detail := bookingDetail(view, s.now(), s.jakarta)
	s.render(w, r, "booking.html", http.StatusOK, PageData{
		Title: "Booking " + view.ID, Booking: &detail,
		BookingResult: result, BookingDate: view.StartAt.In(s.jakarta).Format(jakartaDateLayout),
	})
}

func (s *Server) stylesheet(w http.ResponseWriter, r *http.Request) {
	content, err := assets.ReadFile("static/style.css")
	if err != nil {
		http.Error(w, "Stylesheet unavailable.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (s *Server) requireIdentity(w http.ResponseWriter, r *http.Request) bool {
	state, ok := requestStateFrom(r)
	if ok && state.hasSession && state.session.Identity != nil {
		return true
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
	return false
}
