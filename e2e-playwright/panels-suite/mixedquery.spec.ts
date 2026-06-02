import { test, expect } from '@grafana/plugin-e2e';

test.describe('MixedQuery Panel Virtual Scrolling Performance', () => {
  test('should render only visible rows when dealing with millions of timepoints', async ({
    gotoPanelEditPage,
    page,
  }: { gotoPanelEditPage: any; page: any }) => {
    const panelEditPage = await gotoPanelEditPage({ dashboard: { uid: 'virtual-scroll-test' } });
    
    // 1. Mock the backend query response to return 1 million points

    await panelEditPage.mockQueryDataResponse({
      status: 200,
      body: {
        results: {
          A: {
            frames: [
              {
                schema: {
                  name: 'long_data',
                  refId: 'A',
                  fields: [
                    { name: 'time', type: 'time' },
                    { name: 'value', type: 'number' },
                  ],
                },
                data: {
                  // Simulate 1 million points (values array)
                  // In actual E2E, we might mock a smaller dataset or use chunking, 
                  // but we intercept to simulate the large scale.
                  values: [
                    Array.from({ length: 1000000 }, (_, i) => 1609459200000 + i * 1000), // Time
                    Array.from({ length: 1000000 }, (_, i) => Math.random() * 100),     // Value
                  ],
                },
              },
            ],
          },
        },
      },
    });

    // 2. Setup the panel type to 'mixedquery'
    await panelEditPage.setPanelTitle('Mixed Query Perf Test');
    await panelEditPage.setPanelType('mixedquery');

    // Wait for the panel to render the initial state
    await expect(panelEditPage.getPanelByTitle('Mixed Query Perf Test')).toBeVisible();

    // 3. Verify Virtual Scrolling Behavior
    // Locate the virtual scroll container (assumed class 'virtual-scroll-viewport')
    const viewport = page.locator('.virtual-scroll-viewport');
    
    // Check that not all 1 million rows are rendered in the DOM
    // For example, if row height is 20px and viewport is 400px, 
    // it should render around 20-30 rows max (with overscan).
    const rowLocators = viewport.locator('[data-testid="visible-row"]');
    
    // Wait for rows to appear
    await expect(rowLocators.first()).toBeVisible();

    // Count the DOM nodes
    const renderedRowCount = await rowLocators.count();
    
    // Assertion: Should be significantly less than 1 million (e.g. < 100)
    expect(renderedRowCount).toBeLessThan(100);
    expect(renderedRowCount).toBeGreaterThan(0);

    // 4. Scroll the viewport and verify it updates the rows
    await viewport.evaluate((element: any) => {
      element.scrollTop = 50000; // Scroll deep into the dataset
    });

    // Wait a moment for virtual scroll to render new items
    await page.waitForTimeout(200);

    // Verify DOM still has a small number of rows
    const scrolledRowCount = await rowLocators.count();
    expect(scrolledRowCount).toBeLessThan(100);
    expect(scrolledRowCount).toBeGreaterThan(0);

    // Check performance - UI should still be responsive
    const isResponsive = await page.evaluate(() => {
      return new Promise((resolve) => {
        const start = performance.now();
        requestAnimationFrame(() => {
          resolve(performance.now() - start < 50); // Frame should render in < 50ms
        });
      });
    });
    
    expect(isResponsive).toBeTruthy();
  });
});
