// files.js: the editor's files: 1 Monaco model per file, the file tabs,
// the "Tests" menu, autosave of the stub files and the reset of a stub.
//
// The stub tabs are pinned: they never scroll away, so the learner always
// sees the file they write. A test file opens in 1 preview tab after them;
// the next test file replaces it.
//
// Autosave keeps going when the network does not: a failed save is tried
// again with a growing delay, the page says so in a banner, and the
// unsaved code stays in this browser until the server has it.
import { api, ApiError, errorMessage } from "../api.js";
import { h, icon, load, save } from "../ui/dom.js";
import { confirmDialog } from "../ui/dialog.js";
import { clock } from "../ui/format.js";

const AUTOSAVE_MS = 1500;
const RETRY_MS = [2000, 4000, 8000, 15000, 30000];

export class Files {
  /**
   * @param {object} o
   * @param {any} o.monaco
   * @param {any} o.editor
   * @param {string} o.slug
   * @param {{name: string, kind: string, readonly: boolean, content: string, updatedAt?: string}[]} o.files
   * @param {HTMLElement} o.tabs the tablist
   * @param {HTMLElement} o.testMenu the menu of test files
   * @param {HTMLElement} [o.banner] the place for "Your code is not saved"
   * @param {(state: {kind: string, text: string}) => void} o.onSaveStatus
   * @param {(name: string) => void} o.onEdit called after every edit of a stub file
   * @param {(name: string) => void} o.onOpen called when a file is shown
   * @param {(text: string) => void} [o.alert] tells a screen reader about a failed save
   * @param {() => void} [o.format] formats the active file
   */
  constructor(o) {
    Object.assign(this, o);
    this.entries = new Map(); // name -> {file, model, tab, dirty, timer, saving, viewState, retry}
    this.active = null;
    this.testTab = null;
    this.quiet = false; // set while code replaces content, so it is not saved again
    this.failing = null; // the error of the last failed save, until a save works

    for (const file of o.files) this.add(file);
    this.buildTestMenu();
    const first = o.files.find((f) => f.kind === "stub") || o.files[0];
    if (first) this.open(first.name, { focus: false });
    const saved = o.files.filter((f) => f.updatedAt).map((f) => new Date(f.updatedAt)).sort((a, b) => b - a)[0];
    this.lastSaved = saved || null;
    this.onSaveStatus(saved ? { kind: "ok", text: `Saved at ${clock(saved)}` } : { kind: "", text: "The stub, not changed yet" });
    this.offerBackups();

    // A tab that closes or goes to the background saves at once; keepalive
    // lets the request finish after the page is gone.
    const flushNow = () => {
      if (this.anyDirty()) this.flush({ keepalive: true }).catch(() => {});
    };
    addEventListener("pagehide", flushNow);
    document.addEventListener("visibilitychange", () => {
      if (document.visibilityState === "hidden") flushNow();
    });
    addEventListener("beforeunload", (event) => {
      if (this.anyDirty()) {
        flushNow();
        event.preventDefault();
      }
    });
  }

  add(file) {
    const uri = this.monaco.Uri.parse(`file:///exercises/${this.slug}/${file.name}`);
    const model = this.monaco.editor.createModel(file.content, "go", uri);
    const entry = { file, model, tab: null, dirty: false, timer: 0, saving: null, viewState: null, retry: 0, backupTimer: 0 };
    this.entries.set(file.name, entry);
    if (file.kind === "stub") entry.tab = this.makeTab(file.name, false);
    if (!file.readonly) {
      model.onDidChangeContent(() => {
        if (this.quiet) return;
        this.markDirty(entry);
        this.onEdit(file.name);
      });
    }
  }

  makeTab(name, readonly) {
    const tab = h("button", {
      type: "button", class: `file-tab${readonly ? " is-test" : ""}`, role: "tab", "aria-selected": "false", tabindex: "-1", dataset: { file: name },
      onclick: () => this.open(tab.dataset.file),
      onkeydown: (event) => this.tabKey(event),
    },
    h("span", { class: "file-name" }, name),
    readonly ? h("span", { class: "ro" }, "read only") : null,
    h("span", { class: "dirty", hidden: true }, icon("dot"), h("span", { class: "visually-hidden" }, "unsaved")),
    h("span", { class: "count-badge", hidden: true }));
    this.tabs.append(tab);
    return tab;
  }

