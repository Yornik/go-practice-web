// tests.js: test runs. The run streams NDJSON events: queued, start,
// build, test (go test -json), explain, done, and progress at the end.
// They drive the test tree (the README's steps, their tests and
// subtests), the raw output and the run's problems.
//
// The tree holds only tree items. The output of the selected failing test
// and its actions ("Show problem 2", "Open policy_test.go:50") are in a
// detail pane under the tree, so the keys of the tree never swallow them.
import { apiStream, errorMessage } from "../api.js";
import { balloon, h, icon } from "../ui/dom.js";
import { clock, seconds } from "../ui/format.js";
import { gotWant, shortTest } from "./explain.js";

// file.go:12 or file_test.go:12:5, with an optional directory in front.
const REF = /(?:[\w.\-]+\/)*([\w.\-]+\.go):(\d+)(?::(\d+))?/g;
const ICON = { queued: "ring", run: "arc", pass: "tick", fail: "cross", skip: "dash" };
const WORD = { queued: "queued", run: "running", pass: "pass", fail: "fail", skip: "skipped" };
const STATE_NAME = { queued: "queued", run: "running", pass: "passed", fail: "failed", skip: "skipped" };

export class TestRunner {
  /**
   * @param {object} o
   * @param {string} o.slug
   * @param {string} o.number "07"
   * @param {{id: string, n: number, kind: string, label: string, labelHTML?: string, run: string}[]} o.steps
   * @param {import("./files.js").Files} o.files
   * @param {import("./problems.js").Problems} o.problems
   * @param {HTMLElement} o.tree
   * @param {HTMLElement} o.detail the pane under the tree
   * @param {HTMLElement} o.summary
   * @param {HTMLElement} o.notes
   * @param {HTMLElement} o.output
   * @param {(text: string) => void} o.announce
   * @param {HTMLButtonElement} o.runButton
   * @param {HTMLButtonElement} o.stopButton
   * @param {(p: object) => void} o.onProgress
   * @param {(panel: string) => void} o.showPanel
   * @param {(n: number) => void} o.showProblem
   * @param {() => void} [o.onStart]
   * @param {() => void} [o.onRaceUnavailable]
   * @param {{slug: string, title: string}|null} o.next
   */
  constructor(o) {
    Object.assign(this, o);
    this.ctl = null;
    this.runs = 0;
    this.steps = o.steps.filter((s) => s.kind !== "all").map((s) => ({ ...s, re: compile(s.run) }));
    this.expanded = new Set();
    this.showPassed = new Set();
    this.frame = 0;
    this.stale = new Set();
    this.lastRun = null;
    this.current = null;
    this.tree.addEventListener("keydown", (event) => this.treeKey(event));
    this.reset();
  }

  reset() {
    this.nodes = new Map(); // test name -> node
    this.order = []; // top-level test names in arrival order
    this.allOutput = "";
    this.selectedKey = null;
    this.focusKey = null;
    this.expanded.clear();
    this.showPassed.clear();
  }

  get running() {
    return this.ctl !== null;
  }

  stop() {
    this.ctl?.abort();
  }

  /** Remembers that a file changed after the last run. */
  markStale(name) {
    // An edit during a run counts too: run() takes the files before it
    // starts, so the run's line numbers are those of the older code.
    if ((!this.lastRun && !this.running) || this.stale.has(name)) return;
    this.stale.add(name);
    this.renderSummary();
  }

  /**
   * Shows the last run after a reload: its summary, the step statuses and
   * the failing tests. The output and the explanations are not kept.
   * @param {object} last progress.lastResult
   * @param {number} runNo
   * @param {{name: string, updatedAt?: string}[]} files
   */
  restore(last, runNo, files) {
    if (!last || !last.at) return;
    const at = new Date(last.at);
    this.current = {
      no: runNo, step: last.step || "all", race: last.race, at, state: "done", restored: true,
      done: { durationMs: last.durationMs, buildFailed: last.buildFailed, timedOut: last.timedOut, truncated: last.truncated, exitCode: last.exitCode },
      counts: { pass: last.passed || 0, fail: last.failed || 0, skip: last.skipped || 0 },
    };
    const tops = Object.entries(last.tests || {}).sort(([a], [b]) => a.localeCompare(b));
    for (const [name, state] of tops) this.node(name).state = state;
    for (const name of last.failedTests || []) this.node(name).state = "fail";
    // Parents of a failing subtest failed too.
    for (const n of this.nodes.values()) if (n.state === "queued") n.state = n.children.some((c) => c.state === "fail") ? "fail" : "pass";
    for (const f of files) if (f.kind === "stub" && f.updatedAt && new Date(f.updatedAt) > at) this.stale.add(f.name);
    this.lastRun = this.current;
    this.renderSummary();
    this.render();
  }

