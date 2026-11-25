<script lang="ts">
	import { ltr, t } from '#lib/core/i18n/i18n.svelte.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import StepFoot from './StepFoot.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import FolderPopup from '#lib/ui/FolderPopup.svelte';
	import FolderField from '#lib/ui/FolderField.svelte';
	import { api, message } from '#lib/core/api.ts';
	import { appName, cleanPath, folderProblem, LEAVE, OTHER, unseenFolder, type Plan } from './move.ts';

	// Setup's Choose: every old folder found here is matched on its own, and the import starts straight away. Only a folder that cannot be found asks one plain question.
	let {
		plan,
		planning = false,
		folders,
		here = [],
		picks,
		others,
		busy = false,
		note = '',
		onfolder,
		onother,
		oncommit,
		onstart,
		onback,
		error = '',
		review = false,
		onrefresh
	}: {
		plan: Plan | null;
		planning?: boolean;
		folders: { path: string }[];
		/** Folders on this server a moved folder could go to (Library and mounted folders, one level inside too). */
		here?: string[];
		picks: Record<string, string>;
		others: Record<string, string>;
		busy?: boolean;
		note?: string;
		onfolder: (from: string, pick: string) => void;
		onother: (from: string, to: string) => void;
		oncommit: () => void;
		onstart: () => void;
		onback?: () => void;
		/** A problem planning or starting; shown inside the card, above the foot. */
		error?: string;
		/** Setup's Folders step: every old folder shows (found ones ticked), nothing starts on its own, Continue runs the import. */
		review?: boolean;
		/** Looks again for the folders HolyArrs can see (after a Docker volume was added). */
		onrefresh?: () => void;
	} = $props();

	let tried = new Set<string>();
	let asked = $state<string[]>([]);
	let started = $state(false);
	/** The old folder whose folder browser is open ('' when none). */
	let browsing = $state('');
	/** The row last changed: only it shows the check running. */
	let changed = $state('');
	/** The title folder names under this old folder, for the browser's counts. */
	function titleNames(p: string): string[] {
		const f = items.find((it) => it.folder?.old_path === p)?.folder;
		const lib = f?.path ?? p;
		return (plan?.groups.find((g) => g.name === 'titles')?.items ?? []).filter((it) => it.title?.library === lib && it.title.folder).slice(0, 30).map((it) => it.title!.folder);
	}

	const items = $derived(plan?.groups.find((g) => g.name === 'folders')?.items ?? []);
	const oldRows = $derived((plan?.old_paths ?? []).filter((p) => !folders.some((f) => f.path === p)));
	const titles = $derived(plan?.groups.find((g) => g.name === 'titles')?.items.length ?? 0);
	/** The titles by app and kind ("212 movies from Radarr"), the breakdown at the top. */
	const parts = $derived.by(() => {
		const m = new Map<string, { app: string; kind: string; n: number }>();
		for (const it of plan?.groups.find((g) => g.name === 'titles')?.items ?? []) {
			const app = appName(it.from.split(' ')[0] ?? ''), kind = it.title?.kind ?? '';
			const k = app + '|' + kind;
			m.set(k, { app, kind, n: (m.get(k)?.n ?? 0) + 1 });
		}
		return [...m.values()].sort((a, b) => b.n - a.n);
	});
	const kindWord = (k: string, n: number) => t(k === 'movie' || k === 'movies' ? (n === 1 ? 'migrate.parts.movie' : 'migrate.parts.movies') : k === 'show' ? (n === 1 ? 'migrate.parts.show' : 'migrate.parts.shows') : k === 'artist' || k === 'music' ? (n === 1 ? 'migrate.parts.artist' : 'migrate.parts.artists') : (n === 1 ? 'migrate.title2' : 'migrate.titles3'));
	/** How many of the folder's titles are really in the folder it now points at (none checked: nothing to say). */
	function seen(p: string): { found: number; checked: number; missing: boolean; readOnly: boolean } | null {
		const f = items.find((it) => it.folder?.old_path === p)?.folder;
		return f?.checked || f?.missing ? { found: f.found ?? 0, checked: f.checked ?? 0, missing: !!f.missing, readOnly: !!f.read_only } : null;
	}
	const empty = (p: string) => picks[p] !== LEAVE && !!seen(p) && (seen(p)!.missing || seen(p)!.readOnly || seen(p)!.found === 0);
	const answered = (p: string) => picks[p] === LEAVE || (!!picks[p] && folderProblem(p, picks[p], others[p]) === '' && !empty(p));
	const ready = $derived(!!plan && !planning && oldRows.every((p) => answered(p)));

	/** The same folder here, or the one folder here with the same name (/movies finds /data/Movies). */
	function guess(p: string): string {
		if (!unseenFolder(plan, p)) return p;
		// The folder here that holds this folder's titles, found by the server.
		const sug = items.find((it) => it.folder?.old_path === p)?.folder?.suggest;
		if (sug) return sug;
		const name = (cleanPath(p).split('/').pop() ?? '').toLowerCase();
		const hits = here.filter((h) => (cleanPath(h).split('/').pop() ?? '').toLowerCase() === name);
		return name && hits.length === 1 ? hits[0] : '';
	}
	$effect(() => {
		if (!plan || planning) return;
		for (const p of oldRows) {
			if (picks[p] !== undefined || tried.has(p)) continue;
			tried.add(p);
			const g = guess(p);
			if (g) onfolder(p, g);
			else if (!asked.includes(p)) asked = [...asked, p];
		}
	});
	// A matched folder that holds none of its titles is asked again, never brought over empty.
	$effect(() => {
		if (!plan || planning) return;
		for (const p of oldRows) if (empty(p) && !asked.includes(p)) asked = [...asked, p];
	});
	$effect(() => {
		if (!review && ready && asked.length === 0 && !started) {
			started = true;
			onstart();
		}
	});

	function question(p: string): string {
		const k = items.find((it) => it.folder?.old_path === p)?.folder?.kind;
		return k === 'movie' ? t('migrate.whereMovies') : k === 'show' ? t('migrate.whereShows') : k === 'music' ? t('migrate.whereMusic') : t('migrate.whereFolder');
	}
	const appOf = (p: string) => appName(items.find((x) => x.folder?.old_path === p)?.from.split(' ')[0] ?? '');
	/** The folder the last check looked in: a result shows only once it is for the folder on screen. */
	const checkedFor = (p: string) => cleanPath(items.find((it) => it.folder?.old_path === p)?.folder?.path ?? '');
	/** The old folders by part, in Movies, TV shows, Music order, with each part's title count. */
	const groups = $derived.by(() => {
		const order = [{ kind: 'movie', label: t('setup.movies'), icon: 'film' as const }, { kind: 'show', label: t('setup.tvShows'), icon: 'tv' as const }, { kind: 'music', label: t('setup.music'), icon: 'music' as const }];
		const ps = plan?.old_paths ?? [];
		return order.map((o) => ({ ...o, paths: ps.filter((p) => (kindOf(p) || 'movie') === o.kind) })).filter((g) => g.paths.length).map((g) => ({ ...g, n: g.paths.reduce((a, p) => a + countOf(p), 0) }));
	});
	// A part no old app brought (Music with no Lidarr): + opens its own column here, and each folder saved there becomes a Library folder now.
	const allKinds = $derived([{ kind: 'movie', label: t('setup.movies'), icon: 'film' as const }, { kind: 'show', label: t('setup.tvShows'), icon: 'tv' as const }, { kind: 'music', label: t('setup.music'), icon: 'music' as const }]);
	let extra = $state<Record<string, { path: string; saved: { id: number; path: string }[]; error: string; busy: boolean }>>({});
	const extraOn = $derived(allKinds.filter((k) => extra[k.kind] && !groups.some((g) => g.kind === k.kind)));
	const extraOff = $derived(allKinds.filter((k) => !extra[k.kind] && !groups.some((g) => g.kind === k.kind)));
	async function addExtra(kind: string, label: string) {
		const e = extra[kind];
		const path = e.path.trim();
		if (!path || e.busy) return;
		e.busy = true;
		e.error = '';
		try {
			const f = await api.post<{ id: number; path: string }>('/library-folders', { path, kind, label });
			e.saved = [...e.saved, { id: f.id, path: f.path }];
			e.path = '';
		} catch (err) {
			e.error = message(err);
		} finally {
			e.busy = false;
		}
	}
	async function removeExtra(kind: string, id: number) {
		const e = extra[kind];
		try {
			await api.del(`/library-folders/${id}`);
			e.saved = e.saved.filter((x) => x.id !== id);
		} catch (err) {
			e.error = message(err);
		}
	}
	const kindOf = (p: string) => items.find((it) => it.folder?.old_path === p)?.folder?.kind ?? '';
	/** How many titles the old app kept in this folder. */
	function countOf(p: string): number {
		const lib = items.find((it) => it.folder?.old_path === p)?.folder?.path ?? p;
		return (plan?.groups.find((g) => g.name === 'titles')?.items ?? []).filter((it) => it.title?.library === lib).length;
	}
	function keptBy(p: string): string {
		const it = items.find((x) => x.folder?.old_path === p);
		return t('migrate.keptThemIn', { app: appName(it?.from.split(' ')[0] ?? ''), path: ltr(p) });
	}
