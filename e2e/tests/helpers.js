// Helpers for the E2E tests: the dev server's log and the mails in it, the
// course's reference solutions and a virtual passkey authenticator.
import { expect } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
export const repo = path.resolve(here, "..", "..");

/** The lines of the web server's JSON log, parsed. */
export function webLog() {
  const file = path.join(process.env.GO_PRACTICE_DEV_STATE, "web.log");
  return fs
    .readFileSync(file, "utf8")
    .split("\n")
    .filter(Boolean)
    .map((l) => {
      try {
        return JSON.parse(l);
      } catch {
        return {};
      }
    });
}

/** The setup link the web server logged at start. */
export function setupLink() {
  const line = webLog()
    .reverse()
    .find((e) => typeof e.msg === "string" && e.msg.startsWith("setup link: "));
  if (!line) throw new Error("no setup link in web.log: the database already has an account");
  return new URL(line.msg.slice("setup link: ".length));
}

/**
 * The path and query of the newest link in a mail of the log, for a path
 * such as "/verify-email". to limits it to 1 recipient; since skips the log
 * lines before that index (webLog().length before the action that mails).
 * The dev server logs mail instead of sending it (mail.LogSender).
 */
export function mailLink(pathPrefix, { to, since = 0 } = {}) {
  const re = new RegExp(`https?://[^\\s]*${pathPrefix.replace(/[.?/]/g, "\\$&")}\\?token=[^\\s]+`);
  const mails = webLog()
    .slice(since)
    .filter((e) => typeof e.text === "string" && re.test(e.text) && (!to || e.to === to))
    .reverse();
  if (!mails.length) throw new Error(`no mail with ${pathPrefix}${to ? ` to ${to}` : ""} in web.log`);
  const u = new URL(mails[0].text.match(re)[0]);
  return u.pathname + u.search;
}

/**
 * Waits for the newest verification link mailed to `to` and returns its
 * path and query. The site mails it in the background, so the page that
 * caused it can be there first.
 */
export async function verificationLink(to, { since = 0, timeout = 20_000 } = {}) {
  const deadline = Date.now() + timeout;
  for (;;) {
    try {
      return mailLink("/verify-email", { to, since });
    } catch (err) {
      if (Date.now() > deadline) throw err;
    }
    await new Promise((r) => setTimeout(r, 200));
  }
}

/** Opens the newest verification link mailed to `to` and confirms the address, as a learner does. */
export async function confirmEmail(page, to, { since = 0 } = {}) {
  await page.goto(await verificationLink(to, { since }));
  await page.getByRole("button", { name: "Confirm my address" }).click();
  await expect(page.locator(".form-status")).toContainText(`Your email address ${to} is verified.`);
}

const rot13 = (s) =>
  s.replace(/[a-zA-Z]/g, (c) => {
    const base = c <= "Z" ? 65 : 97;
    return String.fromCharCode(((c.charCodeAt(0) - base + 13) % 26) + base);
  });

/** The decoded reference solution of an exercise: {file name: content}. */
export function solution(slug) {
  const dir = path.join(repo, "course", "solutions", slug);
  const out = {};
  for (const f of fs.readdirSync(dir)) {
    if (f.endsWith(".go.rot13")) out[f.slice(0, -".rot13".length)] = rot13(fs.readFileSync(path.join(dir, f), "utf8"));
  }
  return out;
}

/**
 * Sets a learner's active time in an exercise straight in the dev
 * server's database, as 25 minutes of work would. Only a site that
 * scripts/dev.sh started has its database at hand: false otherwise.
 */
export async function setActiveSeconds(username, slug, seconds) {
  const state = process.env.GO_PRACTICE_DEV_STATE;
  const file = state && path.join(state, "data", "go-practice.db");
  if (process.env.E2E_BASE_URL || !file || !fs.existsSync(file)) return false;
  // node:sqlite is built in from Node 22.13; a dynamic import keeps its
  // experimental warning out of the specs that do not use it.
  const { DatabaseSync } = await import("node:sqlite");
  const db = new DatabaseSync(file);
  try {
    db.exec("PRAGMA busy_timeout = 5000");
    const res = db.prepare("UPDATE progress SET active_seconds = ? WHERE exercise = ? AND user_id = (SELECT id FROM users WHERE username = ?)").run(seconds, slug, username);
    return res.changes === 1;
  } finally {
    db.close();
  }
}

