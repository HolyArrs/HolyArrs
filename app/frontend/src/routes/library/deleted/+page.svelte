<script lang="ts">
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import Poster from '#lib/ui/Poster.svelte';
	import Button from '#lib/ui/Button.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { titleArt, LibraryToolPage, daysLeft, daysLeftWords, heldFileName as fileName, type HeldFile, type RestoreResult } from '#lib/mediafiles/index.ts';
	import { onMount } from 'svelte';
	import { to } from '#lib/core/paths.ts';
	import { api, message } from '#lib/core/api.ts';
	import { bytes, date } from '#lib/core/format.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { plainReason } from '#lib/core/plain.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	// Recently deleted: files waiting in the holding folder. Pick some and Restore puts them back where they were; a deleted title comes back too.

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
	// Posters for the rows, from one brief read of the Library.
	let art = $state<Map<number, string>>(new Map());
	onMount(() => {
		void load();
		void titleArt().then((m) => (art = m));
	});

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
			if (ok) toasts.push(t(ok === 1 ? 'library.deleted.restoredOne' : 'library.deleted.restoredMany', { n: ok }), 'ok');
			if (bad) toasts.push(t('library.deleted.restoreNamedFailed', { name: fileName(bad.path ?? '') || t('library.deleted.aFile') }), 'fail', { body: bad.result });
			picked = new Set();
			await load();
		} catch (e) {
			toasts.push(t('mediafiles.delete.restoreFailed'), 'fail', { body: message(e) });
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>{t('library.deleted.title')} · HolyArrs</title></svelte:head>

<LibraryToolPage title={t('library.deleted.title')} count={loaded && !error && files.length ? t('mediafiles.unit.file.' + (files.length === 1 ? 'one' : 'other'), { n: files.length }) : ''}>
	{#snippet bar()}
		<span class="tp-gap"></span>
		<span class="tp-cap">
			<Button variant="primary" icon="refresh" onclick={restore} disabled={!picked.size} {busy} data-testid="restore-deleted">
				{t('library.deleted.restore')}{picked.size ? ` ${picked.size}` : ''}
			</Button>
		</span>
	{/snippet}

{#if error}
	<LoadError hasData={false} message={plainReason(error).text} onretry={load} />
{:else}
<PageLoader loading={!loaded} label={t('library.deleted.loading')}>
{#if files.length === 0}
	<EmptyState icon="trash" title={t('library.deleted.nothing')} />
{:else}
	<div class="tp-sheet">
		<label class="all">
			<input type="checkbox" checked={all} onchange={() => (picked = all ? new Set() : new Set(files.map((f) => f.id)))} data-testid="deleted-pick-all" />
			{t('library.deleted.selectAll')}
		</label>
		<ul class="tp-rows">
			{#each files as f (f.id)}
				{@const left = daysLeft(f.expires_at)}
				<li class="row-item" class:picked={picked.has(f.id)} data-testid="deleted-file">
					<label class="pick"><input type="checkbox" checked={picked.has(f.id)} onchange={() => toggle(f.id)} aria-label={t('mediafiles.list.select', { name: fileName(f.path) })} /></label>
					<span class="tp-art" style="--poster-ratio: 2 / 3"><Poster src={art.get(f.title_id)} name={f.title} /></span>
					<div class="grow">
						{#if f.title_deleted}
							<span class="name">{f.title}</span>
						{:else}
							<a class="name" href={to(`/library/${f.title_id}`)}>{f.title}</a>
						{/if}
						<span class="tp-sub path" title={f.path}>{fileName(f.path)}</span>
						<span class="tp-sub">{bytes(f.size)} · {t('library.deleted.deletedOn', { date: date(f.deleted_at) })}{f.title_deleted ? ` ${t('library.deleted.withTitle')}` : ''}</span>
					</div>
					<span class="left" class:soon={left !== null && left <= 1} data-testid="deleted-days-left">{daysLeftWords(f.expires_at)}</span>
				</li>
			{/each}
		</ul>
	</div>
{/if}
</PageLoader>
{/if}
</LibraryToolPage>

<style>
	.all { display: flex; align-items: center; gap: 10px; min-height: 44px; padding: 0 20px; color: var(--text-muted); font: 600 11px/1 var(--font-ui); letter-spacing: .06em; text-transform: uppercase; background: color-mix(in srgb, var(--text) 3%, transparent); border-bottom: 1px solid var(--line); cursor: pointer; }
	input[type='checkbox'] { width: 20px; height: 20px; margin: 0; accent-color: var(--accent); }
	.row-item { display: flex; align-items: center; gap: 12px; }
	.row-item.picked { background: color-mix(in srgb, var(--accent) 8%, transparent); }
	.pick { display: inline-flex; align-items: center; justify-content: center; flex: none; min-width: 44px; min-height: 44px; margin: -12px; cursor: pointer; }
	.name { color: var(--text); font-weight: 600; text-decoration: none; overflow-wrap: anywhere; }
	a.name:hover { text-decoration: underline; text-underline-offset: 3px; }
	.grow { flex: 1; min-width: 0; font: var(--t-callout); font-variant-numeric: tabular-nums; }
	.path { overflow-wrap: anywhere; }
	.left { flex: none; font: var(--t-footnote); color: var(--ok-text); font-variant-numeric: tabular-nums; }
	.left.soon { color: var(--fail-text); }
	@media (max-width: 639px) {
		.all { padding: 0 16px; }
		.row-item { flex-wrap: wrap; align-items: flex-start; }
		.left { flex-basis: 100%; padding-left: 44px; }
	}
	@media (pointer: coarse) { a.name { display: inline-block; padding-block: 12px; margin-block: -12px; } }
</style>
