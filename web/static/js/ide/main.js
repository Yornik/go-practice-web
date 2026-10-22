// main.js: the exercise page. It loads the exercise, starts Monaco and
// connects the parts: files and autosave, the check as you type, format,
// test runs, the problems and their explanations, the hint ladder, notes,
// active time, the Keys menu and HUSH's refused offer.
import { api, errorMessage } from "../api.js";
import { isDark } from "../theme.js";
import { h, icon, load, save } from "../ui/dom.js";
import { setupPopovers } from "../ui/menu.js";
import { duration, isMac, modKey, shortDate } from "../ui/format.js";
import { zero } from "../ui/zero.js";
import { setupNotes } from "./notes.js";
import { Hints } from "./hints.js";
import { trackActivity } from "./activity.js";
import { setupLayout, setupTabs } from "./panels.js";
import { EDITOR_OPTIONS, EDITOR_SIZES, calmModels, defineThemes, editorSize, gutterOptions, loadMonaco, sizeOptions, themeName } from "./monaco.js";
import { Files } from "./files.js";
import { Linter } from "./lint.js";
import { TestRunner } from "./tests.js";
import { Problems } from "./problems.js";
import { Leader } from "./leader.js";
import { registerDocs } from "./docs.js";
import { setupWideCode } from "./widecode.js";
import { Hush } from "./hush.js";

const ide = document.querySelector(".ide");
const slug = ide.dataset.exercise;
// The slug of the first course (07-retry for 32-retry): this browser may
// still keep code and cursor positions under it.
const renamedFrom = ide.dataset.renamedFrom || "";
// Code kept in this browser belongs to 1 account: a shared browser never
// offers 1 learner's unsaved code to another.
const user = ide.dataset.user || "";
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
// Monaco's key for Tab focus mode is Ctrl+Shift+M on a Mac: Cmd+M minimises the window.
if (isMac()) for (const k of ide.querySelectorAll("[data-tab-key]")) k.textContent = "Ctrl+Shift+M, then Tab";
const tabs = setupTabs(ide);
let leader = null;
const layout = setupLayout(ide, tabs, () => leader?.redraw());
for (const list of ide.querySelectorAll('[role="tablist"][data-tabs]')) list.addEventListener("gpw:tab", () => leader?.redraw());
setupWideCode(ide);

// The top bar's readouts and the status bar are set in Saira, so every
// text with a digit goes through zero(): Saira's 0 is the same oval as its O.
const setText = (el, text) => el?.replaceChildren(/\d/.test(text) ? zero(text) : text);
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
  cells.passedValue.replaceChildren(zero(shortDate(passedAt || new Date()).replace(/ \d{4}$/, "")));
}

// A save is not a test result: it is ink with a tick, never green.
function setSave({ kind, text }) {
  saveCell.className = `sb-cell gpw-sb-saved${kind === "ok" ? " is-ok" : kind === "fail" ? " is-fail" : ""}`;
  saveCell.replaceChildren(kind === "ok" ? icon("tick") : kind === "fail" ? icon("cross") : "", /\d/.test(text) ? zero(text) : text);
}

