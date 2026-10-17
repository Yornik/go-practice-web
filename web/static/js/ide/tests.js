// tests.js: test runs. The run streams NDJSON events: queued, start,
// build, test (go test -json), explain, done, and progress at the end.
// They drive a live test tree, the raw output and the explanations.
import { apiStream, errorMessage } from "../api.js";
import { explanationElement } from "./explain.js";

const ICONS = { run: "…", pass: "✓", fail: "✗", skip: "–" };
// file.go:12 or file_test.go:12:5, with an optional directory in front.
const REF = /(?:[\w.\-]+\/)*([\w.\-]+\.go):(\d+)(?::(\d+))?/g;

export class TestRunner {
  /**
   * @param {object} o
   * @param {string} o.slug
   * @param {import("./files.js").Files} o.files
   * @param {HTMLElement} o.tree
   * @param {HTMLElement} o.summary
   * @param {HTMLElement} o.output
   * @param {HTMLElement} o.explanations
   * @param {HTMLButtonElement} o.runButton
   * @param {HTMLButtonElement} o.stopButton
   * @param {(p: object) => void} o.onProgress
   * @param {(panel: string) => void} o.showPanel
   */
  constructor(o) {
    Object.assign(this, o);
    this.ctl = null;
    this.reset();
  }

  reset() {
    this.nodes = new Map(); // test name -> {li, row, children, output, state}
    this.packageOutput = "";
    this.allOutput = "";
    this.selected = null;
    this.tree.replaceChildren();
    this.explanations.replaceChildren();
    this.output.replaceChildren();
  }

  get running() {
    return this.ctl !== null;
  }

  stop() {
    this.ctl?.abort();
  }

  /** Runs a step ("all" for every test) with or without -race. */
  async run(step, race) {
    if (this.running) return;
    this.reset();
    this.ctl = new AbortController();
    this.runButton.disabled = true;
    this.stopButton.hidden = false;
    this.showPanel("tests");
    this.setSummary("Saving your files…");
    try {
      await this.files.flush();
      this.setSummary("Starting…");
      await apiStream(`/api/exercises/${this.slug}/test`, { step, race }, (ev) => this.onEvent(ev), { signal: this.ctl.signal });
    } catch (err) {
      if (err.name === "AbortError") this.setSummary("Stopped.", "fail");
      else this.setSummary(errorMessage(err), "fail");
    } finally {
      this.ctl = null;
      this.runButton.disabled = false;
      this.stopButton.hidden = true;
    }
  }

  setSummary(text, kind = "") {
    this.summary.textContent = text;
    if (kind) this.summary.dataset.kind = kind;
    else delete this.summary.dataset.kind;
  }

  onEvent(ev) {
    switch (ev.type) {
      case "queued":
        this.setSummary(`Waiting for the runner: ${ev.position} ${ev.position === 1 ? "run" : "runs"} ahead.`);
        break;
      case "start":
        this.setSummary("Running…");
        break;
      case "build":
        this.appendOutput(null, ev.output || "");
        break;
      case "test":
        this.onTest(ev);
        break;
      case "explain":
        this.onExplain(ev);
        break;
      case "done":
        this.onDone(ev);
        break;
      case "progress":
        this.onProgress(ev);
        break;
      case "error":
        this.setSummary(ev.error, "fail");
        break;
    }
  }

  onTest(ev) {
    if (!ev.test) {
      if (ev.action === "output") this.appendOutput(null, ev.output || "");
      return;
    }
    const node = this.node(ev.test);
    switch (ev.action) {
      case "run":
        this.setState(node, "run");
        break;
      case "pass":
      case "fail":
      case "skip":
        this.setState(node, ev.action);
        if (ev.elapsed !== undefined) node.elapsed.textContent = `${ev.elapsed.toFixed(2)} s`;
        break;
      case "output":
        node.output += ev.output || "";
        this.appendOutput(ev.test, ev.output || "");
        break;
    }
  }

