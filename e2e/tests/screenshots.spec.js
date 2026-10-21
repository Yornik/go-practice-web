// Screenshots of every page, light and dark, at 1440x900 and 390x844
// (".design/FINAL.md", section 14). It runs only when SCREENSHOTS_DIR is
// set, and only on its own, because it uses the setup link:
//
//   SCREENSHOTS_DIR=$PWD/../.design/final npx playwright test screenshots
//
// It passes tier 1 with the course's reference solutions to get a real
// certificate, and it replaces the text of a revealed hint before the
// screenshot, so the pictures do not spoil the course.
import { test, expect } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { confirmEmail, mailLink, setupLink, solution, virtualAuthenticator, watchErrors, webLog } from "./helpers.js";

const dir = process.env.SCREENSHOTS_DIR;
test.skip(!dir, "set SCREENSHOTS_DIR to take the screenshots");
test.setTimeout(30 * 60_000);

const VARIANTS = [
  { size: "desktop", viewport: { width: 1440, height: 900 } },
  { size: "mobile", viewport: { width: 390, height: 844 } },
];
const THEMES = ["light", "dark"];

/** Opens a page in every theme and size and saves the screenshots. */
async function shootAll(browser, name, url, { state, full = true, prepare, mobileFull = true, errors } = {}) {
  for (const v of VARIANTS) {
    for (const theme of THEMES) {
      const ctx = await browser.newContext({ viewport: v.viewport, colorScheme: theme, storageState: state });
      const page = await ctx.newPage();
      const errs = watchErrors(page);
      await page.goto(url);
      await page.evaluate(() => document.fonts.ready);
      if (prepare) await prepare(page, v.size, theme);
      await page.waitForTimeout(400);
      await page.screenshot({ path: path.join(dir, `${name}-${theme}-${v.size}.png`), fullPage: v.size === "mobile" ? mobileFull : full });
      errors.push(...errs.map((e) => `${name} ${theme} ${v.size}: ${e}`));
      await ctx.close();
    }
  }
}

/** Puts code into a file of the open exercise through Monaco's model. */
async function setFile(page, file, content) {
  await page.evaluate(
    ([file, content]) => {
      const m = window.monaco.editor.getModels().find((x) => x.uri.path.endsWith(`/${file}`));
      m.setValue(content);
    },
    [file, content],
  );
}

async function setValidate(page, body) {
  await page.evaluate((body) => {
    const m = window.monaco.editor.getModels().find((x) => x.uri.path.endsWith("/retry.go"));
    const re = /func \(p Policy\) Validate\(\) error \{\n[\s\S]*?\n\}/;
    m.setValue(m.getValue().replace(re, `func (p Policy) Validate() error {\n${body}\n}`));
  }, body);
}

/** Exercise 32 with a wrong Validate, a failing run, hint 1 and a compile error typed after the run. */
async function ideScenario(page, size) {
  await expect(page.locator(".ide")).toHaveAttribute("data-ready", "true", { timeout: 60_000 });
  await setValidate(page, "\t// Check the fields first.\n\treturn nil");
  await expect(page.locator("[data-save-status]")).toContainText("Saved", { timeout: 20_000 });
  await page.locator("[data-step-select]").selectOption("step-1");
  await page.getByRole("button", { name: "Run tests", exact: true }).click();
  await expect(page.locator("[data-run-summary]")).toContainText(/ at \d\d:\d\d/, { timeout: 180_000 });
  // The ladder starts folded; the scenario shows hint 1 revealed.
  const unfold = page.getByRole("button", { name: "Unfold" });
  if (await unfold.count()) await unfold.click();
  const early = page.getByRole("button", { name: "Reveal hint 1 now" });
  if (await early.count()) {
    await early.click();
    await page.getByRole("button", { name: "Reveal it" }).click();
  }
  await hideHintText(page);
  await setValidate(page, "\t// Check the fields first.\n\tif p.MaxAttempts < 1 {\n\t\treturn errInvalid\n\t}\n\treturn nil");
  await expect(page.locator(".chip", { hasText: "Compiler" }).first()).toBeVisible({ timeout: 60_000 });
  await page.locator('.chip[data-n="1"]').first().click();
  if (size === "mobile") await page.evaluate(() => window.scrollTo(0, 0));
  await page.waitForTimeout(800);
}

/**
 * Replaces the text of every revealed hint with a stand-in: the pictures
 * must not spoil the course. Call it after every load that shows a hint.
 */
