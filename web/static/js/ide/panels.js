// panels.js: the IDE's layout. ARIA tabs with arrow keys; the 3 columns
// with resize handles (pointer and arrow keys) and "Hide"; the bottom
// panel's height; the breakpoints (3 columns from 1280 px, 2 from 900 px,
// stacked below); and the folded spec on a phone. Sizes and hidden
// columns are kept in localStorage.
import { load, save } from "../ui/dom.js";

/** Wires every [role=tablist][data-tabs] inside root. Returns show(group, panelId). */
export function setupTabs(root) {
  const lists = new Map();
  for (const list of root.querySelectorAll('[role="tablist"][data-tabs]')) {
    const tabs = () => [...list.querySelectorAll('[role="tab"]')].filter((t) => t.offsetParent !== null || t.getAttribute("aria-selected") === "true");
    const all = [...list.querySelectorAll('[role="tab"]')];
    const select = (tab) => {
      for (const t of all) {
        const on = t === tab;
        t.setAttribute("aria-selected", String(on));
        t.tabIndex = on ? 0 : -1;
        const panel = document.getElementById(t.getAttribute("aria-controls"));
        if (panel) panel.hidden = !on;
      }
      list.dispatchEvent(new CustomEvent("gpw:tab", { detail: tab.getAttribute("aria-controls") }));
    };
    for (const tab of all) {
      tab.addEventListener("click", () => select(tab));
      tab.addEventListener("keydown", (event) => {
        const ts = tabs();
        const i = ts.indexOf(tab);
        let j = -1;
        if (event.key === "ArrowRight") j = (i + 1) % ts.length;
        if (event.key === "ArrowLeft") j = (i - 1 + ts.length) % ts.length;
        if (event.key === "Home") j = 0;
        if (event.key === "End") j = ts.length - 1;
        if (j >= 0) {
          event.preventDefault();
          ts[j].focus();
          select(ts[j]);
        }
      });
    }
    lists.set(list.dataset.tabs, {
      show: (panelId) => {
        const tab = all.find((t) => t.getAttribute("aria-controls") === panelId);
        if (tab) select(tab);
      },
      current: () => all.find((t) => t.getAttribute("aria-selected") === "true")?.getAttribute("aria-controls"),
    });
  }
  return {
    show: (group, panelId) => lists.get(group)?.show(panelId),
    current: (group) => lists.get(group)?.current(),
  };
}

const LIMITS = { task: [300, 560], problems: [340, 600] };
const STEP = 16;

/**
 * The column layout.
 * @param {HTMLElement} ide
 * @param {{show: Function, current: Function}} tabs
 * @param {() => void} onChange called after any change of the layout
 */
