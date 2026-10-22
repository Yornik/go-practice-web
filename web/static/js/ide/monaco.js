// monaco.js: loads the vendored Monaco editor with its AMD loader, and
// defines the site's 2 editor themes (".design/VESPER.md", section 11).
// Monaco is copied to /static/vendor/monaco/vs by scripts/fetch-monaco.sh.

const BASE = "/static/vendor/monaco/vs";
let loading = null;

/** Loads Monaco once and resolves with the global monaco namespace. */
export function loadMonaco() {
  if (loading) return loading;
  loading = new Promise((resolve, reject) => {
    if (window.monaco) {
      resolve(window.monaco);
      return;
    }
    const script = document.createElement("script");
    script.src = `${BASE}/loader.js`;
    script.onload = () => {
      window.require.config({ paths: { vs: BASE } });
      window.require(["vs/editor/editor.main"], () => resolve(window.monaco), reject);
    };
    script.onerror = () => reject(new Error("The editor could not load."));
    document.head.appendChild(script);
  });
  return loading;
}

// The predeclared types and constants: --syn-t, upright, not bold. Monaco's
// Go tokenizer marks them as keywords, so these more specific rules win.
const PREDECLARED = ["bool", "byte", "complex64", "complex128", "float32", "float64", "int", "int8", "int16", "int32", "int64",
  "rune", "string", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "error", "any", "true", "false", "nil"];

// The v2 tokens (VESPER.md 3.11, 11.2), written out: Monaco cannot read
// CSS. No amber fill, no cyan and no glow in the editor.
const PALETTE = {
  light: {
    ink: "070707", ink2: "545659", kw: "1F4E79", str: "7A4500", typ: "3D5F1C", com: "595D61", red: "BD2A1E",
    colors: {
      editor: "#FFFFFF", ink: "#070707", lineNo: "#595D61", surface: "#F5F5F4", signal: "#AD4F00", select: "#FFE2A6",
      wash: "#FFF0D1", rule: "#D6D7D9", ruleStrong: "#76797D", red: "#BD2A1E",
    },
  },
  dark: {
    ink: "EDEEEF", ink2: "A3A6AA", kw: "A9C4DD", str: "DDBE8E", typ: "B5CF9C", com: "8E9398", red: "FF6150",
    colors: {
      editor: "#0A0A0B", ink: "#EDEEEF", lineNo: "#8E9398", surface: "#0E0F10", signal: "#FFAA00", select: "#3D3112",
      wash: "#261B04", rule: "#2A2C2F", ruleStrong: "#777B80", red: "#FF6150",
    },
  },
};

function rules(c) {
  return [
    { token: "", foreground: c.ink },
    { token: "identifier", foreground: c.ink },
    { token: "delimiter", foreground: c.ink },
    { token: "number", foreground: c.ink }, // numbers are ink
    { token: "keyword", foreground: c.kw, fontStyle: "bold" },
    ...PREDECLARED.map((w) => ({ token: `keyword.${w}`, foreground: c.typ, fontStyle: "" })),
    { token: "type", foreground: c.typ },
    { token: "string", foreground: c.str },
    { token: "string.escape", foreground: c.str },
    { token: "string.invalid", foreground: c.red },
    { token: "string.escape.invalid", foreground: c.red },
    // Comments stay upright: italics tire the eye in 2-hour sessions.
    { token: "comment", foreground: c.com },
    { token: "comment.doc", foreground: c.com },
    { token: "annotation", foreground: c.ink2 },
  ];
}

