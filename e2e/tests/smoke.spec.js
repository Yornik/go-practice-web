// The smoke test of docs/DESIGN.md, "Testing": the setup link, sign-up,
// the email gate until the address is verified, sign-out and sign-in with
// a passkey, the IDE on 01-loglevels with hint 1 behind its soft gate, the
// reference solution passing every test with -race, and the verify page of
// a certificate lookup.
import { test, expect } from "@playwright/test";
import { confirmEmail, setupLink, siteRulesOnMonaco, solution, virtualAuthenticator, watchErrors, webLog } from "./helpers.js";

test("setup, sign up, solve 01-loglevels, reveal hint 1", async ({ page, context }) => {
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

  // The course waits until the address is verified. The dev server has no
  // mail server, so the page tells the operator where the link is.
  await page.waitForURL("**/check-email");
  await expect(page.getByRole("heading", { name: "Check your email" })).toBeVisible();
  await expect(page.locator("[data-lede]")).toHaveText("We sent a link to ada@example.com. Open it to start the course.");
  await expect(page.getByText("The link works for 48 hours.")).toBeVisible();
  await expect(page.locator(".callout")).toContainText("The link is in the server log");
  await expect(page.getByRole("link", { name: "Change the address" })).toHaveAttribute("href", "/account");
  await page.goto("/learn");
  await page.waitForURL("**/check-email");

  // A new link replaces the first one; the status line says so.
  const since = webLog().length;
  await page.getByRole("button", { name: "Send the link again" }).click();
  await expect(page.locator("[data-status]")).toHaveText("We sent a new link to ada@example.com. The link before it does not work any more.");
  await confirmEmail(page, "ada@example.com", { since });
  await page.getByRole("link", { name: "Open your course" }).click();
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
  await page.goto("/learn/01-loglevels");
  await expect(page.locator(".ide")).toHaveAttribute("data-ready", "true", { timeout: 60_000 });
  await expect(page.locator("[data-file-tabs] [role=tab] .file-name").first()).toHaveText("loglevels.go");

  // Monaco's widgets keep their own look: no site class is a Monaco class
  // name, and no site rule matches the open suggest list, find widget,
  // tooltip or problem hover. None of these steps changes the code.
  await page.evaluate(() => {
    const ed = window.monaco.editor.getEditors()[0];
    const m = ed.getModel();
    const line = m.getLinesContent().findIndex((l) => l.startsWith("package ")) + 1;
    ed.setPosition({ lineNumber: line, column: m.getLineMaxColumn(line) });
    ed.focus();
  });
  await page.keyboard.press("Control+Space");
  const suggestion = page.locator(".suggest-widget .monaco-list-row").first();
  await expect(suggestion.locator(".label-name")).not.toBeEmpty();
  // The label sits inside its row, which hides what overflows.
  expect(await suggestion.evaluate((row) => {
    const r = row.getBoundingClientRect();
    const l = row.querySelector(".label-name").getBoundingClientRect();
    return l.top >= r.top && l.bottom <= r.bottom;
  })).toBe(true);
  expect(await siteRulesOnMonaco(page)).toEqual([]);
  await page.keyboard.press("Escape");
  await page.keyboard.press("Control+f");
  await expect(page.locator(".find-widget")).toBeVisible();
  expect(await siteRulesOnMonaco(page)).toEqual([]);
  // A tooltip is a context view next to the editor, outside .monaco-editor.
  await page.locator(".find-widget .monaco-custom-toggle").first().hover();
  await expect(page.locator(".context-view .monaco-hover")).toBeVisible();
  expect(await siteRulesOnMonaco(page)).toEqual([]);
  await page.keyboard.press("Escape");
  await expect(page.locator(".find-widget.visible")).toHaveCount(0);
  // The hover of a problem has Monaco's own status bar.
  const marked = await page.evaluate(() => {
    const ed = window.monaco.editor.getEditors()[0];
    const m = ed.getModel();
    const line = m.getLinesContent().findIndex((l) => l.startsWith("package ")) + 1;
    window.monaco.editor.setModelMarkers(m, "e2e", [
      { severity: window.monaco.MarkerSeverity.Error, startLineNumber: line, startColumn: 1, endLineNumber: line, endColumn: m.getLineMaxColumn(line), message: "e2e" },
    ]);
    ed.revealLineInCenter(line);
    const p = ed.getScrolledVisiblePosition({ lineNumber: line, column: 3 });
    const box = ed.getDomNode().getBoundingClientRect();
    return { x: box.left + p.left, y: box.top + p.top + p.height / 2 };
  });
  await page.mouse.move(marked.x, marked.y);
  await expect(page.locator(".monaco-hover .hover-row.status-bar")).toBeVisible();
  expect(await siteRulesOnMonaco(page)).toEqual([]);
  await page.evaluate(() => window.monaco.editor.setModelMarkers(window.monaco.editor.getEditors()[0].getModel(), "e2e", []));
  await page.mouse.move(0, 0);

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
  const files = solution("01-loglevels");
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
  await expect(page.getByRole("heading", { name: "Tier 1 · Badge In" })).toBeVisible();
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
  await page.goto("/learn/02-repeated");
  await expect(page.locator(".ide")).toHaveAttribute("data-ready", "true", { timeout: 60_000 });
  await expect(page.locator("[data-race]")).toBeDisabled();
  await expect(page.locator("[data-race-status]")).toHaveText("-race not available");
  await expect(page.locator("[data-run-notes]")).toContainText("The race detector is not available on this server. Runs go without -race.");
  await page.unrouteAll();

  // Code that a closed tab could not save stays on offer through an edit and
  // a successful save, until the learner restores or discards it.
  const key = "gpw-unsaved:02-repeated:repeated.go";
  const kept = "package repeated\n\n// kept by this browser\n";
  await page.evaluate(([k, content]) => localStorage.setItem(k, JSON.stringify({ at: Date.now(), content })), [key, kept]);
  await page.reload();
  await expect(page.locator(".ide")).toHaveAttribute("data-ready", "true", { timeout: 60_000 });
  const banner = page.locator("[data-save-banner]");
  await expect(banner).toContainText("This browser kept changes to repeated.go");
  await page.evaluate(() => {
    const m = window.monaco.editor.getModels().find((m) => m.uri.path.endsWith("/repeated.go"));
    m.setValue(m.getValue() + "\n// an edit\n");
  });
  await expect(page.locator("[data-save-status]")).toContainText("Saved", { timeout: 20_000 });
  await expect(banner).toContainText("This browser kept changes to repeated.go");
  // The offered copy waits under its own key; the live backup is free for new edits.
  const offered = "gpw-offered:02-repeated:repeated.go";
  expect(JSON.parse(await page.evaluate((k) => localStorage.getItem(k), offered)).map((c) => c.content)).toEqual([kept]);
  await banner.getByRole("button", { name: "Restore them" }).click();
  await expect(banner).toBeHidden();
  expect(await page.evaluate((k) => localStorage.getItem(k), offered)).toBeNull();
  expect(await page.evaluate(() => window.monaco.editor.getModels().find((m) => m.uri.path.endsWith("/repeated.go")).getValue())).toBe(kept);
  // The restore is saved like any edit: the live backup goes once the server has it.
  await expect.poll(() => page.evaluate((k) => localStorage.getItem(k), key), { timeout: 20_000 }).toBeNull();

  // A copy the learner never acted on stays behind a newer one: the newest
  // is offered first, then the older one.
  const older = "package repeated\n\n// older copy\n";
  const newer = "package repeated\n\n// newer copy\n";
  await page.evaluate(([o, k, a, b]) => {
    localStorage.setItem(o, JSON.stringify([{ at: 1, content: a }]));
    localStorage.setItem(k, JSON.stringify({ at: 2, content: b }));
  }, [offered, key, older, newer]);
  await page.reload();
  await expect(page.locator(".ide")).toHaveAttribute("data-ready", "true", { timeout: 60_000 });
  await expect(banner).toContainText("1 older copy waits after this one.");
  await banner.getByRole("button", { name: "discard them" }).click();
  await expect(banner).toContainText("This browser kept changes to repeated.go");
  await expect(banner).not.toContainText("older copy");
  await banner.getByRole("button", { name: "Restore them" }).click();
  await expect(banner).toBeHidden();
  expect(await page.evaluate(() => window.monaco.editor.getModels().find((m) => m.uri.path.endsWith("/repeated.go")).getValue())).toBe(older);
  await expect.poll(() => page.evaluate((k) => localStorage.getItem(k), key), { timeout: 20_000 }).toBeNull();

  // A tab that was open while the course grew kept its code under the slug
  // of the first course. The old link redirects, and the page of the new
  // slug offers that code and takes over the cursor position.
  const oldKey = "gpw-unsaved:07-retry:retry.go";
  const oldCursor = "gpw-cursor:07-retry:retry.go";
  const moved = "package retry\n\n// kept under the old slug\n";
  await page.evaluate(([k, c, content]) => {
    localStorage.setItem(k, JSON.stringify({ at: Date.now(), content }));
    localStorage.setItem(c, "3:1");
  }, [oldKey, oldCursor, moved]);
  await page.goto("/learn/07-retry");
  await expect(page).toHaveURL(/\/learn\/32-retry$/);
  await expect(page.locator(".ide")).toHaveAttribute("data-ready", "true", { timeout: 60_000 });
  await expect(banner).toContainText("This browser kept changes to retry.go");
  const kept32 = await page.evaluate(() => ({
    unsaved: localStorage.getItem("gpw-unsaved:07-retry:retry.go"),
    cursor: localStorage.getItem("gpw-cursor:07-retry:retry.go"),
    offered: JSON.parse(localStorage.getItem("gpw-offered:32-retry:retry.go") || "[]").map((c) => c.content),
  }));
  expect(kept32).toEqual({ unsaved: null, cursor: null, offered: [moved] });
  await expect(page.locator("[data-cursor]")).toHaveText("Line 3, column 1");
  await banner.getByRole("button", { name: "discard them" }).click();
  await expect(banner).toBeHidden();

  expect(errors).toEqual([]);
});
