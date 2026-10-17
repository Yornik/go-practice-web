// admin.js: invites and learners.
import { api, errorMessage, setStatus } from "../api.js";

const form = document.getElementById("invite-form");
const status = document.querySelector('[data-status="invite"]');
const result = document.querySelector("[data-invite-result]");
const link = document.querySelector("[data-invite-link]");

form?.addEventListener("submit", async (event) => {
  event.preventDefault();
  const data = new FormData(form);
  try {
    const res = await api("POST", "/api/admin/invites", {
      email: String(data.get("email") || "").trim(),
      note: String(data.get("note") || "").trim(),
      days: Number(data.get("days") || 7),
    });
    link.value = res.url;
    result.hidden = false;
    let msg = "Invite created. It works once.";
    if (res.mailed) msg += " We emailed it.";
    if (res.mailError) msg += " " + res.mailError;
    setStatus(status, msg, res.mailError ? "error" : "success");
    link.select();
  } catch (err) {
    setStatus(status, errorMessage(err), "error");
  }
});

document.querySelector("[data-copy]")?.addEventListener("click", async () => {
  try {
    await navigator.clipboard.writeText(link.value);
    setStatus(status, "Copied.", "success");
  } catch {
    link.select();
    setStatus(status, "Press Ctrl+C to copy the selected link.");
  }
});

for (const row of document.querySelectorAll("[data-invite]")) {
  row.querySelector("[data-revoke]")?.addEventListener("click", async () => {
    if (!confirm("Revoke this invite? The link stops working.")) return;
    try {
      await api("DELETE", `/api/admin/invites/${row.dataset.invite}`);
      location.reload();
    } catch (err) {
      setStatus(status, errorMessage(err), "error");
    }
  });
}

const usersStatus = document.querySelector('[data-status="users"]');
for (const row of document.querySelectorAll("[data-user]")) {
  const set = async (disabled) => {
    try {
      await api("PUT", `/api/admin/users/${row.dataset.user}`, { disabled });
      location.reload();
    } catch (err) {
      setStatus(usersStatus, errorMessage(err), "error");
    }
  };
  row.querySelector("[data-disable]")?.addEventListener("click", () => {
    if (confirm("Disable this account? The learner is signed out and cannot sign in.")) set(true);
  });
  row.querySelector("[data-enable]")?.addEventListener("click", () => set(false));
}
