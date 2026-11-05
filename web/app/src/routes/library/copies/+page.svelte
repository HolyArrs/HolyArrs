<script lang="ts">
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { session } from '#lib/session.svelte.ts';
	import { to } from '#lib/paths.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Badge from '#lib/ui/Badge.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import Icon from '#lib/Icon.svelte';
	import { plainReason } from '#lib/plain.ts';
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

<div class="page">
<PageHeader title="Video codecs" back={{ href: to('/library'), label: 'Library' }} />

{#if !session.can('manage_library')}
	<EmptyState icon="shield" title="You do not have permission to manage the Library." />
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
		<Button type="submit">Apply</Button>
	</form>

	{#if error}<p class="notice bad" role="alert"><Icon name="alert" size={17} /><span>{plainReason(error).text}</span></p>{/if}
	{#if result}<p class="notice good" role="status"><Icon name="check" size={17} />Started {result.job_ids.length} search{result.job_ids.length === 1 ? '' : 'es'}. Results will use the current Pick rules.</p>{/if}

	<div class="selection-bar">
		<span><b>{selected.length}</b> selected across all pages</span>
		<Button variant="primary" disabled={selected.length === 0 || busy} onclick={review}>Review searches</Button>
	</div>

	{#if preview}
		<section class="review" aria-labelledby="review-title">
			<h2 id="review-title">Review {preview.copies.length} {preview.copies.length === 1 ? 'copy' : 'copies'}</h2>
			<p>This starts {preview.slot_count} search{preview.slot_count === 1 ? '' : 'es'} for the exact selected Versions. Each search applies the current Pick rules and saves candidates for review. It does not guarantee a replacement or download anything automatically.</p>
			<ul>{#each preview.copies as copy (copyKey(copy))}<li><b>{copy.title}</b> · {copy.version} · {codecLabel(copy.codec)}</li>{/each}</ul>
			<div class="actions"><Button variant="primary" disabled={busy} onclick={search}>Start searches</Button><Button variant="plain" onclick={() => preview = null}>Cancel</Button></div>
		</section>
	{/if}

	<PageLoader loading={!loaded} label="Loading copies">
	{#if page.copies.length === 0}
		<EmptyState compact icon="film" title="No movie files match these filters." />
	{:else}
		<div class="table-wrap">
			<table class="copies">
				<caption class="sr-only">Inspected movie files and video codecs</caption>
				<thead><tr><th scope="col"><span class="sr-only">Select</span></th><th scope="col">Movie and Version</th><th scope="col">Codec</th><th scope="col">Audio</th><th scope="col">File</th></tr></thead>
				<tbody>
					{#each page.copies as copy (copyKey(copy))}
						<tr>
							<td class="pick"><label><input type="checkbox" aria-label={`Select ${copy.title}, ${copy.version}`} checked={selectedKeys.has(copyKey(copy))} disabled={copy.protected} onchange={() => toggle(copy)} /></label></td>
							<td><a href={to(`/library/${copy.title_id}`)}><b>{copy.title}</b>{#if copy.year} <span class="muted">({copy.year})</span>{/if}</a><span class="block small">{copy.version}{copy.edition ? ` · ${copy.edition}` : ''}</span>{#if copy.protected}<Badge label="Protected" />{/if}</td>
							<td><span class="codec">{codecLabel(copy.codec)}</span><span class="block xs muted">{copy.container || 'Container unknown'} · {copy.inspection_status || 'not inspected'}</span></td>
							<td>{copy.audio_languages.length ? copy.audio_languages.join(', ') : 'Unknown'}<span class="block xs muted">{copy.audio_tracks} track{copy.audio_tracks === 1 ? '' : 's'}{copy.audio_unknown ? ' · some unknown' : ''}</span></td>
							<td class="path">{copy.relative_path}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
		<nav class="pager" aria-label="Codec pages">
			<Button icon="chevronLeft" disabled={page.offset === 0 || busy} onclick={() => move(-pageSize)}>Previous</Button>
			<span class="count">{page.offset + 1}–{last} of {page.total}</span>
			<Button iconAfter="chevronRight" disabled={last >= page.total || busy} onclick={() => move(pageSize)}>Next</Button>
		</nav>
	{/if}
	</PageLoader>
{/if}
</div>

<style>
	.page { display: flex; flex-direction: column; gap: 16px; padding-bottom: 48px; min-width: 0; }
	.tools { display: flex; align-items: end; gap: 12px; flex-wrap: wrap; margin: 0; }
	.tools label { display: grid; gap: 4px; font: var(--t-subhead); color: var(--text-muted); }
	.selection-bar { display: flex; align-items: center; justify-content: space-between; gap: 16px; min-height: 60px; padding: 8px 8px 8px 16px; border-radius: var(--r-panel); background: var(--surface-2); font: var(--t-callout); }
	.notice { display: flex; align-items: center; gap: 8px; margin: 0; padding: 10px 12px; border: 0; border-radius: var(--r-btn); background: var(--surface-2); font: var(--t-callout); }
	.notice.bad { color: var(--fail); }
	.notice.good { color: var(--text); }
	.notice.good :global(svg) { color: var(--ok); }
	.review { border-radius: var(--r-panel); padding: 16px 20px 20px; margin: 0; background: var(--surface-1); box-shadow: var(--shadow-card); font: var(--t-callout); }
	.review h2 { margin: 0 0 8px; font: var(--t-title3); }
	.review ul { margin: .75rem 0; padding-left: 1.25rem; }
	.actions { display: flex; flex-wrap: wrap; gap: 8px; }
	.count { color: var(--text-muted); font: var(--t-subhead); font-variant-numeric: tabular-nums; }
	.table-wrap { overflow-x: auto; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: var(--shadow-card); }
	.copies { width: 100%; border-collapse: collapse; font: var(--t-callout); font-variant-numeric: tabular-nums; }
	.copies th { position: sticky; top: 0; padding: 10px 12px; text-align: left; background: var(--surface-1); color: var(--text-muted); font: var(--t-footnote); border-bottom: 1px solid var(--line); }
	.copies input[type='checkbox'] { width: 20px; height: 20px; accent-color: var(--accent); }
	/* On touch phones the label around the box is the 44px tap target; the drawn box and the row stay the same. */
	.pick label { display: inline-flex; }
	/* The title link keeps its place in the line; on touch phones its box is 44px tall (inline padding moves nothing). */
	@media (max-width: 767px) and (pointer: coarse) { td > a { padding-block: 13px; } }
	@media (max-width: 767px) and (pointer: coarse) { .pick label { align-items: center; justify-content: center; min-width: 44px; min-height: 44px; margin: -12px; } }
	.copies a { color: var(--text); }
	.copies td { padding: var(--s-3); vertical-align: top; }
	.copies tbody tr + tr { border-top: 1px solid var(--line); }
	.copies tbody tr:hover { background: var(--surface-2); }
	.path { max-width: 24rem; overflow-wrap: anywhere; color: var(--text-muted); font: var(--t-footnote); }
	.codec { font-weight: 700; }
	.block { display: block; }
	.pager { display: flex; justify-content: space-between; align-items: center; gap: var(--s-4); margin-top: var(--s-4); }
	@media (max-width: 700px) {
		.tools, .tools label { width: 100%; }
		.tools .input, .tools :global(.ui-btn) { width: 100%; }
		.selection-bar { align-items: stretch; flex-direction: column; }
		.selection-bar { padding: 12px; }

		.copies th:nth-child(4), .copies td:nth-child(4), .copies th:nth-child(5), .copies td:nth-child(5) { display: none; }
	}
</style>
