<script lang="ts">
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { session } from '#lib/session.svelte.ts';
	import { to } from '#lib/paths.ts';
	import { codecLabel, copiesQuery, copyCodecs, copyKey, copySorts, toggleCopy, type CopyCodec, type CopySearchPreview, type CopySearchResult, type CopySelection, type CopySort, type LibraryCopyPage } from '#lib/library-copies.ts';

	const pageSize = 50;
	let page = $state<LibraryCopyPage>({ copies: [], total: 0, limit: pageSize, offset: 0, codecs: {} });
	let selected = $state<CopySelection[]>([]);
	let preview = $state<CopySearchPreview | null>(null);
	let result = $state<CopySearchResult | null>(null);
	let q = $state('');
	let codec = $state<CopyCodec>('all');
	let sort = $state<CopySort>('title');
	let offset = $state(0);
	let loaded = $state(false);
	let busy = $state(false);
	let error = $state('');

	const selectedKeys = $derived(new Set(selected.map(copyKey)));
	const last = $derived(Math.min(page.total, page.offset + page.copies.length));

	async function load() {
		busy = true;
		try {
			page = await api.get<LibraryCopyPage>(copiesQuery({ q, codec, sort, limit: pageSize, offset }));
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
			loaded = true;
		}
	}

	function applyFilters() {
		offset = 0;
		void load();
	}

	function toggle(copy: CopySelection) {
		selected = toggleCopy(selected, copy);
		preview = null;
		result = null;
	}

	async function review() {
		busy = true;
		try {
			preview = await api.post<CopySearchPreview>('/library/copies/search/preview', { selections: selected });
			result = null;
			error = '';
		} catch (e) {
			error = message(e);
			preview = null;
		} finally {
			busy = false;
		}
	}

	async function search() {
		if (!preview) return;
		busy = true;
		try {
			result = await api.post<CopySearchResult>('/library/copies/search', { selections: selected, review_token: preview.review_token });
			preview = null;
			error = '';
		} catch (e) {
			error = message(e);
			preview = null;
		} finally {
			busy = false;
		}
	}

	function move(by: number) {
		offset = Math.max(0, offset + by);
		void load();
	}

	onMount(load);
</script>

<svelte:head><title>Video codecs · HolyArrs</title></svelte:head>

<header class="page-head">
	<div>
		<p class="eyebrow"><a href={to('/library')}>Library</a></p>
		<h1>Video codecs</h1>
		<p>See the codec read from each movie file and search selected Versions for alternatives.</p>
	</div>
</header>