  /** Returns the tree node of a test, creating it and its parents. */
  node(name) {
    let node = this.nodes.get(name);
    if (node) return node;
    const slash = name.lastIndexOf("/");
    const parent = slash > 0 ? this.node(name.slice(0, slash)) : null;
    const li = document.createElement("li");
    li.className = "test-node";
    li.dataset.test = name;
    li.dataset.state = "run";
    const row = document.createElement("div");
    row.className = "test-row";
    const icon = document.createElement("span");
    icon.className = "test-icon";
    icon.setAttribute("aria-hidden", "true");
    icon.textContent = ICONS.run;
    const label = document.createElement("button");
    label.type = "button";
    label.textContent = slash > 0 ? name.slice(slash + 1) : name;
    const state = document.createElement("span");
    state.className = "visually-hidden";
    const elapsed = document.createElement("span");
    elapsed.className = "test-elapsed";
    row.append(icon, label, state, elapsed);
    const children = document.createElement("ul");
    li.append(row, children);
    (parent ? parent.children : this.tree).appendChild(li);
    node = { name, li, icon, state, elapsed, children, parent, output: "" };
    label.addEventListener("click", () => this.select(node));
    this.nodes.set(name, node);
    return node;
  }

  setState(node, state) {
    node.li.dataset.state = state;
    node.icon.textContent = ICONS[state] || "";
    node.state.textContent = { run: "running", pass: "passed", fail: "failed", skip: "skipped" }[state] || "";
  }

  /** Shows the output of 1 test, with its subtests, in the Output panel. */
  select(node) {
    this.selected = node;
    let text = "";
    for (const [name, n] of this.nodes) if (name === node.name || name.startsWith(node.name + "/")) text += n.output;
    this.renderOutput(text || "(no output)");
    this.showPanel("output");
  }

  appendOutput(test, text) {
    this.allOutput += text;
    if (!test) this.packageOutput += text;
    if (!this.selected) this.renderOutput(this.allOutput);
  }

  /** Writes output with file:line references as links into the editor. */
  renderOutput(text) {
    this.output.replaceChildren();
    let last = 0;
    for (const m of text.matchAll(REF)) {
      const [whole, file, line, col] = m;
      if (!this.files.has(file)) continue;
      this.output.append(text.slice(last, m.index));
      const a = document.createElement("a");
      a.href = "#";
      a.textContent = whole;
      a.addEventListener("click", (event) => {
        event.preventDefault();
        this.files.reveal(file, Number(line), Number(col || 1));
      });
      this.output.append(a);
      last = m.index + whole.length;
    }
    this.output.append(text.slice(last));
  }

  onExplain(ev) {
    const box = document.createElement("section");
    box.className = "explain-group";
    const h = document.createElement("h3");
    h.textContent = ev.test ? `Why ${ev.test} failed` : "What went wrong";
    box.appendChild(h);
    ev.items.forEach((item, i) => box.appendChild(explanationElement(item, i === 0)));
    this.explanations.appendChild(box);
  }

  onDone(ev) {
    if (!this.selected) this.renderOutput(this.allOutput);
    const secs = ((ev.durationMs || 0) / 1000).toFixed(1);
    const counts = `${ev.passed || 0} passed, ${ev.failed || 0} failed, ${ev.skipped || 0} skipped in ${secs} s`;
    if (ev.buildFailed) this.setSummary("The code does not compile. See the explanation and the Output panel.", "fail");
    else if (ev.timedOut) this.setSummary(`The run took too long and was stopped. ${counts}.`, "fail");
    else if (ev.truncated) this.setSummary(`The run printed more than 1 MiB and was stopped. ${counts}.`, "fail");
    else if (ev.exitCode === 0 && !ev.failed) this.setSummary(`PASS: ${counts}.`, "pass");
    else this.setSummary(`FAIL: ${counts}.`, "fail");
    // Tests still marked running never finished (the run was killed).
    for (const node of this.nodes.values()) if (node.li.dataset.state === "run") this.setState(node, "skip");
  }
}
