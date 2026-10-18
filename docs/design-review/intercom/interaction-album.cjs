/* Nested album interaction coverage. Every write is intercepted by the runner. */
const tracks = [
  ['A Very Long Opening Track Name That Must Wrap Cleanly', 1, 1, 'in_library', 'In library'],
  ['Second Track', 1, 2, 'wanted', 'Missing'],
  ['Third Track', 1, 3, 'downloading', 'Downloading'],
  ['Fourth Track', 1, 4, 'wanted', 'Wanted'],
  ['Second Disc With An Intentionally Long Track Title', 2, 1, 'held', 'Needs a decision'],
  ['Final Track', 2, 2, 'wanted', 'Failed']
].map(([name, disc, number, state, state_label], i) => ({
  id: 701 + i, disc, number, name, length_ms: 180000 + i * 1000,
  recording_id: 'rec-' + (701 + i), keep_up_to_date: true, ids: {},
  slots: [{ id: i + 1, version: 'default', version_id: i + 1, state, state_label }]
}));
const pressings = [
  { id: 701, mbid: 'mbid-pressing-701', name: 'Low Orbit (Deluxe Edition)', status: 'official', country: 'US', date: '2024-05-01', formats: ['CD', 'Digital Media'], track_count: 6, chosen: true, chosen_by: 'person', tracks: [] },
  { id: 702, mbid: 'mbid-pressing-702', name: 'Low Orbit (Standard Edition)', status: 'official', country: 'GB', date: '2024-05-02', formats: ['Digital Media'], track_count: 5, chosen: false, tracks: [] }
];
const baseAlbum = { id: 7, mbid: 'mbid-album-7', name: 'Low Orbit', type: 'album', secondary_types: [], release_date: '2024-05-01T00:00:00Z', included: true, keep_up_to_date: true, pressings: 2, pressing: pressings[0].name, pressing_id: 701, tracks: 6, in_library: 1, missing: 5, better_wanted: 0, not_out_yet: 0, state: 'wanted' };

const scenario = {
  name: 'nested album tracks, pressing and recovery controls',
  route: '/library/76/albums/7', supportedWidths: [360, 834, 1440],
  setup: async (ctx) => {
    let album = { ...baseAlbum }; let currentPressings = pressings.map((p) => ({ ...p })); let searches = 0;
    await ctx.routeJSON('GET', '/albums/7', () => ({ album, title_id: 76, pressings: currentPressings }));
    await ctx.routeJSON('GET', '/albums/7/tracks', { tracks });
    await ctx.routeJSON('GET', '/titles/76', { id: 76, kind: 'artist', name: 'Copper Wren', year: 2022, ids: {}, keep_up_to_date: true, why_added: 'requested', release_gate: 'none', gate_delay_days: 0, stage: 'active', gate_open: true, gate_reason: '', artwork: [], versions: [], slots: [], asks: [], artist: { mbid: 'artist-76', status: 'running', chosen: true, new_albums: 'all', album_types: ['album'], secondary_types: [], tracks: [] } });
    await ctx.routeJSON('GET', '/events', { events: [], total: 0, limit: 8, offset: 0 });
    ctx.intercept(async ({ method, path, body }) => {
      if (method === 'POST' && path === '/titles/76/search') { searches++; return searches === 1 ? { status: 503, json: { error: 'Search service unavailable' }, delay: 120 } : { status: 200, json: { job_ids: [701] } }; }
      if (method === 'GET' && path === '/titles/76/releases') return { status: 200, json: { releases: [], total: 0, limit: 50, offset: 0 } };
      if (method === 'PUT' && path === '/albums/7') { album = { ...album, ...body, pressing_id: body.pressing_id ?? album.pressing_id, pressing: body.pressing_id === 702 ? pressings[1].name : album.pressing }; if (body.pressing_id) currentPressings = currentPressings.map((p) => ({ ...p, chosen: p.id === body.pressing_id, chosen_by: p.id === body.pressing_id ? 'person' : undefined })); return { status: 200, json: album }; }
      if (method === 'GET' && path === '/titles/76/rename') return { status: 200, json: { steps: [], plan_hash: 'album-plan-7' } };
      if (method === 'POST' && path === '/titles/76/delete') return { status: 200, json: { title_id: 76, name: 'Copper Wren', scope: 'album', applied: false, held: [], kept: [], holding_days: 7, active_downloads: 0, choice: { delete_files: false, never_add: false } } };
      return null;
    });
  },
  run: async (ctx) => {
    const p = ctx.page;
    await ctx.expect.poll(() => p.getByRole('heading', { name: 'Low Orbit', exact: true }).count()).toBe(1);
    await ctx.expect.poll(() => p.getByTestId('track').count()).toBe(6);
    await ctx.expect.poll(() => p.getByTestId('track-state').count()).toBe(6);
    await ctx.step('review track states', async () => ctx.capture('track-states'));
    const keep = p.getByRole('checkbox', { name: 'Keep up to date', exact: true });
    await ctx.expect.poll(() => keep.isChecked()).toBe(true);
    const search = p.getByTestId('album-search-now');
    await ctx.step('recover Search now after a rejected request', async () => { await search.click(); await ctx.expect.poll(() => p.getByText('Search service unavailable', { exact: false }).count()).toBe(1); await search.click(); await ctx.expect.poll(() => p.getByText('Searching for Low Orbit', { exact: false }).count()).toBe(1); });
    await ctx.expect.poll(() => ctx.attempts.filter((a) => a.method === 'POST' && a.path === '/titles/76/search').length).toBe(2);
    await p.getByTestId('album-choose').click(); await ctx.expect.poll(() => p.getByTestId('show-releases').count()).toBe(1); await p.getByRole('button', { name: 'Close', exact: true }).click();
    await p.locator('#pressing').selectOption('702'); const use = p.getByRole('button', { name: 'Use this Pressing', exact: true }); await use.click(); await ctx.expect.poll(() => p.getByTestId('chosen-pressing').getByText('Low Orbit (Standard Edition)', { exact: false }).count()).toBe(1); await ctx.expect.poll(() => use.isDisabled()).toBe(true);
    await p.locator('label.switch').click(); await ctx.expect.poll(() => keep.isChecked()).toBe(false);
    await p.getByTestId('rename-album').click(); await ctx.expect.poll(() => p.getByTestId('rename-preview').count()).toBe(1); await p.getByRole('button', { name: 'Close', exact: true }).last().click();
    await p.getByTestId('delete-album').click(); await ctx.expect.poll(() => p.getByTestId('delete-plan').count()).toBe(1); await p.getByRole('button', { name: 'Cancel', exact: true }).last().click(); await ctx.expect.poll(() => ctx.attempts.filter((a) => a.method === 'POST' && a.path === '/titles/76/delete').length).toBe(1);
  }
};
module.exports = [scenario];