function colors(k, dark) {
  // The scrollbar is --rule-strong with alpha: 20 %, 40 % and 60 % by day, 25 %, 50 % and 70 % at night.
  const scroll = (alpha) => k.ruleStrong + alpha;
  return {
    "editor.background": k.editor,
    "editorGutter.background": k.editor,
    "editorStickyScroll.background": k.editor,
    "editor.foreground": k.ink,
    "editorLineNumber.activeForeground": k.ink,
    "editorWidget.foreground": k.ink,
    "editorSuggestWidget.foreground": k.ink,
    // Monaco's defaults for the selected row are white text and a pale blue
    // match, made for its dark blue bar; on the wash they would vanish.
    "editorSuggestWidget.selectedForeground": k.ink,
    "editorSuggestWidget.selectedIconForeground": k.ink,
    "editorLineNumber.foreground": k.lineNo,
    "editor.lineHighlightBackground": k.surface,
    "editorMarkerNavigation.background": k.surface,
    "editor.lineHighlightBorder": "#00000000",
    "editorOverviewRuler.border": "#00000000",
    // The cursor is where you write: amber, as every action of a person.
    "editorCursor.foreground": k.signal,
    focusBorder: k.signal,
    "editor.findMatchBorder": k.signal,
    "editorBracketMatch.border": k.signal,
    "editorSuggestWidget.highlightForeground": k.signal,
    "editorSuggestWidget.focusHighlightForeground": k.signal,
    // Advice, not a failure: staticcheck's notes are amber.
    "editorWarning.foreground": k.signal,
    "editorInfo.foreground": k.signal,
    "editorOverviewRuler.warningForeground": k.signal,
    "editorMarkerNavigationWarning.background": k.signal,
    "editor.selectionBackground": k.select,
    "editor.findMatchBackground": k.select,
    "editor.inactiveSelectionBackground": k.wash,
    "editor.selectionHighlightBackground": k.wash,
    "editor.wordHighlightBackground": k.wash,
    "editor.wordHighlightStrongBackground": k.wash,
    "editor.findMatchHighlightBackground": k.wash,
    "editorBracketMatch.background": k.wash,
    "editorSuggestWidget.selectedBackground": k.wash,
    "editorWhitespace.foreground": k.rule,
    "editorIndentGuide.background1": k.rule,
    "editorIndentGuide.activeBackground1": k.ruleStrong,
    "editorError.foreground": k.red,
    "editorOverviewRuler.errorForeground": k.red,
    "editorMarkerNavigationError.background": k.red,
    "scrollbarSlider.background": scroll(dark ? "40" : "33"),
    "scrollbarSlider.hoverBackground": scroll(dark ? "80" : "66"),
    "scrollbarSlider.activeBackground": scroll(dark ? "B3" : "99"),
    // Hover and suggest are floating objects: a surface plane with a strong edge (3.6).
    "editorWidget.background": k.surface,
    "editorHoverWidget.background": k.surface,
    "editorSuggestWidget.background": k.surface,
    "input.background": k.surface,
    "editorWidget.border": k.ruleStrong,
    "editorHoverWidget.border": k.ruleStrong,
    "editorSuggestWidget.border": k.ruleStrong,
    "input.border": k.ruleStrong,
    // A link in a hover ("View Problem") is an action: amber, not Monaco's blue.
    "textLink.foreground": k.signal,
    "textLink.activeForeground": k.signal,
    // No bracket-pair colours (11.3); calmModels turns them off, and
    // these keep any that slip through in ink.
    ...Object.fromEntries([1, 2, 3, 4, 5, 6].map((i) => [`editorBracketHighlight.foreground${i}`, k.ink])),
    "editorBracketHighlight.unexpectedBracket.foreground": k.ink,
  };
}

const forced = matchMedia("(forced-colors: active)");

/** A system colour as #RRGGBB: forced colours pick them, and Monaco needs hex. */
function systemColor(name) {
  const probe = document.createElement("i");
  probe.style.color = name;
  document.body.append(probe);
  const rgb = (getComputedStyle(probe).color.match(/\d+/g) || ["0", "0", "0"]).slice(0, 3).map(Number);
  probe.remove();
  return `#${rgb.map((x) => x.toString(16).padStart(2, "0")).join("").toUpperCase()}`;
}

/**
 * gpw-forced: the editor in forced colours, as the frame draws it. The
 * code is CanvasText with bold keywords on Canvas, the selection
 * Highlight. Monaco paints its own colours there, so the theme reads the
 * system colours of this moment.
 */
function defineForced(monaco) {
  const ink = systemColor("CanvasText");
  const canvas = systemColor("Canvas");
  const highlight = systemColor("Highlight");
  const dark = parseInt(canvas.slice(1, 3), 16) + parseInt(canvas.slice(3, 5), 16) + parseInt(canvas.slice(5, 7), 16) < 384;
  const t = ink.slice(1);
  monaco.editor.defineTheme("gpw-forced", {
    base: dark ? "hc-black" : "hc-light",
    inherit: true,
    rules: rules({ ink: t, ink2: t, kw: t, str: t, typ: t, com: t, red: t }),
    colors: {
      "editor.background": canvas,
      "editor.foreground": ink,
      "editorLineNumber.foreground": ink,
      "editorLineNumber.activeForeground": ink,
      "editorCursor.foreground": ink,
      "editor.selectionBackground": highlight,
      "editor.lineHighlightBorder": ink,
      contrastBorder: ink,
      contrastActiveBorder: highlight,
      focusBorder: highlight,
      "textLink.foreground": ink,
      "textLink.activeForeground": ink,
      ...Object.fromEntries([1, 2, 3, 4, 5, 6].map((i) => [`editorBracketHighlight.foreground${i}`, ink])),
      "editorBracketHighlight.unexpectedBracket.foreground": ink,
      // The squiggles and their underlines too: hc-black's own reds ignore
      // the system palette.
      "editorError.foreground": ink,
      "editorWarning.foreground": ink,
      "editorInfo.foreground": ink,
      "editorError.border": ink,
      "editorWarning.border": ink,
      "editorInfo.border": ink,
      "editorOverviewRuler.errorForeground": ink,
      "editorOverviewRuler.warningForeground": ink,
      "editorOverviewRuler.infoForeground": ink,
    },
  });
}

