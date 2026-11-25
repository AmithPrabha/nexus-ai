import { test, expect } from "@playwright/test";

test("review a discrepancy, correct it, approve and inspect the durable audit", async ({
  page,
}) => {
  await page.goto("/");
  await page.getByRole("link", { name: "Review an invoice" }).click();
  await page
    .getByRole("combobox", { name: "Fictional invoice", exact: true })
    .selectOption("design-services");
  await page.getByRole("button", { name: "Process invoice" }).click();
  await expect(page.getByText("VAT MISMATCH", { exact: true })).toBeVisible({
    timeout: 20_000,
  });
  await page.reload();
  await page
    .getByRole("textbox", { name: "Vat amount", exact: true })
    .fill("24.00");
  await page
    .getByRole("textbox", { name: "Total", exact: true })
    .fill("504.00");
  await page.getByLabel("Review reason").fill("Verified on fictional source");
  await page.getByRole("button", { name: "Save correction" }).click();
  await expect(
    page.getByText("All deterministic checks passed.", { exact: false }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Simulate approval" }).click();
  await expect(page.locator(".state")).toHaveText("Approved");
  await expect(
    page.getByRole("heading", { name: "Audit history" }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Review queue" }).click();
  await expect(
    page.getByRole("link", { name: "Design services" }),
  ).toBeVisible();
});

test("mobile policy answers open cited evidence, abstain, and enforce quotas", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#policy");
  await page
    .getByRole("button", { name: "Who approves purchases above AED 10,000?" })
    .click();
  await page.getByRole("button", { name: "Ask policy question" }).click();
  await page.locator("summary", { hasText: "Procurement handbook" }).click();
  await expect(page.locator("blockquote")).toContainText("finance director");
  await page
    .getByRole("button", {
      name: "What is the current UAE corporate tax rate?",
    })
    .click();
  await page.getByRole("button", { name: "Ask policy question" }).click();
  await expect(
    page.getByText("Insufficient evidence", { exact: true }),
  ).toBeVisible();
  for (let i = 0; i < 9; i++) {
    const done = page.waitForResponse((r) =>
      r.url().endsWith("/policy/questions"),
    );
    await page.getByRole("button", { name: "Ask policy question" }).click();
    await done;
  }
  await expect(
    page.getByRole("alert").filter({ hasText: "allowance" }),
  ).toContainText("allowance");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBeTruthy();
});
