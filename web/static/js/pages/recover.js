// recover.js: both recovery pages: asking for a link, and adding a passkey
// with it.
import { api, errorMessage, setStatus } from "../api.js";
import { createPasskey } from "../webauthn.js";

const status = document.querySelector(".form-status");

const requestForm = document.getElementById("recover-form");
requestForm?.addEventListener("submit", async (event) => {
  event.preventDefault();
  const email = String(new FormData(requestForm).get("email") || "").trim();
  const button = requestForm.querySelector("button[type=submit]");
  button.disabled = true;
  try {
    const res = await api("POST", "/api/auth/recover", { email });
    setStatus(status, res.message, "success");
  } catch (err) {
    setStatus(status, errorMessage(err), "error");
    button.disabled = false;
  }
});

const confirmForm = document.getElementById("recover-confirm-form");
confirmForm?.addEventListener("submit", async (event) => {
  event.preventDefault();
  const removeOthers = confirmForm.querySelector("[name=removeOthers]").checked;
  const button = confirmForm.querySelector("button[type=submit]");
  button.disabled = true;
  setStatus(status, "Creating your passkey…");
  try {
    const begin = await api("POST", "/api/auth/recover/begin", { token: confirmForm.dataset.token });
    const credential = await createPasskey(begin.options);
    const res = await api("POST", "/api/auth/recover/finish", { ceremony: begin.ceremony, credential, removeOthers });
    location.assign(res.next || "/account");
  } catch (err) {
    setStatus(status, errorMessage(err), "error");
    button.disabled = false;
  }
});
