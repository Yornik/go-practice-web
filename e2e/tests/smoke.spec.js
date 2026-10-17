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

  expect(errors).toEqual([]);
});
