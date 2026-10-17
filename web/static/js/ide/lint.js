// lint.js: the check as you type. 1.2 s after the last edit the stub
// files go to check; the diagnostics become problems. A newer edit
// cancels the request in flight.
import { api, errorMessage } from "../api.js";

const DELAY_MS = 1200;

export class Linter {
  /**
   * @param {object} o
   * @param {import("./files.js").Files} o.files
   * @param {string} o.slug
   * @param {import("./problems.js").Problems} o.problems
   */
  constructor({ files, slug, problems }) {
    Object.assign(this, { files, slug, problems });
    this.timer = 0;
    this.inflight = null;
  }

  /** Starts the delay again; called on every edit. */
  schedule() {
    clearTimeout(this.timer);
    this.inflight?.abort();
    this.timer = setTimeout(() => this.run(), DELAY_MS);
  }

  async run() {
    this.inflight?.abort();
    const ctl = new AbortController();
    this.inflight = ctl;
    this.problems.setChecking(true);
    try {
      const res = await api("POST", `/api/exercises/${this.slug}/check`, { files: this.files.stubContents() }, { signal: ctl.signal });
      if (ctl.signal.aborted) return;
      this.problems.setCheck(res.diagnostics || []);
    } catch (err) {
      if (err.name === "AbortError" || err.code === "superseded") return;
      this.problems.setChecking(false, errorMessage(err));
      return;
    } finally {
      if (this.inflight === ctl) this.inflight = null;
    }
    this.problems.setChecking(false);
  }
}
