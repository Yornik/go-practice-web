// hints.js: the hint ladder, docked under the spec. 6 rungs in the
// course's order: hint 1 to 3, the explanation, the walkthrough and the
// solution. The server keeps the order and the soft time gates (25
// minutes of active time before hint 1, 10 minutes between levels); the
// ladder shows the numbers, and an early reveal takes 2 steps. Levels 4
// to 6 open in the reading sheet, which covers your code on purpose.
//
// The spec comes first: the ladder starts folded until hint 1's gate has
// passed, and it shows only the next rung in full. Every rung keeps its
// buttons from the start and only their text and state change, so the
// countdown never takes the keyboard focus away.
import { api, ApiError, errorMessage } from "../api.js";
import { h, load, save, reducedMotion } from "../ui/dom.js";
import { clock } from "../ui/format.js";

const NAMES = { 1: "hint 1", 2: "hint 2", 3: "hint 3", 4: "the explanation", 5: "the walkthrough", 6: "the solution" };
const FOLD_KEY = "gpw-ladder-folded";

export class Hints {
  /**
   * @param {object} o
   * @param {string} o.slug
   * @param {string} o.number "07"
   * @param {HTMLElement} o.ladder
   * @param {{level: number, title: string, html: string, revealedAt: string}[]} o.revealed
   * @param {{firstHintMinutes: number, betweenHintMinutes: number}} o.gates
   * @param {boolean} o.passed
   * @param {number} o.activeSeconds
   * @param {(level: number) => void} o.onReveal
   */
  constructor(o) {
    Object.assign(this, o);
    this.paused = false;
    this.levels = new Map();
    for (const li of this.ladder.querySelectorAll("[data-level]")) {
      const level = Number(li.dataset.level);
      const l = {
        li, level, title: li.dataset.title, after: li.dataset.after,
        balloon: li.querySelector("[data-rung-balloon]"),
        meta: li.querySelector("[data-rung-meta]"),
        gate: li.querySelector("[data-rung-gate]"),
        body: li.querySelector("[data-hint-body]"),
        actions: li.querySelector("[data-rung-actions]"),
        revealed: null, // {at, html}
        confirming: null, // the gate numbers while the 2nd step shows
        busy: false,
        error: "",
      };
      this.build(l);
      this.levels.set(level, l);
    }
    this.rest = this.ladder.querySelector("[data-rung-rest]");
    for (const r of this.revealed) this.store(r);
    this.foldButton = this.ladder.querySelector("[data-ladder-fold]");
    this.sub = this.ladder.querySelector("[data-ladder-sub]");
    this.passedNote = this.ladder.querySelector("[data-ladder-passed]");
    this.foldButton.addEventListener("click", () => this.fold(!this.ladder.classList.contains("is-folded")));
    this.lastReveal = 0;
    // Folded until hint 1 can open without a gate, unless the learner chose.
    const stored = load(FOLD_KEY);
    this.fold(stored === null ? this.highest() === 0 && this.gate(1) !== null : stored === "1", false);
    this.render();
    setInterval(() => this.render(), 30_000);
  }

  /** The parts of a rung that change: made once, then only updated. */
  build(l) {
    l.gateText = h("p", { class: "rung-gate-text" });
    l.ruler = h("div", { class: "ruler", role: "img", hidden: true });
    l.rule = h("p", { class: "rung-note" }, "Active time counts while this tab is in front. It pauses after 10 min without a key, a click or a scroll.");
    l.confirmText = h("p", {});
    l.keep = h("button", { type: "button", class: "btn btn-sm", onclick: () => this.keepWorking(l) }, "Keep working");
    l.early = h("button", { type: "button", class: "btn btn-sm btn-quiet", "data-confirm-reveal": "", onclick: () => this.reveal(l.level, true) }, "Reveal it");
    l.confirm = h("div", { class: "confirm", role: "group", "aria-label": `Reveal ${NAMES[l.level]} early`, hidden: true }, l.confirmText, h("div", { class: "btn-row" }, l.keep, l.early));
    l.button = h("button", { type: "button", class: "btn btn-sm", hidden: true, onclick: () => this.primary(l) });
    l.errorEl = h("p", { class: "field-error", role: "alert", hidden: true });
    l.gate.append(l.gateText, l.ruler, l.rule, l.confirm);
    l.actions.append(l.button, l.errorEl);
  }

  store(r) {
    const l = this.levels.get(r.level);
    if (!l) return;
    l.revealed = { at: new Date(r.revealedAt || Date.now()), html: r.html };
    if (r.level <= 3) l.body.innerHTML = r.html; // course material rendered by the server
  }

  /** New active time from the heartbeat; paused when the tab is not in front. */
  setActive(seconds, { paused = false } = {}) {
    this.activeSeconds = seconds;
    this.paused = paused;
    this.render();
  }

  setPassed(passed) {
    this.passed = passed;
    this.render();
  }

