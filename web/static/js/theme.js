// theme.js: the colour theme. "system" follows the operating system;
// "light" and "dark" are the learner's choice, kept in localStorage under
// gpw-theme (theme-init.js applies it before the first paint). Any module
// can import this one: it has no side effects.

const KEY = "gpw-theme";

/** "system", "light" or "dark": what the learner chose. */
export function themeChoice() {
  const t = document.documentElement.dataset.theme;
  return t === "light" || t === "dark" ? t : "system";
}

/** "light" or "dark": what the page shows now. */
export function currentTheme() {
  const t = themeChoice();
  if (t !== "system") return t;
  return matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

/** Reports whether the page shows the dark theme. */
export function isDark() {
  return currentTheme() === "dark";
}

/** Applies and stores a choice, and tells the page with a gpw:theme event. */
export function setTheme(choice) {
  const root = document.documentElement;
  if (choice === "light" || choice === "dark") root.dataset.theme = choice;
  else delete root.dataset.theme;
  try {
    if (choice === "light" || choice === "dark") localStorage.setItem(KEY, choice);
    else localStorage.removeItem(KEY);
  } catch {
    // The choice lasts for this page only.
  }
  document.dispatchEvent(new CustomEvent("gpw:theme", { detail: currentTheme() }));
}

// The system can switch while the page is open.
matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => {
  if (themeChoice() === "system") document.dispatchEvent(new CustomEvent("gpw:theme", { detail: currentTheme() }));
});