function setRace(text) {
  raceCell.replaceChildren(h("code", {}, "-race"), ` ${text}`);
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
  const showActive = () => setText(cells.active, `${duration(activeSeconds)}${paused ? ", paused" : ""}`);
  showActive();
  setText(cells.runs, String(progress.runs || 0));
  if (progress.status === "passed") showPassed(progress.passedAt);

  const setRevealed = (level) => {
    cells.revealed.textContent = LEVEL_NAMES[level] || "Nothing";
  };
  // HUSH's refused offer (VESPER.md 16.7): it waits for its 7 conditions.
  const hush = new Hush({
    slug, card: $("[data-hush]"), link: $("[data-hush-link]"), heading: $("#problems-title"),
    passed: progress.status === "passed", activeSeconds, gateMinutes: boot.gates?.firstHintMinutes || 25,
  });
  const hints = new Hints({
    slug, number, ladder: $("[data-ladder]"), revealed: boot.revealed || [], gates: boot.gates,
    passed: progress.status === "passed", activeSeconds,
    onReveal: (level) => setRevealed(level),
  });
  setRevealed(hints.highest());

  const notes = $("[data-notes]");
  notes.value = progress.notes || "";
  const notesSave = setupNotes({
    slug, textarea: notes, status: $("[data-notes-status]"),
    onStatus: (text) => setSave({ kind: text.startsWith("Notes saved") ? "ok" : text.startsWith("The notes are not") ? "fail" : "", text }),
  });
  trackActivity({
    slug,
    onActiveSeconds: (s) => {
      activeSeconds = s;
      showActive();
      hints.setActive(s, { paused });
      hush.setActive(s);
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
    // Monaco measures the characters once: the font must be there first,
    // in both weights, because keywords are bold.
    await Promise.all(['14px "Atkinson Hyperlegible Mono"', '700 14px "Atkinson Hyperlegible Mono"'].map((f) => document.fonts.load(f).catch(() => null)));
  } catch (err) {
    editorFailed(`The editor could not load. ${errorMessage(err)}`);
    notesSave.attachLeave();
    return;
  }
  defineThemes(monaco, () => monaco.editor.setTheme(themeName(isDark())));
  calmModels(monaco);
  registerDocs(monaco);
  const host = $("[data-editor]");
  host.replaceChildren();
  // The size lives here once read: with storage blocked, editorSize()
  // would answer the default after every step.
  let size = editorSize();
  const narrow = matchMedia("(max-width: 899px)");
  const editor = monaco.editor.create(host, { ...EDITOR_OPTIONS, ...sizeOptions(size), ...gutterOptions(narrow.matches), theme: themeName(isDark()), ariaLabel: "Code editor" });
  narrow.addEventListener("change", () => editor.updateOptions(gutterOptions(narrow.matches)));
  document.fonts.ready.then(() => monaco.editor.remeasureFonts());
  // "Skip to the editor" lands on the host; the text area inside takes the focus.
  host.addEventListener("focus", () => editor.focus());
  document.addEventListener("gpw:theme", () => monaco.editor.setTheme(themeName(isDark())));

  // The text size of the editor: the Keys menu steps through the sizes
  // (the account page sets it too).
  const sizeItem = $("[data-editor-size]");
  const sizeValue = $("[data-editor-size-value]");
  const showSize = () => setText(sizeValue, `${size}\u00a0px`);
  showSize();
  sizeItem?.addEventListener("click", () => {
    size = EDITOR_SIZES[(EDITOR_SIZES.indexOf(size) + 1) % EDITOR_SIZES.length];
    save("gpw-editor-size", String(size));
    editor.updateOptions(sizeOptions(size));
    showSize();
  });

  let linter;
  let runner;
  let problems;
  let format = () => {};
  const files = new Files({
    monaco, editor, slug, renamedFrom, user, files: boot.files,
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
      hush.typed();
    },
    onOpen: () => leader?.redraw(),
  });
  // After the code autosave's own listeners, so the code goes first.
  notesSave.attachLeave();

  leader = new Leader({
    root: $("[data-layout]"), svg: $("[data-leader]"), editor, files,
    column: $("[data-problems-panel]"), view: $("[data-problem-view]"),
    results: $("#panel-tests"), treeWrap: $("#panel-tests"),
    jump: $("[data-leader-jump]"), treeJump: $("[data-tree-jump]"),
    docks: { top: $(".filebar-end"), bottom: $(".bottom-bar .run-controls") },
  });
  const stepSelect = $("[data-step-select]");
  const race = $("[data-race]");
  // A disabled toggle still asks for -race: the site drops it while the
  // runner has no race detector, and a new runner pod may have one.
  const run = (step = stepSelect.value) => runner.run(step, race.checked || race.disabled);
  const raceUnavailable = () => {
    if (race.disabled) return;
    race.checked = false;
    race.disabled = true;
    race.setAttribute("aria-label", "Run with the race detector, -race. Not available on this server");
    setRace("not available");
  };
  problems = new Problems({
    monaco, editor, files,
    chips: $("[data-chips]"), view: $("[data-problem-view]"),
    counts: [...ide.querySelectorAll("[data-problem-count]")],
    countText: $("[data-problem-count-text]"),
    status: $("[data-lint-status]"), titles, leader, announce,
    // The check renumbers the problems as you type; the tree's pins follow.
    onCount: (n) => {
      layout.setProblemCount(n);
      runner?.problemsChanged();
    },
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

  race.addEventListener("change", () => setRace(race.checked ? "on" : "off"));
  runner = new TestRunner({
    slug, number, steps: boot.steps, files, problems,
    tree: $("[data-test-tree]"), detail: $("[data-test-detail]"), summary: $("[data-run-summary]"), notes: $("[data-run-notes]"),
    firstPassLine: ide.dataset.firstPass || "",
    output: $("[data-output]"), announce,
    runButton: $("[data-run]"), stopButton: $("[data-stop]"),
    next: ide.dataset.nextSlug ? { slug: ide.dataset.nextSlug, title: ide.dataset.nextTitle } : null,
    showPanel: (name) => tabs.show("bottom", `panel-${name}`),
    showProblem,
    onStart: () => {
      layout.expandBottom();
      hush.runStarted();
    },
    onFinish: (result) => hush.runFinished(result),
    onRaceUnavailable: raceUnavailable,
    onProgress: (p) => {
      setText(cells.runs, String(p.runs));
      activeSeconds = p.activeSeconds;
      showActive();
      hush.setActive(p.activeSeconds);
      if (p.status === "passed") {
        showPassed(p.passedAt);
        hints.setPassed(true);
        hush.setPassed();
      }
    },
  });
  problems.tests = runner;
  leader.tests = runner;
  runner.runs = progress.runs || 0;
  if (boot.raceUnavailable) runner.raceNote(false);
  if (progress.lastResult && progress.runs) {
    runner.restore(progress.lastResult, progress.runs, boot.files);
    hush.runFinished(runner.result());
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

  // The Keys menu: each row does what its key does.
  const keyActions = {
    run: () => run(),
    save: () => files.flush().catch(() => {}),
    next: () => problems.next(1),
    format: () => format(),
    tab: () => {
      editor.focus();
      editor.trigger("keys", "editor.action.toggleTabFocusMode", null);
    },
  };
  for (const b of ide.querySelectorAll("[data-key-action]")) b.addEventListener("click", () => keyActions[b.dataset.keyAction]?.());

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
  const cursorKey = (name, from = slug) => `gpw-cursor:${from}:${name}`;
  if (renamedFrom) {
    for (const f of boot.files) {
      const kept = load(cursorKey(f.name, renamedFrom));
      if (kept !== null && load(cursorKey(f.name)) === null) save(cursorKey(f.name), kept);
      save(cursorKey(f.name, renamedFrom), null);
    }
  }
  let cursorTimer = 0;
  let pendingCursor = null;
  const flushCursor = () => {
    clearTimeout(cursorTimer);
    if (pendingCursor) save(cursorKey(pendingCursor.name), pendingCursor.at);
    pendingCursor = null;
  };
  editor.onDidChangeCursorPosition((e) => {
    setText(cursorCell, `Line ${e.position.lineNumber}, column ${e.position.column}`);
    const name = files.activeName();
    // A switch to another file must not drop the last move in this one.
    if (pendingCursor && pendingCursor.name !== name) flushCursor();
    pendingCursor = { name, at: `${e.position.lineNumber}:${e.position.column}` };
    clearTimeout(cursorTimer);
    cursorTimer = setTimeout(flushCursor, 500);
  });
  const first = files.activeName();
  const stored = (load(cursorKey(first)) || "").split(":").map(Number);
  if (stored[0] > 1) {
    editor.setPosition({ lineNumber: stored[0], column: stored[1] || 1 });
    editor.revealLineInCenter(stored[0]);
  } else {
    setText(cursorCell, "Line 1, column 1");
  }
  editor.onDidFocusEditorText(() => hints.editorFocused());

  hush.check();
  ide.dataset.ready = "true"; // the E2E tests wait for this
}

main();
