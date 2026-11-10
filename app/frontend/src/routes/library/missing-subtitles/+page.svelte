<script lang="ts">
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import Poster from '#lib/ui/Poster.svelte';
	import Button from '#lib/ui/Button.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import ProgressBar from '#lib/ui/ProgressBar.svelte';
	import { titleArt, pager, LibraryToolPage } from '#lib/mediafiles/index.ts';
	import { jobsDone, langName, missingName, missingQuery, missingWords, wantLabel, type Language, type MissingRow, type MissingTitle } from '#lib/subtitles/index.ts';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { to } from '#lib/core/paths.ts';
	import { api, message } from '#lib/core/api.ts';
	import { dateTime } from '#lib/core/format.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { plainReason } from '#lib/core/plain.ts';
	import { t as tr } from '#lib/core/i18n/i18n.svelte.ts';

	// Missing subtitles: every movie and episode missing a wished subtitle.

	const limit = 100;
	let rows = $state<MissingRow[]>([]);
	let titles = $state<MissingTitle[]>([]);
	let langs = $state<Language[]>([]);
	let total = $state(0);
	let offset = $state(0);
	let loaded = $state(false);
	let error = $state('');
	let searchingRow = $state(0);

	let language = $state(page.url.searchParams.get('language') ?? '');
	let kind = $state(page.url.searchParams.get('kind') ?? '');
	let title = $state(Number(page.url.searchParams.get('title')) || 0);
	const pages = $derived(pager(total, limit, offset));

	// Search all: the Jobs it queued and how far they are.
	let jobIds = $state<number[]>([]);
	let jobStates = $state<Record<number, string>>({});
	let searchAllPending = $state(false);
	const done = $derived(jobsDone(jobIds, jobStates));
	const running = $derived(jobIds.length > 0 && done < jobIds.length);
	const searchAllBusy = $derived(searchAllPending || running);

	async function load() {
		try {
			const r = await api.get<{ missing: MissingRow[]; total: number; titles: MissingTitle[] }>(
				'/subtitles/missing?' + missingQuery({ language, kind, title }, limit, offset)
			);
			rows = r.missing;
			total = r.total;
			titles = r.titles;
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}

	function filter() {
		offset = 0;
		const q = new URLSearchParams();
		if (language) q.set('language', language);
		if (kind) q.set('kind', kind);
		if (title) q.set('title', String(title));
		const s = q.toString();
		void goto(to('/library/missing-subtitles') + (s ? `?${s}` : ''), { replace: true, reset: false });
		void load();
	}

	async function poll() {
		if (!running) return;
		try {
			const r = await api.get<{ jobs: { ID: number; State: string }[] }>('/jobs?limit=200');
			for (const j of r.jobs) if (jobIds.includes(j.ID)) jobStates[j.ID] = j.State;
		} catch {
			// The next tick tries again.
		}
	}

	// Posters for the rows, from one brief read of the Library.
	let art = $state<Map<number, string>>(new Map());
	onMount(() => {
		void load();
		void titleArt().then((m) => (art = m));
		api.get<{ languages: Language[] }>('/subtitles/languages').then((r) => (langs = r.languages), () => {});
		let timer: ReturnType<typeof setTimeout> | undefined;
		const soon = () => {
			clearTimeout(timer);
			timer = setTimeout(load, 500);
		};
		const tick = setInterval(poll, 3000);
		const offs = [
			live.on('job', (d) => {
				const j = d as { id?: number; state?: string };
				if (j?.id && jobIds.includes(j.id) && j.state) {
					jobStates[j.id] = j.state;
					if (['done', 'failed', 'cancelled'].includes(j.state)) soon();
				}
			}),
			live.on('event', (d) => {
				if (String((d as { Kind?: string })?.Kind).startsWith('subtitle_')) soon();
			}),
			live.on('resync', soon)
		];
		return () => {
			clearTimeout(timer);
			clearInterval(tick);
			offs.forEach((off) => off());
		};
	});

	async function searchAll() {
		if (searchAllBusy) return;
		searchAllPending = true;
		try {
			const body: Record<string, unknown> = {};
			if (language) body.language = language;
			if (kind) body.kind = kind;
			if (title) body.title_id = title;
			const r = await api.post<{ queued: number; job_ids: number[] }>('/subtitles/missing/search', body);
			jobIds = r.job_ids ?? [];
			jobStates = {};
			toasts.push(tr('shows.sheet.searchingSubs'), 'info', { body: tr('mediafiles.unit.file.' + (r.queued === 1 ? 'one' : 'other'), { n: r.queued }) });
		} catch (e) {
			toasts.push(tr('mediafiles.title.startFailed'), 'fail', { body: message(e) });
		} finally {
			searchAllPending = false;
		}
	}

	async function searchOne(r: MissingRow) {
		searchingRow = r.file_id;
		try {
			const q = await api.post<{ job_ids: number[] }>('/subtitles/search', { file_id: r.file_id });
			toasts.push(tr('shows.sheet.searchingSubs'), 'info', { body: missingName(r) });
			for (const id of q.job_ids ?? []) if (!jobIds.includes(id)) jobIds.push(id);
		} catch (e) {
			toasts.push(tr('mediafiles.title.startFailed'), 'fail', { body: message(e) });
		} finally {
			searchingRow = 0;
		}
	}

	function go(off: number | null) {
		if (off === null) return;
		offset = off;
		void load();
	}
</script>

<svelte:head><title>{tr('library.missingSubs.title')} · HolyArrs</title></svelte:head>

<LibraryToolPage title={tr('library.missingSubs.title')} count={loaded && !error ? tr('library.missingSubs.count', { n: total }) : ''}>
	{#snippet bar()}
		<span class="tp-cap">
			<select aria-label={tr('library.missingSubs.language')} bind:value={language} onchange={filter}>
				<option value="">{tr('library.missingSubs.every')}</option>
				{#each langs as l (l.code)}<option value={l.code}>{l.name}</option>{/each}
			</select>
		</span>
		<span class="tp-cap">
			<select aria-label={tr('library.missingSubs.kind')} bind:value={kind} onchange={filter}>
				<option value="">{tr('library.missingSubs.both')}</option>
				<option value="movie">{tr('mediafiles.kinds.movie')}</option>
				<option value="episode">{tr('shows.list.episodes')}</option>
			</select>
		</span>
		<span class="tp-cap">
			<select aria-label={tr('library.missingSubs.movieOrShow')} bind:value={title} onchange={filter}>
				<option value={0}>{tr('library.missingSubs.all')}</option>
				{#each titles as t (t.id)}<option value={t.id}>{t.name} ({t.count})</option>{/each}
			</select>
		</span>
		<span class="tp-gap"></span>
		<span class="tp-cap">
			<Button variant="primary" icon="search" onclick={searchAll} disabled={total === 0} busy={searchAllBusy} data-testid="search-all-subtitles">{tr('library.missingSubs.searchAll')}</Button>
		</span>
	{/snippet}

	{#if jobIds.length}
		<div class="tp-sheet tp-pad progress" aria-live="polite" data-testid="search-all-progress">
			<ProgressBar value={(done / jobIds.length) * 100} label={tr('library.missingSubs.done')} />
			<p class="tp-note">{running ? tr('library.missingSubs.searched', { done, total: jobIds.length }) : tr(jobIds.length === 1 ? 'library.missingSubs.doneOne' : 'library.missingSubs.doneMany', { n: jobIds.length })}</p>
		</div>
	{/if}

	{#if error}
		<LoadError hasData={false} message={plainReason(error).text} onretry={load} />
	{:else}
	<PageLoader loading={!loaded} label={tr('library.missingSubs.loading')}>
	{#if total === 0}
		<EmptyState compact icon="subtitle" title={language || kind || title ? tr('library.copies.nothingMatches') : tr('mediafiles.list.nothingMissing')} />
	{:else}
		<div class="tp-sheet table-wrap">
			<table class="tp-table missing">
				<caption class="sr-only">{tr('library.missingSubs.caption')}</caption>
				<thead><tr><th scope="col">{tr('mediafiles.list.title')}</th><th scope="col">{tr('library.missingSubs.language')}</th><th scope="col">{tr('mediafiles.col.status')}</th><th scope="col"><span class="sr-only">{tr('mediafiles.dialogs.search')}</span></th></tr></thead>
				<tbody>
					{#each rows as r (r.file_id + r.language + String(r.forced))}
						<tr data-testid="missing-subtitle">
							<td><span class="tp-who"><span class="tp-art" style="--poster-ratio: 2 / 3"><Poster src={art.get(r.title_id)} name={missingName(r)} /></span><a href={to(`/library/${r.title_id}`)}>{missingName(r)}</a></span></td>
							<td data-testid="missing-language">{wantLabel({ language: r.language, hearing_impaired: r.hearing_impaired ? 'hi' : 'normal', forced: r.forced }, langs)}</td>
							<td class="status">{missingWords(r, dateTime)}</td>
							<td class="act">
								<Button
									icon="search"
									onclick={() => searchOne(r)}
									disabled={searchingRow === r.file_id}
									aria-label={tr('library.missingSubs.searchFor', { language: langName(r.language, langs), name: missingName(r) })}
									data-testid="missing-search">{tr('mediafiles.dialogs.search')}</Button
								>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
		{#if pages.prev !== null || pages.next !== null}
			<nav class="tp-foot" aria-label={tr('mediafiles.releaseList.pages')}>
				<Button icon="chevronLeft" disabled={pages.prev === null} onclick={() => go(pages.prev)}>{tr('mediafiles.releaseList.back')}</Button>
				<span class="tp-note">{tr('library.copies.range', { from: pages.from, to: pages.to, total })}</span>
				<Button iconAfter="chevronRight" disabled={pages.next === null} onclick={() => go(pages.next)}>{tr('mediafiles.releaseList.more')}</Button>
			</nav>
		{/if}
	{/if}
	</PageLoader>
	{/if}
</LibraryToolPage>

<style>
	.progress { display: grid; gap: 8px; }
	.table-wrap { overflow-x: auto; }
	.status { color: var(--text-muted); font: var(--t-footnote); }
	.act { width: 1%; text-align: right; white-space: nowrap; }
	@media (max-width: 639px) {
		.missing th:nth-child(3), .missing td:nth-child(3) { display: none; }
	}
	/* The title link is the 44px tap target on touch phones. */
	@media (pointer: coarse) { .missing .tp-who > a { display: inline-block; padding-block: 13px; } }
</style>
