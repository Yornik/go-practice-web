// The public pages of the Vesper Labs design (".design/VESPER.md", 8.1 to
// 8.5 and 8.12): every 0 that Saira shows carries its slash, no page
// scrolls sideways from 320 to 1280 px, and the disclaimer is the landing
// page's last element and nowhere else.
import { test, expect } from "@playwright/test";
import { ovalZeros, watchErrors } from "./helpers.js";

const PAGES = ["/", "/guide", "/login", "/recover", "/verify-email?token=x", "/join?invite=nope", "/no-such-page"];

test("every 0 in Saira carries its slash", async ({ page }) => {
  const errors = watchErrors(page);
  for (const width of [1280, 360]) {
    await page.setViewportSize({ width, height: 800 });
    for (const url of PAGES) {
      await page.goto(url);
      await page.evaluate(() => document.fonts.ready);
      expect(await ovalZeros(page), `${url} at ${width} px`).toEqual([]);
    }
  }
  expect(errors).toEqual([]);
});

test("no public page scrolls sideways", async ({ page }) => {
  for (const width of [320, 360, 720, 900, 1280]) {
    await page.setViewportSize({ width, height: 800 });
    for (const url of PAGES) {
      await page.goto(url);
      await page.evaluate(() => document.fonts.ready);
      const over = await page.evaluate(() => document.documentElement.scrollWidth - innerWidth);
      expect(over, `${url} at ${width} px`).toBeLessThanOrEqual(0);
    }
    // A wide block in an open disclosure scrolls in its own frame.
    await page.goto("/guide");
    await page.evaluate(() => document.fonts.ready);
    const open = await page.evaluate(() =>
      [...document.querySelectorAll(".gpw-guide-guides details")].map((d) => {
        d.open = true;
        const over = document.documentElement.scrollWidth - innerWidth;
        d.open = false;
        return `${d.querySelector("summary").textContent.trim()}: ${over}`;
      }),
    );
    expect(open.length, "the guides on Go messages").toBe(4);
    for (const line of open) expect(line, `${line} at ${width} px`).toMatch(/: (0|-\d+)$/);
  }
});

test("the guide's Go code block keeps its tabs and its line breaks", async ({ page }) => {
  for (const width of [1280, 360]) {
    await page.setViewportSize({ width, height: 800 });
    await page.goto("/guide");
    await page.evaluate(() => document.fonts.ready);
    const got = await page.evaluate(() => {
      const pre = [...document.querySelectorAll("#make-it-a-tool pre")].find((p) => p.textContent.includes("func run("));
      const cs = getComputedStyle(pre);
      const lines = Math.round((pre.clientHeight - parseFloat(cs.paddingTop) - parseFloat(cs.paddingBottom)) / parseFloat(cs.lineHeight));
      // The left edge of the first character of a line, from the text itself.
      const left = (needle) => {
        const walk = document.createTreeWalker(pre, NodeFilter.SHOW_TEXT);
        for (let n = walk.nextNode(); n; n = walk.nextNode()) {
          const i = n.data.indexOf(needle);
          if (i < 0) continue;
          const r = document.createRange();
          r.setStart(n, i);
          r.setEnd(n, i + 1);
          return r.getBoundingClientRect().left;
        }
        return NaN;
      };
      const ch = (() => {
        const r = document.createRange();
        const n = document.createTreeWalker(pre, NodeFilter.SHOW_TEXT).nextNode();
        r.setStart(n, 0);
        r.setEnd(n, 1);
        return r.getBoundingClientRect().width;
      })();
      return { lines, whiteSpace: getComputedStyle(pre.querySelector("code")).whiteSpace, indent: Math.round((left("// Parse") - left("func")) / ch) };
    });
    // func main, a blank line, func run, the comment, return 0 and the brace.
    expect(got, `at ${width} px`).toEqual({ lines: 6, whiteSpace: "pre", indent: 4 });
  }
});

test("the disclaimer closes the landing page and no other", async ({ page }) => {
  await page.goto("/");
  const disclaimer = page.locator(".gpw-disclaim");
  await expect(disclaimer).toHaveCount(1);
  await expect(disclaimer).toContainText("are fictional");
  // Nothing visible is drawn below it.
  const last = await page.evaluate(() => {
    const d = document.querySelector(".gpw-disclaim").getBoundingClientRect().bottom + scrollY;
    return [...document.body.querySelectorAll("*")].filter((el) => {
      const r = el.getBoundingClientRect();
      return r.height > 0 && r.top + scrollY > d && getComputedStyle(el).visibility === "visible";
    }).length;
  });
  expect(last).toBe(0);
  for (const url of ["/guide", "/login", "/no-such-page"]) {
    await page.goto(url);
    await expect(page.locator(".gpw-disclaim")).toHaveCount(0);
  }
});

test("the guide's contents mark the section in view", async ({ page }) => {
  await page.goto("/guide");
  const toc = page.getByRole("navigation", { name: "On this page" });
  await expect(toc.getByRole("link", { name: "The story" })).toHaveAttribute("aria-current", "true");
  await page.evaluate(() => document.getElementById("editor-keys").scrollIntoView({ block: "start" }));
  await expect(toc.getByRole("link", { name: "Keys in the editor" })).toHaveAttribute("aria-current", "true");
  await expect(toc.getByRole("link", { name: "The story" })).not.toHaveAttribute("aria-current", "true");
});