  /** Runs a step ("all" for every test) with or without -race. */
  async run(step, race) {
    if (this.running) return;
    this.reset();
    // clearRun can empty the chips, and a run hides the panel that holds
    // them below 1280 px: the Run button keeps the focus instead.
    if (this.problems.chips?.contains(document.activeElement) || this.problems.view?.contains(document.activeElement)) this.runButton.focus();
    this.problems.clearRun();
    this.notes.replaceChildren();
    this.stale.clear();
    this.ctl = new AbortController();
    this.current = { no: this.runs + 1, step, race, at: new Date(), state: "saving", queue: 0, done: null };
    // aria-disabled, not disabled: the button keeps the keyboard focus.
    this.runButton.setAttribute("aria-busy", "true");
    this.runButton.setAttribute("aria-disabled", "true");
    this.runButton.textContent = "Running tests";
    this.stopButton.hidden = false;
    this.onStart?.();
    this.showPanel("tests");
    this.renderSummary();
    this.render();
    this.announce(`Run ${this.current.no} started.`);
    try {
      await this.files.flush();
      this.current.state = "starting";
      this.renderSummary();
      await apiStream(`/api/exercises/${this.slug}/test`, { step, race }, (ev) => this.onEvent(ev), { signal: this.ctl.signal });
      if (!this.current.done && !this.current.error) this.fail("The run ended before the runner reported the result. Try again.");
    } catch (err) {
      if (err.name === "AbortError") {
        this.current.state = "stopped";
        for (const node of this.nodes.values()) if (node.state === "run" || node.state === "queued") node.state = "skip";
        this.announce(`Run ${this.current.no} stopped.`);
      } else {
        this.fail(errorMessage(err), err.code);
      }
    } finally {
      const hadFocus = this.stopButton.contains(document.activeElement);
      this.ctl = null;
      this.runButton.removeAttribute("aria-busy");
      this.runButton.removeAttribute("aria-disabled");
      this.runButton.textContent = "Run tests";
      this.stopButton.hidden = true;
      if (hadFocus) this.runButton.focus();
      this.lastRun = this.current;
      this.renderSummary();
      this.render();
      this.problems.finishRun();
    }
  }

  /** A run that could not start or broke: a NOTE line with "Try again". */
  fail(message, code) {
    this.current.error = message;
    this.current.state = "error";
    this.notes.append(h("p", { class: "callout" }, h("b", {}, "NOTE:"), ` ${message}`,
      h("button", { type: "button", class: "btn btn-sm", onclick: () => this.runButton.click() }, "Try again")));
    this.announce(code === "runner_busy" || code === "rate_limited" ? message : `Run ${this.current.no} failed: ${message}`);
  }

  onEvent(ev) {
    switch (ev.type) {
      case "queued":
        this.current.state = "queued";
        this.current.queue = ev.position || 0;
        this.renderSummary();
        this.announce(`Run ${this.current.no} waits for the runner, ${this.current.queue} ${this.current.queue === 1 ? "run" : "runs"} ahead.`);
        break;
      case "start":
        this.current.state = "running";
        this.renderSummary();
        break;
      case "build":
        this.allOutput += ev.output || "";
        this.renderOutput();
        break;
      case "test":
        this.onTest(ev);
        break;
      case "explain":
        this.problems.addRun(ev.items || [], ev.test, this.current.no, this.current.at);
        this.schedule();
        break;
      case "done":
        this.onDone(ev);
        break;
      case "progress":
        this.onProgressEvent(ev);
        break;
      case "error":
        this.fail(ev.error, ev.code);
        break;
    }
  }