/**
 * Marks every exercise of the course passed for a learner, each with the
 * given active time, straight in the dev server's database: a course
 * certificate without 50 runs. False when the database is not at hand.
 */
export async function passAll(username, seconds) {
  const state = process.env.GO_PRACTICE_DEV_STATE;
  const file = state && path.join(state, "data", "go-practice.db");
  if (process.env.E2E_BASE_URL || !file || !fs.existsSync(file)) return false;
  const slugs = fs.readdirSync(path.join(repo, "course", "exercises")).filter((d) => /^\d\d-/.test(d));
  const { DatabaseSync } = await import("node:sqlite");
  const db = new DatabaseSync(file);
  try {
    db.exec("PRAGMA busy_timeout = 5000");
    const now = new Date().toISOString().replace(/\.\d+Z$/, ".000Z");
    const put = db.prepare(`INSERT INTO progress (user_id, exercise, status, started_at, passed_at, active_seconds, runs, last_run_at)
      VALUES ((SELECT id FROM users WHERE username = ?), ?, 'passed', ?, ?, ?, 3, ?)
      ON CONFLICT (user_id, exercise) DO UPDATE SET status = 'passed', passed_at = excluded.passed_at, active_seconds = excluded.active_seconds`);
    for (const slug of slugs) put.run(username, slug, now, now, seconds, now);
    return slugs.length === 50;
  } finally {
    db.close();
  }
}

/**
 * Runs in the page: what of the verify page's paper and activity leaves
 * its box (VESPER.md 6.15, 8.9). A skyline group leaves its year or a
 * plate meets another, a reading or a value runs out of its cell, or the
 * page scrolls sideways.
 */
export function paperProblems() {
  const out = [];
  const R = (e) => e.getBoundingClientRect();
  const text = (e) => {
    const r = document.createRange();
    r.selectNodeContents(e);
    return r.getBoundingClientRect();
  };
  if (document.documentElement.scrollWidth > document.documentElement.clientWidth) out.push("the page scrolls sideways");
  for (const yr of document.querySelectorAll(".gpw-pp-sky .gpw-sky-yr")) {
    const y = R(yr);
    for (const g of yr.querySelectorAll(".gpw-sky-row > *")) if (R(g).right > y.right + 0.5) out.push("a skyline group leaves its year");
  }
  const plates = [...document.querySelectorAll(".gpw-pp-sky .gpw-sky-plate")].map(R);
  plates.forEach((a, i) => plates.slice(i + 1).forEach((b) => {
    if (a.left < b.right && b.left < a.right && a.top < b.bottom && b.top < a.bottom) out.push("2 skyline plates meet");
  }));
  for (const cell of document.querySelectorAll(".gpw-pp-stats > div, .gpw-act td, .gpw-act th")) {
    const c = R(cell);
    for (const e of cell.matches("div") ? cell.children : [cell]) if (text(e).right > c.right + 1) out.push(`"${e.textContent.trim()}" runs out of its cell`);
  }
  return [...new Set(out)];
}

/** Adds a CDP virtual authenticator that holds discoverable passkeys. */
export async function virtualAuthenticator(context, page) {
  const cdp = await context.newCDPSession(page);
  await cdp.send("WebAuthn.enable");
  const { authenticatorId } = await cdp.send("WebAuthn.addVirtualAuthenticator", {
    options: {
      protocol: "ctap2",
      transport: "internal",
      hasResidentKey: true,
      hasUserVerification: true,
      isUserVerified: true,
      automaticPresenceSimulation: true,
    },
  });
  return { cdp, authenticatorId };
}

/**
 * Site selectors whose class names are all Monaco's, yet that do not reach
 * Monaco's DOM. Each entry gives the reason.
 */
const MONACO_ALLOWED = {
  "svg.icon": "Monaco's .icon is a div, span or label; its icons are a font, never an svg.",
  "pre.code": "Monaco's .code is a span in a hover, never a pre.",
  ".visually-hidden": "Monaco's .visually-hidden has the same job: text off screen that screen readers still read.",
};

