package web

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"time"

	"labreserve.local/labreserve/internal/auth"
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
	resource, found, err := s.store.ResourceByID(ctx, id)
	cancel()
	if err != nil {
		http.Error(w, "The resource is temporarily unavailable.", http.StatusServiceUnavailable)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	date := r.URL.Query().Get("date")
	if date == "" {
		date = s.now().In(s.jakarta).Format("2006-01-02")
	}
	if _, err := time.ParseInLocation("2006-01-02", date, s.jakarta); err != nil {
		http.Error(w, "Choose a valid schedule date.", http.StatusBadRequest)
		return
	}
	s.render(w, r, "resource.html", http.StatusOK, PageData{
		Title: resource.Code + " schedule", Resource: resource, Date: date,
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
