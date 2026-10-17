// The typography rule of the explanations, checked in a real browser: the
// title of a problem sets every captured value, and every word with "()"
// or "[]", in <code>. B612 draws "(" like "[", so "Validate()" in prose
// would read as "Validate[]" (".design/FINAL.md", section 7.6).
import { test, expect } from "@playwright/test";

test("titles put calls and indexes in code", async ({ page }) => {
  await page.goto("/guide");
  const out = await page.evaluate(async () => {
    const { titleNodes, messageParts } = await import("/static/js/ide/explain.js");
    const render = (title, captures) => {
      const p = document.createElement("p");
      p.append(...titleNodes(title, captures));
      return p.innerHTML;
    };
    const parts = messageParts({
      id: "test-got-want",
      raw: {
        source: "test", file: "policy_test.go", line: 50, openable: true,
        message: "zero Policy: Validate() = <nil>, want an error that wraps ErrInvalidPolicy",
        captures: { case: "zero Policy", call: "Validate()", got: "<nil>", want: "an error that wraps ErrInvalidPolicy" },
      },
    });
    return {
      gotWant: render("Validate() returned <nil>, the test wants an error that wraps ErrInvalidPolicy", { call: "Validate()", got: "<nil>", want: "an error that wraps ErrInvalidPolicy" }),
      literal: render("os.Exit(0) stops the test binary", {}),
      index: render("Reading xs[i] past the end", {}),
      parts: parts.filter((p) => p.letter).map((p) => `${p.letter}:${p.group}:${p.text}`),
    };
  });
  expect(out.gotWant).toBe("<code>Validate()</code> returned <code>&lt;nil&gt;</code>, the test wants <code>an error that wraps ErrInvalidPolicy</code>");
  expect(out.literal).toBe("<code>os.Exit(0)</code> stops the test binary");
  expect(out.index).toBe("Reading <code>xs[i]</code> past the end");
  // The place is 1 part, the case gets its own, at most 5 in all.
  expect(out.parts).toEqual(["a:position:policy_test.go:50", "b:case:zero Policy", "c:call:Validate()", "d:got:<nil>", "e:want:an error that wraps ErrInvalidPolicy"]);
});

test("no prose outside code shows parentheses in B612", async ({ page }) => {
  // The 2 parentheses come from a cut of Inter: B612's look like brackets.
  await page.goto("/guide");
  const family = await page.evaluate(() => getComputedStyle(document.body).fontFamily);
  expect(family.startsWith('"Manual Parens"')).toBe(true);
});

test("a staticcheck finding links its related place", async ({ page }) => {
  await page.goto("/guide");
  const out = await page.evaluate(async () => {
    const { problemView } = await import("/static/js/ide/explain.js");
    const revealed = [];
    const files = { has: (f) => f === "retry.go", reveal: (...a) => revealed.push(a.join(":")), model: () => null };
    const diag = {
      source: "staticcheck", code: "SA4023", severity: "warning", file: "retry.go", line: 20, column: 5,
      message: "this comparison is never true",
      related: { file: "retry.go", line: 15, column: 18, message: "the lhs of the comparison gets its value from here" },
    };
    const view = problemView({ n: 1, kind: "check", diag, file: "retry.go", line: 20, column: 5 }, { files, titles: {}, tests: null });
    const rel = view.querySelector(".problem-related");
    rel?.querySelector("a")?.click();
    return { text: rel?.textContent, revealed };
  });
  expect(out.text).toBe("See also line 15 in retry.go: the lhs of the comparison gets its value from here.");
  expect(out.revealed).toEqual(["retry.go:15:18"]);
});
