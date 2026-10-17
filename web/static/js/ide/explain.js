// explain.js: the explanation of 1 problem, as the problems column shows
// it: where it comes from, the plain title, the raw message with its
// parts labelled (the anatomy), the catalog's sections as disclosures and
// the course exercise that teaches the concept. The server sends the body
// as safe HTML (rendered from the reviewed catalog, every captured value
// escaped); every other value goes in as text.
//
// Typography rule: every value the machine printed (a capture, a call, a
// name) is set in mono. B612 draws "(" and ")" almost like "[" and "]",
// and in a Go course "Validate()" must not read as "Validate[]".
import { balloon, h, icon, load, save } from "../ui/dom.js";
import { clock } from "../ui/format.js";

// A plain label per capture group of the catalog (internal/explain/catalog).
const LABELS = {
  name: "The name the message is about.",
  got: "Got: what your code did.",
  want: "Want: what the test or the type expects.",
  have: "Have: what Go found instead.",
  type: "The type.",
  expr: "The expression Go checked.",
  what: "What Go knows about the expression: its kind and its type.",
  call: "The call the message is about.",
  case: "The case: a row of the test's table. Find it by this name in the test file.",
  method: "The method.",
  func: "The function.",
  reason: "Why Go refuses it.",
  value: "The value Go put in the message.",
  op: "The operator or the operation.",
  kind: "The kind of statement or literal.",
  arg: "The argument.",
  found: "What the parser found instead.",
  stmt: "The statement.",
  left: "The type on the left of the operator.",
  right: "The type on the right of the operator.",
  problem: "What is wrong with the literal.",
  which: "Too many or not enough.",
  verb: "The formatting verb.",
  param: "The type parameter.",
  limit: "How long the test waited.",
  iface: "The interface the value must satisfy.",
  context: "Where the value is used.",
  where: "Where the comma is missing.",
  to: "The type it converts to.",
  from: "The type it converts from.",
  result: "The result that is not used.",
  n: "The count.",
  field: "The field.",
  char: "The character Go cannot read.",
  action: "What the call had to do.",
  after: "The time limit of the run.",
  path: "The import path.",
  alias: "The name of the import.",
  pkg: "The package.",
  label: "The label.",
  version: "The Go version the feature needs.",
  feature: "The language feature.",
  lang: "The Go version that go.mod sets.",
  vars: "The number of variables on the left.",
  values: "The number of values on the right.",
  wait: "How long the test waited for the goroutines.",
  bad: "The value that is wrong.",
  good: "The value that works.",
  key: "The key.",
  index: "The index.",
  length: "The length.",
  bounds: "The bounds.",
  size: "The size.",
  shift: "The shift count.",
  bits: "The number of bits.",
  symbol: "The symbol.",
  tag: "The struct tag.",
  format: "The format string.",
  form: "The form of the call.",
  mode: "The mode.",
  lock: "The lock.",
  scope: "The scope.",
  rule: "The rule.",
  detail: "The detail Go added.",
  constraint: "The type constraint.",
  close: "The closing token.",
  at: "Where it happened.",
  first: "The first one.",
  second: "The second one.",
  firstfile: "The file of the first one.",
  secondfile: "The file of the second one.",
  octal: "The number with a leading 0.",
  when: "When it happened.",
  need: "What Go needs.",
};
// Labels that 1 entry says better.
const ENTRY_LABELS = {
  "compile-undefined": { name: "The name that Go cannot find.", have: "The name Go found, with other upper and lower case." },
  "compile-undefined-package": { name: "The package that the file does not import." },
  "compile-declared-and-not-used": { name: "The variable that is never used." },
  "compile-shadowed-err": { name: "The variable that is never used." },
  "compile-imported-and-not-used": { path: "The package you import and do not use." },
  "compile-missing-method": { expr: "The value you use.", what: "Its kind and its type.", iface: "The interface it must satisfy.", method: "The method the type does not have." },
  "test-got-want": { call: "The call the test made.", got: "Got: what your code returned.", want: "Want: what the test expects." },
  "vet-printf-wrong-type": { verb: "The verb.", type: "The type of the argument. The verb does not fit it." },
  "vet-printf-too-few-args": { verb: "The verb that has no argument.", n: "The argument that verb reads.", have: "The arguments the call has." },
  "vet-printf-too-many-args": { want: "The arguments the format reads.", have: "The arguments the call has." },
};
const FALLBACK = "The value Go put in the message.";
// The parts that get the red wash: what the learner must look at.
const ENTRY_KEYS = {
  "vet-printf-wrong-type": ["verb", "type"],
  "vet-printf-too-few-args": ["verb", "n", "have"],
  "vet-printf-too-many-args": ["want", "have"],
  "vet-printf-println-directive": ["verb"],
  "compile-missing-method": ["method"],
};
const KEY_ORDER = ["got", "name", "value", "expr", "call", "type"];
// The parts that 1 entry shows, in this order; the rest stays plain text.
const ENTRY_PARTS = {
  "compile-missing-method": ["expr", "what", "iface", "method"],
  "compile-wrong-method-signature": ["expr", "iface", "method"],
  "compile-pointer-receiver": ["expr", "iface", "method"],
  "compile-cannot-use": ["expr", "what", "want", "context"],
};
// The anatomy letters at most 5 parts: more turns the message into a staircase.
const MAX_PARTS = 5;

