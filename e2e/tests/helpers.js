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
 * The selectors of site.css and ide.css that match an element of Monaco's
 * editor or widgets, apart from the rules without a class (the base element
 * rules) and the rules of the IDE's own decorations. Monaco's widgets are
 * part of the page, so a site class that Monaco also uses reaches them:
 * ".contents" once moved every suggestion out of its row.
 */
export async function siteRulesOnMonaco(page) {
  return page.evaluate(() => {
    const ours = /\.(eol-balloon|gm-problem|gm-advice|ln-problem|gpw-[\w-]+)(?![\w-])/;
    const selectors = [];
    const walk = (rules) => {
      for (const r of rules) {
        if (r.selectorText) selectors.push(...r.selectorText.split(",").map((s) => s.trim()));
        if (r.cssRules) walk(r.cssRules);
      }
    };
    for (const sheet of document.styleSheets) {
      if (/\/(site|ide)\.css/.test(sheet.href || "")) walk(sheet.cssRules);
    }
    const monaco = [...document.querySelectorAll(".monaco-editor, .monaco-editor *, .context-view *, .quick-input-widget *")];
    const matches = (el, sel) => {
      try {
        return el.matches(sel.replace(/::?(before|after|backdrop|placeholder|marker)\b/g, ""));
      } catch {
        return false;
      }
    };
    return selectors.filter((sel) => sel.includes(".") && !ours.test(sel) && monaco.some((el) => matches(el, sel)));
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
