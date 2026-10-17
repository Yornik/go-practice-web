# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repository is

The web IDE for the go-practice course: passkey accounts, a Monaco editor with lint markers, real `go test -race` runs on a sandboxed runner, a hint ladder, progress tracking and signed PDF certificates. `docs/DESIGN.md` is the contract between the components (config, runner API, data model, routes, security rules). Read it before you change behaviour, and update it in the same change.

## Commands

```bash
go vet ./...
go test -race ./...                 # unit and integration tests; the runner tests use course/ and the local Go toolchain
golangci-lint run ./...
scripts/fetch-monaco.sh             # vendors Monaco into web/static/vendor/monaco/ (needed to run the site locally)
scripts/dev.sh                      # runs runner and web locally on http://localhost:8080 with GPW_DEV=true
(cd e2e && npm ci && npx playwright test)   # end-to-end tests against scripts/dev.sh
```

## Rules

- `course/` is a git submodule of github.com/Yornik/go-practice. Do not edit it here. Bump it with `git submodule update --remote course`.
- Hidden course material (hints, explanations, walkthroughs, solutions) reaches a browser only through the reveal API, after the previous level was revealed. Do not embed it in pages or bootstrap JSON.
- The runner accepts only the exercise's stub file names. Keep that check; it blocks path traversal and test replacement.
- Every state-changing endpoint needs the CSRF header and the Origin check described in `docs/DESIGN.md`.
- Comments give the reason, not the description. Docs use short sentences, the active voice and numbers as digits.
- Commit messages and pull request titles follow Conventional Commits (`feat:`, `fix:`, `docs:`, `chore:`): Release Please reads them to choose the next version and to write `CHANGELOG.md`. Do not push `v*` tags by hand; merging the release pull request creates them (see "Build and release" in `docs/DESIGN.md`).
