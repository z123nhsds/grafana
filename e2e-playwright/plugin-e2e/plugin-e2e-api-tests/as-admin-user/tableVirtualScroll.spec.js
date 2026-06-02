const { expect, test } = require('@grafana/plugin-e2e');

function buildLargeAlignedTableQuery(totalRows = 10000) {
  const labels = Array.from({ length: totalRows }, (_, index) => `row-${index}`);
  const timestamps = Array.from({ length: totalRows }, (_, index) => 1700000000000 + index * 1000);
  const leftValues = Array.from({ length: totalRows }, (_, index) => (index % 2 === 0 ? index : null));
  const rightValues = Array.from({ length: totalRows }, (_, index) => (index % 2 === 1 ? index : null));

  return {
    results: {
      A: {
        status: 200,
        frames: [
          {
            schema: {
              refId: 'A',
              fields: [
                {
                  name: 'label',
                  type: 'string',
                  typeInfo: { frame: 'string', nullable: false },
                },
                {
                  name: 'time',
                  type: 'time',
                  typeInfo: { frame: 'time.Time', nullable: false },
                },
                {
                  name: 'left',
                  type: 'number',
                  typeInfo: { frame: 'float64', nullable: true },
                },
                {
                  name: 'right',
                  type: 'number',
                  typeInfo: { frame: 'float64', nullable: true },
                },
              ],
            },
            data: {
              values: [labels, timestamps, leftValues, rightValues],
            },
          },
        ],
      },
    },
  };
}

test('table panel virtualization keeps rendering within the visible window', async ({ panelEditPage }) => {
  const totalRows = 10000;

  await panelEditPage.mockQueryDataResponse(buildLargeAlignedTableQuery(totalRows), 200);
  await panelEditPage.datasource.set('gdev-testdata');
  await panelEditPage.setVisualization('Table');
  await panelEditPage.refreshPanel();

  await expect(panelEditPage.panel.locator).toBeVisible();

  const rows = panelEditPage.panel.locator.locator('[role="row"]');
  await expect(rows.first()).toBeVisible();

  const initialRenderedRows = await rows.count();
  expect(initialRenderedRows).toBeLessThan(200);
  await expect(panelEditPage.panel.locator.getByText('row-0')).toBeVisible();
  await expect(panelEditPage.panel.locator.getByText(`row-${totalRows - 1}`)).not.toBeVisible();

  await panelEditPage.panel.locator.evaluate((panel) => {
    const candidates = Array.from(panel.querySelectorAll('*'));
    const scroller = candidates.find((element) => element.scrollHeight > element.clientHeight);

    if (scroller) {
      scroller.scrollTop = scroller.scrollHeight;
      scroller.dispatchEvent(new Event('scroll'));
    }
  });

  await expect(panelEditPage.panel.locator.getByText(`row-${totalRows - 1}`)).toBeVisible();
  await expect(panelEditPage.panel.locator.getByText('row-0')).not.toBeVisible();
});
