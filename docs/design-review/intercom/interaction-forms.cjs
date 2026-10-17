// Real page components, with browser-only responses for actions and jobs.
const folder = { id: 99001, kind: 'movie', label: 'Preview movies', path: '/preview/movies', health: 'ok' };

module.exports = [
  {
    name: 'Home metric and search navigation', route: '/',
    async run({ page, expect, step }) {
      await step('Library metric opens the Library', async () => {
        await page.getByRole('navigation', { name: 'Library overview' }).getByRole('link', { name: /Library titles/ }).click();
        await expect(page.getByRole('heading', { name: 'Library', exact: true, level: 1 })).toBeVisible();
      });
      await step('back restores Home and Search media navigates', async () => {
        await page.goBack();
        await page.getByRole('link', { name: 'Search media', exact: true }).first().click();
        await expect(page.getByRole('searchbox')).toBeVisible();
      });
    }
  },
  {
    name: 'Import scan review selection and rejected adoption', route: '/library/import',
    async setup(ctx) {
      ctx.routeJSON('GET', '/library-folders', { library_folders: [folder] });
      ctx.routeJSON('POST', '/library-folders/99001/scan', { job_id: 99002 });
      ctx.routeJSON('GET', '/jobs/99002', {
        ID: 99002, State: 'done', Progress: 1,
        Result: { plan: { library_folder_id: folder.id, kind: 'movie', path: folder.path, counts: { matched: 1 }, limit: 20, more: false,
          folders: [{ folder: 'Arrival (2016)', parsed_name: 'Arrival', parsed_year: 2016, main: '/preview/movies/Arrival/movie.mkv',
            videos: [{ path: '/preview/movies/Arrival/movie.mkv', size: 1000000000 }], match: { tmdb_id: '329865', name: 'Arrival', year: 2016, score: 100 },
            others: [], status: 'matched', why: 'Exact name and year.' }]
        } }
      });
      ctx.intercept(async ({ method, path }) => method === 'POST' && path === '/library-folders/99001/adopt'
        ? { status: 503, delay: 200, json: { error: 'Preview adoption intentionally rejected.' } } : null);
    },
    async run({ page, expect, step, capture }) {
      await step('scan produces a review without changing files', async () => {
        await page.getByRole('button', { name: 'Scan (nothing is moved)' }).click();
        await expect(page.getByTestId('adopt-plan')).toBeVisible();
        const selected = page.getByRole('checkbox', { name: 'Adopt Arrival (2016)' });
        await expect(selected).toBeChecked();
        await selected.uncheck();
        await expect(page.getByRole('button', { name: /^Adopt/ })).toBeDisabled();
        await selected.check();
      });
      await capture('scan-review');
      await step('failed adoption preserves the review for recovery', async () => {
        const submit = page.getByRole('button', { name: /^Adopt/ });
        await submit.click();
        await expect(submit).toBeDisabled();
        await expect(page.getByRole('alert')).toHaveText('Preview adoption intentionally rejected.');
        await expect(submit).toBeEnabled();
        await expect(page.getByTestId('adopt-plan')).toBeVisible();
      });
    }
  },
  {
    name: 'Source health period picker', route: '/settings/sources/99001',
    async setup(ctx) {
      ctx.intercept(async ({ method, path, url }) => method === 'GET' && path === '/sources/99001/stats' ? { json: {
        source: { id: 99001, name: 'Preview Source', format: 'torznab', protocol: 'torrent', priority: 1, feed: true, search: true, auto: true,
          health: { state: 'healthy', text: 'Answering.' }, summary: { avg_ms_24h: 120 } },
        days: [{ day: '2026-10-17', calls: Number(url.searchParams.get('days')), failures: 0, grabs: 1, avg_ms: 120 }], recent: []
      } } : null);
    },
    async run({ page, expect }) {
      await expect(page.getByRole('heading', { name: 'Preview Source' })).toBeVisible();
      await page.getByLabel('Days shown').selectOption('30');
      await expect(page.getByText('30 calls', { exact: true })).toBeVisible();
      await page.getByLabel('Days shown').selectOption('7');
      await expect(page.getByText('7 calls', { exact: true })).toBeVisible();
    }
  },
  {
    name: 'This app check, update failure and settings recovery', route: '/settings/this-app',
    async setup(ctx) {
      ctx.routeJSON('GET', '/health', { ok: true, version: 'preview', schema: 1, uptime_s: 60, writes: 0, write_wait_p95_ms: 0, live_clients: 1 });
      ctx.routeJSON('GET', '/health/checks', { checks: [] });
      ctx.routeJSON('GET', '/health/states', { states: [] });
      ctx.routeJSON('GET', '/settings/health', { every_minutes: 30, flap_guard: 2, low_space_mb: 1024, update_check: false, update_check_url: '', update_check_default: 'https://example.invalid/releases', update_check_local: false, update_prerelease: false });
      ctx.routeJSON('GET', '/app/update', { checked: false, newer: false, version: 'preview', on: true });
      ctx.intercept(async ({ method, path, body }) => {
        if (method === 'POST' && path === '/health/checks/run') return { delay: 200, json: { checks: [] } };
        if (method === 'POST' && path === '/app/update/check') return { status: 503, delay: 200, json: { error: 'Preview release feed unavailable.' } };
        if (method === 'PUT' && path === '/settings/health') return { delay: 200, json: { every_minutes: 45, flap_guard: 2, low_space_mb: 1024, update_check: false, update_check_url: '', update_check_default: 'https://example.invalid/releases', update_check_local: false, update_prerelease: false } };
        return null;
      });
    },
    async run({ page, expect, step }) {
      await step('health check exposes a pending state', async () => {
        const check = page.getByRole('button', { name: 'Check again', exact: true });
        await check.click(); await expect(check).toBeDisabled(); await expect(check).toBeEnabled();
      });
      await step('rejected update check stays recoverable', async () => {
        const check = page.getByRole('button', { name: 'Check for updates now' });
        await check.click(); await expect(check).toBeDisabled();
        await expect(page.getByText('Preview release feed unavailable.', { exact: true })).toBeVisible();
        await expect(check).toBeEnabled();
      });
      await step('valid health settings submit succeeds in browser fixture', async () => {
        await page.getByLabel('Check every (minutes)').fill('45');
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        await expect(page.getByText('Health check settings saved.', { exact: true })).toBeVisible();
        await expect(page.getByLabel('Check every (minutes)')).toHaveValue('45');
      });
    }
  }
];
