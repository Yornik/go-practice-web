// leader.js: the trace, an amber line from a problem's line to its
// explanation (".design/VESPER.md", 6.9 and 7.6). Only at 1600 px and
// wider, where the problems column stands beside the editor, and only for
// the selected problem. The trace is decoration: "Line 120 in retry.go" in
// the explanation carries the same fact for keyboard and screen readers.
//
// A leader always points at the problem's own subject. When that subject
// is out of view (a line above or below the editor's view, a test row
// scrolled away or inside a folded step), the leader ends at the edge of
// the view with a quiet button that brings the subject into view. The
// editor's button docks in the bar beside that edge, the file bar above
// or the test panel's bar below, so it never covers code.

import { h } from "../ui/dom.js";
import { zero } from "../ui/zero.js";

const NS = "http://www.w3.org/2000/svg";

export class Leader {
  /**
   * @param {object} o
   * @param {HTMLElement} o.root the positioned box the overlay covers
   * @param {SVGSVGElement} o.svg
   * @param {any} o.editor
   * @param {any} o.files
   * @param {HTMLElement} o.column the scroll area of the problems column
   * @param {HTMLElement} o.view the problem view inside it
   * @param {HTMLElement} o.results the test panel
   * @param {HTMLElement} o.treeWrap the scrolling box of the test tree
   * @param {HTMLButtonElement} o.jump "Line 120 is below"
   * @param {{top: HTMLElement, bottom: HTMLElement}} o.docks where the jump
   *   docks: the first child of the file bar's end, or before the run controls
   * @param {HTMLButtonElement} o.treeJump "TestDoPermanent is below"
   */
  constructor(o) {
    Object.assign(this, o);
    this.problem = null;
    this.frame = 0;
    this.tests = null; // the test runner, set once it exists
    this.problems = null;
    this.wide = matchMedia("(min-width: 1600px)");
    const later = () => this.redraw();
    this.editor.onDidScrollChange(later);
    this.editor.onDidLayoutChange(later);
    this.editor.onDidChangeModel(later);
    this.editor.onDidChangeModelContent(later);
    addEventListener("resize", later);
    this.column.addEventListener("scroll", later, { passive: true });
    this.treeWrap.addEventListener("scroll", later, { passive: true });
    this.wide.addEventListener("change", later);
    this.jump.addEventListener("click", () => {
      const p = this.problem;
      const at = this.place(p);
      // The button hides once its line shows. If it held the focus, the
      // focus goes to that line in the editor, not to the page's body.
      if (at) this.files.reveal(at.file, at.line, 1, { focus: document.activeElement === this.jump });
    });
    this.treeJump.addEventListener("click", () => {
      if (this.treeTarget) this.tests?.revealTest(this.treeTarget);
    });
  }

  /** Selects the problem to draw; animate when the selection is new. */
  set(problem, { animate = true } = {}) {
    this.problem = problem;
    this.draw(animate);
  }

  /** Redraws in the next frame, without motion. */
  redraw() {
    cancelAnimationFrame(this.frame);
    this.frame = requestAnimationFrame(() => this.draw(false));
  }

  clear() {
    this.svg.replaceChildren();
    this.jump.hidden = true;
    this.treeJump.hidden = true;
    this.treeTarget = null;
  }

  /** The editor place of a problem: its line, or for the stub problem the first stub to write. */
  place(p) {
    if (!p) return null;
    if (p.stub) {
      const fn = p.marks?.[0];
      return fn ? { file: fn.file, line: fn.panicLine } : null;
    }
    if (!p.file || !p.line || p.stale) return null;
    return { file: p.file, line: p.line };
  }

  /** The test whose row the leader starts at: the one the learner picked, if it is the problem's. */
  testOf(p) {
    const names = (p.occ || []).map((o) => o.test).filter(Boolean);
    const picked = [this.problems?.preferredTest, this.tests?.selectedKey].find((t) => t && names.includes(t));
    return picked || p.test || "";
  }

