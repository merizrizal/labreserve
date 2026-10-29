package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"labreserve.local/labreserve/internal/auth"
	"labreserve.local/labreserve/internal/bookings"
	"labreserve.local/labreserve/internal/config"
	"labreserve.local/labreserve/internal/database"
	"labreserve.local/labreserve/internal/resources"
)

type resourceStateSessionService interface {
	SetState(context.Context, resources.Actor, resources.StateRequest) (resources.StateResult, error)
}

type countedResourceStateSessionService struct {
	service resourceStateSessionService
	calls   int
}

func (s *countedResourceStateSessionService) SetState(ctx context.Context, actor resources.Actor, request resources.StateRequest) (resources.StateResult, error) {
	s.calls++
	return s.service.SetState(ctx, actor, request)
}

func setResourceStateFromLiveSession(
	ctx context.Context,
	authentication *auth.Service,
	token string,
	service resourceStateSessionService,
	request resources.StateRequest,
) (resources.StateResult, error) {
	session, found, err := authentication.Resolve(ctx, token)
	if err != nil {
		return resources.StateResult{}, err
	}
	if !found || session.Identity == nil || session.AccountID == nil || session.Identity.ID != *session.AccountID {
		return resources.StateResult{}, resources.ErrUnauthenticated
	}
	return service.SetState(ctx, resources.Actor{AccountID: session.Identity.ID}, request)
}