  buildTestMenu() {
    if (!this.testMenu) return;
    this.testMenu.replaceChildren();
    for (const [name, e] of this.entries) {
      if (e.file.kind !== "test") continue;
      this.testMenu.append(h("button", { type: "button", role: "menuitem", onclick: () => this.open(name) }, name));
    }
    this.formatItem = h("button", { type: "button", role: "menuitem", class: "narrow-only", onclick: () => this.format?.() }, "Format the file");
    this.resetItem = h("button", { type: "button", role: "menuitem", class: "danger-item", onclick: () => this.resetActive() }, "Reset");
    this.testMenu.append(h("hr", { role: "separator" }), this.formatItem, this.resetItem);
  }

  tabKey(event) {
    const tabs = [...this.tabs.querySelectorAll('[role="tab"]:not([hidden])')];
    const i = tabs.indexOf(event.currentTarget);
    let j = -1;
    if (event.key === "ArrowRight") j = (i + 1) % tabs.length;
    if (event.key === "ArrowLeft") j = (i - 1 + tabs.length) % tabs.length;
    if (event.key === "Home") j = 0;
    if (event.key === "End") j = tabs.length - 1;
    if (j >= 0) {
      event.preventDefault();
      tabs[j].focus();
      this.open(tabs[j].dataset.file, { focus: false });
    }
  }

  /** The tab of a file: its own for a stub, the preview tab for a test file. */
  tabOf(entry) {
    if (entry.file.kind === "stub") return entry.tab;
    if (!this.testTab) this.testTab = this.makeTab(entry.file.name, true);
    if (this.testTab.dataset.file !== entry.file.name) {
      this.testTab.dataset.file = entry.file.name;
      this.testTab.querySelector(".file-name").textContent = entry.file.name;
      this.testTab.title = `${entry.file.name}, read only`;
    }
    this.testTab.hidden = false;
    return this.testTab;
  }

  /** Shows a file in the editor; a test file goes in the preview tab. */
  open(name, { focus = false } = {}) {
    const entry = this.entries.get(name);
    if (!entry) return null;
    const tab = this.tabOf(entry);
    if (this.active && this.active !== entry) this.active.viewState = this.editor.saveViewState();
    for (const t of this.tabs.querySelectorAll('[role="tab"]')) {
      const on = t === tab;
      t.setAttribute("aria-selected", String(on));
      t.tabIndex = on ? 0 : -1;
    }
    if (this.active !== entry) {
      this.editor.setModel(entry.model);
      const ro = entry.file.readonly;
      this.editor.updateOptions({ readOnly: ro, ariaLabel: `${name}${ro ? ", read only" : ""}, code editor` });
      if (entry.viewState) this.editor.restoreViewState(entry.viewState);
    }
    this.active = entry;
    // The strip does not scroll, so nothing moves the focus start point here.
    if (this.resetItem) {
      const stub = entry.file.kind === "stub";
      this.resetItem.hidden = !stub;
      this.formatItem.hidden = !stub;
      this.resetItem.textContent = `Reset ${entry.file.name} to the stub`;
    }
    if (focus) this.editor.focus();
    this.onOpen?.(name);
    return entry;
  }

  /** Opens a file at a line and column; focus moves to the editor if asked. */
  reveal(name, line, column = 1, { focus = true } = {}) {
    if (!this.open(name)) return;
    this.editor.revealLineInCenterIfOutsideViewport(line);
    this.editor.setPosition({ lineNumber: line, column });
    if (focus) this.editor.focus();
  }

  has(name) {
    return this.entries.has(name);
  }

  isStub(name) {
    return this.entries.get(name)?.file.kind === "stub";
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

  /**
   * The functions that still have the course's stub body, in file order:
   * [{file, name: "Policy.Validate", short: "Validate", line, panicLine}].
   */
  stubFunctions() {
    const out = [];
    const decl = /^func\s+(?:\(\s*(?:\w+\s+)?\*?\s*(\w+)(?:\[[^\]]*\])?\s*\)\s*)?(\w+)/;
    for (const [name, e] of this.entries) {
      if (e.file.kind !== "stub") continue;
      const lines = e.model.getLinesContent();
      lines.forEach((text, i) => {
        if (!/^\s*panic\("TODO: implement"\)/.test(text)) return;
        for (let j = i; j >= 0; j--) {
          const m = lines[j].match(decl);
          if (!m) continue;
          out.push({ file: name, name: m[1] ? `${m[1]}.${m[2]}` : m[2], short: m[2], line: j + 1, panicLine: i + 1 });
          return;
        }
      });
    }
    return out;
  }

