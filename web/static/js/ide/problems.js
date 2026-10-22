// problems.js: every problem the tools found, with 1 number each. The
// same number is on the balloon at the end of the line in the editor, on
// the chip in the problems column, on the failing tests in the tree, on
// the file tab count and in the status bar. Problems come from the check
// while you type and from the last test run. F8 and Shift+F8 move between
// them.
//
// A run's failures are grouped by cause: 1 problem per explanation and
// place (catalog id, file and line), with the tests that hit it. Every
// panic of a course stub is 1 problem: on the untouched stub, 28 failing
// tests have 1 cause, "these functions are still the stub".
//
// The numbers follow the order the learner works in: the problems of the
// check by file and line, then the build, then failing tests in the order
// of the test tree (step 1 first).
import { balloon, h } from "../ui/dom.js";
import { zero } from "../ui/zero.js";
import { byteToUtf16Column } from "./monaco.js";
import { chipLabel, chipText, problemView } from "./explain.js";

const STUB_IDS = new Set(["test-panicked-stub", "runtime-todo-implement"]);

/** The plain title of a problem as text, for the hover. */
function plainTitle(p) {
  if (p.stub && p.stubs?.all.length) return p.stubs.all.length === 1 ? "1 function is still the course stub." : `${p.stubs.all.length} functions are still the course stub.`;
  return p.explain?.title || p.diag?.message || "Problem";
}
// The chips stay on at most 2 rows and show at most 7 (VESPER.md 7.6);
// the rest fold into "N more", so the selected problem's title and its
// anatomy stay in view in a short pane.
const CHIP_CAP = 7;
const CHIP_ROWS = 2;

export class Problems {
  /**
   * @param {object} o
   * @param {any} o.monaco
   * @param {any} o.editor
   * @param {import("./files.js").Files} o.files
   * @param {HTMLElement} o.chips
   * @param {HTMLElement} o.view
   * @param {HTMLElement[]} o.counts the count squares
   * @param {HTMLElement} [o.countText] the hidden count after the Problems heading
   * @param {HTMLElement} o.status the status bar cell
   * @param {Record<string, string>} o.titles
   * @param {import("./leader.js").Leader} [o.leader]
   * @param {(text: string) => void} [o.announce]
   * @param {(n: number) => void} [o.onCount] called with the number of problems after each change
   * @param {{orderOf: (test: string) => number[], stepFor: (test: string) => any, revealTest: (test: string) => void}} [o.tests]
   * @param {(step: string) => void} [o.runStep]
   */
  constructor(o) {
    Object.assign(this, o);
    this.check = []; // problems of the last check
    this.groups = new Map(); // problems of the last test run, by cause
    this.list = [];
    this.selectedKey = null;
    this.shownSig = "";
    this.decorations = new Map(); // file -> decoration collection
    this.checking = false;
    this.checkError = "";
    this.showAllChips = false;
    this.announced = "";
    this.announceTimer = 0;
  }

  /** The problems of a check or format response. */
  setCheck(diagnostics, at = new Date()) {
    this.checkError = "";
    const order = [...this.files.entries.keys()];
    const sorted = [...diagnostics].sort((a, b) => order.indexOf(a.file) - order.indexOf(b.file) || a.line - b.line || a.column - b.column);
    this.check = sorted.map((d) => ({
      kind: "check",
      key: `check|${d.explain?.id || d.message}|${d.file}|${d.line}|${d.column}|${d.message}`,
      diag: d,
      explain: d.explain,
      file: this.files.has(d.file) ? d.file : null,
      line: d.line || 0,
      column: d.column || 0,
      endLine: d.endLine,
      endColumn: d.endColumn,
      severity: severity(d),
      at,
    }));
    this.update();
  }

  /** Starts a new run: the problems of the previous run go. */
  clearRun() {
    this.groups = new Map();
    this.update();
  }

