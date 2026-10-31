import { chromium } from 'playwright';

const GRAFANA = process.env.GRAFANA_URL || 'http://127.0.0.1:3000';
const DS = { type: 'grafana-openobserve-datasource', uid: 'P4A5AAF418F5D059C' };

const browser = await chromium.launch({ executablePath: '/usr/bin/google-chrome', args: ['--no-sandbox'] });
const ctx = await browser.newContext({ viewport: { width: 1700, height: 1100 } });
const page = await ctx.newPage();
const errors = [];
page.on('pageerror', e => errors.push('pageerror: ' + e.message));
page.on('console', m => {
  if (m.type() === 'error') {
    errors.push(m.text());
  }
});

await page.goto(`${GRAFANA}/login`, { waitUntil: 'domcontentloaded' });
await page.fill('input[name="user"]', 'admin');
await page.fill('input[name="password"]', 'admin');
await page.click('button[type="submit"]');
await page.waitForTimeout(3000);

const dashboard = {
  uid: 'oo-smoke',
  title: 'OpenObserve Smoke',
  timezone: 'browser',
  schemaVersion: 39,
  version: 0,
  refresh: '',
  tags: ['openobserve'],
  time: { from: 'now-1h', to: 'now' },
  panels: [
    {
      id: 1, type: 'timeseries', title: 'goroutines by service (SQL)', datasource: DS,
      gridPos: { h: 8, w: 12, x: 0, y: 0 },
      targets: [{ refId: 'A', datasource: DS, queryType: 'metrics', format: 'timeseries',
        rawSql: "SELECT histogram(_timestamp, '1 minute') AS ts, avg(value) AS val, service_name FROM \"go_goroutine_count\" GROUP BY ts, service_name ORDER BY ts",
        enableSSE: true, useCache: true }],
      fieldConfig: { defaults: { unit: 'none' }, overrides: [] },
      options: { legend: { displayMode: 'list', placement: 'bottom' } },
    },
    {
      id: 2, type: 'timeseries', title: 'goroutines (PromQL)', datasource: DS,
      gridPos: { h: 8, w: 12, x: 12, y: 0 },
      targets: [{ refId: 'A', datasource: DS, queryType: 'metrics', queryMode: 'promql',
        expr: 'go_goroutine_count', format: 'timeseries' }],
      fieldConfig: { defaults: {}, overrides: [] },
      options: { legend: { displayMode: 'list', placement: 'bottom' } },
    },
    {
      id: 3, type: 'logs', title: 'logs', datasource: DS,
      gridPos: { h: 10, w: 24, x: 0, y: 8 },
      targets: [{ refId: 'A', datasource: DS, queryType: 'logs', format: 'logs',
        rawSql: 'SELECT * FROM "vnet_bff_dev" LIMIT 50', enableSSE: true }],
      options: {},
    },
    {
      id: 4, type: 'table', title: 'table', datasource: DS,
      gridPos: { h: 8, w: 12, x: 0, y: 18 },
      targets: [{ refId: 'A', datasource: DS, queryType: 'logs', format: 'table',
        rawSql: 'SELECT service_name, body, code_line_number FROM "vnet_bff_dev" LIMIT 20' }],
      fieldConfig: { defaults: {}, overrides: [] },
      options: {},
    },
    {
      id: 5, type: 'trend', title: 'trace spans', datasource: DS,
      gridPos: { h: 8, w: 12, x: 12, y: 18 },
      targets: [{ refId: 'A', datasource: DS, queryType: 'traces', format: 'table',
        rawSql: 'SELECT trace_id, service_name, operation_name, duration FROM "prd" ORDER BY start_time DESC LIMIT 20' }],
      fieldConfig: { defaults: {}, overrides: [] },
      options: {},
    },
  ],
};

const save = await page.request.post(`${GRAFANA}/api/dashboards/db`, {
  data: { dashboard, overwrite: true, message: 'smoke' },
  headers: { 'Content-Type': 'application/json' },
});
console.log('save dashboard:', save.status(), (await save.text()).slice(0, 120));

await page.goto(`${GRAFANA}/d/oo-smoke?orgId=1&from=now-1h&to=now&kiosk`, { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(12000);
await page.screenshot({ path: '.runtime/shot-dashboard.png', fullPage: false });

// Count rendered panels with content
const summaries = await page.evaluate(() => {
  const panels = Array.from(document.querySelectorAll('[data-viz-panel]'));
  return panels.map(p => ({
    title: p.getAttribute('data-viz-panel') || p.querySelector('[data-testid="data-testid Panel header"]')?.textContent,
    hasError: !!p.querySelector('[data-testid="data-testid panel status error"]'),
    hasNoData: /No data/i.test(p.textContent || ''),
  }));
});
console.log('panels:', JSON.stringify(summaries));
console.log('errors:', JSON.stringify(errors.slice(0, 6)));
await browser.close();
