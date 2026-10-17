// admin.js: invite links and the users.
import { api, errorMessage, setStatus } from "../api.js";
import { confirmDialog } from "../ui/dialog.js";
import { toast, toastNext } from "../ui/toast.js";
import { h } from "../ui/dom.js";
import { shortDate } from "../ui/format.js";
import { isBusy, setBusy } from "../ui/fields.js";

const form = document.getElementById("invite-form");
const status = document.querySelector('[data-status="invite"]');
const result = document.querySelector("[data-invite-result]");
const link = document.querySelector("[data-invite-link]");
const mail = document.querySelector("[data-invite-mail]");

form?.addEventListener("submit", async (event) => {
  event.preventDefault();
  const data = new FormData(form);
  const button = form.querySelector("button[type=submit]");
  if (isBusy(button)) return;
  setBusy(button, true, "Creating the invite link");
  setStatus(status, "");
  try {
    const email = String(data.get("email") || "").trim();
    const res = await api("POST", "/api/admin/invites", {
      email,
      note: String(data.get("note") || "").trim(),
      days: Number(data.get("days") || 7),
    });
    link.value = res.url;
    result.hidden = false;
    mail.className = "";
    if (res.mailed) {
      mail.className = "status status-ok";
      mail.textContent = `Sent to ${email}.`;
    } else if (res.mailError) {
      mail.className = "callout callout-warning";
      mail.textContent = `The mail failed: ${res.mailError} Send the link yourself.`;
    } else {
      mail.textContent = "Send the link yourself. It works once.";
    }
    addInviteRow(res);
    link.focus();
    link.select();
  } catch (err) {
    setStatus(status, errorMessage(err), "error");
  } finally {
    setBusy(button, false, "Create the invite link");
  }
});

/** The new invite goes into the table at once, so the page does not contradict itself. */
function addInviteRow(res) {
  const table = document.querySelector("[data-invites-table]");
  const tbody = table?.querySelector("tbody");
  if (!tbody || !res.hash) return;
  const muted = (t) => h("span", { class: "muted" }, t);
  const created = res.createdAt ? new Date(res.createdAt) : new Date();
  const row = h("tr", { "data-invite": res.hash },
    h("td", {}, h("time", { datetime: created.toISOString() }, shortDate(created))),
    h("td", { "data-label": "Status" }, `Open until ${shortDate(res.expiresAt || created)}`),
    h("td", { "data-label": "Email" }, res.email || muted("none")),
    h("td", { "data-label": "Note" }, res.note || muted("none")),
    h("td", { "data-label": "Created by" }, res.createdBy || ""),
    h("td", { class: "actions" }, h("button", { type: "button", class: "btn btn-sm btn-danger", "data-revoke": "" }, "Revoke invite",
      h("span", { class: "visually-hidden" }, ` of ${shortDate(created)}${res.email ? ` for ${res.email}` : ""}`))));
  tbody.prepend(row);
  wireRevoke(row);
  table.hidden = false;
  document.querySelector("[data-invites-empty]")?.setAttribute("hidden", "");
}

document.querySelector("[data-copy]")?.addEventListener("click", async () => {
  try {
    await navigator.clipboard.writeText(link.value);
    toast("Invite link copied.");
  } catch {
    link.select();
    toast("The browser did not allow the copy. The link is selected: press Ctrl+C.", { kind: "info" });
  }
});

for (const row of document.querySelectorAll("[data-invite]")) wireRevoke(row);

function wireRevoke(row) {
  row.querySelector("[data-revoke]")?.addEventListener("click", async () => {
    const ok = await confirmDialog({
      title: "Revoke this invite?",
      text: "The link stops working at once. You can create a new one.",
      confirm: "Revoke invite",
      cancel: "Keep it",
      danger: true,
    });
    if (!ok) return;
    try {
      await api("DELETE", `/api/admin/invites/${row.dataset.invite}`);
      toastNext("Invite revoked.", "ok", undefined, '[data-status="invites"]');
      location.reload();
    } catch (err) {
      setStatus(status, errorMessage(err), "error");
    }
  });
}

const usersStatus = document.querySelector('[data-status="users"]');
for (const row of document.querySelectorAll("[data-user]")) {
  const name = row.dataset.name;
  const update = async (body, done) => {
    try {
      await api("PUT", `/api/admin/users/${row.dataset.user}`, body);
      toastNext(done, "ok", undefined, '[data-status="users"]');
      location.reload();
    } catch (err) {
      setStatus(usersStatus, errorMessage(err), "error");
    }
  };
  row.querySelector("[data-disable]")?.addEventListener("click", async () => {
    const ok = await confirmDialog({
      title: `Disable ${name}?`,
      text: `${name} cannot sign in until you enable the account again. Their sessions end now.`,
      warning: true,
      confirm: "Disable",
      cancel: "Cancel",
      danger: true,
    });
    if (ok) update({ disabled: true }, `${name} disabled.`);
  });
  row.querySelector("[data-enable]")?.addEventListener("click", () => update({ disabled: false }, `${name} enabled.`));
  row.querySelector("[data-make-admin]")?.addEventListener("click", async () => {
    const ok = await confirmDialog({
      title: `Make ${name} an admin?`,
      text: `${name} can then create invite links and disable accounts. You cannot undo this here.`,
      confirm: "Make admin",
      cancel: "Cancel",
    });
    if (ok) update({ admin: true }, `${name} is an admin.`);
  });
}