  /** A declaration in the stub files whose name differs from name only in case: {file, line, name}. */
  findDeclaration(name) {
    const lower = name.toLowerCase();
    for (const [file, e] of this.entries) {
      if (e.file.kind !== "stub") continue;
      const lines = e.model.getLinesContent();
      let inBlock = false;
      for (let i = 0; i < lines.length; i++) {
        const text = lines[i];
        if (/^(var|const|type)\s*\($/.test(text)) inBlock = true;
        else if (inBlock && /^\)/.test(text)) inBlock = false;
        let names = [];
        const m = text.match(/^(?:var|const|type)\s+(\w+)/) || text.match(/^func\s+(?:\([^)]*\)\s*)?(\w+)/);
        if (m) names = [m[1]];
        else if (inBlock) names = [(text.match(/^\s+(\w+)/) || [])[1]].filter(Boolean);
        for (const n of names) {
          if (n !== name && n.toLowerCase() === lower) return { file, line: i + 1, name: n };
        }
      }
    }
    return null;
  }

  /** Shows the number of problems of each file on its tab: red with an error, blue with notes only. */
  setCounts(counts) {
    for (const [name, e] of this.entries) {
      const tab = e.file.kind === "stub" ? e.tab : this.testTab?.dataset.file === name ? this.testTab : null;
      if (!tab) continue;
      const c = counts.get(name) || { n: 0, errors: 0 };
      const badge = tab.querySelector(".count-badge");
      badge.hidden = c.n === 0;
      badge.textContent = String(c.n);
      badge.classList.toggle("is-advice", c.errors === 0);
      badge.setAttribute("aria-label", c.errors === 0 ? (c.n === 1 ? "1 note" : `${c.n} notes`) : c.n === 1 ? "1 problem" : `${c.n} problems`);
    }
  }

  markDirty(entry) {
    entry.dirty = true;
    entry.tab.querySelector(".dirty").hidden = false;
    if (!this.failing) this.onSaveStatus({ kind: "", text: "Not saved yet" });
    clearTimeout(entry.timer);
    entry.timer = setTimeout(() => this.save(entry).catch(() => {}), AUTOSAVE_MS);
    clearTimeout(entry.backupTimer);
    entry.backupTimer = setTimeout(() => this.backup(entry), 400);
  }

  backupKey(entry) {
    return `gpw-unsaved:${this.slug}:${entry.file.name}`;
  }

  /** Keeps the unsaved code in this browser, so a lost save loses nothing. */
  backup(entry) {
    if (entry.dirty || entry.saving) save(this.backupKey(entry), JSON.stringify({ at: Date.now(), content: entry.model.getValue() }));
  }

  async save(entry, { keepalive = false } = {}) {
    clearTimeout(entry.timer);
    if (!entry.dirty) return entry.saving;
    // Saves of 1 file run in order, so an older body never lands last.
    const previous = (entry.saving || Promise.resolve()).catch(() => {});
    entry.dirty = false;
    const content = entry.model.getValue();
    const run = previous.then(async () => {
      if (!this.failing) this.onSaveStatus({ kind: "", text: "Saving" });
      try {
        const res = await api("PUT", `/api/exercises/${this.slug}/files/${encodeURIComponent(entry.file.name)}`, { content }, { keepalive });
        entry.retry = 0;
        this.lastSaved = new Date(res.savedAt || Date.now());
        if (!entry.dirty) {
          entry.tab.querySelector(".dirty").hidden = true;
          save(this.backupKey(entry), null);
        }
        if (![...this.entries.values()].some((e) => e.retry)) this.setFailing(null);
        this.onSaveStatus(this.anyDirty() ? { kind: "", text: "Not saved yet" } : { kind: "ok", text: `Saved at ${clock(this.lastSaved)}` });
      } catch (err) {
        entry.dirty = true;
        this.backup(entry);
        this.retryLater(entry, err);
        throw err;
      }
    });
    entry.saving = run;
    run.finally(() => {
      if (entry.saving === run) entry.saving = null;
    }).catch(() => {});
    return run;
  }

  /** A save failed: say so, and try again with a growing delay. */
  retryLater(entry, err) {
    const signedOut = err instanceof ApiError && err.status === 401;
    const wait = RETRY_MS[Math.min(entry.retry, RETRY_MS.length - 1)];
    entry.retry++;
    this.setFailing({ err, wait, signedOut });
    clearTimeout(entry.timer);
    if (!signedOut) entry.timer = setTimeout(() => this.save(entry).catch(() => {}), wait);
  }

  setFailing(state) {
    const was = this.failing;
    this.failing = state;
    if (!this.banner) return;
    if (!state) {
      this.banner.hidden = true;
      this.banner.replaceChildren();
      return;
    }
    const { err, wait, signedOut } = state;
    const text = signedOut
      ? ["Your code is not saved: this browser is signed out. ", h("a", { href: "/login", target: "_blank", rel: "noopener" }, "Sign in again in a new tab", h("span", { class: "visually-hidden" }, " (opens in a new tab)")), ", then come back. Your code stays in this browser until then. ", h("button", { type: "button", class: "link-button", onclick: () => this.retryAll() }, "Save now")]
      : [`Your code is not saved: ${errorMessage(err)} The site tries again in ${Math.round(wait / 1000)} s. Your code stays in this browser until then. `, h("button", { type: "button", class: "link-button", onclick: () => this.retryAll() }, "Save now")];
    this.banner.replaceChildren(h("b", {}, "WARNING:"), " ", ...text);
    this.banner.hidden = false;
    this.onSaveStatus({ kind: "fail", text: "Not saved" });
    if (!was) this.alert?.(`Your code is not saved: ${signedOut ? "this browser is signed out." : errorMessage(err)}`);
  }

  retryAll() {
    for (const e of this.entries.values()) {
      if (!e.dirty) continue;
      e.retry = 0;
      this.save(e).catch(() => {});
    }
  }

  /** Code that a closed tab could not save: offer it back. */
  offerBackups() {
    if (!this.banner) return;
    const found = [];
    for (const e of this.entries.values()) {
      if (e.file.kind !== "stub") continue;
      let b = null;
      try {
        b = JSON.parse(load(this.backupKey(e)) || "null");
      } catch {
        b = null;
      }
      if (!b || typeof b.content !== "string") continue;
      if (b.content === e.model.getValue()) {
        save(this.backupKey(e), null);
        continue;
      }
      found.push({ e, b });
    }
    if (!found.length) return;
    const names = found.map(({ e }) => e.file.name).join(" and ");
    const at = clock(new Date(Math.max(...found.map(({ b }) => b.at || 0))));
    const restore = () => {
      for (const { e, b } of found) {
        e.model.pushEditOperations([], [{ range: e.model.getFullModelRange(), text: b.content }], () => null);
      }
      this.banner.hidden = true;
      this.banner.replaceChildren();
    };
    const discard = () => {
      for (const { e } of found) save(this.backupKey(e), null);
      this.banner.hidden = true;
      this.banner.replaceChildren();
    };
    this.banner.replaceChildren(h("b", {}, "NOTE:"), ` This browser kept changes to ${names} from ${at} that the site did not save. `,
      h("button", { type: "button", class: "link-button", onclick: restore }, "Restore them"), " or ",
      h("button", { type: "button", class: "link-button", onclick: discard }, "discard them"), ".");
    this.banner.dataset.kind = "note";
    this.banner.hidden = false;
  }

  anyDirty() {
    return [...this.entries.values()].some((e) => e.dirty);
  }

  /** Saves every unsaved file now; a test run uses the saved files. */
  async flush(opts = {}) {
    await Promise.all([...this.entries.values()].map((e) => (e.dirty ? this.save(e, opts) : e.saving)));
  }

  /** Replaces a file's content, as 1 undo step, without saving it. */
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
    const name = entry.file.name;
    const ok = await confirmDialog({
      title: `Reset ${name}?`,
      text: `Reset replaces your code in ${name} with the stub. You cannot undo this.`,
      warning: true,
      confirm: `Reset ${name}`,
      cancel: "Keep working",
      danger: true,
    });
    if (!ok) return;
    clearTimeout(entry.timer);
    entry.dirty = false;
    try {
      const res = await api("POST", `/api/exercises/${this.slug}/files/${encodeURIComponent(name)}/reset`, {});
      this.replace(name, res.content);
      entry.tab.querySelector(".dirty").hidden = true;
      save(this.backupKey(entry), null);
      this.onSaveStatus({ kind: "ok", text: `Reset ${name} to the stub` });
      this.onEdit(name);
    } catch (err) {
      this.onSaveStatus({ kind: "fail", text: `Reset failed: ${errorMessage(err)}` });
    }
  }
}
