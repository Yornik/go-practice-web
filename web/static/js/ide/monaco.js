// monaco.js: loads the vendored Monaco editor with its AMD loader, and
// defines the site's 2 editor themes (".design/FINAL.md", section 11).
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

// Monaco's Go tokenizer marks the builtin types, true, false and nil as
// keywords; these rules colour them like numbers.
const TEAL_WORDS = ["bool", "byte", "complex64", "complex128", "float32", "float64", "int", "int8", "int16", "int32", "int64",
  "rune", "string", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "error", "any", "true", "false", "nil"];

const PALETTE = {
  light: {
    graphite: "1E2821", graphite2: "475A4C", kw: "6E2C7E", str: "7A4E05", num: "0E6670", com: "56685B", red: "B3251A",
    colors: {
      bg: "#F6F9F1", fg: "#1E2821", fg2: "#475A4C", line: "#ECF2E4", blue: "#2346A0", wash: "#E1E8F6", select: "#C9D5EF",
      rule: "#BCCDB0", ruleStrong: "#6B8562", indent: "#E0E8D6", red: "#B3251A", scroll: "#6B8562",
    },
  },
  dark: {
    graphite: "DDE6D7", graphite2: "A3B4A7", kw: "DDA9EE", str: "E3C27C", num: "7ED3D9", com: "91A396", red: "FF9086",
    colors: {
      bg: "#1A251F", fg: "#DDE6D7", fg2: "#A3B4A7", line: "#16201B", blue: "#A3BAFF", wash: "#1E2944", select: "#2E3F6B",
      rule: "#2E4035", ruleStrong: "#5C7766", indent: "#223028", red: "#FF9086", scroll: "#5C7766",
    },
  },
};

function rules(c) {
  return [
    { token: "", foreground: c.graphite },
    { token: "identifier", foreground: c.graphite },
    { token: "delimiter", foreground: c.graphite },
    { token: "keyword", foreground: c.kw },
    ...TEAL_WORDS.map((w) => ({ token: `keyword.${w}`, foreground: c.num })),
    { token: "type", foreground: c.num },
    { token: "number", foreground: c.num },
    { token: "string", foreground: c.str },
    { token: "string.escape", foreground: c.str },
    { token: "string.invalid", foreground: c.red },
    { token: "string.escape.invalid", foreground: c.red },
    // Comments stay upright: italics tire the eye in long sessions.
    { token: "comment", foreground: c.com },
    { token: "comment.doc", foreground: c.com },
    { token: "annotation", foreground: c.graphite2 },
  ];
}

function colors(k, dark) {
  const a = (hex, alpha) => hex + alpha;
  return {
    "editor.background": k.bg,
    "editor.foreground": k.fg,
    "editorGutter.background": k.bg,
    "editorLineNumber.foreground": k.fg2,
    "editorLineNumber.activeForeground": k.fg,
    "editor.lineHighlightBackground": k.line,
    "editor.lineHighlightBorder": "#00000000",
    "editorCursor.foreground": k.blue,
    "editor.selectionBackground": k.select,
    "editor.inactiveSelectionBackground": k.wash,
    "editor.selectionHighlightBackground": k.wash,
    "editor.wordHighlightBackground": k.wash,
    "editor.wordHighlightStrongBackground": k.wash,
    "editor.findMatchBackground": k.select,
    "editor.findMatchBorder": k.blue,
    "editor.findMatchHighlightBackground": k.wash,
    "editorBracketMatch.background": k.wash,
    "editorBracketMatch.border": k.blue,
    "editorWhitespace.foreground": k.rule,
    "editorIndentGuide.background1": k.indent,
    "editorIndentGuide.activeBackground1": k.rule,
    "editorError.foreground": k.red,
    "editorWarning.foreground": k.blue,
    "editorInfo.foreground": k.blue,
    "editorOverviewRuler.border": "#00000000",
    "editorOverviewRuler.errorForeground": k.red,
    "editorOverviewRuler.warningForeground": k.blue,
    "scrollbarSlider.background": a(k.scroll, dark ? "40" : "33"),
    "scrollbarSlider.hoverBackground": a(k.scroll, dark ? "80" : "66"),
    "scrollbarSlider.activeBackground": a(k.scroll, dark ? "B3" : "99"),
    "editorWidget.background": k.bg,
    "editorWidget.border": k.ruleStrong,
    "editorWidget.foreground": k.fg,
    "editorHoverWidget.background": k.bg,
    "editorHoverWidget.border": k.ruleStrong,
    "editorSuggestWidget.background": k.bg,
    "editorSuggestWidget.border": k.ruleStrong,
    "editorSuggestWidget.foreground": k.fg,
    "editorSuggestWidget.selectedBackground": k.wash,
    "editorSuggestWidget.highlightForeground": k.blue,
    "editorMarkerNavigation.background": k.line,
    "editorMarkerNavigationError.background": k.red,
    "editorMarkerNavigationWarning.background": k.blue,
    "input.background": k.bg,
    "input.border": k.ruleStrong,
    focusBorder: k.blue,
    "editorStickyScroll.background": k.bg,
  };
}

/** Defines gpw-light and gpw-dark. */
export function defineThemes(monaco) {
  for (const [name, base, dark] of [["gpw-light", "vs", false], ["gpw-dark", "vs-dark", true]]) {
    const p = PALETTE[dark ? "dark" : "light"];
    monaco.editor.defineTheme(name, { base, inherit: true, rules: rules(p), colors: colors(p.colors, dark) });
  }
}

/** The theme that matches the page. */
export function themeName(dark) {
  return dark ? "gpw-dark" : "gpw-light";
}

/** The editor options of section 11.3. */
export const EDITOR_OPTIONS = {
  fontFamily: '"Spline Sans Mono", ui-monospace, "SFMono-Regular", Menlo, Consolas, monospace',
  fontSize: 13,
  lineHeight: 20,
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
  // The stub's doc comments are part of the spec: long lines wrap at 100
  // columns, or at the edge of a narrower editor, instead of hiding.
  wordWrap: "bounded",
  wordWrapColumn: 100,
  wrappingIndent: "same",
};

/** The sizes the learner can pick for the editor's text, in px. */
export const EDITOR_SIZES = [13, 14, 15, 16, 18];

/** The editor text size the learner picked (account page or Keys popover). */
export function editorSize() {
  let n = 13;
  try {
    n = Number(localStorage.getItem("gpw-editor-size")) || 13;
  } catch {
    // Storage blocked: the default size.
  }
  return EDITOR_SIZES.includes(n) ? n : 13;
}

/** The font size and a line height of 1.5 for it. */
export function sizeOptions(size) {
  return { fontSize: size, lineHeight: Math.round(size * 1.5) };
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