func TestResourceStateSessionUsesResolvedLiveIdentityAndPreservesReads(t *testing.T) {
	runtimePool := webTestPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	migrationPool := webTestPool(t, "LABRESERVE_TEST_DATABASE_URL")
	if err := database.Migrate(t.Context(), migrationPool); err != nil {
		t.Fatalf("apply Resource state session test migrations: %v", err)
	}

	now := time.Date(2040, time.January, 2, 3, 4, 5, 123456000, time.UTC)
	clock := func() time.Time { return now }
	store := database.NewStore(runtimePool)
	authentication := auth.NewService(store, clock)
	application, err := New(store, authentication, config.Config{
		DatabaseURL: "postgres://test", ListenAddr: "127.0.0.1:8080", PublicOrigin: "http://labreserve.test",
	}, clock)
	if err != nil {
		t.Fatalf("create session test application: %v", err)
	}
	server := httptest.NewServer(application.Handler())
	defer server.Close()

	resourceID := insertResourceStateSessionResource(t, migrationPool,
		"Session-State-"+strings.ReplaceAll(webTestUUID(t, migrationPool), "-", ""),
		`<script>legacy-resource-name</script>`, `<img src=x onerror="alert(1)">`, false, now)
	roleChangeResourceID := insertResourceStateSessionResource(t, migrationPool,
		"Session-Role-"+strings.ReplaceAll(webTestUUID(t, migrationPool), "-", ""),
		"Role Change Lab", "Role fixture", true, now)
	bookingID := ""
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if bookingID != "" {
			if _, err := migrationPool.Exec(cleanupCtx, "DELETE FROM public.activity_events WHERE booking_id = $1::uuid", bookingID); err != nil {
				t.Errorf("delete session test Booking Activity: %v", err)
			}
			if _, err := migrationPool.Exec(cleanupCtx, "DELETE FROM public.bookings WHERE id = $1::uuid", bookingID); err != nil {
				t.Errorf("delete session test Booking: %v", err)
			}
		}
		for _, id := range []string{resourceID, roleChangeResourceID} {
			if _, err := migrationPool.Exec(cleanupCtx, "DELETE FROM public.activity_events WHERE resource_id = $1::uuid", id); err != nil {
				t.Errorf("delete session test Resource Activity: %v", err)
			}
			if _, err := migrationPool.Exec(cleanupCtx, "DELETE FROM public.resources WHERE id = $1::uuid", id); err != nil {
				t.Errorf("delete session test Resource: %v", err)
			}
		}
	})

	sessionTokens := make([]string, 0, 3)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, token := range sessionTokens {
			if err := authentication.Logout(cleanupCtx, token); err != nil {
				t.Errorf("delete state session test login: %v", err)
			}
		}
	})
	_, coordinatorToken := loginResourceStateSession(t, server.URL, "jordan@example.test",
		requiredTestPassword(t, "SEED_JORDAN_PASSWORD"), "engineer", "alex@example.test")
	sessionTokens = append(sessionTokens, coordinatorToken)
	_, alexToken := loginResourceStateSession(t, server.URL, "alex@example.test",
		requiredTestPassword(t, "SEED_ALEX_PASSWORD"), "coordinator", "jordan@example.test")
	sessionTokens = append(sessionTokens, alexToken)
	samClient, samToken := loginResourceStateSession(t, server.URL, "sam@example.test",
		requiredTestPassword(t, "SEED_SAM_PASSWORD"), "coordinator", "jordan@example.test")
	sessionTokens = append(sessionTokens, samToken)

	coordinatorSession, found, err := authentication.Resolve(t.Context(), coordinatorToken)
	if err != nil || !found || coordinatorSession.Identity == nil || coordinatorSession.Identity.Role != database.RoleCoordinator {
		t.Fatalf("resolve live Coordinator after tampered role fields: found=%v session=%+v error=%v", found, coordinatorSession, err)
	}
	stateService := &countedResourceStateSessionService{service: resources.NewStateService(store, clock)}
	activated, err := setResourceStateFromLiveSession(t.Context(), authentication, coordinatorToken, stateService,
		resources.StateRequest{ResourceID: resourceID, Active: true})
	if err != nil || !activated.Changed || !activated.Resource.Active || stateService.calls != 1 {
		t.Fatalf("live Coordinator session did not reach authorized state service: result=%+v calls=%d error=%v", activated, stateService.calls, err)
	}

	samSession, found, err := authentication.Resolve(t.Context(), samToken)
	if err != nil || !found || samSession.Identity == nil || samSession.Identity.Role != database.RoleEngineer {
		t.Fatalf("resolve live Engineer session after tampered Coordinator claim: found=%v session=%+v error=%v", found, samSession, err)
	}
	requestID := webTestUUID(t, migrationPool)
	createdBooking, err := bookings.NewService(store, clock).Create(t.Context(), bookings.Actor{AccountID: samSession.Identity.ID}, bookings.CreateRequest{
		ResourceID: resourceID, RequestID: requestID,
		StartAt: now.Add(2 * time.Hour), EndAt: now.Add(3 * time.Hour), Purpose: `<script>legacy-booking-purpose</script>`,
	})
	if err != nil || createdBooking.Replayed {
		t.Fatalf("create retained Booking for existing read paths: result=%+v error=%v", createdBooking, err)
	}
	bookingID = createdBooking.Booking.ID

	for _, engineer := range []struct {
		name  string
		token string
	}{
		{name: "Alex", token: alexToken},
		{name: "Sam", token: samToken},
	} {
		t.Run(engineer.name+" cannot use a claimed Coordinator role", func(t *testing.T) {
			beforeCalls := stateService.calls
			result, err := setResourceStateFromLiveSession(t.Context(), authentication, engineer.token, stateService,
				resources.StateRequest{ResourceID: resourceID, Active: true})
			if !errors.Is(err, resources.ErrForbidden) || result.Changed || stateService.calls != beforeCalls+1 {
				t.Fatalf("tampered role changed authority: result=%+v calls=%d->%d error=%v", result, beforeCalls, stateService.calls, err)
			}
		})
	}

	for _, read := range []struct {
		name string
		path string
	}{
		{name: "resource list", path: "/resources"},
		{name: "schedule", path: "/resources/" + resourceID + "?date=2040-01-02"},
		{name: "booking detail", path: "/bookings/" + bookingID},
		{name: "My Bookings", path: "/my-bookings"},
	} {
		t.Run(read.name+" retains escaped legacy text", func(t *testing.T) {
			response := mustGet(t, samClient, server.URL+read.path)
			body := readAndClose(t, response)
			if response.StatusCode != http.StatusOK {
				t.Fatalf("existing read path status=%d body=%s", response.StatusCode, body)
			}
			if strings.Contains(body, `<script>legacy-resource-name</script>`) || strings.Contains(body, `<img src=x onerror="alert(1)">`) ||
				strings.Contains(body, `<script>legacy-booking-purpose</script>`) {
				t.Fatalf("existing read path rendered legacy HTML-like content as markup: %s", body)
			}
			if !strings.Contains(body, "&lt;script&gt;legacy-resource-name&lt;/script&gt;") {
				t.Fatalf("existing read path did not preserve escaped legacy Resource name: %s", body)
			}
			if (read.name == "resource list" || read.name == "schedule") && !strings.Contains(body, "&lt;img src=x onerror=&#34;alert(1)&#34;&gt;") {
				t.Fatalf("existing Resource read path did not preserve escaped legacy description: %s", body)
			}
			if read.name != "resource list" && !strings.Contains(body, "&lt;script&gt;legacy-booking-purpose&lt;/script&gt;") {
				t.Fatalf("existing Booking read path did not preserve escaped purpose text: %s", body)
			}
			if (read.name == "schedule" || read.name == "booking detail" || read.name == "My Bookings") && !strings.Contains(body, bookingID) {
				t.Fatalf("existing Booking read path omitted retained Booking %s: %s", bookingID, body)
			}
		})
	}

	alexSession, found, err := authentication.Resolve(t.Context(), alexToken)
	if err != nil || !found || alexSession.Identity == nil || alexSession.Identity.Role != database.RoleEngineer {
		t.Fatalf("resolve Engineer before stored-role change: found=%v session=%+v error=%v", found, alexSession, err)
	}
	originalRole := string(alexSession.Identity.Role)
	if originalRole != string(database.RoleEngineer) {
		t.Fatalf("Alex stored role = %q, want Engineer before fixture change", originalRole)
	}
	roleFixtureNeedsRestore := true
	t.Cleanup(func() {
		if !roleFixtureNeedsRestore {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := migrationPool.Exec(cleanupCtx, "UPDATE public.accounts SET role = $1 WHERE id = $2::uuid", originalRole, alexSession.Identity.ID); err != nil {
			t.Errorf("restore Alex's stored role after session test: %v", err)
		}
	})
	if _, err := migrationPool.Exec(t.Context(), "UPDATE public.accounts SET role = $1 WHERE id = $2::uuid", string(database.RoleCoordinator), alexSession.Identity.ID); err != nil {
		t.Fatalf("change current stored role for session test: %v", err)
	}
	updated, found, err := authentication.Resolve(t.Context(), alexToken)
	if err != nil || !found || updated.Identity == nil || updated.Identity.Role != database.RoleCoordinator {
		t.Fatalf("Resolve did not reflect current stored Coordinator role: found=%v session=%+v error=%v", found, updated, err)
	}
	deactivated, err := setResourceStateFromLiveSession(t.Context(), authentication, alexToken, stateService,
		resources.StateRequest{ResourceID: roleChangeResourceID, Active: false})
	if err != nil || !deactivated.Changed || deactivated.Resource.Active {
		t.Fatalf("current stored Coordinator role did not authorize state service: result=%+v error=%v", deactivated, err)
	}
	if _, err := migrationPool.Exec(t.Context(), "UPDATE public.accounts SET role = $1 WHERE id = $2::uuid", originalRole, alexSession.Identity.ID); err != nil {
		t.Fatalf("restore Alex's stored Engineer role: %v", err)
	}
	roleFixtureNeedsRestore = false
	reverted, found, err := authentication.Resolve(t.Context(), alexToken)
	if err != nil || !found || reverted.Identity == nil || reverted.Identity.Role != database.RoleEngineer {
		t.Fatalf("Resolve did not reflect restored stored Engineer role: found=%v session=%+v error=%v", found, reverted, err)
	}
	beforeCalls := stateService.calls
	if _, err := setResourceStateFromLiveSession(t.Context(), authentication, alexToken, stateService,
		resources.StateRequest{ResourceID: roleChangeResourceID, Active: true}); !errors.Is(err, resources.ErrForbidden) || stateService.calls != beforeCalls+1 {
		t.Fatalf("restored Engineer role retained Coordinator authority: calls=%d->%d error=%v", beforeCalls, stateService.calls, err)
	}

	unknownCalls := stateService.calls
	if _, err := setResourceStateFromLiveSession(t.Context(), authentication, "invalid-session-token", stateService,
		resources.StateRequest{ResourceID: resourceID, Active: false}); !errors.Is(err, resources.ErrUnauthenticated) || stateService.calls != unknownCalls {
		t.Fatalf("unknown session reached state service: calls=%d->%d error=%v", unknownCalls, stateService.calls, err)
	}
	cachedCoordinatorActor := resources.Actor{AccountID: coordinatorSession.Identity.ID}
	if cachedCoordinatorActor.AccountID == "" {
		t.Fatal("live Coordinator session did not provide the retained identity fixture")
	}
	if err := authentication.Logout(t.Context(), coordinatorToken); err != nil {
		t.Fatalf("logout live Coordinator session: %v", err)
	}
	freshSession, found, err := authentication.Resolve(t.Context(), coordinatorToken)
	if err != nil || found {
		t.Fatalf("Resolve after Coordinator logout = session=%+v found=%v error=%v, want no live session", freshSession, found, err)
	}
	logoutCalls := stateService.calls
	if _, err := setResourceStateFromLiveSession(t.Context(), authentication, coordinatorToken, stateService,
		resources.StateRequest{ResourceID: resourceID, Active: false}); !errors.Is(err, resources.ErrUnauthenticated) || stateService.calls != logoutCalls {
		t.Fatalf("revoked live session reached state service using retained identity %q: calls=%d->%d error=%v",
			cachedCoordinatorActor.AccountID, logoutCalls, stateService.calls, err)
	}
	retained, found, err := store.ResourceByID(t.Context(), resourceID)
	if err != nil || !found || !retained.Active {
		t.Fatalf("logout or rejected stale session changed Resource state: found=%v resource=%+v error=%v", found, retained, err)
	}
}

