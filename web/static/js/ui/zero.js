// zero.js: the slashed zero for text that a script writes in Saira, the
// client twin of the template function "zero" (VESPER.md 3.3). Saira's 0
// is the same oval as its O, so the whole run goes in a gpw-zs span and
// each 0 in a gpw-z span, whose ::after draws the slash.

/**
 * Returns a gpw-zs span that holds text with every 0 in a gpw-z span.
 * @param {string|number} text
 * @returns {HTMLSpanElement}
 */
export function zero(text) {
  const run = document.createElement("span");
  run.className = "gpw-zs";
  const parts = String(text).split("0");
  parts.forEach((part, i) => {
    if (i > 0) {
      const z = document.createElement("span");
      z.className = "gpw-z";
      z.textContent = "0";
      run.append(z);
    }
    if (part) run.append(part);
  });
  return run;
}