  /** Adds the explanations of 1 failed test (or the build) of a run. */
  addRun(items, test, runNo, at = new Date()) {
    for (const e of items) {
      const raw = e.raw || {};
      const name = test || raw.test || "";
      const stub = STUB_IDS.has(e.id);
      const key = stub ? "run|stub" : `run|${e.id}|${raw.file || ""}|${raw.line || 0}`;
      let g = this.groups.get(key);
      if (!g) {
        const file = raw.openable && this.files.has(raw.file) ? raw.file : null;
        g = { kind: "run", key, explain: e, stub, occ: [], runNo, at, file, line: file ? raw.line : 0, column: raw.column || 0, severity: "error" };
        // The file changed while this run was going: its lines are older.
        if (!stub && file && this.tests?.stale?.has(file)) g.stale = true;
        this.groups.set(key, g);
      }
      g.occ.push({ test: name, explain: e });
    }
    this.update();
  }

  /** The run ended: its first problem in the learner's order takes the selection. */
  finishRun() {
    this.update();
    const first = this.list.find((p) => p.kind === "run");
    if (first) this.select(first.n, { reveal: false });
  }

  /**
   * A file changed after the run: its run problems keep their place in the
   * list, but their line numbers can be wrong now, so they leave the editor.
   * The stub problem finds its lines again in the current code.
   */
  markStale(file) {
    let changed = false;
    for (const g of this.groups.values()) {
      if (g.file === file && !g.stale && !g.stub) {
        g.stale = true;
        changed = true;
      }
    }
    if (changed || [...this.groups.values()].some((g) => g.stub)) this.update();
  }

  /** Shows that a check is running, or that checks failed. */
  setChecking(on, error = "") {
    this.checking = on;
    if (error) this.checkError = error;
    this.renderStatus();
  }

  /** Fills in what a run group knows only now: its order, its first test and, for stubs, the functions. */
  prepare(g) {
    const order = (t) => this.tests?.orderOf(t) || [0];
    g.occ.sort((a, b) => cmp(order(a.test), order(b.test)));
    g.test = g.occ[0]?.test || "";
    // The first test in the tree's order speaks for the group.
    if (g.occ[0]) g.explain = g.occ[0].explain;
    g.order = g.test ? order(g.test) : [-1];
    if (!g.stub) return g;
    const fns = this.files.stubFunctions();
    const hit = new Map(); // function name -> {fn, tests}
    for (const o of g.occ) {
      const fn = stubOf(o.explain, fns, this.files);
      if (!fn) continue;
      if (!hit.has(fn.name)) hit.set(fn.name, { fn, tests: [] });
      hit.get(fn.name).tests.push(o.test);
    }
    // Where the learner starts: the first step that does not pass, with the
    // stubs its tests reached. A panic that ends the test binary can leave
    // that step without results: then the step is known, its stubs are not.
    const step = this.tests?.startStep?.() || this.tests?.stepFor(g.test) || null;
    const start = { step, functions: [], ran: step ? this.tests?.stepRan?.(step) !== false : true };
    for (const o of g.occ) {
      const fn = stubOf(o.explain, fns, this.files);
      if (!fn || (this.tests?.stepFor(o.test)?.id || "") !== (step?.id || "")) continue;
      if (!start.functions.includes(fn)) start.functions.push(fn);
    }
    g.stubs = { all: fns, hit, start };
    // The marks in the editor: the stubs of that step, else the stubs the run reached.
    g.marks = start.functions.length ? start.functions : [...hit.values()].map((x) => x.fn);
    const target = g.marks[0];
    if (target) {
      g.file = target.file;
      g.line = target.panicLine;
      g.column = 0;
    }
    return g;
  }

  update() {
    const before = this.list.length;
    const runs = [...this.groups.values()].map((g) => this.prepare(g)).sort((a, b) => cmp(a.order, b.order));
    this.list = [...this.check, ...runs].map((p, i) => Object.assign(p, { n: i + 1 }));
    let selected = this.list.find((p) => p.key === this.selectedKey);
    if (!selected && this.list.length) selected = this.list[0];
    this.selectedKey = selected ? selected.key : null;
    this.renderMarkers();
    this.renderChips();
    this.renderView(this.list.length > before);
    this.renderStatus();
    const counts = new Map();
    for (const p of this.list) {
      for (const f of p.stub ? new Set((p.marks || []).map((fn) => fn.file)) : [p.file]) {
        if (!f) continue;
        const c = counts.get(f) || { n: 0, errors: 0 };
        c.n++;
        if (p.severity !== "advice") c.errors++;
        counts.set(f, c);
      }
    }
    this.files.setCounts(counts);
    const errors = this.list.filter((p) => p.severity !== "advice").length;
    for (const c of this.counts) {
      c.hidden = this.list.length === 0;
      c.replaceChildren(saira(String(this.list.length)));
      c.classList.toggle("is-advice", errors === 0);
    }
    if (this.countText) this.countText.textContent = this.list.length ? `: ${this.list.length}` : "";
    this.onCount?.(this.list.length);
    this.scheduleAnnounce();
  }

