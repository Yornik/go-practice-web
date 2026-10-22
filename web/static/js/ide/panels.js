// panels.js: the IDE's layout (VESPER.md 7.1, 7.10, 7.11). ARIA tabs with
// arrow keys; 3 columns from 1600 px, 2 columns and 1 bottom band from 900
// px, 1 scrolling column below. The resize handles work with a pointer,
// the arrow keys and a double click (reset); the Keys menu resets the
// layout or gives the editor or the tests more room, for a pointer that
// cannot drag (WCAG 2.5.7). Sizes and hidden columns are kept in
// localStorage. From 900 px the document is the window, so an in-page
// link scrolls only the pane that holds its target.
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

// Column widths in px; the splits in % (VESPER.md 7.1).
const LIMITS = { task: [300, 560], problems: [340, 600], band: [240, 0], split: [20, 80], tests: [20, 60] };
const DEFAULTS = { split: 70, tests: 35, band: 38 };
const STEP = 16; // px per arrow key
const KEYS = {
  task: "gpw-col-task",
  problems: "gpw-col-problems",
  band: "gpw-col-problems-band",
  split: "gpw-split-spec",
  tests: "gpw-row-tests",
  bandRow: "gpw-row-band",
  hideTask: "gpw-hide-task",
  hideProblems: "gpw-hide-problems",
};

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
  const ladder = ide.querySelector("[data-ladder]");
  const problems = ide.querySelector('[data-col="problems"]');
  const wide = matchMedia("(min-width: 1600px)");
  const medium = matchMedia("(min-width: 900px)");

  const num = (key) => Number(load(key)) || 0;
  const state = {
    task: num(KEYS.task),
    problems: num(KEYS.problems),
    band: num(KEYS.band),
    split: num(KEYS.split),
    tests: num(KEYS.tests),
    bandRow: num(KEYS.bandRow),
    hideTask: load(KEYS.hideTask) === "1",
    hideProblems: load(KEYS.hideProblems) === "1",
    compact: true, // the test panel before the first run
  };

  const handles = {
    task: ide.querySelector('[data-handle="task"]'),
    problems: ide.querySelector('[data-handle="problems"]'),
    split: ide.querySelector('[data-handle="split"]'),
    bottom: ide.querySelector('[data-handle="bottom"]'),
  };
  const showButtons = {
    task: ide.querySelector('[data-show="task"]'),
    problems: ide.querySelector('[data-show="problems"]'),
  };
  const clamp = (v, [lo, hi]) => Math.min(hi, Math.max(lo, v));
  const pct = (v) => `${Math.round(v)}`;
  const setVar = (el, name, value) => (value === null ? el.style.removeProperty(name) : el.style.setProperty(name, value));
  // The bottom band from 900 to 1599 px holds the hints too, so it never folds.
  const compactNow = () => state.compact && wide.matches;

  function apply() {
    const tw = state.task ? clamp(state.task, LIMITS.task) : 0;
    const pw = state.problems ? clamp(state.problems, LIMITS.problems) : 0;
    const split = state.split ? clamp(state.split, LIMITS.split) : DEFAULTS.split;
    const tests = state.tests ? clamp(state.tests, LIMITS.tests) : DEFAULTS.tests;
    const band = state.bandRow ? clamp(state.bandRow, LIMITS.tests) : DEFAULTS.band;
    work.classList.toggle("is-compact", compactNow());
    if (!medium.matches) {
      for (const el of [task, ladder, problems, ...Object.values(handles)]) el.hidden = false;
      for (const b of Object.values(showButtons)) b.hidden = true;
      onChange?.();
      return;
    }
    const hideT = state.hideTask;
    const hideP = state.hideProblems;
    task.hidden = hideT;
    ladder.hidden = hideT;
    handles.task.hidden = hideT;
    handles.split.hidden = hideT;
    problems.hidden = hideP;
    handles.problems.hidden = hideP;
    showButtons.task.hidden = !hideT;
    showButtons.problems.hidden = !hideP;
    setVar(main, "--task-w", hideT ? "0px" : tw ? `${tw}px` : null);
    setVar(main, "--vs1", hideT ? "0px" : null);
    setVar(main, "--vs2", hideP ? "0px" : null);
    if (wide.matches) {
      setVar(main, "--probs-w", hideP ? "0px" : pw ? `${pw}px` : null);
      setVar(main, "--spec-fr", `${pct(split)}fr`);
      setVar(main, "--hints-fr", `${pct(100 - split)}fr`);
      setVar(work, "--bottom-row", `minmax(160px, ${pct(tests)}%)`);
      setVar(main, "--band-row", null);
    } else {
      const bw = state.band ? Math.max(LIMITS.band[0], state.band) : 0;
      setVar(main, "--probs-w", hideP ? "0px" : bw ? `${bw}px` : null);
      setVar(main, "--band-row", `minmax(200px, ${pct(band)}%)`);
    }
    // The handles state their values: px for the columns, % for the splits.
    // Without a min and a max a reader assumes 0 to 100, so 320 would be out of range.
    const t = Math.round(task.getBoundingClientRect().width);
    handles.task.setAttribute("aria-valuemin", String(LIMITS.task[0]));
    handles.task.setAttribute("aria-valuemax", String(LIMITS.task[1]));
    handles.task.setAttribute("aria-valuenow", String(t));
    handles.task.setAttribute("aria-valuetext", `Task column ${t} px`);
    const p = Math.round(problems.getBoundingClientRect().width);
    handles.problems.setAttribute("aria-valuemin", String(wide.matches ? LIMITS.problems[0] : LIMITS.band[0]));
    handles.problems.setAttribute("aria-valuemax", String(wide.matches ? LIMITS.problems[1] : Math.max(LIMITS.band[0], Math.round(bandRoom()))));
    handles.problems.setAttribute("aria-valuenow", String(p));
    handles.problems.setAttribute("aria-valuetext", `Problems column ${p} px`);
    if (wide.matches) {
      aria(handles.split, LIMITS.split, split, `Spec ${pct(split)} % of the column`);
      aria(handles.bottom, LIMITS.tests, tests, `Tests ${pct(tests)} % of the height`);
    } else {
      // From 900 to 1599 px both grips sit on 1 line and move the bottom band.
      aria(handles.split, LIMITS.tests, band, `Bottom band ${pct(band)} % of the height`);
      aria(handles.bottom, LIMITS.tests, band, `Bottom band ${pct(band)} % of the height`);
    }
    onChange?.();
  }

  function aria(handle, [lo, hi], now, text) {
    handle.setAttribute("aria-valuemin", String(lo));
    handle.setAttribute("aria-valuemax", String(hi));
    handle.setAttribute("aria-valuenow", pct(now));
    handle.setAttribute("aria-valuetext", text);
  }

  // The room the problems can take in the bottom band: the tests keep 240 px.
  const bandRoom = () => main.getBoundingClientRect().width - (state.hideTask ? 0 : task.getBoundingClientRect().width + 1) - 1 - LIMITS.band[0];

  /** What each handle sets: its current value, a new value, and the key it is kept under. */
  const model = {
    task: {
      vertical: true,
      current: () => task.getBoundingClientRect().width,
      set: (v) => { state.task = Math.round(clamp(v, LIMITS.task)); },
      key: () => KEYS.task,
      value: () => state.task,
      reset: () => { state.task = 0; },
      grow: "ArrowRight",
    },
    problems: {
      vertical: true,
      current: () => problems.getBoundingClientRect().width,
      set: (v) => {
        if (wide.matches) state.problems = Math.round(clamp(v, LIMITS.problems));
        else state.band = Math.round(clamp(v, [LIMITS.band[0], Math.max(LIMITS.band[0], bandRoom())]));
      },
      key: () => (wide.matches ? KEYS.problems : KEYS.band),
      value: () => (wide.matches ? state.problems : state.band),
      reset: () => {
        if (wide.matches) state.problems = 0;
        else state.band = 0;
      },
      grow: "ArrowLeft",
    },
    split: {
      vertical: false,
      // At 1600 px and wider the spec's share of the task column; below, the bottom band.
      current: () => (wide.matches ? task.getBoundingClientRect().height : bandHeight()),
      set: (v) => {
        if (wide.matches) state.split = clamp((v / columnHeight()) * 100, LIMITS.split);
        else setBand(v);
      },
      key: () => (wide.matches ? KEYS.split : KEYS.bandRow),
      reset: () => {
        if (wide.matches) state.split = 0;
        else state.bandRow = 0;
      },
      grow: () => (wide.matches ? "ArrowDown" : "ArrowUp"),
      value: () => (wide.matches ? state.split : state.bandRow),
    },
    bottom: {
      vertical: false,
      current: () => (wide.matches ? ide.querySelector(".bottom-panel").getBoundingClientRect().height : bandHeight()),
      set: (v) => {
        state.compact = false;
        if (wide.matches) state.tests = clamp((v / work.getBoundingClientRect().height) * 100, LIMITS.tests);
        else setBand(v);
      },
      key: () => (wide.matches ? KEYS.tests : KEYS.bandRow),
      reset: () => {
        if (wide.matches) state.tests = 0;
        else state.bandRow = 0;
      },
      grow: "ArrowUp",
      value: () => (wide.matches ? state.tests : state.bandRow),
    },
  };
  const columnHeight = () => Math.max(1, task.getBoundingClientRect().height + ladder.getBoundingClientRect().height);
  const bandHeight = () => ide.querySelector(".bottom-panel").getBoundingClientRect().height;
  const setBand = (px) => {
    state.bandRow = clamp((px / main.getBoundingClientRect().height) * 100, LIMITS.tests);
  };
  const persist = (name) => {
    const m = model[name];
    save(m.key(), m.value() ? Math.round(m.value()) : null);
  };

  for (const [name, handle] of Object.entries(handles)) {
    const m = model[name];
    handle.addEventListener("pointerdown", (event) => {
      if (event.button !== 0) return;
      event.preventDefault();
      handle.setPointerCapture(event.pointerId);
      main.classList.add("is-resizing");
      const start = m.vertical ? event.clientX : event.clientY;
      const size = m.current();
      const move = (e) => {
        const delta = (m.vertical ? e.clientX : e.clientY) - start;
        // The task column and the spec grow towards the pointer; the
        // problems, the tests and the band grow away from the editor.
        const towards = name === "task" || (name === "split" && wide.matches);
        m.set(towards ? size + delta : size - delta);
        apply();
      };
      const up = () => {
        handle.removeEventListener("pointermove", move);
        handle.removeEventListener("pointerup", up);
        handle.removeEventListener("pointercancel", up);
        main.classList.remove("is-resizing");
        persist(name);
        onChange?.();
      };
      handle.addEventListener("pointermove", move);
      handle.addEventListener("pointerup", up);
      handle.addEventListener("pointercancel", up);
    });
    handle.addEventListener("dblclick", () => {
      m.reset();
      save(m.key(), null);
      apply();
    });
    handle.addEventListener("keydown", (event) => {
      const grow = typeof m.grow === "function" ? m.grow() : m.grow;
      const shrink = { ArrowRight: "ArrowLeft", ArrowLeft: "ArrowRight", ArrowUp: "ArrowDown", ArrowDown: "ArrowUp" }[grow];
      if (event.key !== grow && event.key !== shrink) return;
      event.preventDefault();
      m.set(m.current() + (event.key === grow ? STEP : -STEP));
      apply();
      persist(name);
    });
  }

  const hide = (name, on) => {
    state[name === "task" ? "hideTask" : "hideProblems"] = on;
    save(name === "task" ? KEYS.hideTask : KEYS.hideProblems, on ? "1" : "0");
    apply();
    (on ? showButtons[name] : ide.querySelector(`[data-hide="${name}"]`))?.focus();
  };
  for (const b of ide.querySelectorAll("[data-hide]")) b.addEventListener("click", () => hide(b.dataset.hide, true));
  for (const b of Object.values(showButtons)) b.addEventListener("click", () => hide(b.dataset.show, false));

  // The Keys menu's layout commands.
  const resize = (by) => {
    state.compact = false;
    if (wide.matches) {
      state.tests = clamp((state.tests || DEFAULTS.tests) + by, LIMITS.tests);
      save(KEYS.tests, state.tests);
    } else {
      state.bandRow = clamp((state.bandRow || DEFAULTS.band) + by, LIMITS.tests);
      save(KEYS.bandRow, state.bandRow);
    }
    apply();
  };
  const commands = {
    reset: () => {
      for (const k of ["task", "problems", "band", "split", "tests", "bandRow"]) state[k] = 0;
      state.hideTask = false;
      state.hideProblems = false;
      for (const key of Object.values(KEYS)) save(key, null);
      apply();
    },
    editor: () => resize(-10),
    tests: () => resize(10),
  };
  for (const b of ide.querySelectorAll("[data-layout-action]")) b.addEventListener("click", () => commands[b.dataset.layoutAction]?.());

  wide.addEventListener("change", apply);
  medium.addEventListener("change", apply);
  let frame = 0;
  addEventListener("resize", () => {
    cancelAnimationFrame(frame);
    frame = requestAnimationFrame(apply);
  });
  apply();

  // From 900 px the page never scrolls: an in-page link (the skip link,
  // "Hint 1" in the top bar, "A note from HUSH") scrolls only the pane
  // that holds its target, then moves the focus there.
  document.addEventListener("click", (event) => {
    const a = event.target.closest?.('a[href^="#"]');
    if (!a || !medium.matches || event.defaultPrevented) return;
    const id = decodeURIComponent(a.getAttribute("href").slice(1));
    const target = id ? document.getElementById(id) : null;
    if (!target) return;
    event.preventDefault();
    if (state.hideTask && (task.contains(target) || ladder.contains(target))) hide("task", false);
    if (state.hideProblems && problems.contains(target)) hide("problems", false);
    target.scrollIntoView({ block: "nearest", inline: "nearest" });
    if (!target.hasAttribute("tabindex")) target.setAttribute("tabindex", "-1");
    target.focus({ preventScroll: true });
  });

  // On a phone the spec folds at 60vh until the learner asks for all of it.
  const spec = ide.querySelector("[data-spec]");
  const more = ide.querySelector("[data-spec-more]");
  more?.addEventListener("click", () => {
    spec.classList.add("is-full");
    more.hidden = true;
    spec.focus();
  });

  // "More below" names the sections under the visible part of the spec,
  // and goes once the spec is scrolled to its end.
  const cont = ide.querySelector("[data-spec-cont]");
  const sections = [...ide.querySelectorAll("[data-spec-section]")];
  let contFrame = 0;
  const below = () => {
    if (!cont || !spec) return;
    const box = spec.getBoundingClientRect();
    const end = spec.scrollTop + spec.clientHeight >= spec.scrollHeight - 2;
    const names = sections.filter((s) => s.getBoundingClientRect().top > box.bottom - 8).map((s) => s.dataset.specSection);
    cont.hidden = end || names.length === 0;
    if (names.length) cont.textContent = `More below: ${names.join(", ")}`;
  };
  spec?.addEventListener("scroll", () => {
    cancelAnimationFrame(contFrame);
    contFrame = requestAnimationFrame(below);
  }, { passive: true });
  if (spec && "ResizeObserver" in window) new ResizeObserver(() => below()).observe(spec);
  document.fonts?.ready.then(below);

  return {
    /** Shows the problems: the column if it is hidden, or the stacked section on a phone. */
    revealProblems() {
      if (!medium.matches) {
        problems.scrollIntoView({ block: "start" });
        return;
      }
      if (state.hideProblems) hide("problems", false);
    },
    /** The number of problems; the column keeps its width, so nothing changes here. */
    setProblemCount() {},
    /** A run starts, or a past run shows: the test panel opens. */
    expandBottom() {
      if (!state.compact) return;
      state.compact = false;
      apply();
    },
    isWide: () => wide.matches,
  };
}