  /** The start of the leader: [x, y, edge] in page coordinates, or null. */
  start(p) {
    // A failing test: the end of its row in the tree.
    const test = p.kind === "run" ? this.testOf(p) : "";
    if (test && this.results.offsetParent !== null && !this.results.hidden) {
      const box = this.treeWrap.getBoundingClientRect();
      const row = this.treeWrap.querySelector(`[data-test="${CSS.escape(test)}"] > .tree-row`);
      let ref = row;
      if (!ref) {
        // Folded away: its nearest shown parent says above or below.
        const parts = test.split("/");
        for (let i = parts.length - 1; i > 0 && !ref; i--) ref = this.treeWrap.querySelector(`[data-test="${CSS.escape(parts.slice(0, i).join("/"))}"] > .tree-row`);
        if (!ref && this.tests) {
          const step = this.tests.stepFor(test);
          ref = this.treeWrap.querySelector(`[data-key="${CSS.escape(step ? `step:${step.id}` : "step:other")}"] > .tree-row`);
        }
      }
      if (box.height > 0) {
        const r = ref?.getBoundingClientRect();
        if (row && r.height > 0 && r.top >= box.top - 2 && r.bottom <= box.bottom + 2) return [r.right - 6, r.top + r.height / 2, null];
        this.treeTarget = test;
        const above = r && r.bottom <= box.top;
        return [box.right - 12, above ? box.top + 6 : box.bottom - 6, above ? "tree-top" : "tree-bottom"];
      }
    }
    const at = this.place(p);
    if (!at || this.files.activeName() !== at.file) return null;
    const model = this.editor.getModel();
    if (!model || at.line > model.getLineCount()) return null;
    const node = this.editor.getDomNode();
    const er = node.getBoundingClientRect();
    const layout = this.editor.getLayoutInfo();
    const pos = this.editor.getScrolledVisiblePosition({ lineNumber: at.line, column: model.getLineMaxColumn(at.line) });
    const right = er.left + layout.width - layout.verticalScrollbarWidth - 6;
    if (!pos) return null;
    if (pos.top < 0) return [right, er.top + 4, "top"];
    if (pos.top + pos.height > layout.height) return [right, er.top + layout.height - 4, "bottom"];
    // 8 px after the text and the 18 px marker 10 px after it.
    const x = Math.min(er.left + pos.left + 36, right);
    return [x, er.top + pos.top + pos.height / 2, null];
  }

  draw(animate) {
    const p = this.problem;
    this.clear();
    if (!p || !this.wide.matches || this.root.classList.contains("is-resizing")) return;
    const article = this.view.querySelector(".problem");
    if (!article || this.column.offsetWidth < 100) return;
    const s = this.start(p);
    if (!s) return;
    const [x0, y0, edge] = s;
    if (edge === "top" || edge === "bottom") {
      const at = this.place(p);
      this.jump.hidden = false;
      this.jump.dataset.edge = edge;
      const dock = this.docks?.[edge];
      if (edge === "top" && dock && dock.firstElementChild !== this.jump) dock.prepend(this.jump);
      if (edge === "bottom" && dock && dock.previousElementSibling !== this.jump) dock.before(this.jump);
      // A Saira button: its 0 is slashed (VESPER.md 3.3).
      this.jump.replaceChildren(zero(`Line ${at.line} is ${edge === "top" ? "above" : "below"}`));
    } else if (edge) {
      const e = edge.slice(5);
      this.treeJump.hidden = false;
      this.treeJump.dataset.edge = e;
      // A test name is code: it keeps its form inside the capitals.
      this.treeJump.replaceChildren(h("code", {}, this.treeTarget.split("/").pop()), ` is ${e === "top" ? "above" : "below"}`);
    }
    const box = this.root.getBoundingClientRect();
    const col = this.column.getBoundingClientRect();
    const a = article.getBoundingClientRect();
    // The problem hangs on its rule, the article's left border.
    const railX = a.left;
    const top = Math.max(a.top, col.top);
    const bottom = Math.min(a.bottom, col.bottom);
    if (top >= bottom) return;
    const ballY = Math.min(Math.max(a.top + 11, top + 2), bottom - 2);
    const lx = (x) => x - box.left;
    const ly = (y) => y - box.top;
    let d;
    if (y0 >= top + 4 && y0 <= bottom - 4) {
      // The rail is beside the start: straight across.
      d = `M${lx(x0)} ${ly(y0)} H${lx(railX)}`;
    } else {
      // 12 px past the editor's right edge, then down or up to the rule: 1 knee pair.
      const knee = Math.min(this.editor.getDomNode().getBoundingClientRect().right + 12, railX - 4);
      d = `M${lx(x0)} ${ly(y0)} H${lx(knee)} V${ly(ballY)} H${lx(railX)}`;
    }
    const path = document.createElementNS(NS, "path");
    path.setAttribute("d", d);
    this.svg.append(path);
    const dot = (x, y) => {
      const c = document.createElementNS(NS, "circle");
      c.setAttribute("cx", String(lx(x)));
      c.setAttribute("cy", String(ly(y)));
      c.setAttribute("r", "1.5"); // a 3 px dot at the start
      this.svg.append(c);
    };
    dot(x0, y0);
    if (animate) {
      path.style.setProperty("--len", String(path.getTotalLength()));
      this.svg.classList.remove("draw");
      void this.svg.getBoundingClientRect(); // restart the animation
      this.svg.classList.add("draw");
    } else {
      this.svg.classList.remove("draw");
    }
  }
}
