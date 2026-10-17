// notes.js: the notes of an exercise, saved 1 s after the last keystroke.
import { api, errorMessage } from "../api.js";
import { clock } from "../ui/format.js";

export function setupNotes({ slug, textarea, status, onStatus }) {
  let timer = 0;
  let pending = false;
  const show = (text) => {
    status.textContent = text;
    onStatus?.(text);
  };
  const saveNow = async (opts = {}) => {
    pending = false;
    show("Saving the notes");
    try {
      await api("PUT", `/api/exercises/${slug}/notes`, { notes: textarea.value }, opts);
      show(`Notes saved at ${clock(new Date())}`);
    } catch (err) {
      pending = true;
      show(`The notes are not saved: ${errorMessage(err)}`);
    }
  };
  textarea.addEventListener("input", () => {
    status.textContent = "Not saved yet";
    pending = true;
    clearTimeout(timer);
    timer = setTimeout(saveNow, 1000);
  });

  // A link or a closed tab within the 1 s would lose the last words, so
  // leaving saves at once, as the code autosave does; keepalive lets the
  // request finish after the page is gone. Browsers cap keepalive bodies
  // at 64 KiB, so a note near the limit is saved on a best-effort basis.
  const flushNow = () => {
    if (!pending) return;
    clearTimeout(timer);
    saveNow({ keepalive: true });
  };
  addEventListener("pagehide", flushNow);
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "hidden") flushNow();
  });
  addEventListener("beforeunload", (event) => {
    if (pending) {
      flushNow();
      event.preventDefault();
    }
  });
}
