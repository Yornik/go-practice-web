#!/usr/bin/env bash
# Builds and runs the runner and the web server locally:
# http://localhost:8080 with GPW_DEV=true, a fresh data directory and a
# random runner token. Mail goes to the web log. Ctrl+C stops both.
#
# Environment (not GPW_ or GPR_: the servers refuse unknown variables with
# their prefixes):
#   GO_PRACTICE_DEV_STATE  directory for the database, logs and runs; default
#                          a new temporary directory. Reuse 1 to keep your
#                          account.
#   GO_PRACTICE_DEV_PORT   port of the site; default 8080.
#   GO_PRACTICE_DEV_CACHE  build cache of the runner; default
#                          ~/.cache/go-practice-dev, kept between starts.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

state="${GO_PRACTICE_DEV_STATE:-$(mktemp -d "${TMPDIR:-/tmp}/go-practice-dev.XXXXXX")}"
port="${GO_PRACTICE_DEV_PORT:-8080}"
mkdir -p "$state/bin" "$state/data" "$state/runs"
# The build cache outlives the state directory, so a second start is fast.
cache="${GO_PRACTICE_DEV_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/go-practice-dev}"
mkdir -p "$cache"

if [[ ! -f web/static/vendor/monaco/vs/loader.js ]]; then
	scripts/fetch-monaco.sh
fi
if [[ ! -f course/README.md ]]; then
	echo "course/ is empty: run git submodule update --init" >&2
	exit 1
fi

echo "Building into $state/bin"
go build -o "$state/bin/runner" ./cmd/runner
go build -o "$state/bin/web" ./cmd/web

token="$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"

pids=()
stop() {
	trap - INT TERM EXIT
	for pid in ${pids[@]+"${pids[@]}"}; do
		kill "$pid" 2>/dev/null || true
	done
	wait 2>/dev/null || true
}
trap stop INT TERM EXIT

# staticcheck is optional; go install puts it in GOPATH/bin.
PATH="$(go env GOPATH)/bin:$PATH" \
GPR_ADDR="127.0.0.1:8081" \
GPR_METRICS_ADDR="127.0.0.1:9091" \
GPR_TOKEN="$token" \
GPR_COURSE_DIR="$root/course" \
GPR_WORK_DIR="$state/runs" \
GPR_GOCACHE="$cache/go-build" \
GPR_GOCACHE_SEED="" \
	"$state/bin/runner" >"$state/runner.log" 2>&1 &
pids+=($!)

GPW_ADDR="127.0.0.1:$port" \
GPW_METRICS_ADDR="127.0.0.1:9090" \
GPW_PUBLIC_URL="http://localhost:$port" \
GPW_DB_PATH="$state/data/go-practice.db" \
GPW_COURSE_DIR="$root/course" \
GPW_RUNNER_URL="http://127.0.0.1:8081" \
GPW_RUNNER_TOKEN="$token" \
GPW_TRUSTED_PROXIES="" \
GPW_DEV="true" \
	"$state/bin/web" >"$state/web.log" 2>&1 &
pids+=($!)

for _ in $(seq 1 100); do
	if curl -fsS "http://127.0.0.1:$port/readyz" >/dev/null 2>&1; then
		break
	fi
	for pid in "${pids[@]}"; do
		if ! kill -0 "$pid" 2>/dev/null; then
			echo "A server stopped. Logs:" >&2
			tail -n 20 "$state/runner.log" "$state/web.log" >&2
			exit 1
		fi
	done
	sleep 0.2
done

echo
echo "Go Practice runs on http://localhost:$port"
echo "State and logs: $state (web.log, runner.log)"
link="$(grep -o 'setup link: [^"]*' "$state/web.log" | tail -n 1 | sed 's/^setup link: //' || true)"
if [[ -n "$link" ]]; then
	echo "Setup link (creates the admin account): $link"
else
	echo "The database has an account already; sign in at http://localhost:$port/login"
fi
echo "Emails are written to web.log. Ctrl+C stops both servers."
# Portable "wait for the first to exit": macOS ships bash 3.2 without wait -n.
while kill -0 "${pids[0]}" 2>/dev/null && kill -0 "${pids[1]}" 2>/dev/null; do
	sleep 1
done
echo "A server stopped; see the logs in $state." >&2
exit 1
