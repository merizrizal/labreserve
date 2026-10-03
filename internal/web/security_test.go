package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"labreserve.local/labreserve/internal/config"
	"labreserve.local/labreserve/internal/database"
)

func TestResourceTemplateEscapesHTMLLikeContent(t *testing.T) {
	server, err := New(nil, nil, config.Config{PublicOrigin: "http://127.0.0.1:8080"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/resources/example", nil)
	request = request.WithContext(withState(request, requestState{
		hasSession: true,
		session: database.Session{
			Identity:  &database.Identity{ID: "account", DisplayName: "Alex", Role: database.RoleEngineer},
			CSRFToken: []byte("01234567890123456789012345678901"),
		},
	}))
	response := httptest.NewRecorder()
	server.render(response, request, "resource.html", http.StatusOK, PageData{
		Title: "Example schedule",
		Resource: database.Resource{
			ID: "resource", Code: "XSS-01", Name: "Test", Description: `<img src=x onerror="alert(1)">`, Active: true,
		},
		Date: "2026-01-02",
	})
	body := response.Body.String()
	if strings.Contains(body, `<img src=x`) || !strings.Contains(body, "&lt;img") {
		t.Fatalf("HTML-like resource description was not escaped: %s", body)
	}
	if !strings.Contains(body, "No bookings are scheduled for this date.") {
		t.Fatal("empty schedule state was not rendered")
	}
}

func TestStateChangingRequestWithoutCSRFIsRejected(t *testing.T) {
	nextCalled := false
	server := &Server{publicOrigin: "http://127.0.0.1:8080"}
	handler := server.middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		nextCalled = true
	}))
	request := httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader(""))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", "http://127.0.0.1:8080")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || nextCalled {
		t.Fatalf("missing CSRF should be rejected before the handler; status=%d handler=%v", response.Code, nextCalled)
	}
}

func TestUnexpectedOriginIsRejected(t *testing.T) {
	nextCalled := false
	server := &Server{publicOrigin: "http://127.0.0.1:8080"}
	handler := server.middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		nextCalled = true
	}))
	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("_csrf=missing"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", "https://attacker.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || nextCalled {
		t.Fatalf("unexpected Origin should be rejected; status=%d handler=%v", response.Code, nextCalled)
	}
}

func withState(request *http.Request, state requestState) context.Context {
	return context.WithValue(request.Context(), requestStateKey{}, state)
}
