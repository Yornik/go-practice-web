// activity.js: active time. Every 15 s, while the tab is visible and the
// learner typed or clicked in the last 60 s, a heartbeat tells the server.
// The server credits at most 15 s per beat.
import { api } from "../api.js";

const BEAT_MS = 15_000;
const IDLE_MS = 60_000;

export function trackActivity({ slug, onActiveSeconds }) {
  let lastInput = Date.now();
  const touch = () => {
    lastInput = Date.now();
  };
  for (const type of ["keydown", "pointerdown", "wheel"]) document.addEventListener(type, touch, { passive: true, capture: true });
  setInterval(async () => {
    if (document.visibilityState !== "visible" || Date.now() - lastInput > IDLE_MS) return;
    try {
      const res = await api("POST", `/api/exercises/${slug}/heartbeat`, {});
      onActiveSeconds(res.activeSeconds);
    } catch {
      // A missed beat loses 15 s of credit, nothing more.
    }
  }, BEAT_MS);
  return { touch };
}