async function hideHintText(page) {
  await expect(page.locator('[data-level="1"] [data-hint-body]')).not.toBeEmpty();
  await page.evaluate(() => {
    for (const b of document.querySelectorAll("[data-hint-body]")) {
      if (!b.childElementCount && !b.textContent.trim()) continue;
      const p = document.createElement("p");
      p.textContent = "The text of this hint shows here. It is left out of this screenshot, so the course stays unspoiled.";
      b.replaceChildren(p);
    }
  });
}

/** Passes an exercise with the course's reference solution. */
async function pass(page, slug) {
  await page.goto(`/learn/${slug}`);
  await expect(page.locator(".ide")).toHaveAttribute("data-ready", "true", { timeout: 60_000 });
  const files = solution(slug);
  await page.evaluate((files) => {
    for (const m of window.monaco.editor.getModels()) {
      const name = m.uri.path.split("/").pop();
      if (name in files) m.setValue(files[name]);
    }
  }, files);
  await expect(page.locator("[data-save-status]")).toContainText("Saved", { timeout: 20_000 });
  await page.locator("[data-step-select]").selectOption("all");
  await page.getByRole("button", { name: "Run tests", exact: true }).click();
  await expect(page.locator("[data-run-notes]")).toContainText("passes with -race", { timeout: 180_000 });
}

