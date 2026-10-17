// account.js: the profile, the passkeys and the sessions.
import { api, errorMessage, setStatus } from "../api.js";
import { createPasskey } from "../webauthn.js";
import { confirmDialog, promptDialog } from "../ui/dialog.js";
import { toast, toastNext } from "../ui/toast.js";
import { isBusy, setBusy } from "../ui/fields.js";

const profile = document.getElementById("profile-form");
const profileStatus = document.querySelector('[data-status="profile"]');
const passkeyStatus = document.querySelector('[data-status="passkeys"]');

profile?.addEventListener("submit", async (event) => {
  event.preventDefault();
  const data = new FormData(profile);
  const button = profile.querySelector("button[type=submit]");
  if (isBusy(button)) return;
  setBusy(button, true, "Saving");
  try {
    const res = await api("PUT", "/api/account", {
      displayName: String(data.get("displayName") || ""),
      email: String(data.get("email") || ""),
    });
    if (res.verificationSent) {
      toastNext(`Profile saved. A verification link went to ${res.pendingEmail}.`, "ok", undefined, '[data-status="profile"]');
      location.reload();
      return;
    }
    setStatus(profileStatus, "Profile saved.", "success");
  } catch (err) {
    setStatus(profileStatus, errorMessage(err), "error");
  } finally {
    setBusy(button, false, "Save the profile");
  }
});

document.querySelector("[data-resend]")?.addEventListener("click", async () => {
  try {
    const res = await api("POST", "/api/auth/verify-email/resend", {});
    setStatus(profileStatus, `Verification link sent to ${res.email}.`, "success");
  } catch (err) {
    setStatus(profileStatus, errorMessage(err), "error");
  }
});

const nameRule = (v) => (v.trim().length === 0 ? "Enter a name of 1 to 64 characters." : "");

document.querySelector("[data-add-passkey]")?.addEventListener("click", async (event) => {
  const button = event.currentTarget;
  if (isBusy(button)) return;
  setBusy(button, true, "Adding a passkey");
  setStatus(passkeyStatus, "");
  try {
    const begin = await api("POST", "/api/auth/passkeys/begin", {});
    const credential = await createPasskey(begin.options);
    const name = await promptDialog({
      title: "Name the new passkey",
      label: "Name",
      help: "For example the device or the password manager that holds it.",
      confirm: "Save the name",
      cancel: "Skip",
    });
    await api("POST", "/api/auth/passkeys/finish", { ceremony: begin.ceremony, credential, name: name || "" });
    toastNext("Passkey added.", "ok", undefined, '[data-status="passkeys"]');
    location.reload();
  } catch (err) {
    setStatus(passkeyStatus, errorMessage(err, "add"), "error");
    setBusy(button, false, "Add a passkey");
  }
});

for (const row of document.querySelectorAll("[data-passkey]")) {
  const id = row.dataset.passkey;
  const nameEl = row.querySelector("[data-passkey-name]");
  row.querySelector("[data-rename]")?.addEventListener("click", async () => {
    const name = await promptDialog({
      title: "Rename the passkey",
      label: "Name",
      value: nameEl.textContent,
      confirm: "Rename",
      validate: nameRule,
    });
    if (name === null) return;
    try {
      await api("PUT", `/api/auth/passkeys/${id}`, { name });
      nameEl.textContent = name.trim().replace(/\s+/g, " ");
      setStatus(passkeyStatus, "Passkey renamed.", "success");
    } catch (err) {
      setStatus(passkeyStatus, errorMessage(err), "error");
    }
  });
  const del = row.querySelector("[data-delete]");
  del?.addEventListener("click", async () => {
    if (del.getAttribute("aria-disabled") === "true") return;
    const ok = await confirmDialog({
      title: "Delete this passkey?",
      text: `You cannot sign in with "${nameEl.textContent}" afterwards. Your other passkeys keep working.`,
      warning: true,
      confirm: "Delete passkey",
      cancel: "Keep it",
      danger: true,
    });
    if (!ok) return;
    try {
      await api("DELETE", `/api/auth/passkeys/${id}`);
      toastNext("Passkey deleted.", "ok", undefined, '[data-status="passkeys"]');
      location.reload();
    } catch (err) {
      setStatus(passkeyStatus, errorMessage(err), "error");
    }
  });
}

document.querySelector("[data-logout-everywhere]")?.addEventListener("click", async () => {
  const ok = await confirmDialog({
    title: "Sign out everywhere?",
    text: "Every browser that is signed in to this account signs out. This one does too.",
    confirm: "Sign out everywhere",
    cancel: "Stay signed in",
    danger: true,
  });
  if (!ok) return;
  try {
    await api("POST", "/api/auth/logout", { everywhere: true });
    toastNext("You signed out of every session.", "info");
    location.assign("/login");
  } catch (err) {
    toast(errorMessage(err), { kind: "error" });
  }
});

// The editor's text size: the exercise page reads it when it opens.
const size = document.querySelector("[data-editor-size]");
if (size) {
  let stored = "13";
  try {
    stored = localStorage.getItem("gpw-editor-size") || "13";
  } catch {
    // Storage blocked: the default.
  }
  size.value = stored;
  size.addEventListener("change", () => {
    try {
      localStorage.setItem("gpw-editor-size", size.value);
    } catch {
      // Storage blocked: the choice lasts until the page closes.
    }
  });
}