/**
 * The selectors of the site's style sheets that can reach Monaco's editor or its
 * widgets. Monaco's widgets are part of the page, so a site class that Monaco
 * also uses styles them: ".contents" once moved every suggestion out of its
 * row. 2 checks, because most widgets are closed while a test runs:
 *
 * - "names": every class of the selector is a class of Monaco's own CSS, so
 *   it can reach a widget that is not open now.
 * - "live": the selector matches an element in the editor host or in a
 *   context view now.
 *
 * Selectors without a class (the base element rules), the IDE's own
 * decorations inside the editor and MONACO_ALLOWED are left out. A gpw-
 * class is ours and never Monaco's, so a selector with one skips the
 * "names" check, but not the "live" check: ".gpw-x .row" can still reach
 * Monaco's rows. A selector the browser cannot test counts as a hit, so a
 * parse error cannot hide a collision.
 */
export async function siteRulesOnMonaco(page) {
  const { siteRules, monacoClasses, hits } = await page.evaluate((allowed) => {
    // The IDE's own decorations, which Monaco places in the editor on purpose.
    const decorations = /\.(eol-balloon|gm-problem|gm-advice|ln-problem)(?![\w-])/;
    const ours = /\.gpw-[\w-]+(?![\w-])/;
    // A list splits on its top-level commas only: ":is(.a, .b)" is 1 selector.
    const split = (text) => {
      const out = [];
      let depth = 0;
      let start = 0;
      for (let i = 0; i < text.length; i++) {
        const c = text[i];
        if (c === "(" || c === "[") depth++;
        else if (c === ")" || c === "]") depth--;
        else if (c === "," && depth === 0) {
          out.push(text.slice(start, i).trim());
          start = i + 1;
        }
      }
      out.push(text.slice(start).trim());
      return out;
    };
    const walk = (rules, each) => {
      for (const r of rules) {
        if (r.selectorText) split(r.selectorText).forEach(each);
        if (r.cssRules) walk(r.cssRules, each);
      }
    };
    // The classes an element must have; a class inside :not() or an
    // attribute value does not make the selector match.
    const classes = (sel) => [...sel.replace(/\[[^\]]*\]/g, "").replace(/:not\((?:[^()]|\([^()]*\))*\)/g, "").matchAll(/\.([\w-]+)/g)].map((m) => m[1]);
    const site = [];
    const monaco = new Set();
    for (const sheet of document.styleSheets) {
      const href = sheet.href || "";
      if (/\/(site|ide|public|app)\.css/.test(href)) walk(sheet.cssRules, (s) => site.push(s));
      // Monaco's CSS file, and the style elements Monaco adds at run time.
      else if (href.includes("/vendor/monaco/") || (!href && sheet.ownerNode?.tagName === "STYLE")) walk(sheet.cssRules, (s) => classes(s).forEach((c) => monaco.add(c)));
    }
    const live = [...document.querySelectorAll("[data-editor] *, .context-view, .context-view *, .quick-input-widget, .quick-input-widget *")];
    // A pseudo-element or a state like :hover styles the same element.
    const bare = (sel) => sel.replace(/::[\w-]+(\([^)]*\))?|:(before|after|first-line|first-letter)\b/g, "").replace(/:(hover|active|focus-visible|focus-within|focus|visited)\b/g, "");
    const matches = (el, sel) => {
      try {
        return el.matches(bare(sel));
      } catch {
        return true;
      }
    };
    const out = [];
    for (const sel of site) {
      if (!sel.includes(".") || decorations.test(sel) || sel in allowed) continue;
      const cls = classes(sel);
      if (!ours.test(sel) && cls.length && cls.every((c) => monaco.has(c))) out.push(`names: ${sel}`);
      else if (live.some((el) => matches(el, sel))) out.push(`live: ${sel}`);
    }
    return { siteRules: site.length, monacoClasses: monaco.size, hits: out };
  }, MONACO_ALLOWED);
  // An empty sheet list would pass every check; fail instead.
  if (!siteRules || !monacoClasses) throw new Error(`siteRulesOnMonaco read ${siteRules} site selectors and ${monacoClasses} Monaco classes`);
  return hits;
}

