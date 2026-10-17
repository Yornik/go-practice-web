// dialog.js: modal questions as native <dialog> elements, in place of
// confirm(), prompt() and alert(). showModal() traps the focus and
// Escape closes; the focus goes back to the control that opened it.
import { h } from "./dom.js";

let counter = 0;

/**
 * Opens a dialog and resolves with the value of the button that closed it
 * ("" for Escape or Cancel).
 * @param {object} o
 * @param {string} o.title
 * @param {(Node|string)[]} o.body
 * @param {{label: string, value: string, kind?: string, safe?: boolean}[]} o.buttons
 * @param {(form: HTMLFormElement) => string} [o.check] returns an error for the field, or ""
 * @param {(dialog: HTMLDialogElement) => void} [o.onOpen]
 */
export function openDialog({ title, body, buttons, check, onOpen }) {
  const opener = document.activeElement;
  const id = `dialog-${++counter}`;
  return new Promise((resolve) => {
    const form = h("form", { method: "dialog", novalidate: true });
    const dialog = h("dialog", { class: "dialog", "aria-labelledby": `${id}-title` }, form);
    form.append(h("h2", { id: `${id}-title` }, title), ...body.filter(Boolean));
    const row = h("div", { class: "btn-row" });
    for (const b of buttons) {
      row.append(
        h("button", {
          type: b.value ? "submit" : "button",
          class: `btn ${b.kind === "primary" ? "btn-primary" : b.kind === "danger" ? "btn-danger" : ""}`.trim(),
          value: b.value,
          "data-safe": b.safe ? "true" : null,
          onclick: b.value ? null : () => dialog.close(""),
        }, b.label),
      );
    }
    form.append(row);
    form.addEventListener("submit", (event) => {
      const submitter = event.submitter;
      if (submitter && submitter.value && check) {
        const err = check(form);
        if (err) {
          event.preventDefault();
          return;
        }
      }
    });
    dialog.addEventListener("close", () => {
      dialog.remove();
      if (opener && typeof opener.focus === "function" && document.contains(opener)) opener.focus();
      resolve(dialog.returnValue || "");
    });
    document.body.append(dialog);
    dialog.returnValue = "";
    dialog.showModal();
    // The safe choice gets the focus in a destructive dialog.
    const safe = form.querySelector('[data-safe="true"]');
    const first = form.querySelector("input, textarea, select");
    (safe || first || form.querySelector("button"))?.focus();
    onOpen?.(dialog);
  });
}

/**
 * Asks to confirm an action. A destructive question starts with the
 * WARNING label and puts the focus on the safe button.
 * @returns {Promise<boolean>}
 */
export async function confirmDialog({ title, text, warning = false, confirm, cancel = "Cancel", danger = false }) {
  const p = h("p");
  if (warning) p.append(h("b", {}, "WARNING:"), " ");
  p.append(text);
  const v = await openDialog({
    title,
    body: [p],
    buttons: [
      { label: confirm, value: "ok", kind: danger ? "danger" : "primary" },
      { label: cancel, value: "", safe: danger },
    ],
  });
  return v === "ok";
}

/**
 * Asks for 1 line of text. Resolves with the text, or null for Cancel.
 * @returns {Promise<string|null>}
 */
export async function promptDialog({ title, label, value = "", help = "", confirm, cancel = "Cancel", maxLength = 64, validate }) {
  const fid = `prompt-${++counter}`;
  const input = h("input", { class: "input", id: fid, value, maxlength: maxLength, autocomplete: "off", "aria-describedby": help ? `${fid}-help` : null });
  const error = h("p", { class: "field-error", id: `${fid}-error`, hidden: true });
  const field = h("div", { class: "field" }, h("label", { for: fid }, label), input, help ? h("p", { class: "help", id: `${fid}-help` }, help) : null, error);
  const v = await openDialog({
    title,
    body: [field],
    buttons: [
      { label: confirm, value: "ok", kind: "primary" },
      { label: cancel, value: "" },
    ],
    check: () => {
      const msg = validate ? validate(input.value) : "";
      error.textContent = msg;
      error.hidden = !msg;
      input.setAttribute("aria-invalid", msg ? "true" : "false");
      if (msg) {
        input.setAttribute("aria-describedby", `${fid}-error`);
        input.focus();
      }
      return msg;
    },
    onOpen: () => input.select(),
  });
  return v === "ok" ? input.value : null;
}

/** Tells the learner something that needs an answer before going on. */
export async function messageDialog({ title, text, close = "Close" }) {
  await openDialog({ title, body: [h("p", {}, text)], buttons: [{ label: close, value: "ok", kind: "primary" }] });
}
