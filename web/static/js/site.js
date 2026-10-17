// site.js: what every page shares: the menus, the theme control, the
// small-screen navigation and sign-out. Pages load it with a versioned
// URL; other modules must not import it, or it runs twice (a module
// instance is per URL). They import theme.js and ui/*.js.
import { api, errorMessage } from "./api.js";
import { setTheme, themeChoice } from "./theme.js";
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

// Below 720 px the navigation folds into the "Menu" button.
const navToggle = document.querySelector("[data-nav-toggle]");
const navSheet = document.querySelector("[data-nav-sheet]");
const setNav = (open) => {
  navSheet.hidden = !open;
  navToggle.setAttribute("aria-expanded", String(open));
  navToggle.textContent = open ? "Close menu" : "Menu";
};
navToggle?.addEventListener("click", () => setNav(navSheet.hidden));
// Escape closes the sheet and gives the focus back to its button.
navSheet?.addEventListener("keydown", (event) => {
  if (event.key !== "Escape" || navSheet.hidden) return;
  event.preventDefault();
  setNav(false);
  navToggle.focus();
});

// Tables that stack into lists on a phone keep their table semantics:
// Safari drops them when CSS changes the display of a table.
for (const table of document.querySelectorAll("table.table-stack")) {
  table.setAttribute("role", "table");
  for (const el of table.querySelectorAll("thead, tbody")) el.setAttribute("role", "rowgroup");
  for (const tr of table.querySelectorAll("tr")) tr.setAttribute("role", "row");
  for (const th of table.querySelectorAll("th")) th.setAttribute("role", th.getAttribute("scope") === "row" ? "rowheader" : "columnheader");
  for (const td of table.querySelectorAll("td")) td.setAttribute("role", "cell");
}

// A code block that scrolls sideways takes the focus, so a keyboard can
// scroll it in every browser.
const scrollers = () => {
  for (const pre of document.querySelectorAll(".prose pre, .spec-text pre, .xsec-body pre, .reading-body pre")) {
    const wide = pre.scrollWidth > pre.clientWidth + 1;
    if (wide && !pre.hasAttribute("tabindex")) {
      pre.tabIndex = 0;
      pre.setAttribute("role", "region");
      pre.setAttribute("aria-label", pre.closest("section")?.querySelector("h2, h3")?.textContent ? `Code example: ${pre.closest("section").querySelector("h2, h3").textContent}` : "Code example");
    } else if (!wide && pre.getAttribute("role") === "region") {
      pre.removeAttribute("tabindex");
      pre.removeAttribute("role");
      pre.removeAttribute("aria-label");
    }
  }
};
document.fonts.ready.then(scrollers);
let scrollerFrame = 0;
addEventListener("resize", () => {
  cancelAnimationFrame(scrollerFrame);
  scrollerFrame = requestAnimationFrame(scrollers);
});

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
