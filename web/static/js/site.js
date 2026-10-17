// site.js: behaviour every page shares: the theme toggle and sign-out.
// Pages load it with a versioned URL; other modules must not import it,
// or it runs twice (a module instance is per URL). They import theme.js.
import { api, errorMessage } from "./api.js";
import { currentTheme } from "./theme.js";

const root = document.documentElement;
const toggle = document.querySelector("[data-theme-toggle]");

function showTheme() {
  if (toggle) toggle.setAttribute("aria-pressed", String(currentTheme() === "dark"));
}

toggle?.addEventListener("click", () => {
  const next = currentTheme() === "dark" ? "light" : "dark";
  root.dataset.theme = next;
  try {
    localStorage.setItem("gpw-theme", next);
  } catch {
    // The choice lasts for this page only.
  }
  showTheme();
  document.dispatchEvent(new CustomEvent("gpw:theme", { detail: next }));
});
showTheme();

document.querySelector("[data-logout]")?.addEventListener("click", async () => {
  try {
    const res = await api("POST", "/api/auth/logout", {});
    location.assign(res.next || "/");
  } catch (err) {
    alert(errorMessage(err));
  }
});
