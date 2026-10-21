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
 * The selectors of site.css and ide.css that can reach Monaco's editor or its
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
 * decorations and MONACO_ALLOWED are left out. A selector the browser cannot
 * test counts as a hit, so a parse error cannot hide a collision.
 */
export async function siteRulesOnMonaco(page) {
  const { siteRules, monacoClasses, hits } = await page.evaluate((allowed) => {
    const ours = /\.(eol-balloon|gm-problem|gm-advice|ln-problem|gpw-[\w-]+)(?![\w-])/;
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
      if (/\/(site|ide)\.css/.test(href)) walk(sheet.cssRules, (s) => site.push(s));
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
      if (!sel.includes(".") || ours.test(sel) || sel in allowed) continue;
      const cls = classes(sel);
      if (cls.length && cls.every((c) => monaco.has(c))) out.push(`names: ${sel}`);
      else if (live.some((el) => matches(el, sel))) out.push(`live: ${sel}`);
    }
    return { siteRules: site.length, monacoClasses: monaco.size, hits: out };
  }, MONACO_ALLOWED);
  // An empty sheet list would pass every check; fail instead.
  if (!siteRules || !monacoClasses) throw new Error(`siteRulesOnMonaco read ${siteRules} site selectors and ${monacoClasses} Monaco classes`);
  return hits;
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
