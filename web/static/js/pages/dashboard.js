// dashboard.js: each tier's table folds with its "Show" and "Hide" button.
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
