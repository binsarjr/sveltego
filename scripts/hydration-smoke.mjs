#!/usr/bin/env node
// Hydration-parity smoke runner (#446).
//
// Loads each route in headless Chromium, waits for the client entry to
// set `window.__sveltego_hydrated = true`, and asserts no Svelte
// hydration-mismatch warnings landed on the console. A non-zero exit
// fails the playground-smoke job.
//
// Usage:
//   node scripts/hydration-smoke.mjs --base http://localhost:3000 \
//        --routes /,/post/123
//
// Self-test mode proves the gate fires. It injects a synthetic
// hydration_mismatch warning into the page console and expects the
// run to fail; pass --self-test to flip the exit code so CI can
// verify the gate.
//
//   node scripts/hydration-smoke.mjs --base http://localhost:3000 \
//        --routes / --self-test

import { argv, exit } from 'node:process';

const HYDRATION_MARKERS = [
  'hydration_mismatch',
  'hydration_attribute_changed',
  'hydration_failed',
  '[svelte] hydration',
];

const READY_TIMEOUT_MS = 15_000;
const SETTLE_MS = 250;

function parseArgs(args) {
  const out = { base: '', routes: [], crossNav: [], selfTest: false, retries: 2, playwrightFrom: '' };
  for (let i = 2; i < args.length; i++) {
    const a = args[i];
    if (a === '--base') out.base = args[++i];
    else if (a === '--routes') out.routes = args[++i].split(',').map((s) => s.trim()).filter(Boolean);
    else if (a === '--cross-nav') out.crossNav = args[++i].split(',').map((s) => s.trim()).filter(Boolean);
    else if (a === '--self-test') out.selfTest = true;
    else if (a === '--retries') out.retries = Number(args[++i]);
    else if (a === '--playwright-from') out.playwrightFrom = args[++i];
    else if (a === '--help' || a === '-h') {
      console.log('usage: hydration-smoke.mjs --base URL --routes /a,/b [--cross-nav /a,/b] [--self-test] [--playwright-from DIR]');
      exit(0);
    } else throw new Error(`unknown arg: ${a}`);
  }
  if (!out.base) throw new Error('--base is required');
  if (out.routes.length === 0) throw new Error('--routes is required');
  return out;
}

function looksLikeMismatch(text) {
  const lower = String(text).toLowerCase();
  return HYDRATION_MARKERS.some((m) => lower.includes(m.toLowerCase()));
}

async function loadPlaywright(playwrightFrom) {
  try {
    if (playwrightFrom) {
      const { pathToFileURL } = await import('node:url');
      const path = await import('node:path');
      const entry = path.resolve(playwrightFrom, 'playwright', 'index.mjs');
      const mod = await import(pathToFileURL(entry).href);
      return mod.chromium;
    }
    const mod = await import('playwright');
    return mod.chromium;
  } catch (err) {
    console.error('hydration-smoke: playwright is required.');
    console.error('install with: npx playwright install --with-deps chromium');
    console.error('underlying error:', err.message);
    exit(2);
  }
}

async function checkRoute(browser, baseUrl, route, { selfTest }) {
  const url = new URL(route, baseUrl).toString();
  const ctx = await browser.newContext();
  const page = await ctx.newPage();
  const warnings = [];
  const errors = [];

  page.on('console', (msg) => {
    const t = msg.type();
    const text = msg.text();
    if (t === 'warning' || t === 'error') warnings.push({ type: t, text });
    if (looksLikeMismatch(text)) warnings.push({ type: t, text, mismatch: true });
  });
  page.on('pageerror', (err) => errors.push(err.message));

  const t0 = Date.now();
  try {
    await page.goto(url, { waitUntil: 'domcontentloaded', timeout: READY_TIMEOUT_MS });

    if (selfTest) {
      // Self-test does not require a hydrating client — it verifies the
      // harness reacts to a synthetic hydration warning. Wait for the
      // marker only when running against real routes.
      await page.evaluate(() => {
        console.warn('[svelte] hydration_mismatch synthetic regression');
      });
      await page.waitForTimeout(50);
    } else {
      await page.waitForFunction(() => window.__sveltego_hydrated === true, null, {
        timeout: READY_TIMEOUT_MS,
      });
      await page.waitForTimeout(SETTLE_MS);
    }

    const mismatches = warnings.filter((w) => w.mismatch || looksLikeMismatch(w.text));
    const elapsedMs = Date.now() - t0;

    return { url, mismatches, errors: [...errors], elapsedMs };
  } finally {
    await ctx.close();
  }
}

