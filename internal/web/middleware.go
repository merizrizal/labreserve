package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"labreserve.local/labreserve/internal/auth"
	"labreserve.local/labreserve/internal/database"
)

type requestStateKey struct{}

type requestState struct {
	session     database.Session
	hasSession  bool
	cookieToken string
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'; style-src 'self'")

		state := requestState{}
		if cookie, err := r.Cookie(sessionCookieName); err == nil {
			state.cookieToken = cookie.Value
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			session, found, resolveErr := s.auth.Resolve(ctx, cookie.Value)
			cancel()
			if resolveErr != nil {
				http.Error(w, "Authentication service is temporarily unavailable.", http.StatusServiceUnavailable)
				return
			}
			state.session, state.hasSession = session, found
		}
		r = r.WithContext(context.WithValue(r.Context(), requestStateKey{}, state))

		if isStateChanging(r.Method) {
			if !requestOriginAllowed(r, s.publicOrigin) {
				http.Error(w, "Request rejected.", http.StatusForbidden)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Request rejected.", http.StatusBadRequest)
				return
			}
			if !state.hasSession || !auth.ValidCSRF(state.session, r.PostForm.Get("_csrf")) {
				http.Error(w, "Request rejected.", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func requestStateFrom(r *http.Request) (requestState, bool) {
	state, ok := r.Context().Value(requestStateKey{}).(requestState)
	return state, ok
}

func isStateChanging(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

func requestOriginAllowed(r *http.Request, expected string) bool {
	candidate := r.Header.Get("Origin")
	fromReferer := false
	if candidate == "" {
		candidate = r.Header.Get("Referer")
		fromReferer = candidate != ""
	}
	if candidate == "" {
		return true
	}
	actual, err := url.Parse(candidate)
	if err != nil || actual.User != nil || actual.Host == "" || actual.Opaque != "" {
		return false
	}
	if !fromReferer && (actual.Path != "" || actual.RawQuery != "" || actual.Fragment != "") {
		return false
	}
	configured, err := url.Parse(expected)
	if err != nil || configured.Host == "" {
		return false
	}
	actualOrigin, actualOK := originKey(actual)
	expectedOrigin, expectedOK := originKey(configured)
	return actualOK && expectedOK && actualOrigin == expectedOrigin
}

func originKey(value *url.URL) (string, bool) {
	scheme := strings.ToLower(value.Scheme)
	host := strings.ToLower(value.Hostname())
	if host == "" || (scheme != "http" && scheme != "https") {
		return "", false
	}
	port := value.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return fmt.Sprintf("%s://%s:%s", scheme, host, port), true
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	maxAge := int(expires.Sub(s.now()).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: token, Path: "/", Expires: expires,
		MaxAge: maxAge, HttpOnly: true, Secure: s.secureCookie, SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1,
		Expires: time.Unix(1, 0), HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
}