const GUIDES = {
  compile: ["guide-error-message", "How to read a Go error message"],
  vet: ["guide-error-message", "How to read a Go error message"],
  staticcheck: ["guide-error-message", "How to read a Go error message"],
  gofmt: ["guide-error-message", "How to read a Go error message"],
  build: ["guide-error-message", "How to read a Go error message"],
  runtime: ["guide-stack-trace", "How to read a stack trace"],
  race: ["guide-race-report", "How to read a race report"],
  test: ["guide-failing-test", "How to read a failing test"],
};

/** Where the problem comes from, in 1 sentence, and whether the code changed since. */
export function sourceLine(p) {
  const line = sourceOnly(p);
  return p.stale ? `${line} You changed ${p.file} after this run, so the line can differ now.` : line;
}

function sourceOnly(p) {
  const at = clock(p.at);
  if (p.explain?.generic) return "No explanation matches this message yet. This guide shows how to read it.";
  if (p.kind === "check") {
    const tool = { compile: "The compiler", vet: "go vet", staticcheck: "staticcheck", gofmt: "gofmt" }[p.diag?.source] || "A check";
    return `${tool} found this while you typed, at ${at}.`;
  }
  const src = p.explain?.raw?.source || p.explain?.source;
  const id = p.explain?.id || "";
  const tests = distinctTests(p);
  if (id === "test-timeout") return `The test run timed out in run ${p.runNo}, at ${at}.`;
  if (src === "race") return `The race detector found this in run ${p.runNo}, at ${at}.`;
  if (src === "runtime" || id.startsWith("test-panicked")) return `Your code panicked in run ${p.runNo}, at ${at}.`;
  if (src === "build" || src === "compile" || src === "vet") return `The build failed in run ${p.runNo}, at ${at}.`;
  if (tests.length > 1) return `${tests.length} tests failed here in run ${p.runNo}, at ${at}.`;
  if (p.test) return `Test ${shortTest(p.test)} failed in run ${p.runNo}, at ${at}.`;
  return `The run failed in run ${p.runNo}, at ${at}.`;
}

/** The last part of a test name, with the underscores go test adds made spaces again. */
export function shortTest(name) {
  const parts = String(name).split("/");
  return parts[parts.length - 1].replace(/_/g, " ");
}

function topTest(name) {
  return String(name || "").split("/")[0];
}

function distinctTests(p) {
  return [...new Set((p.occ || []).map((o) => o.test).filter(Boolean))];
}

/**
 * The chip: the source and the place, as nodes. "Compiler, line 120",
 * "Panic, line 54", "Panic in TestPolicyBackoff", "Failing test in TestDo".
 */
export function chipLabel(p) {
  const line = p.line && p.file ? `, line ${p.line}` : "";
  if (p.kind === "check") {
    const tool = { compile: "Compiler", vet: "go vet", staticcheck: "staticcheck", gofmt: "gofmt" }[p.diag?.source] || "Check";
    return [`${tool}${line}`];
  }
  if (p.stub && p.stubs?.all.length) return [p.stubs.all.length === 1 ? "1 stub left" : `${p.stubs.all.length} stubs left`];
  const src = p.explain?.raw?.source || p.explain?.source;
  const id = p.explain?.id || "";
  const where = (word) => (line ? [`${word}${line}`] : p.test ? [`${word} in `, h("code", {}, topTest(p.test))] : [word]);
  if (id === "test-timeout") return where("Timeout");
  if (src === "race") return where("Data race");
  if (src === "runtime" || id.startsWith("test-panicked")) return where("Panic");
  if (src === "build" || src === "compile" || src === "vet") return [`Build${line}`];
  return p.test ? ["Failing test in ", h("code", {}, topTest(p.test))] : ["Failing test"];
}

