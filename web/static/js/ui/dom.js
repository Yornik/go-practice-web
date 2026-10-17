// dom.js: builds DOM nodes from code. Text always goes in as text, so
// nothing a user or a tool wrote can become markup by accident.

/**
 * Creates an element. attrs: "class", "text", "html" (only for HTML the
 * server marked safe), "on<event>" handlers, "dataset" (an object),
 * booleans (true sets an empty attribute, false leaves it out) and
 * strings.
 * @param {string} tag
 * @param {Record<string, any>} [attrs]
 * @param {...(Node|string|null|undefined|false)} children
 */
export function h(tag, attrs = {}, ...children) {
  const el = tag.startsWith("svg:")
    ? document.createElementNS("http://www.w3.org/2000/svg", tag.slice(4))
    : document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v === undefined || v === null || v === false) continue;
    if (k === "class") el.setAttribute("class", v);
    else if (k === "text") el.textContent = v;
    else if (k === "html") el.innerHTML = v;
    else if (k === "dataset") Object.assign(el.dataset, v);
    else if (k.startsWith("on") && typeof v === "function") el.addEventListener(k.slice(2), v);
    else el.setAttribute(k, v === true ? "" : String(v));
  }
  append(el, children);
  return el;
}

function append(el, children) {
  for (const c of children) {
    if (c === null || c === undefined || c === false) continue;
    if (Array.isArray(c)) append(el, c);
    else el.append(c instanceof Node ? c : String(c));
  }
}

/** An icon from the sprite in the layout: tick, cross, dash, ring, arc, chevron, dot. */
export function icon(name, cls = "") {
  const svg = h("svg:svg", { class: `icon ${cls}`.trim(), "aria-hidden": "true", focusable: "false" });
  svg.append(h("svg:use", { href: `#i-${name}` }));
  return svg;
}

/** A balloon: a number or letter in a circle, with an accessible name. */
export function balloon(text, { label, cls = "" } = {}) {
  return h("span", { class: `balloon ${cls}`.trim(), role: label ? "img" : null, "aria-label": label, "aria-hidden": label ? null : "true" }, text);
}

/** Inline text with `code` spans for backticks, as plain DOM nodes. */
export function codeText(text) {
  const out = [];
  const parts = String(text).split("`");
  parts.forEach((p, i) => {
    if (!p) return;
    out.push(i % 2 === 1 ? h("code", {}, p) : p);
  });
  return out;
}

/** Reads a value from localStorage; null when storage is blocked. */
export function load(key) {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

/** Writes a value to localStorage; the choice lasts for this page when storage is blocked. */
export function save(key, value) {
  try {
    if (value === null) localStorage.removeItem(key);
    else localStorage.setItem(key, String(value));
  } catch {
    // Storage can be blocked; nothing else depends on it.
  }
}

/** Reports whether the user asked for less motion. */
export function reducedMotion() {
  return matchMedia("(prefers-reduced-motion: reduce)").matches;
}