  onTest(ev) {
    if (ev.action === "output") {
      this.allOutput += ev.output || "";
      if (ev.test) this.node(ev.test).output += ev.output || "";
      this.schedule(true);
      return;
    }
    if (!ev.test) return;
    const node = this.node(ev.test);
    if (ev.action === "run") node.state = "run";
    if (ev.action === "pass" || ev.action === "fail" || ev.action === "skip") {
      node.state = ev.action;
      if (ev.elapsed !== undefined) node.elapsed = ev.elapsed;
    }
    this.schedule();
  }

  /** The node of a test, with its parents. */
  node(name) {
    let node = this.nodes.get(name);
    if (node) return node;
    const slash = name.lastIndexOf("/");
    const parent = slash > 0 ? this.node(name.slice(0, slash)) : null;
    node = { name, parent, children: [], state: "queued", output: "", elapsed: undefined, depth: parent ? parent.depth + 1 : 0 };
    if (parent) parent.children.push(node);
    else this.order.push(name);
    this.nodes.set(name, node);
    return node;
  }

  schedule(outputToo = false) {
    if (outputToo) this.outputDirty = true;
    cancelAnimationFrame(this.frame);
    this.frame = requestAnimationFrame(() => {
      this.render();
      if (this.outputDirty) {
        this.outputDirty = false;
        this.renderOutput();
      }
    });
  }

  onDone(ev) {
    this.current.done = ev;
    this.current.state = "done";
    if (ev.raceUnavailable) this.raceNote();
    // Tests still marked running never finished: the run was stopped.
    // A test that never reported a result failed when go test's -timeout fired
    // or the test binary died (an exit or a crash); a run stopped or cut off
    // from outside skips it. The server keeps the same states in last_result.
    const unfinished = ev.timedOut || ev.exitCode > 0 ? "fail" : "skip";
    // The deepest tests still running are where the run was when it ended.
    const running = (n) => n.state === "run" || n.state === "queued";
    this.current.hung = [...this.nodes.values()].filter((n) => n.state === "run" && !n.children.some(running)).map((n) => n.name);
    for (const node of this.nodes.values()) if (running(node)) node.state = unfinished;
    // Select the first failure in tree order, and show its output.
    const firstFail = this.visibleOrder().find((n) => n.state === "fail" && !n.children.some((c) => c.state === "fail"));
    if (firstFail) {
      this.selectedKey = firstFail.name;
      for (let p = firstFail.parent; p; p = p.parent) this.expanded.add(p.name);
    }
    this.render();
    this.renderOutput();
    this.renderSummary();
    const c = counts([...this.topNodes()]);
    this.announce(ev.buildFailed
      ? `Run ${this.current.no} finished: the build failed.`
      : `Run ${this.current.no} finished: ${c.pass} pass, ${c.fail} ${c.fail === 1 ? "fails" : "fail"}.`);
  }

  onProgressEvent(p) {
    this.runs = p.runs || this.runs + 1;
    this.current.no = this.runs;
    if (p.raceUnavailable) this.raceNote();
    if (p.firstPass) {
      const how = p.raceUnavailable ? "passes. This server has no race detector, so it ran without -race" : "passes with -race";
      const line = h("div", { class: "pass-line" },
        h("p", { class: "status status-ok" }, icon("tick"), h("span", {}, `Exercise ${this.number} ${how}. Rule 5: read the explanation and the walkthrough now. They show the trade-offs.`)));
      if (this.next) line.append(h("p", {}, "Next: ", h("a", { href: `/learn/${this.next.slug}` }, this.next.title)));
      this.notes.append(line);
    }
    this.renderSummary();
    this.onProgress(p);
  }

  /** ran is false before any run: the page only knows the runner has no race detector. */
  raceNote(ran = true) {
    this.onRaceUnavailable?.();
    if (this.notes.querySelector("[data-race-note]")) return;
    const text = ran
      ? " The race detector is not available on this server. The tests ran without -race."
      : " The race detector is not available on this server. Runs go without -race.";
    this.notes.append(h("p", { class: "callout", "data-race-note": "" }, h("b", {}, "NOTE:"), text));
  }

