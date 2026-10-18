const assert = require('node:assert/strict');
const { candidateProfiles } = require('./real-save-candidates.cjs');

const kindQuery = { movie: 'movie', show: 'show', artist: 'music' };
const libraryKind = { movie: 'movie', show: 'show', artist: 'artist' };
const resultTestId = { show: 'show-hit', artist: 'artist-hit' };

function apiItems(value, key) {
	if (Array.isArray(value)) return value;
	return Array.isArray(value?.[key]) ? value[key] : [];
}

function matchesReference(item, reference) {
	const [source, id] = reference.split(':', 2);
	if (item?.ref === reference) return true;
	if (source === 'tmdb' && String(item?.tmdb_id ?? '') === id) return true;
	return String(item?.ids?.[source] ?? '') === id;
}

function findAsk(payload, metadata) {
	return apiItems(payload, 'asks').find((ask) => ask.kind === metadata.kind && (matchesReference(ask, metadata.reference) || ask.name === metadata.name));
}

function peopleIds(ask) {
	return [...new Set([ask.asked_by, ...(ask.people ?? [])].map((person) => person?.id).filter(Number.isInteger))];
}

function requesterNames(ask) {
	return [...new Set([ask.asked_by, ...(ask.people ?? [])].map((person) => person?.name).filter((name) => typeof name === 'string' && name.trim()))];
}

function resultOf(ask, metadata, state = ask.state) {
	assert.ok(Number.isInteger(ask.id), `${metadata.name} ask has an integer id`);
	assert.equal(ask.kind, metadata.kind, `${metadata.name} ask has the expected kind`);
	assert.equal(typeof ask.name, 'string', `${metadata.name} ask has a name`);
	assert.equal(typeof state, 'string', `${metadata.name} ask has a state`);
	const result = {
		id: ask.id,
		kind: ask.kind,
		reference: metadata.reference,
		name: ask.name,
		requester: peopleIds(ask),
		requesterNames: requesterNames(ask),
		state,
		titleId: Number.isInteger(ask.title_id) ? ask.title_id : null,
		peopleIds: peopleIds(ask),
		seasons: [],
		albums: [],
		titleMapped: Number.isInteger(ask.title_id)
	};
	if (ask.kind === 'show') {
		assert.ok(Array.isArray(ask.seasons), `${metadata.name} ask has a seasons array`);
		result.seasons = [...ask.seasons];
	}
	if (ask.kind === 'artist') {
		assert.ok(Array.isArray(ask.albums), `${metadata.name} ask has an albums array`);
		result.albums = ask.albums.map((album) => album.mbid);
	}
	return result;
}

function storeResult(results, ask, metadata, state) {
	const next = resultOf(ask, metadata, state);
	const index = results.requests.findIndex((request) => request.id === next.id);
	if (index >= 0) results.requests[index] = next;
	else results.requests.push(next);
	return next;
}

async function open(page, base, route) {
	await page.goto(new URL(route, base).href, { waitUntil: 'domcontentloaded' });
	await page.locator('body').waitFor();
}

function resultRow(page, metadata) {
	if (resultTestId[metadata.kind]) return page.getByTestId(resultTestId[metadata.kind]).filter({ hasText: metadata.name }).first();
	return page.locator('li.hit').filter({ hasText: metadata.name }).first();
}

async function search(page, base, metadata) {
	await open(page, base, `/search?kind=${kindQuery[metadata.kind]}`);
	await page.getByRole('searchbox').fill(metadata.name);
	await page.getByRole('button', { name: 'Search', exact: true }).click();
	const row = resultRow(page, metadata);
	await row.waitFor();
	return row;
}

async function readFreshAsk(ctx, page, metadata) {
	const ask = findAsk(await ctx.get(page, '/asks'), metadata);
	assert.ok(ask, `${metadata.name} appears in a fresh /asks read`);
	ctx.registerAsk(ask.id);
	return ask;
}

async function existingAsk(ctx, page, metadata) {
	const ask = findAsk(await ctx.get(page, '/asks'), metadata);
	if (!ask) return null;
	const previous = recordedResult(ctx, metadata);
	if (previous) assert.equal(ask.id, previous.id, `${metadata.name} resume uses the recorded request`);
	const allowedNames = new Set(Object.values(ctx.names));
	const names = requesterNames(ask);
	assert.ok(names.length > 0 && names.every((name) => allowedNames.has(name)), `${metadata.name} belongs only to this disposable run`);
	ctx.registerAsk(ask.id);
	storeResult(ctx.results, ask, metadata);
	return ask;
}

