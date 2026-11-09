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
			toasts.push('Searching for subtitles', 'info', { body: r.queued === 1 ? '1 file' : `${r.queued} files` });
		} catch (e) {
			toasts.push(`Couldn't start search`, 'fail', { body: message(e) });
		} finally {
			searchAllPending = false;
		}
	}

	async function searchOne(r: MissingRow) {
		searchingRow = r.file_id;
		try {
			const q = await api.post<{ job_ids: number[] }>('/subtitles/search', { file_id: r.file_id });
			toasts.push('Searching for subtitles', 'info', { body: missingName(r) });
			for (const id of q.job_ids ?? []) if (!jobIds.includes(id)) jobIds.push(id);
		} catch (e) {
			toasts.push(`Couldn't start search`, 'fail', { body: message(e) });
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

<svelte:head><title>Missing subtitles · HolyArrs</title></svelte:head>

<LibraryToolPage title="Missing subtitles" count={loaded && !error ? `${total} missing` : ''}>
	{#snippet bar()}
		<span class="tp-cap">
			<select aria-label="Language" bind:value={language} onchange={filter}>
				<option value="">Every language</option>
				{#each langs as l (l.code)}<option value={l.code}>{l.name}</option>{/each}
			</select>
		</span>
		<span class="tp-cap">
			<select aria-label="Kind" bind:value={kind} onchange={filter}>
				<option value="">Movies and episodes</option>
				<option value="movie">Movies</option>
				<option value="episode">Episodes</option>
			</select>
		</span>
		<span class="tp-cap">
			<select aria-label="Movie or show" bind:value={title} onchange={filter}>
				<option value={0}>All of them</option>
				{#each titles as t (t.id)}<option value={t.id}>{t.name} ({t.count})</option>{/each}
			</select>
		</span>
		<span class="tp-gap"></span>
		<span class="tp-cap">
			<Button variant="primary" icon="search" onclick={searchAll} disabled={total === 0} busy={searchAllBusy} data-testid="search-all-subtitles">Search all</Button>
		</span>
	{/snippet}

	{#if jobIds.length}
		<div class="tp-sheet tp-pad progress" aria-live="polite" data-testid="search-all-progress">
			<ProgressBar value={(done / jobIds.length) * 100} label="Subtitle searches done" />
			<p class="tp-note">{running ? `Searched ${done} of ${jobIds.length} files.` : `Done: searched ${jobIds.length} ${jobIds.length === 1 ? 'file' : 'files'}.`}</p>
		</div>
	{/if}

	{#if error}
		<LoadError hasData={false} message={plainReason(error).text} onretry={load} />
	{:else}
	<PageLoader loading={!loaded} label="Loading missing subtitles">
	{#if total === 0}
		<EmptyState compact icon="subtitle" title={language || kind || title ? 'Nothing matches.' : 'Nothing missing.'} />
	{:else}
		<div class="tp-sheet table-wrap">
			<table class="tp-table missing">
				<caption class="sr-only">Movies and episodes missing subtitles</caption>
				<thead><tr><th scope="col">Title</th><th scope="col">Language</th><th scope="col">Status</th><th scope="col"><span class="sr-only">Search</span></th></tr></thead>
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
									aria-label="Search {langName(r.language, langs)} subtitles for {missingName(r)}"
									data-testid="missing-search">Search</Button
								>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
		{#if pages.prev !== null || pages.next !== null}
			<nav class="tp-foot" aria-label="Pages">
				<Button icon="chevronLeft" disabled={pages.prev === null} onclick={() => go(pages.prev)}>Back</Button>
				<span class="tp-note">{pages.from}–{pages.to} of {total}</span>
				<Button iconAfter="chevronRight" disabled={pages.next === null} onclick={() => go(pages.next)}>More</Button>
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
