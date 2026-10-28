package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
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

var csrfPattern = regexp.MustCompile(`name="_csrf" value="([^"]+)"`)

func TestFoundationHTTPAuthenticationAndResourceBrowsing(t *testing.T) {
	runtimePool := webTestPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	migrationPool := webTestPool(t, "LABRESERVE_TEST_DATABASE_URL")
	store := database.NewStore(runtimePool)
	authentication := auth.NewService(store, time.Now)
	application, err := New(store, authentication, config.Config{
		DatabaseURL: "postgres://test",
		ListenAddr:  "127.0.0.1:8080", PublicOrigin: "http://labreserve.test",
	}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(application.Handler())
	defer func() { server.Close() }()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	response := mustGet(t, client, server.URL+"/resources")
	if response.StatusCode != http.StatusSeeOther || response.Header.Get("Location") != "/login" {
		t.Fatalf("unauthenticated resource list: status=%d location=%q", response.StatusCode, response.Header.Get("Location"))
	}
	response.Body.Close()

	response = mustGet(t, client, server.URL+"/login")
	loginBody := readAndClose(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login page status = %d", response.StatusCode)
	}
	anonymousCookie := findCookie(t, response.Cookies(), "labreserve_session")
	if !anonymousCookie.HttpOnly || anonymousCookie.SameSite != http.SameSiteLaxMode || anonymousCookie.Path != "/" {
		t.Fatalf("unexpected session cookie protections: %#v", anonymousCookie)
	}
	csrf := csrfFrom(t, loginBody)

	alexPassword := requiredTestPassword(t, "SEED_ALEX_PASSWORD")
	failed := submitForm(t, client, server.URL+"/login", "http://labreserve.test", url.Values{
		"_csrf": {csrf}, "login": {"alex@example.test"}, "password": {"wrong-password"},
	})
	failedBody := readAndClose(t, failed)
	if failed.StatusCode != http.StatusUnauthorized || !strings.Contains(failedBody, "login or password was not recognized") {
		t.Fatalf("failed login response: status=%d body=%s", failed.StatusCode, failedBody)
	}
	if strings.Contains(failedBody, "wrong-password") {
		t.Fatal("failed login reflected the submitted password")
	}

	oldToken := anonymousCookie.Value
	success := submitForm(t, client, server.URL+"/login", "http://labreserve.test", url.Values{
		"_csrf": {csrf}, "login": {"alex@example.test"}, "password": {alexPassword},
		"role": {"coordinator"}, "user_id": {"jordan@example.test"},
	})
	success.Body.Close()
	if success.StatusCode != http.StatusSeeOther || success.Header.Get("Location") != "/resources" {
		t.Fatalf("successful login response: status=%d location=%q", success.StatusCode, success.Header.Get("Location"))
	}
	authenticatedCookie := findCookie(t, success.Cookies(), "labreserve_session")
	if authenticatedCookie.Value == oldToken {
		t.Fatal("login did not rotate the anonymous session token")
	}
	var rawTokenMatches int
	if err := migrationPool.QueryRow(context.Background(), "SELECT count(*) FROM sessions WHERE token_hash = $1", []byte(authenticatedCookie.Value)).Scan(&rawTokenMatches); err != nil {
		t.Fatal(err)
	}
	if rawTokenMatches != 0 {
		t.Fatal("raw session token was stored in PostgreSQL")
	}

	server.Close()
	restartedApplication, err := New(store, authentication, config.Config{
		DatabaseURL: "postgres://test",
		ListenAddr:  "127.0.0.1:8080", PublicOrigin: "http://labreserve.test",
	}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	server = httptest.NewServer(restartedApplication.Handler())
	restartedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(restartedURL, []*http.Cookie{authenticatedCookie})
	response = mustGet(t, client, server.URL+"/resources")
	restartedBody := readAndClose(t, response)
	if response.StatusCode != http.StatusOK || !strings.Contains(restartedBody, "Alex Morgan") {
		t.Fatalf("database-backed session did not survive application restart: status=%d", response.StatusCode)
	}

	response = mustGet(t, client, server.URL+"/resources?role=coordinator&user_id=jordan@example.test")
	resourceBody := readAndClose(t, response)
	if response.StatusCode != http.StatusOK || !strings.Contains(resourceBody, "NET-01") || !strings.Contains(resourceBody, "K8S-01") {
		t.Fatalf("resource list: status=%d body=%s", response.StatusCode, resourceBody)
	}
	if !strings.Contains(resourceBody, "Alex Morgan") || !strings.Contains(resourceBody, "(engineer)") || strings.Contains(resourceBody, "(coordinator)") {
		t.Fatal("identity or role was not derived from the authenticated account")
	}

	var resourceID string
	if err := runtimePool.QueryRow(context.Background(), `SELECT id::text FROM resources WHERE lower(code) = 'net-01'`).Scan(&resourceID); err != nil {
		t.Fatal(err)
	}
	response = mustGet(t, client, server.URL+"/resources/"+resourceID+"?date=2026-01-02")
	detailBody := readAndClose(t, response)
	if response.StatusCode != http.StatusOK || !strings.Contains(detailBody, "No bookings are scheduled for this date.") || !strings.Contains(detailBody, "2026-01-02 (Asia/Jakarta)") {
		t.Fatalf("resource schedule empty state: status=%d body=%s", response.StatusCode, detailBody)
	}

	maliciousDescription := `<img src=x onerror="alert(1)"><script>alert(2)</script>`
	var maliciousID string
	if err := migrationPool.QueryRow(context.Background(), `
		INSERT INTO resources (id, code, name, description, active)
		VALUES (gen_random_uuid(), $1, $2, $3, TRUE) RETURNING id::text`,
		"XSS-"+fmt.Sprint(time.Now().UnixNano()), `<script>bad-name</script>`, maliciousDescription).Scan(&maliciousID); err != nil {
		t.Fatalf("insert HTML-like resource fixture: %v", err)
	}
	defer func() {
		_, _ = migrationPool.Exec(context.Background(), "DELETE FROM resources WHERE id = $1", maliciousID)
	}()
	response = mustGet(t, client, server.URL+"/resources/"+maliciousID)
	xssBody := readAndClose(t, response)
	if response.StatusCode != http.StatusOK || strings.Contains(xssBody, `<img src=x`) || strings.Contains(xssBody, `<script>`) || !strings.Contains(xssBody, "&lt;script&gt;") {
		t.Fatalf("HTML-like resource data was not escaped: status=%d body=%s", response.StatusCode, xssBody)
	}

	logoutCSRF := csrfFrom(t, detailBody)
	missingCSRF := submitForm(t, client, server.URL+"/logout?_csrf="+url.QueryEscape(logoutCSRF), "http://labreserve.test", url.Values{})
	missingCSRF.Body.Close()
	if missingCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("CSRF token in query string was accepted: status=%d", missingCSRF.StatusCode)
	}
	wrongOrigin := submitForm(t, client, server.URL+"/logout", "https://attacker.example", url.Values{"_csrf": {logoutCSRF}})
	wrongOrigin.Body.Close()
	if wrongOrigin.StatusCode != http.StatusForbidden {
		t.Fatalf("unexpected Origin was accepted: status=%d", wrongOrigin.StatusCode)
	}
	logout := submitForm(t, client, server.URL+"/logout", "http://labreserve.test", url.Values{"_csrf": {logoutCSRF}})
	logout.Body.Close()
	if logout.StatusCode != http.StatusSeeOther || logout.Header.Get("Location") != "/login" {
		t.Fatalf("logout response: status=%d location=%q", logout.StatusCode, logout.Header.Get("Location"))
	}

	staleClient := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	staleRequest, err := http.NewRequest(http.MethodGet, server.URL+"/resources", nil)
	if err != nil {
		t.Fatal(err)
	}
	staleRequest.AddCookie(&http.Cookie{Name: "labreserve_session", Value: authenticatedCookie.Value})
	staleResponse, err := staleClient.Do(staleRequest)
	if err != nil {
		t.Fatal(err)
	}
	staleResponse.Body.Close()
	if staleResponse.StatusCode != http.StatusSeeOther || staleResponse.Header.Get("Location") != "/login" {
		t.Fatalf("revoked session retained access: status=%d location=%q", staleResponse.StatusCode, staleResponse.Header.Get("Location"))
	}

	coordinatorJar, _ := cookiejar.New(nil)
	coordinatorClient := &http.Client{Jar: coordinatorJar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	loginResponse := mustGet(t, coordinatorClient, server.URL+"/login")
	coordinatorCSRF := csrfFrom(t, readAndClose(t, loginResponse))
	coordinatorLogin := submitForm(t, coordinatorClient, server.URL+"/login", "http://labreserve.test", url.Values{
		"_csrf": {coordinatorCSRF}, "login": {"jordan@example.test"},
		"password": {requiredTestPassword(t, "SEED_JORDAN_PASSWORD")}, "role": {"engineer"},
	})
	coordinatorLogin.Body.Close()
	if coordinatorLogin.StatusCode != http.StatusSeeOther {
		t.Fatalf("coordinator sign-in status = %d", coordinatorLogin.StatusCode)
	}
	response = mustGet(t, coordinatorClient, server.URL+"/resources?role=engineer")
	coordinatorBody := readAndClose(t, response)
	if !strings.Contains(coordinatorBody, "Jordan Lee") || !strings.Contains(coordinatorBody, "(coordinator)") {
		t.Fatal("coordinator identity or role was not derived from the seeded account")
	}

	coordinatorURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	coordinatorCookie := findCookie(t, coordinatorJar.Cookies(coordinatorURL), sessionCookieName)
	coordinatorSession, found, err := authentication.Resolve(context.Background(), coordinatorCookie.Value)
	if err != nil || !found || coordinatorSession.Identity == nil || coordinatorSession.Identity.Role != database.RoleCoordinator {
		t.Fatalf("resolve trusted Coordinator session for Resource service: found=%v session=%+v error=%v", found, coordinatorSession, err)
	}
	resourceNow := time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC)
	resourceService := resources.NewService(store, func() time.Time { return resourceNow })
	resourceCode := fmt.Sprintf("SESSION-AUTH-%d", time.Now().UnixNano())
	created, err := resourceService.Create(context.Background(), resources.Actor{AccountID: coordinatorSession.Identity.ID}, resources.CreateRequest{
		Code: resourceCode, Name: "Resolved Coordinator Resource", Active: true,
	})
	if err != nil {
		t.Fatalf("create Resource from trusted session identity: %v", err)
	}
	var bookingRequestID string
	if err := migrationPool.QueryRow(context.Background(), "SELECT gen_random_uuid()::text").Scan(&bookingRequestID); err != nil {
		t.Fatalf("generate existing-flow Booking request identity: %v", err)
	}
	bookingRequest := bookings.CreateRequest{
		ResourceID: created.Resource.ID, RequestID: bookingRequestID,
		StartAt: resourceNow.Add(4 * time.Hour), EndAt: resourceNow.Add(5 * time.Hour), Purpose: "retained after Resource edit",
	}
	createdBooking, err := bookings.NewService(store, func() time.Time { return resourceNow }).Create(
		context.Background(), bookings.Actor{AccountID: coordinatorSession.Identity.ID}, bookingRequest)
	if err != nil || createdBooking.Replayed {
		t.Fatalf("create Booking through existing service for Resource read-path checks: result=%+v error=%v", createdBooking, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := migrationPool.Exec(cleanupCtx, "DELETE FROM activity_events WHERE booking_id = $1::uuid", createdBooking.Booking.ID); err != nil {
			t.Errorf("delete service-created Resource Booking events: %v", err)
		}
		if _, err := migrationPool.Exec(cleanupCtx, "DELETE FROM bookings WHERE id = $1::uuid", createdBooking.Booking.ID); err != nil {
			t.Errorf("delete service-created Resource Booking: %v", err)
		}
		if _, err := migrationPool.Exec(cleanupCtx, "DELETE FROM activity_events WHERE resource_id = $1::uuid", created.Resource.ID); err != nil {
			t.Errorf("delete trusted-session Resource event: %v", err)
		}
		if _, err := migrationPool.Exec(cleanupCtx, "DELETE FROM resources WHERE id = $1::uuid", created.Resource.ID); err != nil {
			t.Errorf("delete trusted-session Resource: %v", err)
		}
	})
	var eventActor string
	if err := migrationPool.QueryRow(context.Background(), `
		SELECT actor_account_id::text FROM activity_events WHERE resource_id = $1::uuid AND action = 'resource.created'`,
		created.Resource.ID).Scan(&eventActor); err != nil || eventActor != coordinatorSession.Identity.ID {
		t.Fatalf("Resource creation Activity actor=%q error=%v, want authenticated session identity %q", eventActor, err, coordinatorSession.Identity.ID)
	}

	resourceNow = resourceNow.Add(time.Minute)
	const htmlName = `<script>service-resource-name</script>`
	const htmlDescription = `<img src=x onerror="alert(2)">`
	edited, err := resourceService.Edit(context.Background(), resources.Actor{AccountID: coordinatorSession.Identity.ID}, resources.EditRequest{
		ResourceID: created.Resource.ID, Code: created.Resource.Code, Name: htmlName, Description: htmlDescription,
	})
	if err != nil || !edited.Changed {
		t.Fatalf("edit Resource from trusted Coordinator session identity: result=%+v error=%v", edited, err)
	}
	stored, found, err := store.ResourceByID(context.Background(), created.Resource.ID)
	if err != nil || !found || stored.Name != htmlName || stored.Description != htmlDescription {
		t.Fatalf("service did not retain exact HTML-like plain text: found=%v resource=%+v error=%v", found, stored, err)
	}
	response = mustGet(t, coordinatorClient, server.URL+"/resources/"+created.Resource.ID+"?date=2040-01-02")
	serviceXSSBody := readAndClose(t, response)
	if response.StatusCode != http.StatusOK || !strings.Contains(serviceXSSBody, createdBooking.Booking.ID) ||
		strings.Contains(serviceXSSBody, htmlName) || strings.Contains(serviceXSSBody, htmlDescription) ||
		!strings.Contains(serviceXSSBody, "&lt;script&gt;service-resource-name&lt;/script&gt;") ||
		!strings.Contains(serviceXSSBody, "&lt;img src=x onerror=&#34;alert(2)&#34;&gt;") {
		t.Fatalf("service-created HTML-like Resource text or retained Booking was not safely displayed: status=%d body=%s", response.StatusCode, serviceXSSBody)
	}
	response = mustGet(t, coordinatorClient, server.URL+"/resources")
	serviceListBody := readAndClose(t, response)
	if response.StatusCode != http.StatusOK || !strings.Contains(serviceListBody, resourceCode) ||
		!strings.Contains(serviceListBody, "&lt;script&gt;service-resource-name&lt;/script&gt;") ||
		!strings.Contains(serviceListBody, "&lt;img src=x onerror=&#34;alert(2)&#34;&gt;") {
		t.Fatalf("service-created Resource was not safely retained in the existing list: status=%d body=%s", response.StatusCode, serviceListBody)
	}
	response = mustGet(t, coordinatorClient, server.URL+"/bookings/"+createdBooking.Booking.ID)
	serviceBookingBody := readAndClose(t, response)
	if response.StatusCode != http.StatusOK || !strings.Contains(serviceBookingBody, "&lt;script&gt;service-resource-name&lt;/script&gt;") {
		t.Fatalf("Booking detail did not show updated Resource context after edit: status=%d body=%s", response.StatusCode, serviceBookingBody)
	}
	response = mustGet(t, coordinatorClient, server.URL+"/my-bookings")
	serviceMyBookingsBody := readAndClose(t, response)
	if response.StatusCode != http.StatusOK || !strings.Contains(serviceMyBookingsBody, createdBooking.Booking.ID) ||
		!strings.Contains(serviceMyBookingsBody, "&lt;script&gt;service-resource-name&lt;/script&gt;") {
		t.Fatalf("My Bookings did not retain updated Resource context after edit: status=%d body=%s", response.StatusCode, serviceMyBookingsBody)
	}

	revokedSession, revokedFound, err := authentication.Resolve(context.Background(), authenticatedCookie.Value)
	if err != nil || revokedFound || revokedSession.Identity != nil {
		t.Fatalf("revoked Engineer session still resolves: found=%v session=%+v error=%v", revokedFound, revokedSession, err)
	}
	staleCode := resourceCode + "-STALE"
	if _, err := resourceService.Create(context.Background(), resources.Actor{}, resources.CreateRequest{
		Code: staleCode, Name: "Revoked session attempt",
	}); !errors.Is(err, resources.ErrUnauthenticated) {
		t.Fatalf("missing actor after revoked session = %v, want unauthenticated", err)
	}
	var staleResources int
	if err := migrationPool.QueryRow(context.Background(), `
		SELECT count(*) FROM resources WHERE code = $1`, staleCode).Scan(&staleResources); err != nil || staleResources != 0 {
		t.Fatalf("revoked session attempt persisted %d Resources, error=%v", staleResources, err)
	}
}

func webTestPool(t *testing.T, environment string) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv(environment)
	if databaseURL == "" {
		t.Skipf("%s is not configured; use make verify for real PostgreSQL verification", environment)
	}
	parsed, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse %s: %v", environment, err)
	}
	if parsed.ConnConfig.Database != "labreserve_test" {
		t.Fatalf("refusing integration test database %q; expected labreserve_test", parsed.ConnConfig.Database)
	}
	pool, err := database.Open(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("open %s: %v", environment, err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func requiredTestPassword(t *testing.T, name string) string {
	t.Helper()
	password := os.Getenv(name)
	if password == "" {
		t.Fatalf("%s must be set by the verification environment", name)
	}
	return password
}

func mustGet(t *testing.T, client *http.Client, target string) *http.Response {
	t.Helper()
	response, err := client.Get(target)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func submitForm(t *testing.T, client *http.Client, target, origin string, values url.Values) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, target, strings.NewReader(values.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", origin)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func readAndClose(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func csrfFrom(t *testing.T, body string) string {
	t.Helper()
	match := csrfPattern.FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("page does not contain a synchronizer CSRF token: %s", body)
	}
	return match[1]
}

func findCookie(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("response has no %s cookie", name)
	return nil
}