/** The chip as plain text, for titles and announcements. */
export function chipText(p) {
  return chipLabel(p).map((n) => (typeof n === "string" ? n : n.textContent)).join("");
}

// Captures of the position: they are parts of their own, not of the message.
const POSITION = new Set(["file", "line", "column", "code", "test"]);

/** Splits text at the capture values, so the values can be marked. */
function splitCaptures(text, captures) {
  const values = Object.entries(captures || {})
    .filter(([k, v]) => !POSITION.has(k) && !k.startsWith("prev_") && v && v.length > 0 && v.length < 200)
    .sort((a, b) => b[1].length - a[1].length);
  const out = [];
  let rest = text;
  while (rest) {
    let best = null;
    for (const [k, v] of values) {
      const i = rest.indexOf(v);
      if (i >= 0 && (best === null || i < best.i)) best = { i, k, v };
    }
    if (!best) {
      out.push({ text: rest });
      break;
    }
    if (best.i > 0) out.push({ text: rest.slice(0, best.i) });
    out.push({ text: best.v, group: best.k });
    rest = rest.slice(best.i + best.v.length);
  }
  return out;
}

// Code in a plain-text title: a call such as os.Exit(0), an index such as
// x[i], a qualified name such as fmt.Errorf, a verb such as %d.
const CODE_WORD = /[A-Za-z_][\w.]*\([^()\s]*\)|[A-Za-z_]\w*\[[\w:.]*\]|\b[a-z]\w*\.[A-Z]\w*\b|%[-+# 0-9.]*[a-zA-Z]\b|\b[a-z]+\.[a-z]+\b(?=\s)|\*\w+\b|<nil>/g;

/** Text with its code-like words in <code>. */
export function codeNodes(text) {
  const out = [];
  let last = 0;
  for (const m of String(text).matchAll(CODE_WORD)) {
    if (m.index > last) out.push(text.slice(last, m.index));
    out.push(h("code", {}, m[0]));
    last = m.index + m[0].length;
  }
  if (last < text.length) out.push(text.slice(last));
  return out;
}

/** The title as nodes: every captured value in mono, and every code-like word too. */
export function titleNodes(title, captures) {
  return splitCaptures(String(title || ""), captures).flatMap((s) => (s.group ? [h("code", {}, s.text)] : codeNodes(s.text)));
}

const LETTERS = "abcdefghijklmnopqrstuvwxyz";

/**
 * The parts of a 1-line message: [{text, letter?, meaning?, key?, group?, repeat?}].
 * The place (file:line:column) is 1 part; then at most 4 captures, the key
 * part first. A value that appears twice keeps its letter.
 */
export function messageParts(e) {
  const raw = e.raw || {};
  const captures = raw.captures || {};
  const labels = { ...LABELS, ...(ENTRY_LABELS[e.id] || {}) };
  const out = [];
  let used = 0;
  const assertion = raw.file && /_test\.go$/.test(raw.file) && raw.line && raw.source === "test";
  if (raw.file) {
    const pos = `${raw.file}${raw.line ? `:${raw.line}` : ""}${raw.column && !assertion ? `:${raw.column}` : ""}`;
    let meaning;
    if (assertion) meaning = "Where the test checks: the test file and the line. Open it to read the case.";
    else if (!raw.openable) meaning = "A file of the Go runtime or the testing package, and the line. It is not in the editor.";
    else if (raw.column) meaning = "The file, the line and the column. The column counts bytes from 1, and a tab counts as 1.";
    else meaning = raw.line ? "The file and the line. The file opens in the editor." : "The file. It opens in the editor.";
    out.push({ text: pos, letter: LETTERS[used++], meaning, group: "position" }, { text: ": " });
  }
  const code = raw.source === "staticcheck" && raw.code ? raw.code : "";
  const segs = splitCaptures(raw.message || "", captures);
  const present = [...new Set(segs.filter((s) => s.group).map((s) => s.group))];
  const keys = ENTRY_KEYS[e.id] || [KEY_ORDER.find((k) => captures[k] && present.includes(k)) || present[0]].filter(Boolean);
  const order = ENTRY_PARTS[e.id] ? ENTRY_PARTS[e.id].filter((g) => present.includes(g)) : [...keys.filter((k) => present.includes(k)), ...present.filter((g) => !keys.includes(g))];
  const room = MAX_PARTS - used - (code ? 1 : 0);
  const chosen = new Set(order.slice(0, Math.max(0, room)));
  const letterOf = new Map();
  for (const s of segs) {
    if (!s.group || !chosen.has(s.group)) {
      out.push({ text: s.text });
      continue;
    }
    const repeat = letterOf.has(s.group);
    if (!repeat) letterOf.set(s.group, LETTERS[used++]);
    out.push({ text: s.text, letter: letterOf.get(s.group), meaning: labels[s.group] || FALLBACK, key: keys.includes(s.group), group: s.group, repeat });
  }
  if (code) {
    // staticcheck prints its check at the end; the raw message keeps it.
    out.push({ text: " (" }, { text: code, letter: LETTERS[used++], meaning: "The staticcheck check. staticcheck.dev lists each one.", group: "check" }, { text: ")" });
  }
  return out;
}

const REF = /(?:[\w.\-]+\/)*([\w.\-]+\.go):(\d+)(?::(\d+))?/g;

/** The line of a test file that names a case: the first line with "name" in quotes. */
function caseLine(files, file, name) {
  const model = files.model?.(file);
  if (!model || !name) return 0;
  const want = JSON.stringify(name);
  for (let l = 1; l <= model.getLineCount(); l++) if (model.getLineContent(l).includes(want)) return l;
  return 0;
}

/**
 * Renders the anatomy box of a problem.
 * @param {object} e the explanation
 * @param {{has: (f: string) => boolean, model: Function, reveal: (f: string, l: number, c: number) => void}} files
 */
export function anatomy(e, files) {
  const raw = e.raw || {};
  const message = raw.message || "";
  if (message.includes("\n")) {
    // A panic, a race report or a timeout: no balloons, the lines as they are.
    const lines = message.split("\n");
    const head = raw.file ? `${raw.file}${raw.line ? `:${raw.line}` : ""}: ` : "";
    const block = h("pre", { class: "raw-block" });
    const fill = (all) => {
      block.replaceChildren();
      const seen = new Set();
      (all ? lines : lines.slice(0, 16)).forEach((line, i) => {
        const text = (i === 0 ? head : "") + line;
        const refs = [...text.matchAll(REF)].filter((m) => files.has(m[1]));
        if (refs.length === 0) {
          block.append(text, "\n");
          return;
        }
        const span = h("span", { class: "ref-line" });
        let last = 0;
        for (const m of refs) {
          span.append(text.slice(last, m.index));
          // A repeated place is 1 Tab stop, not 9.
          const again = seen.has(m[0]);
          seen.add(m[0]);
          span.append(h("a", { href: "#", tabindex: again ? "-1" : null, onclick: (ev) => { ev.preventDefault(); files.reveal(m[1], Number(m[2]), Number(m[3] || 1)); } }, m[0]));
          last = m.index + m[0].length;
        }
        span.append(text.slice(last));
        block.append(span, "\n");
      });
    };
    fill(false);
    const box = h("div", { class: "anatomy anatomy-plain" }, block);
    if (lines.length > 16) {
      const more = h("button", { type: "button", class: "btn btn-sm raw-more", onclick: () => { fill(true); more.remove(); block.focus(); } }, `Show all ${lines.length} lines`);
      block.tabIndex = -1;
      box.append(more);
    }
    return box;
  }
  const parts = messageParts(e);
  const rawEl = h("div", { class: "raw" });
  const list = h("dl", { class: "parts" });
  for (const p of parts) {
    if (!p.letter) {
      rawEl.append(p.text);
      continue;
    }
    rawEl.append(h("span", { class: `part${p.key ? " is-key" : ""}${p.text.length <= 2 ? " is-short" : ""}`, "data-part": p.letter }, p.text));
    if (p.repeat) continue;
    const dd = h("dd", {}, p.meaning);
    if (p.group === "case" && raw.openable) {
      const line = caseLine(files, raw.file, p.text);
      if (line) dd.append(" ", h("a", { href: "#", onclick: (ev) => { ev.preventDefault(); files.reveal(raw.file, line, 1); } }, `Line ${line} in ${raw.file}`), ".");
    }
    const row = h("div", { class: p.text.length > 14 ? "is-long" : null },
      h("dt", {}, balloon(p.letter, { label: `Part ${p.letter}` }), h("code", { title: p.text }, p.text)), dd);
    list.append(row);
  }
  return h("div", { class: "anatomy" }, rawEl, list);
}

/** The got/want line under a failing test: balloons only, 1 word each. */
export function gotWant(e) {
  const raw = e.raw || {};
  const c = raw.captures || {};
  if (!c.got && !c.want) return null;
  const line = h("p", { class: "gotwant" });
  const prefix = raw.file && raw.line ? `${raw.file}:${raw.line}: ` : "";
  line.append(prefix);
  for (const s of splitCaptures(raw.message || "", { got: c.got, want: c.want })) {
    if (s.group === "got") line.append(h("span", { class: "part is-key", "data-word": "got" }, s.text));
    else if (s.group === "want") line.append(h("span", { class: "part", "data-word": "want" }, s.text));
    else line.append(s.text);
  }
  return line;
}

const DEFAULT_OPEN = new Set(["What it means", "How to find it"]);

/** A disclosure that remembers whether it is open. */
function disclosure(title, body, { key = `gpw-xsec:${title}`, open = DEFAULT_OPEN.has(title), cls = "" } = {}) {
  const stored = load(key);
  const d = h("details", { class: `xsec ${cls}`.trim(), open: stored === null ? open : stored === "1" }, h("summary", {}, icon("chevron"), title), body);
  d.addEventListener("toggle", () => save(key, d.open ? "1" : "0"));
  return d;
}

/** The catalog body split at each heading, as disclosures that remember their state. */
function sections(html, e) {
  const tmp = document.createElement("div");
  tmp.innerHTML = html; // safe HTML from the server's catalog, see the top of this file
  const out = [];
  let body = null;
  for (const node of [...tmp.childNodes]) {
    // The catalog renders its "## " sections as <h3>.
    if (node.nodeName === "H2" || node.nodeName === "H3") {
      const title = node.textContent.trim();
      body = h("div", { class: "xsec-body" });
      out.push({ details: disclosure(title, body), body, title });
      continue;
    }
    if (body) body.append(node);
  }
  for (const s of out) {
    if (s.title !== "Learn more") continue;
    const code = e?.raw?.source === "staticcheck" && /^[A-Z]+\d+$/.test(e.raw.code || "") ? e.raw.code : "";
    if (code) {
      // The check's own page: the raw message shows its code.
      const ul = s.body.querySelector("ul") || s.body.appendChild(h("ul", { class: "explain-links" }));
      ul.append(h("li", {}, h("a", { href: `https://staticcheck.dev/docs/checks/#${code}`, target: "_blank", rel: "noopener noreferrer" },
        `staticcheck: ${code}`, h("span", { class: "visually-hidden" }, " (opens in a new tab)"))));
    }
    const n = s.body.querySelectorAll("a").length;
    if (n) s.details.querySelector("summary").append(" ", h("span", { class: "count" }, n === 1 ? "1 link" : `${n} links`));
  }
  return out.map((s) => s.details);
}

/** The test code around a failing assertion, from the read-only test file. */
function testSource(e, files) {
  const raw = e.raw || {};
  if (!raw.openable || !/_test\.go$/.test(raw.file || "") || !raw.line) return null;
  const model = files.model(raw.file);
  if (!model) return null;
  const from = Math.max(1, raw.line - 4);
  const to = Math.min(model.getLineCount(), raw.line + 3);
  const block = h("div", { class: "source-block", tabindex: "0", role: "region", "aria-label": `The test code at ${raw.file}:${raw.line}` });
  for (let l = from; l <= to; l++) {
    block.append(h("div", { class: `src-line${l === raw.line ? " is-hit" : ""}` }, h("span", { class: "src-no" }, String(l)), h("span", {}, model.getLineContent(l))));
  }
  return disclosure(`The test code at ${raw.file}:${raw.line}`, h("div", { class: "xsec-body" }, block), { key: "gpw-xsec:The test code", open: true, cls: "problem-test-source" });
}

/** A link that moves the editor to a line, with the text in mono. */
function lineLink(files, file, line, text) {
  return h("a", { href: "#", class: "code-link", onclick: (ev) => { ev.preventDefault(); files.reveal(file, line, 1); } }, h("code", {}, text));
}

/** "a, b and c" from nodes. */
function joinNodes(nodes) {
  const out = [];
  nodes.forEach((n, i) => {
    if (i > 0) out.push(i === nodes.length - 1 ? " and " : ", ");
    out.push(n);
  });
  return out;
}

/** The tests a problem was seen in, folded per top-level test. */
function seenIn(p, tests) {
  const names = distinctTests(p);
  if (names.length < 2) return null;
  const tops = new Map();
  for (const n of names) {
    const top = topTest(n);
    if (!tops.has(top)) tops.set(top, { top, subs: 0, seeds: 0, self: false });
    const t = tops.get(top);
    if (n === top) t.self = true;
    else if (/\/seed#\d+$/.test(n)) t.seeds++;
    else t.subs++;
  }
  const list = h("ul", { class: "seen-list" });
  for (const t of tops.values()) {
    const extra = [t.subs ? (t.subs === 1 ? "1 subtest" : `${t.subs} subtests`) : "", t.seeds ? (t.seeds === 1 ? "1 seed" : `${t.seeds} seeds`) : ""].filter(Boolean).join(", ");
    const name = tests ? h("button", { type: "button", class: "link-button code-link", onclick: () => tests.revealTest(names.find((n) => topTest(n) === t.top)) }, h("code", {}, t.top)) : h("code", {}, t.top);
    list.append(h("li", {}, name, extra ? ` (${extra})` : ""));
  }
  const title = tops.size === 1 ? `Seen in ${names.length} cases of 1 test` : `Seen in ${tops.size} tests`;
  return disclosure(title, h("div", { class: "xsec-body" }, list), { key: "gpw-xsec:Seen in", open: false, cls: "seen" });
}

/** A note on a fuzz input, when the failing test is a seed or a corpus file. */
function fuzzNote(p) {
  const name = p.test || "";
  const seed = name.match(/^(\w+)\/seed#(\d+)$/);
  if (seed) return h("p", { class: "problem-fact" }, h("code", {}, `seed#${seed[2]}`), ` is the input of the f.Add call number ${seed[2]} in `, h("code", {}, seed[1]), ", counted from 0.");
  const corpus = name.match(/^(Fuzz\w+)\/([0-9a-f]{8,})$/);
  if (corpus) return h("p", { class: "problem-fact" }, "The input is the file ", h("code", {}, `testdata/fuzz/${corpus[1]}/${corpus[2]}`), ". It holds an input that failed before.");
  return null;
}

/** For "undefined: x": the declaration that differs only in case, if the code has one. */
function caseTwin(e, files) {
  if (e.id !== "compile-undefined") return null;
  const name = e.raw?.captures?.name || "";
  if (!/^\w+$/.test(name) || !files.findDeclaration) return null;
  const twin = files.findDeclaration(name);
  if (!twin) return null;
  return h("p", { class: "problem-fact" }, `${twin.file} declares `, lineLink(files, twin.file, twin.line, twin.name), ` at line ${twin.line}. Go treats `, h("code", {}, name), " and ", h("code", {}, twin.name), " as 2 different names.");
}

/** The body of the stub problem: which stubs are left, and where to start. */
function stubBody(p, { files, runStep }) {
  const { all, start } = p.stubs;
  const out = [];
  if (start?.step || start?.functions.length) {
    const lead = h("p", { class: "problem-lead" });
    const names = joinNodes(start.functions.map((fn) => lineLink(files, fn.file, fn.panicLine, fn.name)));
    const step = start.step ? `${start.step.kind || "step"} ${start.step.n}` : "";
    if (step && names.length) lead.append(`Start with ${step}: `, ...names, ".");
    else if (step && !start.ran) lead.append(`Start with ${step}. Its tests did not run: a panic ended the run first. Run ${step} on its own to see the stubs it needs.`);
    else if (step) lead.append(`Start with ${step}.`);
    else lead.append("Start with ", ...names, ".");
    out.push(lead);
    if (start.step && runStep) out.push(h("p", { class: "problem-actions" }, h("button", { type: "button", class: "btn btn-sm", onclick: () => runStep(start.step.id) }, `Run ${step}`)));
  }
  const byFile = new Map();
  for (const fn of all) {
    if (!byFile.has(fn.file)) byFile.set(fn.file, []);
    byFile.get(fn.file).push(fn);
  }
  const list = h("dl", { class: "stub-list" });
  for (const [file, fns] of byFile) {
    list.append(h("dt", {}, h("code", {}, file)), h("dd", {}, ...joinNodes(fns.map((fn) => h("span", { class: "stub-fn" }, lineLink(files, fn.file, fn.panicLine, fn.name), ` line ${fn.panicLine}`)))));
  }
  out.push(disclosure(all.length === 1 ? "The stub left in your files" : `The ${all.length} stubs left in your files`, h("div", { class: "xsec-body" }, list), { key: "gpw-xsec:The stubs", open: false, cls: "stubs" }));
  return out;
}

/**
 * Renders 1 problem.
 * @param {object} p the problem: {n, kind, explain, diag?, occ?, stubs?, test?, runNo?, at, file, line, column}
 * @param {object} o
 * @param {any} o.files
 * @param {Record<string, string>} o.titles exercise slug -> "01 Parse logfmt log lines"
 * @param {any} [o.tests] the test runner, to show a test in the tree
 * @param {(step: string) => void} [o.runStep]
 */
export function problemView(p, { files, titles, tests, runStep }) {
  const e = p.explain || { title: p.diag?.message || "Problem", html: "", raw: { message: p.diag?.message || "" } };
  const raw = e.raw || {};
  const stub = p.stub && p.stubs?.all.length;
  const article = h("article", { class: "problem", "aria-labelledby": `problem-${p.n}-title`, "data-problem": p.n });
  article.append(balloon(String(p.n), { label: `Problem ${p.n}`, cls: "is-filled" }));
  article.append(h("p", { class: "problem-source" }, sourceLine(p)));
  const title = stub ? [p.stubs.all.length === 1 ? "1 function is still the course stub" : `${p.stubs.all.length} functions are still the course stub`] : titleNodes(e.title, raw.captures);
  article.append(h("h3", { class: "problem-title", id: `problem-${p.n}-title` }, title));
  if (stub) {
    article.append(...stubBody(p, { files, runStep }));
  } else if (p.file && p.line && files.has(p.file)) {
    article.append(h("p", { class: "problem-where" }, h("a", { href: "#", "data-goto": "", onclick: (ev) => { ev.preventDefault(); files.reveal(p.file, p.line, p.column || 1); } }, `Line ${p.line} in ${p.file}`)));
  } else if (raw.file && raw.line) {
    article.append(h("p", { class: "problem-where" }, `Line ${raw.line} in `, h("span", { class: "mono" }, raw.file), raw.openable ? "" : ", a file of Go itself"));
  }
  const twin = caseTwin(e, files);
  if (twin) article.append(twin);
  const fuzz = fuzzNote(p);
  if (fuzz) article.append(fuzz);
  const seen = seenIn(p, tests);
  if (seen) article.append(seen);
  if (raw.message) article.append(anatomy(e, files));
  const src = testSource(e, files);
  if (src) article.append(src);
  article.append(...sections(e.html || "", e));
  article.append(courseLine(e, titles));
  return article;
}

function courseLine(e, titles) {
  const p = h("p", { class: "problem-course" });
  if (e.course) {
    const t = titles[e.course] || e.course;
    p.append("Taught in ", h("a", { href: `/learn/${e.course}` }, t), ".");
    return p;
  }
  const src = e.raw?.source || e.source;
  const [id, name] = GUIDES[src] || GUIDES.compile;
  if (e.generic) {
    p.append("The guide has all 4 ways to read Go messages: ", h("a", { href: `/guide#${id}` }, name), ".");
    return p;
  }
  p.append("No exercise is about this on its own. The guide shows how to read it: ", h("a", { href: `/guide#${id}` }, name), ".");
  return p;
}
