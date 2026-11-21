import { chromium } from 'playwright';

// The reference and the candidate must come out of the same browser build, or
// text rasterisation alone moves the diff. Playwright's pinned Chromium is
// that build; CHROMIUM_PATH overrides it for a machine that cannot download it,
// and then both sides have to be shot again with that binary.
export function launchBrowser() {
  const executablePath = process.env.CHROMIUM_PATH;
  return chromium.launch(executablePath ? { executablePath } : {});
}

export function newContext(browser, viewport) {
  return browser.newContext({
    viewport,
    deviceScaleFactor: 1,
    locale: 'pt-BR',
    timezoneId: 'America/Sao_Paulo',
  });
}

export async function settle(page) {
  await page.evaluate(() => document.fonts.ready);
  await page.waitForTimeout(500);
}
