// verify-email.js: uses the link from the verification email. It posts
// when the learner presses the button, not on load, so a mail scanner
// that opens links (some run scripts) does not use the token. The answer
// turns the access panel into the result (VESPER.md 8.5).
import { api, errorMessage, setStatus } from "../api.js";
import { isBusy, setBusy } from "../ui/fields.js";
import { clock, shortDate } from "../ui/format.js";
import { zero } from "../ui/zero.js";

const form = document.getElementById("verify-email-form");
const status = document.querySelector(".form-status");

/** The address as 2 unbreakable halves, so a long one breaks only after its "@". */
function address(email) {
  const at = email.lastIndexOf("@");
  const half = (text) => {
    const span = document.createElement("span");
    span.className = "gpw-nobr";
    span.textContent = text;
    return span;
  };
  if (at < 0) return [half(email)];
  return [half(email.slice(0, at + 1)), document.createElement("wbr"), half(email.slice(at + 1))];
}

function showVerified(email) {
  const now = new Date();
  for (const el of document.querySelectorAll("[data-before]")) el.hidden = true;
  for (const el of document.querySelectorAll("[data-after]")) el.hidden = false;
  for (const el of document.querySelectorAll("[data-address]")) el.replaceChildren(...address(email));
  document.querySelector("[data-verified-at]")?.replaceChildren(zero(`${shortDate(now)}, ${clock(now)}`));
  const confirmed = document.querySelector("[data-confirmed-at]");
  if (confirmed) confirmed.textContent = `Confirmed at ${clock(now)}.`;
}

form?.addEventListener("submit", async (event) => {
  event.preventDefault();
  const button = form.querySelector("button[type=submit]");
  if (isBusy(button)) return;
  setBusy(button, true);
  status?.classList.remove("visually-hidden");
  try {
    const res = await api("POST", "/api/auth/verify-email", { token: form.dataset.token });
    // The panel shows the result; the status line stays for screen
    // readers, which do not hear a heading change.
    setStatus(status, `Your email address ${res.email} is verified.`, "success");
    status?.classList.add("visually-hidden");
    showVerified(res.email);
    // The button is gone: the focus goes to what comes next.
    document.querySelector("[data-verified-next] a")?.focus();
  } catch (err) {
    const expired = err?.status === 410 || err?.code === "gone" || /expired|used/i.test(err?.message || "");
    setStatus(status, expired ? "This link expired, or someone used it. Send a new link from the account page." : errorMessage(err), "error");
    setBusy(button, false);
  }
});