  selected() {
    return this.list.find((p) => p.key === this.selectedKey) || null;
  }

  /** The problem of a failed test, for "Show problem 2" and the balloon in the tree. */
  forTest(test) {
    const runs = this.list.filter((p) => p.kind === "run");
    return runs.find((p) => p.occ.some((o) => o.test === test))
      || runs.find((p) => p.occ.some((o) => o.test && test.startsWith(`${o.test}/`)))
      || null;
  }

  /**
   * Selects a problem by number. reveal opens its file at the line;
   * focus moves the focus to the explanation.
   */
  select(n, { reveal = true, focus = false, test = null } = {}) {
    const p = this.list.find((q) => q.n === n);
    if (!p) return;
    const changed = this.selectedKey !== p.key;
    this.selectedKey = p.key;
    this.preferredTest = test;
    this.renderChips();
    this.renderMarkers();
    this.renderView(changed);
    if (reveal && p.file && p.line) this.files.reveal(p.file, p.line, this.utf16Column(p), { focus: false });
    if (focus) this.view.focus({ preventScroll: false });
    this.onSelect?.(p);
  }

  /** F8 and Shift+F8. */
  next(dir = 1) {
    if (!this.list.length) return;
    const cur = this.selected();
    const i = cur ? this.list.indexOf(cur) : -1;
    const j = (i + dir + this.list.length) % this.list.length;
    const p = this.list[j];
    this.select(p.n, { reveal: true });
    if (p.file) this.editor.focus();
    this.announce?.(`Problem ${p.n} of ${this.list.length}${p.line && p.file ? `, line ${p.line}` : ""}: ${titleText(p, this.files)}`);
  }

  utf16Column(p) {
    const model = this.files.model(p.file);
    if (!model || !p.line) return 1;
    const line = Math.min(p.line, model.getLineCount());
    return byteToUtf16Column(model.getLineContent(line), p.column || 1);
  }

  /** The places a problem marks in the editor: [{file, line, column, endLine, endColumn}]. */
  places(p) {
    if (p.stub) {
      return (p.marks || []).map((fn) => ({ file: fn.file, line: fn.panicLine, column: 0 }));
    }
    if (!p.file || !p.line || p.stale) return [];
    return [{ file: p.file, line: p.line, column: p.column, endLine: p.endLine, endColumn: p.endColumn }];
  }

  /** The second place of a staticcheck finding, in Monaco's form. */
  related(p) {
    const r = p.diag?.related;
    const model = r?.file && r.line ? this.files.model(r.file) : null;
    if (!model) return undefined;
    const line = Math.min(r.line, model.getLineCount());
    const text = model.getLineContent(line);
    const endLine = Math.min(r.endLine || line, model.getLineCount());
    const endColumn = r.endLine && r.endColumn ? byteToUtf16Column(model.getLineContent(endLine), r.endColumn) : model.getLineMaxColumn(endLine);
    return [{ resource: model.uri, startLineNumber: line, startColumn: byteToUtf16Column(text, r.column || 1), endLineNumber: endLine, endColumn, message: r.message || "Related place" }];
  }

