// hush.js: HUSH's refused offer (VESPER.md 6.22, 16.7). The card is static
// text in the page: this module only decides when it shows. It never
// calls the network, never takes the focus on its own, is not a live
// region and has no accept action. It shows only when all 7 conditions
// hold:
//
//  1. story effects are on;
//  2. the exercise has never passed;
//  3. the last run finished with a failing test, a build failure, a panic
//     or a timeout (a runner error does not count);
//  4. the active time in the exercise is at least hint 1's gate (25 min);
//  5. the card has not shown for this exercise before (gpw-hush-seen);
//  6. no run is in progress;
//  7. no keystroke in the editor for 5 s.
//
// So it comes at most once per exercise, to a learner who struggles past
// 25 minutes, at the moment Noor's notes open.
import { load, save } from "../ui/dom.js";
import { setStory, storyOn } from "../theme.js";

const SEEN_KEY = "gpw-hush-seen";
const SEEN_MAX = 50;
const QUIET_MS = 5000;

export class Hush {
  /**
   * @param {object} o
   * @param {string} o.slug
   * @param {HTMLElement} o.card the section in the problems scroll area
   * @param {HTMLElement} o.link "A note from HUSH" in the Problems header
   * @param {HTMLElement} o.heading the Problems heading, where the focus goes after Dismiss
   * @param {boolean} o.passed
   * @param {number} o.activeSeconds
   * @param {number} o.gateMinutes hint 1's gate
   */
  constructor({ slug, card, link, heading, passed, activeSeconds, gateMinutes }) {
    Object.assign(this, { slug, card, link, heading, passed, activeSeconds, gateMinutes });
    this.result = null; // how the last run ended: {finished, failing}
    this.running = false;
    this.lastKey = 0;
    this.timer = 0;
    this.shown = false;
    card.querySelector("[data-hush-dismiss]")?.addEventListener("click", () => this.dismiss());
    card.querySelector("[data-hush-off]")?.addEventListener("click", () => {
      setStory(false);
      heading.focus();
    });
    // Story effects turned on again can complete the conditions.
    document.addEventListener("gpw:story", () => this.check());
    // The test hook of the e2e screenshots (VESPER.md 14.3): the card shows
    // at once, without its conditions, and this browser does not count it.
    document.addEventListener("gpw:hush-show", () => this.show(false));
  }

  setActive(seconds) {
    this.activeSeconds = seconds;
    this.check();
  }

  /** The exercise passed: the card never comes now. One that shows stays. */
  setPassed() {
    this.passed = true;
    clearTimeout(this.timer);
  }

  /** A keystroke in the editor: the card waits for 5 s of quiet. */
  typed() {
    this.lastKey = Date.now();
    if (this.timer) this.check();
  }

  /** A run starts: it cancels a wait. */
  runStarted() {
    this.running = true;
    clearTimeout(this.timer);
    this.timer = 0;
  }

  /** A run ended, or the page restored the last one. */
  runFinished(result) {
    this.running = false;
    this.result = result;
    this.check();
  }

  seen() {
    try {
      const list = JSON.parse(load(SEEN_KEY) || "[]");
      return Array.isArray(list) ? list : [];
    } catch {
      return [];
    }
  }

  ready() {
    return !this.shown && storyOn() && !this.passed && !this.running
      && Boolean(this.result?.finished && this.result.failing)
      && this.activeSeconds >= this.gateMinutes * 60
      && !this.seen().includes(this.slug);
  }

  check() {
    clearTimeout(this.timer);
    this.timer = 0;
    if (!this.ready()) return;
    const wait = this.lastKey + QUIET_MS - Date.now();
    if (wait > 0) {
      this.timer = setTimeout(() => this.check(), wait);
      return;
    }
    this.show(true);
  }

  /** Shows the card and the header link. No focus, no scroll: the learner finds it. */
  show(remember) {
    if (this.shown) return;
    this.shown = true;
    this.card.hidden = false;
    this.link.hidden = false;
    // At that moment, so a reload does not show it again.
    if (remember) save(SEEN_KEY, JSON.stringify([...this.seen().filter((s) => s !== this.slug), this.slug].slice(-SEEN_MAX)));
  }

  dismiss() {
    this.card.hidden = true;
    this.link.hidden = true;
    this.heading.focus();
  }
}