  *topNodes() {
    for (const name of this.order) yield this.nodes.get(name);
  }

  renderSummary() {
    const c = this.current;
    const s = this.summary;
    s.replaceChildren();
    delete s.dataset.kind;
    if (!c) {
      s.textContent = "Run the tests to see each step here.";
      return;
    }
    const head = `Run ${c.no}`;
    switch (c.state) {
      case "saving":
        s.textContent = `${head}: saving your files.`;
        return;
      case "starting":
        s.textContent = `${head}: starting.`;
        return;
      case "queued":
        s.textContent = `${head}: waiting for the runner, ${c.queue} ${c.queue === 1 ? "run" : "runs"} ahead.`;
        return;
      case "running":
        s.textContent = `${head}: running.`;
        return;
      case "stopped":
        s.textContent = `${head} stopped. The results below are not complete.`;
        return;
      case "error":
        s.textContent = `${head} did not run.`;
        return;
    }
    const d = c.done || {};
    const at = `${head} at ${clock(c.at)}`;
    const k = this.nodes.size ? counts([...this.topNodes()]) : { pass: c.counts?.pass || 0, fail: c.counts?.fail || 0, skip: c.counts?.skip || 0 };
    const parts = [`${k.pass} pass`, `${k.fail} ${k.fail === 1 ? "fails" : "fail"}`];
    if (k.skip) parts.push(`${k.skip} skipped`);
    if (d.buildFailed) {
      s.textContent = `${at}: the build failed. The tests did not run. ${c.restored ? "Run the tests again to see why." : "The problems explain why."}`;
      s.dataset.kind = "fail";
    } else if (d.timedOut) {
      // A dump from go test's own timeout names where the test waits; a run
      // the runner killed at its deadline has none, so name the test.
      const where = c.restored ? ""
        : this.problems.list.some((p) => p.kind === "run") ? "The problems show where a test waits."
        : c.hung?.length ? `${c.hung.join(" and ")} ${c.hung.length === 1 ? "was" : "were"} still running: look for a channel, a lock or a loop that never ends.`
        : "";
      s.textContent = `${at}: the run took too long and stopped. ${parts.join(", ")}. ${where}`.trim();
      s.dataset.kind = "fail";
    } else if (d.truncated) {
      s.textContent = `${at}: the run printed more than 1 MiB and stopped. ${parts.join(", ")}.`;
      s.dataset.kind = "fail";
    } else {
      s.textContent = `${at}: ${parts.join(", ")}, ${seconds(d.durationMs)}.`;
      s.dataset.kind = k.fail || d.exitCode ? "fail" : "pass";
    }
    if (c.restored && (k.fail || d.exitCode) && !d.buildFailed) s.append(" Run the tests again to see the output and the explanations.");
    if (this.stale.size) {
      s.append(" ", h("span", { class: "stale" }, `You changed ${[...this.stale].join(" and ")} after this run.`));
    }
  }

  /** The step of a top-level test: the first README step whose -run matches it. */
  stepOf(name) {
    return this.steps.findIndex((s) => s.re && s.re.test(name));
  }

  /** The step a test belongs to, for the problems: {id, n, kind, label} or null. */
  stepFor(test) {
    const i = this.stepOf(String(test || "").split("/")[0]);
    return i >= 0 ? this.steps[i] : null;
  }

  /**
   * The step the learner should work on: in a run of 1 step, that step;
   * in a full run, the first step of the README whose tests did not all
   * pass. A step whose tests never ran counts: a panic can end the test
   * binary before them.
   */
  startStep() {
    const c = this.current;
    if (!c) return null;
    if (c.step !== "all") return this.steps.find((s) => s.id === c.step) || null;
    for (const [i, step] of this.steps.entries()) {
      const tops = [...this.topNodes()].filter((n) => this.stepOf(n.name) === i);
      if (tops.length === 0 || tops.some((n) => n.state !== "pass" && n.state !== "skip")) return step;
    }
    return null;
  }

