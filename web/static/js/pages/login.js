// login.js: sign in with a passkey, with the modal button and with the
// autofill list of the username field.
import { api, errorMessage, setStatus } from "../api.js";
import { autofillSupported, getPasskey, supported } from "../webauthn.js";

const form = document.getElementById("login-form");
const status = document.querySelector(".form-status");
const next = form?.dataset.next || "/learn";
let autofill = null; // the AbortController of the waiting autofill request

async function signIn(conditional, signal) {
  const begin = await api("POST", "/api/auth/login/begin", { conditional });
  const credential = await getPasskey(begin.options, { signal, conditional });
  setStatus(status, "Checking your passkey…");
  const res = await api("POST", "/api/auth/login/finish", { ceremony: begin.ceremony, credential, next });
  location.assign(res.next || "/learn");
}

async function startAutofill() {
  if (!(await autofillSupported())) return;
  autofill = new AbortController();
  try {
    await signIn(true, autofill.signal);
  } catch (err) {
    // An abort means the user pressed the button instead.
    if (err.name !== "AbortError") setStatus(status, errorMessage(err), "error");
  }
}

form?.addEventListener("submit", async (event) => {
  event.preventDefault();
  if (!supported()) {
    setStatus(status, "This browser does not support passkeys.", "error");
    return;
  }
  // Only 1 WebAuthn request can run at a time.
  autofill?.abort();
  const button = form.querySelector("button[type=submit]");
  button.disabled = true;
  setStatus(status, "Waiting for your passkey…");
  try {
    await signIn(false);
  } catch (err) {
    setStatus(status, errorMessage(err), "error");
    button.disabled = false;
    startAutofill();
  }
});

if (!supported()) {
  setStatus(status, "This browser does not support passkeys. Use a current browser.", "error");
} else {
  startAutofill();
}
