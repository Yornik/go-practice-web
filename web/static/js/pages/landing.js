// landing.js: the leader lines of Figure 1. Each note sits as close to
// its line as the notes above it allow; a blue line ties the end of the
// marked code to the note's balloon. They draw once, on the first
// layout; a resize redraws them without motion. Below 721 px there are no
// leaders: each line ends with its balloon number instead.
import { reducedMotion } from "../ui/dom.js";

const body = document.querySelector("[data-leaders]");
const NS = "http://www.w3.org/2000/svg";

if (body) {
  const svg = body.querySelector(".leaders");
  const list = body.querySelector(".fig-notes");
  const sheet = body.querySelector(".fig-sheet");
  const code = body.querySelector(".fig-code");
  const wide = matchMedia("(min-width: 721px)");
  let drawn = false;

  const layout = () => {
    const items = [...list.children];
    svg.replaceChildren();
    for (const li of items) li.style.cssText = "";
    list.style.cssText = "";
    if (!wide.matches) return;

    const box = body.getBoundingClientRect();
    const sheetBox = sheet.getBoundingClientRect();
    const codeBox = code.getBoundingClientRect();
    const listBox = list.getBoundingClientRect();
    const heights = items.map((li) => li.getBoundingClientRect().height);
    list.style.position = "relative";
    let next = 0;
    let bottom = 0;
    items.forEach((li, i) => {
      const eol = body.querySelector(`.eol[data-note="${li.dataset.note}"]`).getBoundingClientRect();
      const lineY = eol.top + eol.height / 2 - listBox.top;
      const top = Math.max(lineY - 11, next);
      Object.assign(li.style, { position: "absolute", left: "0", right: "0", top: `${top}px` });
      next = top + heights[i] + 16;
      bottom = top + heights[i];

      // The leader starts 8 px after the text, inside the visible code.
      const x0 = Math.min(eol.left + 8, codeBox.right - 4) - box.left;
      const y0 = eol.top + eol.height / 2 - box.top;
      const knee = sheetBox.right - box.left + 10 + (items.length - 1 - i) * 8;
      const bx = listBox.left - box.left - 1;
      const by = top + 11 + (listBox.top - box.top);
      const d = `M${x0} ${y0} H${knee}${Math.abs(by - y0) > 1 ? ` V${by}` : ""} H${bx}`;
      const path = document.createElementNS(NS, "path");
      path.setAttribute("d", d);
      const len = knee - x0 + Math.abs(by - y0) + (bx - knee);
      path.style.setProperty("--len", String(len));
      path.style.setProperty("--delay", `${i * 150}ms`);
      const dot = document.createElementNS(NS, "circle");
      dot.setAttribute("cx", String(x0));
      dot.setAttribute("cy", String(y0));
      dot.setAttribute("r", "2.5");
      svg.append(path, dot);
    });
    list.style.height = `${bottom}px`;
    svg.classList.toggle("draw", !drawn && !reducedMotion());
    drawn = true;
  };

  document.fonts.ready.then(layout);
  let frame = 0;
  addEventListener("resize", () => {
    cancelAnimationFrame(frame);
    frame = requestAnimationFrame(layout);
  });
  code.addEventListener("scroll", () => {
    cancelAnimationFrame(frame);
    frame = requestAnimationFrame(layout);
  });
}

// Figure 2: each note sits beside the part of the figure it is about (the
// raw message, the parts list, the rule), so no note floats free. Only
// where the notes have their own column.
const notes = document.querySelector("[data-fig2-notes]");
if (notes) {
  const fig = notes.closest(".fig2");
  const side = matchMedia("(min-width: 1181px)");
  const align = () => {
    const items = [...notes.children];
    for (const li of items) li.style.marginTop = "";
    notes.style.paddingTop = "";
    if (!side.matches) return;
    notes.style.paddingTop = "0";
    let bottom = notes.getBoundingClientRect().top;
    for (const li of items) {
      const target = fig.querySelector(`[data-fig2="${li.dataset.for}"]`);
      if (!target) continue;
      const want = target.getBoundingClientRect().top;
      const top = li.getBoundingClientRect().top;
      const gap = Math.max(0, Math.max(want, bottom + 16) - top);
      li.style.marginTop = `${gap}px`;
      bottom = li.getBoundingClientRect().bottom;
    }
  };
  document.fonts.ready.then(align);
  addEventListener("resize", () => requestAnimationFrame(align));
}
