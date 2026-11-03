<script lang="ts">
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import { to } from '#lib/paths.ts';
	// Recently deleted: files waiting in the holding folder. Pick some and
	// Restore puts them back where they were; a deleted title comes back too.
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { bytes, date } from '#lib/format.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { plainReason } from '#lib/plain.ts';
	import { daysLeft, daysLeftWords, fileName, type HeldFile, type RestoreResult } from '#lib/holding.ts';

	let files = $state<HeldFile[]>([]);
	let loaded = $state(false);
	let error = $state('');
	let picked = $state<Set<number>>(new Set());
	let busy = $state(false);
	const all = $derived(files.length > 0 && picked.size === files.length);

	async function load() {
		try {
			const r = await api.get<{ files: HeldFile[] }>('/holding');
			files = r.files;
			picked = new Set([...picked].filter((id) => files.some((f) => f.id === id)));
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}
	onMount(() => void load());

	function toggle(id: number) {
		const next = new Set(picked);
		if (!next.delete(id)) next.add(id);
		picked = next;
	}

	async function restore() {
		if (!picked.size || busy) return;
		busy = true;
		try {
			const r = await api.post<{ results: RestoreResult[] }>('/holding/restore', { ids: [...picked] });
			const ok = r.results.filter((x) => x.result === 'restored').length;
			const bad = r.results.find((x) => x.result !== 'restored');
			if (ok) toasts.push(ok === 1 ? 'Restored 1 file.' : `Restored ${ok} files.`, 'ok');
			if (bad) toasts.push(`${fileName(bad.path ?? '') || 'A file'}: ${bad.result}`, 'fail');
			picked = new Set();
			await load();
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Recently deleted · HolyArrs</title></svelte:head>

<div class="page">
<PageHeader title="Recently deleted" back={{ href: to('/library'), label: 'Library' }}>
	{#snippet actions()}
		<Button variant="primary" icon="refresh" onclick={restore} disabled={!picked.size} {busy} data-testid="restore-deleted">
			Restore{picked.size ? ` ${picked.size}` : ''}
		</Button>
	{/snippet}
</PageHeader>

{#if error}
	<LoadError hasData={false} message={plainReason(error).text} onretry={load} />
{:else}
<PageLoader loading={!loaded} label="Loading deleted files">
{#if files.length === 0}
	<EmptyState icon="trash" title="Nothing deleted" text="Files you delete wait here for a while, so you can put them back." />
{:else}
	<label class="all small muted">
		<input type="checkbox" checked={all} onchange={() => (picked = all ? new Set() : new Set(files.map((f) => f.id)))} data-testid="deleted-pick-all" />
		{files.length} {files.length === 1 ? 'file' : 'files'}
	</label>
	<ul class="rows-list">
		{#each files as f (f.id)}
			{@const left = daysLeft(f.expires_at)}
			<li class="row-item" data-testid="deleted-file">
				<input type="checkbox" checked={picked.has(f.id)} onchange={() => toggle(f.id)} aria-label="Select {fileName(f.path)}" />
				<div class="grow">
					{#if f.title_deleted}
						<span class="name">{f.title}</span>
					{:else}
						<a class="name" href={to(`/library/${f.title_id}`)}>{f.title}</a>
					{/if}
					<div class="sub path">{f.path}</div>
					<div class="sub muted">
						{bytes(f.size)} · deleted {date(f.deleted_at)}{f.title_deleted ? ' with the title' : ''}
					</div>
				</div>
				<span class="left" class:soon={left !== null && left <= 1} data-testid="deleted-days-left">{daysLeftWords(f.expires_at)}</span>
			</li>
		{/each}
	</ul>
{/if}
</PageLoader>
{/if}
</div>

<style>
	.page { display: flex; flex-direction: column; gap: 16px; padding-bottom: 48px; min-width: 0; }
	.name { font: var(--t-headline); color: var(--text); overflow-wrap: anywhere; }
	.sub { margin-top: 2px; font: var(--t-subhead); font-weight: 400; font-variant-numeric: tabular-nums; }
	.path { overflow-wrap: anywhere; }
	.muted { color: var(--text-muted); }
	.all { display: flex; align-items: center; gap: 8px; margin: 0 0 -8px; padding: 0 16px; font: var(--t-subhead); }
	.rows-list {
		margin: 0;
		padding: 4px;
		list-style: none;
		border-radius: var(--r-panel);
		background: var(--surface-1);
		box-shadow: var(--shadow-card);
	}
	.row-item {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		padding: 10px 12px;
	}
	.row-item + .row-item { border-top: 1px solid var(--line); }
	.grow { flex: 1; min-width: 0; }
	.left { flex: none; font: var(--t-subhead); color: var(--ok-text); font-variant-numeric: tabular-nums; }
	.left.soon { color: var(--fail-text); }
	@media (max-width: 600px) {
		.row-item { flex-wrap: wrap; align-items: flex-start; }
		.left { flex-basis: 100%; padding-left: 28px; }
	}
</style>
