// dashboard.js: each folded tier opens its table with its "Show" button,
// and a tag stamped since the last view fades in on the ring.
for (const button of document.querySelectorAll("[data-fold]")) {
  const target = document.getElementById(button.getAttribute("aria-controls"));
  const label = button.firstChild;
  button.addEventListener("click", () => {
    const open = target.hidden;
    target.hidden = !open;
    button.setAttribute("aria-expanded", String(open));
    label.textContent = open ? "Hide" : "Show";
  });
}

// certificates.js leaves this mark after a claim. The fade answers the
// claim, so it plays once, on the next view of this page (VESPER.md 8.7).
const NEW_TAG = "gpw-ring-new";
try {
  if (sessionStorage.getItem(NEW_TAG)) {
    sessionStorage.removeItem(NEW_TAG);
    document.querySelector("[data-ring]")?.classList.add("gpw-ring-new");
  }
} catch {
  // Storage blocked: the tag shows without the fade.
}
