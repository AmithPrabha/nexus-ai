import { expect, test } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { resolve } from "node:path";
import { createHash, randomBytes, randomUUID } from "node:crypto";

// An explicitly selected local Compose project owns these synthetic test records.
const project = process.env.NEXUS_E2E_PROJECT;
test("named reviewer signs in, approves, signs out and finds the decision again", async ({ page }) => {
  test.skip(!project, "Set NEXUS_E2E_PROJECT to the local test database's Compose project");
  const tenant = randomUUID();
  const user = randomUUID();
  const sql = (input: string) => execFileSync("docker", [
    "compose", "-f", resolve(__dirname, "../../../compose.yaml"), "-p", project!, "exec", "-T", "postgres", "psql",
    "-Xq", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "nexus",
  ], { input, stdio: ["pipe", "pipe", "pipe"] });
  sql(`BEGIN;
    INSERT INTO tenants(id,kind,name) VALUES('${tenant}','COMPANY','Example accounts payable');
    INSERT INTO users(id,subject,display_name) VALUES('${user}','${user}','Alex Reviewer');
    INSERT INTO tenant_memberships(tenant_id,user_id,role) VALUES('${tenant}','${user}','REVIEWER');
    COMMIT;`);
  async function signIn() {
    const code = randomBytes(32).toString("base64url");
    const digest = createHash("sha256").update(code).digest("hex");
    sql(`INSERT INTO login_codes VALUES(decode('${digest}','hex'),'${tenant}','${user}',now()+interval '10 minutes')`);
    await page.getByLabel("One-time sign-in code").fill(code);
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    await expect(page.getByText("Example accounts payable", {exact:true})).toBeVisible();
  }
  await page.goto("/");
  await page.getByRole("link", {name:"Organization sign-in"}).click();
  await signIn();
  await page.getByRole("link", {name:"Review an invoice"}).click();
  await page.getByRole("button", {name:"Process invoice"}).click();
  await page.getByRole("button", {name:"Approve as Alex Reviewer"}).click();
  await expect(page.locator(".state").first()).toHaveText("Approved");
  const invoice = page.url();
  await page.getByRole("button", {name:"Sign out"}).click();
  await signIn();
  await page.goto(invoice);
  await expect(page.locator(".timeline li").last()).toContainText("Alex Reviewer");
  await expect(page.locator(".timeline li").last()).toContainText("Approved");
});
