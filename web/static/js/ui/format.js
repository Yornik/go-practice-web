// format.js: numbers, times and durations as the copy rules write them:
// "1 h 12 min", "14:06", "17 Oct 2026", with a no-break space between a
// number and its unit.

const NB = " ";
const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

/** Active time: "35 min", "1 h 12 min". */
export function duration(seconds) {
  const m = Math.floor((seconds || 0) / 60);
  if (m < 60) return `${m}${NB}min`;
  const rest = m % 60;
  return rest ? `${Math.floor(m / 60)}${NB}h ${rest}${NB}min` : `${Math.floor(m / 60)}${NB}h`;
}

/** Whole minutes: "1 min", "6 min". */
export function minutes(n) {
  return `${n}${NB}min`;
}

/** A run time: "1.4 s". */
export function seconds(ms) {
  return `${((ms || 0) / 1000).toFixed(1)}${NB}s`;
}

/** The local clock time: "14:06". */
export function clock(t) {
  const d = t instanceof Date ? t : new Date(t);
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

/** A short date: "17 Oct 2026". */
export function shortDate(t) {
  const d = t instanceof Date ? t : new Date(t);
  return `${d.getDate()} ${MONTHS[d.getMonth()]} ${d.getFullYear()}`;
}

/** "1 problem", "2 problems". */
export function count(n, one, many = `${one}s`) {
  return `${n} ${n === 1 ? one : many}`;
}

/** Reports whether the platform shows Cmd instead of Ctrl. */
export function isMac() {
  const p = navigator.userAgentData?.platform || navigator.platform || "";
  return /mac|iphone|ipad/i.test(p);
}

/** The name of the main modifier key on this platform. */
export function modKey() {
  return isMac() ? "Cmd" : "Ctrl";
}
