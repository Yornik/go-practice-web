// monaco.js: loads the vendored Monaco editor with its AMD loader. Monaco
// is copied to /static/vendor/monaco/vs by scripts/fetch-monaco.sh.

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

/** Defines the site's light and dark editor themes. */
export function defineThemes(monaco) {
  monaco.editor.defineTheme("gpw-light", {
    base: "vs",
    inherit: true,
    rules: [],
    colors: { "editor.background": "#ffffff", "editorLineNumber.foreground": "#8a9099" },
  });
  monaco.editor.defineTheme("gpw-dark", {
    base: "vs-dark",
    inherit: true,
    rules: [],
    colors: { "editor.background": "#1a1d23", "editorLineNumber.foreground": "#6b7280" },
  });
}

/** The theme that matches the page. */
export function themeName(dark) {
  return dark ? "gpw-dark" : "gpw-light";
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
