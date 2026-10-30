import assert from 'node:assert/strict';
import { mkdir } from 'node:fs/promises';
import { dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright-core';

const base = process.env.E2E_BASE_URL;
const password = process.env.E2E_PASSWORD;
if (!base || !['127.0.0.1', 'localhost'].includes(new URL(base).hostname)) throw new Error('E2E_BASE_URL must point to an isolated local test environment.');
if (!password) throw new Error('Supply the seeded development password through E2E_PASSWORD.');
const screenshot = fileURLToPath(new URL('../../.cache/publication-smoke.png', import.meta.url));
const browser = await chromium.launch({ channel: process.env.E2E_BROWSER_CHANNEL ?? 'msedge', headless: true, timeout: 120000 });
const context = await browser.newContext({ baseURL: base, viewport: { width: 1365, height: 900 } });
const page = await context.newPage();
page.setDefaultTimeout(45000); page.setDefaultNavigationTimeout(120000);
const errors = [];
page.on('pageerror', error => errors.push(error.message));
const login = async email => {
  await page.getByLabel('Email address').fill(email);
  await page.getByLabel('Password', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await page.waitForURL('**/workspace');
};
try {
  await page.goto('/');
  await page.getByText('Illustrative sample', { exact: false }).waitFor();
  await page.getByLabel('Appearance', { exact: true }).selectOption('dark');
  await page.reload();
  assert.equal(await page.locator('html').getAttribute('data-theme'), 'dark');
  await page.getByLabel('Appearance', { exact: true }).selectOption('light');
  assert.equal(await page.locator('html').getAttribute('data-theme'), 'light');
  await page.goto('/login'); await login('manager@tessera.local');
  await page.getByRole('link', { name: 'Rosters', exact: true }).click();
  const weekInput = page.getByLabel('Week beginning Monday');
  const current = await weekInput.inputValue();
  const next = new Date(current + 'T12:00:00Z'); next.setUTCDate(next.getUTCDate() + 7);
  const week = next.toISOString().slice(0, 10);
  await weekInput.fill(week); await weekInput.press('Tab');
  await page.getByRole('button', { name: '+ Create draft', exact: true }).click();
  await page.getByText('Draft roster created.', { exact: true }).waitFor();
  await page.getByRole('button', { name: '+ Add shift', exact: true }).click();
  await page.getByLabel('Required qualification').selectOption({ label: 'Team member' });
  await page.getByLabel('Start', { exact: true }).fill(week + 'T09:00');
  await page.getByLabel('End', { exact: true }).fill(week + 'T17:00');
  await page.getByRole('button', { name: 'Save shift', exact: true }).click();
  const select = page.getByLabel(/^Employee for Team member at/);
  await select.waitFor();
  const [assigned] = await Promise.all([
    page.waitForResponse(response => response.url().endsWith('/assignment') && response.request().method() === 'PUT' && response.status() === 200),
    select.selectOption({ label: 'Sam Taylor' }),
  ]);
  const result = await assigned.json();
  await page.getByRole('button', { name: 'Publish roster', exact: true }).click();
  await page.getByRole('button', { name: 'Confirm publication', exact: true }).click();
  await page.getByText('Roster published. Employees can now see their assigned shifts.', { exact: true }).waitFor();
  assert.equal(await page.getByRole('button', { name: 'Edit', exact: true }).isDisabled(), true);
  assert.equal(await select.inputValue(), result.shift.employee_id);
  await page.getByRole('link', { name: 'Activity', exact: true }).click();
  await page.getByRole('heading', { name: 'Roster published', exact: true }).waitFor();
  await page.getByRole('link', { name: 'Overview', exact: true }).click();
  await page.getByRole('button', { name: 'Sign out', exact: true }).click();
  await page.waitForURL('**/login'); await login('employee@tessera.local');
  await page.getByRole('link', { name: 'My shifts', exact: true }).click();
  await page.getByRole('heading', { name: 'Team member', exact: true }).waitFor();
  assert.equal(await page.locator('.my-shift-card').count(), 1);
  assert.match(await page.locator('.my-shift-card').textContent(), /Main site/);
  assert.match(await page.locator('.my-shift-card').textContent(), /09:00/);
  await page.reload(); await page.getByRole('heading', { name: 'Team member', exact: true }).waitFor();
  await mkdir(dirname(screenshot), { recursive: true }); await page.screenshot({ path: screenshot, fullPage: true });
  await page.goto('/rosters'); await page.waitForURL('**/workspace');
  assert.deepEqual(errors, []);
  console.log('Browser smoke passed: manager create/assign/publish/activity/logout, employee own shifts/session restore, and manager-route denial.');
} catch (error) {
  await mkdir(dirname(screenshot), { recursive: true });
  await page.screenshot({ path: screenshot, fullPage: true }).catch(() => {});
  throw new Error(String(error?.stack ?? error).replaceAll(password, '[redacted]'));
} finally { await context.close(); await browser.close(); }
