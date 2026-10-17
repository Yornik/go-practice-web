// panels.js: the IDE's tab lists (ARIA tabs with arrow keys) and the
// splitters that resize the side and bottom panels.

/** Wires every [role=tablist] inside root. */
export function setupTabs(root) {
  const lists = new Map();
  for (const list of root.querySelectorAll('[role="tablist"][data-tabs]')) {
    const tabs = [...list.querySelectorAll('[role="tab"]')];
    const select = (tab) => {
      for (const t of tabs) {
        const on = t === tab;
        t.setAttribute("aria-selected", String(on));
        t.tabIndex = on ? 0 : -1;
        document.getElementById(t.getAttribute("aria-controls")).hidden = !on;
      }
    };
    for (const tab of tabs) {
      tab.addEventListener("click", () => select(tab));
      tab.addEventListener("keydown", (event) => {
        const i = tabs.indexOf(tab);
        let j = -1;
        if (event.key === "ArrowRight") j = (i + 1) % tabs.length;
        if (event.key === "ArrowLeft") j = (i - 1 + tabs.length) % tabs.length;
        if (event.key === "Home") j = 0;
        if (event.key === "End") j = tabs.length - 1;
        if (j >= 0) {
          event.preventDefault();
          tabs[j].focus();
          select(tabs[j]);
        }
      });
    }
    lists.set(list.dataset.tabs, (panelId) => {
      const tab = tabs.find((t) => t.getAttribute("aria-controls") === panelId);
      if (tab) select(tab);
    });
  }
  /** Shows a panel by its id, for example "bottom-output". */
  return (group, panelId) => lists.get(group)?.(panelId);
}

const SIZES = { side: ["--side-width", 15, 70, 34], bottom: ["--bottom-height", 15, 75, 38] };

function load(name) {
  try {
    return Number(localStorage.getItem(`gpw-split-${name}`)) || 0;
  } catch {
    return 0;
  }
}

function store(name, value) {
  try {
    localStorage.setItem(`gpw-split-${name}`, String(value));
  } catch {
    // The size lasts for this page only.
  }
}

/** Makes the splitters draggable and keyboard operable. */
export function setupSplitters(root) {
  for (const handle of root.querySelectorAll("[data-splitter]")) {
    const name = handle.dataset.splitter;
    const [prop, min, max, def] = SIZES[name];
    const box = name === "side" ? root.querySelector(".ide-body") : root.querySelector(".ide-main");
    const apply = (pct) => {
      const v = Math.min(max, Math.max(min, pct));
      box.style.setProperty(prop, `${v}%`);
      handle.setAttribute("aria-valuenow", String(Math.round(v)));
      return v;
    };
    let current = apply(load(name) || def);
    handle.setAttribute("aria-valuemin", String(min));
    handle.setAttribute("aria-valuemax", String(max));
    handle.addEventListener("pointerdown", (event) => {
      event.preventDefault();
      handle.setPointerCapture(event.pointerId);
      const rect = box.getBoundingClientRect();
      const move = (e) => {
        current = name === "side"
          ? apply(((e.clientX - rect.left) / rect.width) * 100)
          : apply(((rect.bottom - e.clientY) / rect.height) * 100);
      };
      const up = () => {
        handle.removeEventListener("pointermove", move);
        handle.removeEventListener("pointerup", up);
        store(name, current);
      };
      handle.addEventListener("pointermove", move);
      handle.addEventListener("pointerup", up);
    });
    handle.addEventListener("keydown", (event) => {
      const grow = name === "side" ? "ArrowRight" : "ArrowUp";
      const shrink = name === "side" ? "ArrowLeft" : "ArrowDown";
      if (event.key !== grow && event.key !== shrink) return;
      event.preventDefault();
      current = apply(current + (event.key === grow ? 2 : -2));
      store(name, current);
    });
  }
}
