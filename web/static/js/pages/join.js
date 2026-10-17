// join.js: sign-up with an invite or the setup link: the form, then a new
// passkey.
import { api, errorMessage, setStatus } from "../api.js";
import { createPasskey } from "../webauthn.js";

const form = document.getElementById("join-form");
const status = document.querySelector(".form-status");

form?.addEventListener("submit", async (event) => {
  event.preventDefault();
  const data = new FormData(form);
  const body = {
    username: String(data.get("username") || "").trim().toLowerCase(),
    displayName: String(data.get("displayName") || "").trim(),
    email: String(data.get("email") || "").trim(),
  };
  body[form.dataset.mode === "setup" ? "setup" : "invite"] = form.dataset.token;
  const button = form.querySelector("button[type=submit]");
  button.disabled = true;
  setStatus(status, "Creating your passkey…");
  try {
    const begin = await api("POST", "/api/auth/register/begin", body);
    const credential = await createPasskey(begin.options);
    setStatus(status, "Saving your account…");
    const res = await api("POST", "/api/auth/register/finish", { ceremony: begin.ceremony, credential });
    location.assign(res.next || "/learn");
  } catch (err) {
    setStatus(status, errorMessage(err), "error");
    button.disabled = false;
  }
});
