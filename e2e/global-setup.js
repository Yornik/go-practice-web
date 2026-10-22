// Playwright's webServer waits only for the web server's /readyz, which
// does not depend on the runner. The runner listens after its race probe,
// which compiles the standard library with -race on a cold cache: a minute
// or more on a CI machine. A check or a run before that fails, so the
// tests wait for the runner of scripts/dev.sh too.
// scripts/dev.sh puts the runner on the port after the site's.
const RUNNER = `http://127.0.0.1:${Number(process.env.GO_PRACTICE_DEV_PORT || 8080) + 1}/readyz`;

export default async function globalSetup() {
  // A site that is already running has its runner out of reach.
  if (process.env.E2E_BASE_URL) return;
  const deadline = Date.now() + 300_000;
  for (;;) {
    try {
      const res = await fetch(RUNNER);
      if (res.ok) return;
    } catch {
      // Not listening yet.
    }
    if (Date.now() > deadline) throw new Error(`the runner is not ready after 300 s: ${RUNNER}`);
    await new Promise((r) => setTimeout(r, 500));
  }
}