export function setupLayout(ide, tabs, onChange) {
  const main = ide.querySelector("[data-layout]");
  const work = ide.querySelector(".work");
  const task = ide.querySelector('[data-col="task"]');
  const problems = ide.querySelector('[data-col="problems"]');
  const panel = ide.querySelector("[data-problems-panel]");
  const sideSlot = ide.querySelector("[data-problems-slot-side]");
  const bottomSlot = ide.querySelector("[data-problems-slot-bottom]");
  const wide = matchMedia("(min-width: 1280px)");
  const medium = matchMedia("(min-width: 900px)");

  const state = {
    task: Number(load("gpw-col-task")) || 0,
    problems: Number(load("gpw-col-problems")) || 0,
    bottom: Number(load("gpw-row-bottom")) || 0,
    hideTask: load("gpw-hide-task") === "1",
    hideProblems: load("gpw-hide-problems") === "1",
    compact: true, // the test panel before the first run
    empty: true, // no problems: the problems column is a rail
    railOpen: false, // the learner opened the empty column
  };

  const handles = {
    task: ide.querySelector('[data-handle="task"]'),
    problems: ide.querySelector('[data-handle="problems"]'),
    bottom: ide.querySelector('[data-handle="bottom"]'),
  };
  const showButtons = {
    task: ide.querySelector('[data-show="task"]'),
    problems: ide.querySelector('[data-show="problems"]'),
  };

  // The test panel takes 40% of the work column; before the first run it
  // is its tab bar and 1 line, so the editor gets the room.
  const defaults = () => ({
    task: wide.matches ? Math.min(420, Math.max(340, innerWidth * 0.26)) : Math.min(360, Math.max(300, innerWidth * 0.3)),
    problems: Math.min(480, Math.max(380, innerWidth * 0.3)),
    bottom: Math.round((innerHeight - 52 - 28 - 40) * 0.4),
  });
  const COMPACT = 96;
  const RAIL = 40;
  const clamp = (v, [lo, hi]) => Math.min(hi, Math.max(lo, v));

  function apply() {
    const d = defaults();
    const tw = clamp(state.task || d.task, LIMITS.task);
    const pw = clamp(state.problems || d.problems, LIMITS.problems);
    const bh = state.compact ? COMPACT : Math.min(Math.max(state.bottom || d.bottom, 160), Math.round(innerHeight * 0.6));
    work.style.setProperty("--bottom-h", `${bh}px`);
    work.classList.toggle("is-compact", state.compact);
    handles.bottom.setAttribute("aria-valuenow", String(bh));
    if (!medium.matches) {
      main.style.gridTemplateColumns = "";
      task.hidden = false;
      problems.hidden = false;
      movePanel(sideSlot);
      onChange?.();
      return;
    }
    const cols = [];
    task.hidden = state.hideTask;
    handles.task.hidden = state.hideTask;
    showButtons.task.hidden = !state.hideTask;
    cols.push(state.hideTask ? "0px" : `${tw}px`, state.hideTask ? "0px" : "1px");
    handles.task.setAttribute("aria-valuenow", String(Math.round(tw)));
    if (wide.matches) {
      const rail = state.empty && !state.railOpen && !state.hideProblems;
      problems.hidden = state.hideProblems;
      problems.classList.toggle("is-rail", rail);
      handles.problems.hidden = state.hideProblems || rail;
      showButtons.problems.hidden = !state.hideProblems;
      cols.push("minmax(480px, 1fr)", state.hideProblems ? "0px" : "1px", state.hideProblems ? "0px" : rail ? `${RAIL}px` : `${pw}px`);
      handles.problems.setAttribute("aria-valuenow", String(Math.round(pw)));
      movePanel(sideSlot);
    } else {
      problems.classList.remove("is-rail");
      cols.push("minmax(0, 1fr)");
      movePanel(bottomSlot);
    }
    main.style.gridTemplateColumns = cols.join(" ");
    onChange?.();
  }

  function movePanel(slot) {
    if (panel.parentElement === slot) return;
    slot.append(panel);
    // The Problems tab only exists from 900 to 1279 px; leave it if it was open.
    if (slot === sideSlot && tabs.current("bottom") === "panel-problems") tabs.show("bottom", "panel-tests");
  }

  for (const [name, handle] of Object.entries(handles)) {
    const vertical = name !== "bottom";
    if (vertical) {
      handle.setAttribute("aria-valuemin", String(LIMITS[name][0]));
      handle.setAttribute("aria-valuemax", String(LIMITS[name][1]));
    } else {
      handle.setAttribute("aria-valuemin", "160");
    }
    const current = () => {
      if (name === "task") return task.getBoundingClientRect().width;
      if (name === "problems") return problems.getBoundingClientRect().width;
      return ide.querySelector(".bottom").getBoundingClientRect().height;
    };
    const set = (v) => {
      if (name === "bottom") {
        state.compact = false;
        state.bottom = Math.round(Math.max(160, Math.min(v, innerHeight * 0.6)));
      } else state[name] = Math.round(clamp(v, LIMITS[name]));
      apply();
    };
    const persist = () => save(name === "bottom" ? "gpw-row-bottom" : `gpw-col-${name}`, state[name]);
    handle.addEventListener("pointerdown", (event) => {
      event.preventDefault();
      handle.setPointerCapture(event.pointerId);
      main.classList.add("is-resizing");
      const start = vertical ? event.clientX : event.clientY;
      const size = current();
      const move = (e) => {
        const delta = (vertical ? e.clientX : e.clientY) - start;
        // The task column grows to the right; the others grow towards the editor.
        set(name === "task" ? size + delta : size - delta);
      };
      const up = () => {
        handle.removeEventListener("pointermove", move);
        handle.removeEventListener("pointerup", up);
        handle.removeEventListener("pointercancel", up);
        main.classList.remove("is-resizing");
        persist();
        onChange?.();
      };
      handle.addEventListener("pointermove", move);
      handle.addEventListener("pointerup", up);
      handle.addEventListener("pointercancel", up);
    });
    handle.addEventListener("keydown", (event) => {
      const grow = { task: "ArrowRight", problems: "ArrowLeft", bottom: "ArrowUp" }[name];
      const shrink = { task: "ArrowLeft", problems: "ArrowRight", bottom: "ArrowDown" }[name];
      if (event.key !== grow && event.key !== shrink) return;
      event.preventDefault();
      set(current() + (event.key === grow ? STEP : -STEP));
      persist();
    });
  }

  const hide = (name, on) => {
    state[name === "task" ? "hideTask" : "hideProblems"] = on;
    save(name === "task" ? "gpw-hide-task" : "gpw-hide-problems", on ? "1" : "0");
    apply();
    (on ? showButtons[name] : ide.querySelector(`[data-hide="${name}"]`))?.focus();
  };
  for (const b of ide.querySelectorAll("[data-hide]")) b.addEventListener("click", () => hide(b.dataset.hide, true));
  for (const b of Object.values(showButtons)) b.addEventListener("click", () => hide(b.dataset.show, false));

  wide.addEventListener("change", apply);
  medium.addEventListener("change", apply);
  addEventListener("resize", () => {
    if (!state.bottom) apply();
  });
  apply();

  // On a phone the spec folds at 60vh until the learner asks for all of it.
  const spec = ide.querySelector("[data-spec]");
  const more = ide.querySelector("[data-spec-more]");
  more?.addEventListener("click", () => {
    spec.classList.add("is-full");
    more.hidden = true;
    spec.focus();
  });

  // The rail of the empty problems column opens it, to read the empty state.
  const rail = ide.querySelector("[data-problems-rail]");
  rail?.addEventListener("click", () => {
    state.railOpen = true;
    apply();
    ide.querySelector("[data-problem-view]")?.focus();
  });

  return {
    /** Shows the problem view: the column, the Problems tab or the stacked section. */
    revealProblems() {
      if (wide.matches) {
        if (state.hideProblems) hide("problems", false);
      } else if (medium.matches) {
        tabs.show("bottom", "panel-problems");
      } else {
        panel.scrollIntoView({ block: "start" });
      }
    },
    /** The number of problems: none folds the column to a rail, the first one opens it. */
    setProblemCount(n) {
      const empty = n === 0;
      if (empty === state.empty) return;
      state.empty = empty;
      if (!empty) state.railOpen = false;
      // The rail hides the column; the focus inside it would drop to the page.
      else if (problems?.contains(document.activeElement)) state.railOpen = true;
      if (rail) rail.querySelector("[data-rail-count]").textContent = String(n);
      apply();
    },
    /** A run starts, or a past run shows: the test panel takes its height. */
    expandBottom() {
      if (!state.compact) return;
      state.compact = false;
      apply();
    },
    isWide: () => wide.matches,
  };
}