function recordedResult(ctx, metadata, state) {
	return ctx.results.requests.find((request) => request.reference === metadata.reference && (!state || request.state === state));
}

async function createMovieAsk(ctx, page, metadata) {
	const existing = await existingAsk(ctx, page, metadata);
	if (existing) return existing;
	const row = await search(page, ctx.base, metadata);
	await row.getByRole('button', { name: /^(Ask|Ask too)$/ }).click();
	await row.getByRole('link', { name: /You asked/ }).waitFor();
	const ask = await readFreshAsk(ctx, page, metadata);
	assert.equal(ask.state, 'asked', `${metadata.name} remains pending before an owner decision`);
	storeResult(ctx.results, ask, metadata);
	return ask;
}

async function createShowAsk(ctx, page, metadata) {
	const existing = await existingAsk(ctx, page, metadata);
	if (existing) {
		assert.deepEqual(existing.seasons, metadata.seasons, `${metadata.name} retains the selected season`);
		return existing;
	}
	const row = await search(page, ctx.base, metadata);
	await row.getByRole('button', { name: /^(Ask|Ask too)$/ }).click();
	const dialog = page.getByRole('dialog', { name: `Ask for ${metadata.name}` });
	await dialog.getByLabel('Only some seasons').check();
	await dialog.getByLabel('Season 1', { exact: true }).check();
	await dialog.getByRole('button', { name: 'Ask for season 1', exact: true }).click();
	await dialog.waitFor({ state: 'hidden' });
	const ask = await readFreshAsk(ctx, page, metadata);
	assert.deepEqual(ask.seasons, metadata.seasons, `${metadata.name} persists the selected season`);
	storeResult(ctx.results, ask, metadata);
	return ask;
}

async function createMusicAsk(ctx, page, metadata) {
	const existing = await existingAsk(ctx, page, metadata);
	if (existing) {
		assert.deepEqual(existing.albums.map((item) => item.mbid), metadata.albums, `${metadata.name} retains the selected album`);
		return existing;
	}
	const row = await search(page, ctx.base, metadata);
	await row.getByRole('button', { name: /^(Ask|Ask too)$/ }).click();
	const dialog = page.getByRole('dialog', { name: `Ask for ${metadata.name}` });
	await dialog.getByLabel('Only some albums').check();
	const album = dialog.getByTestId('ask-album').filter({ hasText: metadata.albumName });
	await album.getByRole('checkbox').check();
	await dialog.getByRole('button', { name: `Ask for ${metadata.albumName}`, exact: true }).click();
	await dialog.waitFor({ state: 'hidden' });
	const ask = await readFreshAsk(ctx, page, metadata);
	assert.deepEqual(ask.albums.map((item) => item.mbid), metadata.albums, `${metadata.name} persists the selected album`);
	storeResult(ctx.results, ask, metadata);
	return ask;
}

async function wishlistRow(page, base, metadata) {
	await open(page, base, '/wishlist');
	const row = page.getByTestId('ask').filter({ hasText: metadata.name });
	await row.waitFor();
	return row;
}

async function approve(ctx, metadata) {
	const existing = await existingAsk(ctx, ctx.owner, metadata);
	if (existing?.state === 'approved') {
		assert.ok(Number.isInteger(existing.title_id), `${metadata.name} existing approval links a Library title`);
		ctx.registerTitle(existing.title_id);
		return existing;
	}
	assert.ok(!existing || existing.state === 'asked', `${metadata.name} can only be approved from a pending state`);
	await ctx.owner.setViewportSize({ width: 1440, height: 900 });
	const row = await wishlistRow(ctx.owner, ctx.base, metadata);
	await row.getByRole('button', { name: 'Approve', exact: true }).click();
	const dialog = ctx.owner.getByRole('dialog', { name: new RegExp(`^Approve ${metadata.name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}`) });
	await dialog.getByLabel('Note (optional)').fill('Approved in the isolated preview check.');
	await dialog.getByRole('button', { name: 'Approve and add', exact: true }).click();
	await dialog.waitFor({ state: 'hidden' });
	const ask = await readFreshAsk(ctx, ctx.owner, metadata);
	assert.equal(ask.state, 'approved', `${metadata.name} is approved in a fresh request read`);
	assert.ok(Number.isInteger(ask.title_id), `${metadata.name} approval links a Library title`);
	ctx.registerTitle(ask.title_id);
	storeResult(ctx.results, ask, metadata);
	return ask;
}