  /** The highest revealed level, 0 for none. */
  highest() {
    let n = 0;
    for (const [level, l] of this.levels) if (l.revealed) n = Math.max(n, level);
    return n;
  }

  /** The gate of the next level: {have, needed, kind} or null when it is open. */
  gate(level) {
    if (this.passed) return null;
    if (level === 1) {
      const have = Math.floor((this.activeSeconds || 0) / 60);
      const needed = this.gates.firstHintMinutes;
      return have >= needed ? null : { kind: "active", have, needed };
    }
    const prev = this.levels.get(level - 1)?.revealed;
    if (!prev) return null;
    const since = Math.floor((Date.now() - prev.at.getTime()) / 60000);
    const needed = this.gates.betweenHintMinutes;
    return since >= needed ? null : { kind: "since", have: since, needed, at: prev.at };
  }

  render() {
    const next = this.highest() + 1;
    this.passedNote.hidden = !this.passed;
    const locked = [];
    for (const [level, l] of this.levels) {
      // Hidden parts keep no text, so nothing stale is read or found.
      if (!l.confirming || l.revealed) l.confirmText.textContent = "";
      if (l.revealed || level !== next) l.gateText.textContent = "";
      l.errorEl.hidden = !l.error;
      l.errorEl.textContent = l.error;
      l.button.setAttribute("aria-disabled", String(l.busy));
      if (l.revealed) {
        this.state(l, "revealed");
        l.li.hidden = false;
        l.meta.textContent = `revealed at ${clock(l.revealed.at)}`;
        l.gate.hidden = true;
        l.button.hidden = level < 4;
        l.button.textContent = "Read it";
        continue;
      }
      if (level !== next) {
        this.state(l, "locked");
        l.li.hidden = true;
        l.meta.textContent = l.after;
        l.gate.hidden = true;
        l.button.hidden = true;
        locked.push(level);
        continue;
      }
      this.state(l, "next");
      l.li.hidden = false;
      l.meta.textContent = "";
      const g = this.gate(level);
      const name = NAMES[level];
      l.gate.hidden = false;
      l.confirm.hidden = !l.confirming;
      if (l.confirming) {
        const c = l.confirming;
        l.confirmText.textContent = c.kind === "active"
          ? `The course asks you to try for ${c.needed} minutes of active time before hint 1. You have ${minutesText(c.have)}.`
          : `The course asks you to wait ${c.needed} minutes between hints. You revealed ${NAMES[level - 1]} ${sinceText(c)}.`;
      }
      if (!g) l.gateText.textContent = "";
      l.gateText.hidden = !g || !!l.confirming;
      l.ruler.hidden = !g || !!l.confirming;
      l.rule.hidden = !g || g.kind !== "active" || !!l.confirming;
      if (g) {
        l.gateText.textContent = g.kind === "active"
          ? `You can reveal it after ${g.needed} min of active time. You have ${g.have} min${this.paused ? ", and the count is paused" : ""}.`
          : `You can reveal it in ${g.needed - g.have} min.`;
        updateRuler(l.ruler, g);
      }
      l.button.hidden = !!l.confirming;
      l.button.textContent = g ? `Reveal ${name} now` : `Reveal ${name}`;
    }
    if (this.rest) {
      this.rest.hidden = locked.length === 0;
      this.rest.textContent = locked.length ? `Then ${listText(locked.map((n) => NAMES[n]))}, 1 at a time.` : "";
    }
    this.renderSummary();
  }

  state(l, s) {
    l.li.dataset.state = s;
    l.balloon.className = `balloon${s === "revealed" ? " is-filled" : s === "locked" ? " is-locked" : ""}`;
    l.balloon.setAttribute("aria-label", `${l.title}: ${s === "revealed" ? "revealed" : s === "next" ? "next" : "locked"}`);
  }

  /** The rung's button: reveal, ask first, or read. */
  primary(l) {
    if (l.busy) return;
    if (l.revealed) {
      this.read(l.level);
      return;
    }
    const g = this.gate(l.level);
    if (g) this.askEarly(l, g);
    else this.reveal(l.level, false);
  }

  /** The 2nd step of an early reveal, inside the rung. The safe choice has the focus. */
  askEarly(l, g) {
    l.confirming = g;
    l.error = "";
    this.render();
    l.keep.focus();
  }

  keepWorking(l) {
    l.confirming = null;
    this.render();
    l.button.focus();
  }

