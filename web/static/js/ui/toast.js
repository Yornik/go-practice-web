// toast.js: the result of an action whose effect is not visible where
// the learner looks. 1 toast at a time; a plain success stays 6 s (longer
// while it has the focus or the pointer). A failure, or a toast with an
// action, stays until it is closed: a keyboard user needs the time to
// reach its button.
//
// The live regions exist from the first paint (layout.html), and a toast
// only adds its text to them, so screen readers announce it.
import { h } from "./dom.js";

let current = null;

/**
 * Shows a toast.
 * @param {string} message in the past tense of the button's verb: "Passkey added."
 * @param {{kind?: "ok"|"info"|"error", action?: {label: string, href?: string, onClick?: () => void}}} [o]
 */
export function toast(message, { kind = "ok", action } = {}) {
  const region = document.querySelector(kind === "error" ? "[data-toasts-alert]" : "[data-toasts]") || document.querySelector("[data-toasts]");
  if (!region) return;
  current?.remove();
  const close = () => {
    clearTimeout(timer);
    el.remove();
    if (current === el) current = null;
  };
  const el = h(
    "div",
    { class: "toast", "data-kind": kind },
    h("p", {}, message),
    action
      ? action.href
        ? h("a", { class: "btn btn-sm", href: action.href }, action.label)
        : h("button", { type: "button", class: "btn btn-sm", onclick: () => { action.onClick?.(); close(); } }, action.label)
      : null,
    h("button", { type: "button", class: "btn btn-sm btn-quiet", onclick: close }, "Close"),
  );
  region.append(el);
  current = el;
  let timer = 0;
  if (kind === "error" || action) return;
  let held = false;
  const arm = () => {
    clearTimeout(timer);
    timer = setTimeout(() => { if (!held) close(); }, 6000);
  };
  const hold = () => { held = true; clearTimeout(timer); };
  const release = () => { held = false; arm(); };
  el.addEventListener("pointerenter", hold);
  el.addEventListener("pointerleave", release);
  el.addEventListener("focusin", hold);
  el.addEventListener("focusout", release);
  arm();
}

const KEY = "gpw-toast";

/**
 * Keeps a result for the next page, for actions that reload or navigate.
 * With target, the next page shows it in that status line, next to the
 * item that changed, instead of a toast that fires while the page loads.
 * @param {string} message
 * @param {"ok"|"info"|"error"} [kind]
 * @param {{label: string, href: string}} [action] a link; a function cannot survive the page
 * @param {string} [target] a selector of a status line on the next page
 */
export function toastNext(message, kind = "ok", action = undefined, target = undefined) {
  try {
    sessionStorage.setItem(KEY, JSON.stringify({ message, kind, action, target }));
  } catch {
    // Without storage the next page shows no message; the action still happened.
  }
}

/** Shows the message a previous page kept. */
export function showPendingToast() {
  try {
    const raw = sessionStorage.getItem(KEY);
    if (!raw) return;
    sessionStorage.removeItem(KEY);
    const { message, kind, action, target } = JSON.parse(raw);
    const line = target ? document.querySelector(target) : null;
    if (line) {
      // After the page settles, so the live region announces it.
      setTimeout(() => {
        line.textContent = message;
        line.dataset.kind = kind === "error" ? "error" : kind === "info" ? "info" : "success";
      }, 600);
      return;
    }
    setTimeout(() => toast(message, { kind, action }), 600);
  } catch {
    // Nothing to show.
  }
}