  renderMarkers() {
    const { monaco } = this;
    const sev = monaco.MarkerSeverity;
    const selected = this.selected();
    const byFile = new Map();
    for (const p of this.list) {
      for (const at of this.places(p)) {
        if (!byFile.has(at.file)) byFile.set(at.file, []);
        byFile.get(at.file).push({ p, at });
      }
    }
    for (const [name] of this.files.entries) {
      const model = this.files.model(name);
      const markers = [];
      const decos = [];
      for (const { p, at } of byFile.get(name) || []) {
        const line = Math.min(Math.max(at.line, 1), model.getLineCount());
        const startColumn = this.utf16Column({ ...at, line });
        let endLine = line;
        let endColumn;
        if (at.endLine && at.endColumn) {
          endLine = Math.min(at.endLine, model.getLineCount());
          endColumn = byteToUtf16Column(model.getLineContent(endLine), at.endColumn);
        } else {
          const word = model.getWordAtPosition({ lineNumber: line, column: startColumn });
          endColumn = word ? word.endColumn : Math.max(startColumn + 1, model.getLineMaxColumn(line));
          if (!word && startColumn >= model.getLineMaxColumn(line)) endColumn = model.getLineMaxColumn(line);
        }
        const advice = p.severity === "advice";
        const e = p.explain || {};
        markers.push({
          startLineNumber: line,
          startColumn: at.column ? startColumn : model.getLineFirstNonWhitespaceColumn(line) || 1,
          endLineNumber: endLine,
          endColumn: at.column ? endColumn : model.getLineMaxColumn(line),
          // The hover shows the marker first, so the plain title leads
          // it (VESPER.md 7.4), then the raw message as the tool printed it.
          // No source: Monaco would print it after "See problem 1.", and
          // the problem's chip already names the tool. A marker is plain
          // text, so the title cannot take its own weight here without a
          // site rule on Monaco's hover (VESPER.md 14.2).
          message: `${plainTitle(p)}\n\n${e.raw?.message || p.diag?.message || ""}\n\nSee problem ${p.n}.`,
          severity: advice ? sev.Warning : sev.Error,
          relatedInformation: this.related(p),
        });
        decos.push({
          range: new monaco.Range(line, 1, line, 1),
          options: {
            glyphMarginClassName: advice ? "gm-advice" : "gm-problem",
            lineNumberClassName: advice ? undefined : "ln-problem",
            stickiness: monaco.editor.TrackedRangeStickiness.NeverGrowsWhenTypingAtEdges,
          },
        });
        decos.push({
          range: new monaco.Range(line, model.getLineMaxColumn(line), line, model.getLineMaxColumn(line)),
          options: {
            // An empty range shows its injected text only with showIfCollapsed.
            showIfCollapsed: true,
            after: { content: String(p.n), inlineClassName: `eol-balloon${p === selected ? " is-selected" : ""}` },
            stickiness: monaco.editor.TrackedRangeStickiness.NeverGrowsWhenTypingAtEdges,
          },
        });
      }
      monaco.editor.setModelMarkers(model, "gpw", markers);
      const coll = this.decorations.get(name) || [];
      this.decorations.set(name, model.deltaDecorations(coll, decos));
    }
  }

  renderChips() {
    const selected = this.selected();
    const want = this.list.map((p) => `${p.key}#${p.n}`).join("\n");
    // Rebuild only when the set of chips changes; a new selection updates
    // the chips in place, so the chip that has the focus keeps it.
    let hadFocus = false;
    if (want !== this.chipsSig) {
      hadFocus = this.chips.contains(document.activeElement);
      this.chipsSig = want;
      // The chip is set in Saira: its numbers go through zero(). It shows
      // the short label; its name and title keep the long one.
      const nodes = this.list.map((p) => h("button", {
        type: "button", role: "tab", class: `chip${p.severity === "advice" ? " is-advice" : ""}`,
        "aria-controls": "problem-view", "data-n": p.n, id: `chip-${p.n}`, title: chipText(p), "aria-label": `Problem ${p.n}: ${chipText(p)}`,
        onclick: () => this.select(p.n),
        onkeydown: (event) => this.chipKey(event),
      }, balloon(saira(String(p.n))), h("span", { class: "chip-label" }, chipLabel(p, { short: true }).map((n) => (typeof n === "string" ? saira(n) : n)))));
      nodes.push(h("button", { type: "button", class: "chip chip-more", hidden: true, onclick: () => this.showMoreChips() }));
      this.chips.replaceChildren(...nodes);
      if (!this.chipsWatch) {
        // Refit when the pane's width changes; in the next frame, so the
        // refit never resizes what the observer is reporting on.
        let width = -1;
        this.chipsWatch = new ResizeObserver(([entry]) => {
          if (Math.round(entry.contentRect.width) === width) return;
          width = Math.round(entry.contentRect.width);
          cancelAnimationFrame(this.chipsFrame);
          this.chipsFrame = requestAnimationFrame(() => this.fitChips());
        });
        this.chipsWatch.observe(this.chips);
      }
    }
    for (const chip of this.chips.querySelectorAll('[role="tab"]')) {
      const on = Number(chip.dataset.n) === selected?.n;
      chip.setAttribute("aria-selected", String(on));
      chip.tabIndex = on ? 0 : -1;
    }
    this.fitChips();
    // After the loop: only then does a new chip carry aria-selected.
    if (hadFocus) (this.chips.querySelector('[aria-selected="true"]') || this.chips.querySelector('[role="tab"]:not([hidden])') || this.view).focus({ preventScroll: true });
    if (selected) this.view.setAttribute("aria-labelledby", `chip-${selected.n}`);
    else this.view.removeAttribute("aria-labelledby");
  }

