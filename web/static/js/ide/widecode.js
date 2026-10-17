// widecode.js: a code block of the spec that is wider than the task
// column, such as the API listing of the Task section, gets "Open wide".
// It shows the block over the editor at its full width; Escape closes it.
// In the column the block wraps with a hanging indent, so nothing hides.
import { h } from "../ui/dom.js";

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
  };
  check();
  let t = 0;
  new ResizeObserver(() => {
    clearTimeout(t);
    t = setTimeout(check, 150);
  }).observe(root.querySelector(".spec-text") || root);
}

function openWide(pre, opener) {
  const heading = pre.closest("section")?.querySelector("h2")?.textContent || "Code";
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
  dialog.querySelector(".reading-head .btn").focus();
}
