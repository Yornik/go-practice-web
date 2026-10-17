// join.js: sign-up with an invite or the setup link: the details, then a
// new passkey. The fields are checked here first, so the learner sees
// every problem at once, next to its field.
import { api, ApiError, errorMessage, setStatus } from "../api.js";
import { createPasskey } from "../webauthn.js";
import { clearFieldErrors, fieldError, focusFirstInvalid, isBusy, looksLikeEmail, setBusy } from "../ui/fields.js";

const form = document.getElementById("join-form");
const status = form?.querySelector(".form-status");
const fields = {
  username: document.getElementById("join-username"),
  displayName: document.getElementById("join-name"),
  email: document.getElementById("join-email"),
};
// The server's codes, by the field they are about.
const FIELD_OF = {
  bad_username: "username",
  username_taken: "username",
  bad_display_name: "displayName",
  bad_email: "email",
  email_required: "email",
  email_taken: "email",
};
const LABEL = { username: "Username", displayName: "Display name", email: "Email address" };

function check(body) {
  const problems = {};
  if (!/^[a-z0-9_-]{3,32}$/.test(body.username)) problems.username = "Username: use 3 to 32 characters: lower-case letters, digits, - and _.";
  if (!body.displayName) problems.displayName = "Display name: enter the name to print on your certificates.";
  else if ([...body.displayName].length > 64) problems.displayName = "Display name: use at most 64 characters.";
  if (!body.email) problems.email = "Email address: enter your email address. It is your way back in if you lose your passkey.";
  else if (!looksLikeEmail(body.email)) problems.email = "Email address: enter an address like name@example.com.";
  return problems;
}

form?.addEventListener("submit", async (event) => {
  event.preventDefault();
  const button = form.querySelector("button[type=submit]");
  if (isBusy(button)) return;
  const data = new FormData(form);
  const body = {
    username: String(data.get("username") || "").trim().toLowerCase(),
    displayName: String(data.get("displayName") || "").trim(),
    email: String(data.get("email") || "").trim(),
  };
  clearFieldErrors(form);
  setStatus(status, "");
  const problems = check(body);
  for (const [name, message] of Object.entries(problems)) fieldError(fields[name], message);
  if (focusFirstInvalid(form)) return;
  body[form.dataset.mode === "setup" ? "setup" : "invite"] = form.dataset.token;
  const label = button.textContent;
  setBusy(button, true, "Creating your passkey");
  try {
    const begin = await api("POST", "/api/auth/register/begin", body);
    const credential = await createPasskey(begin.options);
    button.textContent = "Saving your account";
    const res = await api("POST", "/api/auth/register/finish", { ceremony: begin.ceremony, credential });
    location.assign(res.next || "/learn");
  } catch (err) {
    setBusy(button, false, label);
    const field = err instanceof ApiError ? FIELD_OF[err.code] : null;
    if (field) {
      fieldError(fields[field], `${LABEL[field]}: ${err.message}`);
      fields[field].focus();
      return;
    }
    // A passkey ceremony error belongs to the button, not to a field.
    setStatus(status, errorMessage(err, "register"), "error");
  }
});
