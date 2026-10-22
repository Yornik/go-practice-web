// Screenshots of the pages of the Vesper v2 design (".design/VESPER.md"
// 14.3 and 17.2), with the checks of its tools/shoot.mjs --check. It runs
// only when SCREENSHOTS_DIR is set, and only on its own, because it uses
// the setup link:
//
//   SCREENSHOTS_DIR=screenshots npx playwright test screenshots
//
// writes e2e/screenshots/, which .gitignore keeps out. Each page is shot
// at 1440 by 900 and 390 by 844 (SCREENSHOTS_SIZES="1280x800,360x780"
// picks other sizes, such as those of the frames' shots), light and dark,
// with story effects on and off: <page>-<width>-<day|night>[-story-off].png.
// The dashboard, the certificates page and the IDE are also shot once in
// forced colours.
//
// The learner Ada passes tier 1 with the course's reference solutions for
// a real certificate, then fails a test of 07 and types a compile error:
// the IDE of the frame, with HUSH's card through its test hook. The text
// of a revealed hint is replaced before every shot, so the pictures do not
// spoil the course.
//
// Every shot is also checked: no horizontal scroll, no page error or CSP
// violation, the skyline's plates clear of towers and of each other, a
// tier's count in the weight of its line, every 0 in Saira slashed, and on
// the IDE no site rule that reaches Monaco. With story effects on and
// dark, a tab walk asserts that no focus ring is cut by the window or by
// an ancestor that clips (VESPER.md 3.10).
import { test, expect } from "@playwright/test";
import { spawn } from "node:child_process";
import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { confirmEmail, editorRingGaps, mailLink, ovalZeros, repo, setupLink, siteRulesOnMonaco, solution, verificationLink, virtualAuthenticator, watchErrors, webLog } from "./helpers.js";

const dir = process.env.SCREENSHOTS_DIR && path.resolve(process.env.SCREENSHOTS_DIR);
test.skip(!dir, "set SCREENSHOTS_DIR to take the screenshots");
test.setTimeout(90 * 60_000);
// A missing button fails the run at once, not after the whole timeout.
test.use({ actionTimeout: 30_000 });

const SIZES = (process.env.SCREENSHOTS_SIZES || "1440x900,390x844").split(",").map((s) => {
  const [width, height] = s.trim().split("x").map(Number);
  return { width, height, phone: width < 720 };
});
const THEMES = [
  ["light", "day"],
  ["dark", "night"],
];
const STORIES = ["on", "off"];

const problems = [];
const errors = [];

/** The variants of 1 page: every size, theme and story setting. */
function* variants({ sizes = SIZES, stories = STORIES } = {}) {
  for (const size of sizes) for (const [scheme, tag] of THEMES) for (const story of stories) yield { size, scheme, tag, story };
}

/** A context for 1 variant. Story effects off is the browser's stored choice (VESPER.md 16.5). */
async function open(browser, { size, scheme, story }, { state, forced } = {}) {
  const ctx = await browser.newContext({
    viewport: { width: size.width, height: size.height },
    colorScheme: scheme,
    reducedMotion: "reduce",
    forcedColors: forced ? "active" : "none",
    storageState: state,
  });
  if (story === "off") {
    await ctx.addInitScript(() => {
      try {
        localStorage.setItem("gpw-story", "off");
      } catch {
        // A page without storage keeps story effects on; the shot shows it.
      }
    });
  }
  const page = await ctx.newPage();
  return { ctx, page, errs: watchErrors(page) };
}

const fileName = (name, { size, tag, story }) => `${name}-${size.width}-${tag}${story === "off" ? "-story-off" : ""}.png`;

