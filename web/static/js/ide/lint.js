// lint.js: lint as you type. 1.2 s after the last edit the stub files go
// to check; the diagnostics become Monaco markers and the Problems list. A
// newer edit cancels the request in flight.
import { api, errorMessage } from "../api.js";
import { byteToUtf16Column } from "./monaco.js";
import { explanationElement } from "./explain.js";

const DELAY_MS = 1200;

export class Linter {
  /**
   * @param {object} o
   * @param {any} o.monaco
   * @param {import("./files.js").Files} o.files
   * @param {string} o.slug
   * @param {HTMLElement} o.list the Problems list
   * @param {HTMLElement} o.count the problem counter in the tab
   * @param {(text: string) => void} o.onStatus
   */
  constructor({ monaco, files, slug, list, count, onStatus }) {
    Object.assign(this, { monaco, files, slug, list, count, onStatus });
    this.timer = 0;
    this.inflight = null;
  }

  /** Starts the delay again; called on every edit. */
  schedule() {
    clearTimeout(this.timer);
    this.inflight?.abort();
    this.timer = setTimeout(() => this.run(), DELAY_MS);
  }

  async run() {
    this.inflight?.abort();
    const ctl = new AbortController();
    this.inflight = ctl;
    this.onStatus("Checking…");
    try {
      const res = await api("POST", `/api/exercises/${this.slug}/check`, { files: this.files.stubContents() }, { signal: ctl.signal });
      if (ctl.signal.aborted) return;
      this.show(res.diagnostics || []);
      const n = (res.diagnostics || []).length;
      this.onStatus(n === 0 ? "No problems" : `${n} ${n === 1 ? "problem" : "problems"}`);
    } catch (err) {
      if (err.name === "AbortError") return;
      this.onStatus(`Checks unavailable: ${errorMessage(err)}`);
    } finally {
      if (this.inflight === ctl) this.inflight = null;
    }
  }

  /** Sets markers and fills the Problems list. Also used by format. */
  show(diagnostics) {
    const byFile = new Map();
    for (const d of diagnostics) {
      if (!byFile.has(d.file)) byFile.set(d.file, []);
      byFile.get(d.file).push(d);
    }
    for (const name of Object.keys(this.files.stubContents())) {
      const model = this.files.model(name);
      const markers = (byFile.get(name) || []).map((d) => this.marker(model, d));
      this.monaco.editor.setModelMarkers(model, "go", markers);
    }
    this.renderList(diagnostics);
  }

  marker(model, d) {
    const line = Math.min(Math.max(d.line || 1, 1), model.getLineCount());
    const text = model.getLineContent(line);
    const startColumn = byteToUtf16Column(text, d.column || 1);
    let endLine = d.endLine || line;
    let endColumn;
    if (d.endLine && d.endColumn) {
      endLine = Math.min(endLine, model.getLineCount());
      endColumn = byteToUtf16Column(model.getLineContent(endLine), d.endColumn);
    } else {
      // Only a start: mark the word there, or 1 character.
      const word = model.getWordAtPosition({ lineNumber: line, column: startColumn });
      endColumn = word ? word.endColumn : startColumn + 1;
    }
    const sev = this.monaco.MarkerSeverity;
    return {
      startLineNumber: line,
      startColumn,
      endLineNumber: endLine,
      endColumn,
      message: d.explain?.title ? `${d.message}\n\n${d.explain.title}` : d.message,
      severity: d.severity === "error" ? sev.Error : d.severity === "warning" ? sev.Warning : sev.Info,
      source: d.code ? `${d.source} ${d.code}` : d.source,
    };
  }

  renderList(diagnostics) {
    this.count.textContent = String(diagnostics.length);
    this.list.replaceChildren();
    if (diagnostics.length === 0) {
      const li = document.createElement("li");
      li.className = "muted";
      li.textContent = "No problems found.";
      this.list.appendChild(li);
      return;
    }
    for (const d of diagnostics) {
      const li = document.createElement("li");
      li.className = "problem";
      li.dataset.severity = d.severity;
      const head = document.createElement("div");
      head.className = "problem-head";
      const sev = document.createElement("span");
      sev.className = "problem-severity";
      sev.textContent = d.severity;
      const pos = document.createElement("button");
      pos.type = "button";
      pos.className = "problem-pos";
      pos.textContent = `${d.file}:${d.line}:${d.column}`;
      pos.addEventListener("click", () => {
        const model = this.files.model(d.file);
        if (!model) return;
        const line = Math.min(d.line || 1, model.getLineCount());
        this.files.reveal(d.file, line, byteToUtf16Column(model.getLineContent(line), d.column || 1));
      });
      const src = document.createElement("span");
      src.className = "muted";
      src.textContent = d.code ? `${d.source} ${d.code}` : d.source;
      const msg = document.createElement("span");
      msg.className = "problem-message";
      msg.textContent = d.message;
      head.append(sev, pos, src, msg);
      li.appendChild(head);
      if (d.explain) li.appendChild(explanationElement(d.explain));
      this.list.appendChild(li);
    }
  }
}
