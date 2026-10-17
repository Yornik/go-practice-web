// verify-email.js: uses the link from the verification email. It posts
// instead of verifying on load, so a mail scanner that opens links does
// not use the token.
import { api, errorMessage, setStatus } from "../api.js";

const form = document.getElementById("verify-email-form");
const status = document.querySelector(".form-status");

form?.addEventListener("submit", async (event) => {
  event.preventDefault();
  const button = form.querySelector("button[type=submit]");
  button.disabled = true;
  try {
    const res = await api("POST", "/api/auth/verify-email", { token: form.dataset.token });
    setStatus(status, `Done: ${res.email} is verified.`, "success");
    button.hidden = true;
  } catch (err) {
    setStatus(status, errorMessage(err), "error");
    button.disabled = false;
  }
});
