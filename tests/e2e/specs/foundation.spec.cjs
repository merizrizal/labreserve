const { test, expect } = require("@playwright/test");

function requiredPassword(name) {
  const value = process.env[name];
  if (!value) throw new Error(`${name} must be supplied to the browser verification`);
  return value;
}

async function signIn(page, login, password) {
  await page.goto("/login");
  await page.getByLabel("Email").fill(login);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page).toHaveURL(/\/resources$/);
}

test("failed and successful sign-in use seeded account identity and role", async ({ page }) => {
  await page.goto("/resources");
  await expect(page).toHaveURL(/\/login$/);

  await page.getByLabel("Email").fill("alex@example.test");
  await page.getByLabel("Password").fill("incorrect-password");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("alert")).toContainText("login or password was not recognized");

  await page.getByLabel("Password").fill(requiredPassword("SEED_ALEX_PASSWORD"));
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page).toHaveURL(/\/resources$/);
  await expect(page.getByRole("heading", { name: "Resources" })).toBeVisible();
  await expect(page.getByText("Alex Morgan")).toBeVisible();
  await expect(page.locator(".role")).toHaveText("(engineer)");

  await page.goto("/resources?role=coordinator&user_id=not-the-current-user");
  await expect(page.locator(".role")).toHaveText("(engineer)");
});

test("coordinator role comes from the seeded account, not request parameters", async ({ page }) => {
  await signIn(page, "jordan@example.test", requiredPassword("SEED_JORDAN_PASSWORD"));
  await expect(page.getByText("Jordan Lee")).toBeVisible();
  await expect(page.locator(".role")).toHaveText("(coordinator)");

  await page.goto("/resources?role=engineer&user_id=alex@example.test");
  await expect(page.locator(".role")).toHaveText("(coordinator)");
});

test("users browse a resource's empty schedule; CSRF and logout invalidate the session", async ({ page, request, baseURL }) => {
  await signIn(page, "sam@example.test", requiredPassword("SEED_SAM_PASSWORD"));
  await page.getByRole("link", { name: /NET-01 — Network Test Bench/ }).click();
  await expect(page.getByRole("heading", { name: "NET-01 — Network Test Bench" })).toBeVisible();
  await expect(page.getByText("No bookings are scheduled for this date.")).toBeVisible();
  await expect(page.getByText("Asia/Jakarta").first()).toBeVisible();

  const sessionCookie = (await page.context().cookies(baseURL)).find((cookie) => cookie.name === "labreserve_session");
  expect(sessionCookie).toBeTruthy();
  const headers = {
    Cookie: `labreserve_session=${sessionCookie.value}`,
    Origin: baseURL,
  };
  const missingCsrf = await request.post("/logout", { headers, form: {} });
  expect(missingCsrf.status()).toBe(403);

  await page.getByRole("button", { name: "Sign out" }).click();
  await expect(page).toHaveURL(/\/login$/);
  const reusedSession = await request.get("/resources", { headers, maxRedirects: 0 });
  expect(reusedSession.status()).toBe(303);
  expect(reusedSession.headers().location).toBe("/login");
});
