// guide.js: the contents list marks the section in view, and "Print the
// checklist" prints the checklist card alone.

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
    if (!current) return;
    for (const [id, a] of links) {
      if (id === current) a.setAttribute("aria-current", "location");
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
