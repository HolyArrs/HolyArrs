<script lang="ts">
	import { to } from '#lib/paths.ts';
	// Missing subtitles: every movie and episode missing a wished subtitle,
	// by language, kind and show; Search all with its progress, or one row.
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api, message } from '#lib/api.ts';
	import { dateTime } from '#lib/format.ts';
	import { live } from '#lib/live.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { pager } from '#lib/releases.ts';
	import EmptyState from '#lib/EmptyState.svelte';
	import Icon from '#lib/Icon.svelte';
	import { jobsDone, langName, missingName, missingQuery, missingWords, wantLabel, type Language, type MissingRow, type MissingTitle } from '#lib/subtitles.ts';

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
	const done = $derived(jobsDone(jobIds, jobStates));
	const running = $derived(jobIds.length > 0 && done < jobIds.length);

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
			// the next tick tries again
		}
	}

	onMount(() => {
		void load();
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
		try {
			const body: Record<string, unknown> = {};
			if (language) body.language = language;
			if (kind) body.kind = kind;
			if (title) body.title_id = title;
			const r = await api.post<{ queued: number; job_ids: number[] }>('/subtitles/missing/search', body);
			jobIds = r.job_ids ?? [];
			jobStates = {};
			toasts.push(r.queued === 1 ? 'Looking for 1 file now.' : `Looking for ${r.queued} files now.`, 'ok');
		} catch (e) {
			toasts.push(message(e), 'fail');
		}
	}

	async function searchOne(r: MissingRow) {
		searchingRow = r.file_id;
		try {
			const q = await api.post<{ job_ids: number[] }>('/subtitles/search', { file_id: r.file_id });
			toasts.push(`Looking for subtitles of ${missingName(r)} now.`, 'ok');
			for (const id of q.job_ids ?? []) if (!jobIds.includes(id)) jobIds.push(id);
		} catch (e) {
			toasts.push(message(e), 'fail');
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

<header class="page-head">
	<div>
		<p class="eyebrow">Library management</p>
		<h1>Missing subtitles</h1>
		<p>Every movie and episode without a subtitle it should have. HolyArrs keeps looking on its own; search now to look at once.</p>
	</div>
	<button class="btn btn-dark" type="button" onclick={searchAll} disabled={running || total === 0} aria-busy={running} data-testid="search-all-subtitles">
		<Icon name="search" size={18} /> Search all
	</button>
</header>

<form class="filters" onsubmit={(e) => (e.preventDefault(), filter())} aria-label="Filters">
	<div class="field">
		<label for="f-lang">Language</label>
		<select id="f-lang" class="input" bind:value={language} onchange={filter}>
			<option value="">Every language</option>
			{#each langs as l (l.code)}<option value={l.code}>{l.name}</option>{/each}
		</select>
	</div>
	<div class="field">
		<label for="f-kind">Kind</label>
		<select id="f-kind" class="input" bind:value={kind} onchange={filter}>
			<option value="">Movies and episodes</option>
			<option value="movie">Movies</option>
			<option value="episode">Episodes</option>
		</select>
	</div>
	<div class="field">
		<label for="f-title">Movie or show</label>
		<select id="f-title" class="input" bind:value={title} onchange={filter}>
			<option value={0}>All of them</option>
			{#each titles as t (t.id)}<option value={t.id}>{t.name} ({t.count})</option>{/each}
		</select>
	</div>
</form>

{#if jobIds.length}
	<div class="progress" aria-live="polite" data-testid="search-all-progress">
		<progress class="bar" max={jobIds.length} value={done} aria-label="Subtitle searches done"></progress>
		<p class="xs muted">{running ? `Searched ${done} of ${jobIds.length} files.` : `Done: searched ${jobIds.length} ${jobIds.length === 1 ? 'file' : 'files'}.`}</p>
	</div>
{/if}

{#if error}
	<p class="notice bad" role="alert">{error}</p>
{:else if !loaded}
	<div class="sk" style="height: 160px"></div>
{:else if total === 0}
	<EmptyState icon="subtitle" title={language || kind || title ? 'Nothing missing here' : 'No subtitles missing'}>
		<p>
			{language || kind || title
				? 'Nothing matches these filters.'
				: 'Every movie and episode has the subtitles it should. Set the languages in Settings > Subtitles or on a movie or show.'}
		</p>
	</EmptyState>
{:else}
	<p class="small muted count">{total} missing {total === 1 ? 'subtitle' : 'subtitles'}</p>
	<ul class="card rows-list">
		{#each rows as r (r.file_id + r.language + String(r.forced))}
			<li class="row-item" data-testid="missing-subtitle">
				<div class="grow">
					<a class="small" href={to(`/library/${r.title_id}`)}><b>{missingName(r)}</b></a>
					<div class="xs">
						<span data-testid="missing-language">{wantLabel({ language: r.language, hearing_impaired: r.hearing_impaired ? 'hi' : 'normal', forced: r.forced }, langs)}</span>
						<span class="muted"> · {missingWords(r, dateTime)}</span>
					</div>
				</div>
				<button
					class="btn btn-quiet btn-sm"
					type="button"
					onclick={() => searchOne(r)}
					disabled={searchingRow === r.file_id}
					aria-label="Search {langName(r.language, langs)} subtitles for {missingName(r)}"
					data-testid="missing-search">Search</button
				>
			</li>
		{/each}
	</ul>
	{#if pages.prev !== null || pages.next !== null}
		<nav class="row pager" aria-label="Pages">
			<button class="btn btn-quiet" type="button" disabled={pages.prev === null} onclick={() => go(pages.prev)}>Back</button>
			<span class="xs muted">{pages.from} to {pages.to} of {total}</span>
			<button class="btn btn-quiet" type="button" disabled={pages.next === null} onclick={() => go(pages.next)}>More</button>
		</nav>
	{/if}
{/if}

<style>
	.eyebrow { margin: 0 0 var(--s-2); font-size: 11px; font-weight: var(--weight-semibold); letter-spacing: .09em; text-transform: uppercase; color: var(--text-muted); }
	.filters {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: var(--s-3);
		margin-bottom: var(--s-4);
		padding: var(--s-3);
		border: 1px solid var(--line);
		border-radius: var(--r-card);
		background: var(--surface-1);
	}
	.filters .field + .field {
		margin-top: 0;
	}
	.progress {
		margin-bottom: var(--s-4);
	}
	.count {
		min-height: 52px;
		margin: 0;
		padding: var(--s-3) var(--s-4);
		border: 1px solid var(--line);
		border-bottom: 0;
		border-radius: var(--r-card) var(--r-card) 0 0;
		background: var(--surface-2);
		display: flex;
		align-items: center;
	}
	.rows-list {
		padding: 0;
		border-radius: 0 0 var(--r-card) var(--r-card);
		overflow: hidden;
		box-shadow: none;
	}
	.row-item {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		padding: var(--s-3) var(--s-4);
		border: 0;
		border-radius: 0;
		background: transparent;
	}
	.row-item + .row-item {
		margin-top: 0;
		border-top: 1px solid var(--line);
	}
	.grow {
		flex: 1;
		min-width: 0;
	}
	.btn-sm {
		min-height: var(--tap);
		padding-inline: var(--s-3);
		font-size: var(--text-xs);
	}
	.pager {
		justify-content: space-between;
		margin-top: var(--s-3);
	}
	@media (max-width: 600px) {
		.filters {
			grid-template-columns: 1fr;
		}
		.page-head > :global(.btn), .row-item :global(.btn), .pager :global(.btn) { min-height: 44px; }
		.page-head > :global(.btn) { width: 100%; }
		.row-item { align-items: flex-start; flex-direction: column; }
		.row-item :global(.btn) { width: 100%; }
	}
</style>
