// site.js: what every page shares: the menus, the theme and story
// controls, the small-screen navigation and sign-out. Pages load it with a
// versioned URL; other modules must not import it, or it runs twice (a
// module instance is per URL). They import theme.js and ui/*.js.
import { api, errorMessage } from "./api.js";
import { setStory, setTheme, storyOn, themeChoice } from "./theme.js";
import { nearestHeading } from "./ui/dom.js";
import { setupMenus } from "./ui/menu.js";
import { showPendingToast, toast } from "./ui/toast.js";

setupMenus();

function showTheme() {
  const choice = themeChoice();
  for (const item of document.querySelectorAll("[data-theme-choice]")) {
    item.setAttribute("aria-checked", String(item.dataset.themeChoice === choice));
  }
  for (const radio of document.querySelectorAll("[data-theme-radio]")) radio.checked = radio.value === choice;
}

for (const item of document.querySelectorAll("[data-theme-choice]")) {
  item.addEventListener("click", () => {
    setTheme(item.dataset.themeChoice);
    showTheme();
  });
}
for (const radio of document.querySelectorAll("[data-theme-radio]")) {
  radio.addEventListener("change", () => {
    if (radio.checked) {
      setTheme(radio.value);
      showTheme();
    }
  });
}
document.addEventListener("gpw:theme", showTheme);
showTheme();

// Story effects: the header toggle, the theme menu's 4th item and the
// account page's checkbox all go through setStory, which keeps them in
// sync. The server renders them "on"; this sets what theme-init.js read.
document.addEventListener("click", (event) => {
  if (event.target.closest("[data-story-toggle], [data-story-item]")) setStory(!storyOn());
});
document.addEventListener("change", (event) => {
  if (event.target.matches("input[data-story-check]")) setStory(event.target.checked);
});
setStory(storyOn());

// At 1060 px and less the navigation folds into the "Menu" button.
const navToggle = document.querySelector("[data-nav-toggle]");
const navSheet = document.querySelector("[data-nav-sheet]");
const setNav = (open) => {
  navSheet.hidden = !open;
  navToggle.setAttribute("aria-expanded", String(open));
  navToggle.textContent = open ? "Close menu" : "Menu";
};
navToggle?.addEventListener("click", () => setNav(navSheet.hidden));
// Escape closes the sheet and gives the focus back to its button, also
// when the focus is on that button. A menu inside the sheet that Escape
// closed first marks the event as handled. While a modal dialog is open,
// Escape is the dialog's: the sheet's button is inert behind it, and
// preventDefault here would cancel the dialog's own close.
document.addEventListener("keydown", (event) => {
  if (event.key !== "Escape" || event.defaultPrevented || !navSheet || navSheet.hidden) return;
  if (document.querySelector("dialog[open]")) return;
  event.preventDefault();
  setNav(false);
  navToggle.focus();
});

// Tables that stack into lists on a phone keep their table semantics:
// Safari drops them when CSS changes the display of a table.
for (const table of document.querySelectorAll("table.table-stack, table.gpw-tbl, table.gpw-act, table.gpw-pk, table.gpw-adm")) {
  table.setAttribute("role", "table");
  for (const el of table.querySelectorAll("thead, tbody")) el.setAttribute("role", "rowgroup");
  for (const tr of table.querySelectorAll("tr")) tr.setAttribute("role", "row");
  for (const th of table.querySelectorAll("th")) th.setAttribute("role", th.getAttribute("scope") === "row" ? "rowheader" : "columnheader");
  for (const td of table.querySelectorAll("td")) td.setAttribute("role", "cell");
}

// A code block or a table that scrolls sideways takes the focus, so a
// keyboard can scroll it in every browser (WCAG 2.1.1). A ResizeObserver
// catches every change of width: the window, an IDE column, the fonts. A
// table scrolls in its box (gpw-tscroll, from the Markdown renderer): the
// box takes the region role, so the table keeps its own.
const SCROLLERS = ".prose pre, .spec-text pre, .xsec-body pre, .reading-body pre, .reading-inner pre, .gpw-tscroll";
const scroller = (el) => {
  const wide = el.scrollWidth > el.clientWidth + 1;
  if (wide && !el.hasAttribute("tabindex")) {
    const what = el.classList.contains("gpw-tscroll") ? "Table" : "Code example";
    const heading = nearestHeading(el);
    el.tabIndex = 0;
    el.setAttribute("role", "region");
    el.setAttribute("aria-label", heading ? `${what}: ${heading}` : what);
    el.dataset.scroller = "";
  } else if (!wide && el.hasAttribute("data-scroller")) {
    el.removeAttribute("tabindex");
    el.removeAttribute("role");
    el.removeAttribute("aria-label");
    delete el.dataset.scroller;
  }
};
const scrollWatch = new ResizeObserver((entries) => {
  for (const entry of entries) scroller(entry.target);
});
const watchScrollers = (root) => {
  for (const el of root.querySelectorAll(SCROLLERS)) scrollWatch.observe(el);
};
document.fonts.ready.then(() => watchScrollers(document));
// The IDE adds the reading sheets later and says so with this event.
document.addEventListener("gpw:content", (event) => watchScrollers(event.target));

for (const button of document.querySelectorAll("[data-logout]")) {
  button.addEventListener("click", async () => {
    try {
      const res = await api("POST", "/api/auth/logout", {});
      location.assign(res.next || "/");
    } catch (err) {
      toast(`You are still signed in. ${errorMessage(err)}`, { kind: "error" });
    }
  });
}

showPendingToast();
