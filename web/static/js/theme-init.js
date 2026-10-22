// Runs before the first paint (a classic script in <head>), so a page
// never flashes in the wrong theme or shows story parts the learner
// turned off. "light" or "dark" is the learner's choice; without one the
// system setting decides through CSS. gpw-story is "off" or absent
// (VESPER.md 16.5).
(function () {
  try {
    var t = localStorage.getItem("gpw-theme");
    if (t === "light" || t === "dark") document.documentElement.dataset.theme = t;
    if (localStorage.getItem("gpw-story") === "off") document.documentElement.dataset.story = "off";
  } catch (e) {
    // Storage can be blocked; the system theme and the story still work.
  }
})();