/** Opens a page in every variant, shoots it and checks it. */
async function shoot(browser, name, url, { state, prepare, full = true, sizes, stories, walk = true } = {}) {
  for (const v of variants({ sizes, stories })) {
    const { ctx, page, errs } = await open(browser, v, { state });
    await page.goto(url);
    await page.evaluate(() => document.fonts.ready);
    if (prepare) await prepare(page, v);
    await page.waitForTimeout(300);
    const file = fileName(name, v);
    await page.screenshot({ path: path.join(dir, file), fullPage: full });
    await check(page, file, { walk: walk && v.story === "on" && v.scheme === "dark" });
    errors.push(...errs.map((e) => `${file}: ${e}`));
    await ctx.close();
  }
}

/** The checks of tools/shoot.mjs --check on the page as it is. */
async function check(page, file, { walk, ide = false }) {
  const found = [];
  const wide = await page.evaluate(() => {
    const vw = document.documentElement.clientWidth;
    if (document.documentElement.scrollWidth <= vw) return [];
    return [...document.querySelectorAll("body *")]
      .filter((el) => el.getBoundingClientRect().right > vw + 0.5 && el.getBoundingClientRect().width > 0)
      .slice(0, 6)
      .map((el) => `${el.tagName.toLowerCase()}.${[...el.classList].join(".")}`);
  });
  if (wide.length) found.push(`horizontal scroll: ${wide.join(", ")}`);
  found.push(...(await page.evaluate(skylinePlates)));
  found.push(...(await page.evaluate(tierCounts)));
  found.push(...(await ovalZeros(page)).map((z) => `0 without its slash: ${z}`));
  if (ide) found.push(...(await siteRulesOnMonaco(page)).map((s) => `site rule on Monaco: ${s}`));
  if (ide) found.push(...(await page.evaluate(treePins)));
  if (walk) found.push(...(await tabWalk(page)));
  problems.push(...found.map((p) => `${file}: ${p}`));
}

/**
 * Tabs through the page and returns every focus ring that the window or a
 * clipping ancestor cuts. In the IDE the walk stops at the editor, where
 * Tab types.
 */
async function tabWalk(page) {
  const out = [];
  const seen = new Set();
  await page.evaluate(() => {
    document.activeElement?.blur();
    window.scrollTo(0, 0);
  });
  for (let i = 0; i < 200; i++) {
    await page.keyboard.press("Tab");
    const r = await page.evaluate(ringCut);
    if (!r || seen.has(r.key)) break;
    seen.add(r.key);
    if (r.cut) out.push(`focus ring cut: ${r.cut}`);
    if (r.editor) {
      // Computed style says nothing here: Monaco covers the host. Read pixels.
      out.push(...(await editorRingGaps(page)).map((g) => `editor ring: ${g}`));
      break;
    }
  }
  return out;
}

// Runs in the page: the focused element's ring and every box that clips it.
function ringCut() {
  const el = document.activeElement;
  if (!el || el === document.body) return null;
  const key = el.outerHTML.slice(0, 160) + el.getBoundingClientRect().top;
  const editor = !!el.closest(".monaco-editor, [data-editor]");
  const cs = getComputedStyle(el);
  const r = el.getBoundingClientRect();
  if (cs.outlineStyle === "none" || parseFloat(cs.outlineWidth) === 0 || (!r.width && !r.height)) return { key, editor };
  const grow = Math.max(parseFloat(cs.outlineOffset) + parseFloat(cs.outlineWidth), 0);
  const ring = { l: r.left - grow, t: r.top - grow, r: r.right + grow, b: r.bottom + grow };
  const name = `${el.tagName.toLowerCase()}.${[...el.classList].join(".")} "${(el.getAttribute("aria-label") || el.textContent || "").trim().slice(0, 28)}"`;
  const vw = document.documentElement.clientWidth;
  const vh = document.documentElement.clientHeight;
  const cut = [];
  if (ring.l < -0.5 || ring.r > vw + 0.5) cut.push(`window x ${ring.l.toFixed(1)}..${ring.r.toFixed(1)}`);
  if (ring.t < -0.5 || ring.b > vh + 0.5) cut.push(`window y ${ring.t.toFixed(1)}..${ring.b.toFixed(1)}`);
  for (let a = el.parentElement; a && a !== document.documentElement; a = a.parentElement) {
    const s = getComputedStyle(a);
    if (s.display === "contents") continue;
    if (s.overflowX === "visible" && s.overflowY === "visible") continue;
    if (a === document.body) continue;
    const ar = a.getBoundingClientRect();
    const box = { l: ar.left + a.clientLeft, t: ar.top + a.clientTop, r: ar.left + a.clientLeft + a.clientWidth, b: ar.top + a.clientTop + a.clientHeight };
    const sides = [ring.l < box.l - 0.5 && "left", ring.r > box.r + 0.5 && "right", ring.t < box.t - 0.5 && "top", ring.b > box.b + 0.5 && "bottom"].filter(Boolean);
    if (sides.length) cut.push(`${a.tagName.toLowerCase()}.${[...a.classList].join(".")} ${sides.join("+")}`);
  }
  return { key, editor, cut: cut.length ? `${name}: ${cut.join("; ")}` : "" };
}

