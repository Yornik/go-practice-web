// The smoke test of docs/DESIGN.md, "Testing": the setup link, sign-up,
// sign-out and sign-in with a passkey, the IDE on 01-logfmt with hint 1
// behind its soft gate, the reference solution passing every test with
// -race, and the verify page of a certificate lookup.
import { test, expect } from "@playwright/test";
import { setupLink, solution, virtualAuthenticator, watchErrors } from "./helpers.js";

test("setup, sign up, solve 01-logfmt, reveal hint 1", async ({ page, context }) => {
  const errors = watchErrors(page);
  await virtualAuthenticator(context, page);

  // The first account comes from the setup link.
  const link = setupLink();
  await page.goto(link.pathname + link.search);
  await expect(page.getByRole("heading", { name: "Set up Go Practice" })).toBeVisible();
  await page.getByLabel("Username").fill("ada");
  await page.getByLabel("Display name").fill("Ada Admin");
  await page.getByLabel("Email address").fill("ada@example.com");
  await page.getByRole("button", { name: "Create my account and passkey" }).click();
  await page.waitForURL("**/learn");
  await expect(page.getByRole("heading", { name: "Your course" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Admin" })).toBeVisible();

  // Sign out, then sign in through passkey autofill (conditional UI).
  // Chrome's virtual authenticator answers the autofill request at once;
  // a real browser waits until the user picks the passkey.
  const signOut = async () => {
    await page.getByRole("button", { name: "Ada Admin" }).click();
    await page.getByRole("menuitem", { name: "Sign out" }).click();
    await page.waitForURL((u) => u.pathname === "/");
  };
  await signOut();
  await page.goto("/login");
  await page.waitForURL("**/learn");

  // Sign out again and use the button, as in a browser without autofill.
  await signOut();
  await page.addInitScript(() => {
    PublicKeyCredential.isConditionalMediationAvailable = async () => false;
  });
  await page.goto("/login");
  await page.getByRole("button", { name: "Sign in with a passkey" }).click();
  await page.waitForURL("**/learn");

  // The IDE.
  await page.goto("/learn/01-logfmt");
  await expect(page.locator(".ide")).toHaveAttribute("data-ready", "true", { timeout: 60_000 });
  await expect(page.locator("[data-file-tabs] [role=tab] .file-name").first()).toHaveText("logfmt.go");

  // The ladder starts folded: the spec comes first.
  await expect(page.locator("[data-ladder-sub]")).toContainText("Nothing revealed.");
  await page.getByRole("button", { name: "Unfold" }).click();

  // Hint 1 asks for a confirmation before 25 minutes of work; the safe
  // choice has the focus.
  await page.getByRole("button", { name: "Reveal hint 1 now" }).click();
  await expect(page.getByRole("button", { name: "Keep working" })).toBeFocused();
  const gate = page.locator("[data-gate]");
  await expect(gate).toContainText("25 minutes");
  await page.getByRole("button", { name: "Reveal it" }).click();
  const hint1 = page.locator('[data-level="1"] [data-hint-body]');
  await expect(hint1).not.toBeEmpty();
  await expect(gate).not.toContainText("25 minutes");
  await expect(page.getByRole("button", { name: "Reveal hint 2 now" })).toBeVisible();

  // Paste the reference solution; autosave stores it.
  const files = solution("01-logfmt");
  await page.evaluate((files) => {
    for (const m of window.monaco.editor.getModels()) {
      const name = m.uri.path.split("/").pop();
      if (name in files) m.setValue(files[name]);
    }
  }, files);
  await expect(page.locator("[data-save-status]")).toContainText("Saved", { timeout: 20_000 });
  await expect(page.locator("[data-lint-status]")).toHaveText(/problem/, { timeout: 60_000 });

  // All tests with -race pass and mark the exercise passed.
  await page.locator("[data-step-select]").selectOption("all");
  await expect(page.locator("[data-race]")).toBeChecked();
  await page.getByRole("button", { name: "Run tests", exact: true }).click();
  await expect(page.locator("[data-run-summary]")).toContainText(" 0 fail", { timeout: 180_000 });
  await expect(page.locator("[data-run-notes]")).toContainText("Exercise 01 passes with -race");
  await expect(page.locator("[data-cell-passed]")).toBeVisible();
  await expect(page.locator('.test-node[data-state="pass"]').first()).toBeVisible();

  // The dashboard counts it.
  await page.goto("/learn");
  await expect(page.locator('[data-stat="passed"]')).toHaveText("1");

  // Certificates: nothing to claim yet; the verify lookup works.
  await page.goto("/certificates");
  await expect(page.getByRole("heading", { name: /Tier 1/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /^Claim the/ })).toHaveCount(0);
  await page.goto("/verify?id=gp-0000-0000-0000");
  await expect(page.getByRole("heading", { name: "No certificate has this ID." })).toBeVisible();

  // A runner without a race detector: the bootstrap says so, and the IDE
  // disables the toggle before the first run. The dev runner has one, so
  // the answer is rewritten in the browser.
  await page.route("**/api/exercises/02-*", async (route) => {
    const res = await route.fetch();
    await route.fulfill({ response: res, json: { ...(await res.json()), raceUnavailable: true } });
  });
  await page.goto("/learn/02-accesslog");
  await expect(page.locator(".ide")).toHaveAttribute("data-ready", "true", { timeout: 60_000 });
  await expect(page.locator("[data-race]")).toBeDisabled();
  await expect(page.locator("[data-race-status]")).toHaveText("-race not available");
  await expect(page.locator("[data-run-notes]")).toContainText("The race detector is not available on this server. Runs go without -race.");
  await page.unrouteAll();

  // Code that a closed tab could not save stays on offer through an edit and
  // a successful save, until the learner restores or discards it.
  const key = "gpw-unsaved:02-accesslog:accesslog.go";
  const kept = "package accesslog\n\n// kept by this browser\n";
  await page.evaluate(([k, content]) => localStorage.setItem(k, JSON.stringify({ at: Date.now(), content })), [key, kept]);
  await page.reload();
  await expect(page.locator(".ide")).toHaveAttribute("data-ready", "true", { timeout: 60_000 });
  const banner = page.locator("[data-save-banner]");
  await expect(banner).toContainText("This browser kept changes to accesslog.go");
  await page.evaluate(() => {
    const m = window.monaco.editor.getModels().find((m) => m.uri.path.endsWith("/accesslog.go"));
    m.setValue(m.getValue() + "\n// an edit\n");
  });
  await expect(page.locator("[data-save-status]")).toContainText("Saved", { timeout: 20_000 });
  await expect(banner).toContainText("This browser kept changes to accesslog.go");
  expect(JSON.parse(await page.evaluate((k) => localStorage.getItem(k), key)).content).toBe(kept);
  await banner.getByRole("button", { name: "Restore them" }).click();
  await expect(banner).toBeHidden();
  expect(await page.evaluate(() => window.monaco.editor.getModels().find((m) => m.uri.path.endsWith("/accesslog.go")).getValue())).toBe(kept);

  expect(errors).toEqual([]);
});
