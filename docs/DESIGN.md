# go-practice-web design

go-practice-web is a self-hosted web IDE for the [go-practice](https://github.com/Yornik/go-practice) course. A learner signs in with a passkey, reads an exercise, writes Go in a browser editor with highlighting and lint markers, and runs the real tests (with `-race`) on a sandboxed runner in the cluster. The site tracks progress, reveals hints 1 level at a time and issues signed PDF certificates with a public verify page.

This file is the contract between the components. If code and this file disagree, fix one of them in the same pull request.

## Decisions

| Topic | Decision |
|---|---|
| Public URL | `https://go-learn.party` (apex). The passkey relying party ID is `go-learn.party`. |
| Sign-up | Invite only. The first account is created with a one-time setup link that the web server logs at start when no user exists. That account is admin. Admins create single-use invite links. |
| Login | Passkeys only (WebAuthn, discoverable credentials). No passwords. |
| Recovery | A verified email address. A recovery link (single use, 15 min) lets the user register a new passkey. |
| Mail | SMTP with implicit TLS through Resend (`smtp.resend.com:465`, user `resend`, password = API key), sender `go-practice@yornik.eu`. The cluster already sends Resend mail from `yornik.eu`. |
| Database | SQLite (`modernc.org/sqlite`, pure Go) on 1 PVC, 1 replica. `journal_mode=DELETE` because the PVC is NFS. Daily online backup with `VACUUM INTO`, 7 rotating files. |
| Code execution | A separate `runner` Deployment in its own namespace runs `go vet`, `staticcheck`, `gofmt` and `go test -race` in a sandbox. The web server calls it over HTTP with a bearer token. |
| Certificates | 1 per tier (5) and 1 for the full course. PDF with a QR code to `https://go-learn.party/verify/<id>`. Ed25519-signed. The owner chooses per certificate whether the verify page shows hint and solution counts. |
| Frontend | Server-rendered `html/template` pages, vanilla ES modules, Monaco editor vendored from npm. No frontend framework and no Node at run time. Designed with the frontend-design skill. |

## Repository layout

```text
cmd/web/                The web server (main package only: config, wiring).
cmd/runner/             The runner server (main package only).
cmd/gendocs/            Build step: indexes the standard library docs of the runner's Go release for the editor.
internal/runnerapi/     Request and response types shared by web and runner. The wire contract.
internal/runner/        Runner side: workspace preparation, sandboxed exec, go test -json parsing.
internal/course/        Loads the course from course/: exercises, steps, files, rendered READMEs, hidden material.
internal/store/         SQLite access, migrations (embedded SQL), backups.
internal/auth/          Passkeys (go-webauthn), sessions, invites, setup token, email tokens, rate limits.
internal/mail/          SMTP sender and the email templates. A log-only sender when SMTP is not configured.
internal/cert/          Certificate eligibility, signing, PDF rendering (signintech/gopdf), QR (rsc.io/qr).
internal/godoc/         The embedded standard library docs index (data/stdlib.json.gz), lookups and the /api/docs handler.
internal/web/           HTTP handlers, middleware (CSRF, CSP, sessions), template rendering, API.
web/templates/          html/template pages and partials.
web/static/             CSS, JS modules, fonts, images. Monaco is copied to web/static/vendor/monaco/ at build time.
course/                 Git submodule: github.com/Yornik/go-practice, pinned to a commit.
scripts/                fetch-monaco.sh, dev.sh, e2e helpers.
e2e/                    Playwright tests (Node, dev only).
docs/                   This file and operator docs.
Dockerfile.web, Dockerfile.runner
```

Go module: `github.com/yornik/go-practice-web`, `go 1.26` (the minimum of its dependencies). The images build with the current Go release (`golang:1.27-bookworm`). The course itself stays at `go 1.24`.

## Configuration

All configuration is environment variables. Unknown or invalid values stop the process at start with a clear error.

### Web (`GPW_`)

| Variable | Default | Meaning |
|---|---|---|
| `GPW_ADDR` | `:8080` | HTTP listen address for the site. |
| `GPW_METRICS_ADDR` | `:9090` | Prometheus `/metrics`. Not routed publicly. |
| `GPW_PUBLIC_URL` | (required) | `https://go-learn.party`. Origin for WebAuthn, links in emails and certificates. An origin only (no path or query); `https` unless `GPW_DEV`. |
| `GPW_DB_PATH` | `/data/go-practice.db` | SQLite file. Backups go to `<dir>/backups/`. |
| `GPW_COURSE_DIR` | `/course` | Checkout of go-practice. |
| `GPW_RUNNER_URL` | (required) | For example `http://runner.go-practice-runner.svc:8081`. |
| `GPW_RUNNER_TOKEN` | (required) | Bearer token for the runner, at least 32 characters; equal to `GPR_TOKEN`. |
| `GPW_SMTP_HOST`, `GPW_SMTP_PORT`, `GPW_SMTP_USER`, `GPW_SMTP_PASSWORD` | empty | When the host or the password is empty or `REPLACE_ME`, mail is logged instead of sent. |
| `GPW_MAIL_FROM` | `Go Practice <go-practice@yornik.eu>` | Sender. |
| `GPW_TRUSTED_PROXIES` | `10.244.0.0/16` | CIDRs whose `X-Forwarded-For` is trusted for client IPs, separated by commas or spaces; empty trusts none. It must cover the Traefik pods (the cluster's pod CIDR; k3s uses `10.42.0.0/16`): otherwise every client counts as Traefik's address, and 1 client can use up the sign-in limits for everyone. Only trusted proxies may reach the web pods (NetworkPolicy), or a pod could forge `X-Forwarded-For`. |
| `GPW_DEV` | `false` | Local development: allows `http://localhost`, drops the `__Host-` cookie prefix and `Secure`. |

### Runner (`GPR_`)

| Variable | Default | Meaning |
|---|---|---|
| `GPR_ADDR` | `:8081` | Listen address. |
| `GPR_METRICS_ADDR` | `:9091` | Prometheus `/metrics`. |
| `GPR_TOKEN` | (required) | Expected bearer token, at least 32 characters (`openssl rand -hex 32`). |
| `GPR_COURSE_DIR` | `/course` | Checkout of go-practice (tests, testdata, go.mod). |
| `GPR_WORK_DIR` | `/tmp/runs` | Per-run directories. |
| `GPR_GOCACHE` | `/cache/go-build` | Shared build cache. Seeded at start from `GPR_GOCACHE_SEED` if empty. |
| `GPR_GOCACHE_SEED` | `/opt/gocache-seed` | Pre-warmed cache baked into the image. |
| `GPR_MAX_CONCURRENT` | `2` | Concurrent `go` invocations. More requests queue up to `GPR_MAX_QUEUE`. |
| `GPR_MAX_QUEUE` | `8` | Queue length. When full: HTTP 429 with `Retry-After`. |
| `GPR_MAX_QUEUE_WAIT` | `12s` | Longest wait of a test run for a slot. A run that waited longer does not start: its stream ends with a `done` event with `busy`. Keep this plus `GPR_TEST_TIMEOUT` under the 60 s Traefik timeout with a few seconds to spare; the runner logs a WARN at start when the sum is above 55 s. |
| `GPR_TEST_TIMEOUT` | `40s` | Wall clock for `go test`, the build included. `go test` itself gets `-timeout` 5 s less, so that a hanging test ends with Go's own panic and goroutine dump; the runner's kill at `GPR_TEST_TIMEOUT` is the backstop. After SIGTERM the runner lets running jobs finish for this long plus 5 s, then kills them; set `terminationGracePeriodSeconds` to at least this plus 15 s. |
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
`source` is one of `gofmt`, `compile`, `vet`, `staticcheck`. `code` is the vet analyzer name (run `go vet -json` to get it) or the staticcheck code; the explain catalog matches on it. `severity` is `error`, `warning` or `info` (vet and staticcheck findings are warnings). Diagnostics for test files have the test file's name; the UI shows them only in the problems list. `related` (staticcheck only, optional) is the first related position of a finding, `{file, line, column, endLine, endColumn, message}`, for example where the left-hand side of SA4023's comparison gets its value: `{"file": "retry.go", "line": 15, "column": 18, "endLine": 15, "endColumn": 32, "message": "the lhs of the comparison gets its value from here and has a concrete type"}`. The runner keeps it only when it is in a file of the exercise (not in the standard library), so the UI can show it next to the finding and link it.

`POST /v1/format` — `gofmt` one file.
```json
{"exercise": "07-retry", "file": "retry.go", "content": "..."}
```
Response: `{"content": "...formatted...", "diagnostics": [...]}`. On a syntax error, `content` is the input unchanged and `diagnostics` holds the error.

`POST /v1/test` — `go test -json -count=1 -timeout=<see Timeouts> [-race] [-run <regexp>] ./exercises/<exercise>/`.
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
 "timedOut": false, "truncated": false, "buildFailed": false, "raceUnavailable": false, "busy": false}
```
`test` events map 1 to 1 to `go test -json` events (Action, Test, Elapsed, Output). Package-level output (no `test`) is sent with an empty `test`. Output is capped at 1 MiB per run; after the cap the runner kills the run and sets `truncated`. The `done` fields that are false or 0 are left out. `raceUnavailable` is set when the request asked for `race` and the run went without it, because the race detector does not work on this node (see "Race detector"). `busy` is set when the run never started: it waited longer than `GPR_MAX_QUEUE_WAIT` for a slot; `exitCode` is -1 and the client may try again in a few seconds. The stream has started with the `queued` events by then, so the answer is this `done` event, not a 429.

`GET /v1/info` (with the token) — what this runner can do, probed once at start, before the runner listens:
```json
{"goVersion": "go1.27.2", "raceAvailable": true, "staticcheck": true,
 "sandbox": "landlock ABI 6: files, ptrace, tcp, signals", "landlockABI": 6}
```
When `raceAvailable` is false, `raceError` holds the line of the probe's output that names the problem, for operators (for example `go run -race exited with code 1: ==12==FATAL: ThreadSanitizer: unexpected memory mapping ...`). `sandbox` is the text of the `sandbox for runs` start log line; `landlockABI` 0 means runs are not confined. The answer does not change until the process restarts.

`GET /healthz` (process alive) and `GET /readyz` (course loaded, `go version` works, work dir writable). The body of a ready answer is `ok`, plus a second line `race detector not available: <reason>` when the race probe failed: the runner stays ready, because runs without `-race` work.

Details the runner fixes beyond the wire types:

- **Errors.** 400 for an unknown exercise, a file name that is not a stub, a missing stub (`missing_file`), a file or total over the limit or an invalid `run` regular expression; 413 for a body over 6 MiB + 64 KiB (JSON escaping can grow 1 MiB of content up to 6 times); 429 `queue_full` with `Retry-After: 5`; 503 `shutting_down` while draining (`/v1/info` still answers then: it starts no work); 503 `not_ready` from `/v1/info` before the start probe ran; 504 `timeout` when `go vet` runs out of `GPR_CHECK_TIMEOUT` (a `staticcheck` that runs out is skipped and the vet results are returned).
- **Check.** A file that does not parse returns only the `gofmt` diagnostics and skips the tools. Columns are byte columns, as the Go tools count them. `staticcheck` runs only when vet found no compile error, and only its findings in stub files are returned: the course's test files are not the learner's to fix. Its cache lives next to `GPR_GOCACHE` (`/cache/staticcheck`).
- **Queue position.** `position` counts every request ahead, the running ones included, so it is at least 1. It is sent again whenever it changes.
- **Queue wait.** A test request waits at most `GPR_MAX_QUEUE_WAIT` for a slot, then ends with `{"type": "done", "exitCode": -1, "busy": true}` and leaves the queue. Behind the web server sits Traefik, which cuts any response after 60 s; a run that starts late would be cut off there with no result at all. Checks have no such cap: the web server bounds them with its own deadline.
- **Timeouts.** `go test` gets `-timeout` set to `GPR_TEST_TIMEOUT` minus 5 s (at least a quarter of `GPR_TEST_TIMEOUT`). When it fires, the test binary panics with `panic: test timed out after 35s`, lists the tests that were running and prints every goroutine, which shows the learner where the code waits; go test exits 1, and `done` has `exitCode` 1 and `timedOut`. The panic is in the output of the test that was running, and no test reports `fail`. The runner's own kill at `GPR_TEST_TIMEOUT` stays as the backstop for what the test binary's alarm does not cover: a slow build, a hang in `init` or in `TestMain`. That run has `exitCode` -1 and `timedOut`.
- **Race detector.** At start, before it listens, the runner builds and runs a tiny program (2 goroutines and a channel) with `go run -race`, in a scratch workspace in `GPR_WORK_DIR`, with the environment and the sandbox of a learner's run. If it fails, for example with `FATAL: ThreadSanitizer` because the RuntimeDefault seccomp profile of the node (Talos) blocks `personality(ADDR_NO_RANDOMIZE)`, the runner logs 1 WARN line (`race detector not available: test runs go without -race`, with the reason), reports `raceAvailable: false` in `/v1/info` and on `/readyz`, and runs every test request without `-race`, with `raceUnavailable` in its `done` event. The probe runs once per process; a new pod probes again.
- **Counts.** `passed`, `failed` and `skipped` count leaf tests, so 1 failing subtest is 1 failure, not 2. A parent test that fails while all its subtests pass counts as 1 failure.
- **Killed runs.** A run killed for the timeout, the output cap, a client that left or a shutdown has `exitCode` -1. Of these, `timedOut` is set only for a run that was killed at the deadline; it is also set when go test's own `-timeout` fired (see "Timeouts").
- **Paths in output.** The workspace path is cut from all output, so a stack trace shows `exercises/07-retry/retry.go:12`.
- **Workspace.** `GPR_WORK_DIR/run-<random>/` holds `mod/` (the module: `go.mod`, `exercises/<exercise>/`), and the per-run `home/`, `gopath/` and `tmp/`. At start the runner removes `run-*` directories a previous process left; nothing else in `GPR_WORK_DIR` is touched.

### Sandbox rules

- File names in `files` must be exactly the exercise's stub file names (`*.go`, not `_test.go`). Anything else is a 400. This blocks path traversal and test replacement. Max 256 KiB per file, 1 MiB total.
- Each request gets a fresh directory `GPR_WORK_DIR/<random>/` with `go.mod` from the course, the exercise's `*_test.go` and `testdata/` copied from the course, and the learner's files. The directory is deleted when the request ends.
- `go` runs as a child process in its own session (and so its own process group), with a minimal environment: `HOME`, `PATH`, `GOCACHE`, `GOPATH` and `TMPDIR` (`HOME`, `GOPATH` and `TMPDIR` per run, so that a killed run leaves nothing in `/tmp`), `GOFLAGS=-mod=mod`, `GOPROXY=off`, `GOTOOLCHAIN=local`, `GOTELEMETRY=off`, `CGO_ENABLED=1` (the race detector needs cgo), `HTTP_PROXY`/`HTTPS_PROXY`/`ALL_PROXY=http://127.0.0.1:9` (a dead end for code that honours proxies; not a security boundary). `staticcheck` also gets `STATICCHECK_CACHE`. `GOTELEMETRY` cannot be set from the environment, so each per-run `HOME` holds the telemetry mode file with `off`; otherwise `go` starts a telemetry child in its own session, outside the process group.
- **Landlock.** Learner code runs as the runner's user, so the kernel's file permissions do not separate runs. Every child starts confined by Landlock (no privileges needed; `internal/runner/sandbox_linux.go`). The runner restricts a spare thread (never the main thread: the kernel checks signals and ptrace against it) with `no_new_privs` and 2 Landlock layers, starts the child from it and lets the thread end. What a run may do:
  - Files: read and execute `/usr`, `/bin`, `/sbin`, `/lib*`, GOROOT, the directories of `go` and `staticcheck` and of `PATH`; read `/etc`, `/proc`, `/sys`; read and write `/dev` (no device nodes); read, write and execute its own workspace, `GPR_GOCACHE` and the staticcheck cache. Everything else is denied: other runs' workspaces, the course checkout (it holds `solutions/` in development), `/tmp`, `/var/run/secrets`.
  - Processes: Landlock's ptrace rule keeps a run from `ptrace` and from `/proc/<pid>/mem`, `environ` and `exe` of processes outside its domain, such as PID 1 (`tini`, whose environment holds `GPR_TOKEN`) and the runner. With ABI 6 (Linux 6.12) it also cannot signal them or reach abstract Unix sockets outside. A process with `CAP_PERFMON` or `CAP_SYS_ADMIN` may still read other environments, so the pod must not grant them.
  - TCP (ABI 4, Linux 6.7): bind only to an ephemeral port (port 0) and connect only to ports in the ephemeral range (`net.ipv4.ip_local_port_range`), which is what `httptest.Server` and `net.Listen("tcp", "127.0.0.1:0")` need. The runner's own API and metrics, the Kubernetes API, DNS over TCP and web servers on their usual ports are out of reach. UDP is not covered.
  - The level depends on the node's kernel; the runner logs it at start (`sandbox for runs` with the field `sandbox`; WARN when Landlock is missing, in which case runs are not confined). When the kernel has Landlock but refuses the rules, the run fails instead of running unconfined.
- **Processes that outlive a run.** On timeout, cancellation or output over the cap the runner kills the session's process group (SIGKILL), and it does so after a normal exit too. A process that left the group (`setpgid`) or the session (`setsid`) survives that, so the runner is a child subreaper: such a process becomes the runner's child when its parent dies. After every command and every 5 s the runner sweeps: every descendant outside the session of a running command is stopped (SIGSTOP, repeated until nothing in the set runs, so a fork bomb cannot outrun it) and then killed, through a pidfd checked against the process start time, and zombies are reaped. A run cannot join the runner's own process group, because `setpgid` works only within a session. The count is the metric `runner_stray_processes_killed_total` and a WARN log line. `tini` is PID 1 and passes SIGTERM on; the runner process is non-dumpable, so learner code cannot read its memory even without Landlock.
- **Limits.** Each child gets `oom_score_adj` 1000 (the OOM killer takes learner code before the runner), no core dumps and at most 256 MiB per file it writes. Output is capped at 1 MiB per run, and runs at `GPR_TEST_TIMEOUT` (the test binary's own `-timeout` fires 5 s earlier). The race probe at start runs under the same rules, so it fails exactly where a learner's `-race` run would.
- The runner image holds only `go.mod` and `exercises/` of the course. `solutions/` stays out: without Landlock (an old node kernel) learner code can read every file in the image.
- The runner process holds no secret but its own token. The pod has no service account token, runs as UID 65532, has a read-only root filesystem, drops all capabilities and uses the RuntimeDefault seccomp profile. `/tmp` and `/cache` are `emptyDir` volumes with size limits.
- Network isolation is a NetworkPolicy (no egress at all, ingress only from the web namespace). It only works if the cluster's CNI enforces NetworkPolicies. Document this; do not depend on it for anything except defense in depth.

What the deployment must provide, because code cannot:

- A node kernel with Landlock in its LSM list, Linux 6.12 or later for every rule above (check the runner's start log). The RuntimeDefault seccomp profile of containerd 1.7 and later allows the Landlock system calls.
- For `-race`: a seccomp profile that lets ThreadSanitizer call `personality(ADDR_NO_RANDOMIZE)` where the node needs it. The RuntimeDefault profile on Talos does not; the runner then turns `-race` off (see "Race detector") and says so in `/v1/info`, `/readyz` and the IDE. A `Localhost` seccomp profile that allows this 1 call brings `-race` back.
- `securityContext`: `runAsNonRoot`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`, `readOnlyRootFilesystem`, `seccompProfile: RuntimeDefault`, `automountServiceAccountToken: false`.
- A PID limit for the pod. Kubernetes sets it per node (kubelet `podPidsLimit`; on k3s `--kubelet-arg=pod-max-pids=1024`). Without it a fork bomb in a run can use up the node's PIDs for up to `GPR_TEST_TIMEOUT`.
- `resources.limits` for CPU and memory (Go then sizes `GOMAXPROCS` from the CPU limit), and `ephemeral-storage`.
- `emptyDir.sizeLimit` on `/tmp` and `/cache` (about 2 GiB for `/cache`). A run can still fill them with many files under the 256 MiB per-file limit; the kubelet then evicts the pod, which restarts with an empty `/tmp`.
- NetworkPolicies: no egress from the runner namespace (UDP included), ingress to `:8081` only from the web pods and to `:9091` only from Prometheus.

Remaining risks, accepted for an invite-only course:

- The build cache is shared and writable by runs (go must write to it). A learner who plants a forged build output for a standard library package changes the test binaries of later runs, other learners' included, inside their sandbox: their tests could pass or fail at will, and the planted code could copy their files into the cache for the attacker's next run to read. The fix is a `GOCACHEPROG` that serves the read-only seed of the image and keeps each run's writes apart. The staticcheck cache has the same property; there it only changes lint messages.
- A pass is what the test binary reports, and learner code runs inside it: it can print test2json lines and exit 0. The web server requires every top-level test to report (see "Progress"), which stops the cheap fakes, but a determined learner can still fake a pass, and the reference solutions are public in the course repository. Certificates record what the site saw, not proof of skill.
- Without Landlock ABI 6 a run can signal the runner and `tini` (same user), which restarts the pod; without ABI 4 it can connect to the runner's own ports (it needs the token, which Landlock's ptrace rule protects).

## Data model

SQLite, migrations in `internal/store/migrations/NNNN_name.sql`, applied in order at start inside a transaction, version in `schema_migrations`. A database with a migration this build does not know stops the start. Times are UTC RFC 3339 text with milliseconds and a fixed width (`2026-10-17T09:12:33.000Z`), so that text order is time order. IDs are integers except where noted.

| Table | Columns (key ones) |
|---|---|
| `users` | id, username (unique, 3–32 chars `[a-z0-9_-]`), display_name, email (unique, nullable), email_verified_at, is_admin, disabled_at, webauthn_handle (32 random bytes, unique), created_at |
| `credentials` | id (credential ID bytes, PK), user_id, public_key, attestation_type, aaguid, sign_count, clone_warning, transports, attachment, user_present, user_verified, backup_eligible, backup_state, name (user label), created_at, last_used_at |
| `sessions` | token_hash (SHA-256, PK), user_id, created_at, expires_at, last_seen_at, user_agent |
| `webauthn_ceremonies` | id (random, PK), kind (register, login, add, recover), user_id (nullable), data (JSON SessionData), expires_at |
| `invites` | token_hash (PK), created_by, email (nullable), note, created_at, expires_at, used_by, used_at |
| `setup_tokens` | token_hash (PK), created_at, used_at |
| `email_tokens` | token_hash (PK), user_id, purpose (verify, recover), email, created_at, expires_at, used_at |
| `code` | user_id, exercise, file, content, updated_at; PK (user_id, exercise, file) |
| `progress` | user_id, exercise, status (started, passed), started_at, passed_at, active_seconds, runs, last_run_at, last_result (JSON summary), notes; PK (user_id, exercise) |
| `reveals` | user_id, exercise, level (1–6), revealed_at; PK (user_id, exercise, level) |
| `certificates` | id (text, PK, `GP-XXXX-XXXX-XXXX` from a Crockford base32 alphabet), user_id, kind (tier1..tier5, course), name_on_cert, issued_at, show_stats, stats (JSON), payload (canonical JSON that was signed), signature, revoked_at, replaced_by; a partial unique index allows 1 valid certificate per user and kind |
| `settings` | key (PK), value — holds the certificate signing key (generated at first start) |
| `audit` | id, user_id (nullable), event, detail (JSON), ip, created_at |
| `probe` | id (always 1), value, updated_at — readiness writes a random value and reads it back |

`internal/store` makes each flow atomic, so the web layer needs no locks: a registration uses its invite or setup token in the same transaction as the new user and passkey; recovery uses its token, adds the passkey and optionally removes the others and the sessions together; deleting a passkey cannot remove the last one; a reveal of level N succeeds only when level N−1 exists, checked in the insert itself; `MarkPassed` sets `passed_at` once; re-issuing a certificate revokes the old one and sets `replaced_by` in the same transaction. A new email token deletes the user's unused tokens of the same purpose. A change of email stores a verify token for the new address; `VerifyEmail` moves it into `users.email`, so the old verified address works for recovery until then.

Connection settings: 1 `*sql.DB` with 4 connections, `busy_timeout` 10 s, `foreign_keys=ON`, `synchronous=FULL`, `journal_mode=DELETE`, and `BEGIN IMMEDIATE` transactions (a deferred transaction that reads and then writes can fail with `SQLITE_BUSY` without waiting). The daily backup runs `VACUUM INTO` a temporary file, checks it with `PRAGMA integrity_check`, fsyncs it and renames it to `go-practice-<weekday>.db`, so a bad copy never replaces a good one.

## Authentication and accounts

Library: `github.com/go-webauthn/webauthn`. Relying party: ID = host of `GPW_PUBLIC_URL`, origins = `[GPW_PUBLIC_URL]`, display name `Go Practice`. Registration requires a resident key (discoverable credential) and user verification `preferred`. Attestation `none`.

Flows:

1. **Setup.** At every start while `users` is empty, generate a 32-byte token, store its hash in place of any unused setup token, and log `setup link: <PUBLIC_URL>/setup?token=<token>` at WARN level. A pod that restarts before setup so logs a link that works; the older link stops working. `/setup` works like an invite and makes the account admin.
2. **Sign-up with an invite.** `/join?invite=<token>`: the user picks a username, display name and email, then creates a passkey. On success: the account and credential are stored, the invite is used, the user is signed in and a verification email goes out. An invite is single use and expires (default 7 days). A link that no longer works says why, with the date: "This invite link expired on 24 Oct 2026." or "Someone used this invite link on 17 Oct 2026."
3. **Login.** `/login`: "Sign in with a passkey" (discoverable flow, no username) plus conditional UI (passkey autofill) when the browser supports it.
4. **Sessions.** Cookie `__Host-gpw_session` (HttpOnly, Secure, SameSite=Lax, Path=/), 32 random bytes, stored as SHA-256, 30-day expiry, sliding (refreshed at most once per hour, and the cookie is renewed with it). Logout deletes the row; "sign out everywhere" deletes all the user's rows. With `GPW_DEV` the cookie is `gpw_session` without `Secure`, because browsers refuse the prefix over http. A disabled account has no session.
5. **Account page.** List, rename and delete passkeys (not the last one), add a passkey, change display name, change email (sends a new verification), sign out everywhere. An unverified address is replaced at once; a verified one stays in use for recovery until the new one is verified, and it cannot be removed (it is the only way back in).
6. **Recovery.** `/recover`: enter an email address. The response is always the same message, and the lookup and the mail run in the background, so neither the answer nor its timing tells whether the address has an account. If an enabled user with that verified email exists, send a link `/recover/confirm?token=<token>` (15 min, single use). That page lets the user create a new passkey and optionally remove all old ones and every session. Afterwards an email reports that a passkey was added through recovery.
7. **Security notices.** Email the user when a passkey is added or removed, when the email changes (to the old and the new address), and when recovery is used.
8. **Admin.** `/admin`: create invite links (optionally emailed; the mail goes out at once, so the admin sees a failure), list and revoke invites, list users with progress, disable or enable a user. Admins cannot disable themselves or remove their own admin flag.
9. **Ceremonies.** A begin endpoint answers `{ceremony, options}`: `options` is go-webauthn's `{publicKey, mediation}` for `navigator.credentials.create` or `.get`, with byte fields in base64url. The finish request is `{ceremony, credential, ...}` with the browser's `PublicKeyCredential` as JSON. The server half (go-webauthn `SessionData`) is stored in `webauthn_ceremonies` for 5 minutes and taken once, so a ceremony cannot be replayed. The ID travels in the body, not a cookie, so the autofill request of the login page and a click on its button can run side by side.
10. **Sign counter.** A login whose signature counter does not grow beyond the stored one (when either is non-zero) is refused with 403 `passkey_cloned`, and the passkey is flagged on the account page: 2 copies of the key may exist. Synced passkeys always report 0 and never trip this. A credential that is unknown or was removed gets 400 `unknown_passkey`.

Protection:

- CSRF: every state-changing request is `POST`/`PUT`/`DELETE` with a JSON body, must carry `X-GPW-CSRF: 1`, and its `Origin` must equal `GPW_PUBLIC_URL`. SameSite=Lax is the second layer.
- CSP (set by the app on every response except the certificate PDF): `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; worker-src 'self' blob:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'`. An HTML page adds the SHA-256 of its import map to `script-src` (see "Frontend"). Add only what Monaco proves it needs, with a comment. Monaco needed `font-src data:`: its codicon icon font is a data URL in its CSS. Its workers are blob URLs that call `importScripts`, which `worker-src blob:` and `script-src 'self'` allow. The other headers: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: same-origin` (links carry tokens), `Cross-Origin-Opener-Policy: same-origin`, a `Permissions-Policy` without camera, microphone, geolocation, payment and USB, and HSTS outside dev mode.
- Rate limits (in memory token buckets, per client IP and per account): WebAuthn begin endpoints 20/min per IP; recovery 5/hour per IP and 3/hour per address (resending a verification link counts against the address too); check 30/min per user, and 1 check in flight per user (a newer check cancels the older one, at the runner too, which answers 409 `superseded`; the runner's slots are shared by everyone); format 60/min per user; test runs 10/min per user; at most 1 test run in flight per user (409 `run_in_progress`); saves, resets, notes and heartbeats together 120/min per user (each is a synced write on the shared NFS volume); verification links 5/hour per account and 3/hour per address (a change of email mails the new address); certificate claims 10/hour per user; certificate PDFs 30/min per IP. A limit answers 429 with `Retry-After`. Per-IP limits count an IPv6 client per /64, because it can use any address of its /64. The client IP is the peer address, or, when the peer is in `GPW_TRUSTED_PROXIES`, the rightmost `X-Forwarded-For` entry that is not a trusted proxy.
- Templates escape everything. Markdown from the course is trusted content (it is in our repository), rendered server-side by goldmark with raw HTML disabled.

## Course content

`internal/course` loads `GPW_COURSE_DIR` at start and keeps it in memory:

- Exercises in directory order. For each: number, slug, title (line 1 `# NN · Title`), tier and time (line 3 `**Tier N · about X h**`), the Go topics and the problem-solving text from line 3, and the README rendered to HTML with goldmark (GFM tables, heading IDs, chroma highlighting with CSS classes).
- The README is split into sections by `## ` heading so the UI can place them: Task, Rules, Examples, Run the tests, Docs you can use, Stretch goals. The "Stuck?" section is not shown; the hint ladder replaces it.
- Steps from the "Run the tests" section: every line `go test ... -run <regexp> ... # step N: label` (or `# part N: label`) becomes `{id, n, kind, label, run, race, command}`, with `id` `step-N` or `part-N`. Add an "All tests" step that is the full run: `id` `all`, empty `run`, `race` true. The API's `step` is the `id`. The runner takes `-run` only, so a README step with `-skip` (18-certcheck step 1) becomes an explicit `-run '^(TestA|TestB)$'` of the top-level tests that match `-run` and not `-skip`.
- Files: stub files (editable) and test files (read only), with content. `testdata/` names are listed but not shown.
- Hidden material decoded from ROT13: hints 1–3 (split on `## Hint N`), explanation, walkthrough, solution files (they must match the stub file names). Load decodes and checks it; each level is rendered to HTML on its first reveal, because highlighting it all would triple the start time. Never sent to a browser before the level is revealed.
- Markdown: goldmark with GFM and heading IDs; raw HTML in the course is shown as escaped text, not dropped (a `<Field>` outside backticks is a placeholder). A relative link to an exercise becomes `/learn/<slug>`; any other relative link goes to `https://github.com/Yornik/go-practice/blob/main/<path>`. `course.HighlightCSS()` returns the chroma CSS of the site's own styles (`gpw-light` and `gpw-dark`, the editor's syntax colours) scoped under `[data-theme="light"]` and `[data-theme="dark"]`; without a `data-theme` attribute the system preference decides.
- The root README sections (Why this exists, Start here, The rules, How to work an exercise, A problem-solving checklist, Make it a tool) for the guide page; the guide leaves out "Start here" (it sets up Go on your own machine) and adds the 4 generic guides of the explanation catalog. The descriptions of the ladder levels come from the README's "Level | What it gives you" table (`Course.LevelDescription`). Course-wide tiers: 1 01–03, 2 04–09, 3 10–14, 4 15–19, 5 20–25 (parse from the root README tier table).

## Learning features

- **Code.** The editor shows the user's saved version of each stub file, or the stub if none. Autosave 1.5 s after the last edit (`PUT`). "Reset file" restores the stub after a confirmation.
- **Lint as you type.** 1.2 s after the last edit the UI calls check with all stub files; diagnostics become Monaco markers and a Problems list. A new edit cancels the pending request (AbortController).
- **Format.** Button and `Shift+Alt+F`: format the active file.
- **Run tests.** A step selector (from the README steps, plus "All tests") and a Run button (`Ctrl/Cmd+Enter`). The output panel shows a live test tree (pass, fail, skip, running) and the raw output of the selected test. File and line references in output (`retry_test.go:42:`) are links that open that file at that line, for the exercise's files only. `-race` is on by default; a toggle turns it off for faster runs. When the runner's race detector does not work, every run goes without it, the IDE disables the toggle and says "race detector not available on this server" (the bootstrap's or the progress event's `raceUnavailable`). A disabled toggle still asks for `-race`, so a new runner pod with a race detector uses it. A run tests the saved files, so the browser saves its pending edits first.
- **Progress.** An exercise is `started` when first opened (the IDE bootstrap request). It becomes `passed` the first time a run without a `-run` filter and with `-race` ends with exit code 0, zero failures, at least 1 pass and no timeout, truncation or build failure, and every top-level test and fuzz target of the exercise reported `pass` or `skip` (examples are left out: one without an output comment never runs). On a runner whose race detector does not work (`raceAvailable` false in `/v1/info`, or `raceUnavailable` in the runner's `done` event), the same run without `-race` passes: no run can do better there. Learner code runs inside the test binary and can print test2json lines and exit 0; the last rule stops that cheap fake (see "Remaining risks" under "Sandbox rules"). Active time counts 15-second heartbeats while the tab is visible and in front (the window has the focus) and the user pressed a key, clicked or scrolled in the last 10 minutes: the course asks learners to work examples on paper and to read the spec twice, and that thinking counts. The server credits the time since the user's previous heartbeat, at most 15 s, so a second tab or a fast client adds nothing. The IDE shows "paused" in the Active cell while it sends no heartbeats. Runs that reach the runner's `done` event are counted, with a summary in `last_result`: the step, `-race`, the counts, the duration, the exit flags, the time, the result of each top-level test (`tests`) and the names of the failed subtests (`failedTests`, at most 200 each), so the IDE shows the last run's tree after a reload. The output and the explanations are not kept. Notes are free text per exercise (at most 64 KiB), autosaved.
- **Hint ladder.** Levels in order: hint 1, hint 2, hint 3, explanation, walkthrough, solution. The ladder is docked under the spec; levels 4 to 6 open in a reading sheet that covers the code, so rule 4 (do not copy the solution) holds by design. The server refuses level N if N−1 is not revealed (409 `reveal_order`). Soft gates from the course rules, checked by the server: before hint 1, if the active time is under 25 minutes, it answers 409 `{gate: "time", kind: "active", level, minutes, needed: 25}`; before levels 2 to 6 the same with `kind: "since"` and 10 minutes since the previous reveal. The UI shows the numbers and repeats the request with `confirm: true`. Once the exercise is passed there are no gates (rule 5 asks learners to read the explanation and the walkthrough after a pass); the order stays. Revealing a level again returns it without a gate. Revealed levels stay visible.
- **Course page (`/learn`).** The learning log of step 10 of the course README: the course to scale (25 bars as wide as their hours, by tier), the next exercise, and 1 table per tier (estimate, active time, runs, the help revealed, the date of the first pass) with the state of the tier's certificate.

## Learning-first feedback

This is a learning tool, not a developer tool. A learner must never see only "error at line 236". Every problem the
tools find comes with an explanation that teaches, written for the course's medior audience.

- **Catalog, not AI.** `internal/explain` holds a curated catalog of explanations (Markdown files with a small front
  matter, embedded). The course forbids LLM help, so explanations are deterministic and reviewed. Each entry matches
  one kind of message: a compiler or type error, a `go vet` analyzer, a staticcheck check code, a runtime panic, a race
  report, a deadlock, a test timeout, a failing assertion, an example output mismatch, a build problem.
- **Entry shape.** `id`, `source` (compile, vet, staticcheck, runtime, race, test, build), a `match` regular expression
  with named groups (or an analyzer name or a check code; `fallback: true` marks a broad pattern, such as any got/want
  message, that loses to every more specific match), a `title` with placeholders, and a body with these parts:
  *What it means* (plain language, using the captured names), *Why Go does this* (the language or runtime rule
  behind it), *How to find it* (where to look, what to print, which part of the message points where), *Example* (a
  small generic snippet that shows the mistake and the fix; never the exercise's solution), *Learn more* (Go spec,
  Effective Go, the Go blog, pkg.go.dev, staticcheck.dev docs) and, when it exists, the course exercise that teaches
  the concept (for example a data race points to 12-workerpool).
- **Verified against the real tools.** Every entry has a fixture: a tiny program that triggers it, compiled with the
  toolchain of the runner image, so the matcher is tested against the exact message text. A test fails when a
  message changes in a new Go release.
- **Raw message stays visible.** The UI shows the real message too, with its parts labelled (file, line, column,
  message), because learning to read real Go errors is part of the course.
- **Fallbacks.** A message without a catalog entry gets the generic guides: "How to read a Go error message", "How to
  read a stack trace", "How to read a race report", "How to read a failing test".
- **Where it runs.** The web server enriches runner results: every diagnostic in a check response gets an `explain`
  object `{id, title, html, links: [{label, url}], course}` (or the generic guide). In a test stream the web server
  adds `{"type": "explain", "test": "<name>", "items": [...]}` after a test fails or a build fails, from that test's
  output (panic, race, timeout, got/want assertion with the failing file:line, Example mismatch). When go test's
  `-timeout` fires, no test reports `fail`: the panic and the goroutine dump are in the output of the test that was
  running, so a failed run also explains, before `done`, every test that started but never reported a result. The
  timeout's `raw.file:raw.line` is where the test that hangs waits: the goroutines parked in the testing package (the
  alarm, goroutine 1, a parent test that waits in `t.Run` for its subtest) are skipped, and the goroutines that run a
  test come before the ones the code started. A parent of an unfinished subtest keeps the explanations of its own
  messages (a got/want it logged first) but not the timeout or exit ones: the run ended in the subtest. A run whose
  output was cut (`truncated`) or that was killed before it ended (`exitCode` -1 without `timedOut`) gets no
  explanation of its unfinished tests or of its package output, because that output is incomplete.
- **Exits.** Learner code that calls `os.Exit` or `log.Fatal` ends the test binary without a panic: no test reports a
  result, nothing prints a stack, and `go test -json` (Go 1.27) adds only `FAIL\tpkg\t0.01s` to the package output, with
  `exit status 1` before it only when the binary printed nothing at all. Of the tests that never reported a result, the
  one whose output has a printed line after its `=== RUN` line was running (a parent that waits in `t.Run` has none) and
  gets the explanation; `log.Fatal`'s date and time prefix picks the entry about `log.Fatal`. An exit in `init` or in a
  package-level variable is explained from the package output. An exit right after a goroutine dump is a watchdog
  that stopped a test that hangs (the guard of 08-ratelimit's tests): it gets the guide "How to read a stack trace" at
  the first exercise frame of the goroutine that hangs, as a test timeout does. `signal: killed` is not an exit of the
  code and gets none, and neither does output that ends inside a line, as the per-test cap of the tracked output cuts
  it. While the tests run, `os.Exit(0)` panics (`go test` passes `-test.paniconexit0`), so that test fails with a
  stack as usual; `os.Exit(0)` in `init` ends the binary as a pass without any test, which "Progress" never counts.
- **No noise.** A parent test whose own output is only its `--- FAIL` line failed because a subtest failed. It gets no
  explanation (not even the guide "How to read a failing test"); its subtests have theirs. A generic guide goes out
  at most once per run.
- **The explain object.** `explain.Explanation` in JSON: `id`, `source`, `title` (plain text: escape it like any text),
  `html` (safe HTML, ends with the links), `links`, `course` (exercise slug, optional), `generic` (true for a guide)
  and `raw` `{source, file, line, column, openable, code, test, message, captures}`, the labelled parts of the
  original message. `raw.openable` is true only when `raw.file` is a stub or test file of the exercise; the web server
  sets it, because only it knows the exercise. The UI links `raw.file:raw.line` to the editor, or shows the source
  around it, only when `openable` is true, and otherwise shows the reference as plain text: a test helper that recovers
  a runtime error reports the runtime's own line (`panic.go:115: panicked: runtime error: index out of range`), and
  the editor has no `panic.go`, `runtime_*.go` or `testing.go`. The web server calls `explain.Explain(diagnostic)` per check diagnostic, `explain.ExplainBuildOutput`
  for a `build` event (diagnostics and explanations, parallel) and `explain.ExplainTestOutput(test, output)` for each
  failed test with its collected output, the top-level test included: in `go test -json` a panic belongs to the
  top-level test, not to the subtest. `explain.Refs(output)` lists the stub and test file references for links.
  Authors follow `internal/explain/catalog/README.md`.
- **Failing assertions.** For `file_test.go:NN:` references (with `raw.openable`) the UI shows the test source around
  that line (test files are visible anyway) and explains how to read got/want, so the learner sees which case failed and why. A course
  helper that recovers a panic with `t.Helper()` reports the panic's own line in a stub (`slo.go:76: panicked: ...`);
  that is an assertion too, with `raw.file` the stub file.
- **Docs in the editor.** `go doc` is allowed by the course rules, so the editor has hover documentation and
  completion for the standard library: a build step (`cmd/gendocs`) indexes the standard library of the image's Go
  version with `go/doc` into a compressed JSON (package, exported identifiers, signature, first doc paragraph, link
  to pkg.go.dev). Monaco uses it for hover on `pkg.Name`, completion after `pkg.`, and signature help.
  - The index covers linux/amd64 (the runner's platform) and leaves out `internal`, `vendor`, `cmd`, `testdata`,
    runtime glue (`runtime/cgo`, `runtime/race`, sanitizers), `syscall/js` and the experiments `simd` and `arena`.
    `builtin` is in it, so `append`, `len` and `error` have docs too.
  - Members: funcs, types, methods (also those of interfaces), struct fields, consts and vars. Each has `name`, `kind`
    (`func`, `type`, `method`, `const`, `var`, `field`), `recv` (the owning type for methods and fields),
    `signature` (gofmt style, 1 line except small interfaces), `doc` (first paragraph, plain text) and `deprecated`
    (the text after `Deprecated:`, so a learner sees what to use instead). Types list `embeds`, so `rw.Read` on an
    `io.ReadWriter` finds `io.Reader.Read`; 2 matches at the same depth are ambiguous, as for the compiler.
  - The index is committed as `internal/godoc/data/stdlib.json.gz` (about 0.45 MB) and embedded, so the web image
    needs no GOROOT. Regenerate it with the runner's release: `GOTOOLCHAIN=go1.27.2 go generate ./internal/godoc`.
    The `go:generate` line passes `-go go1.27` and fails with another release; bump it with `Dockerfile.runner`.
  - The routes are under `/api/docs/` (see HTTP routes). A miss answers 404 with a message that teaches the usual
    cause: a package name instead of the import path ("did you mean "net/http"?"), wrong case ("Did you mean Cut?"),
    or a name that is not exported.

## Certificates

- Eligibility: a tier certificate when every exercise of that tier is `passed`; the course certificate when all 25 are `passed`. Each kind can be issued once per user (re-issuing replaces the name and the stats choice; the old ID is revoked and points to the new one).
- Claim: the user enters the name to print (default display name) and chooses whether to show stats. Stats: active time, runs, hints revealed (levels 1–3), explanations and walkthroughs revealed, solutions revealed, per exercise and in total.
- Signature: Ed25519 over the canonical payload JSON `{"id","kind","name","issuedAt","exercises":[slugs],"stats"?}`: compact, fields in this order, no HTML escaping, `issuedAt` RFC 3339 UTC in whole seconds. `stats` is `{activeSeconds, runs, hints, explanations, walkthroughs, solutions, exercises: [{slug, activeSeconds, runs, hints, explanation, walkthrough, solution}]}`. Verification accepts only canonical bytes, so the verify page shows exactly what was signed. The key pair is generated at first start and stored in `settings` (key `cert.ed25519.seed`, the 32-byte seed in base64; the first writer wins when 2 processes start at once); the public key is served at `/.well-known/go-practice-certificates.pub` (raw base64) and its fingerprint (the first 8 bytes of the SHA-256 of the raw public key, as `3F2A 91C4 0B7E D215`) is shown on verify pages and in the PDF.
- IDs: 12 random symbols (60 bits) from Crockford base32. `cert.ParseID` accepts lower case, spaces, missing hyphens and the look-alikes I, L and O. Names: 1–70 characters that the PDF font can print (Latin and Cyrillic, no CJK or emoji); `cert.CleanName` checks this before a claim.
- PDF: A4 landscape, a drawing sheet on white paper: a frame 10 mm in, a drawing area with a 5 mm construction grid that holds the name, what was passed ("Each exercise passed its full test suite with go test -race."), the tier and its exercises (or the 5 tiers and the course to scale), the activity as a 1-row title block when the owner chose it, a vector QR code to the verify URL, and the title block in the bottom right corner (name, issued, checked by, exercises, certificate ID, key fingerprint). Fonts: B612 Regular and Bold and Spline Sans Mono, converted losslessly from the site's woff2 files with fontTools, and Inter Bold (4.1, cut to Latin, Greek and Cyrillic) for a name with a letter outside B612's Latin-1 set or with parentheses, which B612 draws almost like square brackets (the whole name is then set in Inter, never 2 fonts in 1 name); each with its `OFL.txt` in `internal/cert/fonts/`. The content of the drawing area sits in its vertical middle. A name the fonts cannot print is refused with the letters it cannot print ("The PDF cannot print 漢. Use Latin, Greek or Cyrillic letters."). The PDF is built from the verified payload, the QR code and the printed URL link to the verify page, and a revoked certificate gets a 12 mm red band across the top: "Revoked. Replaced by GP-...".
- Verify page `/verify/<id>`: public, no login. Shows valid, revoked (with the replacement) or unknown; the name, kind, date, exercises, stats if chosen, signature status and the fingerprint. Links to the PDF at `/verify/<id>.pdf`.

## HTTP routes

Pages (HTML): `/` landing (public), `/login`, `/join`, `/setup`, `/recover`, `/recover/confirm`, `/verify-email`, `/learn` dashboard, `/learn/<exercise>` IDE, `/guide` (public), `/account`, `/certificates`, `/admin`, `/verify` (public lookup form; `?id=` redirects to the canonical `/verify/<id>`), `/verify/<id>`, `/verify/<id>.pdf`. Also `/healthz`, `/readyz`, `/static/...`, `/highlight.css` (the chroma CSS of `course.HighlightCSS()`, linked with the course version) and `/.well-known/go-practice-certificates.pub`. A page that needs a session redirects to `/login?next=<path>`; `next` must be a local path.

API (JSON, session required unless marked public; all mutations need the CSRF header):

| Method and path | Purpose |
|---|---|
| `POST /api/auth/register/begin` `{invite or setup, username, displayName, email}` → `{ceremony, options}` | Sign-up (public) |
| `POST /api/auth/register/finish` `{ceremony, credential}` → `{user, next}` and the session cookie | Sign-up (public) |
| `POST /api/auth/login/begin` `{conditional}` → `{ceremony, options}`; `POST /api/auth/login/finish` `{ceremony, credential, next}` → `{user, next}` | Login (public) |
| `POST /api/auth/logout` `{everywhere}` | Sign out this browser, or every session |
| `POST /api/auth/passkeys/begin` → `{ceremony, options}`, `.../finish` `{ceremony, credential, name}`; `PUT /api/auth/passkeys/<id>` `{name}`; `DELETE /api/auth/passkeys/<id>` (`<id>` is the credential ID in base64url) | Manage passkeys |
| `POST /api/auth/recover` `{email}` (public); `POST /api/auth/recover/begin` `{token}` (public); `POST /api/auth/recover/finish` `{ceremony, credential, removeOthers}` (public) | Recovery |
| `POST /api/auth/verify-email` `{token}` (public; the page posts, so a mail scanner that opens links does not use the token); `POST /api/auth/verify-email/resend` | Email verification |
| `PUT /api/account` `{displayName, email}` → `{user, email, verificationSent, pendingEmail}` | Account |
| `GET /api/exercises/<ex>` → `{exercise:{slug, number, title, tier, time, package, prev, next}, steps (each with `labelHTML`: the label with the stubs' names and the test names in `<code>`), files:[{name, kind:"stub"|"test", readonly, content, saved, updatedAt}], progress, revealed:[{level, title, html, revealedAt}], ladder:[{level, title}], gates, raceUnavailable}` | IDE bootstrap. `raceUnavailable` comes from the cached `/v1/info` answer only (no runner call), so the `-race` toggle is right before the first run; it is false while the site does not know. |
| `PUT /api/exercises/<ex>/files/<file>` `{content}` → `{savedAt}`; `POST /api/exercises/<ex>/files/<file>/reset` → `{content}` | Code |
| `GET /api/docs/pkg?path=strings` → `{path, name, synopsis, doc, deprecated, url, goVersion}`; `GET /api/docs/member?path=strings&name=Cut` (or `name=Builder.WriteString`) → `{path, package, name, kind, recv, signature, doc, deprecated, url}`; `GET /api/docs/complete?path=strings&prefix=Cu&limit=100` → `{path, prefix, items:[member], incomplete}`; `GET /api/docs/packages?prefix=net/` → `{prefix, items:[package], incomplete}`. Served by `godoc.Handler()`, with `ETag` and `Cache-Control: public, max-age=86400` | Standard library docs |
| `POST /api/exercises/<ex>/check` `{files}` → `{diagnostics:[{...diagnostic, explain}], durationMs}`; stubs the browser leaves out are sent with their saved content | Lint |
| `POST /api/exercises/<ex>/format` `{file, content}` → `{content, diagnostics}` | Format |
| `POST /api/exercises/<ex>/test` `{step}` or `{run}`, and `race` → NDJSON stream | Run |
| `POST /api/exercises/<ex>/reveal` `{level, confirm}` → `{level, title, html, revealedAt}`, 409 `{gate:"time", kind, level, minutes, needed}` or 409 `reveal_order` | Hints |
| `POST /api/exercises/<ex>/heartbeat` → `{activeSeconds}`; `PUT /api/exercises/<ex>/notes` `{notes}` | Progress |
| `GET /api/certificates` → `{kinds:[{kind, title, exercises, passed, total, eligible, certificate?}]}`; `POST /api/certificates` `{kind, name, showStats}` → 201 `{id, url, pdf, replaced}`, 403 `not_eligible` | Certificates |
| `POST /api/admin/invites` `{email?, note?, days}` → 201 `{url, mailed, mailError?, hash, createdAt, expiresAt, email, note, createdBy}` (the admin page adds the row at once); `DELETE /api/admin/invites/<hash>` (hex SHA-256 from the admin page); `PUT /api/admin/users/<id>` `{disabled?, admin?}` | Admin |

Errors are JSON `{"error": "human readable message", "code": "machine_code"}` with a fitting status, also for an unknown path under `/api/`. A refused CSRF header is 403 `csrf`, a foreign or missing Origin 403 `origin`, a missing session 401 `unauthenticated`, a non-admin 403 `forbidden`, a check replaced by a newer check of the same user 409 `superseded`, a rate limit 429 `rate_limited` with `Retry-After`. Runner failures map to 503 `runner_unavailable` (not reachable or restarting), 429 `runner_busy` with the runner's `Retry-After`, 504 `runner_timeout`, 502 `runner_auth` (the 2 tokens differ) or 502 `runner_error`; the runner's 400 codes pass through.

The test stream (`application/x-ndjson`, flushed per line, each write with a 30 s deadline) forwards the runner's events and adds:

- `{"type": "explain", "test": "<name>", "items": [explanation]}` right after a failed test, from that test's own output. A subtest whose output gives only a generic guide waits for its parent: a panic is reported on the top-level test, so the parent's specific explanation (for example "still the course stub") replaces the subtest's guide. A parent whose only failure is a failed subtest gets none. A generic guide is sent at most once per run. Explanations of the tests that never finished (go test's `-timeout` fired), of the build output and of package-level output come before `done`; the last 2 have an empty `test`.
- The runner's `done` event, with `raceUnavailable: true` also when the web server left out `-race` itself.
- `{"type": "progress", "status", "allTests", "passed", "firstPass", "passedAt", "runs", "activeSeconds", "raceUnavailable"}` after `done`: `allTests` is true for a run without filter and with `-race` (or without `-race` when the race detector is not available), `passed` for a clean run, `firstPass` when this run passed the exercise. `raceUnavailable` (left out when false) is true when the runner's race detector does not work, whatever the learner chose: the run went without `-race`, and the IDE shows "race detector not available on this server".
- `{"type": "error", "error", "code"}` when the runner stream breaks after the response started (`runner_stream`), or with `runner_busy` in place of `done` and `progress` when the run waited longer than `GPR_MAX_QUEUE_WAIT` in the runner's queue (the runner's `done` with `busy`). The run is then not counted. Before the stream starts, a full runner queue is a 429 `runner_busy` with `Retry-After`; the IDE shows the message of both the same way, so the learner sees "The test runner is busy. Try again in a few seconds." either way.

The body is `{step}` (a step ID; its `-run` and `-race` are the defaults), `{run}` (a hand-written `-run` expression, at most 512 bytes) or `{}` (every test); `race` overrides the default. A browser that leaves cancels the run at the runner. The web server asks the runner's `GET /v1/info` before a run and keeps the answer for 5 minutes (a new runner pod can land on another node). A failed fetch is tried again 10 s later, and meanwhile `-race` is assumed to work: the runner drops it itself if not, and its `done` event says so, which also updates the kept answer. A runner that was not reachable makes the next run ask again. When `raceAvailable` is false, the web server sends no `-race`, and it keeps that answer for 30 s only: a rollout can move the Service to a new pod with a race detector without a failed call, and until the site asks again its runs go without `-race` and can pass the exercise. A kept `raceAvailable: true` needs no such care, because the runner's `done` event corrects it on the next run.

## Frontend

The visual design, the copy rules and every page layout are in `.design/FINAL.md` (kept out of the repository with `.design/`; it is the design contract of the frontend). This section lists what the code must keep.

- Pages are server-rendered; JS modules enhance them. Every page works with keyboard only, has visible focus (a 2 px blue outline), meets WCAG 2.1 AA contrast, respects `prefers-reduced-motion`, and supports light and dark: the system setting, or the learner's choice ("System", "Light", "Dark" in the "Theme" menu and on the account page), stored in `localStorage` under `gpw-theme`. `js/theme-init.js` applies it before the first paint. Every page except the IDE works at 320 px without horizontal scroll.
- One CSS system: `css/site.css` (tokens as custom properties, light and dark, the components and the pages, in `@layer tokens, base, components, pages, ide`) and `css/ide.css` (the IDE only). A Go test checks that the 2 dark token blocks hold the same values. There are no shadows and no gradients except the construction grid.
- Fonts: B612 (UI and prose) and Spline Sans Mono (code and machine output), OFL 1.1, from the npm packages `@fontsource/b612@5.3.0` and `@fontsource/spline-sans-mono@5.3.0`, committed as woff2 with their licences in `web/static/fonts/`. B612 draws "(" and ")" almost like "[" and "]", so the 2 parentheses come from Inter (`@fontsource/inter@5.2.8`, OFL 1.1), cut to those 2 glyphs with pyftsubset and loaded first in the manual's stack with `unicode-range: U+0028-0029`. Code in prose and every value a tool printed is set in mono, where the parentheses are clear; tests in `internal/explain` and `internal/web` fail on code-like words (`Name()`, `x[i]`) outside code in the catalog and the templates. `scripts/fetch-fonts.sh` reproduces the files and checks the registry's sha512 integrity. Icons are 1 inline SVG sprite in the layout; every icon has a word next to it.
- Headers: the public header (landing, guide, verify, sign-in pages) and the app header (course, guide, certificates, account, admin). Below 720 px the navigation folds into a "Menu" button. The IDE has its own top bar, a title block of the exercise.
- No `prompt()`, `confirm()` or `alert()`: questions are native `<dialog>` elements (`js/ui/dialog.js`), results of actions that are not visible where the learner looks are toasts (`js/ui/toast.js`), menus are menu buttons with arrow keys (`js/ui/menu.js`). A destructive dialog starts with `WARNING:` and puts the focus on the safe button.
- Code layout: `web/web.go` embeds `web/templates` and `web/static`; with `GPW_DEV=true` the server reads them from `./web` instead (templates parsed on every request, static files with `Cache-Control: no-cache`). Templates are `layout.html` (headers, menus, the course strip, the icon sprite) plus 1 file per page in `pages/`, each parsed with the layout.
- Static URLs carry `?v=<first 12 hex of the SHA-256 of that file>`; a request with the current version is cached for a year (`immutable`), `vendor/` (Monaco, fetched by `scripts/fetch-monaco.sh`) for a day. CSS files are served with their `url(/static/...)` references rewritten to versioned URLs, and hashed after the rewrite. ES modules import each other by plain path; every page carries a server-generated import map (`<script type="importmap">`) from each module path to its versioned URL. The map is the only inline script: the page's CSP allows it by its SHA-256 (`script-src 'self' 'sha256-...'`), which is the same on every page of a deploy. In dev mode the hashes are computed again when a file changes.
- JS: `js/api.js` (the only fetch wrapper; adds `X-GPW-CSRF`), `js/webauthn.js` (base64url and `navigator.credentials`), `js/theme-init.js` (a classic script in `<head>`), `js/site.js` (menus, theme, small-screen navigation, sign-out), `js/theme.js`, `js/ui/*.js` (DOM helper, menus, dialogs, toasts, formats), `js/pages/*.js` (1 per page) and `js/ide/*.js`.
- The IDE (`/learn/<exercise>`): 3 columns from 1280 px: the task (Spec, Steps, Notes, with the hint ladder docked under the spec), your work (file tabs, Monaco, and a bottom panel with Tests and Output), and the problems column. The side columns resize (pointer and arrow keys) and hide; the sizes are kept in `localStorage`. From 900 to 1279 px the problems move into the bottom panel as a Problems tab; below 900 px the parts stack and the page scrolls. Modules: `main` (wiring, title block, status bar, keys), `monaco` (loader, the `gpw-light` and `gpw-dark` themes, editor options), `files` (models; the stub tabs, which never scroll, and 1 preview tab for a read-only test file; the Tests menu; autosave, which flushes with `keepalive` when the tab hides or closes, tries a failed save again with a growing delay and keeps unsaved code in `localStorage` until the server has it; reset), `lint` (check as you type), `problems` (1 number per problem on the end-of-line balloon, the chip, the failing tests in the tree, the file tab and the status bar; markers; F8 and Shift+F8. A run's failures are grouped by cause: 1 problem per catalog id and place, with the tests that hit it, and every panic of a course stub is 1 problem that lists the stubs left and the step to start with. The numbers follow the learner's order: the check's problems by line, then the build, then failing tests in the order of the tree. Advice of staticcheck counts as a note, in blue, not as a problem), `explain` (a problem: its source, the plain title, the anatomy of the raw message with its parts labelled, the catalog sections as disclosures that remember their state, the test code around a failing assertion, the course exercise that teaches it), `leader` (from 1280 px, a decorative line from the problem's line or failing test to its explanation; when that line or test row is out of view, it ends at the edge with a button that brings it into view), `tests` (the run and the test tree: README steps, tests, subtests, with `role="tree"`; the output of the selected failing test and its actions sit in a pane under the tree, outside it; after a reload the tree of the last run comes from `last_result`), `hints` (the ladder, folded until hint 1's gate passes, with only the next rung in full; the 2-step early reveal, whose safe choice "Keep working" has the focus; the reading sheet that opens levels 4 to 6 over your code), `panels`, `notes`, `activity`, `docs`. Hidden material reaches the IDE only through the reveal API and the bootstrap's already revealed levels.
- Course Markdown is highlighted with the site's own chroma styles (`gpw-light`, `gpw-dark`), the same 4 syntax colours as the editor.
- Emails use the same palette with inline styles and no web fonts or remote images; a notice about a loss of access starts with `WARNING:`.

## Observability

- Logs: `log/slog` JSON to stdout. One line per HTTP request (method, route pattern, status, duration, bytes, user ID; probes and static files at DEBUG), per failed runner call, per email. A request is never logged with its query string: links carry tokens.
- Metrics on `GPW_METRICS_ADDR` and `GPR_METRICS_ADDR` (`:9091` for the runner) with `prometheus/client_golang`: HTTP requests by route and status, runner requests and durations by kind and result, queue length, active sessions, users, passed exercises, issued certificates. The web server's names: `gpw_http_requests_total{route,method,status}`, `gpw_http_request_duration_seconds{route}`, `gpw_runner_requests_total{kind,result}`, `gpw_runner_request_duration_seconds{kind,result}`, and the gauges `gpw_users`, `gpw_active_sessions`, `gpw_passed_exercises`, `gpw_certificates`, read from the database at scrape time; plus the Go and process collectors.
- `/healthz`: process alive. `/readyz`: database read and write (a row in a probe table) and the course loaded. The web readiness does not depend on the runner; the IDE shows a clear message when the runner is down. `/readyz` is reachable through the public route, so the probe runs at most once per 2 s, and detached from the request: a client that hangs up cannot leave a cached failure for the kubelet.
- The HTTP server reads headers within 10 s and the request within 30 s, accepts at most 64 KiB of headers and gives every response 2 minutes to be written; a test stream moves that deadline 30 s ahead before each event instead.

## Testing

- Unit tests per package. `internal/runner` tests use the real course submodule and the local Go toolchain: a stub fails, a reference solution passes, go test's own `-timeout` prints the goroutines, the runner's timeout kills the process group, a run that waits too long in the queue ends `busy`, file-name validation, output cap. The race probe runs for real in every test server; a fake `go` command that fails every `-race` invocation like ThreadSanitizer on Talos (and runs the real `go` otherwise) checks the fallback.
- `internal/auth` tests use a software authenticator, `internal/auth/authtest` (ES256, "none" attestation, CBOR via fxamacker/cbor, a controllable sign counter), against a real store with a fake clock.
- `internal/web` tests use `httptest` with a fake runner (an `httptest.Server` that speaks the runner API) and `authtest` for real sign-ups and logins through the API.
- E2E (`e2e/`, Playwright, Chromium): with a CDP virtual authenticator: setup link, sign up, sign out, sign in, open 01-logfmt, type the reference solution, run all tests (passes), reveal hint 1 with the soft gate, claim nothing yet; an admin creates an invite and a second user joins; recovery by email (log sender) adds a new passkey; a verify page for a seeded certificate renders and the PDF downloads. `playwright.config.js` starts `scripts/dev.sh` in a fresh state directory (`GO_PRACTICE_DEV_STATE`) and reads the setup link and the mails from its `web.log`; with `E2E_BASE_URL` set it tests a running site instead. `tests/smoke.spec.js` covers the first part of the list (to "claim nothing yet") and fails on any page error or CSP violation. `tests/screenshots.spec.js` runs only with `SCREENSHOTS_DIR` set (`SCREENSHOTS_DIR=$PWD/../.design/build npx playwright test screenshots`): it captures every page in light and dark at 1440x900 and 390x844, passes tier 1 with the reference solutions for a real certificate, and replaces the text of a revealed hint before its screenshot. Chrome's virtual authenticator answers a passkey autofill request at once, so the smoke test signs in through autofill and then, with autofill switched off, through the button.
- CI runs `go vet`, `go test -race ./...`, `golangci-lint`, the E2E suite and builds both images.

## Build and release

- `Dockerfile.runner`: `golang:1.27-bookworm`, installs `staticcheck` (pinned), copies the course, pre-warms the build cache (`go test -race -run '^$'` and `go vet` over all exercises) into `/opt/gocache-seed`, runs as 65532.
- `Dockerfile.web`: build stage with Go and the Monaco tarball from the npm registry (`scripts/fetch-monaco.sh`, checked against a pinned SHA-256); final stage `gcr.io/distroless/static-debian12:nonroot` with the binary, templates and static files embedded (`embed.FS`) and the course copied to `/course` (`README.md`, `go.mod`, `exercises/`, `solutions/`; without git metadata, so `course.Version()` is the content digest).
- `scripts/dev.sh` builds both binaries and runs them on `127.0.0.1` (site `:8080`, runner `:8081`, metrics `:9090` and `:9091`) with a random 64-character token, `GPW_DEV=true`, a state directory for the database, runs and logs, and a runner build cache in `~/.cache/go-practice-dev` that outlives it. Its own settings are `GO_PRACTICE_DEV_STATE`, `GO_PRACTICE_DEV_PORT` and `GO_PRACTICE_DEV_CACHE`: the servers refuse unknown `GPW_` and `GPR_` variables. It prints the setup link.
- The web server shuts down within 25 s of SIGTERM (a test stream still running is cut and the runner kills its run), waits for the emails in flight, and closes the database. It backs up the database 10 minutes after start and then daily, and cleans expired sessions, ceremonies and email tokens hourly.
- `.dockerignore` keeps `.git`, `.design/`, `e2e/` and a local `web/static/vendor/` out of the build context, so `Dockerfile.web` copies all of `web/static/` and the fetched Monaco stays.
- GitHub Actions (`.github/workflows/ci.yml`), on pull requests, pushes to `main` and tags, with Go 1.27.2, the release of the images and of the explain fixtures:
  - `check`: gofmt (without `testdata/`), `go vet`, golangci-lint v2.15.0 and `go test -race ./...` with staticcheck v0.8.2 required. Both tools are built from source by that Go release, because they load its standard library.
  - `e2e`: `scripts/fetch-monaco.sh` and the Playwright suite against `scripts/dev.sh`, in Chromium.
  - `images`: on a tag `vX.Y.Z`, after the 2 jobs above pass, build and push `ghcr.io/yornik/go-practice-web:vX.Y.Z` and `ghcr.io/yornik/go-practice-runner:vX.Y.Z` (linux/amd64). Only the exact tag: the gitops repository pins it, and Renovate bumps it.

## Deployment (sharkshere-gitops)

One ArgoCD Application `go-practice` with `path: manifests/go-practice`. Two namespaces:

- `go-practice`: web Deployment (1 replica, `Recreate`), Service `go-practice-web` (`:8080` for the site, `:9090` for the ServiceMonitor only; no ExternalDNS annotations), IngressRoute (websecure, `letsencrypt`) plus the `web` redirect route, middlewares (the shared `security-headers`, `rate-limit` and `no-compression` of the `traefik` namespace, the last because a compressed NDJSON stream holds test events back; its own `redirect-https`, and `fail2ban-go-practice` on the `/api/auth/` route only), PVC `go-practice-data-ssd` (`truenas-ssd`, 2Gi), ksops Secret (runner token, SMTP password), ServiceMonitor, NetworkPolicies (default deny; ingress to `:8080` from Traefik and to `:9090` from monitoring; egress to DNS, the runner's `:8081` and SMTP on `:465`).
- `go-practice-runner`: runner Deployment, Service `runner` (cluster internal, `:8081` and `:9091`), ksops Secret (runner token), NetworkPolicies, ServiceMonitor. Its own namespace keeps arbitrary code away from the web Secrets, like `gitlab-ci`.

DNS: the site is on the zone apex, where a CNAME is not valid, and the ExternalDNS annotations that the subdomains of the estate use publish a CNAME to the jump host. So the A and AAAA records of `go-learn.party` come from a dummy Service, `dns-go-learn-party` in `manifests/external-dns/dns-records.yaml`, whose `external-dns.kubernetes.io/target` lists the jump hosts' addresses; the web Service has no DNS annotations. There is no `www` record: the site accepts 1 origin, the WebAuthn relying party. CronJobs in the same directory publish a reject-all SPF and DMARC policy (the site mails from `yornik.eu`) and a CAA record for Let's Encrypt.

Probes: the web pod's liveness probe is `/healthz` and its readiness probe `/readyz`, both on the site port (`http`, 8080). `/readyz` is cheap: it runs the database probe at most once per 2 s and answers from the cached result in between (see "Observability"), and it does not depend on the runner. The runner's startup and liveness probes are `/healthz` and its readiness probe `/readyz`, on `:8081`; it stays ready without a race detector.

Timeouts: `GPR_MAX_QUEUE_WAIT` plus `GPR_TEST_TIMEOUT` stays under the 60 s timeout of Traefik's entry points; the defaults (12 s and 40 s) leave 8 s. The runner's `terminationGracePeriodSeconds` is at least `GPR_TEST_TIMEOUT` plus 15 s.

Manifests follow the repository's `AGENTS.md`, `REVIEW.md` checklist B and `docs/styleguide.md`.
