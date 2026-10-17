// recover.js: both recovery pages: asking for a link, and adding a passkey
// with it.
import { api, ApiError, errorMessage, setStatus } from "../api.js";
import { createPasskey } from "../webauthn.js";
import { toastNext } from "../ui/toast.js";
import { clearFieldErrors, fieldError, isBusy, looksLikeEmail, setBusy } from "../ui/fields.js";

const requestForm = document.getElementById("recover-form");
requestForm?.addEventListener("submit", async (event) => {
  event.preventDefault();
  const status = requestForm.querySelector(".form-status");
  const input = requestForm.querySelector("[name=email]");
  const email = input.value.trim();
  const button = requestForm.querySelector("button[type=submit]");
  if (isBusy(button)) return;
  clearFieldErrors(requestForm);
  setStatus(status, "");
  if (!looksLikeEmail(email)) {
    fieldError(input, "Email address: enter the address of your account, like name@example.com.");
    input.focus();
    return;
  }
  setBusy(button, true);
  try {
    await api("POST", "/api/auth/recover", { email });
    setStatus(status, "If an account uses this address, a recovery link is on its way. The link works once, for 15 minutes.", "success");
  } catch (err) {
    if (err instanceof ApiError && err.code === "bad_email") {
      fieldError(input, `Email address: ${err.message}`);
      input.focus();
    } else {
      setStatus(status, errorMessage(err), "error");
    }
  } finally {
    setBusy(button, false);
  }
});

const confirmForm = document.getElementById("recover-confirm-form");
const removeOthers = confirmForm?.querySelector("[name=removeOthers]");
const warning = confirmForm?.querySelector("[data-remove-warning]");
removeOthers?.addEventListener("change", () => {
  warning.hidden = !removeOthers.checked;
});
confirmForm?.addEventListener("submit", async (event) => {
  event.preventDefault();
  const status = confirmForm.querySelector(".form-status");
  const button = confirmForm.querySelector("button[type=submit]");
  if (isBusy(button)) return;
  const label = button.textContent;
  setBusy(button, true, "Creating your passkey");
  setStatus(status, "");
  try {
    const begin = await api("POST", "/api/auth/recover/begin", { token: confirmForm.dataset.token });
    const credential = await createPasskey(begin.options);
    const res = await api("POST", "/api/auth/recover/finish", { ceremony: begin.ceremony, credential, removeOthers: removeOthers.checked });
    toastNext("Passkey added.", "ok", undefined, '[data-status="passkeys"]');
    location.assign(res.next || "/account");
  } catch (err) {
    setStatus(status, errorMessage(err, "add"), "error");
    setBusy(button, false, label);
  }
});
