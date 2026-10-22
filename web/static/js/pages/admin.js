// admin.js: invite links and the users.
import { api, errorMessage, setStatus } from "../api.js";
import { confirmDialog } from "../ui/dialog.js";
import { toast, toastNext } from "../ui/toast.js";
import { h, icon } from "../ui/dom.js";
import { shortDate } from "../ui/format.js";
import { isBusy, setBusy } from "../ui/fields.js";

const form = document.getElementById("invite-form");
const status = document.querySelector('[data-status="invite"]');
const result = document.querySelector("[data-invite-result]");
const link = document.querySelector("[data-invite-link]");
const linkParts = [document.querySelector('label[for="invite-link"]'), document.querySelector(".gpw-adm-copy")];
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
    // An invite that an SMTP server took comes without a url: the mail
    // holds the only copy, so opening it proves the address.
    link.value = res.url || "";
    for (const part of linkParts) if (part) part.hidden = !res.url;
    result.hidden = false;
    mail.className = "";
    mail.replaceChildren();
    if (res.mailed && !res.url) {
      // A callout wraps; a status line stays on 1 line.
      mail.className = "callout";
      mail.textContent = `Sent to ${email}. The link is only in the mail. If it does not arrive, revoke this invite and create a new one.`;
    } else if (res.mailed) {
      // A confirmation is ink with a tick: green means a test result.
      mail.className = "gpw-okword";
      mail.append(icon("tick", "gpw-pi-s"), `Sent to ${email}.`);
    } else if (res.mailError) {
      mail.className = "callout callout-warning";
      mail.textContent = `The mail failed: ${res.mailError} Send the link yourself.`;
    } else {
      mail.textContent = "Send the link yourself. It works once.";
    }
    addInviteRow(res);
    if (res.url) {
      link.focus();
      link.select();
    }
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
  const muted = (t) => h("span", { class: "gpw-quiet" }, t);
  const created = res.createdAt ? new Date(res.createdAt) : new Date();
  const row = h("tr", { "data-invite": res.hash },
    h("td", { "data-label": "Created" }, h("time", { datetime: created.toISOString() }, shortDate(created))),
    h("td", { "data-label": "Status" }, `Open until ${shortDate(res.expiresAt || created)}`),
    h("td", { "data-label": "Email" }, res.email || muted("none")),
    h("td", { "data-label": "Note" }, res.note || muted("none")),
    h("td", { "data-label": "Created by" }, res.createdBy || ""),
    h("td", { class: "gpw-adm-act" }, h("button", { type: "button", class: "btn btn-sm", "data-revoke": "", "aria-haspopup": "dialog" }, "Revoke invite",
      h("span", { class: "gpw-vh" }, ` of ${shortDate(created)}${res.email ? ` for ${res.email}` : ""}`))));
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
      warning: true,
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
