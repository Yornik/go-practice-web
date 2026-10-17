// theme.js: the current colour theme, without side effects, so any module
// can import it. site.js owns the toggle.

/** "light" or "dark": the user's choice, else the system setting. */
export function currentTheme() {
  const t = document.documentElement.dataset.theme;
  if (t === "light" || t === "dark") return t;
  return matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

/** Reports whether the page shows the dark theme. */
export function isDark() {
  return currentTheme() === "dark";
}
