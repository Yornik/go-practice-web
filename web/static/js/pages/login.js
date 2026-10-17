// login.js: sign in with a passkey, with the button and with the
// autofill list of the username field.
import { api, ApiError, errorMessage, setStatus } from "../api.js";
import { autofillSupported, getPasskey, supported } from "../webauthn.js";
import { isBusy, setBusy } from "../ui/fields.js";

const form = document.getElementById("login-form");
const status = form?.querySelector(".form-status");
const button = form?.querySelector("button[type=submit]");
const next = form?.dataset.next || "/learn";
let autofill = null; // the AbortController of the waiting autofill request

// The server's codes, in words that say what to do next.
const MESSAGES = {
  unknown_passkey: "This passkey is not registered here. It can belong to another site, or it was removed. Try another passkey, or recover your account.",
  passkey_cloned: "This passkey reported an older counter than before. 2 copies of the key can exist. Sign in with another passkey and check the account page.",
};

function message(err) {
  return MESSAGES[err?.code] || errorMessage(err, "login");
}

async function signIn(conditional, signal) {
  const begin = await api("POST", "/api/auth/login/begin", { conditional });
  const credential = await getPasskey(begin.options, { signal, conditional });
  setStatus(status, "Checking your passkey.", "info");
  const res = await api("POST", "/api/auth/login/finish", { ceremony: begin.ceremony, credential, next });
  location.assign(res.next || "/learn");
}

async function startAutofill() {
  if (!(await autofillSupported())) return;
  autofill = new AbortController();
  try {
    await signIn(true, autofill.signal);
  } catch (err) {
    // An abort means the learner pressed the button instead. A browser
    // that cannot fill in passkeys still has the button, so only an answer
    // of the server (after the learner picked a passkey) is worth showing.
    if (err instanceof ApiError) setStatus(status, message(err), "error");
  }
}

form?.addEventListener("submit", async (event) => {
  event.preventDefault();
  if (!supported()) {
    setStatus(status, "This browser does not support passkeys. Use a current version of Chrome, Edge, Firefox or Safari.", "error");
    return;
  }
  if (isBusy(button)) return;
  // Only 1 WebAuthn request can run at a time.
  autofill?.abort();
  // Busy, not disabled: the button keeps the keyboard focus.
  setBusy(button, true, "Waiting for your passkey");
  setStatus(status, "");
  try {
    await signIn(false);
  } catch (err) {
    setStatus(status, message(err), "error");
    setBusy(button, false, "Sign in with a passkey");
    startAutofill();
  }
});

if (!supported()) {
  setStatus(status, "This browser does not support passkeys. Use a current version of Chrome, Edge, Firefox or Safari.", "error");
} else {
  startAutofill();
}
