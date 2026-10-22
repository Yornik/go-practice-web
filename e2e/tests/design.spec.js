// The shared parts of the Vesper Labs design (".design/VESPER.md"): the
// header's controls, Story effects (16.5) and the slashed zero (3.3).
import { test, expect } from "@playwright/test";
import { watchErrors } from "./helpers.js";

test("story effects stay off across pages and stay in sync", async ({ page }) => {
  const errors = watchErrors(page);
  await page.goto("/guide");
  const root = page.locator("html");
  await expect(root).not.toHaveAttribute("data-story", "off");

  // The theme menu holds System, Light, Dark, then Story effects.
  await page.getByRole("button", { name: "Theme", exact: true }).click();
  const menu = page.getByRole("menu", { name: "Theme" });
  await expect(menu.locator('[role="menuitemradio"]')).toHaveText(["System", "Light", "Dark"]);
  const item = menu.getByRole("menuitemcheckbox", { name: "Story effects" });
  await expect(item).toHaveAttribute("aria-checked", "true");
  await item.click();
  await expect(root).toHaveAttribute("data-story", "off");
  expect(await page.evaluate(() => localStorage.getItem("gpw-story"))).toBe("off");

  // theme-init.js sets it before the first paint on the next page.
  await page.goto("/verify");
  await expect(root).toHaveAttribute("data-story", "off");
  await page.getByRole("button", { name: "Theme", exact: true }).click();
  await expect(page.getByRole("menuitemcheckbox", { name: "Story effects" })).toHaveAttribute("aria-checked", "false");
  await page.getByRole("menuitemcheckbox", { name: "Story effects" }).click();
  await expect(root).not.toHaveAttribute("data-story", "off");
  expect(await page.evaluate(() => localStorage.getItem("gpw-story"))).toBeNull();
  expect(errors).toEqual([]);
});

test("the menu sheet holds the links, Story effects and the theme on a phone", async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 780 });
  await page.goto("/guide");
  await page.getByRole("button", { name: "Menu", exact: true }).click();
  const toggle = page.locator("[data-nav-toggle]");
  await expect(toggle).toHaveAttribute("aria-expanded", "true");
  await expect(toggle).toHaveText("Close menu");
  const sheet = page.locator("[data-nav-sheet]");
  await expect(sheet.getByRole("link", { name: "Verify a certificate" })).toBeVisible();
  // The pressed state carries the meaning; the state word is not in the name (6.5).
  const story = sheet.getByRole("button", { name: "Story effects", exact: true });
  await expect(story).toHaveAttribute("aria-pressed", "true");
  await expect(story).toHaveText(/^story effects\s+on$/i, { useInnerText: true });
  await story.click();
  await expect(story).toHaveAttribute("aria-pressed", "false");
  await expect(story).toHaveText(/^story effects\s+off$/i, { useInnerText: true });
  await sheet.getByLabel("Dark").check();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  // Escape closes the sheet and gives the focus back to its button.
  await page.keyboard.press("Escape");
  await expect(sheet).toBeHidden();
  await expect(toggle).toBeFocused();
  // Escape on the button itself closes it too.
  await page.keyboard.press("Enter");
  await expect(sheet).toBeVisible();
  await expect(toggle).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(sheet).toBeHidden();
});

test("story effects off show Off before site.js runs", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("gpw-story", "off"));
  // site.js never arrives: what shows is what theme-init.js and the CSS draw.
  await page.route("**/js/site.js*", (route) => route.abort());
  await page.goto("/guide");
  const got = await page.locator(".site-head [data-story-toggle] b").first().evaluate((b) => {
    const amber = document.createElement("i");
    amber.style.color = "var(--signal)";
    b.append(amber);
    const signal = getComputedStyle(amber).color;
    amber.remove();
    const word = [...b.children].filter((s) => getComputedStyle(s).display !== "none").map((s) => s.textContent);
    return { word, amber: getComputedStyle(b).color === signal };
  });
  expect(got).toEqual({ word: ["Off"], amber: false });
});

test("the zero helper slashes every 0", async ({ page }) => {
  await page.goto("/guide");
  const html = await page.evaluate(async () => {
    const { zero } = await import("/static/js/ui/zero.js");
    return zero("40 000, T10").outerHTML;
  });
  const z = '<span class="gpw-z">0</span>';
  expect(html).toBe(`<span class="gpw-zs">4${z} ${z}${z}${z}, T1${z}</span>`);
});