  async reveal(level, confirm) {
    const l = this.levels.get(level);
    if (l.busy) return;
    l.busy = true;
    l.error = "";
    l.early.setAttribute("aria-disabled", "true");
    this.render();
    try {
      const res = await api("POST", `/api/exercises/${this.slug}/reveal`, { level, confirm });
      l.confirming = null;
      this.store(res);
      this.lastReveal = Date.now();
      if (level <= 3 && !reducedMotion()) {
        l.li.classList.add("is-opening");
        setTimeout(() => l.li.classList.remove("is-opening"), 400);
      }
      l.busy = false;
      this.render();
      this.onReveal?.(level);
      if (level <= 3) l.body.focus();
      else this.read(level);
    } catch (err) {
      l.busy = false;
      if (err instanceof ApiError && err.status === 409 && err.body.gate) {
        l.confirming = { kind: err.body.kind, have: err.body.minutes, needed: err.body.needed, at: this.levels.get(level - 1)?.revealed?.at };
        this.render();
        l.keep.focus();
      } else {
        l.error = err instanceof ApiError && err.code === "reveal_order" ? `Reveal ${NAMES[level - 1]} first. The ladder opens 1 level at a time.` : errorMessage(err);
        this.render();
        (l.confirming ? l.keep : l.button).focus();
      }
    } finally {
      l.early.removeAttribute("aria-disabled");
    }
  }

  fold(on, remember = true) {
    this.ladder.classList.toggle("is-folded", on);
    this.foldButton.textContent = on ? "Unfold" : "Fold";
    this.foldButton.setAttribute("aria-expanded", String(!on));
    if (remember) save(FOLD_KEY, on ? "1" : "0");
    this.renderSummary();
  }

  /** The editor got the focus: fold the ladder 1 minute after a reveal. */
  editorFocused() {
    if (this.lastReveal && Date.now() - this.lastReveal >= 60_000 && !this.ladder.classList.contains("is-folded")) {
      this.lastReveal = 0;
      this.fold(true);
    }
  }

  renderSummary() {
    if (!this.sub) return;
    if (!this.ladder.classList.contains("is-folded")) {
      this.sub.textContent = "Reveal 1 level at a time.";
      return;
    }
    const top = this.highest();
    const parts = [top ? `${capital(NAMES[top])} revealed.` : "Nothing revealed."];
    const next = top + 1;
    if (next <= 6) {
      const g = this.gate(next);
      if (!g) parts.push(`${capital(NAMES[next])} is ready.`);
      else if (g.kind === "active") parts.push(`Hint 1 after ${g.needed - g.have} more min of active time.`);
      else parts.push(`${capital(NAMES[next])} in ${g.needed - g.have} min.`);
    }
    this.sub.textContent = parts.join(" ");
  }

  /** Opens level 4, 5 or 6 in the reading sheet. */
  read(level) {
    const l = this.levels.get(level);
    if (!l?.revealed) return;
    const body = h("div", { class: "reading-body" });
    if (level === 6) body.append(h("p", { class: "callout" }, h("b", {}, "NOTE:"), " Rule 4. Read it, close it, wait, and write your own version from memory."));
    const content = h("div", {});
    content.innerHTML = l.revealed.html; // course material rendered by the server
    body.append(...content.childNodes);
    const close = () => dialog.close();
    const title = `${l.title}, exercise ${this.number}`;
    const dialog = h("dialog", { class: "reading-sheet", "aria-labelledby": "reading-title" },
      h("div", { class: "reading-head" },
        h("h2", { id: "reading-title" }, title),
        h("button", { type: "button", class: "btn btn-primary", onclick: close }, "Close and write")),
      h("div", { class: "reading-inner" }, body,
        h("p", { class: "mt-6" }, h("button", { type: "button", class: "btn btn-primary", onclick: close }, "Close and write"))));
    const task = document.getElementById("task-col");
    const right = task && !task.hidden && innerWidth >= 900 ? task.getBoundingClientRect().right : 0;
    dialog.style.setProperty("--sheet-w", `${innerWidth - right}px`);
    // The rung's own button, not whatever had the focus: it outlives renders.
    const opener = l.button;
    dialog.addEventListener("close", () => {
      dialog.remove();
      opener.focus();
    });
    document.body.append(dialog);
    dialog.showModal();
    dialog.querySelector(".reading-head .btn").focus();
  }
}

function updateRuler(el, g) {
  const ticks = g.needed;
  el.setAttribute("aria-label", `${g.have} of ${g.needed} minutes have passed`);
  el.style.setProperty("--elapsed", `${Math.min(100, (g.have / g.needed) * 100)}%`);
  if (el.childElementCount !== ticks) {
    el.replaceChildren(...Array.from({ length: ticks }, (_, i) => h("i", { class: ticks > 10 && i % 5 === 0 ? "long" : null })));
  }
}

function minutesText(n) {
  return n === 1 ? "1 minute" : `${n} minutes`;
}

/** "at 12:40, less than 1 minute ago" or "at 12:40, 4 minutes ago". */
function sinceText(g) {
  const ago = g.have < 1 ? "less than 1 minute ago" : `${minutesText(g.have)} ago`;
  return g.at ? `at ${clock(g.at)}, ${ago}` : ago;
}

function listText(items) {
  if (items.length <= 1) return items.join("");
  return `${items.slice(0, -1).join(", ")} and ${items[items.length - 1]}`;
}

function capital(s) {
  const t = s.replace(/^the /, "");
  return t.charAt(0).toUpperCase() + t.slice(1);
}