// Runs in the page: no tier plate of a skyline meets a tower, another
// plate or its year's label, or leaves its year (VESPER.md 6.15).
function skylinePlates() {
  const out = [];
  const R = (el) => el.getBoundingClientRect();
  const hit = (a, b, pad = 0) => a.left < b.right + pad && b.left < a.right + pad && a.top < b.bottom + pad && b.top < a.bottom + pad;
  const tower = (tw) => {
    const r = R(tw);
    const roof = tw.classList.contains("gpw-tw-roof") ? 7 : 0;
    return { left: r.left, right: r.right, top: r.top - roof, bottom: r.bottom };
  };
  for (const sky of document.querySelectorAll(".gpw-sky")) {
    if (!sky.getClientRects().length) continue;
    const plates = [...sky.querySelectorAll(".gpw-sky-plate")].filter((p) => p.getClientRects().length);
    for (const p of plates) {
      const pr = R(p);
      const id = p.textContent.trim().slice(0, 12);
      for (const tw of sky.querySelectorAll(".gpw-tw")) if (hit(pr, tower(tw))) out.push(`skyline: ${id} plate meets a tower`);
      for (const q of plates) if (q !== p && hit(pr, R(q), 2)) out.push(`skyline: ${id} plate meets another plate`);
      const yr = p.closest(".gpw-sky-yr");
      const row = yr?.querySelector(".gpw-sky-row");
      if (row && (pr.left < R(row).left - 0.5 || pr.right > R(row).right + 0.5)) out.push(`skyline: ${id} plate leaves its year`);
      const yl = yr?.querySelector(".gpw-sky-yl");
      if (yl && yl.getClientRects().length && hit(pr, R(yl), 2)) out.push(`skyline: ${id} plate meets the year label`);
    }
  }
  return [...new Set(out)];
}

// Runs in the page: each pin in the test tree is the number of the chip
// of its test's problem, and the output's "Show problem N" is the
// selected test's pin. The check renumbers the problems as you type.
function treePins() {
  const out = [];
  for (const li of document.querySelectorAll('.test-tree [role="treeitem"][data-test]')) {
    const pin = li.querySelector(":scope > .tree-row .balloon")?.textContent.trim();
    if (!pin) continue;
    const chip = document.querySelector(`.chip[data-n="${pin}"]`);
    const top = li.dataset.test.split("/")[0];
    if (chip && !chip.title.includes(top)) out.push(`tree pin ${pin} of ${li.dataset.test} names chip "${chip.title}"`);
    const show = li.querySelector("[data-test-detail] button")?.textContent.trim();
    if (show && show.toLowerCase() !== `show problem ${pin}`) out.push(`"${show}" under ${li.dataset.test}, whose pin is ${pin}`);
  }
  return out;
}

