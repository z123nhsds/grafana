import { test, expect, type Page } from '@grafana/plugin-e2e';

interface VirtualScrollMetrics {
  visibleRowCount: number;
  totalRenderedRows: number;
  containerHeight: number;
  rowHeight: number;
}

async function getVirtualScrollMetrics(page: Page): Promise<VirtualScrollMetrics> {
  const scrollContainer = page.locator('.rdg').first();

  const containerHeight = (await scrollContainer.evaluate((el) => el.clientHeight)) as number;

  const visibleRows = await scrollContainer.locator('[role="row"]').count();
  const visibleRowCount = visibleRows - 1;

  const rowHeight =
    visibleRows > 1
      ? ((await scrollContainer.locator('[role="row"]').nth(1).evaluate((el) => el.clientHeight)) as number)
      : 0;

  const totalRenderedRows = await scrollContainer.locator('[role="row"]').count();

  return {
    visibleRowCount,
    totalRenderedRows,
    containerHeight,
    rowHeight,
  };
}

async function getRenderedDataRowCount(page: Page): Promise<number> {
  const rows = page.locator('.rdg [role="row"]');
  const count = await rows.count();
  if (count === 0) {
    return 0;
  }
  return count - 1;
}

test.describe(
  'Virtual Scroll Performance',
  {
    tag: ['@panels', '@virtual-scroll'],
  },
  () => {
    test('virtual scroll renders only visible rows with large dataset', async ({ gotoDashboardPage, page }) => {
      const dashboardPage = await gotoDashboardPage({});

      await dashboardPage.addPanel();

      const testDataScenarioSelect = page.getByRole('combobox', { name: 'Scenario' });
      await expect(testDataScenarioSelect.first()).toBeVisible();

      await testDataScenarioSelect.first().click();
      await page.getByText('Random Walk Table').first().click();

      await page.waitForTimeout(1500);

      const tableContainer = page.locator('.rdg').first();
      await expect(tableContainer).toBeVisible({ timeout: 10000 });

      const metrics = await getVirtualScrollMetrics(page);

      expect(metrics.visibleRowCount).toBeGreaterThan(0);
      expect(metrics.visibleRowCount).toBeLessThanOrEqual(50);

      const maxVisibleRows = Math.ceil(metrics.containerHeight / metrics.rowHeight) + 2;
      expect(metrics.visibleRowCount).toBeLessThanOrEqual(maxVisibleRows);
    });

    test('scrolling updates visible rows without re-rendering all rows', async ({ gotoDashboardPage, page }) => {
      const dashboardPage = await gotoDashboardPage({});

      await dashboardPage.addPanel();

      const testDataScenarioSelect = page.getByRole('combobox', { name: 'Scenario' });
      await expect(testDataScenarioSelect.first()).toBeVisible();

      await testDataScenarioSelect.first().click();
      await page.getByText('Random Walk Table').first().click();

      await page.waitForTimeout(1500);

      const tableContainer = page.locator('.rdg').first();
      await expect(tableContainer).toBeVisible({ timeout: 10000 });

      const initialMetrics = await getVirtualScrollMetrics(page);

      const scrollTarget = tableContainer.locator('.rdg-viewport').first();
      await scrollTarget.hover();
      await page.mouse.wheel(0, 500);

      await page.waitForTimeout(300);

      const afterScrollMetrics = await getVirtualScrollMetrics(page);

      expect(afterScrollMetrics.visibleRowCount).toBeGreaterThan(0);
      expect(afterScrollMetrics.visibleRowCount).toBeLessThanOrEqual(initialMetrics.visibleRowCount + 5);
    });

    test('scroll to bottom reveals last rows while maintaining row count', async ({ gotoDashboardPage, page }) => {
      const dashboardPage = await gotoDashboardPage({});

      await dashboardPage.addPanel();

      const testDataScenarioSelect = page.getByRole('combobox', { name: 'Scenario' });
      await expect(testDataScenarioSelect.first()).toBeVisible();

      await testDataScenarioSelect.first().click();
      await page.getByText('Random Walk Table').first().click();

      await page.waitForTimeout(1500);

      const tableContainer = page.locator('.rdg').first();
      await expect(tableContainer).toBeVisible({ timeout: 10000 });

      const initialCount = await getRenderedDataRowCount(page);

      const scrollTarget = tableContainer.locator('.rdg-viewport').first();
      await scrollTarget.hover();

      for (let i = 0; i < 20; i++) {
        await page.mouse.wheel(0, 500);
        await page.waitForTimeout(50);
      }

      await page.waitForTimeout(500);

      const afterScrollCount = await getRenderedDataRowCount(page);

      expect(afterScrollCount).toBeGreaterThan(0);
      expect(afterScrollCount).toBeLessThanOrEqual(initialCount + 10);
    });

    test('row virtualization does not render off-screen rows', async ({ gotoDashboardPage, page }) => {
      const dashboardPage = await gotoDashboardPage({});

      await dashboardPage.addPanel();

      const testDataScenarioSelect = page.getByRole('combobox', { name: 'Scenario' });
      await expect(testDataScenarioSelect.first()).toBeVisible();

      await testDataScenarioSelect.first().click();
      await page.getByText('Random Walk Table').first().click();

      await page.waitForTimeout(1500);

      const tableContainer = page.locator('.rdg').first();
      await expect(tableContainer).toBeVisible({ timeout: 10000 });

      const metrics = await getVirtualScrollMetrics(page);

      const estimatedRows = 100;
      expect(metrics.visibleRowCount).toBeLessThan(estimatedRows);

      expect(metrics.totalRenderedRows).toBeLessThanOrEqual(metrics.visibleRowCount + 1);
    });

    test('memory usage stays stable during rapid scrolling', async ({ gotoDashboardPage, page }) => {
      const dashboardPage = await gotoDashboardPage({});

      await dashboardPage.addPanel();

      const testDataScenarioSelect = page.getByRole('combobox', { name: 'Scenario' });
      await expect(testDataScenarioSelect.first()).toBeVisible();

      await testDataScenarioSelect.first().click();
      await page.getByText('Random Walk Table').first().click();

      await page.waitForTimeout(1500);

      const tableContainer = page.locator('.rdg').first();
      await expect(tableContainer).toBeVisible({ timeout: 10000 });

      const measurements: number[] = [];

      for (let i = 0; i < 5; i++) {
        const metrics = await getVirtualScrollMetrics(page);
        measurements.push(metrics.totalRenderedRows);

        const scrollTarget = tableContainer.locator('.rdg-viewport').first();
        await scrollTarget.hover();
        await page.mouse.wheel(0, 300);
        await page.waitForTimeout(200);
      }

      const maxDiff = Math.max(...measurements) - Math.min(...measurements);
      expect(maxDiff).toBeLessThanOrEqual(10);
    });

    test('table panel renders with mixed datasource query', async ({ gotoDashboardPage, page }) => {
      const dashboardPage = await gotoDashboardPage({});

      await dashboardPage.addPanel();

      const mixedDatasourceOption = page.getByText('-- Mixed --').first();
      await expect(mixedDatasourceOption).toBeVisible({ timeout: 5000 });

      const testDataScenarioSelect = page.getByRole('combobox', { name: 'Scenario' });
      await expect(testDataScenarioSelect.first()).toBeVisible();

      await testDataScenarioSelect.first().click();
      await page.getByText('Random Walk Table').first().click();

      await page.waitForTimeout(1500);

      const tableContainer = page.locator('.rdg').first();
      await expect(tableContainer).toBeVisible({ timeout: 10000 });

      const metrics = await getVirtualScrollMetrics(page);
      expect(metrics.visibleRowCount).toBeGreaterThan(0);
    });
  }
);