  /**
   * Shows the chips that fit on CHIP_ROWS rows, at most CHIP_CAP of them,
   * and folds the rest into "N more". The selected chip always shows.
   */
  fitChips() {
    const chips = [...this.chips.querySelectorAll('[role="tab"]')];
    const more = this.chips.querySelector(".chip-more");
    if (!more) return;
    for (const c of chips) c.hidden = false;
    more.hidden = true;
    // A pane that is not laid out (a hidden tab) has nothing to measure:
    // the observer fits it when it gets its width.
    if (this.showAllChips || !this.chips.offsetWidth) return;
    const rows = () => {
      const tops = [];
      for (const c of this.chips.children) {
        if (c.hidden) continue;
        if (!tops.some((t) => Math.abs(t - c.offsetTop) < 4)) tops.push(c.offsetTop);
      }
      return tops.length;
    };
    // "1 more" would take the place of the 1 chip it folds.
    if (chips.length <= CHIP_CAP + 1 && rows() <= CHIP_ROWS) return;
    const selected = this.selected();
    more.hidden = false;
    let shown = chips.length;
    let folded = 0;
    for (let i = chips.length - 1; i >= 0; i--) {
      if (folded > 0 && shown <= CHIP_CAP && rows() <= CHIP_ROWS) break;
      if (Number(chips[i].dataset.n) === selected?.n) continue;
      chips[i].hidden = true;
      shown--;
      folded++;
      // Its width changes with the number, so the next check measures it.
      more.replaceChildren(saira(`${folded} more`));
    }
  }

  /** "N more": every chip, wrapped; the focus goes to the first that was folded. */
  showMoreChips() {
    const first = this.chips.querySelector('[role="tab"][hidden]');
    this.showAllChips = true;
    this.fitChips();
    first?.focus();
  }

  chipKey(event) {
    const chips = [...this.chips.querySelectorAll('[role="tab"]:not([hidden])')];
    const i = chips.indexOf(event.currentTarget);
    let j = -1;
    if (event.key === "ArrowRight") j = (i + 1) % chips.length;
    if (event.key === "ArrowLeft") j = (i - 1 + chips.length) % chips.length;
    if (event.key === "Home") j = 0;
    if (event.key === "End") j = chips.length - 1;
    if (j < 0) return;
    event.preventDefault();
    const n = Number(chips[j].dataset.n);
    this.select(n);
    this.chips.querySelector(`[data-n="${n}"]`)?.focus();
  }

  renderView(animate) {
    const p = this.selected();
    if (!p) {
      this.shownSig = "";
      this.view.replaceChildren(h("p", { class: "problems-empty" }, "Nothing to explain. When the compiler, go vet or a test finds a problem, it appears here, tied to its line."));
      this.leader?.set(null);
      return;
    }
    const sig = `${p.key}|${p.n}|${p.stale ? 1 : 0}|${p.occ?.length || 0}|${p.stubs ? p.stubs.all.map((f) => `${f.name}@${f.panicLine}`).join(",") : ""}`;
    if (sig !== this.shownSig) {
      const hadFocus = this.view.contains(document.activeElement) && document.activeElement !== this.view;
      this.view.replaceChildren(problemView(p, { files: this.files, titles: this.titles, tests: this.tests, runStep: this.runStep }));
      this.shownSig = sig;
      if (hadFocus) this.view.focus({ preventScroll: true });
      this.leader?.set(p, { animate });
    } else {
      this.leader?.redraw();
    }
  }

