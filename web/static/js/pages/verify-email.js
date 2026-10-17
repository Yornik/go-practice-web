// verify-email.js: uses the link from the verification email. It posts
// when the learner presses the button, not on load, so a mail scanner
// that opens links (some run scripts) does not use the token.
import { api, errorMessage, setStatus } from "../api.js";
import { isBusy, setBusy } from "../ui/fields.js";

const form = document.getElementById("verify-email-form");
const status = form?.querySelector(".form-status");

form?.addEventListener("submit", async (event) => {
  event.preventDefault();
  const button = form.querySelector("button[type=submit]");
  if (isBusy(button)) return;
  setBusy(button, true);
  try {
    const res = await api("POST", "/api/auth/verify-email", { token: form.dataset.token });
    setStatus(status, `Your email address ${res.email} is verified.`, "success");
    button.hidden = true;
    const next = document.querySelector("[data-verified-next]");
    next.hidden = false;
    // The button is gone: the focus goes to what comes next.
    next.querySelector("a")?.focus();
  } catch (err) {
    const expired = err?.status === 410 || err?.code === "gone" || /expired|used/i.test(err?.message || "");
    setStatus(status, expired ? "This link expired, or someone used it. Send a new link from the account page." : errorMessage(err), "error");
    setBusy(button, false);
  }
});
