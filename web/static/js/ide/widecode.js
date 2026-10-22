// widecode.js: a code block of the spec that is wider than the task
// column, such as the API listing of the Task section, gets "Open wide".
// It shows the block over the editor at its full width; Escape closes it.
// In the column the block wraps with a hanging indent, so nothing hides.
import { h, nearestHeading } from "../ui/dom.js";

/** Adds "Open wide" to the spec's code blocks that need it, now and after a resize. */
export function setupWideCode(root) {
  const blocks = [...root.querySelectorAll(".spec-text pre")];
  if (!blocks.length) return;
  const probe = h("span", { class: "ch-probe", "aria-hidden": "true" }, "0".repeat(10));
  const check = () => {
    for (const pre of blocks) {
      if (!probe.isConnected) pre.append(probe);
      const ch = probe.getBoundingClientRect().width / 10 || 7.2;
      probe.remove();
      const longest = Math.max(...pre.textContent.split("\n").map((l) => l.replace(/\t/g, "    ").length));
      const fits = longest * ch <= pre.clientWidth - 24;
      let button = pre.previousElementSibling?.matches?.(".wide-open") ? pre.previousElementSibling : null;
      if (fits) {
        button?.remove();
        continue;
      }
      if (!button) {
        button = h("button", { type: "button", class: "btn btn-quiet wide-open", onclick: () => openWide(pre, button) }, "Open wide");
        pre.before(button);
      }
    }
    nameButtons(blocks);
  };
  check();
  let t = 0;
  new ResizeObserver(() => {
    clearTimeout(t);
    t = setTimeout(check, 150);
  }).observe(root.querySelector(".spec-text") || root);
}

/**
 * Names each "Open wide" after the heading over its block, and numbers the
 * blocks under 1 heading, so a list of buttons tells them apart (WCAG
 * 2.4.6). The name starts with the visible words (2.5.3).
 */
function nameButtons(blocks) {
  const named = blocks.map((pre) => ({
    heading: nearestHeading(pre) || "Code",
    button: pre.previousElementSibling?.matches?.(".wide-open") ? pre.previousElementSibling : null,
  }));
  const count = new Map();
  for (const b of named) count.set(b.heading, (count.get(b.heading) || 0) + 1);
  const seen = new Map();
  for (const b of named) {
    const n = (seen.get(b.heading) || 0) + 1;
    seen.set(b.heading, n);
    b.button?.setAttribute("aria-label", count.get(b.heading) > 1 ? `Open wide: ${b.heading}, code block ${n}` : `Open wide: ${b.heading}`);
  }
}

function openWide(pre, opener) {
  const heading = nearestHeading(pre) || "Code";
  const close = () => dialog.close();
  const dialog = h("dialog", { class: "reading-sheet wide-code", "aria-labelledby": "wide-title" },
    h("div", { class: "reading-head" },
      h("h2", { id: "wide-title" }, heading),
      h("button", { type: "button", class: "btn btn-primary", onclick: close }, "Close")),
    h("div", { class: "reading-inner" }, pre.cloneNode(true)));
  const task = document.getElementById("task-col");
  const right = task && !task.hidden && innerWidth >= 900 ? task.getBoundingClientRect().right : 0;
  dialog.style.setProperty("--sheet-w", `${innerWidth - right}px`);
  dialog.addEventListener("close", () => {
    dialog.remove();
    opener.focus();
  });
  document.body.append(dialog);
  dialog.showModal();
  // site.js makes the copy a focusable region when it still scrolls
  // sideways, so a keyboard can reach the cut-off code (WCAG 2.1.1).
  dialog.dispatchEvent(new CustomEvent("gpw:content", { bubbles: true }));
  dialog.querySelector(".reading-head .btn").focus();
}