test("screenshots of every page", async ({ browser, page, context }) => {
  fs.mkdirSync(dir, { recursive: true });
  const errors = [];
  const anon = { errors };

  // Public pages.
  await shootAll(browser, "landing", "/", anon);
  await shootAll(browser, "guide", "/guide", anon);
  await shootAll(browser, "login", "/login", anon);
  await shootAll(browser, "recover", "/recover", anon);
  await shootAll(browser, "verify-lookup", "/verify", anon);
  await shootAll(browser, "verify-unknown", "/verify/GP-0000-0000-0000", anon);
  await shootAll(browser, "not-found", "/no-such-page", anon);

  // The setup page, then the first account.
  const link = setupLink();
  await shootAll(browser, "setup", link.pathname + link.search, anon);
  await virtualAuthenticator(context, page);
  await page.goto(link.pathname + link.search);
  await page.getByLabel("Username").fill("ada");
  await page.getByLabel("Display name").fill("Ada Lovelace");
  await page.getByLabel("Email address").fill("ada@example.org");
  await page.getByRole("button", { name: "Create my account and passkey" }).click();
  await page.waitForURL("**/check-email");
  const state = await context.storageState();
  const signedIn = { state, errors };

  // The email gate: the page a new account waits on, and the page of the
  // link. Confirming the address opens the course.
  await shootAll(browser, "check-email", "/check-email", signedIn);
  await expect.poll(() => webLog().some((e) => e.to === "ada@example.org" && typeof e.text === "string" && e.text.includes("/verify-email?token=")), { timeout: 20_000 }).toBe(true);
  await shootAll(browser, "verify-email", mailLink("/verify-email", { to: "ada@example.org" }), anon);
  await confirmEmail(page, "ada@example.org");
  await shootAll(browser, "course-empty", "/learn", signedIn);

  // An invite for a second learner: the admin page with its result, and the join page.
  await page.goto("/admin");
  await page.getByLabel("Email (optional)").fill("grace@example.org");
  await page.getByLabel("Note (optional)").fill("Welcome to the course. Start with 01.");
  await page.getByRole("button", { name: "Create the invite link" }).click();
  // The field fills when the API answers; inputValue does not wait for that.
  await expect(page.locator("[data-invite-link]")).not.toHaveValue("");
  const invite = new URL(await page.locator("[data-invite-link]").inputValue());
  await shootAll(browser, "join", invite.pathname + invite.search, anon);

  // The IDE on the untouched stub: "All tests" gives 1 problem for every
  // stub panic, not 1 per test.
  for (const v of VARIANTS) {
    for (const theme of THEMES) {
      const ctx = await browser.newContext({ viewport: v.viewport, colorScheme: theme, storageState: state });
      const p = await ctx.newPage();
      const errs = watchErrors(p);
      await p.goto("/learn/32-retry");
      await expect(p.locator(".ide")).toHaveAttribute("data-ready", "true", { timeout: 60_000 });
      await p.screenshot({ path: path.join(dir, `ide-start-${theme}-${v.size}.png`), fullPage: v.size === "mobile" });
      await p.locator("[data-step-select]").selectOption("all");
      await p.getByRole("button", { name: "Run tests", exact: true }).click();
      await expect(p.locator("[data-run-summary]")).toContainText(/ at \d\d:\d\d/, { timeout: 180_000 });
      await expect(p.locator(".chip", { hasText: "stubs left" })).toHaveCount(1);
      if (v.size === "mobile") await p.evaluate(() => window.scrollTo(0, 0));
      await p.waitForTimeout(800);
      await p.screenshot({ path: path.join(dir, `ide-stub-${theme}-${v.size}.png`), fullPage: v.size === "mobile" });
      errors.push(...errs.map((e) => `ide-stub ${theme} ${v.size}: ${e}`));
      await ctx.close();
    }
  }

  // The IDE: a failing run, a problem with its explanation, hint 1 revealed.
  for (const v of VARIANTS) {
    for (const theme of THEMES) {
      const ctx = await browser.newContext({ viewport: v.viewport, colorScheme: theme, storageState: state });
      const p = await ctx.newPage();
      const errs = watchErrors(p);
      await p.goto("/learn/32-retry");
      await ideScenario(p, v.size);
      await p.screenshot({ path: path.join(dir, `ide-${theme}-${v.size}.png`), fullPage: v.size === "mobile" });
      if (v.size === "desktop") {
        // The got/want problem with its case, and the test file it opens.
        await p.locator(".chip", { hasText: "Failing test" }).first().click();
        await p.waitForTimeout(500);
        await p.screenshot({ path: path.join(dir, `ide-gotwant-${theme}-${v.size}.png`) });
        // The early-reveal step of the ladder and the claim of the next level.
        await p.getByRole("button", { name: "Reveal hint 2 now" }).click();
        await p.waitForTimeout(300);
        await p.screenshot({ path: path.join(dir, `ide-gate-${theme}-${v.size}.png`) });
        // After a reload: the last run's tree. The reload shows hint 1 again,
        // so its text is replaced again before any picture.
        await p.reload();
        await expect(p.locator(".ide")).toHaveAttribute("data-ready", "true", { timeout: 60_000 });
        await hideHintText(p);
        await p.waitForTimeout(800);
        await p.screenshot({ path: path.join(dir, `ide-reload-${theme}-${v.size}.png`) });
        // 1100 px: 2 columns, the problems in the test panel.
        await p.setViewportSize({ width: 1100, height: 800 });
        await p.getByRole("tab", { name: /^Problems/ }).click();
        await p.waitForTimeout(500);
        await p.screenshot({ path: path.join(dir, `ide-1100-${theme}-${v.size}.png`) });
      }
      errors.push(...errs.map((e) => `ide ${theme} ${v.size}: ${e}`));
      await ctx.close();
    }
  }

  // Tier 1 with the reference solutions, for a real certificate.
  for (const slug of ["01-loglevels", "02-repeated", "03-pagenoise", "04-waves", "05-envfile"]) await pass(page, slug);
  await shootAll(browser, "course", "/learn", signedIn);
  await shootAll(browser, "certificates-eligible", "/certificates", signedIn);
  await shootAll(browser, "claim-dialog", "/certificates", {
    ...signedIn,
    full: false,
    mobileFull: false,
    prepare: async (p) => {
      await p.getByRole("button", { name: "Claim the tier 1 certificate" }).click();
      await expect(p.locator("dialog[open]")).toBeVisible();
    },
  });
  await page.goto("/certificates");
  await page.getByRole("button", { name: "Claim the tier 1 certificate" }).click();
  await page.getByLabel("Show my activity on the certificate and the verify page").check();
  await page.getByRole("button", { name: "Issue the certificate" }).click();
  await expect(page.locator('[data-kind="tier1"] [data-card-status]')).toContainText("Certificate issued.");
  const verifyHref = await page.getByRole("link", { name: "Verify page" }).first().getAttribute("href");
  await shootAll(browser, "certificates", "/certificates", signedIn);
  await shootAll(browser, "verify", verifyHref, anon);

  await shootAll(browser, "account", "/account", signedIn);
  await shootAll(browser, "admin", "/admin", signedIn);

  // Recovery: the address is verified, so a link goes out; open it.
  await page.goto("/recover");
  await page.getByLabel("Email address").fill("ada@example.org");
  await page.getByRole("button", { name: "Send a recovery link" }).click();
  await expect(page.locator(".form-status")).toContainText("on its way");
  await expect.poll(() => webLog().some((e) => typeof e.text === "string" && e.text.includes("/recover/confirm?token=")), { timeout: 20_000 }).toBe(true);
  await shootAll(browser, "recover-confirm", mailLink("/recover/confirm"), anon);

  expect(errors).toEqual([]);
});