async function freshTitle(ctx, ask, metadata) {
	const titles = apiItems(await ctx.get(ctx.owner, '/titles'), 'titles');
	const title = titles.find((item) => item.id === ask.title_id);
	assert.ok(title, `${metadata.name} appears in a fresh Library read`);
	const linked = (title.asks ?? []).find((item) => item.id === ask.id && item.title_id === title.id);
	assert.ok(linked, `${metadata.name} Library title links back to its request`);
	assert.equal(linked.state, 'approved', `${metadata.name} Library request is approved`);
	if (metadata.seasons) {
		assert.ok(Array.isArray(linked.seasons), `${metadata.name} Library link includes seasons`);
		assert.deepEqual(linked.seasons, metadata.seasons, `${metadata.name} Library link keeps the requested seasons`);
	}
	if (metadata.albums) {
		assert.ok(Array.isArray(linked.albums), `${metadata.name} Library link includes albums`);
		assert.deepEqual(linked.albums.map((album) => album.mbid), metadata.albums, `${metadata.name} Library link keeps the requested albums`);
	}
	const stored = ctx.results.requests.find((request) => request.id === ask.id);
	assert.ok(stored, `${metadata.name} has a recorded lifecycle result`);
	stored.titleMapped = true;
	stored.titleId = title.id;
	stored.titleName = title.name;
	return title;
}

async function verifyShowScope(ctx, title, metadata) {
	const detail = await ctx.get(ctx.owner, `/titles/${title.id}`);
	const seasons = detail.show?.seasons ?? [];
	assert.ok(seasons.length >= 1, `${metadata.name} exposes its requested season`);
	assert.equal(seasons.find((season) => season.number === 1)?.keep_up_to_date, true, `${metadata.name} keeps Season 1 up to date`);
	assert.equal(seasons.filter((season) => season.number !== 1).some((season) => season.keep_up_to_date), false, `${metadata.name} leaves other seasons off`);
	await ctx.owner.setViewportSize({ width: 834, height: 900 });
	await open(ctx.owner, ctx.base, `/library/${title.id}`);
	const season = ctx.owner.getByTestId('season').filter({ hasText: 'Season 1' });
	await season.scrollIntoViewIfNeeded();
	await ctx.expect(season.getByRole('checkbox', { name: 'Keep Season 1 up to date', exact: true })).toBeChecked();
	await ctx.capture(ctx.owner, `${metadata.name}-season-scope`);
}

async function verifyMusicScope(ctx, title, metadata) {
	const albums = apiItems(await ctx.get(ctx.owner, `/titles/${title.id}/albums`), 'albums');
	assert.ok(albums.length > 1, `${metadata.name} exposes selected and unselected albums`);
	const selected = albums.find((album) => album.mbid === metadata.albums[0]);
	assert.ok(selected, `${metadata.albumName} appears in the approved artist`);
	assert.equal(selected.keep_up_to_date, true, `${metadata.albumName} is kept up to date`);
	const unselected = albums.find((album) => album.name === metadata.unselectedAlbumName);
	assert.ok(unselected, `${metadata.unselectedAlbumName} appears as an unselected album`);
	assert.equal(unselected.keep_up_to_date, false, `${metadata.unselectedAlbumName} is not kept up to date`);
	assert.equal(albums.filter((album) => album.mbid !== metadata.albums[0]).some((album) => album.keep_up_to_date), false, `${metadata.name} leaves every unselected album off`);
	await ctx.owner.setViewportSize({ width: 1440, height: 900 });
	await open(ctx.owner, ctx.base, `/library/${title.id}`);
	const album = ctx.owner.getByTestId('album').filter({ hasText: metadata.albumName });
	await album.scrollIntoViewIfNeeded();
	await ctx.expect(album.getByRole('checkbox', { name: `Keep up to date: ${metadata.albumName}`, exact: true })).toBeChecked();
	await ctx.capture(ctx.owner, `${metadata.name}-album-scope`);
}

async function setLayout(ctx, page, name) {
	const button = page.getByRole('button', { name, exact: true });
	if (await button.getAttribute('aria-pressed') !== 'true') await button.click();
	await ctx.expect(button).toHaveAttribute('aria-pressed', 'true');
}

