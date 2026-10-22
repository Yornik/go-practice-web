// guide.js: the contents list marks the section in view, "Print the
// checklist" prints the checklist card alone, and inline code that holds
// a phrase wraps at its spaces.
import { h } from "../ui/dom.js";

const toc = document.querySelector("[data-toc]");
if (toc && "IntersectionObserver" in window) {
  const links = new Map([...toc.querySelectorAll("a")].map((a) => [a.getAttribute("href").slice(1), a]));
  const visible = new Set();
  const mark = () => {
    // The first section in document order that is in view wins.
    let current = null;
    for (const id of links.keys()) {
      if (visible.has(id)) {
        current = id;
        break;
      }
    }
    // Above the first section, the contents still say where you are.
    if (!current && scrollY < 1) current = links.keys().next().value;
    if (!current) return;
    for (const [id, a] of links) {
      if (id === current) a.setAttribute("aria-current", "true");
      else a.removeAttribute("aria-current");
    }
  };
  const observer = new IntersectionObserver(
    (entries) => {
      for (const e of entries) {
        if (e.isIntersecting) visible.add(e.target.id);
        else visible.delete(e.target.id);
      }
      mark();
    },
    { rootMargin: "0px 0px -60% 0px" },
  );
  for (const id of links.keys()) {
    const section = document.getElementById(id);
    if (section) observer.observe(section);
  }
}

document.querySelector("[data-print-checklist]")?.addEventListener("click", () => {
  document.body.classList.add("print-checklist");
  window.print();
  document.body.classList.remove("print-checklist");
});

// Inline code stays on 1 line, because browsers break after a hyphen and
// "-race" must not split. A phrase such as "race detected during
// execution of test" is wider than a phone, so it wraps at its spaces and
// keeps each word whole. The text is the same, so a copy is exact.
for (const code of document.querySelectorAll(".gpw-guide .gpw-prose :not(pre) > code")) {
  if (!/\s/.test(code.textContent)) continue;
  code.classList.add("gpw-code-words");
  for (const node of [...code.childNodes]) {
    if (node.nodeType !== Node.TEXT_NODE) continue;
    node.replaceWith(...node.data.split(/(\s+)/).filter(Boolean).map((p) => (/\s/.test(p) ? p : h("span", { class: "gpw-nobr" }, p))));
  }
}