  renderStatus() {
    const s = this.status;
    s.classList.remove("is-fail", "is-ok", "is-advice");
    s.replaceChildren();
    const errors = this.list.filter((p) => p.severity !== "advice");
    const notes = this.list.length - errors.length;
    if (this.list.length) {
      // The count as a square, then the word (VESPER.md 7.9).
      s.classList.add(errors.length ? "is-fail" : "is-advice");
      // The square holds the number; the space keeps "2 problems" whole for a screen reader and a copy.
      const shown = errors.length || notes;
      s.append(h("span", { class: `count-badge${errors.length ? "" : " is-advice"}` }, saira(String(shown))));
      const words = [errors.length ? (errors.length === 1 ? "problem" : "problems") : notes === 1 ? "note" : "notes"];
      if (errors.length && notes) words.push(notes === 1 ? "1 note" : `${notes} notes`);
      s.append(saira(` ${words.join(", ")}`));
      return;
    }
    if (this.checkError) {
      s.append(`Checks are not available: ${this.checkError}`);
      return;
    }
    s.append(this.checking ? "Checking your code" : "No problems");
  }

  /** Tells a screen reader how many problems there are, 2 s after the count settles. */
  scheduleAnnounce() {
    clearTimeout(this.announceTimer);
    this.announceTimer = setTimeout(() => {
      const errors = this.list.filter((p) => p.severity !== "advice").length;
      const notes = this.list.length - errors;
      const head = errors ? (errors === 1 ? "1 problem" : `${errors} problems`) : notes ? (notes === 1 ? "1 note" : `${notes} notes`) : "No problems";
      const text = this.list.length ? `${head}: ${this.list.slice(0, 3).map((p) => chipText(p)).join("; ")}${this.list.length > 3 ? "; and more" : ""}.` : "No problems.";
      if (text === this.announced) return;
      const first = this.announced === "";
      this.announced = text;
      // The page load is not news.
      if (!first || this.list.length) this.announce?.(text);
    }, 2000);
  }
}

/** Text in a Saira cell: every 0 slashed (VESPER.md 3.3). */
function saira(text) {
  return /0/.test(text) ? zero(text) : text;
}

function severity(d) {
  if (d.source === "staticcheck" && d.code && !/^SA/.test(d.code)) return "advice";
  if (d.severity && d.severity !== "error" && d.source !== "vet" && d.source !== "compile") return "advice";
  return "error";
}

function cmp(a, b) {
  for (let i = 0; i < Math.max(a.length, b.length); i++) {
    const d = (a[i] ?? -1) - (b[i] ?? -1);
    if (d) return d;
  }
  return 0;
}

/** The stub function a stub panic comes from: by its line in a stub file, or by the call the test names. */
function stubOf(e, fns, files) {
  const raw = e.raw || {};
  if (raw.file && files.isStub(raw.file)) {
    const fn = fns.find((f) => f.file === raw.file && raw.line >= f.line && raw.line <= f.panicLine);
    if (fn) return fn;
  }
  const call = (raw.message || "").match(/^(?:goroutine \d+: )?(?:\S+ = )?(\w+) panicked/)?.[1]
    || (raw.captures?.func || "").split(".").pop()
    || (raw.message || "").match(/(\w+) panicked/)?.[1];
  if (!call) return null;
  return fns.find((f) => f.short === call) || null;
}

function titleText(p, files) {
  if (p.stub && p.stubs?.all.length) return stubTitle(p.stubs.all.length);
  return p.explain?.title || p.diag?.message || "Problem";
}

/** "11 functions are still the course stub". */
export function stubTitle(n) {
  return n === 1 ? "1 function is still the course stub" : `${n} functions are still the course stub`;
}
