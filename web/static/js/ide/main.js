// main.js: the exercise page. It loads the exercise, starts Monaco and
// connects the parts: files and autosave, the check as you type, format,
// test runs, the problems and their explanations, the hint ladder, notes
// and active time.
import { api, errorMessage } from "../api.js";
import { isDark } from "../theme.js";
import { h, icon, load, save } from "../ui/dom.js";
import { setupPopovers } from "../ui/menu.js";
import { duration, modKey, shortDate } from "../ui/format.js";
import { setupNotes } from "./notes.js";
import { Hints } from "./hints.js";
import { trackActivity } from "./activity.js";
import { setupLayout, setupTabs } from "./panels.js";
import { EDITOR_OPTIONS, defineThemes, editorSize, loadMonaco, sizeOptions, themeName } from "./monaco.js";
import { Files } from "./files.js";
import { Linter } from "./lint.js";
import { TestRunner } from "./tests.js";
import { Problems } from "./problems.js";
import { Leader } from "./leader.js";
import { registerDocs } from "./docs.js";
import { setupWideCode } from "./widecode.js";

const ide = document.querySelector(".ide");
const slug = ide.dataset.exercise;
const number = ide.dataset.number;
const $ = (sel) => ide.querySelector(sel);
const titles = JSON.parse(ide.dataset.titles || "{}");

// Live regions exist from the start, so a screen reader hears the first
// message too. polite for news, assertive for a save that failed.
const politeRegion = $("[data-announce]");
const alertRegion = $("[data-alert]");
function say(region, text) {
  region.textContent = "";
  // A new text node in the next frame, so the same text twice is read twice.
  requestAnimationFrame(() => {
    region.textContent = text;
  });
}
const announce = (text) => say(politeRegion, text);
const alertNow = (text) => say(alertRegion, text);

setupPopovers(ide);
for (const k of ide.querySelectorAll("[data-mod]")) k.textContent = modKey();
const tabs = setupTabs(ide);
let leader = null;
const layout = setupLayout(ide, tabs, () => leader?.redraw());
for (const list of ide.querySelectorAll('[role="tablist"][data-tabs]')) list.addEventListener("gpw:tab", () => leader?.redraw());
setupWideCode(ide);

// The title block and the status bar.
const cells = {
  active: $("[data-cell-active]"),
  runs: $("[data-cell-runs]"),
  revealed: $("[data-cell-revealed]"),
  passed: $("[data-cell-passed]"),
  passedValue: $("[data-progress-status]"),
};
const saveCell = $("[data-save-status]");
const cursorCell = $("[data-cursor]");
const raceCell = $("[data-race-status]");
const LEVEL_NAMES = ["Nothing", "Hint 1", "Hint 2", "Hint 3", "Explanation", "Walkthrough", "Solution"];

function showPassed(passedAt) {
  cells.passed.hidden = false;
  cells.passedValue.replaceChildren(icon("tick"), shortDate(passedAt || new Date()).replace(/ \d{4}$/, ""));
  cells.passedValue.setAttribute("aria-label", `Passed ${shortDate(passedAt || new Date())}`);
}

function setSave({ kind, text }) {
  saveCell.className = `sb-cell${kind === "ok" ? " is-ok" : kind === "fail" ? " is-fail" : ""}`;
  saveCell.replaceChildren(kind === "ok" ? icon("tick") : kind === "fail" ? icon("cross") : "", text);
}

function editorFailed(message) {
  const host = $("[data-editor]");
  host.replaceChildren(h("div", { class: "editor-error" },
    h("p", { class: "callout callout-warning" }, h("b", {}, "WARNING:"), ` ${message}`),
    h("p", { class: "mt-3" }, "Reload the page. If it fails again, the site owner can check the server log.")));
  setSave({ kind: "fail", text: "The editor did not load" });
}

