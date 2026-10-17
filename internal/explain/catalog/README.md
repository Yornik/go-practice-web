# The explain catalog

This catalog turns what the Go tools print into explanations that teach. A learner who sees `undefined: backof` or a race report also gets a short text: what it means, why Go does this, how to find the cause, a small example and where to read more. The course forbids LLM help, so every text here is written and reviewed by hand, and every entry is tested against the real output of the Go release the runner uses (Go 1.27.2).

The reader is a medior engineer: someone who can program, knows basic Go and is rusty. An entry teaches the concept and how to find the problem. It never solves the exercise.

## Files

```text
catalog/
  README.md                         this file (not an entry)
  <source>/<id>.md                  1 entry per file
testdata/fixtures/<id>/             1 or more fixtures per entry (see "Fixtures")
```

- `<source>` is one of `compile`, `vet`, `staticcheck`, `runtime`, `race`, `test`, `build`, `guide`.
- `<id>` is the file name without `.md`. It is lower-case words joined by `-` and starts with the source: `compile-undefined`, `vet-printf-wrong-type`, `staticcheck-sa4006`.
- The loader checks all of this when the package starts. A broken entry is left out and `go test ./internal/explain/` fails with the reason.

## An entry

```markdown
---
id: compile-declared-and-not-used
source: compile
match: ^declared and not used: (?P<name>\S+)$
title: The variable {{name}} is declared but its value is never read
links:
- The Go spec: Variable declarations | https://go.dev/ref/spec#Variable_declarations
- Effective Go: Unused imports and variables | https://go.dev/doc/effective_go#blank_unused
---
## What it means

You declared the local variable `{{name}}`, but no code reads its value. ...

## Why Go does this

...

## How to find it

...

## Example

...

## Learn more
```

### Front matter

The block between the 2 `---` lines holds `key: value` lines. Values are literal: no quotes, no escapes, no comments. A regular expression or a URL with `#` needs nothing special.

