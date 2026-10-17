// account.js: profile, passkeys and sessions.
import { api, errorMessage, setStatus } from "../api.js";
import { createPasskey } from "../webauthn.js";

const profile = document.getElementById("profile-form");
const profileStatus = document.querySelector('[data-status="profile"]');
const passkeyStatus = document.querySelector('[data-status="passkeys"]');

profile?.addEventListener("submit", async (event) => {
  event.preventDefault();
  const data = new FormData(profile);
  try {
    const res = await api("PUT", "/api/account", {
      displayName: String(data.get("displayName") || ""),
      email: String(data.get("email") || ""),
    });
    const msg = res.verificationSent
      ? `Saved. We sent a verification link to ${res.pendingEmail}.`
      : "Saved.";
    setStatus(profileStatus, msg, "success");
  } catch (err) {
    setStatus(profileStatus, errorMessage(err), "error");
  }
});

document.querySelector("[data-resend]")?.addEventListener("click", async () => {
  try {
    const res = await api("POST", "/api/auth/verify-email/resend", {});
    setStatus(profileStatus, `We sent a new link to ${res.email}.`, "success");
  } catch (err) {
    setStatus(profileStatus, errorMessage(err), "error");
  }
});

document.querySelector("[data-add-passkey]")?.addEventListener("click", async (event) => {
  const button = event.currentTarget;
  button.disabled = true;
  setStatus(passkeyStatus, "Creating a passkey…");
  try {
    const begin = await api("POST", "/api/auth/passkeys/begin", {});
    const credential = await createPasskey(begin.options);
    const name = prompt("Name this passkey, for example the device it is on:", "") || "";
    await api("POST", "/api/auth/passkeys/finish", { ceremony: begin.ceremony, credential, name });
    location.reload();
  } catch (err) {
    setStatus(passkeyStatus, errorMessage(err), "error");
    button.disabled = false;
  }
});

for (const row of document.querySelectorAll("[data-passkey]")) {
  const id = row.dataset.passkey;
  const nameEl = row.querySelector("[data-passkey-name]");
  row.querySelector("[data-rename]")?.addEventListener("click", async () => {
    const name = prompt("New name for this passkey:", nameEl.textContent);
    if (name === null) return;
    try {
      await api("PUT", `/api/auth/passkeys/${id}`, { name });
      nameEl.textContent = name.trim().replace(/\s+/g, " ");
      setStatus(passkeyStatus, "Renamed.", "success");
    } catch (err) {
      setStatus(passkeyStatus, errorMessage(err), "error");
    }
  });
  row.querySelector("[data-delete]")?.addEventListener("click", async () => {
    if (!confirm(`Remove the passkey "${nameEl.textContent}"? You cannot sign in with it afterwards.`)) return;
    try {
      await api("DELETE", `/api/auth/passkeys/${id}`);
      location.reload();
    } catch (err) {
      setStatus(passkeyStatus, errorMessage(err), "error");
    }
  });
}

document.querySelector("[data-logout-everywhere]")?.addEventListener("click", async () => {
  if (!confirm("Sign out on every browser, this one included?")) return;
  try {
    await api("POST", "/api/auth/logout", { everywhere: true });
    location.assign("/login");
  } catch (err) {
    alert(errorMessage(err));
  }
});