async function main() {
  let boot;
  try {
    boot = await api("GET", `/api/exercises/${slug}`);
  } catch (err) {
    editorFailed(`The exercise could not load: ${errorMessage(err)}`);
    return;
  }
  const progress = boot.progress || {};
  let activeSeconds = progress.activeSeconds || 0;
  let paused = false;
  const showActive = () => {
    cells.active.textContent = `${duration(activeSeconds)}${paused ? ", paused" : ""}`;
  };
  showActive();
  cells.runs.textContent = String(progress.runs || 0);
  if (progress.status === "passed") showPassed(progress.passedAt);

  const setRevealed = (level) => {
    cells.revealed.textContent = LEVEL_NAMES[level] || "Nothing";
  };
  const hints = new Hints({
    slug, number, ladder: $("[data-ladder]"), revealed: boot.revealed || [], gates: boot.gates,
    passed: progress.status === "passed", activeSeconds,
    onReveal: (level) => setRevealed(level),
  });
  setRevealed(hints.highest());

  const notes = $("[data-notes]");
  notes.value = progress.notes || "";
  setupNotes({
    slug, textarea: notes, status: $("[data-notes-status]"),
    onStatus: (text) => setSave({ kind: text.startsWith("Notes saved") ? "ok" : text.startsWith("The notes are not") ? "fail" : "", text }),
  });
  trackActivity({
    slug,
    onActiveSeconds: (s) => {
      activeSeconds = s;
      showActive();
      hints.setActive(s, { paused });
    },
    onPaused: (p) => {
      paused = p;
      showActive();
      hints.setActive(activeSeconds, { paused });
    },
  });

  let monaco;
  try {
    monaco = await loadMonaco();
    // Monaco measures the characters once: the font must be there first.
    await document.fonts.load('13px "Spline Sans Mono"').catch(() => null);
  } catch (err) {
    editorFailed(`The editor could not load. ${errorMessage(err)}`);
    return;
  }
  defineThemes(monaco);
  registerDocs(monaco);
  const host = $("[data-editor]");
  host.replaceChildren();
  const editor = monaco.editor.create(host, { ...EDITOR_OPTIONS, ...sizeOptions(editorSize()), theme: themeName(isDark()), ariaLabel: "Code editor" });
  document.fonts.ready.then(() => monaco.editor.remeasureFonts());
  // "Skip to the editor" lands on the host; the text area inside takes the focus.
  host.addEventListener("focus", () => editor.focus());
  document.addEventListener("gpw:theme", () => monaco.editor.setTheme(themeName(isDark())));

  // The text size of the editor, from the Keys popover (the account page sets it too).
  const sizeSelect = $("[data-editor-size]");
  if (sizeSelect) {
    sizeSelect.value = String(editorSize());
    sizeSelect.addEventListener("change", () => {
      save("gpw-editor-size", sizeSelect.value);
      editor.updateOptions(sizeOptions(Number(sizeSelect.value)));
    });
  }

  let linter;
  let runner;
  let problems;
  let format = () => {};
  const files = new Files({
    monaco, editor, slug, files: boot.files,
    tabs: $("[data-file-tabs]"),
    testMenu: $("[data-test-files]"),
    banner: $("[data-save-banner]"),
    onSaveStatus: setSave,
    alert: alertNow,
    format: () => format(),
    onEdit: (name) => {
      linter.schedule();
      runner?.markStale(name);
      problems?.markStale(name);
    },
    onOpen: () => leader?.redraw(),
  });

  leader = new Leader({
    root: $("[data-layout]"), svg: $("[data-leader]"), editor, files,
    column: $('[data-col="problems"]'), view: $("[data-problem-view]"),
    results: $("#panel-tests"), treeWrap: $("[data-tree-wrap]"),
    jump: $("[data-leader-jump]"), treeJump: $("[data-tree-jump]"),
  });
  const stepSelect = $("[data-step-select]");
  const race = $("[data-race]");
  const run = (step = stepSelect.value) => runner.run(step, race.checked);
  problems = new Problems({
    monaco, editor, files,
    chips: $("[data-chips]"), view: $("[data-problem-view]"),
    counts: [...ide.querySelectorAll("[data-problem-count]")],
    status: $("[data-lint-status]"), titles, leader, announce,
    onCount: (n) => layout.setProblemCount(n),
    runStep: (step) => {
      stepSelect.value = step;
      run(step);
    },
  });
  leader.problems = problems;
  linter = new Linter({ files, slug, problems });
  linter.run();

  const showProblem = (n, test = null) => {
    layout.revealProblems();
    problems.select(n, { reveal: true, focus: true, test });
  };

  race.addEventListener("change", () => {
    raceCell.textContent = race.checked ? "-race on" : "-race off";
  });
  runner = new TestRunner({
    slug, number, steps: boot.steps, files, problems,
    tree: $("[data-test-tree]"), detail: $("[data-test-detail]"), summary: $("[data-run-summary]"), notes: $("[data-run-notes]"),
    output: $("[data-output]"), announce,
    runButton: $("[data-run]"), stopButton: $("[data-stop]"),
    next: ide.dataset.nextSlug ? { slug: ide.dataset.nextSlug, title: ide.dataset.nextTitle } : null,
    showPanel: (name) => tabs.show("bottom", `panel-${name}`),
    showProblem,
    onStart: () => layout.expandBottom(),
    onProgress: (p) => {
      cells.runs.textContent = String(p.runs);
      activeSeconds = p.activeSeconds;
      showActive();
      if (p.status === "passed") {
        showPassed(p.passedAt);
        hints.setPassed(true);
      }
    },
  });
  problems.tests = runner;
  leader.tests = runner;
  runner.runs = progress.runs || 0;
  if (progress.lastResult && progress.runs) {
    runner.restore(progress.lastResult, progress.runs, boot.files);
    layout.expandBottom();
    // The next run starts from the step of the last one.
    const last = progress.lastResult.step;
    if (last && [...stepSelect.options].some((o) => o.value === last)) stepSelect.value = last;
  }
  $("[data-run]").addEventListener("click", () => run());
  $("[data-stop]").addEventListener("click", () => runner.stop());
  for (const b of ide.querySelectorAll("[data-step]")) {
    b.addEventListener("click", () => {
      stepSelect.value = b.dataset.step;
      run(b.dataset.step);
    });
  }

  format = async () => {
    const name = files.activeName();
    if (!name || !files.isStub(name)) return;
    try {
      const res = await api("POST", `/api/exercises/${slug}/format`, { file: name, content: files.model(name).getValue() });
      if (res.diagnostics?.length) {
        problems.setCheck(res.diagnostics);
        setSave({ kind: "fail", text: `${name} does not parse, so gofmt cannot format it` });
        return;
      }
      if (res.content !== files.model(name).getValue()) {
        editor.pushUndoStop();
        files.model(name).pushEditOperations([], [{ range: files.model(name).getFullModelRange(), text: res.content }], () => null);
        editor.pushUndoStop();
      }
      setSave({ kind: "ok", text: `Formatted ${name}` });
    } catch (err) {
      setSave({ kind: "fail", text: `Format failed: ${errorMessage(err)}` });
    }
  };
  $("[data-format]").addEventListener("click", () => format());

  // The hover on a problem's line: the plain title in the manual's font,
  // above the raw message that the marker shows.
  monaco.languages.registerHoverProvider("go", {
    provideHover(model, position) {
      const name = model.uri.path.split("/").pop();
      const found = problems.atLine(name, position.lineNumber);
      if (!found.length) return null;
      return {
        range: new monaco.Range(position.lineNumber, 1, position.lineNumber, model.getLineMaxColumn(position.lineNumber)),
        contents: found.map((p) => ({
          value: `<span class="gpw-hover-title">${escapeHTML(`Problem ${p.n}: ${hoverTitle(p)}`)}</span>`,
          supportHtml: true,
        })),
      };
    },
  });

  const { KeyMod, KeyCode } = monaco;
  editor.addCommand(KeyMod.CtrlCmd | KeyCode.KeyS, () => files.flush().catch(() => {}));
  editor.addCommand(KeyMod.CtrlCmd | KeyCode.Enter, () => run());
  editor.addCommand(KeyMod.Shift | KeyMod.Alt | KeyCode.KeyF, () => format());
  editor.addCommand(KeyCode.F8, () => problems.next(1));
  editor.addCommand(KeyMod.Shift | KeyCode.F8, () => problems.next(-1));
  document.addEventListener("keydown", (event) => {
    if (editor.hasTextFocus()) return;
    const mod = event.ctrlKey || event.metaKey;
    if (mod && event.key === "Enter") {
      event.preventDefault();
      run();
    } else if (mod && event.key.toLowerCase() === "s") {
      event.preventDefault();
      files.flush().catch(() => {});
    } else if (event.key === "F8" && !document.querySelector("dialog[open]")) {
      event.preventDefault();
      problems.next(event.shiftKey ? -1 : 1);
    }
  });

  // The cursor: shown in the status bar, kept per file, so the editor
  // opens where the learner left it.
  const cursorKey = (name) => `gpw-cursor:${slug}:${name}`;
  let cursorTimer = 0;
  editor.onDidChangeCursorPosition((e) => {
    cursorCell.textContent = `Line ${e.position.lineNumber}, column ${e.position.column}`;
    clearTimeout(cursorTimer);
    const name = files.activeName();
    cursorTimer = setTimeout(() => save(cursorKey(name), `${e.position.lineNumber}:${e.position.column}`), 500);
  });
  const first = files.activeName();
  const stored = (load(cursorKey(first)) || "").split(":").map(Number);
  if (stored[0] > 1) {
    editor.setPosition({ lineNumber: stored[0], column: stored[1] || 1 });
    editor.revealLineInCenter(stored[0]);
  } else {
    cursorCell.textContent = "Line 1, column 1";
  }
  editor.onDidFocusEditorText(() => hints.editorFocused());

  ide.dataset.ready = "true"; // the E2E tests wait for this
}

function hoverTitle(p) {
  if (p.stub && p.stubs?.all.length) return p.stubs.all.length === 1 ? "1 function is still the course stub" : `${p.stubs.all.length} functions are still the course stub`;
  return p.explain?.title || p.diag?.message || "Problem";
}

function escapeHTML(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
}

main();
