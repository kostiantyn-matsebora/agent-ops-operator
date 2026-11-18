import { expect, test, type Page } from '@playwright/test'
import { TESTID, testid } from '../src/model/testids'
import { startHarness, type Harness } from './support/mock'
import { mockStart } from './support/mock-start'

// The workspace look's new-conversation dialog in a real browser.

let h: Harness
test.beforeEach(async () => {
  h = await startHarness()
})
test.afterEach(async () => {
  await h.stop()
})

async function openDialog(page: Page) {
  await page.goto(`${h.url}/conversations?look=workspace`)
  await expect(page.getByTestId(TESTID.lookSwitcher)).toHaveAttribute('data-active', 'workspace', { timeout: 15_000 })
  await page.getByTestId(TESTID.newConversation).click()
  await expect(page.getByTestId(TESTID.destinationCards)).toBeVisible()
}

test('starts a conversation addressed to the chosen destination', async ({ page }) => {
  const seen = await mockStart(page)
  await openDialog(page)
  await page.getByTestId(testid.destination('k8s-observe')).click()
  await page.getByLabel('task', { exact: true }).fill('check the nodes')
  await page.keyboard.press('Shift+Enter')
  await expect(page.getByTestId(TESTID.destinationCards)).toBeHidden()
  expect(seen.bodies[0].task).toBe('/k8s-observe check the nodes')
})

test('shows the server error and keeps the dialog open', async ({ page }) => {
  await mockStart(page, { fail: 'no Ready Pipeline serves it' })
  await openDialog(page)
  await page.getByLabel('task', { exact: true }).fill('go')
  await page.getByTestId(TESTID.startConversation).click()
  await expect(page.getByRole('alert')).toContainText('no Ready Pipeline serves it')
})