  /** Reports whether any test of a step reported a result in this run. */
  stepRan(step) {
    const i = this.steps.indexOf(step);
    return [...this.topNodes()].some((n) => this.stepOf(n.name) === i && n.state !== "queued" && n.state !== "run");
  }

  /** A sort key for a test in the order of the tree: [step, top-level test, subtest...]. */
  orderOf(test) {
    const parts = String(test || "").split("/");
    const top = parts[0];
    const step = this.stepOf(top);
    const key = [step >= 0 ? step : this.steps.length, this.order.indexOf(top)];
    let node = this.nodes.get(top);
    let name = top;
    for (const p of parts.slice(1)) {
      name = `${name}/${p}`;
      const i = node ? node.children.findIndex((c) => c.name === name) : -1;
      key.push(i);
      node = node?.children[i];
    }
    return key;
  }

  /** The nodes in the order the tree shows them, folded or not. */
  visibleOrder() {
    const out = [];
    const walk = (nodes) => {
      for (const n of [...nodes].sort((a, b) => rank(a) - rank(b))) {
        out.push(n);
        walk(n.children);
      }
    };
    for (const g of this.groups()) walk(g.tests);
    return out;
  }

  /** The groups the tree shows: the steps of this run, with their tests. */
  groups() {
    const c = this.current;
    const only = c && c.step !== "all" ? this.steps.findIndex((s) => s.id === c.step) : -1;
    const groups = this.steps.map((s) => ({ key: `step:${s.id}`, step: s, tests: [] }));
    const other = { key: "step:other", step: { n: "", label: "Other tests", run: "" }, tests: [] };
    for (const node of this.topNodes()) {
      const i = only >= 0 ? only : this.stepOf(node.name);
      if (i >= 0) groups[i].tests.push(node);
      else other.tests.push(node);
    }
    let out = groups;
    if (only >= 0) out = [groups[only]];
    else if (c) out = groups.filter((g) => g.tests.length || !c.done);
    if (other.tests.length) out.push(other);
    return out;
  }

  render() {
    const hadFocus = this.tree.contains(document.activeElement);
    const items = [];
    if (!this.current) {
      this.tree.replaceChildren();
      this.renderDetail();
      return;
    }
    const done = this.current.state === "done" || this.current.state === "stopped";
    // Only the first failing step opens by itself: on the untouched stub
    // every step fails, and 4 open steps bury the first one to work on.
    let firstFail = null;
    for (const g of this.groups()) {
      const k = counts(g.tests);
      let state = "queued";
      if (k.fail) state = "fail";
      else if (k.run || (!done && k.total === 0 && this.current.state === "running")) state = k.run ? "run" : "queued";
      else if (k.total && k.pass + k.skip === k.total) state = k.pass ? "pass" : "skip";
      else if (done && k.total === 0) state = "skip";
      // The first failing step opens, the rest fold, unless the learner chose.
      if (state === "fail" && !firstFail) firstFail = g.key;
      const open = this.expanded.has(g.key) || (state === "fail" && firstFail === g.key && !this.expanded.has(`-${g.key}`));
      const count = state === "fail" ? `${k.fail} of ${k.total} ${k.fail === 1 ? "fails" : "fail"}`
        : state === "pass" ? `${k.pass} of ${k.total} pass`
        : state === "skip" ? (k.total ? "skipped" : "no tests ran")
        : state === "run" ? "running" : "queued";
      this.updateStepStatus(g.step, state, count);
      const label = h("span", { class: "tree-label" });
      if (g.step.labelHTML) label.innerHTML = g.step.labelHTML; // escaped by the server
      else label.textContent = g.step.label || g.step.title || `Step ${g.step.n}`;
      const li = this.item({ key: g.key, level: 1, state, open, cls: "is-step", expandable: g.tests.length > 0,
        name: `${g.step.n === "" ? "" : `${capital(g.step.kind || "step")} ${g.step.n}: `}${label.textContent}, ${STATE_NAME[state]}: ${count}` },
      icon(ICON[state]), h("span", { class: "tree-no" }, g.step.n === "" ? "" : String(g.step.n)),
      label,
      h("span", { class: "tree-filter" }, g.step.run ? `-run ${g.step.run}` : ""),
      h("span", { class: "tree-count" }, count));
      if (open && g.tests.length) li.append(this.children(g.tests, 2, g.key));
      items.push(li);
    }
    this.tree.replaceChildren(...items);
    this.rovingTabindex();
    if (hadFocus) this.tree.querySelector('[role="treeitem"][tabindex="0"]')?.focus();
    this.renderDetail();
  }

