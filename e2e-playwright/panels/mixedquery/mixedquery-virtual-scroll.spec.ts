import { test, expect } from '@grafana/plugin-e2e';

import mixedQueryDashboard from './test-data/mixed-query-million-points.json';

test.use({
  featureToggles: {
    newLogsPanel: true,
  },
  viewport: { width: 1920, height: 1080 },
});

test.describe(
  'MixedQueryPanel - Virtual Scrolling Performance',
  {
    tag: ['@panels', '@performance'],
  },
  () => {
    test('should only render visible rows in viewport with 1M data points', async ({
      gotoDashboardPage,
      page,
    }) => {
      const dashboardPage = await gotoDashboardPage({
        uid: 'mixed-query-million-points',
      });

      await dashboardPage.waitForQueryData();

      const panel = dashboardPage.getPanelByTitle('Multi-Source Mixed Query');
      const panelElement = panel.locator;

      await expect(panelElement).toBeVisible();

      const scrollContainer = panelElement.locator('[data-testid="virtual-scroll-container"]');
      await expect(scrollContainer).toBeVisible();

      const initialRowCount = await scrollContainer.locator('[data-testid="data-row"]').count();

      expect(initialRowCount).toBeLessThan(50);

      const containerHeight = await scrollContainer.evaluate(
        (el: HTMLElement) => el.clientHeight
      );
      const rowHeight = await scrollContainer
        .locator('[data-testid="data-row"]')
        .first()
        .evaluate((el: HTMLElement) => el.clientHeight);

      const expectedVisibleRows = Math.floor(containerHeight / rowHeight) + 2;

      expect(initialRowCount).toBeLessThanOrEqual(expectedVisibleRows + 5);
    });

    test('should update rendered rows on scroll without re-rendering all', async ({
      gotoDashboardPage,
      page,
    }) => {
      const dashboardPage = await gotoDashboardPage({
        uid: 'mixed-query-million-points',
      });

      await dashboardPage.waitForQueryData();

      const panel = dashboardPage.getPanelByTitle('Multi-Source Mixed Query');
      const scrollContainer = panel.locator.locator('[data-testid="virtual-scroll-container"]');

      await scrollContainer.scrollIntoViewIfNeeded();

      const firstVisibleRowText = await scrollContainer
        .locator('[data-testid="data-row"]')
        .first()
        .textContent();

      await scrollContainer.evaluate((el: HTMLElement, scrollAmount: number) => {
        el.scrollTop += scrollAmount;
      }, 5000);

      await page.waitForTimeout(200);

      const newFirstVisibleRowText = await scrollContainer
        .locator('[data-testid="data-row"]')
        .first()
        .textContent();

      expect(newFirstVisibleRowText).not.toBe(firstVisibleRowText);

      const rowCountAfterScroll = await scrollContainer.locator('[data-testid="data-row"]').count();
      expect(rowCountAfterScroll).toBeLessThan(50);
    });

    test('should stream data from multiple datasources and merge in real-time', async ({
      gotoDashboardPage,
      page,
    }) => {
      const dashboardPage = await gotoDashboardPage({
        uid: 'mixed-query-streaming',
      });

      const panel = dashboardPage.getPanelByTitle('Streaming Mixed Query');

      await expect(panel.locator).toBeVisible();

      await page.waitForTimeout(2000);

      const dataSourceLabels = panel.locator.locator('[data-testid="source-label"]');
      const labelCount = await dataSourceLabels.count();

      expect(labelCount).toBeGreaterThanOrEqual(2);

      const sources: string[] = [];
      for (let i = 0; i < labelCount; i++) {
        const text = await dataSourceLabels.nth(i).textContent();
        if (text) sources.push(text);
      }

      expect(sources).toContain('prometheus');
      expect(sources).toContain('loki');
    });

    test('should maintain stable FPS during rapid streaming updates', async ({
      gotoDashboardPage,
      page,
    }) => {
      const dashboardPage = await gotoDashboardPage({
        uid: 'mixed-query-streaming',
      });

      const panel = dashboardPage.getPanelByTitle('Streaming Mixed Query');
      const scrollContainer = panel.locator.locator('[data-testid="virtual-scroll-container"]');

      await scrollContainer.scrollIntoViewIfNeeded();

      const frameTimes: number[] = [];
      let lastTime = performance.now();

      const observer = await page.evaluateHandle(() => {
        const times: number[] = [];
        let last = performance.now();

        const check = () => {
          const now = performance.now();
          times.push(now - last);
          last = now;
          requestAnimationFrame(check);
        };
        requestAnimationFrame(check);

        return {
          getTimes: () => times,
        };
      });

      await page.waitForTimeout(5000);

      const collectedTimes = await observer.evaluate((h: any) => h.getTimes());

      const avgFrameTime = collectedTimes.reduce((a: number, b: number) => a + b, 0) / collectedTimes.length;

      expect(avgFrameTime).toBeLessThan(33);
    });

    test('should handle panel resize without breaking virtual scroll', async ({
      gotoDashboardPage,
      page,
    }) => {
      const dashboardPage = await gotoDashboardPage({
        uid: 'mixed-query-million-points',
      });

      await dashboardPage.waitForQueryData();

      const panel = dashboardPage.getPanelByTitle('Multi-Source Mixed Query');

      const initialRowCount = await panel.locator
        .locator('[data-testid="data-row"]')
        .count();

      await dashboardPage.getByGrafanaSelector(
        dashboardPage.selectors.components.NavToolbar.editDashboard.editButton
      ).click();

      const panelHeader = panel.locator.locator('[aria-label="Panel header"]');
      const box = await panelHeader.boundingBox();

      if (box) {
        await page.mouse.move(box.x + box.width - 10, box.y + box.height / 2);
        await page.mouse.down();
        await page.mouse.move(box.x + box.width + 200, box.y + box.height / 2);
        await page.mouse.up();
      }

      await page.waitForTimeout(500);

      const rowCountAfterResize = await panel.locator
        .locator('[data-testid="data-row"]')
        .count();

      expect(rowCountAfterResize).toBeGreaterThan(0);
      expect(rowCountAfterResize).toBeLessThan(100);
    });
  }
);