async function verifyLibraryAtWidths(ctx, title, metadata, requesterNames, options = {}) {
	for (const width of [360, 834, 1440]) {
		await ctx.owner.setViewportSize({ width, height: 900 });
		await open(ctx.owner, ctx.base, `/library?kind=${libraryKind[metadata.kind]}`);
		await setLayout(ctx, ctx.owner, 'Posters');
		const card = ctx.owner.getByTestId('library-card').filter({ hasText: title.name });
		await ctx.expect(card).toBeVisible();
		await card.scrollIntoViewIfNeeded();
		await ctx.expect(card).toHaveAttribute('href', new RegExp(`/library/${title.id}(?:\\?|$)`));
		await ctx.expect(card.getByTestId('request-approved')).toHaveText('Approved');
		for (const name of requesterNames) await ctx.expect(card.getByTestId('requested-by')).toContainText(name);
		if (options.requireStatus !== false) await ctx.expect.poll(() => card.locator('.chip').count()).toBeGreaterThan(1);
		await ctx.capture(ctx.owner, `${metadata.name}-${width}-card`);
		await setLayout(ctx, ctx.owner, 'List');
		const row = ctx.owner.getByTestId('library-row').filter({ hasText: title.name });
		await ctx.expect(row).toBeVisible();
		await row.scrollIntoViewIfNeeded();
		await ctx.expect(row.locator('a.row-link')).toHaveAttribute('href', new RegExp(`/library/${title.id}(?:\\?|$)`));
		await ctx.expect(row.getByTestId('request-approved')).toHaveText('Approved');
		for (const name of requesterNames) await ctx.expect(row.getByTestId('requested-by')).toContainText(name);
		if (options.requireStatus !== false) await ctx.expect.poll(() => row.locator('.chip').count()).toBeGreaterThan(1);
		await ctx.capture(ctx.owner, `${metadata.name}-${width}-list`);
	}
}

async function verifyRequesterFilter(ctx, title, names) {
	await ctx.owner.setViewportSize({ width: 360, height: 900 });
	await open(ctx.owner, ctx.base, '/library?kind=movie');
	await setLayout(ctx, ctx.owner, 'Posters');
	for (const name of names) {
		const menu = ctx.owner.locator('details.requester-menu');
		if (!(await menu.evaluate((element) => element.open))) await menu.locator('summary').click();
		const select = ctx.owner.getByLabel('Requested by', { exact: true });
		const option = select.locator('option').filter({ hasText: name }).first();
		const value = await option.getAttribute('value');
		assert.ok(value, `Requested by offers ${name}`);
		await select.selectOption(value);
		const card = ctx.owner.getByTestId('library-card').filter({ hasText: title.name });
		await ctx.expect(card).toBeVisible();
		await card.scrollIntoViewIfNeeded();
		await ctx.expect(ctx.owner.getByRole('button', { name: 'Remove requester filter' })).toContainText(name);
	}
}

async function ensureNoLibraryTitle(ctx, metadata) {
	const titles = apiItems(await ctx.get(ctx.owner, '/titles'), 'titles');
	assert.equal(titles.some((title) => matchesReference(title, metadata.reference) || title.name === metadata.name), false, `${metadata.name} is not in the Library`);
}

async function saveAlerts(page, button) {
	const responsePromise = page.waitForResponse((response) => response.request().method() === 'PUT' && new URL(response.url()).pathname.endsWith('/api/v1/my/alerts'));
	await button.click();
	const response = await responsePromise;
	assert.ok(response.ok(), `saving Alerts succeeds (${response.status()})`);
	await button.waitFor({ state: 'visible' });
}

async function saveAndRestoreMemberAlerts(ctx) {
	await open(ctx.member, ctx.base, '/account');
	const alerts = ctx.member.locator('section').filter({ has: ctx.member.getByRole('heading', { name: 'Alerts for you' }) });
	const email = alerts.getByLabel('Email address');
	const save = alerts.getByRole('button', { name: 'Save', exact: true });
	await email.fill('preview-e2e@example.test');
	await saveAlerts(ctx.member, save);
	await ctx.member.getByText('Saved where Alerts reach you.').waitFor();
	await ctx.member.reload({ waitUntil: 'domcontentloaded' });
	await ctx.expect(alerts.getByLabel('Email address')).toHaveValue('preview-e2e@example.test');
	await alerts.getByLabel('Email address').fill('');
	await saveAlerts(ctx.member, alerts.getByRole('button', { name: 'Save', exact: true }));
	await ctx.member.reload({ waitUntil: 'domcontentloaded' });
	await ctx.expect(alerts.getByLabel('Email address')).toHaveValue('');
}

