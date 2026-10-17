<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, message } from '#lib/api.ts';
	import { to } from '#lib/paths.ts';
	import { session } from '#lib/session.svelte.ts';
	import { collectionBody, collectionResultLine, collectionStart, type CollectionChoice, type CollectionDetail, type CollectionReview } from '#lib/collections.ts';

	const id = Number(page.params.id);
	let detail = $state<CollectionDetail | null>(null);
	let selected = $state<number[]>([]);
	let choice = $state<CollectionChoice>(collectionStart());
	let review = $state<CollectionReview | null>(null);
	let busy = $state(false);
	let error = $state('');

	onMount(load);
	async function load() {
		try {
			detail = await api.get<CollectionDetail>(`/collections/${id}`);
			selected = detail.titles.map((t) => t.id);
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
		catch (e) { error = message(e); review = null; await load(); }
		finally { busy = false; }
	}
	function fileChoiceChanged() {
		if (!choice.delete_files) choice.acknowledge_protection = false;
		review = null;
	}
	async function retry() {
		review = null;
		await load();
	}
</script>

<svelte:head><title>{detail?.name ?? 'Movie collection'} · HolyArrs</title></svelte:head>
<a class="back small muted" href={to('/library/collections')}>Movie series</a>
<section class="workspace">

{#if error}<div class="notice bad" role="alert">{error}</div>{/if}
{#if detail}
	<div class="page-head"><div><p class="eyebrow">Movie collection</p><h1>{detail.name}</h1><p class="muted">Removing a movie removes its Library entry even when it belongs to another collection.</p></div></div>
	<div class="stack">
		{#each detail.titles as title (title.id)}
			<label class="panel title">
				<input type="checkbox" checked={selected.includes(title.id)} onchange={() => toggle(title.id)} disabled={!!review?.applied} />
				<span class="grow"><b>{title.name}</b> <span class="muted">{title.year ?? ''}</span><small class="muted">{title.version_ids.length} Version{title.version_ids.length === 1 ? '' : 's'} · {title.files.length} File{title.files.length === 1 ? '' : 's'}</small></span>
				{#if title.protected}<span class="chip warn">Protected copy</span>{/if}
			</label>
		{/each}
	</div>

	{#if session.can('manage_library') && !review?.applied}
		<section class="panel choices" aria-labelledby="remove-heading">
			<h2 id="remove-heading">Review removal</h2>
			<label><input type="checkbox" bind:checked={choice.delete_files} onchange={fileChoiceChanged} /> Move HolyArrs-managed files to the holding folder</label>
			<label><input type="checkbox" bind:checked={choice.never_add} onchange={() => review = null} /> Put removed movies on Never-add</label>
			{#if choice.delete_files && detail.titles.some((t) => selected.includes(t.id) && t.protected)}
				<label class="danger"><input type="checkbox" bind:checked={choice.acknowledge_protection} onchange={() => review = null} /> I understand protected copies in the selected titles will also be removed</label>
			{/if}
			{#if !choice.never_add}<p class="muted small">An outside list may add these movies again later.</p>{/if}
			{#if !review}<button class="btn btn-red" onclick={preview} disabled={busy || selected.length === 0}>Review {selected.length} selected</button>{/if}
		</section>
	{/if}

	{#if review && !review.applied}
		<section class="panel stack" aria-labelledby="confirm-heading">
			<h2 id="confirm-heading">Confirm this exact selection</h2>
			{#each review.titles.filter((t) => t.selected) as title (title.id)}
				<div><b>{title.name}</b> <span class="muted">· Versions {title.version_ids.join(', ') || 'none'} · Files {title.files.map((f) => f.id).join(', ') || 'none'}</span>{#if title.skip}<p class="warn-text small">{title.skip}</p>{/if}</div>
			{/each}
			<div class="row"><button class="btn btn-red" onclick={apply} disabled={busy}>Remove reviewed movies</button><button class="btn btn-ghost" onclick={() => review = null} disabled={busy}>Cancel</button></div>
		</section>
	{/if}

	{#if review?.applied}
		<section class="panel stack" aria-live="polite"><h2>Movie series removal finished</h2>{#each review.results ?? [] as result}<p class:bad-text={result.status === 'failed'}>{collectionResultLine(result)}</p>{/each}<div class="row"><button class="btn btn-ghost" onclick={retry}>Review remaining movies</button><a class="btn btn-ghost" href={to('/library/collections')}>Back to movie series</a></div></section>
	{/if}
{/if}
</section>

<style>
	.workspace { overflow: hidden; border: 1px solid var(--line); border-radius: 16px; background: var(--surface-1); }
	.workspace .page-head { margin: 0; padding: var(--s-5); border-bottom: 1px solid var(--line); }
	.workspace > .stack { gap: 0; }
	.title { display: flex; align-items: center; gap: .8rem; min-height: 72px; padding: var(--s-4) var(--s-5); border: 0; border-bottom: 1px solid var(--line); border-radius: 0; box-shadow: none; }
	.title small { display: block; margin-top: .2rem; }
	.choices { display: grid; gap: var(--s-2); margin: 0; padding: var(--s-5); border: 0; border-top: 1px solid var(--line); border-radius: 0; box-shadow: none; }
	.choices label { display: flex; align-items: flex-start; gap: var(--s-3); min-height: var(--tap); padding: var(--s-3); border: 1px solid var(--line); border-radius: 10px; background: var(--surface-1); }
	.choices input { width: 20px; height: 20px; flex: none; accent-color: var(--ha-red); }
	.workspace > .panel.stack { margin: 0; padding: var(--s-5); border: 0; border-top: 1px solid var(--line); border-radius: 0; box-shadow: none; }
	.workspace > :global(.notice) { margin: var(--s-4); }
	.danger, .warn-text { color: var(--wait); }
	.bad-text { color: var(--fail); }
</style>