// Runs in the page: a tier's count is set like its line, Saira 600 in ink;
// only the line under it is quiet (VESPER.md 8.6). The slashed-zero span
// round the count must not take the quiet style.
function tierCounts() {
  const out = [];
  for (const p of document.querySelectorAll(".gpw-tierbar .gpw-prog")) {
    const walk = document.createTreeWalker(p, NodeFilter.SHOW_TEXT, { acceptNode: (n) => (n.data.trim() ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_SKIP) });
    const first = walk.nextNode();
    if (!first) continue;
    const got = getComputedStyle(first.parentElement);
    const want = getComputedStyle(p);
    if (got.fontWeight !== want.fontWeight || got.color !== want.color) {
      out.push(`tier count "${p.textContent.trim().slice(0, 24)}" is ${got.fontWeight} ${got.color}, its line ${want.fontWeight} ${want.color}`);
    }
  }
  return out;
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

/** Sets the body of ParseHeartbeat in 07's rollcall.go through Monaco's model. */
async function setParse(page, body) {
  await page.evaluate((body) => {
    const m = window.monaco.editor.getModels().find((x) => x.uri.path.endsWith("/rollcall.go"));
    const re = /func ParseHeartbeat\(line string\) \(Heartbeat, error\) \{\n[\s\S]*?\n\}/;
    m.setValue(m.getValue().replace(re, `func ParseHeartbeat(line string) (Heartbeat, error) {\n${body}\n}`));
  }, body);
  await expect(page.locator("[data-save-status]")).toContainText("Saved", { timeout: 20_000 });
}

const ready = (page) => expect(page.locator(".ide")).toHaveAttribute("data-ready", "true", { timeout: 60_000 });

// The site allows 10 test runs a minute per learner; the shots run more.
let lastRun = 0;
async function runTests(page, step) {
  await page.locator("[data-step-select]").selectOption(step);
  const wait = lastRun + 7_000 - Date.now();
  if (wait > 0) await page.waitForTimeout(wait);
  lastRun = Date.now();
  await page.getByRole("button", { name: "Run tests", exact: true }).click();
}

// 07's ParseHeartbeat, wrong on purpose: the tests of step 1 fail.
const WRONG = "\treturn Heartbeat{}, nil";
// The same with a compile error typed after the run: rollcall.go:36:12: undefined: strings.
const BROKEN = "\tfields := strings.Fields(line)\n\t// 2 fields: the node and the time. What if a line has 3?\n\t_ = fields\n\treturn Heartbeat{}, nil";

/** Passes an exercise with the course's reference solution. */
async function pass(page, slug) {
  await page.goto(`/learn/${slug}`);
  await ready(page);
  const files = solution(slug);
  await page.evaluate((files) => {
    for (const m of window.monaco.editor.getModels()) {
      const name = m.uri.path.split("/").pop();
      if (name in files) m.setValue(files[name]);
    }
  }, files);
  await expect(page.locator("[data-save-status]")).toContainText("Saved", { timeout: 20_000 });
  await runTests(page, "all");
  await expect(page.locator("[data-run-notes]")).toContainText("passes with -race", { timeout: 180_000 });
}

/**
 * The IDE of the frame: 07 with a failing run of step 1, hint 1 revealed,
 * a compile error typed after the run, problem 1 selected and HUSH's card
 * through its test hook. A reload shows only the last run's tree, so each
 * page runs the tests itself.
 */
async function ideState(page, v) {
  await ready(page);
  await setParse(page, WRONG);
  await runTests(page, "step-1");
  await expect(page.locator("[data-run-summary]")).toContainText(/ at \d\d:\d\d/, { timeout: 180_000 });
  await setParse(page, BROKEN);
  await expect(page.locator(".chip", { hasText: "Compiler" }).first()).toBeVisible({ timeout: 60_000 });
  // The failures of 1 problem fold under the test that stands for them (7.5).
  await expect(page.locator(".test-tree .tree-more", { hasText: "the same way" }).first()).toBeAttached();
  await hideHintText(page);
  await page.locator('.chip[data-n="1"]').first().click();
  await page.evaluate(() => document.dispatchEvent(new CustomEvent("gpw:hush-show")));
  if (v.size.phone) await page.evaluate(() => window.scrollTo(0, 0));
  await page.waitForTimeout(800);
}

/**
 * A second site from the binary scripts/dev.sh built, with GPW_MOVED_FROM
 * set, for the sign-in page's move note (VESPER.md 8.3). It has its own
 * database; it serves only public pages, so its runner token is a dummy.
 */
async function movedSite() {
  const state = process.env.GO_PRACTICE_DEV_STATE;
  const bin = state && path.join(state, "bin", "web");
  if (process.env.E2E_BASE_URL || !bin || !fs.existsSync(bin)) return null;
  const devPort = Number(process.env.GO_PRACTICE_DEV_PORT || 8080);
  const port = devPort + 2;
  fs.mkdirSync(path.join(state, "moved"), { recursive: true });
  const env = {
    PATH: process.env.PATH,
    HOME: process.env.HOME,
    GPW_ADDR: `127.0.0.1:${port}`,
    GPW_METRICS_ADDR: `127.0.0.1:${port + 1010}`,
    GPW_PUBLIC_URL: `http://localhost:${port}`,
    GPW_DB_PATH: path.join(state, "moved", "go-practice.db"),
    GPW_COURSE_DIR: path.join(repo, "course"),
    GPW_RUNNER_URL: `http://127.0.0.1:${devPort + 1}`,
    GPW_RUNNER_TOKEN: crypto.randomBytes(32).toString("hex"),
    GPW_TRUSTED_PROXIES: "",
    GPW_DEV: "true",
    GPW_MOVED_FROM: "go-learn.party",
  };
  const log = fs.openSync(path.join(state, "moved", "web.log"), "w");
  const child = spawn(bin, [], { env, stdio: ["ignore", log, log] });
  const base = `http://localhost:${port}`;
  const deadline = Date.now() + 60_000;
  for (;;) {
    try {
      if ((await fetch(`http://127.0.0.1:${port}/readyz`)).ok) break;
    } catch {
      // Not listening yet.
    }
    if (Date.now() > deadline || child.exitCode !== null) {
      child.kill();
      throw new Error(`the moved site did not start; see ${path.join(state, "moved", "web.log")}`);
    }
    await new Promise((r) => setTimeout(r, 200));
  }
  return { base, stop: () => child.kill() };
}

/** Signs up through an invite or the setup link in a fresh context, and confirms the address. */
async function signUp(browser, link, { username, name, email }) {
  const ctx = await browser.newContext();
  const page = await ctx.newPage();
  await virtualAuthenticator(ctx, page);
  const since = webLog().length;
  await page.goto(link);
  await page.getByLabel("Username").fill(username);
  await page.getByLabel("Display name").fill(name);
  await page.getByLabel("Email address").fill(email);
  await page.getByRole("button", { name: "Create my account and passkey" }).click();
  await page.waitForURL("**/check-email");
  await confirmEmail(page, email, { since });
  const state = await ctx.storageState();
  await ctx.close();
  return state;
}

test("screenshots of the pages", async ({ browser, page, context }) => {
  fs.mkdirSync(dir, { recursive: true });

  // Public pages, as a visitor.
  await shoot(browser, "landing", "/");
  await shoot(browser, "guide", "/guide");
  await shoot(browser, "gate", "/login");
  await shoot(browser, "recover", "/recover");
  await shoot(browser, "verify-lookup", "/verify");
  await shoot(browser, "verify-unknown", "/verify/GP-0000-0000-0000");
  await shoot(browser, "not-found-visitor", "/no-such-page");

  // The sign-in page and recovery after a move, on a second site.
  const moved = await movedSite();
  if (moved) {
    try {
      await shoot(browser, "gate-moved", `${moved.base}/login`);
      await shoot(browser, "recover-moved", `${moved.base}/recover`);
    } finally {
      moved.stop();
    }
  }

  // The setup page, then the first account. Its address waits for its link.
  const link = setupLink();
  await shoot(browser, "setup", link.pathname + link.search);
  await virtualAuthenticator(context, page);
  await page.goto(link.pathname + link.search);
  await page.getByLabel("Username").fill("ada");
  await page.getByLabel("Display name").fill("Ada Lovelace");
  await page.getByLabel("Email address").fill("ada@example.org");
  await page.getByRole("button", { name: "Create my account and passkey" }).click();
  await page.waitForURL("**/check-email");
  const state = await context.storageState();
  await shoot(browser, "gate-check-email", "/check-email", { state });

  // The link's page before and after "Confirm my address". The answer is
  // stubbed in the shots, so the token stays for the real confirmation.
  const verifyLink = await verificationLink("ada@example.org");
  await shoot(browser, "gate-verify-email", verifyLink);
  await shoot(browser, "gate-verify-email-done", verifyLink, {
    prepare: async (p) => {
      await p.route("**/api/auth/verify-email", (route) => route.fulfill({ json: { email: "ada@example.org" } }));
      await p.getByRole("button", { name: "Confirm my address" }).click();
      await expect(p.getByRole("link", { name: "Open your course" })).toBeVisible();
    },
  });
  await confirmEmail(page, "ada@example.org");
  await shoot(browser, "dashboard-day1", "/learn", { state });

  // An invite for a second learner: the join page, then Grace signs up
  // and meets the 403 of the admin page.
  await page.goto("/admin");
  await page.getByLabel("Email (optional)").fill("grace@example.org");
  await page.getByLabel("Note (optional)").fill("Welcome to the course. Start with 01.");
  await page.getByRole("button", { name: "Create the invite link" }).click();
  // The field fills when the API answers; inputValue does not wait for that.
  await expect(page.locator("[data-invite-link]")).not.toHaveValue("");
  const invite = new URL(await page.locator("[data-invite-link]").inputValue());
  await shoot(browser, "gate-join", invite.pathname + invite.search);
  const grace = await signUp(browser, invite.pathname + invite.search, { username: "grace", name: "Grace Hopper", email: "grace@example.org" });
  await shoot(browser, "forbidden", "/admin", { state: grace });

  // Tier 1 with the reference solutions, then the claim of its tag.
  for (const slug of ["01-loglevels", "02-repeated", "03-pagenoise", "04-waves", "05-envfile"]) await pass(page, slug);
  await shoot(browser, "certificates-ready", "/certificates", { state });
  await shoot(browser, "certificates-claim", "/certificates", {
    state,
    full: false,
    walk: false,
    prepare: async (p) => {
      await p.getByRole("button", { name: "Claim the tier 1 certificate" }).click();
      await expect(p.locator("dialog[open]")).toBeVisible();
    },
  });
  await page.goto("/certificates");
  await page.getByRole("button", { name: "Claim the tier 1 certificate" }).click();
  await page.getByLabel("Show my activity on the certificate and the verify page").check();
  await page.getByRole("button", { name: "Issue the certificate" }).click();
  const verifyFirst = await page.getByRole("link", { name: "Verify page of tier 1", exact: true }).getAttribute("href");

  // Issued again, the first ID shows as revoked.
  await page.getByRole("button", { name: "Issue again of tier 1", exact: true }).click();
  await page.getByRole("button", { name: "Issue the certificate" }).click();
  await expect(page.getByRole("link", { name: "Verify page of tier 1", exact: true })).not.toHaveAttribute("href", verifyFirst);
  const verifyHref = await page.getByRole("link", { name: "Verify page of tier 1", exact: true }).getAttribute("href");

  // Tier 2: a wrong ParseHeartbeat fails step 1 of 07, and hint 1 is
  // revealed early. Each shot of the IDE runs step 1 again and types a
  // compile error after the run.
  await page.goto("/learn/07-rollcall");
  await ready(page);
  await setParse(page, WRONG);
  await runTests(page, "step-1");
  await expect(page.locator("[data-run-summary]")).toContainText(/ at \d\d:\d\d/, { timeout: 180_000 });
  const unfold = page.getByRole("button", { name: "Unfold" });
  if (await unfold.count()) await unfold.click();
  await page.getByRole("button", { name: "Reveal hint 1 now" }).click();
  await page.getByRole("button", { name: "Reveal it" }).click();
  await expect(page.locator('[data-level="1"] [data-hint-body]')).not.toBeEmpty();

  // The IDE in every variant, then at 1366 by 768 and 1100 by 800 (2
  // columns) by night and by day, and the got/want problem.
  for (const v of variants()) {
    const { ctx, page: p, errs } = await open(browser, v, { state });
    await p.goto("/learn/07-rollcall");
    await ideState(p, v);
    const file = fileName("ide", v);
    await p.screenshot({ path: path.join(dir, file), fullPage: v.size.phone });
    await check(p, file, { walk: v.story === "on" && v.scheme === "dark", ide: true });
    // The chip shows the test's name; its name says "Failing test in".
    const failing = p.getByRole("tab", { name: /Failing test/ }).first();
    if (!v.size.phone && v.story === "on" && (await failing.count())) {
      await failing.click();
      await p.waitForTimeout(500);
      const gw = fileName("ide-gotwant", v);
      await p.screenshot({ path: path.join(dir, gw) });
      await check(p, gw, { walk: false, ide: true });
    }
    errors.push(...errs.map((e) => `${file}: ${e}`));
    await ctx.close();
  }
  for (const size of [
    { width: 1366, height: 768 },
    { width: 1100, height: 800 },
  ]) {
    for (const v of variants({ sizes: [size], stories: ["on"] })) {
      const { ctx, page: p, errs } = await open(browser, v, { state });
      await p.goto("/learn/07-rollcall");
      await ideState(p, v);
      const file = fileName("ide", v);
      await p.screenshot({ path: path.join(dir, file) });
      await check(p, file, { walk: false, ide: true });
      errors.push(...errs.map((e) => `${file}: ${e}`));
      await ctx.close();
    }
  }

  // The course page of a learner in tier 2, the certificates and verify.
  await shoot(browser, "dashboard", "/learn", { state });
  await shoot(browser, "certificates", "/certificates", { state });
  await shoot(browser, "verify", verifyHref);
  await shoot(browser, "verify-revoked", verifyFirst);
  await shoot(browser, "not-found", "/no-such-page", { state });
  await shoot(browser, "account", "/account", { state });
  await shoot(browser, "admin", "/admin", { state });

  // Forced colours, once each (VESPER.md 15).
  const forced = { size: SIZES[0], scheme: "dark", tag: "forced", story: "on" };
  for (const [name, url] of [
    ["dashboard", "/learn"],
    ["certificates", "/certificates"],
    ["ide", "/learn/07-rollcall"],
  ]) {
    const { ctx, page: p, errs } = await open(browser, forced, { state, forced: true });
    await p.goto(url);
    await p.evaluate(() => document.fonts.ready);
    if (name === "ide") await ideState(p, forced);
    const file = fileName(name, forced);
    await p.screenshot({ path: path.join(dir, file), fullPage: name !== "ide" });
    if (name === "ide") await check(p, file, { walk: false, ide: true });
    errors.push(...errs.map((e) => `${file}: ${e}`));
    await ctx.close();
  }

  // Recovery: the address is verified, so a link goes out; open it.
  await page.goto("/recover");
  await page.getByLabel("Email address").fill("ada@example.org");
  await page.getByRole("button", { name: "Send a recovery link" }).click();
  await expect(page.locator(".form-status")).toContainText("on its way");
  await expect.poll(() => webLog().some((e) => typeof e.text === "string" && e.text.includes("/recover/confirm?token=")), { timeout: 20_000 }).toBe(true);
  await shoot(browser, "recover-confirm", mailLink("/recover/confirm"));

  expect(errors).toEqual([]);
  expect(problems).toEqual([]);
});
