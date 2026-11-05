/**
 * Records the README feature tour.
 *
 * Every frame is a real screenshot of a live Grafana + OpenObserve; there is no
 * mock or animation. Frames are written to tests/live/tour/frames and assembled
 * by make_tour.py next to this file.
 *
 * Usage:
 *   GRAFANA_URL=http://127.0.0.1:3000 DS_UID=<uid> node tests/live/tour/record.mjs
 */
import { chromium } from 'playwright';
import fs from 'node:fs';
import path from 'node:path';

const G = process.env.GRAFANA_URL || 'http://127.0.0.1:3000';
const DS = { type: 'linpr-openobserve-datasource', uid: process.env.DS_UID || '' };
const OUT = path.resolve('tests/live/tour/frames');

fs.rmSync(OUT, { recursive: true, force: true });
fs.mkdirSync(OUT, { recursive: true });

const browser = await chromium.launch({ executablePath: '/usr/bin/google-chrome', args: ['--no-sandbox'] });
const ctx = await browser.newContext({ viewport: { width: 1280, height: 760 }, deviceScaleFactor: 1 });
const page = await ctx.newPage();

const errors = [];
page.on('pageerror', e => errors.push(e.message));

let frame = 0;
const fps = 10;

/** Capture one frame; `hold` seconds of stillness is expressed as repeats. */
async function grab(holdFrames = 1) {
  for (let i = 0; i < holdFrames; i++) {
    const name = `${OUT}/f${String(frame).padStart(4, '0')}.png`;
    await page.screenshot({ path: name });
    frame++;
  }
}

/** Capture a slow pan down a scrolling region so motion reads on a static view. */
async function scrollCapture(scrollPx = 400, steps = 14, holdFrames = 2) {
  const step = Math.round(scrollPx / steps);
  for (let i = 0; i < steps; i++) {
    await page.mouse.wheel(0, step);
    await page.waitForTimeout(120);
    await grab(1);
  }
  await grab(holdFrames);
}

async function exploreUrl(q) {
  const left = {
    datasource: 'linpr-openobserve-datasource',
    queries: [{ refId: 'A', datasource: DS, ...q }],
    range: { from: 'now-6h', to: 'now' },
  };
  return `${G}/explore?orgId=1&left=${encodeURIComponent(JSON.stringify(left))}`;
}

// ---------- 0. login (not recorded: the tour starts on the product) ----------
await page.goto(`${G}/login`, { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(1200);
await page.fill('input[name="user"]', 'admin');
await page.fill('input[name="password"]', 'admin');
await page.click('button[type="submit"]');
await page.waitForTimeout(3000);

// ---------- 2. logs in Explore ----------
await page.goto(await exploreUrl({ queryType: 'logs', rawSql: 'SELECT * FROM "vnet_bff_dev" LIMIT 50', format: 'logs' }),
  { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(9000);
await grab(6);
await scrollCapture(520, 16, 3);

// ---------- 3. SQL metrics: multi-series ----------
await page.goto(await exploreUrl({
  queryType: 'metrics',
  rawSql: "SELECT histogram(_timestamp, '1 minute') AS ts, avg(value) AS \"value\", service_name FROM \"go_goroutine_count\" GROUP BY ts, service_name ORDER BY ts",
  format: 'timeseries',
}), { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(9000);
await grab(12);

// ---------- 4. PromQL ----------
await page.goto(await exploreUrl({
  queryType: 'metrics', queryMode: 'promql', expr: 'go_goroutine_count', format: 'timeseries',
}), { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(9000);
await grab(12);

// ---------- 5. trace waterfall ----------
await page.goto(await exploreUrl({
  queryType: 'traces',
  rawSql: "SELECT * FROM \"prd\" WHERE trace_id = 'ccfda4fa3d171e97071e9ff154cbefa9' ORDER BY start_time LIMIT 500",
  format: 'trace',
}), { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(10000);
await grab(8);
await scrollCapture(360, 12, 2);

// ---------- 6. dashboard (mixed) ----------
// The showcase and variables dashboards are created by ui_dash_check.mjs and
// record_vars.mjs; the tour only visits them.
await page.goto(`${G}/d/oo-showcase?orgId=1&from=now-6h&to=now&kiosk`, { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(12000);
await grab(8);
await scrollCapture(700, 20, 3);

// ---------- 7. variables dashboard ----------
await page.goto(`${G}/d/oo-vars?orgId=1&from=now-6h&to=now&kiosk`, { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(10000);
await grab(10);

// ---------- 8. config editor ----------
await page.goto(`${G}/connections/datasources/edit/P4A5AAF418F5D059C`, { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(5000);
await grab(6);
await scrollCapture(600, 18, 3);

console.log('frames:', frame, 'fps:', fps, 'errors:', JSON.stringify(errors.slice(0, 3)));
fs.writeFileSync(`${OUT}/meta.txt`, String(fps));
await browser.close();
