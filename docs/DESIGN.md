# go-practice-web design

go-practice-web is a self-hosted web IDE for the [go-practice](https://github.com/Yornik/go-practice) course. A learner signs in with a passkey, reads an exercise, writes Go in a browser editor with highlighting and lint markers, and runs the real tests (with `-race`) on a sandboxed runner in the cluster. The site tracks progress, reveals hints 1 level at a time and issues signed PDF certificates with a public verify page.

This file is the contract between the components. If code and this file disagree, fix one of them in the same pull request.

## Decisions

| Topic | Decision |
|---|---|
| Public URL | `https://go-practice.dev` (apex). The passkey relying party ID is `go-practice.dev`. |
| Sign-up | Invite only. The first account is created with a one-time setup link that the web server logs at start when no user exists. That account is admin. Admins create single-use invite links. |
| Login | Passkeys only (WebAuthn, discoverable credentials). No passwords. |
| Recovery | A verified email address. A recovery link (single use, 15 min) lets the user register a new passkey. |
| Mail | SMTP with implicit TLS through Resend (`smtp.resend.com:465`, user `resend`, password = API key), sender `go-practice@yornik.eu`. The cluster already sends Resend mail from `yornik.eu`. |
| Database | SQLite (`modernc.org/sqlite`, pure Go) on 1 PVC, 1 replica. `journal_mode=DELETE` because the PVC is NFS. Daily online backup with `VACUUM INTO`, 7 rotating files. |
| Code execution | A separate `runner` Deployment in its own namespace runs `go vet`, `staticcheck`, `gofmt` and `go test -race` in a sandbox. The web server calls it over HTTP with a bearer token. |
| Certificates | 1 per tier (5) and 1 for the full course. PDF with a QR code to `https://go-practice.dev/verify/<id>`. Ed25519-signed. The owner chooses per certificate whether the verify page shows hint and solution counts. |
| Frontend | Server-rendered `html/template` pages, vanilla ES modules, Monaco editor vendored from npm. No frontend framework and no Node at run time. Designed with the frontend-design skill. |

## Repository layout

```text
cmd/web/                The web server (main package only: config, wiring).
cmd/runner/             The runner server (main package only).
internal/runnerapi/     Request and response types shared by web and runner. The wire contract.
internal/runner/        Runner side: workspace preparation, sandboxed exec, go test -json parsing.
internal/course/        Loads the course from course/: exercises, steps, files, rendered READMEs, hidden material.
internal/store/         SQLite access, migrations (embedded SQL), backups.
internal/auth/          Passkeys (go-webauthn), sessions, invites, setup token, email tokens, rate limits.
internal/mail/          SMTP sender and the email templates. A log-only sender when SMTP is not configured.
internal/cert/          Certificate eligibility, signing, PDF rendering (signintech/gopdf), QR (rsc.io/qr).
internal/web/           HTTP handlers, middleware (CSRF, CSP, sessions), template rendering, API.
web/templates/          html/template pages and partials.
web/static/             CSS, JS modules, fonts, images. Monaco is copied to web/static/vendor/monaco/ at build time.
course/                 Git submodule: github.com/Yornik/go-practice, pinned to a commit.
scripts/                fetch-monaco.sh, dev.sh, e2e helpers.
e2e/                    Playwright tests (Node, dev only).
docs/                   This file and operator docs.
Dockerfile.web, Dockerfile.runner
```

Go module: `github.com/yornik/go-practice-web`, `go 1.24`. The images build with the current Go release (`golang:1.27-bookworm`).

## Configuration

All configuration is environment variables. Unknown or invalid values stop the process at start with a clear error.

### Web (`GPW_`)

| Variable | Default | Meaning |
|---|---|---|
| `GPW_ADDR` | `:8080` | HTTP listen address for the site. |
| `GPW_METRICS_ADDR` | `:9090` | Prometheus `/metrics`. Not routed publicly. |
| `GPW_PUBLIC_URL` | (required) | `https://go-practice.dev`. Origin for WebAuthn, links in emails and certificates. |
| `GPW_DB_PATH` | `/data/go-practice.db` | SQLite file. Backups go to `<dir>/backups/`. |
| `GPW_COURSE_DIR` | `/course` | Checkout of go-practice. |
| `GPW_RUNNER_URL` | (required) | For example `http://runner.go-practice-runner.svc:8081`. |
| `GPW_RUNNER_TOKEN` | (required) | Bearer token for the runner. |
| `GPW_SMTP_HOST`, `GPW_SMTP_PORT`, `GPW_SMTP_USER`, `GPW_SMTP_PASSWORD` | empty | When the host or the password is empty or `REPLACE_ME`, mail is logged instead of sent. |
| `GPW_MAIL_FROM` | `Go Practice <go-practice@yornik.eu>` | Sender. |
| `GPW_TRUSTED_PROXIES` | `10.244.0.0/16` | CIDRs whose `X-Forwarded-For` is trusted for client IPs. |
| `GPW_DEV` | `false` | Local development: allows `http://localhost`, drops the `__Host-` cookie prefix and `Secure`. |

### Runner (`GPR_`)

| Variable | Default | Meaning |
|---|---|---|
| `GPR_ADDR` | `:8081` | Listen address. |
| `GPR_METRICS_ADDR` | `:9091` | Prometheus `/metrics`. |
| `GPR_TOKEN` | (required) | Expected bearer token. |
| `GPR_COURSE_DIR` | `/course` | Checkout of go-practice (tests, testdata, go.mod). |
| `GPR_WORK_DIR` | `/tmp/runs` | Per-run directories. |
| `GPR_GOCACHE` | `/cache/go-build` | Shared build cache. Seeded at start from `GPR_GOCACHE_SEED` if empty. |
| `GPR_GOCACHE_SEED` | `/opt/gocache-seed` | Pre-warmed cache baked into the image. |
| `GPR_MAX_CONCURRENT` | `2` | Concurrent `go` invocations. More requests queue up to `GPR_MAX_QUEUE`. |
| `GPR_MAX_QUEUE` | `8` | Queue length. When full: HTTP 429 with `Retry-After`. |
| `GPR_TEST_TIMEOUT` | `45s` | Wall clock for `go test`. Under the 60 s Traefik timeout. |
| `GPR_CHECK_TIMEOUT` | `20s` | Wall clock for vet, staticcheck and gofmt together. |

## Runner

### API

The wire types live in `internal/runnerapi`. Every request carries `Authorization: Bearer <GPR_TOKEN>`; a wrong token gets 401 with no body. All bodies are JSON.

`POST /v1/check` — gofmt syntax check, `go vet`, then `staticcheck` (if installed) on the learner's files with the exercise tests in place.

```json
{"exercise": "07-retry", "files": {"retry.go": "package retry\n...", "retryafter.go": "..."}}
```
Response 200:
```json
{"diagnostics": [{"file": "retry.go", "line": 12, "column": 5, "endLine": 0, "endColumn": 0,
                  "severity": "error", "source": "compile", "message": "undefined: foo"}],
 "durationMs": 1830}
```
`source` is one of `gofmt`, `compile`, `vet`, `staticcheck`. `severity` is `error`, `warning` or `info` (vet and staticcheck findings are warnings). Diagnostics for test files have the test file's name; the UI shows them only in the problems list.

`POST /v1/format` — `gofmt` one file.
```json
{"exercise": "07-retry", "file": "retry.go", "content": "..."}
```
Response: `{"content": "...formatted...", "diagnostics": [...]}`. On a syntax error, `content` is the input unchanged and `diagnostics` holds the error.

`POST /v1/test` — `go test -json -count=1 [-race] [-run <regexp>] ./exercises/<exercise>/`.
```json
{"exercise": "07-retry", "files": {...}, "run": "TestBackoff|TestDo", "race": true}
```
Response: `Content-Type: application/x-ndjson`, streamed. Each line is an event:
```json
{"type": "queued", "position": 1}
{"type": "start"}
{"type": "build", "output": "..."}                     // compiler output when the build fails
{"type": "test", "action": "run|pass|fail|skip|output", "test": "TestDo/ctx_cancelled", "elapsed": 0.01, "output": "..."}
{"type": "done", "exitCode": 0, "passed": 12, "failed": 0, "skipped": 0, "durationMs": 4100,
 "timedOut": false, "truncated": false, "buildFailed": false}
```
`test` events map 1 to 1 to `go test -json` events (Action, Test, Elapsed, Output). Package-level output (no `test`) is sent with an empty `test`. Output is capped at 1 MiB per run; after the cap the runner kills the run and sets `truncated`.

`GET /healthz` (process alive) and `GET /readyz` (course loaded, `go version` works, work dir writable).

### Sandbox rules

- File names in `files` must be exactly the exercise's stub file names (`*.go`, not `_test.go`). Anything else is a 400. This blocks path traversal and test replacement. Max 256 KiB per file, 1 MiB total.
- Each request gets a fresh directory `GPR_WORK_DIR/<random>/` with `go.mod` from the course, the exercise's `*_test.go` and `testdata/` copied from the course, and the learner's files. The directory is deleted when the request ends.
- `go` runs as a child process in its own process group, with a minimal environment: `HOME`, `PATH`, `GOCACHE`, `GOPATH` (per run), `GOFLAGS=-mod=mod`, `GOPROXY=off`, `GOTOOLCHAIN=local`, `GOTELEMETRY=off`, `CGO_ENABLED=1` (the race detector needs cgo), `HTTP_PROXY`/`HTTPS_PROXY`/`ALL_PROXY=http://127.0.0.1:9` (a dead end for code that honours proxies; not a security boundary).
- On timeout, cancellation or output over the cap the runner kills the whole process group (SIGKILL).
- The runner process holds no secret but its own token. The pod has no service account token, runs as UID 65532, has a read-only root filesystem, drops all capabilities and uses the RuntimeDefault seccomp profile. `/tmp` and `/cache` are `emptyDir` volumes with size limits.
- Network isolation is a NetworkPolicy (no egress at all, ingress only from the web namespace). It only works if the cluster's CNI enforces NetworkPolicies. Document this; do not depend on it for anything except defense in depth.

## Data model

SQLite, migrations in `internal/store/migrations/NNNN_name.sql`, applied in order at start inside a transaction, version in `schema_migrations`. Times are UTC RFC 3339 text. IDs are integers except where noted.

| Table | Columns (key ones) |
|---|---|
| `users` | id, username (unique, 3–32 chars `[a-z0-9_-]`), display_name, email (unique, nullable), email_verified_at, is_admin, disabled_at, webauthn_handle (32 random bytes, unique), created_at |
| `credentials` | id (credential ID bytes, PK), user_id, public_key, attestation_type, aaguid, sign_count, transports, backup_eligible, backup_state, name (user label), created_at, last_used_at |
| `sessions` | token_hash (SHA-256, PK), user_id, created_at, expires_at, last_seen_at, user_agent |
| `webauthn_ceremonies` | id (random, PK), kind (register, login, add, recover), user_id (nullable), data (JSON SessionData), expires_at |
| `invites` | token_hash (PK), created_by, email (nullable), note, created_at, expires_at, used_by, used_at |
| `setup_tokens` | token_hash (PK), created_at, used_at |
| `email_tokens` | token_hash (PK), user_id, purpose (verify, recover), email, created_at, expires_at, used_at |
| `code` | user_id, exercise, file, content, updated_at; PK (user_id, exercise, file) |
| `progress` | user_id, exercise, status (started, passed), started_at, passed_at, active_seconds, runs, last_run_at, last_result (JSON summary), notes; PK (user_id, exercise) |
| `reveals` | user_id, exercise, level (1–6), revealed_at; PK (user_id, exercise, level) |
| `certificates` | id (text, PK, `GP-XXXX-XXXX-XXXX` from a Crockford base32 alphabet), user_id, kind (tier1..tier5, course), name_on_cert, issued_at, show_stats, stats (JSON), payload (canonical JSON that was signed), signature, revoked_at |
| `settings` | key (PK), value — holds the certificate signing key (generated at first start) |
| `audit` | id, user_id (nullable), event, detail (JSON), ip, created_at |

## Authentication and accounts

Library: `github.com/go-webauthn/webauthn`. Relying party: ID = host of `GPW_PUBLIC_URL`, origins = `[GPW_PUBLIC_URL]`, display name `Go Practice`. Registration requires a resident key (discoverable credential) and user verification `preferred`. Attestation `none`.

Flows:

1. **Setup.** At start, if `users` is empty and no unused setup token exists, generate a 32-byte token, store its hash, and log `setup link: <PUBLIC_URL>/setup?token=<token>` once at WARN level. `/setup` works like an invite and makes the account admin.
2. **Sign-up with an invite.** `/join?invite=<token>`: the user picks a username, display name and email, then creates a passkey. On success: the account and credential are stored, the invite is used, the user is signed in and a verification email goes out. An invite is single use and expires (default 7 days).
3. **Login.** `/login`: "Sign in with a passkey" (discoverable flow, no username) plus conditional UI (passkey autofill) when the browser supports it.
4. **Sessions.** Cookie `__Host-gpw_session` (HttpOnly, Secure, SameSite=Lax, Path=/), 32 random bytes, stored as SHA-256, 30-day expiry, sliding (refreshed at most once per hour). Logout deletes the row.
5. **Account page.** List, rename and delete passkeys (not the last one), add a passkey, change display name, change email (sends a new verification), sign out everywhere.
6. **Recovery.** `/recover`: enter an email address. The response is always the same message. If a user with that verified email exists, send a link `/recover/confirm?token=<token>` (15 min, single use). That page lets the user create a new passkey and optionally remove all old ones. Afterwards an email reports that a passkey was added through recovery.
7. **Security notices.** Email the user when a passkey is added or removed, when the email changes (to the old and the new address), and when recovery is used.
8. **Admin.** `/admin`: create invite links (optionally emailed), list and revoke invites, list users with progress, disable or enable a user. Admins cannot remove their own admin flag.

Protection:

- CSRF: every state-changing request is `POST`/`PUT`/`DELETE` with a JSON body, must carry `X-GPW-CSRF: 1`, and its `Origin` must equal `GPW_PUBLIC_URL`. SameSite=Lax is the second layer.
- CSP (set by the app on every HTML response): `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; worker-src 'self' blob:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'`. Add only what Monaco proves it needs, with a comment.
- Rate limits (in memory, per client IP and per account): WebAuthn begin endpoints 20/min per IP; recovery 5/hour per IP and 3/hour per address; check 30/min per user; test runs 10/min per user; at most 1 test run in flight per user.
- Templates escape everything. Markdown from the course is trusted content (it is in our repository), rendered server-side by goldmark with raw HTML disabled.

## Course content

`internal/course` loads `GPW_COURSE_DIR` at start and keeps it in memory:

- Exercises in directory order. For each: number, slug, title (line 1 `# NN · Title`), tier and time (line 3 `**Tier N · about X h**`), the Go topics and the problem-solving text from line 3, and the README rendered to HTML with goldmark (GFM tables, heading IDs, chroma highlighting with CSS classes).
- The README is split into sections by `## ` heading so the UI can place them: Task, Rules, Examples, Run the tests, Docs you can use, Stretch goals. The "Stuck?" section is not shown; the hint ladder replaces it.
- Steps from the "Run the tests" section: every line `go test ... -run <regexp> ... # step N: label` (or `# part N: label`) becomes `{n, label, run, race}`. Add an "All tests" step that is the full run.
- Files: stub files (editable) and test files (read only), with content. `testdata/` names are listed but not shown.
- Hidden material decoded from ROT13: hints 1–3 (split on `## Hint N`), explanation, walkthrough, solution files. Rendered to HTML. Never sent to a browser before the level is revealed.
- The root README sections (Why this exists, Start here, The rules, How to work an exercise, A problem-solving checklist, Make it a tool) for the guide page. Course-wide tiers: 1 01–03, 2 04–09, 3 10–14, 4 15–19, 5 20–25 (parse from the root README tier table).

## Learning features

- **Code.** The editor shows the user's saved version of each stub file, or the stub if none. Autosave 1.5 s after the last edit (`PUT`). "Reset file" restores the stub after a confirmation.
- **Lint as you type.** 1.2 s after the last edit the UI calls check with all stub files; diagnostics become Monaco markers and a Problems list. A new edit cancels the pending request (AbortController).
- **Format.** Button and `Shift+Alt+F`: format the active file.
- **Run tests.** A step selector (from the README steps, plus "All tests") and a Run button (`Ctrl/Cmd+Enter`). The output panel shows a live test tree (pass, fail, skip, running) and the raw output of the selected test. File and line references in output (`retry_test.go:42:`) are links that open that file at that line. `-race` is on by default; a toggle turns it off for faster runs.
- **Progress.** An exercise is `started` when first opened. It becomes `passed` the first time an "All tests" run with `-race` passes with zero failures. Active time counts 15-second heartbeats while the tab is visible and the user typed or clicked in the last 60 s. Runs are counted. Notes are free text per exercise, autosaved.
- **Hint ladder.** Levels in order: hint 1, hint 2, hint 3, explanation, walkthrough, solution. The server refuses level N if N−1 is not revealed. Soft gates from the course rules: before hint 1, if the active time is under 25 minutes, the UI says so and asks for a confirmation; between hints the same with 10 minutes since the previous reveal. Revealed levels stay visible.
- **Dashboard.** Tiers with their exercises, status, time, hints used, and overall progress. A "learning log" table (time, runs, hints, passed date per exercise) matches step 10 of the course README.

## Certificates

- Eligibility: a tier certificate when every exercise of that tier is `passed`; the course certificate when all 25 are `passed`. Each kind can be issued once per user (re-issuing replaces the name and the stats choice; the old ID is revoked and points to the new one).
- Claim: the user enters the name to print (default display name) and chooses whether to show stats. Stats: active time, runs, hints revealed (levels 1–3), explanations and walkthroughs revealed, solutions revealed, per exercise and in total.
- Signature: Ed25519 over the canonical payload JSON `{"id","kind","name","issuedAt","exercises":[slugs],"stats"?}`. The key pair is generated at first start and stored in `settings`; the public key is served at `/.well-known/go-practice-certificates.pub` (raw base64) and its fingerprint is shown on verify pages and in the PDF.
- PDF: A4 landscape, embedded OFL fonts, vector QR code to the verify URL, certificate ID, name, what was completed (tier name and its exercises, or the 5 tiers), date, the fingerprint, and stats if chosen. It must look like a real, well-designed certificate.
- Verify page `/verify/<id>`: public, no login. Shows valid, revoked (with the replacement) or unknown; the name, kind, date, exercises, stats if chosen, signature status and the fingerprint. Links to the PDF at `/verify/<id>.pdf`.

## HTTP routes

Pages (HTML): `/` landing (public), `/login`, `/join`, `/setup`, `/recover`, `/recover/confirm`, `/verify-email`, `/learn` dashboard, `/learn/<exercise>` IDE, `/guide`, `/account`, `/certificates`, `/admin`, `/verify/<id>`, `/verify/<id>.pdf`.

API (JSON, session required unless marked public; all mutations need the CSRF header):

| Method and path | Purpose |
|---|---|
| `POST /api/auth/register/begin` `{invite or setup token, username, displayName, email}` → WebAuthn creation options | Sign-up |
| `POST /api/auth/register/finish` (credential JSON) | Sign-up |
| `POST /api/auth/login/begin` → request options; `POST /api/auth/login/finish` | Login (public) |
| `POST /api/auth/logout` | |
| `POST /api/auth/passkeys/begin`, `.../finish`; `PUT /api/auth/passkeys/<id>` `{name}`; `DELETE /api/auth/passkeys/<id>` | Manage passkeys |
| `POST /api/auth/recover` `{email}` (public); `POST /api/auth/recover/begin` `{token}`; `POST /api/auth/recover/finish` | Recovery |
| `PUT /api/account` `{displayName, email}` | Account |
| `GET /api/exercises/<ex>` → `{exercise meta, steps, files:[{name, kind:"stub"|"test", readonly, content}], progress, revealed:[{level, html}]}` | IDE bootstrap |
| `PUT /api/exercises/<ex>/files/<file>` `{content}`; `POST /api/exercises/<ex>/files/<file>/reset` → `{content}` | Code |
| `POST /api/exercises/<ex>/check` `{files}` → runner check response | Lint |
| `POST /api/exercises/<ex>/format` `{file, content}` → runner format response | Format |
| `POST /api/exercises/<ex>/test` `{step or run, race}` → NDJSON stream (runner events, plus a final `{"type":"progress", ...}`) | Run |
| `POST /api/exercises/<ex>/reveal` `{level, confirm}` → `{level, html}` or 409 `{gate:"time", minutes, needed}` | Hints |
| `POST /api/exercises/<ex>/heartbeat`; `PUT /api/exercises/<ex>/notes` `{notes}` | Progress |
| `GET /api/certificates`; `POST /api/certificates` `{kind, name, showStats}` → `{id}` | Certificates |
| `POST /api/admin/invites` `{email?, note?, days}` → `{url}`; `DELETE /api/admin/invites/<hash>`; `PUT /api/admin/users/<id>` `{disabled}` | Admin |

Errors are JSON `{"error": "human readable message", "code": "machine_code"}` with a fitting status.

## Frontend

- Pages are server-rendered; JS modules enhance them. Every page works with keyboard only, has visible focus, meets WCAG 2.1 AA contrast, and supports light and dark (system default, with a toggle stored in `localStorage`).
- The IDE page is a full-height layout: a left panel with tabs (Spec, Steps, Hints, Notes), the editor with file tabs (stub files editable, test files marked read only), a bottom panel (Tests, Problems, Output), and a status bar (saved state, lint state, Go version, keyboard hints). Panels are resizable. Below 900 px wide the panels stack.
- Monaco is the editor: Go language, a custom theme that matches the site in light and dark, markers from check, `Ctrl/Cmd+S` saves, `Ctrl/Cmd+Enter` runs.
- The design follows the frontend-design skill: a distinctive, polished identity for a Go learning platform, not a generic template. Consistent tokens (CSS custom properties) for colour, type, spacing and radius.

## Observability

- Logs: `log/slog` JSON to stdout. One line per HTTP request (method, path pattern, status, duration, user ID), per runner call, per email.
- Metrics on `GPW_METRICS_ADDR` and `GPR_METRICS_ADDR` (`:9091` for the runner) with `prometheus/client_golang`: HTTP requests by route and status, runner requests and durations by kind and result, queue length, active sessions, users, passed exercises, issued certificates.
- `/healthz`: process alive. `/readyz`: database read and write (a row in a probe table) and the course loaded. The web readiness does not depend on the runner; the IDE shows a clear message when the runner is down.

## Testing

- Unit tests per package. `internal/runner` tests use the real course submodule and the local Go toolchain: a stub fails, a reference solution passes, a timeout kills the process group, file-name validation, output cap.
- `internal/web` tests use `httptest` with a fake runner and the go-webauthn test helpers or a virtual authenticator.
- E2E (`e2e/`, Playwright, Chromium): with a CDP virtual authenticator: setup link, sign up, sign out, sign in, open 01-logfmt, type the reference solution, run all tests (passes), reveal hint 1 with the soft gate, claim nothing yet; an admin creates an invite and a second user joins; recovery by email (log sender) adds a new passkey; a verify page for a seeded certificate renders and the PDF downloads.
- CI runs `go vet`, `go test -race ./...`, `golangci-lint`, the E2E suite and builds both images.

## Build and release

- `Dockerfile.runner`: `golang:1.27-bookworm`, installs `staticcheck` (pinned), copies the course, pre-warms the build cache (`go test -race -run '^$'` and `go vet` over all exercises) into `/opt/gocache-seed`, runs as 65532.
- `Dockerfile.web`: build stage with Go and the Monaco tarball from the npm registry; final stage `gcr.io/distroless/static-debian12:nonroot` with the binary, templates and static files embedded (`embed.FS`) and the course copied to `/course`.
- GitHub Actions: CI on pull requests; on a tag `vX.Y.Z` build and push `ghcr.io/yornik/go-practice-web:vX.Y.Z` and `ghcr.io/yornik/go-practice-runner:vX.Y.Z` (linux/amd64). The gitops repository pins these tags; Renovate bumps them.

## Deployment (sharkshere-gitops)

One ArgoCD Application `go-practice` with `path: manifests/go-practice`. Two namespaces:

- `go-practice`: web Deployment (1 replica, `Recreate`), Service with ExternalDNS annotations, IngressRoute (websecure, `letsencrypt`) plus the `web` redirect route, middlewares (security headers, redirect, fail2ban scoped to `/api/auth/`), PVC `go-practice-data-ssd` (`truenas-ssd`, 2Gi), ksops Secret (runner token, SMTP password), ServiceMonitor.
- `go-practice-runner`: runner Deployment, Service (cluster internal), ksops Secret (runner token), NetworkPolicies, ServiceMonitor. Its own namespace keeps arbitrary code away from the web Secrets, like `gitlab-ci`.

Manifests follow the repository's `AGENTS.md`, `REVIEW.md` checklist B and `docs/styleguide.md`.
