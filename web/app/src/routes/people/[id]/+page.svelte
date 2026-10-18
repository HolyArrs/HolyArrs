<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, message } from '#lib/api.ts';
	import { session } from '#lib/session.svelte.ts';
	import FilmographyList from '#lib/FilmographyList.svelte';
	import Poster from '#lib/Poster.svelte';
	import Button from '#lib/ui/Button.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import Icon from '#lib/Icon.svelte';
	import { to } from '#lib/paths.ts';
	import { plainReason } from '#lib/plain.ts';
	import { actionableSelection, normalizeSelection, runReviewed, type FilmographyPage, type PersonMovie, type PersonRole, type ReviewedResult } from '#lib/people.ts';
	import type { AskLimit } from '#lib/types.ts';

	const id = page.params.id ?? '';
	let role = $state<PersonRole>('all');
	let current = $state(1);
	let data = $state<FilmographyPage | null>(null);
	let selected = $state<string[]>([]);
	let reviewed = $state<PersonMovie[] | null>(null);
	let results = $state<ReviewedResult[]>([]);
	let limit = $state<AskLimit | null>(null);
	let busy = $state(false);
	let submitting = $state(false);
	let error = $state('');
	let loadRevision = 0;
	const canSubmit = $derived(session.can('manage_library') || session.can('ask'));
	const action = $derived(session.can('manage_library') ? 'add' : 'ask');

	async function load(pageNumber = current) {
		const revision = ++loadRevision;
		busy = true;
		error = '';
		try {
			const next = await api.get<FilmographyPage>(`/people/${encodeURIComponent(id)}?role=${role}&page=${pageNumber}&page_size=24`);
			if (revision !== loadRevision) return;
			data = next;
			current = pageNumber;
		} catch (err) {
			if (revision === loadRevision) error = message(err);
		} finally {
			if (revision === loadRevision) busy = false;
		}
	}

	function select(tmdb: string, checked: boolean) {
		selected = checked ? normalizeSelection([...selected, tmdb]) : selected.filter((id) => id !== tmdb);
		reviewed = null;
		results = [];
	}

	async function changeRole(next: PersonRole) {
		role = next;
		selected = [];
		reviewed = null;
		results = [];
		await load(1);
	}

	async function review() {
		busy = true;
		error = '';
		try {
			const r = await api.post<{ movies: PersonMovie[]; automatic_future_credits: boolean }>(`/people/${encodeURIComponent(id)}/movies/review`, { tmdb_ids: selected });
			reviewed = r.movies;
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}

	async function submitReviewed() {
		if (!reviewed) return;
		submitting = true;
		results = await runReviewed((path, body) => api.post(path, body), action, actionableSelection(reviewed));
		submitting = false;
		if (action === 'ask') void api.get<AskLimit>('/asks/limit').then((r) => (limit = r)).catch(() => {});
		await load(current);
	}

	onMount(() => {
		void load();
		if (session.can('ask') && !session.can('manage_library')) void api.get<AskLimit>('/asks/limit').then((r) => (limit = r)).catch(() => {});
	});
</script>

<svelte:head><title>{data?.person.name ?? 'Person'} · HolyArrs</title></svelte:head>

<div class="workspace">
<PageHeader title={data?.person.name ?? 'Person'} subtitle={data ? `${data.person.department || 'Department unavailable'}${data.person.birthplace ? ` · ${data.person.birthplace}` : ''}` : undefined} back={{ href: to('/people'), label: 'People' }} />
{#if error && !data}<LoadError hasData={false} message={plainReason(error).text} onretry={() => load(current)} {busy} />
{:else if error}<p class="notice bad" role="alert"><Icon name="alert" size={17} /><span>{plainReason(error).text}</span></p>{/if}
{#if busy && !data}<div class="person-head" aria-busy="true"><Skeleton shape="poster" /><Skeleton lines={4} /></div>{/if}
{#if data}
	<div class="person-head">
		<div class="portrait"><Poster src={data.person.profile} name={data.person.name} /></div>
		<div class="who">
			{#if data.person.biography}<p class="bio">{data.person.biography}</p>{/if}
		</div>
	</div>

	<div class="toolbar">
		<SegmentedControl label="Credit role" value={role} options={[{ value: 'all', label: 'All' }, { value: 'actor', label: 'Actor' }, { value: 'director', label: 'Director' }]} onchange={(v) => changeRole(v as PersonRole)} />
		{#if selected.length}<Button variant="primary" onclick={review} disabled={busy}>Review {selected.length} selected</Button>{/if}
	</div>

	{#if !data.complete}<p class="notice">The provider returns this filmography as one unpaged list and does not promise it is complete.</p>{/if}
	{#if limit && action === 'ask' && !limit.no_limit && !limit.movies.no_limit}
		<p class="note">Movie asks this rolling week: {limit.movies.used} used, {limit.movies.left} left.</p>
	{/if}

	<h2 class="sr-only">Movies</h2>
	<FilmographyList movies={data.movies} {selected} selectable={canSubmit} onselect={select} />
	{#if !data.movies.length}<EmptyState compact icon="film" title="No movies match this role on this page." />{/if}

	<nav class="pages" aria-label="Filmography pages">
		<Button icon="chevronLeft" disabled={busy || current <= 1} onclick={() => load(current - 1)}>Previous</Button>
		<span class="note">Page {current}{#if data.total_pages} of {data.total_pages}{/if}</span>
		<Button iconAfter="chevronRight" disabled={busy || current >= data.total_pages} onclick={() => load(current + 1)}>Next</Button>
	</nav>

	{#if reviewed}
		<section class="card review" aria-labelledby="review-title">
			<h2 id="review-title">Review {reviewed.length} movie{reviewed.length === 1 ? '' : 's'}</h2>
			<p>Only these movies will be {action === 'add' ? 'added' : 'asked for'}. Future credits are not followed automatically.</p>
			<ul>{#each reviewed as movie (movie.ids.tmdb)}<li>{movie.name}{#if movie.year} ({movie.year}){/if}</li>{/each}</ul>
			<Button variant="primary" disabled={!actionableSelection(reviewed).length} busy={submitting} onclick={submitReviewed}>{action === 'add' ? 'Add reviewed movies' : 'Ask for reviewed movies'}</Button>
		</section>
	{/if}
	{#if results.length}
		<section class="card" aria-labelledby="results-title">
			<h2 id="results-title">Results</h2>
			<ul class="results">{#each results as result}<li class:failed={!result.ok}>{result.tmdb_id}: {result.ok ? 'Submitted' : result.error}</li>{/each}</ul>
		</section>
	{/if}
	<p class="attribution">{data.attribution}</p>
{/if}
</div>

<style>
	.workspace { display: flex; flex-direction: column; gap: 16px; padding-bottom: 48px; min-width: 0; }
	.person-head { display: grid; grid-template-columns: 140px minmax(0, 1fr); gap: 24px; align-items: start; }
	.portrait { width: 140px; border-radius: var(--r-card); overflow: hidden; box-shadow: var(--shadow-card); }
	.who { min-width: 0; }
	.bio { margin: 0; max-width: 76ch; font: var(--t-body); line-height: 1.6; text-wrap: pretty; }
	.toolbar { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; }
	.pages { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
	.note { margin: 0; color: var(--text-muted); font: var(--t-subhead); font-weight: 400; font-variant-numeric: tabular-nums; }
	.notice { display: flex; align-items: center; gap: 8px; margin: 0; padding: 10px 12px; border-radius: var(--r-btn); background: var(--surface-2); font: var(--t-callout); }
	.notice.bad { color: var(--fail); }
	.card { margin: 0; padding: 16px 20px 20px; border: 0; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: var(--shadow-card); font: var(--t-callout); }
	.card h2 { margin: 0 0 8px; font: var(--t-title3); }
	.card p, .card ul { margin: 0 0 12px; }
	.card ul { padding-left: 20px; }
	.failed { color: var(--fail); }
	.attribution { margin: 0; color: var(--text-muted); font: var(--t-footnote); }
	@media (max-width: 560px) {
		.person-head { grid-template-columns: 96px minmax(0, 1fr); gap: 14px; }
		.portrait { width: 96px; }

		.toolbar :global(.ui-btn) { width: 100%; }
	}
</style>
