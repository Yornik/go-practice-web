// The smoke test of docs/DESIGN.md, "Testing": the setup link, sign-up,
// the email gate until the address is verified, sign-out and sign-in with
// a passkey, the IDE on 01-loglevels with hint 1 behind its soft gate, the
// reference solution passing every test with -race, and the verify page of
// a certificate lookup.
import { test, expect } from "@playwright/test";
import { confirmEmail, editorRingGaps, paperProblems, passAll, setActiveSeconds, setupLink, siteRulesOnMonaco, solution, virtualAuthenticator, watchErrors, webLog } from "./helpers.js";

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
  await expect(page.getByRole("heading", { name: "Your page of the Ledger" })).toBeVisible();
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
  // No bracket-pair colours (11.3): Monaco takes them from each model.
  await expect(page.locator('.monaco-editor [class*="bracket-highlighting-"]')).toHaveCount(0);

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

  // The layout (".design/VESPER.md" 7.1, 7.10): from 900 px the document is
  // the window and never scrolls, the Keys button stays inside it, and from
  // 1280 px the editor holds 80 columns or more.
  const layout = () => page.evaluate(() => {
    const ed = window.monaco.editor.getEditors()[0];
    const keys = document.querySelector(".sb-button").getBoundingClientRect();
    const ch = ed.getOption(window.monaco.editor.EditorOption.fontInfo).typicalHalfwidthCharacterWidth;
    return {
      fits: document.documentElement.scrollHeight === innerHeight && document.documentElement.scrollWidth === innerWidth,
      keys: keys.left >= 0 && keys.right <= innerWidth && keys.bottom <= innerHeight,
      wide: innerWidth < 1280 || Math.floor(ed.getLayoutInfo().contentWidth / ch) >= 80,
    };
  });
  for (const [width, height] of [[1600, 900], [1366, 768], [1280, 800], [1024, 768], [900, 800]]) {
    await page.setViewportSize({ width, height });
    await expect.poll(layout, { message: `layout at ${width} by ${height}` }).toEqual({ fits: true, keys: true, wide: true });
  }
  await page.setViewportSize({ width: 1280, height: 720 });

  // "Skip to the editor" puts the focus in the editor, and its 3 px ring
  // shows on all 4 sides, over Monaco (VESPER.md 3.10, 15).
  await page.getByRole("link", { name: "Skip to the editor" }).focus();
  await page.keyboard.press("Enter");
  await expect.poll(() => page.evaluate(() => !!document.activeElement?.closest("[data-editor]"))).toBe(true);
  expect(await editorRingGaps(page)).toEqual([]);

  // HUSH's card (16.7), forced on by its test hook: it takes no focus; its
  // link scrolls only the problems pane; Dismiss gives the focus to the
  // Problems heading.
  await page.evaluate(() => document.dispatchEvent(new CustomEvent("gpw:hush-show")));
  const hush = page.locator("#hush");
  await expect(hush).toBeVisible();
  await expect(hush).toContainText("REFUSED: RULE 1");
  await expect(hush).not.toBeFocused();
  await page.getByRole("link", { name: "A note from HUSH" }).click();
  await expect(hush).toBeFocused();
  expect(await page.evaluate(() => document.documentElement.scrollTop + document.body.scrollTop)).toBe(0);
  await hush.getByRole("button", { name: "Dismiss" }).click();
  await expect(hush).toBeHidden();
  await expect(page.getByRole("link", { name: "A note from HUSH" })).toBeHidden();
  await expect(page.locator("#problems-title")).toBeFocused();

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

  // HUSH's card by its 7 conditions (16.7), not by the hook: 25 active
  // minutes, a failing run, no run going, 5 s without a key, story effects
  // on, once per exercise, never after a pass.
  const ready = () => expect(page.locator(".ide")).toHaveAttribute("data-ready", "true", { timeout: 60_000 });
  const runStep1 = async () => {
    await page.locator("[data-step-select]").selectOption("step-1");
    await page.getByRole("button", { name: "Run tests", exact: true }).click();
  };
  const ranAt = page.locator("[data-run-summary]");
  const ideStory = page.locator(".ide-top [data-story-toggle]");
  const fresh = async ({ story }) => {
    await page.evaluate((story) => {
      localStorage.removeItem("gpw-hush-seen");
      if (story) localStorage.removeItem("gpw-story");
      else localStorage.setItem("gpw-story", "off");
    }, story);
    await page.reload();
    await ready();
  };
  const typeInEditor = () => page.evaluate(() => {
    const ed = window.monaco.editor.getEditors()[0];
    const m = ed.getModel();
    ed.focus();
    ed.setPosition({ lineNumber: m.getLineCount(), column: m.getLineMaxColumn(m.getLineCount()) });
    ed.trigger("keyboard", "type", { text: "\n// " });
  });
  if (await setActiveSeconds("ada", "01-loglevels", 1600)) {
    await page.reload();
    await ready();
    await expect(hush).toBeHidden(); // no run yet
    await runStep1();
    await expect(ranAt).toContainText(/ at \d\d:\d\d/, { timeout: 180_000 });
    await expect(hush).toBeVisible();
    expect(JSON.parse(await page.evaluate(() => localStorage.getItem("gpw-hush-seen")))).toEqual(["01-loglevels"]);

    // A problem the check adds renumbers the run's problem; the tree's
    // pins and "Show problem N" follow.
    const pins = () => page.locator(".test-tree .tree-row .balloon").allInnerTexts();
    const before = await pins();
    expect(before.length).toBeGreaterThan(0);
    await page.evaluate(() => {
      const m = window.monaco.editor.getEditors()[0].getModel();
      m.setValue(`${m.getValue()}\nvar _ = undefinedName\n`);
    });
    await expect(page.locator(".chip", { hasText: "Compiler" })).toBeVisible({ timeout: 60_000 });
    await expect.poll(pins).toEqual(before.map((n) => String(Number(n) + 1)));
    const shown = await page.locator("[data-test-detail] button", { hasText: "Show problem" }).innerText();
    expect(shown.toLowerCase()).toBe(`show problem ${Number(before[0]) + 1}`);
    await page.evaluate(() => {
      const m = window.monaco.editor.getEditors()[0].getModel();
      m.setValue(m.getValue().replace("\nvar _ = undefinedName\n", ""));
    });
    await expect.poll(pins, { timeout: 60_000 }).toEqual(before);

    // Once per exercise: a reload does not bring it back.
    await page.reload();
    await ready();
    await expect(hush).toBeHidden();
    // Story effects off at load hide it; turning them on shows it.
    await fresh({ story: false });
    await expect(hush).toBeHidden();
    await ideStory.click();
    await expect(hush).toBeVisible();
    // Typing: it waits for 5 s without a key.
    await fresh({ story: false });
    await typeInEditor();
    await ideStory.click();
    await page.waitForTimeout(1500);
    await expect(hush).toBeHidden();
    await expect(hush).toBeVisible({ timeout: 10_000 });
    // A run cancels the wait; the card comes after the failing run ends.
    await fresh({ story: false });
    await typeInEditor();
    await ideStory.click();
    await runStep1();
    await expect(hush).toBeHidden();
    await expect(ranAt).toContainText(/ at \d\d:\d\d/, { timeout: 180_000 });
    await expect(hush).toBeVisible({ timeout: 10_000 });
    await page.evaluate(() => localStorage.removeItem("gpw-hush-seen"));
  }

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
  // The first pass's story line (16.8: 01 takes line 1), and its plain twin with story effects off.
  const passLine = page.locator("[data-run-notes] .pass-ok");
  await expect(passLine).toHaveText("All green, and this time it is true.", { useInnerText: true });
  await page.getByRole("button", { name: /^Story effects/ }).click();
  await expect(passLine).toHaveText("All tests pass.", { useInnerText: true });
  await page.getByRole("button", { name: /^Story effects/ }).click();
  await expect(page.locator("[data-cell-passed]")).toBeVisible();
  await expect(page.locator('.test-node[data-state="pass"]').first()).toBeVisible();
  // The tree's keys keep the focused row in view (WCAG 2.4.7): an item
  // holds its whole subtree, so focusing it alone does not scroll.
  await page.setViewportSize({ width: 1280, height: 560 });
  await page.locator('.test-tree [role="treeitem"][tabindex="0"]').focus();
  for (const key of ["End", "Home", "End", "ArrowUp", "Home"]) {
    await page.keyboard.press(key);
    const seen = await page.evaluate(() => {
      const row = document.activeElement.querySelector(":scope > .tree-row").getBoundingClientRect();
      let box = { top: 0, bottom: innerHeight };
      for (let a = document.activeElement.parentElement; a; a = a.parentElement) {
        if (/(auto|scroll)/.test(getComputedStyle(a).overflowY)) {
          const r = a.getBoundingClientRect();
          box = { top: Math.max(r.top, 0), bottom: Math.min(r.bottom, innerHeight) };
          break;
        }
      }
      return row.top >= box.top - 1 && row.bottom <= box.bottom + 1;
    });
    expect(seen, `the focused row after ${key}`).toBe(true);
  }
  await page.setViewportSize({ width: 1280, height: 720 });
  // After a pass HUSH's card never comes, whatever else holds.
  await page.evaluate(() => localStorage.removeItem("gpw-hush-seen"));
  await page.reload();
  await expect(page.locator(".ide")).toHaveAttribute("data-ready", "true", { timeout: 60_000 });
  await expect(hush).toBeHidden();

  // The dashboard counts it.
  await page.goto("/learn");
  await expect(page.locator(".gpw-bignum .gpw-vh")).toHaveText("1 of 50 tools is in the Ledger.");

  // Certificates: nothing to claim yet; the verify lookup works.
  await page.goto("/certificates");
  await expect(page.getByRole("heading", { name: "Tier 1, Badge In" })).toBeVisible();
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
  // The keys hold the account: another account's copy on the same browser
  // waits for its owner and is never offered here.
  const user = await page.locator(".ide").getAttribute("data-user");
  const key = `gpw-unsaved:${user}:02-repeated:repeated.go`;
  const otherKey = "gpw-unsaved:999999:02-repeated:repeated.go";
  const kept = "package repeated\n\n// kept by this browser\n";
  const theirs = "package repeated\n\n// another account's code\n";
  await page.evaluate(([k, content, o, other]) => {
    localStorage.setItem(o, JSON.stringify({ at: Date.now() + 1000, content: other }));
    localStorage.setItem(k, JSON.stringify({ at: Date.now(), content }));
  }, [key, kept, otherKey, theirs]);
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
  const offered = `gpw-offered:${user}:02-repeated:repeated.go`;
  expect(JSON.parse(await page.evaluate((k) => localStorage.getItem(k), offered)).map((c) => c.content)).toEqual([kept]);
  await banner.getByRole("button", { name: "Restore them" }).click();
  await expect(banner).toBeHidden();
  expect(await page.evaluate((k) => localStorage.getItem(k), offered)).toBeNull();
  expect(await page.evaluate(() => window.monaco.editor.getModels().find((m) => m.uri.path.endsWith("/repeated.go")).getValue())).toBe(kept);
  expect(JSON.parse(await page.evaluate((k) => localStorage.getItem(k), otherKey)).content).toBe(theirs);
  await page.evaluate((k) => localStorage.removeItem(k), otherKey);
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
  // of the first course, in a build that kept no account in the key. The
  // old link redirects, and the page of the new slug offers that code and
  // takes over the cursor position.
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
    offered: JSON.parse(localStorage.getItem(`gpw-offered:${document.querySelector(".ide").dataset.user}:32-retry:retry.go`) || "[]").map((c) => c.content),
  }));
  expect(kept32).toEqual({ unsaved: null, cursor: null, offered: [moved] });
  await expect(page.locator("[data-cursor]")).toHaveText("Line 3, column 1");
  await banner.getByRole("button", { name: "discard them" }).click();
  await expect(banner).toBeHidden();

  // The course page (VESPER.md 8.6): the on-call exercise's tier is open,
  // and a folded tier opens its table with "Show".
  await page.goto("/learn");
  await expect(page.locator("#tier-7-log")).toBeVisible();
  await expect(page.locator("#tier-2-log")).toBeHidden();
  await page.getByRole("button", { name: "Show tier 2", exact: true }).click();
  await expect(page.locator("#tier-2-log")).toBeVisible();
  await expect(page.getByRole("button", { name: "Hide tier 2", exact: true })).toHaveAttribute("aria-expanded", "true");

  // The account page's Story effects checkbox is the header's toggle (16.5).
  await page.goto("/account");
  const storyCheck = page.getByRole("checkbox", { name: /Show story effects/ });
  const storyToggle = page.locator(".site-head-in [data-story-toggle]");
  await expect(storyCheck).toBeChecked();
  await storyCheck.uncheck();
  await expect(page.locator("html")).toHaveAttribute("data-story", "off");
  await expect(storyToggle).toHaveAttribute("aria-pressed", "false");
  await storyToggle.click();
  await expect(storyCheck).toBeChecked();
  await expect(page.locator("html")).not.toHaveAttribute("data-story", "off");

  // With the menu sheet open, the first Escape in a dialog closes the
  // dialog, not the sheet (VESPER.md 15).
  await page.setViewportSize({ width: 1000, height: 800 });
  await page.getByRole("button", { name: "Menu", exact: true }).click();
  await page.getByRole("button", { name: "Sign out everywhere" }).click();
  await expect(page.locator("dialog[open]")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.locator("dialog[open]")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Sign out everywhere" })).toBeFocused();
  await expect(page.locator("[data-nav-sheet]")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.locator("[data-nav-sheet]")).toBeHidden();

  // No page of the learner scrolls sideways at 320 px.
  await page.setViewportSize({ width: 320, height: 720 });
  for (const path of ["/learn", "/certificates", "/account", "/admin", "/verify"]) {
    await page.goto(path);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), path).toBe(true);
  }

  // A wide table scrolls in its box, and the box is the focusable region:
  // the table keeps its role and its column headers (WCAG 1.3.1). 09's
  // table of statuses is wider than the spec column at 1024 px.
  await page.setViewportSize({ width: 1024, height: 800 });
  await page.goto("/learn/09-statuspage");
  await expect(page.locator(".ide")).toHaveAttribute("data-ready", "true", { timeout: 60_000 });
  const box = page.locator(".gpw-tscroll[role=region]").first();
  await expect(box).toHaveAttribute("tabindex", "0");
  await expect(box).toHaveAttribute("aria-label", /^Table: /);
  expect(await box.locator("table").getAttribute("role")).toBeNull();
  const cdp = await page.context().newCDPSession(page);
  const { nodes } = await cdp.send("Accessibility.getFullAXTree");
  const roles = new Set(nodes.filter((n) => !n.ignored).map((n) => n.role?.value));
  expect(roles.has("table") && roles.has("columnheader")).toBe(true);

  // A course certificate's verify page keeps its paper and its activity in
  // their boxes at every width: its parts follow their own width, not the
  // window's. 90 min per exercise makes values of hours and minutes.
  if (await passAll("ada", 5400)) {
    await page.goto("/certificates");
    const issued = await page.evaluate(() =>
      fetch("/api/certificates", { method: "POST", headers: { "X-GPW-CSRF": "1", "Content-Type": "application/json" }, body: JSON.stringify({ kind: "course", showStats: true }) }).then((r) => r.json()),
    );
    for (const width of [360, 721, 768, 960, 1101, 1180, 1280]) {
      await page.setViewportSize({ width, height: 800 });
      await page.goto(issued.url);
      await page.locator("details.gpw-disc2").evaluate((d) => { d.open = true; });
      expect(await page.evaluate(paperProblems), `the verify page at ${width} px`).toEqual([]);
    }
  }

  expect(errors).toEqual([]);
});
