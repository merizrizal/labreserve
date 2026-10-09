const { test, expect } = require("@playwright/test");

const { Pool } = require("pg");
let verificationDatabase;

function getVerificationPool() {
  const connectionString = process.env.LABRESERVE_E2E_DATABASE_URL;
  if (!connectionString) throw new Error("LABRESERVE_E2E_DATABASE_URL must be supplied to browser verification");
  if (!verificationDatabase) verificationDatabase = new Pool({ connectionString, max: 1 });
  return verificationDatabase;
}

test.afterAll(async () => {
  if (verificationDatabase) await verificationDatabase.end();
});

async function expectRequestCounts(login, requestID, expectedBookings, expectedEvents) {
  const result = await getVerificationPool().query(
    `SELECT
       (SELECT count(*)::int FROM bookings b JOIN accounts a ON a.id = b.owner_account_id
        WHERE a.login = $1 AND b.request_id = $2) AS booking_count,
       (SELECT count(*)::int FROM activity_events e
        JOIN bookings b ON b.id = e.booking_id
        JOIN accounts a ON a.id = b.owner_account_id
        WHERE a.login = $1 AND b.request_id = $2 AND e.action = 'booking.created'
          AND e.actor_account_id = b.owner_account_id) AS event_count`,
    [login, requestID],
  );
  expect(result.rows[0].booking_count).toBe(expectedBookings);
  expect(result.rows[0].event_count).toBe(expectedEvents);
}

async function expectIntervalCounts(resourceID, start, end, expectedBookings, expectedEvents) {
  const result = await getVerificationPool().query(
    `SELECT count(*)::int AS booking_count,
       (SELECT count(*)::int FROM activity_events e JOIN bookings eb ON eb.id = e.booking_id
        WHERE eb.resource_id = $1 AND eb.start_at = $2::timestamptz
          AND eb.end_at = $3::timestamptz AND e.action = 'booking.created') AS event_count
     FROM bookings b
     WHERE b.resource_id = $1 AND b.start_at = $2::timestamptz
       AND b.end_at = $3::timestamptz AND b.state = 'confirmed'`,
    [resourceID, expectedUTCFromJakarta(start), expectedUTCFromJakarta(end)],
  );
  expect(result.rows[0].booking_count).toBe(expectedBookings);
  expect(result.rows[0].event_count).toBe(expectedEvents);
}

function requiredPassword(name) {
  const value = process.env[name];
  if (!value) throw new Error(`${name} must be supplied to the browser verification`);
  return value;
}

async function signIn(page, login = "alex@example.test", passwordEnvironment = "SEED_ALEX_PASSWORD") {
  await page.goto("/login");
  await page.getByLabel("Email").fill(login);
  await page.getByLabel("Password").fill(requiredPassword(passwordEnvironment));
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page).toHaveURL(/\/resources$/);
}

async function openResource(page, name) {
  const resourceLink = page.getByRole("link", { name });
  const resourceID = (await resourceLink.getAttribute("href")).split("/").pop();
  await resourceLink.click();
  return resourceID;
}

async function openNetworkLab(page) {
  return openResource(page, /NET-01 — Network Test Bench/);
}

function controlledNow() {
  const value = process.env.LABRESERVE_TEST_NOW;
  if (!value) throw new Error("LABRESERVE_TEST_NOW must be supplied to browser verification");
  const instant = new Date(value);
  if (Number.isNaN(instant.getTime())) throw new Error("LABRESERVE_TEST_NOW must be a valid instant");
  return instant;
}

function jakartaDateAfter(days) {
  const parts = Object.fromEntries(
    new Intl.DateTimeFormat("en-US", {
      timeZone: "Asia/Jakarta",
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
    })
      .formatToParts(controlledNow())
      .filter((part) => part.type !== "literal")
      .map((part) => [part.type, part.value]),
  );
  return new Date(Date.UTC(Number(parts.year), Number(parts.month) - 1, Number(parts.day) + days))
    .toISOString()
    .slice(0, 10);
}

function expectedUTCFromJakarta(value) {
  return new Date(`${value}:00+07:00`).toISOString().replace(".000Z", "Z");
}

async function fillBookingForm(form, start, end, purpose) {
  await form.getByLabel("Start time (Asia/Jakarta)").fill(start);
  await form.getByLabel("End time (Asia/Jakarta)").fill(end);
  await form.getByLabel("Purpose").fill(purpose);
}