</script>

<div class="setup-choose" aria-busy={planning || busy}>
	<SettingsCard id="move-step" title={t('migrate.whatToMove')}>
		{#if !plan}
			<!-- Placeholders in the real layout while the plan loads, so nothing jumps when it arrives. -->
			{#if review}<ul class="frows" aria-hidden="true">{#each [0, 1, 2] as i (i)}<li class="frow"><span class="fwhat"><span class="ghost line"></span><span class="ghost line short"></span></span><span class="ghost field"></span><span class="ghost skip"></span></li>{/each}</ul>
			{:else}<div class="parts" aria-hidden="true">{#each [0, 1, 2] as i (i)}<div class="part ghost"></div>{/each}</div>
			{#each [0, 1] as i (i)}<div class="ask" aria-hidden="true"><span class="ghost line"></span><span class="ghost field"></span></div>{/each}{/if}
			<p class="sr-only" role="status">{t('migrate.updating')}</p>
		{:else}
			{#if review}
				<!-- Like Folders without an import: a count tile per part, then a column per part with each old app folder: the folder found here (its check as a badge), change it in the folder browser, or skip it. -->
				<div class="fp" style="--cols: repeat({groups.length + extraOn.length}, minmax(0, 1fr)){extraOff.length ? ` repeat(${extraOff.length}, minmax(140px, 0.45fr))` : ''}">
					<div class="tiles" data-testid="move-summary">
						{#each groups as g (g.kind)}<div class="tile"><span class="ic" aria-hidden="true"><Icon name={g.icon} size={18} /></span><b class="big">{g.n}</b><span class="sub2">{kindWord(g.kind, g.n)} · {t(g.paths.length === 1 ? 'welcome.folders.oneFolder' : 'welcome.folders.nFolders', { n: g.paths.length })}</span></div>{/each}
						{#each extraOn as k (k.kind)}<div class="tile"><span class="ic" aria-hidden="true"><Icon name={k.icon} size={18} /></span><b class="big">{extra[k.kind].saved.length}</b><span class="sub2">{t(extra[k.kind].saved.length === 1 ? 'welcome.folders.oneFolder' : 'welcome.folders.nFolders', { n: extra[k.kind].saved.length })}</span></div>{/each}
						{#each extraOff as k (k.kind)}<button type="button" class="tile off" onclick={() => (extra = { ...extra, [k.kind]: { path: '', saved: [], error: '', busy: false } })} data-testid="folder-part-{k.kind}"><span class="ic" aria-hidden="true"><Icon name="plus" size={18} /></span><b class="small">{k.label}</b><span class="sub2">{t('welcome.folders.addPart')}</span></button>{/each}
					</div>
					<div class="cols">
						{#each groups as g (g.kind)}
							<section class="col" aria-label={g.label}>
								<h3 class="h"><Icon name={g.icon} size={18} />{g.label}</h3>
								{#each g.paths as p (p)}
									{@const pick = picks[p] ?? (oldRows.includes(p) ? '' : p)}
									{@const err = pick === LEAVE ? '' : folderProblem(p, pick, others[p])}
									{@const c = pick && pick !== LEAVE && !(planning && changed === p) && checkedFor(p) === cleanPath(pick) ? seen(p) : null}
									<div class="saved" class:skipped={pick === LEAVE} data-testid={asked.includes(p) ? 'folder-question' : 'folder-found'}>
										<button type="button" class="pickhit" aria-haspopup="dialog" onclick={() => (browsing = p)} data-testid="folder-pick" aria-label={question(p)}>
											<span class="path" class:none={!pick || pick === LEAVE || pick === OTHER} dir="ltr">{pick === LEAVE ? t('migrate.skipTheseTitles') : pick && pick !== OTHER ? ltr(pick) : t('home.chooseAFolder')}</span>
											{#if planning && changed === p}<span class="spin" aria-hidden="true"></span>
											{:else if c?.readOnly}<span class="hit bad" data-testid="folder-readonly-badge">{t('migrate.readOnly')}</span>
											{:else if c && !c.missing && c.found > 0}<span class="hit" class:some={c.found * 5 < c.checked * 4} data-testid="folder-seen">{c.found === c.checked ? t('migrate.allHere') : c.found * 5 >= c.checked * 4 ? t('migrate.nFound', { found: c.found }) : t('migrate.someHere', { found: c.found, checked: c.checked })}</span>
											{:else if c}<span class="hit bad">{c.missing ? t('migrate.browse.cantSee') : '0/' + c.checked}</span>{/if}
											<Icon name="folder" size={16} />
										</button>
										<button type="button" class="q" aria-pressed={pick === LEAVE} onclick={() => { changed = p; onfolder(p, pick === LEAVE ? guess(p) : LEAVE); }} aria-label={t('migrate.skip')} title={t('migrate.skip')}><Icon name={pick === LEAVE ? 'refresh' : 'x'} size={16} /></button>
									</div>
									<p class="from" dir="auto">{appOf(p)} · <span dir="ltr">{ltr(p)}</span></p>
									{#if err}<p class="form-error ferr" role="alert">{err}</p>
									{:else if c?.readOnly}<p class="form-error ferr" role="alert" data-testid="folder-readonly">{t('migrate.folderReadOnly')}</p>
									{:else if c?.missing}<p class="form-error ferr" role="alert" data-testid="folder-missing">{t('migrate.folderMissing')}</p>
									{:else if c && c.found === 0}<p class="form-error ferr" role="alert" data-testid="folder-none">{t('migrate.folderNone')}</p>{/if}
								{/each}
							</section>
						{/each}
						{#each extraOn as k (k.kind)}
							<section class="col" aria-label={k.label}>
								<h3 class="h"><Icon name={k.icon} size={18} />{k.label}</h3>
								{#each extra[k.kind].saved as f (f.id)}<div class="saved"><span class="path pad" dir="ltr">{f.path}</span><button type="button" class="q" onclick={() => removeExtra(k.kind, f.id)} aria-label={t('welcome.folders.remove', { path: f.path })}><Icon name="x" size={16} /></button></div>{/each}
								<FolderField id="folder-extra-{k.kind}" label={k.label} kind={k.kind} hint={'/data/' + k.kind} bind:value={extra[k.kind].path} error={extra[k.kind].error} disabled={extra[k.kind].busy} onsave={() => addExtra(k.kind, k.label)} />
							</section>
						{/each}
						{#each extraOff as k (k.kind)}<span class="gap" aria-hidden="true"></span>{/each}
					</div>
				</div>
				{#if browsing}<FolderPopup names={titleNames(browsing)} title={question(browsing)} hint={keptBy(browsing)} current={picks[browsing] && picks[browsing] !== LEAVE && picks[browsing] !== OTHER ? picks[browsing] : ''} onclose={() => (browsing = '')} onpick={(to) => { const p = browsing; browsing = ''; changed = p; onfolder(p, to); }} />{/if}
			{:else}
			{#if titles > 0}
				<div class="parts" data-testid="move-summary">
					{#each parts as c (c.app + c.kind)}<div class="part"><b class="num">{c.n}</b><span class="what">{kindWord(c.kind, c.n)}</span><span class="from">{t('migrate.parts.from', { app: c.app })}</span></div>{/each}
				</div>
			{/if}
			{#if review}
				<!-- Every old folder: one already a Library folder here, or matched on its own, shows ticked. -->
				{#each (plan.old_paths ?? []).filter((p) => !asked.includes(p) && (!oldRows.includes(p) || answered(p))) as p (p)}
					<p class="found" data-testid="folder-found"><span class="tick" aria-hidden="true"><Icon name="check" size={14} /></span><span dir="auto">{keptBy(p)} → <span dir="ltr">{picks[p] === LEAVE ? t('migrate.skipTheseTitles') : ltr(picks[p] || p)}</span></span></p>
				{/each}
			{/if}
			{/if}
			{#if !review}{#each asked as p, n (p)}
				{@const pick = picks[p] ?? ''}
				{@const err = pick === LEAVE ? '' : folderProblem(p, pick, others[p])}
				<section class="ask" data-testid="folder-question">
					<h3 class="head">{question(p)}</h3>
					<p class="sub" dir="auto">{keptBy(p)}</p>
					<div class="row">
						<label class="sr-only" for="setup-folder-{n}">{question(p)}</label>
						{#if review}
							<button type="button" id="setup-folder-{n}" class="input pickbtn" class:none={!pick || pick === LEAVE} aria-expanded={browsing === p} aria-describedby={err ? `setup-folder-${n}-err` : undefined} onclick={() => (browsing = browsing === p ? '' : p)} data-testid="folder-pick"><span dir="ltr">{pick && pick !== LEAVE && pick !== OTHER ? ltr(pick) : t('home.chooseAFolder')}</span><Icon name="folder" size={16} /></button>
						{:else}
													<select id="setup-folder-{n}" class="input" value={pick === LEAVE ? '' : pick} aria-invalid={err ? 'true' : undefined} aria-describedby={err ? `setup-folder-${n}-err` : undefined} onchange={(e) => onfolder(p, e.currentTarget.value)}>
							<option value="" disabled>{t('home.chooseAFolder')}</option>
							{#each here as h (h)}<option value={h}>{ltr(h)}</option>{/each}
							<option value={OTHER}>{t('migrate.otherPath')}</option>
						</select>
						{/if}
						<button type="button" class="quiet" class:on={pick === LEAVE} aria-pressed={pick === LEAVE} onclick={() => onfolder(p, LEAVE)}>{t('migrate.skipTheseTitles')}</button>
					</div>
					
					{#if pick === OTHER}
						<label class="sr-only" for="setup-folder-{n}-other">{t('migrate.pathHereFor', { path: p })}</label>
						<input id="setup-folder-{n}-other" dir="ltr" class="input mono" value={others[p] ?? ''} aria-invalid={err ? 'true' : undefined} aria-describedby={err ? `setup-folder-${n}-err` : undefined} placeholder="/data/movies" oninput={(e) => onother(p, e.currentTarget.value)} onblur={oncommit} onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), oncommit())} />
					{/if}
					{#if err}<p class="form-error" id="setup-folder-{n}-err" role="alert">{err}</p>
					{:else if pick && pick !== LEAVE && !planning && seen(p)}
						{@const c = seen(p)!}
						{#if c.missing}<p class="form-error" role="alert" data-testid="folder-missing">{t('migrate.folderMissing')}</p>
						{:else if c.found === 0}<p class="form-error" role="alert" data-testid="folder-none">{t('migrate.folderNone')}</p>
						{:else}<p class="seen" class:some={c.found * 5 < c.checked * 4} data-testid="folder-seen"><Icon name={c.found * 5 < c.checked * 4 ? 'alert' : 'check'} size={14} /><span>{t(c.found * 5 < c.checked * 4 ? 'migrate.folderSome' : 'migrate.folderFound', { found: c.found, checked: c.checked })}</span></p>{/if}
					{/if}
				</section>
			{/each}{/if}
			{/if}
		{#if review && oldRows.some((p) => !answered(p))}<p class="sub docker" data-testid="folder-docker-hint"><span>{t('migrate.notListedAddVolume')}</span></p>{/if}
		<p class="sub" role="status">{!plan ? '' : (review ? busy || (planning && !changed && !plan) : asked.length === 0 || busy) && !error ? note || t('settings.move.checking') : ''}</p>
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
	</SettingsCard>
	<!-- With no question asked the import starts on its own; if that start fails, Import and Back come back so the step is never a dead end. -->
	{#if review || asked.length > 0 || (started && !busy) || error}
		<StepFoot setup label={review ? t('welcome.continue') : t('settings.move.move')} onclick={onstart} disabled={!ready} {busy} />
	{/if}
</div>

<style>
	.parts { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 8px; margin: 0 0 2px; }
	.part { display: flex; align-items: baseline; flex-wrap: wrap; gap: 2px 6px; padding: 8px 12px; border-radius: 10px; border: 1px solid var(--g-line, rgb(255 255 255 / 0.12)); background: var(--g-bg, rgb(255 255 255 / 0.05)); min-width: 0; }
	.part .num { font-size: 20px; }
	.ghost { display: block; border-radius: 10px; background: color-mix(in srgb, var(--text) 7%, transparent); }
	.part.ghost { height: 42px; }
	.ghost.line { width: 240px; max-width: 60%; height: 14px; border-radius: 6px; }
	.ghost.field { height: 44px; }
	.ghost.line.short { width: 140px; margin-top: 6px; height: 10px; }
	.ghost.skip { width: 48px; height: 20px; }
	@media (prefers-reduced-motion: no-preference) { .ghost { animation: ghost 1.2s ease-in-out infinite alternate; } }
	@keyframes ghost { to { opacity: 0.45; } }
	.what { font: 600 14px/1.3 var(--font-ui); color: var(--text); }
	.from { font: var(--t-footnote); color: var(--text-muted); }
	.num { font: 700 26px/1.1 var(--font-ui); color: var(--text); }
	.sub { color: var(--text-muted); font: var(--t-footnote); margin: 0; }
	.sub:empty { display: none; }
	.ask { margin-top: 10px; display: flex; flex-direction: column; gap: 4px; }
	.ask :global(.form-error) { margin: 0; }
	.ask > .input { min-height: 44px; height: 44px; }
	.head { margin: 0; font: var(--t-subhead); font-weight: 600; color: var(--text); }
	.row { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 12px; }
	.row select, .row .pickbtn { flex: 1 1 220px; min-width: 0; min-height: 44px; height: 44px; }
	.pickbtn { display: flex; align-items: center; justify-content: space-between; gap: 8px; text-align: start; cursor: pointer; }
	.pickbtn span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.pickbtn.none span { color: var(--text-muted); }
	.quiet { background: none; border: 0; min-height: 44px; padding: 0 12px; border-radius: var(--r-sm); color: var(--text-muted); font: 500 15px/1.2 var(--font-ui); cursor: pointer; }
	.quiet:hover { color: var(--text); background: color-mix(in srgb, var(--text) 6%, transparent); }
	.quiet:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.quiet.on { color: var(--text); font-weight: 600; }
	.found { margin: 10px 0 0; display: flex; align-items: flex-start; gap: 8px; color: var(--text); font: var(--t-footnote); overflow-wrap: anywhere; }
	.found .tick { flex: none; display: inline-flex; color: var(--ok-text); margin-top: 2px; }
	.docker { margin-top: 12px; display: flex; flex-wrap: wrap; align-items: center; gap: 4px 8px; }
	.seen { margin: 0; display: flex; align-items: center; gap: 6px; font: var(--t-footnote); color: var(--ok-text); }
	.seen.some { color: var(--warn-text, #f5a524); }
	.frows { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
	.frow { display: grid; grid-template-columns: minmax(140px, 200px) minmax(0, 1fr) auto; align-items: center; gap: 6px 12px; padding-block: 8px; padding-inline: 16px 8px; border-radius: 14px; border: 1px solid var(--g-line, rgb(255 255 255 / 0.12)); background: var(--g-bg, rgb(255 255 255 / 0.05)); }
	.pickhit .hit { flex: none; margin-inline-start: auto; font: 600 12px/1 var(--font-ui); padding: 3px 8px; border-radius: 999px; color: var(--ok-text); background: color-mix(in srgb, var(--ok-text) 16%, transparent); }
	.pickhit .hit.some { color: var(--warn-text, #f5a524); background: color-mix(in srgb, var(--warn-text, #f5a524) 16%, transparent); }
	.pickhit .hit.bad { color: var(--bad-text, #ff6b6b); background: color-mix(in srgb, var(--bad-text, #ff6b6b) 16%, transparent); }
	.ferr { grid-column: 2 / -1; margin: 0; }
	.spin { flex: none; margin-inline-start: auto; width: 14px; height: 14px; border-radius: 50%; border: 2px solid color-mix(in srgb, var(--text) 20%, transparent); border-top-color: var(--ok-text); animation: sc-spin 0.8s linear infinite; }
	@media (prefers-reduced-motion: reduce) { .spin { animation-duration: 2.4s; } }
	@keyframes sc-spin { to { transform: rotate(360deg); } }
	.fp { display: flex; flex-direction: column; gap: 14px; }
	.fp .tiles, .fp .cols { display: grid; grid-template-columns: var(--cols); gap: 12px; }
	@media (max-width: 700px) { .fp .tiles, .fp .cols { grid-template-columns: 1fr; } }
	.fp .tile { display: grid; grid-template-columns: auto 1fr; column-gap: 10px; align-items: center; padding: 14px 16px; border-radius: 16px; border: 1px solid var(--g-line, var(--line)); background: var(--g-bg, var(--surface-1)); }
	.fp .tile .ic { grid-row: 1 / 3; color: var(--text-muted); }
	.fp .big { font: 700 28px/1 var(--font-ui); color: var(--text); font-variant-numeric: tabular-nums; }
	.fp .sub2 { font: var(--t-footnote); color: var(--text-muted); }
	.fp .col { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
	.fp .h { margin: 0 0 2px; display: flex; align-items: center; gap: 8px; font: 600 16px/1.2 var(--font-ui); color: var(--text); }
	.fp .saved { display: flex; align-items: center; gap: 4px; min-height: 44px; padding: 0 4px; border-radius: 12px; border: 1px solid var(--g-line, var(--line)); background: var(--g-bg, var(--surface-1)); min-width: 0; }
	.fp .saved.skipped { opacity: 0.55; }
	.fp .pickhit { flex: 1; min-width: 0; display: flex; align-items: center; gap: 8px; height: 40px; padding: 0 8px; border: 0; border-radius: 9px; background: none; color: var(--text); cursor: pointer; text-align: start; }
	.fp .pickhit:hover, .fp .q:hover { background: color-mix(in srgb, var(--text) 8%, transparent); }
	.fp .path { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 400 14px/1.3 var(--font-ui); }
	.fp .path.none { color: var(--text-muted); }
	.fp .q { flex: none; width: 32px; height: 32px; display: grid; place-items: center; border: 0; border-radius: 8px; background: none; color: var(--text-muted); cursor: pointer; }
	.fp .pickhit:focus-visible, .fp .q:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.fp .from { margin: 0 0 4px 4px; font: var(--t-footnote); color: var(--text-muted); overflow-wrap: anywhere; }
	.fp .ferr { margin: 0; }
	.fp .tile.off { border-style: dashed; background: none; color: var(--text); cursor: pointer; text-align: start; font: inherit; }
	.fp .tile.off:hover { background: color-mix(in srgb, var(--text) 6%, transparent); }
	.fp .tile.off:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.fp .small { font: 600 16px/1.2 var(--font-ui); color: var(--text); }
	.fp .path.pad { padding: 0 8px; }
	@media (max-width: 700px) { .fp .gap { display: none; } }
	.setup-choose { display: flex; flex-direction: column; flex: 1 1 auto; justify-content: flex-start; }
</style>