async function checkWithRetry(browser, baseUrl, route, opts) {
  let last;
  for (let i = 0; i <= opts.retries; i++) {
    try {
      const res = await checkRoute(browser, baseUrl, route, opts);
      if (res.mismatches.length === 0 && res.errors.length === 0) return res;
      last = res;
      if (opts.selfTest) return res;
      console.error(`  retry ${i + 1}: ${res.mismatches.length} mismatch / ${res.errors.length} pageerror`);
    } catch (err) {
      last = { url: route, mismatches: [], errors: [err.message], elapsedMs: 0 };
      console.error(`  retry ${i + 1}: ${err.message}`);
    }
  }
  return last;
}

async function checkCrossNav(browser, baseUrl, routes, opts) {
  if (routes.length < 2) {
    return { ok: true };
  }
  const ctx = await browser.newContext();
  const page = await ctx.newPage();
  const consoleMsgs = [];
  page.on('console', (msg) => consoleMsgs.push({ type: msg.type(), text: msg.text() }));
  page.on('pageerror', (err) => consoleMsgs.push({ type: 'pageerror', text: err.message }));
  try {
    const startUrl = new URL(routes[0], baseUrl).toString();
    await page.goto(startUrl, { waitUntil: 'domcontentloaded', timeout: READY_TIMEOUT_MS });
    await page.waitForFunction(() => window.__sveltego_hydrated === true, null, {
      timeout: READY_TIMEOUT_MS,
    });
    for (let i = 1; i < routes.length; i++) {
      const target = new URL(routes[i], baseUrl).toString();
      await page.evaluate(async (href) => {
        await window.__sveltego_router__.goto(href);
      }, target);
      await page.waitForFunction(
        (path) => location.pathname + location.search === path,
        new URL(target).pathname + new URL(target).search,
        { timeout: READY_TIMEOUT_MS },
      );
      await page.waitForTimeout(SETTLE_MS);
    }
    const mismatches = consoleMsgs.filter((m) => looksLikeMismatch(m.text));
    const errors = consoleMsgs.filter((m) => m.type === 'pageerror').map((m) => m.text);
    return { ok: mismatches.length === 0 && errors.length === 0, mismatches, errors };
  } finally {
    await ctx.close();
  }
}

async function main() {
  const opts = parseArgs(argv);
  const chromium = await loadPlaywright(opts.playwrightFrom);
  const browser = await chromium.launch({ headless: true });

  let failed = false;
  try {
    for (const route of opts.routes) {
      console.log(`hydration-smoke: ${opts.base}${route}`);
      const res = await checkWithRetry(browser, opts.base, route, opts);
      if (res.mismatches.length > 0) {
        console.error(`  FAIL ${route}: ${res.mismatches.length} hydration warning(s)`);
        for (const w of res.mismatches) {
          console.error(`    [${w.type}] ${w.text}`);
        }
        failed = true;
      } else if (res.errors.length > 0) {
        console.error(`  FAIL ${route}: page errors`);
        for (const e of res.errors) console.error(`    ${e}`);
        failed = true;
      } else {
        console.log(`  OK   ${route} (${res.elapsedMs}ms)`);
      }
    }

    if (opts.crossNav.length >= 2) {
      console.log(`hydration-smoke: cross-nav ${opts.crossNav.join(' -> ')}`);
      const res = await checkCrossNav(browser, opts.base, opts.crossNav, opts);
      if (!res.ok) {
        if (res.mismatches?.length) {
          console.error(`  FAIL cross-nav: ${res.mismatches.length} hydration warning(s)`);
          for (const m of res.mismatches) console.error(`    [${m.type}] ${m.text}`);
        }
        if (res.errors?.length) {
          console.error('  FAIL cross-nav: page errors');
          for (const e of res.errors) console.error(`    ${e}`);
        }
        failed = true;
      } else {
        console.log('  OK   cross-nav');
      }
    }
  } finally {
    await browser.close();
  }

  if (opts.selfTest) {
    if (!failed) {
      console.error('self-test: expected synthetic mismatch to fail the gate, but no mismatch detected.');
      exit(3);
    }
    console.log('self-test: synthetic mismatch was caught (gate works).');
    exit(0);
  }

  exit(failed ? 1 : 0);
}

main().catch((err) => {
  console.error('hydration-smoke: fatal:', err);
  exit(2);
});
