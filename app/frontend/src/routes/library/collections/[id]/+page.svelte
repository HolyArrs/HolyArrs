<script lang="ts">
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import StatusPill from '#lib/ui/StatusPill.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { collectionBody, collectionResultLine, collectionStart, type CollectionChoice, type CollectionDetail, type CollectionReview } from '#lib/mediafiles/index.ts';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, message } from '#lib/core/api.ts';
	import { to } from '#lib/core/paths.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { plainReason } from '#lib/core/plain.ts';
	import { t as tr } from '#lib/core/i18n/i18n.svelte.ts';

	const id = Number(page.params.id);
	let detail = $state<CollectionDetail | null>(null);
	let selected = $state<number[]>([]);
	let choice = $state<CollectionChoice>(collectionStart());
	let review = $state<CollectionReview | null>(null);
	let busy = $state(false);
	let error = $state('');

	onMount(load);
	async function load(preserveError = false) {
		try {
			detail = await api.get<CollectionDetail>(`/collections/${id}`);
			selected = detail.titles.map((t) => t.id);
			if (!preserveError) error = '';
		} catch (e) { error = message(e); }
	}
	function toggle(titleId: number) {
		selected = selected.includes(titleId) ? selected.filter((x) => x !== titleId) : [...selected, titleId];
		review = null;
	}
	async function preview() {
		busy = true; error = '';
		try { review = await api.post<CollectionReview>(`/collections/${id}/remove/preview`, collectionBody(selected, choice)); }
		catch (e) { error = message(e); }
		finally { busy = false; }
	}
	async function apply() {
		if (!review) return;
		busy = true; error = '';
		try { review = await api.post<CollectionReview>(`/collections/${id}/remove`, collectionBody(selected, choice, review.review_token)); }
		catch (e) { error = message(e); review = null; await load(true); }
		finally { busy = false; }
	}
	function fileChoiceChanged() {
		if (!choice.delete_files) choice.acknowledge_protection = false;
		review = null;
	}
	async function retry() {
		review = null;
		busy = true;
		try { await load(); }
		finally { busy = false; }
	}
</script>

<svelte:head><title>{detail?.name ?? tr('library.collection.movieCollection')} · HolyArrs</title></svelte:head>
<div class="page">
<PageHeader title={detail?.name ?? tr('library.collection.collection')} subtitle={detail ? tr('library.collection.alsoRemoves') : undefined} back={{ href: to('/library/collections'), label: tr('library.collection.collection') }} />