| Key | Required | Meaning |
|---|---|---|
| `id` | yes | The file name without `.md`. |
| `source` | yes | The directory name. Decides which messages the entry sees (see "Sources"). |
| `match` | for compile, build, runtime, race, test | A Go regular expression ([syntax](https://pkg.go.dev/regexp/syntax)) with named groups `(?P<name>...)`. It may appear more than once; then the first line that matches wins, and every line must have the same named groups. |
| `analyzer` | vet: this, `match` or both | The `go vet` analyzer, for example `printf`. The loader knows the analyzers of Go 1.27 (`go tool vet help`). |
| `code` | staticcheck: this, `match` or both | A check such as `SA4006`, or a category such as `SA4` that matches every `SA4xxx` without its own entry. |
| `title` | yes | 1 line, at most 100 characters, no period at the end. It can use placeholders. |
| `course` | no | The slug of the exercise that teaches the concept, for example `12-workerpool`. Only when the exercise really practises it; read its README first. |
| `fallback` | no | `true` for a broad pattern that should lose to every other entry that matches the same message, such as any got/want assertion or any race report. Only for compile, build, runtime, race and test entries. |
| `links` | yes, at least 1 | 1 per line: `- label | https://...`. Only `https` links to `go.dev`, `pkg.go.dev` and `staticcheck.dev`. |

### Sources

Each source gets a different kind of text. `match` runs against that text, and the engine fills in some values itself (the built-in placeholders).

| Source | Where it comes from | Text that `match` runs against | Built-in placeholders |
|---|---|---|---|
| `compile` | Compiler and type errors from a check, gofmt syntax errors, a failed test build | The message without its position: `undefined: backof`. Indented continuation lines such as `have (int)` and `want (string)` follow after `\n\t` in a test run, and after a plain `\n` in the editor's check, so write `\n\s*`. The editor's check reports syntax errors with go/parser and type errors with go vet, which word some messages differently from the compiler: give such an entry 1 `match` per wording. | `file`, `line`, `column`, `code` (empty) |
| `build` | go command errors without a position: go.mod, missing packages | The line, for example `go: updates to go.mod needed; to update it:` and its continuation lines | as compile; usually empty, so use groups |
| `vet` | `go vet` findings | The message: `fmt.Printf format %d has arg s of wrong type string` | `file`, `line`, `column`, `code` (the analyzer) |
| `staticcheck` | staticcheck findings | The message without the code: `this value of err is never used` | `file`, `line`, `column`, `code` (the check) |
| `runtime` | A panic or a fatal error in test output | The first line, without `[recovered, repanicked]`: `panic: runtime error: index out of range [5] with length 2`, `fatal error: all goroutines are asleep - deadlock!`. A panic value with several lines keeps them. | `func`, `file`, `line` (the first frame in the exercise package), `test` |
| `race` | A race detector report | The whole report, from `WARNING: DATA RACE` to the closing `=====` line, with the full stacks. Use `(?s)` to match a frame, such as `runtime\.mapassign` for a map. | `access` and `prev_access` (`read` or `write`), `func`, `file`, `line` and `prev_func`, `prev_file`, `prev_line` (the first frame in the exercise package of each access), `test` |
| `test` | A failing test | 1 of 3 texts: an assertion, the text after `name_test.go:NN: ` plus its continuation lines, joined by `\n` and trimmed (a helper that recovers a panic reports the stub line instead: `slo.go:76: panicked: ...`); an Example mismatch, `got:\n...\nwant:\n...`; a timeout, `panic: test timed out after 45s`. | `file`, `line` (of the assertion, or for a timeout the first frame in the exercise package; empty for Examples), `test` |
| `guide` | Never matches. The fallback when no entry matches. | none | none |

Compile and build entries search each other: an import problem can come from either.

The 4 guides are `guide-error-message` (compile, build, vet, staticcheck), `guide-stack-trace` (panics, fatal errors, timeouts), `guide-race-report` and `guide-failing-test` (assertions, Examples, a failure without output). Their bodies have free sections that end with `## Learn more`.

### Matching

For 1 message the engine looks at the entries of its source and takes the most specific match:

1. `analyzer` or `code` equal to the message's code, and `match` matches.
2. `analyzer` or `code` equal, without `match`: a catch-all for that check.
3. `match` matches. This includes an entry with `analyzer` when the code is unknown: go test runs some vet checks before the tests and prints their findings without the analyzer name, so only `match` can recognise them.
4. `match` of an entry with `fallback: true` matches.
5. A category `code` such as `SA4`, with a `match` that matches.
6. A category `code` without `match`.

When 2 entries match at the same level, the first file in name order wins. Do not rely on that: keep your patterns disjoint, and mark a pattern that is broad on purpose with `fallback: true`. Anchor them with `^` and `$`.

### Placeholders

`{{name}}` in the title or the body is replaced by a named group of `match` or a built-in placeholder of the source.

- In the body the value is HTML-escaped. In the title it is plain text, shortened to 60 characters.
- A placeholder cannot go in a code block (syntax highlighting splits it, so it is never filled) or in a link target. The loader rejects both.
- An optional group (`(?:...)?`) can be empty. Use its placeholder only where an empty value still reads well, or not at all.
- Guides have no placeholders.
- Go code such as `[]T{{1, 2}}` is fine; only `{{` with a name and `}}` is a placeholder.

### Body

The body has exactly these `##` sections, in this order. Use no other heading levels, no raw HTML and no images.

| Section | What to write |
|---|---|
| What it means | The message in plain language, with the captured names: "You declared `{{name}}`, but ...". What the learner sees happen. |
| Why Go does this | The language, runtime or tool rule behind it, and why that rule is useful. Name the spec section or the doc. |
| How to find it | Where to look (`{{file}}:{{line}}`, which frame, which block of a report), which part of the message points where, the questions to ask, what to print. |
| Example | A small generic snippet with the mistake, then the fix. Tag code blocks with `go`, or `text` for output. |
| Learn more | Can stay empty. The engine adds the links from the front matter and the course exercise below it. 1 sentence when you want to say why the exercise helps. |

## Writing rules

- **Never solve an exercise.** Use generic names in examples (`Sum`, `hits`, `port`), not the API of an exercise. Never copy from `course/solutions/` and do not decode it to write an entry. When an example would solve a step of the course, choose another example.
- **Teach how to find it.** The learner should be able to find the next problem of the same kind without the explanation.
- **Length.** 120 to 250 words of prose per entry; code blocks and headings do not count. Guides 150 to 400. The test enforces it.
- **Style.** The course style: short sentences, the active voice, numbers as digits, no contractions. Speak to the learner as "you". No "simply", "just" or "obviously".
- **Verify every Go fact.** Write the fixture first and read the real output of Go 1.27.2. Check every claim about the language in the spec or the package docs, not from memory.
- **Links.** Only pages you opened, and anchors you checked. Prefer the spec, Effective Go, the Go blog, the docs on pkg.go.dev and staticcheck.dev. Run the link test.

## Fixtures

Every entry needs at least 1 fixture: a tiny Go module that makes the tool print the message. The test runs it with the runner's Go release and checks that the engine picks the entry with the right values. When a new Go release changes a message, the fixture fails before a learner sees a wrong explanation.

```text
testdata/fixtures/<id>[-<variant>]/
  go.mod        module example.com/fixture, go 1.24
  x.go          the code with the mistake
  x_test.go     for test commands
  want          what to expect
```

The `want` file:

```text
id: race-data-race
command: test -race
file: x.go
line: 13
test: TestHits
capture: func=Hits.func1
capture: prev_line=13
```

| Key | Meaning |
|---|---|
| `id` | The entry that must explain the output. A guide id is allowed, to test a fallback. |
| `command` | `build` (go build), `vet` (go vet -json, read as the runner reads it: type errors as text lines, findings as JSON), `gofmt` (the runner's syntax check: go/parser of the runner's release, see `testdata/parsecheck`), `staticcheck` (staticcheck -f json), or `test` followed by go test flags such as `-race` (go test -json -count=1). |
| `file`, `line`, `test` | Optional: the expected file, line and test of the explanation. |
| `capture` | Optional, repeatable: `name=value` for a placeholder value. |

- Keep fixtures small, deterministic and formatted with gofmt. The exceptions are a syntax error, and a mistake that gofmt would repair, such as a misplaced `//go:build` line.
- When the editor words a message differently from the compiler, add a fixture for each: `<id>-gofmt` for a syntax error, `<id>-vet` for a type error.
- A race fixture: assert only values that are the same in every run. Which access is caught first changes.
- A deadlock in a test only shows as `fatal error: all goroutines are asleep` with `-timeout=0`: the timer of the default timeout keeps the runtime from seeing it. Use `command: test -timeout=0`.
- In go test -json output a panic belongs to the top-level test, not to the subtest. Write `test: TestSum`, not `TestSum/empty`.

## Run the tests

```bash
go test -short ./internal/explain/                    # loader, style rules, matching; no toolchain
go test ./internal/explain/                           # also every fixture with Go 1.27.2 (a few seconds)
EXPLAIN_SHOW=compile-undefined go test -run TestShow -v ./internal/explain/   # print an entry as HTML
EXPLAIN_CHECK_LINKS=1 go test -run TestLinks ./internal/explain/            # fetch every link and anchor
```

The staticcheck fixtures need a staticcheck built with Go 1.27; an older build cannot load the Go 1.27 standard library, and the fixtures are skipped with the reason. Build one and point `STATICCHECK` at it:

```bash
GOTOOLCHAIN=go1.27.2 GOBIN=/tmp/sc go install honnef.co/go/tools/cmd/staticcheck@v0.8.2   # release 2026.2.2
STATICCHECK=/tmp/sc/staticcheck go test ./internal/explain/
```

`EXPLAIN_GOTOOLCHAIN` runs the fixtures with another Go release, for example before the runner image moves to it.

## Before you finish

- `go test ./internal/explain/` passes, with `STATICCHECK` set when you wrote staticcheck entries.
- The link test passes.
- You read your entry once as HTML with `EXPLAIN_SHOW`.
- Every fact is checked, and no example solves an exercise.