  /** The tests of a step or the subtests of a test: failures first, passes folded. */
  children(nodes, level, key) {
    const group = h("ul", { role: "group" });
    const sorted = [...nodes].sort((a, b) => rank(a) - rank(b));
    const passed = sorted.filter((n) => n.state === "pass" || n.state === "skip");
    const rest = sorted.filter((n) => !(n.state === "pass" || n.state === "skip"));
    const showAll = this.showPassed.has(key) || rest.length === 0;
    for (const n of showAll ? sorted : rest) group.append(this.testItem(n, level));
    if (!showAll && passed.length) {
      const word = passed.every((n) => n.state === "skip") ? "skipped" : "pass";
      const text = `${passed.length} more ${passed.length === 1 ? "test" : "tests"} ${word}`;
      const li = this.item({ key: `more:${key}`, level, state: "pass", cls: "tree-more", name: `${text}. Show them.`, action: () => { this.showPassed.add(key); this.render(); } },
        icon("tick"), h("span", { class: "tree-label" }, text), h("span", { class: "tree-count" }, "show"));
      group.append(li);
    }
    return group;
  }

  testItem(n, level) {
    const failingChild = n.children.some((c) => c.state === "fail");
    const open = n.children.length > 0 && (this.expanded.has(n.name) || (failingChild && !this.expanded.has(`-${n.name}`)));
    const problem = n.state === "fail" ? this.problems.forTest(n.name) : null;
    const leaf = n.state === "fail" && !failingChild;
    const end = problem && leaf
      ? h("span", { class: "tree-end" }, h("span", { class: "tree-count" }, WORD.fail), balloon(String(problem.n)))
      : h("span", { class: "tree-count" }, n.state === "run" ? "running" : WORD[n.state]);
    const shown = level > 2 ? shortTest(n.name) : n.name;
    const name = `${shown}, ${STATE_NAME[n.state]}${problem && leaf ? `, problem ${problem.n}` : ""}`;
    const li = this.item({ key: n.name, level, state: n.state, open, cls: "tree-test", expandable: n.children.length > 0, test: n.name, name },
      icon(ICON[n.state]), h("span", { class: "tree-label mono", title: n.name }, shown), end);
    li.style.setProperty("--indent", `${(level - 2) * 22 + 22}px`);
    if (this.selectedKey === n.name && n.state === "fail") li.setAttribute("aria-controls", "test-detail");
    if (open) li.append(this.children(n.children, level + 1, n.name));
    return li;
  }

  /** The output of the selected failing test, with its actions, under the tree. */
  renderDetail() {
    const n = this.selectedKey ? this.nodes.get(this.selectedKey) : null;
    if (!n || n.state !== "fail" || (!n.output && !this.current?.restored)) {
      this.detail.hidden = true;
      this.detail.replaceChildren();
      this.detailKey = null;
      return;
    }
    const problem = this.problems.forTest(n.name);
    const sig = `${n.name}|${n.output.length}|${problem?.n || 0}`;
    if (sig === this.detailKey) return;
    const hadFocus = this.detail.contains(document.activeElement);
    this.detailKey = sig;
    const head = h("p", { class: "detail-head" }, h("span", { class: "visually-hidden" }, "Output of "), h("code", {}, n.name));
    const body = [];
    if (n.output) {
      const pre = h("pre", {});
      linkRefs(pre, trimOutput(n.output), this.files);
      body.push(pre);
    } else {
      body.push(h("p", { class: "help" }, "The output of this run is not kept after a reload. Run the tests again to see it."));
    }
    const e = problem?.occ?.find((o) => o.test === n.name)?.explain || problem?.explain;
    const gw = e && e.id === "test-got-want" ? gotWant(e) : null;
    if (gw) body.push(gw);
    const actions = h("div", { class: "btn-row" });
    if (problem) actions.append(h("button", { type: "button", class: "btn btn-sm", onclick: () => this.showProblem(problem.n, n.name) }, `Show problem ${problem.n}`));
    const raw = e?.raw;
    if (raw?.openable && raw.file && raw.line && this.files.has(raw.file)) {
      actions.append(h("button", { type: "button", class: "btn btn-sm", onclick: () => this.files.reveal(raw.file, raw.line, raw.column || 1) }, `Open ${raw.file}:${raw.line}`));
    }
    if (actions.childElementCount) body.push(actions);
    this.detail.replaceChildren(head, ...body);
    this.detail.setAttribute("aria-label", `Output of ${n.name}`);
    this.detail.hidden = false;
    if (hadFocus) this.detail.querySelector("button, a")?.focus();
  }