{#if error && !detail}
	<LoadError hasData={false} message={plainReason(error).text} onretry={retry} {busy} />
{:else if error}
	<p class="alert" role="alert"><Icon name="alert" size={17} /><span>{plainReason(error).text}</span></p>
{/if}
<PageLoader loading={!detail && !error} label={tr('library.collection.loading')}>
{#if detail}
	<ul class="card titles">
		{#each detail.titles as title (title.id)}
			<li>
				<label class="title">
					<input type="checkbox" checked={selected.includes(title.id)} onchange={() => toggle(title.id)} disabled={!!review?.applied} />
					<span class="grow"><span class="name">{title.name}</span>{#if title.year}<span class="muted">{` ${title.year}`}</span>{/if}<small>{tr('library.collection.counts', { versions: tr(title.version_ids.length === 1 ? 'library.collection.versionOne' : 'library.collection.versionMany', { n: title.version_ids.length }), files: tr(title.files.length === 1 ? 'library.collection.fileOne' : 'library.collection.fileMany', { n: title.files.length }) })}</small></span>
					{#if title.protected}<StatusPill kind="missing" label={tr('library.collection.protectedCopy')} />{/if}
				</label>
			</li>
		{/each}
	</ul>

	{#if session.can('manage_library') && !review?.applied}
		<section class="card choices" aria-labelledby="remove-heading">
			<h2 id="remove-heading">{tr('library.collection.review')}</h2>
			<label><input type="checkbox" bind:checked={choice.delete_files} onchange={fileChoiceChanged} /> {tr('library.collection.moveFiles')}</label>
			<label><input type="checkbox" bind:checked={choice.never_add} onchange={() => review = null} /> {tr('library.collection.neverAdd')}</label>
			{#if choice.delete_files && detail.titles.some((t) => selected.includes(t.id) && t.protected)}
				<label class="danger"><input type="checkbox" bind:checked={choice.acknowledge_protection} onchange={() => review = null} /> {tr('library.collection.understand')}</label>
			{/if}
			{#if !choice.never_add}<p class="note">{tr('library.collection.outsideList')}</p>{/if}
			{#if !review}<div><Button variant="primary" onclick={preview} disabled={selected.length === 0} {busy}>{tr('library.collection.reviewN', { n: selected.length })}</Button></div>{/if}
		</section>
	{/if}

	{#if review && !review.applied}
		<section class="card choices" aria-labelledby="confirm-heading">
			<h2 id="confirm-heading">{tr('library.collection.confirm')}</h2>
			{#each review.titles.filter((t) => t.selected) as title (title.id)}
				<div class="confirm-row">
					<span class="name">{title.name}</span>
					<span class="muted"> · {tr('library.collection.counts', { versions: tr(title.version_ids.length === 1 ? 'library.collection.versionOne' : 'library.collection.versionMany', { n: title.version_ids.length }), files: tr(title.files.length === 1 ? 'library.collection.fileOne' : 'library.collection.fileMany', { n: title.files.length }) })}</span>
					{#if title.skip}<p class="warn-text">{title.skip}</p>{/if}
					{#if session.can('settings')}<details class="details"><summary>{tr('library.collection.details')}</summary>{tr('library.collection.ids', { versions: title.version_ids.join(', ') || tr('library.collection.none'), files: title.files.map((f) => f.id).join(', ') || tr('library.collection.none') })}</details>{/if}
				</div>
			{/each}
			<div class="actions"><Button variant="destructive" onclick={apply} {busy}>{tr('library.collection.removeReviewed')}</Button><Button variant="plain" onclick={() => review = null} disabled={busy}>{tr('mediafiles.dialogs.cancel')}</Button></div>
		</section>
	{/if}

	{#if review?.applied}
		<section class="card choices" aria-live="polite"><h2>{tr('library.collection.removed')}</h2>{#each review.results ?? [] as result}<p class:bad-text={result.status === 'failed'}>{collectionResultLine(result)}</p>{/each}<div class="actions"><Button onclick={retry}>{tr('library.collection.reviewRemaining')}</Button><Button variant="plain" href={to('/library/collections')}>{tr('library.collection.back')}</Button></div></section>
	{/if}
{/if}
</PageLoader>
</div>

<style>
	.page { display: flex; flex-direction: column; gap: 16px; padding-bottom: 48px; min-width: 0; }
	.card { margin: 0; padding: 4px; list-style: none; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: var(--shadow-card); }
	.titles > li + li { border-top: 1px solid var(--line); }
	.title { display: flex; align-items: center; gap: 12px; min-height: 56px; padding: 8px 12px; border-radius: var(--r-btn); cursor: pointer; }
	.title:hover { background: var(--surface-2); }
	.title small { display: block; margin-top: 2px; color: var(--text-muted); font: var(--t-subhead); font-weight: 400; }
	.name { font: var(--t-headline); }
	.muted { color: var(--text-muted); }
	.grow { flex: 1; min-width: 0; }
	input[type='checkbox'] { width: 20px; height: 20px; flex: none; accent-color: var(--accent); }
	.choices { display: grid; gap: 8px; padding: 16px 20px 20px; }
	.choices h2 { margin: 0 0 4px; font: var(--t-title3); }
	.choices label { display: flex; align-items: flex-start; gap: 12px; min-height: 44px; padding: 10px 12px; border-radius: var(--r-btn); background: var(--surface-2); font: var(--t-callout); }
	.choices p { margin: 0; font: var(--t-callout); }
	.note { color: var(--text-muted); }
	.confirm-row { padding: 8px 0; border-bottom: 1px solid var(--line); font: var(--t-callout); }
	.details { margin-top: 4px; color: var(--text-muted); font: var(--t-footnote); }
	.details summary { cursor: pointer; min-height: 32px; display: inline-flex; align-items: center; }
	.actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 8px; }
	.alert { display: flex; align-items: center; gap: 8px; margin: 0; padding: 10px 12px; border-radius: var(--r-btn); background: var(--surface-2); color: var(--fail); font: var(--t-callout); }
	.danger, .warn-text { color: var(--wait); }
	.bad-text { color: var(--fail); }
</style>
