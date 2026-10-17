// hints.js: the hint ladder. The server keeps the order and the soft time
// gates; a gate answers 409 and the learner confirms to go on.
import { api, ApiError, errorMessage } from "../api.js";

export class Hints {
  /**
   * @param {object} o
   * @param {string} o.slug
   * @param {HTMLElement} o.ladder the <ol> with 1 <li data-level> per level
   * @param {HTMLElement} o.gate the confirmation box
   * @param {{level: number, html: string}[]} o.revealed
   */
  constructor({ slug, ladder, gate, revealed }) {
    this.slug = slug;
    this.gate = gate;
    this.levels = new Map();
    for (const li of ladder.querySelectorAll("[data-level]")) {
      const level = Number(li.dataset.level);
      const button = li.querySelector("[data-reveal]");
      button.addEventListener("click", () => this.reveal(level, false));
      this.levels.set(level, { li, button, body: li.querySelector("[data-hint-body]"), revealed: false });
    }
    for (const r of revealed) this.show(r);
    this.gate.querySelector("[data-gate-cancel]").addEventListener("click", () => this.closeGate());
    this.gate.querySelector("[data-gate-confirm]").addEventListener("click", () => {
      const level = Number(this.gate.dataset.level);
      this.closeGate();
      this.reveal(level, true);
    });
    this.update();
  }

  show(r) {
    const l = this.levels.get(r.level);
    if (!l) return;
    l.revealed = true;
    l.body.innerHTML = r.html; // course material rendered by the server
    l.li.dataset.state = "revealed";
  }

  /** Shows the reveal button on the next level only. */
  update() {
    let nextFound = false;
    for (const [, l] of [...this.levels].sort((a, b) => a[0] - b[0])) {
      if (l.revealed) {
        l.button.hidden = true;
        continue;
      }
      l.button.hidden = nextFound;
      l.li.dataset.state = nextFound ? "locked" : "next";
      nextFound = true;
    }
  }

  async reveal(level, confirm) {
    const l = this.levels.get(level);
    l.button.disabled = true;
    try {
      const res = await api("POST", `/api/exercises/${this.slug}/reveal`, { level, confirm });
      this.show(res);
      this.update();
      l.body.focus?.();
    } catch (err) {
      if (err instanceof ApiError && err.status === 409 && err.body.gate) {
        this.openGate(level, err.body);
      } else {
        alert(errorMessage(err));
      }
    } finally {
      l.button.disabled = false;
    }
  }

  openGate(level, g) {
    const text =
      g.kind === "active"
        ? `You have worked on this exercise for ${g.minutes} ${g.minutes === 1 ? "minute" : "minutes"}. The course asks you to try for ${g.needed} minutes before hint 1. Struggling a little longer is where the learning happens.`
        : `Your last reveal was ${g.minutes} ${g.minutes === 1 ? "minute" : "minutes"} ago. The course asks you to wait ${g.needed} minutes between hints and try with what you know.`;
    this.gate.querySelector("[data-gate-text]").textContent = text;
    this.gate.dataset.level = String(level);
    this.gate.hidden = false;
    this.gate.querySelector("[data-gate-cancel]").focus();
  }

  closeGate() {
    this.gate.hidden = true;
  }
}