/**
 * The visible text runs in Saira that hold a 0 outside its slash: a gpw-z
 * span in HTML, a gpw-sv-z tspan (Atkinson Mono, slashed) in SVG. Stricter
 * than the frames' audit: it checks every 0, not only the first of a
 * number (VESPER.md 14.3).
 */
export async function ovalZeros(page) {
  return page.evaluate(() => {
    const out = [];
    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
    for (let n = walker.nextNode(); n; n = walker.nextNode()) {
      if (!n.data.includes("0")) continue;
      const el = n.parentElement;
      if (!/^"?Saira/.test(getComputedStyle(el).fontFamily)) continue;
      // Text for screen readers only is never drawn.
      if (el.closest(".gpw-vh, .visually-hidden, [hidden]")) continue;
      const box = el.getBoundingClientRect();
      if (!box.width && !box.height) continue;
      const slashed = el.closest("svg") ? el.closest(".gpw-sv-z") : el.classList.contains("gpw-z");
      if (!slashed || n.data !== "0") out.push(`${el.tagName.toLowerCase()}.${[...el.classList].join(".")}: ${n.data.trim().slice(0, 40)}`);
    }
    return out;
  });
}

/** Collects page errors and CSP violations, so a test can assert none. */
export function watchErrors(page) {
  const errors = [];
  page.on("pageerror", (err) => errors.push(`page error: ${err.message}`));
  page.on("console", (msg) => {
    if (msg.type() === "error" && /Content.Security.Policy|Refused to/i.test(msg.text())) errors.push(`CSP: ${msg.text()}`);
  });
  return errors;
}

/**
 * The editor's focus ring, read from pixels, not from computed style:
 * Monaco's root covers the host, so a ring drawn under it has the right
 * style and never shows (VESPER.md 3.10). Returns the sides of the editor
 * whose middle pixel, 1 px inside the edge, is not the --focus colour.
 * Call it while the focus is in the editor.
 */
export async function editorRingGaps(page) {
  const { box, focus } = await page.evaluate(() => {
    const host = document.querySelector("[data-editor]");
    const r = host.getBoundingClientRect();
    const rgb = getComputedStyle(host, "::after").outlineColor.match(/\d+/g).slice(0, 3).map(Number);
    return { box: { x: r.left, y: r.top, w: r.width, h: r.height, vw: innerWidth, vh: innerHeight }, focus: rgb };
  });
  const png = await page.screenshot();
  // On a phone the editor can run past the window: check the sides in view.
  const midY = (Math.max(box.y, 0) + Math.min(box.y + box.h, box.vh)) / 2;
  const points = Object.fromEntries(
    Object.entries({
      left: [box.x + 1, midY],
      top: [box.x + box.w / 2, box.y + 1],
      right: [box.x + box.w - 2, midY],
      bottom: [box.x + box.w / 2, box.y + box.h - 2],
    }).filter(([, [x, y]]) => x >= 0 && y >= 0 && x < box.vw && y < box.vh),
  );
  // A blank page decodes the shot: the site's CSP does not allow data: images.
  const blank = await page.context().newPage();
  try {
    const got = await blank.evaluate(async ({ b64, points }) => {
      const img = new Image();
      img.src = `data:image/png;base64,${b64}`;
      await img.decode();
      const c = new OffscreenCanvas(img.width, img.height);
      const g = c.getContext("2d");
      g.drawImage(img, 0, 0);
      const out = {};
      for (const [side, [x, y]] of Object.entries(points)) out[side] = [...g.getImageData(Math.round(x), Math.round(y), 1, 1).data].slice(0, 3);
      return out;
    }, { b64: png.toString("base64"), points });
    return Object.entries(got)
      .filter(([, c]) => c.some((v, i) => Math.abs(v - focus[i]) > 8))
      .map(([side, c]) => `${side} is rgb(${c.join(", ")}), not rgb(${focus.join(", ")})`);
  } finally {
    await blank.close();
  }
}
