package web

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
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
	inactiveTransitionResourceID := insertResourceStateSessionResource(t, migrationPool,
		"Session-Inactive-"+strings.ReplaceAll(webTestUUID(t, migrationPool), "-", ""),
		"Inactive Transition Lab", "Inactive state fixture", false, now)
	eligibleDeactivationResourceID := insertResourceStateSessionResource(t, migrationPool,
		"Session-Eligible-"+strings.ReplaceAll(webTestUUID(t, migrationPool), "-", ""),
		"Eligible Deactivation Lab", "Retained cancelled Booking fixture", true, now)
	coordinatorControlResourceID := insertResourceStateSessionResource(t, migrationPool,
		"Session-Control-"+strings.ReplaceAll(webTestUUID(t, migrationPool), "-", ""),
		"Coordinator Control Lab", "Eligible control fixture", true, now)
	bookingID := ""
	eligibleCancelledBookingID := ""
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, id := range []string{bookingID, eligibleCancelledBookingID} {
			if id == "" {
				continue
			}
			if _, err := migrationPool.Exec(cleanupCtx, "DELETE FROM public.activity_events WHERE booking_id = $1::uuid", id); err != nil {
				t.Errorf("delete session test Booking Activity: %v", err)
			}
			if _, err := migrationPool.Exec(cleanupCtx, "DELETE FROM public.bookings WHERE id = $1::uuid", id); err != nil {
				t.Errorf("delete session test Booking: %v", err)
			}
		}
		for _, id := range []string{resourceID, roleChangeResourceID, inactiveTransitionResourceID,
			eligibleDeactivationResourceID, coordinatorControlResourceID} {
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

	eligibleBooking, err := bookings.NewService(store, clock).Create(t.Context(), bookings.Actor{AccountID: samSession.Identity.ID}, bookings.CreateRequest{
		ResourceID: eligibleDeactivationResourceID, RequestID: webTestUUID(t, migrationPool),
		StartAt: now.Add(4 * time.Hour), EndAt: now.Add(5 * time.Hour), Purpose: "retained future cancelled eligibility fixture",
	})
	if err != nil || eligibleBooking.Replayed {
		t.Fatalf("create retained Booking for eligible deactivation fixture: result=%+v error=%v", eligibleBooking, err)
	}
	eligibleCancelledBookingID = eligibleBooking.Booking.ID
	cancelled, err := bookings.NewCancellationService(store, clock).Cancel(t.Context(), bookings.Actor{AccountID: samSession.Identity.ID}, eligibleCancelledBookingID)
	if err != nil || !cancelled.Changed || cancelled.Booking.State != bookings.StateCancelled {
		t.Fatalf("retain future Cancelled Booking for eligible deactivation fixture: result=%+v error=%v", cancelled, err)
	}
	assertResourceStateSessionDeactivationEligibility(t, migrationPool, eligibleDeactivationResourceID, now, true)

	assertResourceStateSessionDeactivationEligibility(t, migrationPool, coordinatorControlResourceID, now, false)
	controlBefore := resourceStateSessionPersistedSnapshot(t, migrationPool, coordinatorControlResourceID)
	controlResult, err := setResourceStateFromLiveSession(t.Context(), authentication, coordinatorToken, stateService,
		resources.StateRequest{ResourceID: coordinatorControlResourceID, Active: false})
	if err != nil || !controlResult.Changed || controlResult.Resource.Active || !controlResult.Resource.UpdatedAt.Equal(now) {
		t.Fatalf("eligible live Coordinator deactivation positive control: result=%+v error=%v", controlResult, err)
	}
	controlAfter := resourceStateSessionPersistedSnapshot(t, migrationPool, coordinatorControlResourceID)
	if controlBefore[0] == controlAfter[0] || controlBefore[1] == controlAfter[1] {
		t.Fatalf("eligible Coordinator positive control did not persist its Resource/Activity transition: before=%v after=%v",
			controlBefore, controlAfter)
	}

	for _, engineer := range []struct {
		name  string
		token string
	}{
		{name: "Alex", token: alexToken},
		{name: "Sam", token: samToken},
	} {
		resolvedEngineer, found, err := authentication.Resolve(t.Context(), engineer.token)
		if err != nil || !found || resolvedEngineer.Identity == nil || resolvedEngineer.AccountID == nil ||
			resolvedEngineer.Identity.ID != *resolvedEngineer.AccountID || resolvedEngineer.Identity.Role != database.RoleEngineer {
			t.Fatalf("resolve stored Engineer identity for %s: found=%v session=%+v error=%v", engineer.name, found, resolvedEngineer, err)
		}
		for _, request := range []struct {
			name       string
			resourceID string
			active     bool
			eligible   bool
		}{
			{name: "Active to Inactive", resourceID: resourceID, active: false},
			{name: "Eligible Active to Inactive", resourceID: eligibleDeactivationResourceID, active: false, eligible: true},
			{name: "Inactive to Active", resourceID: inactiveTransitionResourceID, active: true},
			{name: "Active to Active", resourceID: resourceID, active: true},
			{name: "Inactive to Inactive", resourceID: inactiveTransitionResourceID, active: false},
		} {
			t.Run(engineer.name+"/"+request.name, func(t *testing.T) {
				if request.eligible {
					assertResourceStateSessionDeactivationEligibility(t, migrationPool, request.resourceID, now, true)
				}
				before := resourceStateSessionPersistedSnapshot(t, migrationPool, request.resourceID)
				beforeCalls := stateService.calls
				result, err := setResourceStateFromLiveSession(t.Context(), authentication, engineer.token, stateService,
					resources.StateRequest{ResourceID: request.resourceID, Active: request.active})
				if err != resources.ErrForbidden || result != (resources.StateResult{}) || stateService.calls != beforeCalls+1 {
					t.Fatalf("stored Engineer session state request result=%+v calls=%d->%d error=%v, want exact forbidden outcome",
						result, beforeCalls, stateService.calls, err)
				}
				if after := resourceStateSessionPersistedSnapshot(t, migrationPool, request.resourceID); after != before {
					t.Fatalf("forbidden Engineer session request changed Resource/Booking/Activity state: before=%v after=%v", before, after)
				}
			})
		}
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

func resourceStateSessionPersistedSnapshot(t *testing.T, pool *pgxpool.Pool, resourceID string) [3]string {
	t.Helper()
	var snapshot [3]string
	if err := pool.QueryRow(t.Context(), `
		SELECT to_jsonb(resource)::text,
		       COALESCE((
		           SELECT jsonb_agg(to_jsonb(event) ORDER BY event.id)::text
		           FROM public.activity_events AS event WHERE event.resource_id = resource.id
		       ), '[]'),
		       COALESCE((
		           SELECT jsonb_agg(
		               jsonb_build_object(
		                   'booking', to_jsonb(booking),
		                   'activity', COALESCE((
		                       SELECT jsonb_agg(to_jsonb(booking_event) ORDER BY booking_event.id)
		                       FROM public.activity_events AS booking_event WHERE booking_event.booking_id = booking.id
		                   ), '[]'::jsonb)
		               ) ORDER BY booking.id
		           )::text
		           FROM public.bookings AS booking WHERE booking.resource_id = resource.id
		       ), '[]')
		FROM public.resources AS resource
		WHERE resource.id = $1::uuid`, resourceID).Scan(&snapshot[0], &snapshot[1], &snapshot[2]); err != nil {
		t.Fatalf("snapshot session-test Resource, Bookings, and Activity: %v", err)
	}
	return snapshot
}

func assertResourceStateSessionDeactivationEligibility(
	t *testing.T,
	pool *pgxpool.Pool,
	resourceID string,
	operationAt time.Time,
	requireFutureCancelled bool,
) {
	t.Helper()
	var active bool
	var blocking, futureCancelled int64
	if err := pool.QueryRow(t.Context(), `
		SELECT resource.active,
		       count(booking.id) FILTER (WHERE booking.state = 'confirmed' AND booking.end_at > $2),
		       count(booking.id) FILTER (WHERE booking.state = 'cancelled' AND booking.end_at > $2)
		FROM public.resources AS resource
		LEFT JOIN public.bookings AS booking ON booking.resource_id = resource.id
		WHERE resource.id = $1::uuid
		GROUP BY resource.active`, resourceID, operationAt).Scan(&active, &blocking, &futureCancelled); err != nil {
		t.Fatalf("verify live-session deactivation eligibility for Resource %s: %v", resourceID, err)
	}
	if !active || blocking != 0 || (requireFutureCancelled && futureCancelled == 0) {
		t.Fatalf("live-session fixture is not eligible at %s: active=%v future Confirmed blockers=%d future Cancelled=%d require future Cancelled=%v",
			operationAt, active, blocking, futureCancelled, requireFutureCancelled)
	}
}

func TestResourceStateSessionSurvivesPostRestart(t *testing.T) {
	if os.Getenv("LABRESERVE_TEST_RESOURCE_STATE_RESTART_REPLAY") != "1" {
		t.Skip("run by scripts/verify.sh after PostgreSQL and application restart")
	}

	runtimePool := webTestPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	store := database.NewStore(runtimePool)
	now := time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC)
	authentication := auth.NewService(store, func() time.Time { return now })
	token := strings.Repeat("A", base64.RawURLEncoding.EncodedLen(32))
	session, found, err := authentication.Resolve(t.Context(), token)
	if err != nil || !found || session.Identity == nil || session.AccountID == nil ||
		session.Identity.ID != *session.AccountID || session.Identity.Login != "jordan@example.test" ||
		session.Identity.Role != database.RoleCoordinator {
		t.Fatalf("seeded Coordinator session did not survive restart: found=%v identity=%+v error=%v", found, session.Identity, err)
	}

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://verify-app:8080/resources", nil)
	if err != nil {
		t.Fatalf("create request to restarted verification application: %v", err)
	}
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("request existing resource list from restarted application: %v", err)
	}
	body := readAndClose(t, response)
	if response.StatusCode != http.StatusOK || !strings.Contains(body, "DEMO-01 — Customer Demo Environment") ||
		!strings.Contains(body, "Jordan Lee") {
		t.Fatalf("restarted app did not accept retained session/read seeded Resource list: status=%d body=%s", response.StatusCode, body)
	}

	var demoResourceID string
	if err := runtimePool.QueryRow(t.Context(), `SELECT id::text FROM public.resources WHERE code = 'DEMO-01'`).Scan(&demoResourceID); err != nil {
		t.Fatalf("resolve seeded DEMO-01 Resource for its detail view: %v", err)
	}
	detailRequest, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
		"http://verify-app:8080/resources/"+demoResourceID, nil)
	if err != nil {
		t.Fatalf("create request to restarted DEMO-01 detail view: %v", err)
	}
	detailRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	detailResponse, err := client.Do(detailRequest)
	if err != nil {
		t.Fatalf("request DEMO-01 detail from restarted verification application: %v", err)
	}
	detailBody := readAndClose(t, detailResponse)
	if detailResponse.StatusCode != http.StatusOK ||
		!strings.Contains(detailBody, "<h1>DEMO-01 — Customer Demo Environment</h1>") ||
		!strings.Contains(detailBody, `<p class="state inactive">Inactive</p>`) {
		t.Fatalf("restarted DEMO-01 detail did not bind its persisted inactive state to that Resource: status=%d body=%s",
			detailResponse.StatusCode, detailBody)
	}
}
