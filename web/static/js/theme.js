// theme.js: the 2 display settings of a browser. The colour theme:
// "system" follows the operating system; "light" and "dark" are the
// learner's choice, kept in localStorage under gpw-theme. Story effects:
// on by default, "off" kept under gpw-story (VESPER.md 16.5).
// theme-init.js applies both before the first paint. Any module can
// import this one: it has no side effects beyond the media listener.

const THEME_KEY = "gpw-theme";
const STORY_KEY = "gpw-story";

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
    if (choice === "light" || choice === "dark") localStorage.setItem(THEME_KEY, choice);
    else localStorage.removeItem(THEME_KEY);
  } catch {
    // The choice lasts for this page only.
  }
  document.dispatchEvent(new CustomEvent("gpw:theme", { detail: currentTheme() }));
}

/** Reports whether story effects are on (the default). */
export function storyOn() {
  return document.documentElement.dataset.story !== "off";
}

/**
 * Turns story effects on or off everywhere at once: the storage,
 * data-story on <html> (the CSS swaps each toggle's state word from it),
 * every [data-story-toggle] button's aria-pressed, every
 * [data-story-item] menu item, every input[data-story-check], and a
 * gpw:story event, named like gpw:theme.
 */
export function setStory(on) {
  const root = document.documentElement;
  if (on) delete root.dataset.story;
  else root.dataset.story = "off";
  try {
    if (on) localStorage.removeItem(STORY_KEY);
    else localStorage.setItem(STORY_KEY, "off");
  } catch {
    // The choice lasts for this page only.
  }
  for (const b of document.querySelectorAll("[data-story-toggle]")) b.setAttribute("aria-pressed", String(on));
  for (const item of document.querySelectorAll("[data-story-item]")) item.setAttribute("aria-checked", String(on));
  for (const c of document.querySelectorAll("input[data-story-check]")) c.checked = on;
  document.dispatchEvent(new CustomEvent("gpw:story", { detail: on }));
}

// The system can switch while the page is open.
matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => {
  if (themeChoice() === "system") document.dispatchEvent(new CustomEvent("gpw:theme", { detail: currentTheme() }));
});