/**
 * Defines gpw-light, gpw-dark and gpw-forced, and keeps the editor's theme
 * right when forced colours start or stop: onChange sets it again.
 */
export function defineThemes(monaco, onChange = () => {}) {
  for (const [name, base, dark] of [["gpw-light", "vs", false], ["gpw-dark", "vs-dark", true]]) {
    const p = PALETTE[dark ? "dark" : "light"];
    monaco.editor.defineTheme(name, { base, inherit: true, rules: rules(p), colors: colors(p.colors, dark) });
  }
  if (forced.matches) defineForced(monaco);
  forced.addEventListener("change", () => {
    if (forced.matches) defineForced(monaco);
    onChange();
  });
}

/** The theme that matches the page. */
export function themeName(dark) {
  if (forced.matches) return "gpw-forced";
  return dark ? "gpw-dark" : "gpw-light";
}

/**
 * Turns bracket-pair colours off on every model, now and later (VESPER.md
 * 11.3). Monaco 0.57 takes them from the model's own options, which the
 * editor option bracketPairColorization does not reach.
 */
export function calmModels(monaco) {
  const calm = (model) => model.updateOptions({ bracketColorizationOptions: { enabled: false, independentColorPoolPerBracketType: false } });
  monaco.editor.getModels().forEach(calm);
  monaco.editor.onDidCreateModel(calm);
}


/** The editor options of section 11.3. */
export const EDITOR_OPTIONS = {
  fontFamily: '"Atkinson Hyperlegible Mono", ui-monospace, "SFMono-Regular", Menlo, Consolas, monospace',
  fontSize: 14,
  lineHeight: 20,
  fontWeight: "400",
  fontLigatures: false,
  letterSpacing: 0,
  tabSize: 4,
  insertSpaces: false, // gofmt uses tabs
  detectIndentation: false,
  minimap: { enabled: false },
  stickyScroll: { enabled: false },
  bracketPairColorization: { enabled: false },
  guides: { indentation: true, bracketPairs: false },
  matchBrackets: "always",
  renderLineHighlight: "line",
  renderWhitespace: "none",
  cursorBlinking: "solid",
  cursorSmoothCaretAnimation: "off",
  smoothScrolling: false,
  scrollBeyondLastLine: false,
  glyphMargin: true,
  lineNumbersMinChars: 3,
  padding: { top: 8, bottom: 8 },
  fixedOverflowWidgets: true,
  occurrencesHighlight: "singleFile",
  folding: true,
  showFoldingControls: "mouseover",
  "semanticHighlighting.enabled": false,
  accessibilitySupport: "auto",
  automaticLayout: true,
  // Lines wrap at the column edge, so nothing hides; a wrapped part starts
  // 1 level deeper, so a wrapped comment never reads as a statement.
  wordWrap: "on",
  wrappingIndent: "indent",
};

/** The sizes the learner can pick for the editor's text, in px. */
export const EDITOR_SIZES = [13, 14, 15, 16, 18];

/** The size of a new browser: 14 px on 20 px rows (VESPER.md 11.3). */
export const EDITOR_SIZE = 14;

/** The editor text size the learner picked (account page or Keys menu). */
export function editorSize() {
  let n = EDITOR_SIZE;
  try {
    n = Number(localStorage.getItem("gpw-editor-size")) || EDITOR_SIZE;
  } catch {
    // Storage blocked: the default size.
  }
  return EDITOR_SIZES.includes(n) ? n : EDITOR_SIZE;
}

/**
 * The gutter: below 900 px 34 px of line numbers and 14 px for the problem
 * mark (VESPER.md 7.11), so a phone shows more of each line; folding goes,
 * its arrows need room. Wider, the options of 11.3.
 */
export function gutterOptions(narrow) {
  return narrow
    ? { lineNumbersMinChars: 2, lineDecorationsWidth: 8, folding: false }
    : { lineNumbersMinChars: EDITOR_OPTIONS.lineNumbersMinChars, lineDecorationsWidth: 10, folding: EDITOR_OPTIONS.folding };
}

/** The font size and its row height: 20 px rows at 14 px, in proportion for the others. */
export function sizeOptions(size) {
  return { fontSize: size, lineHeight: Math.round((size * 20) / 14) };
}

/**
 * Converts a byte column of the Go tools to the UTF-16 column Monaco uses.
 * They differ on lines with non-ASCII text.
 */
export function byteToUtf16Column(lineText, byteColumn) {
  if (!lineText || byteColumn <= 1) return Math.max(1, byteColumn);
  const encoder = new TextEncoder();
  let bytes = 0;
  let units = 0;
  for (const ch of lineText) {
    const n = encoder.encode(ch).length;
    if (bytes + n > byteColumn - 1) break;
    bytes += n;
    units += ch.length;
  }
  // Past the end of the line: keep the overshoot, Monaco clamps it.
  return units + 1 + Math.max(0, byteColumn - 1 - bytes);
}
