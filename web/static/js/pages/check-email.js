// check-email.js: the page a new account waits on until its address is
// verified. "Send the link again" asks for a new link; the rate limits
// answer with their own message, which the status line shows.
import { ApiError, api, errorMessage, setStatus } from "../api.js";
import { isBusy, setBusy } from "../ui/fields.js";

const button = document.querySelector("[data-resend]");
const status = document.querySelector("[data-status]");

button?.addEventListener("click", async () => {
  if (isBusy(button)) return;
  setBusy(button, true);
  setStatus(status, "");
  try {
    const res = await api("POST", "/api/auth/verify-email/resend", {});
    setStatus(status, `We sent a new link to ${res.email}. The link before it does not work any more.`, "success");
    // An expired link said so in the lede; a new one is out now.
    const lede = document.querySelector("[data-lede]");
    if (lede) {
      const address = document.createElement("strong");
      address.textContent = res.email;
      lede.replaceChildren("We sent a link to ", address, ". Open it to start the course.");
    }
  } catch (err) {
    // The link was used elsewhere, perhaps on a phone: nothing waits.
    if (err instanceof ApiError && err.code === "already_verified") {
      location.assign("/learn");
      return;
    }
    setStatus(status, errorMessage(err), "error");
  } finally {
    setBusy(button, false);
  }
});
