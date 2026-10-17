// docs.js: standard library docs in the editor: hover on pkg.Name and
// completion after "pkg.", from the server's index (/api/docs/). The
// file's imports map package names to import paths.
import { api } from "../api.js";

const cache = new Map();

function cached(url) {
  if (!cache.has(url)) cache.set(url, api("GET", url).catch(() => null));
  return cache.get(url);
}

/** Returns a map from package name to import path for a Go source. */
export function imports(src) {
  const out = new Map();
  const add = (alias, path) => {
    if (alias === "_" || alias === ".") return;
    let name = alias;
    if (!name) {
      const parts = path.split("/");
      name = parts[parts.length - 1];
      // math/rand/v2 is package rand.
      if (/^v\d+$/.test(name) && parts.length > 1) name = parts[parts.length - 2];
    }
    out.set(name, path);
  };
  const spec = /^\s*(?:([\w.]+)\s+)?"([^"]+)"/;
  for (const m of src.matchAll(/^import\s+(?:([\w.]+)\s+)?"([^"]+)"/gm)) add(m[1], m[2]);
  for (const block of src.matchAll(/^import\s*\(([\s\S]*?)\)/gm)) {
    for (const line of block[1].split("\n")) {
      const m = line.match(spec);
      if (m) add(m[1], m[2]);
    }
  }
  return out;
}

const KINDS = { func: "Function", method: "Method", type: "Class", const: "Constant", var: "Variable", field: "Field" };

function markdown(d) {
  const parts = ["```go\n" + d.signature + "\n```"];
  if (d.deprecated) parts.push(`**Deprecated:** ${d.deprecated}`);
  if (d.doc) parts.push(d.doc);
  if (d.url) parts.push(`[Open on pkg.go.dev](${d.url})`);
  return { value: parts.join("\n\n"), isTrusted: false };
}

/** Registers hover and completion for Go. */
export function registerDocs(monaco) {
  monaco.languages.registerCompletionItemProvider("go", {
    triggerCharacters: ["."],
    async provideCompletionItems(model, position) {
      const before = model.getLineContent(position.lineNumber).slice(0, position.column - 1);
      const m = before.match(/([A-Za-z_]\w*)\.([A-Za-z_]\w*)?$/);
      if (!m) return { suggestions: [] };
      const path = imports(model.getValue()).get(m[1]);
      if (!path) return { suggestions: [] };
      const prefix = m[2] || "";
      const res = await cached(`/api/docs/complete?path=${encodeURIComponent(path)}&prefix=${encodeURIComponent(prefix)}&limit=200`);
      if (!res) return { suggestions: [] };
      const range = {
        startLineNumber: position.lineNumber,
        endLineNumber: position.lineNumber,
        startColumn: position.column - prefix.length,
        endColumn: position.column,
      };
      return {
        incomplete: res.incomplete,
        suggestions: res.items
          .filter((d) => !d.recv || prefix.includes("."))
          .map((d) => ({
            label: d.name,
            kind: monaco.languages.CompletionItemKind[KINDS[d.kind] || "Text"],
            detail: d.signature,
            documentation: markdown(d),
            insertText: d.name,
            range,
          })),
      };
    },
  });

  monaco.languages.registerHoverProvider("go", {
    async provideHover(model, position) {
      const word = model.getWordAtPosition(position);
      if (!word) return null;
      const line = model.getLineContent(position.lineNumber);
      const before = line.slice(0, word.startColumn - 1);
      const pkgs = imports(model.getValue());
      const range = new monaco.Range(position.lineNumber, word.startColumn, position.lineNumber, word.endColumn);
      const sel = before.match(/([A-Za-z_]\w*)\.$/);
      if (sel && pkgs.has(sel[1])) {
        const d = await cached(`/api/docs/member?path=${encodeURIComponent(pkgs.get(sel[1]))}&name=${encodeURIComponent(word.word)}`);
        return d ? { range, contents: [markdown(d)] } : null;
      }
      if (pkgs.has(word.word) && line.charAt(word.endColumn - 1) === ".") {
        const p = await cached(`/api/docs/pkg?path=${encodeURIComponent(pkgs.get(word.word))}`);
        if (!p) return null;
        return { range, contents: [{ value: `**package ${p.name}** (\`${p.path}\`)\n\n${p.synopsis || ""}\n\n[Open on pkg.go.dev](${p.url})` }] };
      }
      return null;
    },
  });
}
