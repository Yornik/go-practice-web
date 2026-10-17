// Helpers for the E2E tests: the dev server's log, the course's reference
// solutions and a virtual passkey authenticator.
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

/** Collects page errors and CSP violations, so a test can assert none. */
export function watchErrors(page) {
  const errors = [];
  page.on("pageerror", (err) => errors.push(`page error: ${err.message}`));
  page.on("console", (msg) => {
    if (msg.type() === "error" && /Content.Security.Policy|Refused to/i.test(msg.text())) errors.push(`CSP: ${msg.text()}`);
  });
  return errors;
}
