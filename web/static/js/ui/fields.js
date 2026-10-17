// fields.js: errors that belong to 1 field: under the field, tied to it
// with aria-describedby and aria-invalid, and starting with its label, so
// the message says which field it is about.

/** Shows message under input. */
export function fieldError(input, message) {
  const id = `${input.id}-error`;
  let p = document.getElementById(id);
  if (!p) {
    p = document.createElement("p");
    p.id = id;
    p.className = "field-error";
    input.closest(".field")?.append(p) || input.after(p);
  }
  p.textContent = message;
  input.setAttribute("aria-invalid", "true");
  const ids = new Set((input.getAttribute("aria-describedby") || "").split(/\s+/).filter(Boolean));
  ids.add(id);
  input.setAttribute("aria-describedby", [...ids].join(" "));
}

/** Removes every field error of a form. */
export function clearFieldErrors(form) {
  for (const input of form.querySelectorAll('[aria-invalid="true"]')) {
    input.removeAttribute("aria-invalid");
    const id = `${input.id}-error`;
    document.getElementById(id)?.remove();
    const ids = (input.getAttribute("aria-describedby") || "").split(/\s+/).filter((x) => x && x !== id);
    if (ids.length) input.setAttribute("aria-describedby", ids.join(" "));
    else input.removeAttribute("aria-describedby");
  }
}

/** Moves the focus to the first field with an error; reports whether there was one. */
export function focusFirstInvalid(form) {
  const first = form.querySelector('[aria-invalid="true"]');
  first?.focus();
  return Boolean(first);
}

/** Marks a button as busy without disabling it, so it keeps the keyboard focus. */
export function setBusy(button, busy, text) {
  if (busy) {
    button.setAttribute("aria-disabled", "true");
    button.setAttribute("aria-busy", "true");
  } else {
    button.removeAttribute("aria-disabled");
    button.removeAttribute("aria-busy");
  }
  if (text) button.textContent = text;
}

/** Reports whether a button is busy; its click handler then does nothing. */
export function isBusy(button) {
  return button.getAttribute("aria-disabled") === "true";
}

/** A simple check for an email address: something@something.something. */
export function looksLikeEmail(s) {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(s);
}
