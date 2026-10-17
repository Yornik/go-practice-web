// Runs before the first paint (a classic script in <head>), so a page
// never flashes in the wrong theme. "light" or "dark" is the user's choice;
// without one the system setting decides through CSS.
(function () {
  try {
    var t = localStorage.getItem("gpw-theme");
    if (t === "light" || t === "dark") document.documentElement.dataset.theme = t;
  } catch (e) {
    // Storage can be blocked; the system theme still works.
  }
})();
