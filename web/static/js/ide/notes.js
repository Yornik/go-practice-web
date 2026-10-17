// notes.js: the notes of an exercise, saved 1 s after the last keystroke.
import { api, errorMessage } from "../api.js";
import { clock } from "../ui/format.js";

export function setupNotes({ slug, textarea, status, onStatus }) {
  let timer = 0;
  const show = (text) => {
    status.textContent = text;
    onStatus?.(text);
  };
  const saveNow = async () => {
    show("Saving the notes");
    try {
      await api("PUT", `/api/exercises/${slug}/notes`, { notes: textarea.value });
      show(`Notes saved at ${clock(new Date())}`);
    } catch (err) {
      show(`The notes are not saved: ${errorMessage(err)}`);
    }
  };
  textarea.addEventListener("input", () => {
    status.textContent = "Not saved yet";
    clearTimeout(timer);
    timer = setTimeout(saveNow, 1000);
  });
}