  item({ key, level, state, open = false, cls = "", expandable = false, test, action, name }, ...row) {
    const id = `ti-${hash(key)}`;
    const li = h("li", {
      role: "treeitem", class: `test-node ${cls}`.trim(), "aria-level": level, "data-state": state, "data-key": key,
      "data-test": test || null, "aria-expanded": expandable ? String(open) : null,
      "aria-selected": String(this.selectedKey === key), tabindex: "-1", "aria-labelledby": `${id}-name`,
    }, h("div", { class: "tree-row", onclick: (ev) => { ev.stopPropagation(); this.activate(key, expandable, open, action); } },
      h("span", { class: "visually-hidden", id: `${id}-name` }, name || ""), ...row.map((r) => hide(r))));
    li.style.setProperty("--indent", `${(level - 1) * 22}px`);
    return li;
  }

  activate(key, expandable, open, action) {
    this.focusKey = key;
    if (action) {
      action();
      return;
    }
    this.selectedKey = key;
    if (expandable) this.toggle(key, !open);
    else this.render();
    const node = this.nodes.get(key);
    if (node?.state === "fail") {
      const p = this.problems.forTest(key);
      if (p) this.problems.select(p.n, { reveal: false, test: key });
    }
    this.tree.querySelector(`[data-key="${CSS.escape(key)}"]`)?.focus();
  }

  /** Shows a test in the tree: opens its step and parents, selects and scrolls to it. */
  revealTest(name) {
    if (!name || !this.nodes.has(name)) return;
    const node = this.nodes.get(name);
    const step = this.stepOf(name.split("/")[0]);
    const groupKey = step >= 0 ? `step:${this.steps[step].id}` : "step:other";
    this.expanded.delete(`-${groupKey}`);
    this.expanded.add(groupKey);
    for (let p = node.parent; p; p = p.parent) {
      this.expanded.delete(`-${p.name}`);
      this.expanded.add(p.name);
      this.showPassed.add(p.name);
    }
    this.showPassed.add(groupKey);
    this.selectedKey = name;
    this.focusKey = name;
    this.showPanel("tests");
    this.render();
    const li = this.tree.querySelector(`[data-key="${CSS.escape(name)}"]`);
    li?.querySelector(".tree-row")?.scrollIntoView({ block: "nearest" });
    li?.focus({ preventScroll: true });
    const p = this.problems.forTest(name);
    if (p) this.problems.select(p.n, { reveal: false, test: name });
  }

  toggle(key, open) {
    this.expanded.delete(key);
    this.expanded.delete(`-${key}`);
    this.expanded.add(open ? key : `-${key}`);
    this.render();
  }

  rovingTabindex() {
    const items = [...this.tree.querySelectorAll('[role="treeitem"]')];
    const target = items.find((li) => li.dataset.key === this.focusKey) || items.find((li) => li.getAttribute("aria-selected") === "true") || items[0];
    if (target) target.tabIndex = 0;
  }

