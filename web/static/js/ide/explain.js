// explain.js: renders the explanations the server attaches to diagnostics
// and failed tests. The server sends the body as safe HTML (rendered from
// the reviewed catalog, every captured value escaped); the title is plain
// text and is set as text.

/**
 * Returns a <details> element for 1 explanation.
 * @param {{id: string, title: string, html: string, generic?: boolean, raw?: object}} item
 * @param {boolean} [open]
 */
export function explanationElement(item, open = false) {
  const details = document.createElement("details");
  details.className = "explanation";
  details.dataset.explain = item.id;
  if (item.generic) details.dataset.generic = "true";
  details.open = open;
  const summary = document.createElement("summary");
  summary.textContent = item.title;
  details.appendChild(summary);
  const raw = item.raw || {};
  if (raw.message) {
    const parts = document.createElement("p");
    parts.className = "explanation-raw";
    const where = raw.file ? `${raw.file}${raw.line ? ":" + raw.line : ""}${raw.column ? ":" + raw.column : ""}` : "";
    parts.textContent = [where && `File ${where}`, raw.code && `Check ${raw.code}`, raw.test && `Test ${raw.test}`, `Message: ${raw.message}`]
      .filter(Boolean)
      .join(" · ");
    details.appendChild(parts);
  }
  const body = document.createElement("div");
  body.className = "prose";
  body.innerHTML = item.html; // safe HTML from the server's catalog, see above
  details.appendChild(body);
  return details;
}
