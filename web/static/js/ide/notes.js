// notes.js: the notes of an exercise, saved 1 s after the last keystroke.
import { api, ApiError, errorMessage } from "../api.js";
import { clock } from "../ui/format.js";

// Browsers refuse keepalive requests whose bodies add up to more than
// 64 KiB per page. The code saves need room too, so larger notes leave
// without keepalive and depend on the beforeunload prompt.
const KEEPALIVE_MAX = 16 * 1024;
const RETRY_MS = 5000;

/** A retry can fix a lost connection, a busy server or a rate limit; not a refused note. */
function retryable(err) {
  return !(err instanceof ApiError) || err.status === 0 || err.status === 429 || err.status >= 500;
}

/**
 * Sets up the notes autosave. It returns attachLeave, which main.js calls
 * after the code autosave has added its own page-leave listeners:
 * listeners run in the order they were added, and the code goes first.
 * Until then (Monaco still loading, or failed), the notes leave on their own.
 */
export function setupNotes({ slug, textarea, status, onStatus }) {
  let timer = 0;
  let pending = false; // typed text that no save has taken yet
  let inflight = 0; // saves sent and not answered: a closing page cancels them
  let attached = false;
  const show = (text) => {
    status.textContent = text;
    onStatus?.(text);
  };
  const body = () => ({ notes: textarea.value });
  const saveNow = async (opts = {}) => {
    clearTimeout(timer);
    pending = false;
    inflight++;
    show("Saving the notes");
    try {
      await api("PUT", `/api/exercises/${slug}/notes`, body(), opts);
      show(`Notes saved at ${clock(new Date())}`);
    } catch (err) {
      pending = true;
      clearTimeout(timer);
      // A later keystroke must not be the only way to try again.
      if (retryable(err)) timer = setTimeout(saveNow, RETRY_MS);
      show(`The notes are not saved: ${errorMessage(err)}`);
    } finally {
      inflight--;
    }
  };
  textarea.addEventListener("input", () => {
    status.textContent = "Not saved yet";
    pending = true;
    clearTimeout(timer);
    timer = setTimeout(saveNow, 1000);
  });
  // A tab in the background can be discarded without a pagehide; the page
  // is still alive, so a normal request saves the notes at once.
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "hidden" && pending) saveNow();
  });

  // A link or a closed tab within the 1 s, or while a save is still on its
  // way, would lose the last words: the page sends them with keepalive.
  const leave = () => {
    if (!pending && !inflight) return;
    if (new Blob([JSON.stringify(body())]).size <= KEEPALIVE_MAX) saveNow({ keepalive: true });
  };
  addEventListener("pagehide", () => {
    if (!attached) leave();
  });
  addEventListener("beforeunload", (event) => {
    if (pending || inflight) event.preventDefault();
  });

  return {
    attachLeave() {
      if (attached) return;
      attached = true;
      addEventListener("pagehide", leave);
    },
  };
}
