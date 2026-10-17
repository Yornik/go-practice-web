// certificates.js: claim or re-issue a certificate.
import { api, errorMessage, setStatus } from "../api.js";

for (const form of document.querySelectorAll("[data-claim]")) {
  const status = form.querySelector(".form-status");
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const button = form.querySelector("button[type=submit]");
    button.disabled = true;
    try {
      const res = await api("POST", "/api/certificates", {
        kind: form.dataset.claim,
        name: form.querySelector("[name=name]").value,
        showStats: form.querySelector("[name=showStats]").checked,
      });
      setStatus(status, `Issued ${res.id}. Opening its page…`, "success");
      location.assign(res.url);
    } catch (err) {
      setStatus(status, errorMessage(err), "error");
      button.disabled = false;
    }
  });
}
