// files.js: the editor's files: 1 Monaco model per file, the file tabs,
// autosave of the stub files and "Reset file".
import { api, errorMessage } from "../api.js";

const AUTOSAVE_MS = 1500;

export class Files {
  /**
   * @param {object} o
   * @param {any} o.monaco
   * @param {any} o.editor
   * @param {string} o.slug
   * @param {{name: string, kind: string, readonly: boolean, content: string}[]} o.files
   * @param {HTMLElement} o.tabs
   * @param {(text: string, kind?: string) => void} o.onSaveStatus
   * @param {() => void} o.onEdit called after every edit of a stub file
   */
  constructor({ monaco, editor, slug, files, tabs, onSaveStatus, onEdit }) {
    this.monaco = monaco;
    this.editor = editor;
    this.slug = slug;
    this.tabs = tabs;
    this.onSaveStatus = onSaveStatus;
    this.onEdit = onEdit;
    this.entries = new Map(); // name -> {file, model, tab, dirty, timer, saving, viewState}
    this.active = null;
    this.quiet = false; // set while the code replaces content, so it is not saved again

    for (const file of files) this.add(file);
    const first = files.find((f) => f.kind === "stub") || files[0];
    if (first) this.open(first.name);
    window.addEventListener("beforeunload", (event) => {
      if ([...this.entries.values()].some((e) => e.dirty)) {
        this.flush();
        event.preventDefault();
      }
    });
  }

  add(file) {
    const uri = this.monaco.Uri.parse(`file:///exercises/${this.slug}/${file.name}`);
    const model = this.monaco.editor.createModel(file.content, "go", uri);
    const tab = document.createElement("button");
    tab.type = "button";
    tab.setAttribute("role", "tab");
    tab.setAttribute("aria-selected", "false");
    tab.tabIndex = -1;
    tab.textContent = file.name;
    tab.dataset.file = file.name;
    if (file.readonly) tab.classList.add("readonly");
    tab.addEventListener("click", () => this.open(file.name));
    tab.addEventListener("keydown", (event) => this.tabKey(event));
    this.tabs.appendChild(tab);
    const entry = { file, model, tab, dirty: false, timer: 0, saving: null, viewState: null };
    this.entries.set(file.name, entry);
    if (!file.readonly) {
      model.onDidChangeContent(() => {
        if (this.quiet) return;
        this.markDirty(entry);
        this.onEdit();
      });
    }
  }

  tabKey(event) {
    const tabs = [...this.tabs.querySelectorAll('[role="tab"]')];
    const i = tabs.indexOf(event.currentTarget);
    let j = -1;
    if (event.key === "ArrowRight") j = (i + 1) % tabs.length;
    if (event.key === "ArrowLeft") j = (i - 1 + tabs.length) % tabs.length;
    if (j >= 0) {
      event.preventDefault();
      tabs[j].focus();
      this.open(tabs[j].dataset.file);
    }
  }

  /** Shows a file in the editor. */
  open(name) {
    const entry = this.entries.get(name);
    if (!entry) return null;
    if (this.active && this.active !== entry) this.active.viewState = this.editor.saveViewState();
    for (const e of this.entries.values()) {
      const on = e === entry;
      e.tab.setAttribute("aria-selected", String(on));
      e.tab.tabIndex = on ? 0 : -1;
    }
    this.editor.setModel(entry.model);
    this.editor.updateOptions({ readOnly: entry.file.readonly });
    if (entry.viewState) this.editor.restoreViewState(entry.viewState);
    this.active = entry;
    return entry;
  }

  /** Opens a file at a line and column and focuses the editor. */
  reveal(name, line, column = 1) {
    if (!this.open(name)) return;
    this.editor.revealLineInCenter(line);
    this.editor.setPosition({ lineNumber: line, column });
    this.editor.focus();
  }

  has(name) {
    return this.entries.has(name);
  }

  model(name) {
    return this.entries.get(name)?.model;
  }

  activeName() {
    return this.active?.file.name;
  }

  /** The content of every stub file, as check wants it. */
  stubContents() {
    const out = {};
    for (const [name, e] of this.entries) if (e.file.kind === "stub") out[name] = e.model.getValue();
    return out;
  }

  markDirty(entry) {
    entry.dirty = true;
    entry.tab.classList.add("dirty");
    this.onSaveStatus("Unsaved changes");
    clearTimeout(entry.timer);
    entry.timer = setTimeout(() => this.save(entry), AUTOSAVE_MS);
  }

  async save(entry) {
    clearTimeout(entry.timer);
    if (!entry.dirty) return entry.saving;
    // Saves of 1 file run in order, so an older body never lands last.
    const previous = entry.saving || Promise.resolve();
    entry.dirty = false;
    const content = entry.model.getValue();
    entry.saving = previous.then(async () => {
      this.onSaveStatus("Saving…");
      try {
        await api("PUT", `/api/exercises/${this.slug}/files/${encodeURIComponent(entry.file.name)}`, { content });
        if (!entry.dirty) entry.tab.classList.remove("dirty");
        this.onSaveStatus(this.anyDirty() ? "Unsaved changes" : "Saved");
      } catch (err) {
        entry.dirty = true;
        this.onSaveStatus(`Not saved: ${errorMessage(err)}`, "error");
        throw err;
      }
    });
    return entry.saving;
  }

  anyDirty() {
    return [...this.entries.values()].some((e) => e.dirty);
  }

  /** Saves every unsaved file now; a test run uses the saved files. */
  async flush() {
    await Promise.all([...this.entries.values()].map((e) => (e.dirty ? this.save(e) : e.saving)));
  }

  /** Replaces a file's content without marking it for saving. */
  replace(name, content) {
    const entry = this.entries.get(name);
    if (!entry) return;
    this.quiet = true;
    try {
      // pushEditOperations keeps undo working, unlike setValue.
      entry.model.pushEditOperations([], [{ range: entry.model.getFullModelRange(), text: content }], () => null);
    } finally {
      this.quiet = false;
    }
  }

  /** Restores the stub of the active file after a confirmation. */
  async resetActive() {
    const entry = this.active;
    if (!entry || entry.file.readonly) return;
    if (!confirm(`Reset ${entry.file.name} to the original stub? Your version of this file is lost.`)) return;
    clearTimeout(entry.timer);
    entry.dirty = false;
    try {
      const res = await api("POST", `/api/exercises/${this.slug}/files/${encodeURIComponent(entry.file.name)}/reset`, {});
      this.replace(entry.file.name, res.content);
      entry.tab.classList.remove("dirty");
      this.onSaveStatus("Saved");
      this.onEdit();
    } catch (err) {
      this.onSaveStatus(`Reset failed: ${errorMessage(err)}`, "error");
    }
  }
}
