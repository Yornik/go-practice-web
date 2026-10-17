// activity.js: active time. Every 15 s, while this tab is visible and in
// front, and the learner pressed a key, clicked or scrolled in the last
// 10 minutes, a heartbeat tells the server. The server credits at most 15 s
// per beat. 10 minutes, not 1: the course asks learners to work examples
// on paper and to read the spec twice, and that thinking time counts.
import { api } from "../api.js";

const BEAT_MS = 15_000;
const IDLE_MS = 10 * 60_000;

/**
 * @param {{slug: string, onActiveSeconds: (s: number) => void, onPaused: (paused: boolean) => void}} o
 */
export function trackActivity({ slug, onActiveSeconds, onPaused }) {
  let lastInput = Date.now();
  let paused = null;
  const touch = () => {
    lastInput = Date.now();
    tick();
  };
  for (const type of ["keydown", "pointerdown", "wheel"]) document.addEventListener(type, touch, { passive: true, capture: true });
  const counting = () => document.visibilityState === "visible" && document.hasFocus() && Date.now() - lastInput <= IDLE_MS;
  const tick = () => {
    const now = !counting();
    if (now !== paused) {
      paused = now;
      onPaused?.(paused);
    }
  };
  addEventListener("focus", tick);
  addEventListener("blur", tick);
  document.addEventListener("visibilitychange", tick);
  setInterval(async () => {
    tick();
    if (paused) return;
    try {
      const res = await api("POST", `/api/exercises/${slug}/heartbeat`, {});
      onActiveSeconds(res.activeSeconds);
    } catch {
      // A missed beat loses 15 s of credit, nothing more.
    }
  }, BEAT_MS);
  tick();
  return { touch };
}
