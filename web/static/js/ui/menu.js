// menu.js: menu buttons (the theme menu, the account menu and the test
// files menu of the IDE). The button opens the menu; arrow keys move
// between the items, Escape closes it and gives the focus back.

const open = new Set();

/** Wires every [data-menu] inside root that is not wired yet. */
export function setupMenus(root = document) {
  for (const wrap of root.querySelectorAll("[data-menu]")) {
    if (wrap.dataset.menuReady) continue;
    wrap.dataset.menuReady = "true";
    menu(wrap);
  }
}

function items(wrap) {
  // getClientRects is empty for an item that CSS hides, such as the theme items on a wide screen.
  return [...wrap.querySelectorAll('[role^="menuitem"]')].filter((el) => !el.hidden && el.getClientRects().length > 0 && el.getAttribute("aria-disabled") !== "true");
}

function menu(wrap) {
  const button = wrap.querySelector("[data-menu-button]");
  const panel = button.nextElementSibling;
  const state = { wrap, button, panel, close: (focus) => close(state, focus) };

  button.addEventListener("click", () => {
    if (panel.hidden) show(state, 0);
    else close(state, false);
  });
  button.addEventListener("keydown", (event) => {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      show(state, event.key === "ArrowDown" ? 0 : -1);
    }
  });
  panel.addEventListener("keydown", (event) => {
    const list = items(wrap);
    const i = list.indexOf(document.activeElement);
    let j = -1;
    if (event.key === "ArrowDown") j = (i + 1) % list.length;
    if (event.key === "ArrowUp") j = (i - 1 + list.length) % list.length;
    if (event.key === "Home") j = 0;
    if (event.key === "End") j = list.length - 1;
    if (j >= 0) {
      event.preventDefault();
      list[j].focus();
      return;
    }
    if (event.key === "Escape") {
      event.preventDefault();
      close(state, true);
    }
    if (event.key === "Tab") close(state, false);
  });
  // An item that acts closes the menu before its own handler runs (capture
  // phase), so the focus is back on the menu button when the action opens a
  // dialog, and the dialog returns the focus there.
  panel.addEventListener("click", (event) => {
    if (event.target.closest('[role^="menuitem"]')) close(state, true);
  }, true);
}

function show(state, index) {
  for (const other of open) if (other !== state) close(other, false);
  state.panel.hidden = false;
  state.button.setAttribute("aria-expanded", "true");
  open.add(state);
  const list = items(state.wrap);
  // A menu of choices opens on the current choice; a menu of actions on its first item.
  const choices = list.every((el) => el.getAttribute("role") === "menuitemradio");
  const checked = choices ? list.findIndex((el) => el.getAttribute("aria-checked") === "true") : -1;
  const target = index === 0 && checked >= 0 ? checked : index < 0 ? list.length - 1 : index;
  list[target]?.focus();
}

function close(state, focus) {
  if (state.panel.hidden) return;
  state.panel.hidden = true;
  state.button.setAttribute("aria-expanded", "false");
  open.delete(state);
  if (focus) state.button.focus();
}

document.addEventListener("pointerdown", (event) => {
  for (const state of [...open]) if (!state.wrap.contains(event.target)) close(state, false);
});

/** Closes every open menu. */
export function closeMenus() {
  for (const state of [...open]) close(state, false);
}

/**
 * Wires every [data-popover]: a button that shows a small panel of
 * information. It is not a menu: the panel has no actions.
 */
export function setupPopovers(root = document) {
  for (const wrap of root.querySelectorAll("[data-popover]")) {
    const button = wrap.querySelector("[data-popover-button]");
    const panel = document.getElementById(button.getAttribute("aria-controls"));
    const set = (open) => {
      panel.hidden = !open;
      button.setAttribute("aria-expanded", String(open));
    };
    button.addEventListener("click", () => set(panel.hidden));
    wrap.addEventListener("keydown", (event) => {
      if (event.key === "Escape" && !panel.hidden) {
        set(false);
        button.focus();
      }
    });
    document.addEventListener("pointerdown", (event) => {
      if (!wrap.contains(event.target)) set(false);
    });
  }
}
