// main.js: the IDE page. It loads the exercise, starts Monaco and connects
// the parts: files and autosave, lint, format, test runs, hints, notes and
// active time.
import { api, errorMessage } from "../api.js";
import { isDark } from "../theme.js";
import { setupNotes } from "./notes.js";
import { Hints } from "./hints.js";
import { trackActivity } from "./activity.js";
import { setupTabs, setupSplitters } from "./panels.js";
import { loadMonaco, defineThemes, themeName } from "./monaco.js";
import { Files } from "./files.js";
import { Linter } from "./lint.js";
import { TestRunner } from "./tests.js";
import { registerDocs } from "./docs.js";

const root = document.querySelector(".ide");
const slug = root.dataset.exercise;
const $ = (sel) => root.querySelector(sel);

const showPanel = setupTabs(root);
setupSplitters(root);

const statusBadge = $("[data-progress-status]");
function showStatus(status) {
  statusBadge.textContent = status === "passed" ? "Passed" : "Started";
  statusBadge.className = `badge badge-${status === "passed" ? "passed" : "started"}`;
}

const saveStatus = $("[data-save-status]");
const lintStatus = $("[data-lint-status]");

async function main() {
  let boot;
  try {
    boot = await api("GET", `/api/exercises/${slug}`);
  } catch (err) {
    $("[data-editor]").textContent = `The exercise could not load: ${errorMessage(err)}`;
    return;
  }
  showStatus(boot.progress.status);
  new Hints({ slug, ladder: $("[data-hint-ladder]"), gate: $("[data-gate]"), revealed: boot.revealed });
  setupNotes({ slug, textarea: $("[data-notes]"), status: $("[data-notes-status]"), initial: boot.progress.notes });
  trackActivity({ slug, onActiveSeconds: () => {} });

  let monaco;
  try {
    monaco = await loadMonaco();
  } catch (err) {
    $("[data-editor]").textContent = `${errorMessage(err)} Reload the page to try again.`;
    return;
  }
  defineThemes(monaco);
  registerDocs(monaco);
  const host = $("[data-editor]");
  host.replaceChildren();
  const editor = monaco.editor.create(host, {
    theme: themeName(isDark()),
    automaticLayout: true,
    minimap: { enabled: false },
    fontSize: 14,
    tabSize: 4,
    insertSpaces: false, // gofmt uses tabs
    scrollBeyondLastLine: false,
    renderWhitespace: "selection",
    "semanticHighlighting.enabled": false,
  });
  document.addEventListener("gpw:theme", () => monaco.editor.setTheme(themeName(isDark())));

  let linter;
  const files = new Files({
    monaco,
    editor,
    slug,
    files: boot.files,
    tabs: $("[data-file-tabs]"),
    onSaveStatus: (text) => {
      saveStatus.textContent = text;
    },
    onEdit: () => linter.schedule(),
  });
  linter = new Linter({
    monaco,
    files,
    slug,
    list: $("[data-problems]"),
    count: $("[data-problem-count]"),
    onStatus: (text) => {
      lintStatus.textContent = text;
    },
  });
  linter.run();

  const runner = new TestRunner({
    slug,
    files,
    tree: $("[data-test-tree]"),
    summary: $("[data-run-summary]"),
    output: $("[data-output]"),
    explanations: $("[data-explanations]"),
    runButton: $("[data-run]"),
    stopButton: $("[data-stop]"),
    showPanel: (name) => showPanel("bottom", `bottom-${name}`),
    onProgress: (p) => {
      showStatus(p.status);
      if (p.firstPass) {
        $("[data-run-summary]").textContent += " You passed this exercise! It counts toward your certificates.";
      }
    },
  });
  const stepSelect = $("[data-step-select]");
  const race = $("[data-race]");
  const run = (step = stepSelect.value) => runner.run(step, race.checked);
  $("[data-run]").addEventListener("click", () => run());
  $("[data-stop]").addEventListener("click", () => runner.stop());
  for (const b of root.querySelectorAll("[data-step]")) {
    b.addEventListener("click", () => {
      stepSelect.value = b.dataset.step;
      run(b.dataset.step);
    });
  }

  const format = async () => {
    const name = files.activeName();
    if (!name || !(name in files.stubContents())) return;
    try {
      const res = await api("POST", `/api/exercises/${slug}/format`, { file: name, content: files.model(name).getValue() });
      if (res.diagnostics?.length) {
        linter.show(res.diagnostics);
        showPanel("bottom", "bottom-problems");
        return;
      }
      if (res.content !== files.model(name).getValue()) {
        editor.pushUndoStop();
        files.model(name).pushEditOperations([], [{ range: files.model(name).getFullModelRange(), text: res.content }], () => null);
        editor.pushUndoStop();
      }
    } catch (err) {
      saveStatus.textContent = `Format failed: ${errorMessage(err)}`;
    }
  };
  $("[data-format]").addEventListener("click", format);
  $("[data-reset]").addEventListener("click", () => files.resetActive());

  const { KeyMod, KeyCode } = monaco;
  editor.addCommand(KeyMod.CtrlCmd | KeyCode.KeyS, () => files.flush());
  editor.addCommand(KeyMod.CtrlCmd | KeyCode.Enter, () => run());
  editor.addCommand(KeyMod.Shift | KeyMod.Alt | KeyCode.KeyF, format);
  document.addEventListener("keydown", (event) => {
    if ((event.ctrlKey || event.metaKey) && event.key === "Enter" && !editor.hasTextFocus()) {
      event.preventDefault();
      run();
    }
  });
  root.dataset.ready = "true"; // the E2E tests wait for this
}

main();