async function bookingIDsOnPage(page) {
  return page.locator('.booking-row a[href^="/bookings/"]').evaluateAll((links) =>
    links.map((link) => link.getAttribute("href").split("/").pop()),
  );
}

async function insertMyBookingsFixture(pool, ownerID, resourceID, start, purpose, state = "confirmed") {
  const end = new Date(start.getTime() + 30 * 60 * 1000);
  const cancelledBy = state === "cancelled" ? ownerID : null;
  const cancelledAt = state === "cancelled" ? start : null;
  const result = await pool.query(
    `INSERT INTO bookings (
       id, resource_id, owner_account_id, request_id, start_at, end_at,
       purpose, state, created_at, cancelled_by_account_id, cancelled_at
     ) VALUES (gen_random_uuid(), $1, $2, gen_random_uuid(), $3, $4, $5, $6, $3, $7, $8)
     RETURNING id::text`,
    [resourceID, ownerID, start, end, purpose, state, cancelledBy, cancelledAt],
  );
  return result.rows[0].id;
}

test("My Bookings is owner-scoped, retained, paginated, and read-only", async ({ page, request, browser, baseURL }) => {
  const unauthenticated = await request.get(`${baseURL}/my-bookings`, { maxRedirects: 0 });
  expect(unauthenticated.status()).toBe(303);

  await signIn(page);
  expect(await page.evaluate(() => Intl.DateTimeFormat().resolvedOptions().timeZone)).toBe("America/New_York");
  await page.goto("/my-bookings");
  await expect(page.getByRole("heading", { name: "My Bookings" })).toBeVisible();
  await expect(page.getByText("You don't have any bookings yet.")).toBeVisible();
  await expect(page.getByRole("link", { name: "Browse resources" })).toHaveAttribute("href", "/resources");

  const pool = getVerificationPool();
  const accountIDs = await pool.query("SELECT login, id::text FROM accounts WHERE login = ANY($1)", [["alex@example.test", "sam@example.test"]]);
  const ownerIDs = Object.fromEntries(accountIDs.rows.map((row) => [row.login, row.id]));
  const resourceRows = await pool.query("SELECT code, id::text FROM resources WHERE lower(code) = ANY($1)", [["net-01", "k8s-01", "demo-01"]]);
  const resources = Object.fromEntries(resourceRows.rows.map((row) => [row.code.toLowerCase(), row.id]));
  expect(Object.keys(ownerIDs)).toHaveLength(2);
  expect(Object.keys(resources)).toHaveLength(3);

  const prefix = `task3a-e2e-${Date.now()}`;
  const now = controlledNow();
  const past = new Date(now.getTime() - 48 * 60 * 60 * 1000);
  await insertMyBookingsFixture(pool, ownerIDs["alex@example.test"], resources["net-01"], past, `${prefix} past`);
  const inUse = new Date(now.getTime() - 5 * 60 * 1000);
  await insertMyBookingsFixture(pool, ownerIDs["alex@example.test"], resources["k8s-01"], inUse, `${prefix} in use`);
  const upcoming = new Date(now.getTime() + 24 * 60 * 60 * 1000);
  const xssPurpose = `<script>window.task3aXss = true</script> ${prefix} upcoming`;
  const upcomingID = await insertMyBookingsFixture(pool, ownerIDs["alex@example.test"], resources["demo-01"], upcoming, xssPurpose);
  const cancelled = new Date(now.getTime() + 2 * 24 * 60 * 60 * 1000);
  await insertMyBookingsFixture(pool, ownerIDs["alex@example.test"], resources["demo-01"], cancelled, `${prefix} cancelled`, "cancelled");

  for (let index = 0; index < 52; index++) {
    const start = new Date(now.getTime() + 3 * 24 * 60 * 60 * 1000 + Math.floor(index / 2) * 30 * 60 * 1000);
    const resourceID = index % 2 === 0 ? resources["net-01"] : resources["k8s-01"];
    await insertMyBookingsFixture(pool, ownerIDs["alex@example.test"], resourceID, start, `${prefix} page-${index}`);
  }
  const samBookingStart = new Date(now.getTime() + 4 * 24 * 60 * 60 * 1000);
  const samBookingID = await insertMyBookingsFixture(pool, ownerIDs["sam@example.test"], resources["demo-01"], samBookingStart, `${prefix} Sam only`);

  const expectedRows = await pool.query(
    "SELECT id::text FROM bookings WHERE owner_account_id = $1 ORDER BY start_at ASC, id ASC",
    [ownerIDs["alex@example.test"]],
  );
  expect(expectedRows.rows).toHaveLength(56);
  const expectedIDs = expectedRows.rows.map((row) => row.id);
  const beforeReads = await pool.query("SELECT (SELECT count(*)::int FROM bookings) AS bookings, (SELECT count(*)::int FROM activity_events) AS events");

  await page.goto(`/my-bookings?owner_id=${ownerIDs["sam@example.test"]}&account_id=${ownerIDs["sam@example.test"]}`);
  await expect(page.locator(".booking-row")).toHaveCount(25);
  await expect(page.getByRole("link", { name: "Next page" })).toHaveAttribute("href", "/my-bookings?page=2");
  await expect(page.getByText("Upcoming", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("In use", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("Past", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("Cancelled", { exact: true }).first()).toBeVisible();
  const firstPageIDs = await bookingIDsOnPage(page);
  expect(firstPageIDs).toEqual(expectedIDs.slice(0, 25));
  expect(firstPageIDs).toContain(upcomingID);
  expect(await page.locator("script").count()).toBe(0);
  expect(await page.evaluate(() => window.task3aXss)).toBeUndefined();
  const upcomingRow = page.locator(".booking-row").filter({ has: page.getByRole("link", { name: `Booking ${upcomingID}` }) });
  await expect(upcomingRow).toContainText("Resource: DEMO-01 — Customer Demo Environment");
  await expect(upcomingRow.locator("p").nth(2)).toHaveText(`Purpose: ${xssPurpose}`);
  await expect(upcomingRow).toContainText("Asia/Jakarta");
  await expect(upcomingRow.getByRole("link", { name: `Booking ${upcomingID}` })).toBeVisible();
  expect(firstPageIDs).not.toContain(samBookingID);
  await expect(page.getByText(`${prefix} Sam only`)).toHaveCount(0);

  await page.getByRole("link", { name: `Booking ${upcomingID}` }).click();
  await expect(page.getByRole("heading", { name: "Booking details" })).toBeVisible();
  await expect(page.getByText("DEMO-01 — Customer Demo Environment")).toBeVisible();
  await page.goto("/my-bookings");

  await page.getByRole("link", { name: "Next page" }).click();
  await expect(page).toHaveURL(/\/my-bookings\?page=2$/);
  await expect(page.locator(".booking-row")).toHaveCount(25);
  const secondPageIDs = await bookingIDsOnPage(page);
  expect(secondPageIDs).toEqual(expectedIDs.slice(25, 50));
  expect(new Set([...firstPageIDs, ...secondPageIDs]).size).toBe(50);

  await page.getByRole("link", { name: "Next page" }).click();
  await expect(page).toHaveURL(/\/my-bookings\?page=3$/);
  await expect(page.locator(".booking-row")).toHaveCount(6);
  const thirdPageIDs = await bookingIDsOnPage(page);
  expect(thirdPageIDs).toEqual(expectedIDs.slice(50));
  expect(new Set([...firstPageIDs, ...secondPageIDs, ...thirdPageIDs]).size).toBe(56);
  await expect(page.getByRole("link", { name: "Next page" })).toHaveCount(0);

  const samContext = await browser.newContext({ baseURL, timezoneId: "America/New_York" });
  try {
    const samPage = await samContext.newPage();
    await signIn(samPage, "sam@example.test", "SEED_SAM_PASSWORD");
    await samPage.goto("/my-bookings");
    const samRows = samPage.locator(".booking-row");
    await expect(samRows).toHaveCount(1);
    await expect(samRows.first()).toContainText(`${prefix} Sam only`);
    await expect(samRows.first()).not.toContainText(`${prefix} page-`);
  } finally {
    await samContext.close();
  }

  const afterReads = await pool.query("SELECT (SELECT count(*)::int FROM bookings) AS bookings, (SELECT count(*)::int FROM activity_events) AS events");
  expect(afterReads.rows[0]).toEqual(beforeReads.rows[0]);
});

test("booking form creates, safely replays, and displays Jakarta cross-midnight bookings", async ({ page, request, baseURL }) => {
  await signIn(page);
  expect(await page.evaluate(() => Intl.DateTimeFormat().resolvedOptions().timeZone)).toBe("America/New_York");
  const resourceID = await openNetworkLab(page);
  const unauthenticatedForm = await request.get(`${baseURL}/resources/${resourceID}/bookings/new`, { maxRedirects: 0 });
  expect(unauthenticatedForm.status()).toBe(303);
  await page.getByRole("link", { name: "Book this resource" }).click();

  const form = page.locator(`form[action="/resources/${resourceID}/bookings"]`);
  const requestID = await form.locator('input[name="request_id"]').inputValue();
  expect(requestID).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i);
  const csrf = await form.locator('input[name="_csrf"]').inputValue();
  const sessionCookie = (await page.context().cookies(baseURL)).find((cookie) => cookie.name === "labreserve_session");
  expect(sessionCookie).toBeTruthy();
  const action = `${baseURL}/resources/${resourceID}/bookings`;
  const headers = { Cookie: `labreserve_session=${sessionCookie.value}`, Origin: baseURL };
  const unauthenticatedSchedule = await request.get(`${baseURL}/resources/${resourceID}`, { maxRedirects: 0 });
  expect(unauthenticatedSchedule.status()).toBe(303);
  const startDate = jakartaDateAfter(1);
  const endDate = jakartaDateAfter(2);
  const start = `${startDate}T23:30`;
  const end = `${endDate}T01:00`;
  const purpose = "<script>window.labreserveBookingXss = true</script> cross-midnight test";
  const submission = {
    _csrf: csrf,
    request_id: requestID,
    start_jakarta: start,
    end_jakarta: end,
    purpose,
  };
  const missingCsrf = await request.post(action, {
    headers,
    form: { request_id: requestID, start_jakarta: start, end_jakarta: end, purpose },
    maxRedirects: 0,
  });
  expect(missingCsrf.status()).toBe(403);
  await expectRequestCounts("alex@example.test", requestID, 0, 0);

  const invalidTransport = await request.post(action, {
    headers,
    form: { ...submission, start_jakarta: "not-a-Jakarta-time" },
    maxRedirects: 0,
  });
  expect(invalidTransport.status()).toBe(422);
  const invalidBody = await invalidTransport.text();
  expect(invalidBody).toContain("Enter valid start and end date/time values in the Asia/Jakarta format.");
  expect(invalidBody).toContain(`name="request_id" value="${requestID}"`);
  await expectRequestCounts("alex@example.test", requestID, 0, 0);

  await fillBookingForm(form, start, end, purpose);
  await form.evaluate((node) => {
    for (const [name, value] of Object.entries({ owner_id: "00000000-0000-4000-8000-000000000000", role: "coordinator" })) {
      const field = document.createElement("input");
      field.type = "hidden";
      field.name = name;
      field.value = value;
      node.append(field);
    }
  });
  await form.getByRole("button", { name: "Create booking" }).click();
  await expect(page).toHaveURL(/\/bookings\/[0-9a-f-]+\?result=created$/i);
  await expect(page.getByRole("status")).toContainText("Booking created successfully");
  await expect(page.getByRole("main").getByText("Alex Morgan", { exact: true })).toBeVisible();
  await expect(page.getByText(purpose, { exact: true })).toBeVisible();
  const detailTimes = page.locator(".booking-detail time");
  await expect(detailTimes.nth(0)).toHaveAttribute("datetime", expectedUTCFromJakarta(start));
  await expect(detailTimes.nth(1)).toHaveAttribute("datetime", expectedUTCFromJakarta(end));
  expect(await page.evaluate(() => window.labreserveBookingXss)).toBeUndefined();
  expect(await page.locator("script").count()).toBe(0);

  const bookingID = new URL(page.url()).pathname.split("/").pop();
  await expectRequestCounts("alex@example.test", requestID, 1, 1);
  await expectIntervalCounts(resourceID, start, end, 1, 1);
  const changedRequest = await request.post(action, {
    headers,
    form: { ...submission, purpose: "changed data under a successful request identifier" },
    maxRedirects: 0,
  });
  expect(changedRequest.status()).toBe(409);
  const changedBody = await changedRequest.text();
  expect(changedBody).toContain("request identifier already belongs to different booking data");
  expect(changedBody).toContain(`name="request_id" value="${requestID}"`);
  await expectRequestCounts("alex@example.test", requestID, 1, 1);

  const replay = await request.post(action, { headers, form: submission, maxRedirects: 0 });
  expect(replay.status()).toBe(303);
  expect(replay.headers().location).toBe(`/bookings/${bookingID}?result=replayed`);
  await expectRequestCounts("alex@example.test", requestID, 1, 1);
  await expectIntervalCounts(resourceID, start, end, 1, 1);
  await page.goto(replay.headers().location);
  await expect(page.getByRole("status")).toContainText("Showing the existing booking, not a second creation");

  await page.goto(`/resources/${resourceID}?date=${startDate}`);
  await expect(page.getByRole("link", { name: `Booking ${bookingID}` })).toBeVisible();
  await expect(page.getByText("Upcoming", { exact: true })).toBeVisible();
  await expect(page.locator(".booking-row time").nth(0)).toContainText("23:30");
  await expect(page.locator(".booking-row time").nth(1)).toContainText("01:00");
  await expect(page.locator(".booking-row time").nth(0)).toHaveAttribute("datetime", expectedUTCFromJakarta(start));
  await expect(page.locator(".booking-row time").nth(1)).toHaveAttribute("datetime", expectedUTCFromJakarta(end));

  await page.goto(`/resources/${resourceID}?date=${endDate}`);
  await expect(page.getByRole("link", { name: `Booking ${bookingID}` })).toBeVisible();

  await page.goto("/resources");
  const otherResourceID = await openResource(page, /K8S-01 — Kubernetes Integration Lab/);
  await page.getByRole("link", { name: "Book this resource" }).click();
  const otherForm = page.locator(`form[action="/resources/${otherResourceID}/bookings"]`);
  const otherRequestID = await otherForm.locator('input[name="request_id"]').inputValue();
  expect(otherRequestID).not.toBe(requestID);
  const otherPurpose = `${purpose} on a different Resource`;
  await fillBookingForm(otherForm, start, end, otherPurpose);
  await otherForm.getByRole("button", { name: "Create booking" }).click();
  await expect(page).toHaveURL(/\/bookings\/[0-9a-f-]+\?result=created$/i);
  await expect(page.getByRole("main").getByText("Alex Morgan", { exact: true })).toBeVisible();
  const otherDetailTimes = page.locator(".booking-detail time");
  await expect(otherDetailTimes.nth(0)).toHaveAttribute("datetime", expectedUTCFromJakarta(start));
  await expect(otherDetailTimes.nth(1)).toHaveAttribute("datetime", expectedUTCFromJakarta(end));
  await expectRequestCounts("alex@example.test", otherRequestID, 1, 1);
  await expectIntervalCounts(otherResourceID, start, end, 1, 1);
});

test("Engineer B cannot overlap Engineer A, and an adjacent retry appears in schedule order", async ({ page, browser }) => {
  await signIn(page);
  const resourceID = await openNetworkLab(page);
  const bookingDate = jakartaDateAfter(8);
  const start = `${bookingDate}T10:00`;
  const end = `${bookingDate}T11:00`;

  await page.goto(`/resources/${resourceID}/bookings/new`);
  const engineerAForm = page.locator(`form[action="/resources/${resourceID}/bookings"]`);
  const engineerARequestID = await engineerAForm.locator('input[name="request_id"]').inputValue();
  await fillBookingForm(engineerAForm, start, end, "Engineer A contested booking");
  await engineerAForm.getByRole("button", { name: "Create booking" }).click();
  await expect(page).toHaveURL(/\/bookings\/[0-9a-f-]+\?result=created$/i);
  const firstBookingID = new URL(page.url()).pathname.split("/").pop();
  await expectRequestCounts("alex@example.test", engineerARequestID, 1, 1);
  await expectIntervalCounts(resourceID, start, end, 1, 1);
  await expect(page.getByRole("main").getByText("Alex Morgan", { exact: true })).toBeVisible();

  const engineerBContext = await browser.newContext({ baseURL: process.env.PLAYWRIGHT_BASE_URL || "http://127.0.0.1:18080", timezoneId: "America/New_York" });
  try {
    const engineerBPage = await engineerBContext.newPage();
    await signIn(engineerBPage, "sam@example.test", "SEED_SAM_PASSWORD");
    expect(await engineerBPage.evaluate(() => Intl.DateTimeFormat().resolvedOptions().timeZone)).toBe("America/New_York");
    const engineerBResourceID = await openNetworkLab(engineerBPage);
    expect(engineerBResourceID).toBe(resourceID);
    await engineerBPage.getByRole("link", { name: "Book this resource" }).click();
    const engineerBForm = engineerBPage.locator(`form[action="/resources/${resourceID}/bookings"]`);
    const retryRequestID = await engineerBForm.locator('input[name="request_id"]').inputValue();
    await fillBookingForm(engineerBForm, start, end, "Engineer B overlapping booking");
    await engineerBForm.getByRole("button", { name: "Create booking" }).click();
    await expect(engineerBPage).toHaveURL(`/resources/${resourceID}/bookings`);
    await expect(engineerBPage.getByRole("alert")).toContainText("selected interval is no longer available");
    expect(await engineerBForm.locator('input[name="request_id"]').inputValue()).toBe(retryRequestID);
    await expectRequestCounts("sam@example.test", retryRequestID, 0, 0);
    await expectIntervalCounts(resourceID, start, end, 1, 1);

    await page.goto(`/resources/${resourceID}?date=${bookingDate}`);
    let rows = page.locator(".booking-row");
    await expect(rows).toHaveCount(1);
    await expect(rows.getByRole("link", { name: `Booking ${firstBookingID}` })).toBeVisible();
    await expect(rows.first().getByText("Alex Morgan", { exact: true })).toBeVisible();
    await expect(rows.first()).toContainText("Purpose: Engineer A contested booking");

    await fillBookingForm(engineerBForm, `${bookingDate}T11:00`, `${bookingDate}T12:00`, "Engineer B adjacent booking");
    expect(await engineerBForm.locator('input[name="request_id"]').inputValue()).toBe(retryRequestID);
    await engineerBForm.getByRole("button", { name: "Create booking" }).click();
    await expect(engineerBPage).toHaveURL(/\/bookings\/[0-9a-f-]+\?result=created$/i);
    const adjacentBookingID = new URL(engineerBPage.url()).pathname.split("/").pop();
    await expectRequestCounts("sam@example.test", retryRequestID, 1, 1);
    await expectIntervalCounts(resourceID, `${bookingDate}T11:00`, `${bookingDate}T12:00`, 1, 1);
    await expectIntervalCounts(resourceID, start, end, 1, 1);
    expect(adjacentBookingID).not.toBe(firstBookingID);

    await engineerBPage.goto(`/resources/${resourceID}?date=${bookingDate}`);
    rows = engineerBPage.locator(".booking-row");
    await expect(rows).toHaveCount(2);
    await expect(rows.nth(0).getByRole("link", { name: `Booking ${firstBookingID}` })).toBeVisible();
    await expect(rows.nth(0).getByText("Alex Morgan", { exact: true })).toBeVisible();
    await expect(rows.nth(0).locator("time").nth(0)).toContainText("10:00");
    await expect(rows.nth(0).locator("time").nth(1)).toContainText("11:00");
    await expect(rows.nth(1).getByRole("link", { name: `Booking ${adjacentBookingID}` })).toBeVisible();
    await expect(rows.nth(1).getByText("Sam Rivera", { exact: true })).toBeVisible();
    await expect(rows.nth(1).locator("time").nth(0)).toContainText("11:00");
    await expect(rows.nth(1).locator("time").nth(1)).toContainText("12:00");
    await expect(rows.nth(1).locator("time").nth(0)).toHaveAttribute("datetime", expectedUTCFromJakarta(`${bookingDate}T11:00`));
  } finally {
    await engineerBContext.close();
  }
});

test("Coordinator creates a Booking for their authenticated account", async ({ page }) => {
  await signIn(page, "jordan@example.test", "SEED_JORDAN_PASSWORD");
  const resourceID = await openResource(page, /DEMO-01 — Customer Demo Environment/);
  await page.getByRole("link", { name: "Book this resource" }).click();
  const form = page.locator(`form[action="/resources/${resourceID}/bookings"]`);
  const bookingDate = jakartaDateAfter(15);
  const start = `${bookingDate}T10:00`;
  const end = `${bookingDate}T11:00`;
  const coordinatorRequestID = await form.locator('input[name="request_id"]').inputValue();
  await fillBookingForm(form, start, end, "Coordinator self-booking");
  await form.evaluate((node) => {
    for (const [name, value] of Object.entries({ owner_id: "00000000-0000-4000-8000-000000000000", role: "engineer" })) {
      const field = document.createElement("input");
      field.type = "hidden";
      field.name = name;
      field.value = value;
      node.append(field);
    }
  });
  await form.getByRole("button", { name: "Create booking" }).click();
  await expect(page).toHaveURL(/\/bookings\/[0-9a-f-]+\?result=created$/i);
  await expect(page.getByRole("main").getByText("Jordan Lee", { exact: true })).toBeVisible();
  await expectRequestCounts("jordan@example.test", coordinatorRequestID, 1, 1);
  await expectIntervalCounts(resourceID, start, end, 1, 1);
  const times = page.locator(".booking-detail time");
  await expect(times.nth(0)).toHaveAttribute("datetime", expectedUTCFromJakarta(start));
  await expect(times.nth(1)).toHaveAttribute("datetime", expectedUTCFromJakarta(end));
});