  treeKey(event) {
    const cur = event.target.closest('[role="treeitem"]');
    // Only the tree items take the tree's keys.
    if (!cur || event.target !== cur) return;
    const items = [...this.tree.querySelectorAll('[role="treeitem"]')];
    const i = items.indexOf(cur);
    const key = cur.dataset.key;
    const expandable = cur.hasAttribute("aria-expanded");
    const open = cur.getAttribute("aria-expanded") === "true";
    const go = (el) => {
      if (!el) return;
      event.preventDefault();
      cur.tabIndex = -1;
      el.tabIndex = 0;
      el.focus();
      this.focusKey = el.dataset.key;
    };
    switch (event.key) {
      case "ArrowDown":
        go(items[i + 1]);
        break;
      case "ArrowUp":
        go(items[i - 1]);
        break;
      case "Home":
        go(items[0]);
        break;
      case "End":
        go(items[items.length - 1]);
        break;
      case "ArrowRight":
        event.preventDefault();
        if (expandable && !open) this.toggle(key, true);
        else if (open) go(cur.querySelector('[role="treeitem"]'));
        break;
      case "ArrowLeft":
        event.preventDefault();
        if (open) this.toggle(key, false);
        else go(cur.parentElement.closest('[role="treeitem"]'));
        break;
      case "Enter":
      case " ":
        event.preventDefault();
        cur.querySelector(".tree-row").click();
        break;
    }
  }

  updateStepStatus(step, state, count) {
    const row = document.querySelector(`[data-step-row="${CSS.escape(step.id || "")}"] [data-step-status]`);
    if (!row) return;
    row.textContent = state === "queued" || state === "run" ? "" : `Run ${this.current.no}: ${count}`;
    row.className = `step-status${state === "pass" ? " is-pass" : state === "fail" ? " is-fail" : ""}`;
  }

  renderOutput() {
    this.output.replaceChildren();
    if (!this.allOutput) {
      this.output.textContent = this.running ? "No output yet." : this.current?.restored ? "The output of the last run is not kept after a reload. Run the tests again to see it." : "The run printed nothing.";
      return;
    }
    linkRefs(this.output, this.allOutput, this.files);
  }
}

/** Hides a row's visible parts from the accessibility tree: the hidden name says it all. */
function hide(node) {
  if (node instanceof Element) node.setAttribute("aria-hidden", "true");
  return node;
}

function hash(s) {
  let x = 0;
  for (let i = 0; i < s.length; i++) x = (x * 31 + s.charCodeAt(i)) | 0;
  return (x >>> 0).toString(36);
}

function capital(s) {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

function counts(nodes) {
  const c = { pass: 0, fail: 0, skip: 0, run: 0, total: nodes.length };
  for (const n of nodes) {
    if (n.state === "pass") c.pass++;
    else if (n.state === "fail") c.fail++;
    else if (n.state === "skip") c.skip++;
    else c.run++;
  }
  return c;
}

function rank(n) {
  return { fail: 0, run: 1, queued: 2, skip: 3, pass: 4 }[n.state] ?? 5;
}

/** The output without go test's own bookkeeping lines. */
function trimOutput(text) {
  return text
    .split("\n")
    .filter((l) => !/^\s*(=== (RUN|PAUSE|CONT|NAME)|--- (PASS|FAIL|SKIP))/.test(l))
    .join("\n")
    .trim();
}

/** Writes output with file:line references as links into the editor; a repeated place is 1 Tab stop. */
function linkRefs(el, text, files) {
  let last = 0;
  const seen = new Set();
  for (const m of text.matchAll(REF)) {
    const [whole, file, line, col] = m;
    if (!files.has(file)) continue;
    el.append(text.slice(last, m.index));
    const again = seen.has(whole);
    seen.add(whole);
    el.append(h("a", { href: "#", tabindex: again ? "-1" : null, onclick: (event) => { event.preventDefault(); event.stopPropagation(); files.reveal(file, Number(line), Number(col || 1)); } }, whole));
    last = m.index + whole.length;
  }
  el.append(text.slice(last));
}

/** Go's -run pattern as a JavaScript RegExp, for the top-level test names. */
function compile(run) {
  if (!run) return null;
  // -run splits at "/" for subtests; the first part matches the top level.
  const top = run.split("/")[0];
  try {
    return new RegExp(top);
  } catch {
    return null;
  }
}