func loginResourceStateSession(t *testing.T, serverURL, login, password, claimedRole, claimedUserID string) (*http.Client, string) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create session test cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	loginPage := mustGet(t, client, serverURL+"/login")
	loginBody := readAndClose(t, loginPage)
	if loginPage.StatusCode != http.StatusOK {
		t.Fatalf("login page status=%d body=%s", loginPage.StatusCode, loginBody)
	}
	anonymousCookie := findCookie(t, loginPage.Cookies(), sessionCookieName)
	response := submitForm(t, client, serverURL+"/login", "http://labreserve.test", url.Values{
		"_csrf": {csrfFrom(t, loginBody)}, "login": {login}, "password": {password},
		"role": {claimedRole}, "user_id": {claimedUserID},
	})
	body := readAndClose(t, response)
	if response.StatusCode != http.StatusSeeOther || response.Header.Get("Location") != "/resources" {
		t.Fatalf("session login status=%d location=%q body=%s", response.StatusCode, response.Header.Get("Location"), body)
	}
	authenticatedCookie := findCookie(t, response.Cookies(), sessionCookieName)
	if authenticatedCookie.Value == anonymousCookie.Value {
		t.Fatal("login did not rotate the anonymous session")
	}
	return client, authenticatedCookie.Value
}

func insertResourceStateSessionResource(
	t *testing.T,
	migrationPool *pgxpool.Pool,
	code, name, description string,
	active bool,
	createdAt time.Time,
) string {
	t.Helper()
	var id string
	if err := migrationPool.QueryRow(t.Context(), `
		INSERT INTO public.resources (id, code, name, description, active, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $5)
		RETURNING id::text`, code, name, description, active, createdAt).Scan(&id); err != nil {
		t.Fatalf("create state session Resource fixture: %v", err)
	}
	return id
}

func webTestUUID(t *testing.T, migrationPool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := migrationPool.QueryRow(t.Context(), "SELECT gen_random_uuid()::text").Scan(&id); err != nil {
		t.Fatalf("generate Resource state session fixture UUID: %v", err)
	}
	return id
}
