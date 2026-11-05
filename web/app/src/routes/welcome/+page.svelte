<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Icon, { type IconName } from '#lib/Icon.svelte';
	import ConnectBridge from '../settings/sources/ConnectBridge.svelte';
	import { page } from '$app/state';
	import { goto, replaceState } from '$app/navigation';
	import { onMount } from 'svelte';
	import { loaderReady, playLoader, SETUP_HOLD_MS } from '#lib/loader.ts';
	import { api, message } from '#lib/api.ts';
	import { to } from '#lib/paths.ts';
	import { session } from '#lib/session.svelte.ts';
	import { partsNow, refreshParts } from '#lib/parts.svelte.ts';
	import { needsProblem, type PartName } from '#lib/parts.ts';
	import { applyParts } from '#lib/controls.ts';
	import { theme } from '#lib/theme.svelte.ts';
	import { emptyForm, formBody, kindWords, testVersion, type DownloaderForm, type DownloaderView } from '#lib/downloaders.ts';
	import type { LibraryFolder } from '#lib/types.ts';
	const mark = $derived(theme.value === 'light' ? '/brand/mark-color.svg' : '/brand/mark-color-on-dark.svg');

	// First run, right after the owner is made, is one fixed path: parts,
	// Library folders, a Downloader, a Source, then an optional import from
	// other apps, then Home. Steps 3-5 can be skipped. A later visit to
	// /welcome (no first=1) changes only the parts and returns to Home.
	type Step = 'parts' | 'folders' | 'downloader' | 'source' | 'import';
	const ORDER: Step[] = ['parts', 'folders', 'downloader', 'source', 'import'];
	const firstRun = $derived(page.url.searchParams.get('first') === '1');
	function stepFromUrl(): Step {
		const s = page.url.searchParams.get('step');
		if (s === 'folder') return 'folders';
		return ORDER.includes(s as Step) ? (s as Step) : 'parts';
	}
	// The step is kept in the address so a reload stays on it.
	let step = $state<Step>(stepFromUrl());
	let busy = $state(false);
	let error = $state('');

	function go(s: Step) {
		error = '';
		step = s;
		replaceState(to(s === 'parts' ? '/welcome?first=1' : `/welcome?first=1&step=${s}`), page.state);
		void enter(s);
	}
	const stepNo = $derived(ORDER.indexOf(step) + 1);
	const stepCount = ORDER.length;
	const prev = $derived(ORDER[ORDER.indexOf(step) - 1]);

	// Setup is done: a short Orbit while Home opens.
	async function home() {
		playLoader('', SETUP_HOLD_MS);
		await goto(to('/'), { replace: true }).finally(loaderReady);
	}

	// ---- 1. Parts ----
	let want = $state<Record<'movies' | 'shows' | 'music' | 'subtitles', boolean>>({ movies: true, shows: true, music: false, subtitles: true });
	let family = $state<'yes' | 'no'>('no');
	const choices: { part: 'movies' | 'shows' | 'music' | 'subtitles'; label: string; icon: IconName }[] = [
		{ part: 'movies', label: 'Movies', icon: 'film' },
		{ part: 'shows', label: 'TV shows', icon: 'tv' },
		{ part: 'music', label: 'Music', icon: 'music' },
		{ part: 'subtitles', label: 'Subtitles', icon: 'subtitle' }
	];
	const full = $derived<Record<PartName, boolean>>({ ...want, wishlist: family === 'yes' });
	const problem = $derived(needsProblem(full));

	async function saveParts(parts: Record<PartName, boolean>) {
		error = '';
		busy = true;
		try {
			await applyParts(parts);
			await refreshParts();
			if (firstRun) go('folders');
			else await home();
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
		}
	}
	function submitParts(e: SubmitEvent) {
		e.preventDefault();
		if (problem) return void (error = problem);
		void saveParts(full);
	}

	// ---- 2. Library folders: one row per picked part that keeps files ----
	type Kind = 'movie' | 'show' | 'music';
	type Row = { kind: Kind; label: string; icon: IconName; path: string; saved: string; error: string };
	const kindRows: { part: PartName; kind: Kind; label: string; icon: IconName }[] = [
		{ part: 'movies', kind: 'movie', label: 'Movies', icon: 'film' },
		{ part: 'shows', kind: 'show', label: 'TV shows', icon: 'tv' },
		{ part: 'music', kind: 'music', label: 'Music', icon: 'music' }
	];
	let rows = $state<Row[]>([]);
	async function loadFolders() {
		await refreshParts();
		const on = (p: PartName) => partsNow.answer?.parts.find((x) => x.part === p)?.on !== false;
		let have: LibraryFolder[] = [];
		try {
			have = (await api.get<{ library_folders: LibraryFolder[] }>('/library-folders')).library_folders ?? [];
		} catch {
			// show empty rows
		}
		rows = kindRows
			.filter((k) => on(k.part))
			.map((k) => ({ kind: k.kind, label: k.label, icon: k.icon, path: '', saved: have.find((f) => f.kind === k.kind)?.path ?? '', error: '' }));
	}
	async function saveFolders(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		busy = true;
		let failed = false;
		for (const r of rows) {
			if (r.saved || !r.path.trim()) continue;
			r.error = '';
			try {
				// Named after its part ("Movies"), not the last bit of the path.
				const f = await api.post<LibraryFolder>('/library-folders', { path: r.path.trim(), kind: r.kind, label: r.label });
				r.saved = f.path;
			} catch (err) {
				r.error = message(err);
				failed = true;
			}
		}
		busy = false;
		if (failed) return;
		if (!rows.some((r) => r.saved)) return void (error = 'Add at least one folder.');
		go('downloader');
	}

	// ---- 3. Downloader ----
	const dlKinds: DownloaderForm['kind'][] = ['qbittorrent', 'sabnzbd', 'transmission', 'nzbget', 'deluge'];
	const dlPorts: Record<DownloaderForm['kind'], string> = { qbittorrent: '8080', sabnzbd: '8080', transmission: '9091', nzbget: '6789', deluge: '8112' };
	let dl = $state<DownloaderForm>(emptyForm());
	// Every connected downloader, each with its last test answer; the form shows
	// when there is none yet or the owner adds another.
	let dlList = $state<DownloaderView[]>([]);
	let dlTests = $state<Record<number, { ok: boolean; text: string }>>({});
	let dlAdding = $state(false);
	const dlForm = $derived(dlList.length === 0 || dlAdding);
	const dlUser = $derived(dl.kind === 'qbittorrent' || dl.kind === 'transmission' || dl.kind === 'nzbget');
	function pickDl(k: DownloaderForm['kind']) {
		const url = dl.url;
		dl = { ...emptyForm(k), url: url !== 'http://' ? url : 'http://' };
	}
	async function loadDownloader() {
		try {
			const r = await api.get<{ downloaders: DownloaderView[] }>('/downloaders');
			dlList = r.downloaders ?? [];
		} catch {
			dlList = [];
		}
		// Each one is tested again, so the row shows its version or that it is down.
		await Promise.all(dlList.map((d) => testDl(d.id)));
	}
	async function testDl(id: number) {
		try {
			const t = await api.post<{ ok: boolean; text: string }>(`/downloaders/${id}/test`);
			dlTests = { ...dlTests, [id]: { ok: t.ok, text: t.text } };
		} catch (err) {
			dlTests = { ...dlTests, [id]: { ok: false, text: message(err) } };
		}
	}
	async function saveDownloader(e: SubmitEvent) {
		e.preventDefault();
		if (!dlForm) return go('source');
		error = '';
		busy = true;
		try {
			const v = await api.post<DownloaderView>('/downloaders', formBody(dl));
			dlList = [...dlList, v];
			dlAdding = false;
			dl = emptyForm();
			await testDl(v.id);
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}

	// ---- 4. Source ----
	let sources = $state(0);
	async function loadSources() {
		try {
			sources = ((await api.get<{ sources: unknown[] }>('/sources')).sources ?? []).length;
		} catch {
			sources = 0;
		}
	}

	async function enter(s: Step) {
		if (s === 'folders') await loadFolders();
		if (s === 'downloader') await loadDownloader();
		if (s === 'source') await loadSources();
	}
	onMount(() => void enter(step));

	const titles: Record<Step, { h: string; p: string }> = {
		parts: { h: 'What do you use?', p: 'You can change this later.' },
		folders: { h: 'Library folders', p: 'Where should finished downloads go?' },
		downloader: { h: 'Downloader', p: 'Where HolyArrs sends downloads.' },
		source: { h: 'Source', p: 'Where HolyArrs finds releases.' },
		import: { h: 'Bring your setup', p: 'Coming from Sonarr, Radarr or Overseerr?' }
	};
</script>

<svelte:head><title>{titles[step].h} · HolyArrs</title></svelte:head>

<div class="welcome">
	<div class="brand"><img src={to(mark)} alt="" width="32" height="32" /><span>HolyArrs</span></div>
	{#if firstRun && prev && session.can('users')}
		<span class="corner"><IconButton icon="chevronLeft" variant="tinted" label="Back" onclick={() => go(prev)} disabled={busy} /></span>
	{/if}
	{#if firstRun}
		<ol class="dots" aria-label="Step {stepNo} of {stepCount}">
			{#each ORDER as s, i (s)}<li class:done={i + 1 < stepNo} class:now={i + 1 === stepNo}></li>{/each}
		</ol>
	{/if}
	<header class="head">
		<h1>{titles[step].h}</h1>
		<p>{titles[step].p}</p>
	</header>

	{#if !session.can('users') || (step !== 'parts' && !session.can('settings'))}
		<p class="notice warn">Only the owner, or an admin who can change settings, can finish setup.</p>
	{:else if step === 'parts'}
		<form class="card" onsubmit={submitParts} data-testid="welcome">
			<!-- Big tiles, each a real checkbox (it covers the tile, so a tap anywhere toggles it). -->
			<div class="tiles" role="group" aria-label="Parts">
				{#each choices as c (c.part)}
					<label class="tile" class:on={want[c.part]}>
						<input type="checkbox" bind:checked={want[c.part]} aria-label={c.label} />
						<span class="tick" aria-hidden="true"><Icon name="check" size={14} stroke={2.5} /></span>
						<Icon name={c.icon} size={28} />
						<span class="tile-label">{c.label}</span>
					</label>
				{/each}
			</div>

			<fieldset class="family">
				<legend>Will family or friends ask for titles?</legend>
				<div class="seg">
					<label class:on={family === 'no'}><input type="radio" name="family" value="no" bind:group={family} />No, just me</label>
					<label class:on={family === 'yes'}><input type="radio" name="family" value="yes" bind:group={family} />Yes</label>
				</div>
				<p class="hint">Yes adds a Requests page. They ask, you approve.</p>
			</fieldset>

			{#if error || problem}<p class="form-error" role="alert">{error || problem}</p>{/if}
			<Button variant="primary" size="lg" full type="submit" disabled={busy || !!problem} {busy}>Continue</Button>
			<button class="alt" type="button" disabled={busy} onclick={() => saveParts({ movies: true, shows: false, music: false, subtitles: false, wishlist: false })}>Just movies</button>
		</form>
	{:else if step === 'folders'}
		<form class="card" onsubmit={saveFolders} data-testid="welcome-folders">
			<ul class="rows">
				{#each rows as r (r.kind)}
					<li class="frow" class:done={!!r.saved}>
						<span class="ficon" aria-hidden="true"><Icon name={r.icon} size={22} /></span>
						<div class="fbody">
							<span class="fname">{r.label}</span>
							{#if r.saved}
								<span class="fsaved mono"><Icon name="check" size={14} stroke={2.5} />{r.saved}</span>
							{:else}
								<input class="input mono" bind:value={r.path} aria-label="{r.label} folder" placeholder={r.kind === 'show' ? '/shows' : r.kind === 'music' ? '/music' : '/movies'} autocomplete="off" spellcheck="false" />
								{#if r.error}<p class="form-error" role="alert">{r.error}</p>{/if}
							{/if}
						</div>
					</li>
				{/each}
			</ul>
			{#if rows.some((r) => !r.saved)}<p class="hint">The path as HolyArrs sees it. On Docker, the path inside the container. Leave one empty to add it later.</p>{/if}
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
			<Button variant="primary" size="lg" full type="submit" disabled={busy} {busy}>Continue</Button>
		</form>
	{:else if step === 'downloader'}
		<form class="card" onsubmit={saveDownloader} data-testid="welcome-downloader">
			{#if dlList.length}
				<ul class="conn" aria-label="Connected downloaders">
					{#each dlList as d (d.id)}
						{@const test = dlTests[d.id]}
						<li>
							<div class="conn-row">
							<div class="conn-text"><b>{d.name}</b><span class="addr mono">{d.url}</span></div>
							{#if test && !test.ok}
								<span class="tag bad">Not connected</span>
							{:else if test && testVersion(test.text)}
								<span class="tag ok mono"><Icon name="check" size={12} stroke={2.5} />{testVersion(test.text)}</span>
							{/if}
							</div>
							{#if test && !test.ok}<p class="form-error" role="alert">{test.text}</p>{/if}
						</li>
					{/each}
				</ul>
				{#if !dlForm}
					<button class="add-row" type="button" disabled={busy} onclick={() => { error = ''; dlAdding = true; }}><Icon name="plus" size={16} />Add another</button>
				{/if}
			{/if}
			{#if dlForm}
				<div class="tiles five" role="radiogroup" aria-label="Downloader app">
					{#each dlKinds as k (k)}
						<label class="tile small" class:on={dl.kind === k}>
							<input type="radio" name="dlkind" value={k} checked={dl.kind === k} onchange={() => pickDl(k)} aria-label={kindWords[k]} />
							<span class="tile-label">{kindWords[k]}</span>
						</label>
					{/each}
				</div>
				<div class="field">
					<label for="dl-url">Address</label>
					<input id="dl-url" class="input mono" type="url" bind:value={dl.url} placeholder="http://192.168.1.10:{dlPorts[dl.kind]}" autocomplete="off" spellcheck="false" />
				</div>
				<div class="two">
					{#if dlUser}
						<div class="field">
							<label for="dl-user">User name</label>
							<input id="dl-user" class="input" bind:value={dl.username} autocomplete="off" autocapitalize="none" spellcheck="false" />
						</div>
					{/if}
					<div class="field">
						<label for="dl-secret">{dl.kind === 'sabnzbd' ? 'API key' : 'Password'}</label>
						<input id="dl-secret" class="input" type="password" bind:value={dl.secret} autocomplete="new-password" />
					</div>
				</div>
			{/if}
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
			<Button variant="primary" size="lg" full type="submit" disabled={busy} {busy}>{dlForm ? 'Connect' : 'Continue'}</Button>
			{#if dlForm && dlList.length}
				<button class="alt" type="button" disabled={busy} onclick={() => { error = ''; dlAdding = false; dl = emptyForm(); }}>Cancel</button>
			{:else}
				<button class="alt" type="button" disabled={busy} onclick={() => go('source')}>Skip for now</button>
			{/if}
		</form>
	{:else if step === 'source'}
		<div class="card source" data-testid="welcome-source">
			<ConnectBridge onchange={loadSources} />
			{#if sources > 0}
				<Button variant="primary" size="lg" full type="button" disabled={busy} onclick={() => go('import')}>Continue</Button>
			{:else}
				<button class="alt" type="button" disabled={busy} onclick={() => go('import')}>Skip for now</button>
			{/if}
		</div>
	{:else}
		<div class="card" data-testid="welcome-import">
			<div class="tiles two-up">
				<a class="tile big" href={to('/settings/move')}>
					<Icon name="download" size={26} />
					<span class="tile-label">Bring my setup</span>
					<span class="tile-sub">Library, sources, downloaders and people.</span>
				</a>
				<button class="tile big" type="button" disabled={busy} onclick={home}>
					<Icon name="home" size={26} />
					<span class="tile-label">Start fresh</span>
					<span class="tile-sub">Open Home.</span>
				</button>
			</div>
		</div>
	{/if}
</div>

<style>
	.welcome {
		position: relative;
		width: min(560px, calc(100% - 32px));
		min-height: 100dvh;
		margin: 0 auto;
		padding: clamp(28px, 7vh, 72px) 0 48px;
	}
	.brand { display: flex; align-items: center; justify-content: center; gap: 10px; font: 600 17px/1 var(--font-ui); color: var(--text); }
	/* The round back button sits top-left, level with the logo. */
	.corner { position: absolute; top: calc(clamp(28px, 7vh, 72px) - 2px); left: 0; }
	.dots { display: flex; justify-content: center; gap: 6px; margin: 22px 0 0; }
	.dots li { width: 6px; height: 6px; border-radius: 3px; background: var(--line-strong); transition: width var(--dur) var(--ease-out), background var(--dur) var(--ease-out); }
	.dots li.done { background: var(--text-muted); }
	.dots li.now { width: 22px; background: var(--accent-fill); }
	.head { margin: 18px 0 28px; text-align: center; }
	.head h1 { font: var(--t-large-title); letter-spacing: var(--t-large-title-track); text-wrap: balance; }
	.head p { margin: 8px auto 0; max-width: 44ch; color: var(--text-muted); font: var(--t-body); text-wrap: pretty; }
	.welcome > :global(.card) {
		position: relative;
		border: 0;
		border-radius: 24px;
		background: var(--surface-1);
		box-shadow: var(--shadow-card), 0 0 0 1px var(--line);
		padding: clamp(18px, 4vw, 28px);
	}
	fieldset { border: 0; padding: 0; margin: 0; min-width: 0; }
	legend { margin-bottom: 12px; font: var(--t-headline); color: var(--text); }

	/* Tiles: an icon and a word; picked tiles take the accent, with a tick. */
	.tiles { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; }
	.tiles.five { grid-template-columns: repeat(3, minmax(0, 1fr)); margin-bottom: 18px; }
	.tiles.two-up { grid-template-columns: repeat(2, minmax(0, 1fr)); }
	.tile {
		position: relative;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 10px;
		min-height: 108px;
		padding: 14px 8px;
		border-radius: 16px;
		border: 1px solid var(--line);
		background: var(--surface-2);
		color: var(--text-muted);
		font: inherit;
		text-decoration: none;
		cursor: pointer;
		transition: border-color var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out);
	}
	.tile.small { min-height: 56px; padding: 10px 6px; }
	.tile.big { min-height: 150px; padding: 18px 14px; }
	.tile.big:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	/* The real control covers the whole tile, invisible, so taps and clicks land on it. */
	.tile input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; }
	.tile-label { font: 600 14px/1.2 var(--font-ui); color: var(--text); text-align: center; }
	.tile-sub { font: var(--t-footnote); color: var(--text-muted); text-align: center; }
	@media (hover: hover) { .tile:hover { border-color: var(--line-strong); } }
	.tile:active { transform: scale(0.98); }
	.tile.on { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 7%, var(--surface-1)); color: var(--accent-text); }
	.tile:has(input:focus-visible) { box-shadow: var(--focus-ring); }
	.tick {
		position: absolute;
		top: 9px;
		right: 9px;
		width: 20px;
		height: 20px;
		border-radius: 50%;
		display: grid;
		place-items: center;
		border: 1.5px solid var(--line-strong);
		color: transparent;
		transition: background-color var(--dur-fast) var(--ease-out), border-color var(--dur-fast) var(--ease-out);
	}
	.tile.on .tick { background: var(--accent); border-color: var(--accent); color: #fff; }

	/* Family: a two-way switch, the picked side lifted. */
	.family { margin-top: 26px; }
	.seg { display: grid; grid-template-columns: 1fr 1fr; gap: 4px; padding: 4px; border-radius: 12px; background: var(--surface-2); border: 1px solid var(--line); }
	.seg label {
		position: relative;
		display: grid;
		place-items: center;
		min-height: 40px;
		border-radius: 9px;
		color: var(--text-muted);
		font: 500 15px/1 var(--font-ui);
		cursor: pointer;
		transition: background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out);
	}
	.seg input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; }
	.seg label.on { background: var(--surface-1); color: var(--text); box-shadow: 0 1px 3px rgb(0 0 0 / 0.14), 0 0 0 0.5px var(--line); }
	.seg label:has(input:focus-visible) { box-shadow: var(--focus-ring); }
	.hint { display: block; margin-top: 10px; color: var(--text-muted); font: var(--t-footnote); }

	.welcome :global(.card) > :global(.ui-btn) { margin-top: 24px; }
	.alt {
		display: block;
		margin: 10px auto 0;
		min-height: 36px;
		padding: 0 12px;
		border: 0;
		border-radius: var(--r-sm);
		background: none;
		color: var(--text-muted);
		font: 500 14px/1 var(--font-ui);
		cursor: pointer;
	}
	@media (hover: hover) { .alt:hover { color: var(--text); background: color-mix(in srgb, var(--text) 6%, transparent); } }
	.alt:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.alt:disabled { opacity: 0.5; cursor: default; }

	/* Folders: one row per part, its icon, its name, its path. */
	.rows { display: flex; flex-direction: column; gap: 10px; }
	.frow { display: flex; align-items: flex-start; gap: 14px; padding: 12px; border-radius: 16px; border: 1px solid var(--line); background: var(--surface-2); }
	.frow.done { background: color-mix(in srgb, var(--ok, #30d158) 7%, var(--surface-1)); }
	.ficon { flex: none; width: 40px; height: 40px; border-radius: 12px; display: grid; place-items: center; background: var(--surface-1); color: var(--text-muted); box-shadow: 0 0 0 1px var(--line); }
	.fbody { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 6px; }
	.fname { font: 600 15px/1.2 var(--font-ui); color: var(--text); padding-top: 2px; }
	.fsaved { display: flex; align-items: center; gap: 6px; color: var(--text-muted); font-size: 13px; overflow-wrap: anywhere; }
	.fsaved :global(svg) { color: var(--ok, #30d158); flex: none; }
	.frow .form-error { margin-top: 0; }

	/* A connected downloader or source: a plain row, its version as a small tag. */
	.conn { list-style: none; margin: 0 0 12px; padding: 0; display: grid; gap: 10px; }
	.conn-row { display: flex; align-items: center; gap: 12px; padding: 14px 16px; border-radius: 16px; background: var(--surface-2); border: 1px solid var(--line); }
	.conn-text { flex: 1; min-width: 0; }
	.conn-text b { display: block; font: 600 15px/1.3 var(--font-ui); color: var(--text); }
	.conn-text .addr { display: block; margin-top: 2px; font-size: 12.5px; color: var(--text-muted); overflow-wrap: anywhere; }
	.tag { flex: none; display: inline-flex; align-items: center; gap: 4px; padding: 4px 8px; border-radius: 999px; border: 1px solid var(--line); color: var(--text-muted); font: 500 12px/1.2 var(--font-ui); }
	.tag.mono { font-family: var(--font-mono); }
	.tag.ok { color: var(--ok-text, #30d158); border-color: color-mix(in srgb, var(--ok-text, #30d158) 40%, transparent); }
	.tag.bad { color: var(--danger-text, #ff6961); border-color: color-mix(in srgb, var(--danger-text, #ff6961) 40%, transparent); }
	.add-row { width: 100%; margin-top: 0; display: flex; align-items: center; gap: 10px; min-height: 48px; padding: 0 16px; border: 1px dashed color-mix(in srgb, var(--text) 18%, transparent); border-radius: 16px; background: none; color: var(--text-muted); font: 500 15px/1 var(--font-ui); cursor: pointer; transition: color 0.15s, border-color 0.15s; }
	.add-row:hover:not(:disabled) { color: var(--text); border-color: color-mix(in srgb, var(--text) 32%, transparent); }
	.add-row:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.two { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 12px; margin-top: 12px; align-items: start; }
	.two > :global(.field) { margin: 0; }
	.source :global(.settings-card), .source :global(section) { box-shadow: none; }

	@media (max-width: 520px) {
		.tiles { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.tiles.five { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.tiles.five .tile:last-child:nth-child(odd) { grid-column: span 2; }
		.tiles.two-up { grid-template-columns: 1fr; }
		.tile { min-height: 96px; }
		.tile.small { min-height: 52px; }
		.tile.big { min-height: 110px; }
	}
	@media (pointer: coarse) {
		.alt { min-height: 44px; }
		.seg label { min-height: 44px; }
	}
</style>
