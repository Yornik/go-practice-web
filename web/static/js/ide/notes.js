// notes.js: the notes of an exercise, saved 1 s after the last keystroke.
import { api, errorMessage } from "../api.js";
import { clock } from "../ui/format.js";

// Browsers refuse keepalive requests whose bodies add up to more than
// 64 KiB per page. The code saves need room too, so larger notes leave
// without keepalive and depend on the beforeunload prompt.
const KEEPALIVE_MAX = 16 * 1024;
const RETRY_MS = 5000;

/**
 * Sets up the notes autosave. It returns attachLeave, which main.js calls
 * after the code autosave has added its own page-leave listeners:
 * listeners run in the order they were added, and the code goes first.
 */
export function setupNotes({ slug, textarea, status, onStatus }) {
  let timer = 0;
  let pending = false;
  const show = (text) => {
    status.textContent = text;
    onStatus?.(text);
  };
  const body = () => ({ notes: textarea.value });
  const saveNow = async (opts = {}) => {
    clearTimeout(timer);
    pending = false;
    show("Saving the notes");
    try {
      await api("PUT", `/api/exercises/${slug}/notes`, body(), opts);
      show(`Notes saved at ${clock(new Date())}`);
    } catch (err) {
      // A later keystroke must not be the only way to try again.
      pending = true;
      clearTimeout(timer);
      timer = setTimeout(saveNow, RETRY_MS);
      show(`The notes are not saved: ${errorMessage(err)}`);
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

  return {
    attachLeave() {
      // A link or a closed tab within the 1 s would lose the last words.
      addEventListener("pagehide", () => {
        if (!pending) return;
        const size = new Blob([JSON.stringify(body())]).size;
        if (size <= KEEPALIVE_MAX) saveNow({ keepalive: true });
      });
      addEventListener("beforeunload", (event) => {
        if (pending) event.preventDefault();
      });
    },
  };
}