{#if !session.can('manage_library')}
	<p class="notice bad" role="alert">You do not have permission to manage the Library.</p>
{:else}
	<form class="tools" onsubmit={(event) => { event.preventDefault(); applyFilters(); }}>
		<label>Find by title <input class="input" type="search" bind:value={q} maxlength="256" /></label>
		<label>Codec
			<select class="input" bind:value={codec} onchange={applyFilters}>
				{#each copyCodecs as value}<option {value}>{value === 'all' ? 'All codecs' : `${codecLabel(value)} (${page.codecs[value] ?? 0})`}</option>{/each}
			</select>
		</label>
		<label>Sort
			<select class="input" bind:value={sort} onchange={applyFilters}>
				{#each copySorts as value}<option {value}>{value === 'title' ? 'Title' : value === 'codec' ? 'Codec' : 'Recently added'}</option>{/each}
			</select>
		</label>
		<button class="btn btn-ghost" type="submit">Apply</button>
	</form>

	{#if error}<p class="notice bad" role="alert">{error}</p>{/if}
	{#if result}<p class="notice good" role="status">Started {result.job_ids.length} search{result.job_ids.length === 1 ? '' : 'es'}. Results will use the current Pick rules.</p>{/if}

	<div class="selection-bar">
		<span><b>{selected.length}</b> selected across all pages</span>
		<button class="btn btn-dark" type="button" disabled={selected.length === 0 || busy} onclick={review}>Review searches</button>
	</div>

	{#if preview}
		<section class="review" aria-labelledby="review-title">
			<h2 id="review-title">Review {preview.copies.length} {preview.copies.length === 1 ? 'copy' : 'copies'}</h2>
			<p>This starts {preview.slot_count} search{preview.slot_count === 1 ? '' : 'es'} for the exact selected Versions. Each search applies the current Pick rules and saves candidates for review. It does not guarantee a replacement or download anything automatically.</p>
			<ul>{#each preview.copies as copy (copyKey(copy))}<li><b>{copy.title}</b> · {copy.version} · {codecLabel(copy.codec)}</li>{/each}</ul>
			<div class="row"><button class="btn btn-dark" type="button" disabled={busy} onclick={search}>Start searches</button><button class="btn btn-quiet" type="button" onclick={() => preview = null}>Cancel</button></div>
		</section>
	{/if}

	{#if !loaded}
		<p class="muted">Reading inspected movie files…</p>
	{:else if page.copies.length === 0}
		<p class="muted">No movie files match these filters.</p>
	{:else}
		<div class="table-wrap">
			<table class="list-table">
				<caption class="sr-only">Inspected movie files and video codecs</caption>
				<thead><tr><th scope="col"><span class="sr-only">Select</span></th><th scope="col">Movie and Version</th><th scope="col">Codec</th><th scope="col">Audio</th><th scope="col">File</th></tr></thead>
				<tbody>
					{#each page.copies as copy (copyKey(copy))}
						<tr>
							<td><input type="checkbox" aria-label={`Select ${copy.title}, ${copy.version}`} checked={selectedKeys.has(copyKey(copy))} disabled={copy.protected} onchange={() => toggle(copy)} /></td>
							<td><a href={to(`/library/${copy.title_id}`)}><b>{copy.title}</b>{#if copy.year} <span class="muted">({copy.year})</span>{/if}</a><span class="block small">{copy.version}{copy.edition ? ` · ${copy.edition}` : ''}</span>{#if copy.protected}<span class="chip">Protected</span>{/if}</td>
							<td><span class="codec">{codecLabel(copy.codec)}</span><span class="block xs muted">{copy.container || 'Container unknown'} · {copy.inspection_status || 'not inspected'}</span></td>
							<td>{copy.audio_languages.length ? copy.audio_languages.join(', ') : 'Unknown'}<span class="block xs muted">{copy.audio_tracks} track{copy.audio_tracks === 1 ? '' : 's'}{copy.audio_unknown ? ' · some unknown' : ''}</span></td>
							<td class="path">{copy.relative_path}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
		<nav class="pager" aria-label="Codec pages">
			<button class="btn btn-quiet" type="button" disabled={page.offset === 0 || busy} onclick={() => move(-pageSize)}>Previous</button>
			<span>{page.offset + 1}–{last} of {page.total}</span>
			<button class="btn btn-quiet" type="button" disabled={last >= page.total || busy} onclick={() => move(pageSize)}>Next</button>
		</nav>
	{/if}
{/if}

<style>
	.eyebrow { margin: 0 0 var(--s-2); font-size: 11px; font-weight: var(--weight-semibold); letter-spacing: .09em; text-transform: uppercase; color: var(--text-muted); }
	.tools { display: flex; align-items: end; gap: var(--s-3); flex-wrap: wrap; margin-bottom: 0; padding: var(--s-4); border: 1px solid var(--line); border-radius: var(--r-card) var(--r-card) 0 0; background: var(--surface-1); }
	.tools label { display: grid; gap: var(--s-1); font-size: var(--text-xs); color: var(--text-muted); }
	.selection-bar { display: flex; align-items: center; justify-content: space-between; gap: var(--s-4); min-height: 64px; padding: var(--s-2) var(--s-4); border: 1px solid var(--line); border-top: 0; background: var(--surface-2); }
	.review { border: 1px solid var(--line); border-radius: var(--r-card); padding: var(--s-5); margin: var(--s-4) 0; background: var(--surface-1); }
	.review h2 { margin-top: 0; }
	.review ul { margin: .75rem 0; padding-left: 1.25rem; }
	.table-wrap { overflow-x: auto; border: 1px solid var(--line); border-radius: var(--r-card); background: var(--surface-1); }
	.list-table { width: 100%; border-collapse: collapse; }
	.list-table th { padding-inline: var(--s-3); text-align: left; background: var(--surface-2); }
	.list-table td { padding: var(--s-3); vertical-align: top; }
	.list-table tbody tr + tr { border-top: 1px solid var(--line); }
	.list-table tbody tr:hover { background: var(--surface-2); }
	.path { max-width: 24rem; overflow-wrap: anywhere; color: var(--text-muted); font-size: var(--text-sm); }
	.codec { font-weight: 700; }
	.block { display: block; }
	.pager { display: flex; justify-content: space-between; align-items: center; gap: var(--s-4); margin-top: var(--s-4); }
	@media (max-width: 700px) {
		.tools, .tools label { width: 100%; }
		.tools .input, .tools button { width: 100%; }
		.selection-bar { align-items: stretch; flex-direction: column; }
		.selection-bar :global(.btn), .pager :global(.btn) { min-height: 44px; }
		.list-table th:nth-child(4), .list-table td:nth-child(4), .list-table th:nth-child(5), .list-table td:nth-child(5) { display: none; }
	}
</style>