async function markMemberMessagesRead(ctx) {
	await open(ctx.member, ctx.base, '/wishlist');
	const before = await ctx.get(ctx.member, '/messages');
	const button = ctx.member.getByRole('button', { name: 'Mark all read', exact: true });
	if (before.unread > 0) {
		await ctx.expect(button).toBeVisible();
		const saved = ctx.member.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname.endsWith('/api/v1/messages/read'));
		await button.click();
		assert.ok((await saved).ok(), 'mark-all-read save is accepted');
		await button.waitFor({ state: 'hidden' });
	}
	await ctx.member.reload({ waitUntil: 'domcontentloaded' });
	await ctx.expect(ctx.member.getByRole('button', { name: 'Mark all read', exact: true })).toHaveCount(0);
	const messages = await ctx.get(ctx.member, '/messages');
	assert.equal(messages.unread, 0, 'member messages remain read after refresh');
	ctx.results.messageRead = { initialUnread: before.unread, persistedUnread: messages.unread };
}

module.exports = async function run(ctx) {
	const references = candidateProfiles[ctx.profileName]?.references;
	assert.ok(references, `real-save candidate profile exists: ${ctx.profileName}`);
	if (candidateProfiles[ctx.profileName].musicOnly) {
		await ctx.step('fresh music album request approves and persists in Library', async () => {
			await ctx.member.setViewportSize({ width: 390, height: 900 });
			const request = await createMusicAsk(ctx, ctx.member, references.music);
			const memberRow = await wishlistRow(ctx.member, ctx.base, references.music);
			await ctx.expect(memberRow.getByRole('button', { name: 'Approve', exact: true })).toHaveCount(0);
			await ctx.capture(ctx.member, `music-album-${request.state}`);
			const approved = await approve(ctx, references.music);
			assert.deepEqual(approved.albums.map(album => album.mbid), references.music.albums);
			const title = await freshTitle(ctx, approved, references.music);
			await verifyMusicScope(ctx, title, references.music);
			// Approval creates the artist before background track metadata is ready.
			await ctx.expect.poll(async () => {
				const fresh = apiItems(await ctx.get(ctx.owner, '/titles'), 'titles').find(item => item.id === title.id);
				return fresh?.artist?.missing ?? 0;
			}).toBeGreaterThan(0);
			await verifyLibraryAtWidths(ctx, title, references.music, [ctx.names.member]);
			await ctx.owner.reload({ waitUntil: 'domcontentloaded' });
			await freshTitle(ctx, approved, references.music);
			await ctx.capture(ctx.owner, 'music-library-refreshed');
		});
		assert.equal(ctx.results.requests.length, 1);
		return;
	}
	await ctx.step('member alert destination saves, reloads, and restores cleanly', async () => {
		await saveAndRestoreMemberAlerts(ctx);
	});

	await ctx.step('movie request joins, restricts member decisions, and reaches Library', async () => {
		await ctx.member.setViewportSize({ width: 360, height: 900 });
		const first = await createMovieAsk(ctx, ctx.member, references.movie);
		await wishlistRow(ctx.member, ctx.base, references.movie);
		await ctx.capture(ctx.member, `movie-${first.state}-member`);
		await ctx.second.setViewportSize({ width: 360, height: 900 });
		const joined = await createMovieAsk(ctx, ctx.second, references.movie);
		assert.equal(joined.id, first.id, 'both members share one movie request');
		assert.ok(peopleIds(joined).length >= 2, 'joined movie request records both people');
		ctx.registerAsk(joined.id);
		storeResult(ctx.results, joined, references.movie);
		const memberRow = await wishlistRow(ctx.member, ctx.base, references.movie);
		await ctx.expect(memberRow.getByRole('button', { name: 'Approve', exact: true })).toHaveCount(0);
		await ctx.expect(memberRow.getByRole('button', { name: 'Turn down', exact: true })).toHaveCount(0);
		const approved = await approve(ctx, references.movie);
		const title = await freshTitle(ctx, approved, references.movie);
		await ctx.capture(ctx.owner, 'movie-approved-owner');
		await verifyLibraryAtWidths(ctx, title, references.movie, [ctx.names.member, ctx.names.second]);
		await verifyRequesterFilter(ctx, title, [ctx.names.member, ctx.names.second]);
	});

	await ctx.step('show season request approves with its requested scope', async () => {
		await ctx.member.setViewportSize({ width: 834, height: 900 });
		const request = await createShowAsk(ctx, ctx.member, references.show);
		await wishlistRow(ctx.member, ctx.base, references.show);
		await ctx.capture(ctx.member, `show-season-${request.state}`);
		const approved = await approve(ctx, references.show);
		assert.deepEqual(approved.seasons, [1], 'approved show keeps only Season 1');
		const title = await freshTitle(ctx, approved, references.show);
		await verifyShowScope(ctx, title, references.show);
		await verifyLibraryAtWidths(ctx, title, references.show, [ctx.names.member]);
	});

	if (references.music) {
		await ctx.step('music album request approves with its requested scope', async () => {
			await ctx.member.setViewportSize({ width: 1440, height: 900 });
			const request = await createMusicAsk(ctx, ctx.member, references.music);
			await wishlistRow(ctx.member, ctx.base, references.music);
			await ctx.capture(ctx.member, `music-album-${request.state}`);
			const approved = await approve(ctx, references.music);
			assert.deepEqual(approved.albums.map((album) => album.mbid), references.music.albums, `approved artist keeps only ${references.music.albumName}`);
			const title = await freshTitle(ctx, approved, references.music);
			await verifyMusicScope(ctx, title, references.music);
			await verifyLibraryAtWidths(ctx, title, references.music, [ctx.names.member], { requireStatus: false });
		});
	} else {
		assert.equal(typeof ctx.results.musicUnavailable, 'string', 'omitted music lifecycle records why it is unavailable');
	}

	await ctx.step('turned-down movie persists its note without entering Library', async () => {
		await ctx.member.setViewportSize({ width: 360, height: 900 });
		const note = 'Not part of this preview Library check.';
		const pending = await createMovieAsk(ctx, ctx.member, references.denied);
		if (pending.state === 'asked') {
			const ownerRow = await wishlistRow(ctx.owner, ctx.base, references.denied);
			await ownerRow.getByRole('button', { name: 'Turn down', exact: true }).click();
			const dialog = ctx.owner.getByRole('dialog', { name: new RegExp(`^Turn down ${references.denied.name}`) });
			await dialog.getByLabel('Why (sent to the people who asked)').fill(note);
			await dialog.getByRole('button', { name: 'Turn down', exact: true }).click();
			await dialog.waitFor({ state: 'hidden' });
		} else {
			assert.equal(pending.state, 'turned_down', `${references.denied.name} resumed in its completed state`);
		}
		const denied = await readFreshAsk(ctx, ctx.owner, references.denied);
		assert.equal(denied.state, 'turned_down', 'owner decision persists as turned down');
		assert.equal(denied.note, note, 'turn-down note persists');
		storeResult(ctx.results, denied, references.denied);
		const memberRow = await wishlistRow(ctx.member, ctx.base, references.denied);
		await ctx.expect(memberRow.getByTestId('ask-state')).toHaveText('Turned down');
		await ctx.expect(memberRow).toContainText(note);
		await ctx.capture(ctx.member, 'movie-denied-member');
		await ensureNoLibraryTitle(ctx, references.denied);
	});

	await ctx.step('member withdraws a pending movie without entering Library', async () => {
		await ctx.member.setViewportSize({ width: 360, height: 900 });
		const completed = recordedResult(ctx, references.withdrawn, 'withdrawn');
		if (completed) {
			assert.equal(apiItems(await ctx.get(ctx.member, '/asks'), 'asks').some((ask) => ask.id === completed.id), false, 'previously withdrawn request remains absent');
			await ensureNoLibraryTitle(ctx, references.withdrawn);
			return;
		}
		const pending = await createMovieAsk(ctx, ctx.member, references.withdrawn);
		await ctx.capture(ctx.member, 'movie-withdraw-pending');
		const row = await wishlistRow(ctx.member, ctx.base, references.withdrawn);
		ctx.member.once('dialog', (dialog) => dialog.accept());
		await row.getByRole('button', { name: 'Take back', exact: true }).click();
		await row.waitFor({ state: 'detached' });
		assert.equal(apiItems(await ctx.get(ctx.member, '/asks'), 'asks').some((ask) => ask.id === pending.id), false, 'withdrawn request is absent from a fresh request read');
		storeResult(ctx.results, pending, references.withdrawn, 'withdrawn');
		await ctx.capture(ctx.member, 'movie-withdrawn-member');
		await ensureNoLibraryTitle(ctx, references.withdrawn);
	});

	await ctx.step('new member marks lifecycle messages read and refresh keeps them read', async () => {
		await markMemberMessagesRead(ctx);
	});

	const expectedLifecycleCount = references.music ? 5 : 4;
	assert.equal(ctx.results.requests.length, expectedLifecycleCount, `results record all ${expectedLifecycleCount} genuine request lifecycles`);
};
