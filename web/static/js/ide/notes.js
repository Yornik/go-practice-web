// notes.js: the notes of an exercise, saved 1 s after the last keystroke.
import { api, errorMessage } from "../api.js";

export function setupNotes({ slug, textarea, status, initial }) {
  textarea.value = initial || "";
  let timer = 0;
  const save = async () => {
    status.textContent = "Saving…";
    try {
      await api("PUT", `/api/exercises/${slug}/notes`, { notes: textarea.value });
      status.textContent = "Saved";
    } catch (err) {
      status.textContent = `Not saved: ${errorMessage(err)}`;
    }
  };
  textarea.addEventListener("input", () => {
    status.textContent = "Unsaved changes";
    clearTimeout(timer);
    timer = setTimeout(save, 1000);
  });
}
