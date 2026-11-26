<script lang="ts">
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import Poster from '#lib/ui/Poster.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import { titleArt, LibraryToolPage, daysLeft, daysLeftWords, heldFileName as fileName, heldOfReason, type HeldFile, type RestoreResult, type RestoreHow } from '#lib/mediafiles/index.ts';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { to } from '#lib/core/paths.ts';
	import { api, message } from '#lib/core/api.ts';
	import { bytes, date } from '#lib/core/format.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { plainReason } from '#lib/core/plain.ts';
	import { appLocale, t } from '#lib/core/i18n/i18n.svelte.ts';

	// Recently deleted: files deleted by hand and copies an upgrade replaced, waiting in the holding folder. Pick some and Restore puts them back; when another file sits in a copy's place, the person chooses: swap back, keep both, or cancel. Nothing is ever overwritten.

	type Reason = 'all' | 'deleted' | 'replaced';
	let files = $state<HeldFile[]>([]);
	let loaded = $state(false);
	let error = $state('');
	let picked = $state<Set<number>>(new Set());
	let busy = $state(false);
	let reason = $state<Reason>('all');
	let keepDays = $state<number | null>(null);
	// ?title=<id> shows one title's files (from its title page or History).
	const titleId = $derived(Number(page.url.searchParams.get('title')) || 0);
	const titleName = $derived(files.find((f) => f.title_id === titleId)?.title ?? '');
	const shown = $derived(heldOfReason(files, reason));
	const all = $derived(shown.length > 0 && shown.every((f) => picked.has(f.id)));
	const replacedN = $derived(files.filter((f) => f.replaced).length);

	async function load() {
		try {
			const r = await api.get<{ files: HeldFile[] }>(titleId ? `/holding?title_id=${titleId}` : '/holding');
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
		void titleArt().then((m) => (art = m));
		void api.get<{ holding_days: number }>('/settings/files').then((r) => (keepDays = r.holding_days), () => {});
	});
	$effect(() => {
		void titleId;
		void load();
	});

	function toggle(id: number) {
		const next = new Set(picked);
		if (!next.delete(id)) next.add(id);
		picked = next;
	}

	// Files whose place was taken wait here, one question at a time.
	let taken = $state<{ id: number; name: string; canSwap: boolean }[]>([]);
	let askOpen = $state(false);
	const asking = $derived(taken[0]);

	async function send(ids: number[], how: RestoreHow) {
		const r = await api.post<{ results: RestoreResult[] }>('/holding/restore', { ids, how });
		const ok = r.results.filter((x) => x.result === 'restored');
		const bad = r.results.find((x) => x.result !== 'restored' && !x.taken);
		if (ok.length) {
			const one = ok.length === 1 ? fileName(ok[0].path ?? '') : '';
			toasts.push(how === 'swap' && one ? t('library.deleted.swappedBack', { name: one }) : t(ok.length === 1 ? 'library.deleted.restoredOne' : 'library.deleted.restoredMany', { n: ok.length }), 'ok');
		}
		if (bad) toasts.push(t('library.deleted.restoreNamedFailed', { name: fileName(bad.path ?? '') || t('library.deleted.aFile') }), 'fail', { body: bad.result });
		return r.results.filter((x) => x.taken).map((x) => ({ id: x.id, name: fileName(x.path ?? ''), canSwap: !!x.can_swap }));
	}

	async function restore() {
		if (!picked.size || busy) return;
		busy = true;
		try {
			taken = await send([...picked], '');
			askOpen = taken.length > 0;
			picked = new Set();
			await load();
		} catch (e) {
			toasts.push(t('mediafiles.delete.restoreFailed'), 'fail', { body: message(e) });
		} finally {
			busy = false;
		}
	}

	async function answer(how: RestoreHow | 'cancel') {
		const one = asking;
		if (!one || busy) return;
		if (how !== 'cancel') {
			busy = true;
			try {
				await send([one.id], how);
				await load();
			} catch (e) {
				toasts.push(t('mediafiles.delete.restoreFailed'), 'fail', { body: message(e) });
			} finally {
				busy = false;
			}
		}
		taken = taken.slice(1);
		askOpen = taken.length > 0;
	}
	$effect(() => {
		if (!askOpen && taken.length) taken = [];
	});
</script>

<svelte:head><title>{t('library.deleted.title')} · HolyArrs</title></svelte:head>

<LibraryToolPage title={t('library.deleted.title')} count={loaded && !error && files.length ? t('mediafiles.unit.file.' + (files.length === 1 ? 'one' : 'other'), { n: files.length.toLocaleString(appLocale()) }) : ''}>
	{#snippet bar()}
		{#if replacedN && replacedN < files.length}
			<SegmentedControl size="sm" label={t('library.deleted.show')} bind:value={reason} options={[{ value: 'all', label: t('library.deleted.all') }, { value: 'deleted', label: t('library.deleted.tagDeleted') }, { value: 'replaced', label: t('library.deleted.tagReplaced') }]} testid="deleted-reason" />
		{/if}
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
	<div class="note">
		<p data-testid="deleted-keep-note">{keepDays === 0 ? t('library.deleted.keptUntilEmptied') : keepDays ? t(keepDays === 1 ? 'library.deleted.keptOneDay' : 'library.deleted.keptDays', { n: keepDays }) : t('library.deleted.keptIntro')}
			<a href={to('/settings/naming#holding-days')}>{t('library.deleted.change')}</a></p>
		{#if titleId}<Chip label={titleName ? t('library.deleted.onlyTitle', { title: titleName }) : t('library.deleted.oneTitle')} selected onclick={() => goto(to('/library/deleted'))} onremove={() => goto(to('/library/deleted'))} />{/if}
	</div>
{#if shown.length === 0}
	<EmptyState icon="trash" title={t('library.deleted.nothing')} />
{:else}
	<div class="tp-sheet">
		<label class="all">
			<input type="checkbox" checked={all} onchange={() => (picked = all ? new Set() : new Set(shown.map((f) => f.id)))} data-testid="deleted-pick-all" />
			{t('library.deleted.selectAll')}
		</label>
		<ul class="tp-rows">
			{#each shown as f (f.id)}
				{@const left = daysLeft(f.expires_at)}
				<li class="row-item" class:picked={picked.has(f.id)} data-testid="deleted-file">
					<label class="pick"><input type="checkbox" checked={picked.has(f.id)} onchange={() => toggle(f.id)} aria-label={t('mediafiles.list.select', { name: fileName(f.path) })} /></label>
					<span class="tp-art" style="--poster-ratio: 2 / 3"><Poster src={art.get(f.title_id)} name={f.title} /></span>
					<div class="grow">
						<span class="top">
							{#if f.title_deleted}
								<span class="name">{f.title}</span>
							{:else}
								<a class="name" href={to(`/library/${f.title_id}`)}>{f.title}</a>
							{/if}
							<span class="tag" class:rep={f.replaced} data-testid="deleted-reason-tag">{f.replaced ? t('library.deleted.tagReplaced') : t('library.deleted.tagDeleted')}</span>
						</span>
						<span class="tp-sub path" title={f.path}>{fileName(f.path)}</span>
						<span class="tp-sub">{bytes(f.size)} · {f.replaced ? (f.replaced_by ? t('library.deleted.replacedBy', { date: date(f.deleted_at), name: fileName(f.replaced_by) }) : t('library.deleted.replacedOn', { date: date(f.deleted_at) })) : t('library.deleted.deletedOn', { date: date(f.deleted_at) })}{f.title_deleted ? ` ${t('library.deleted.withTitle')}` : ''}</span>
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

<Sheet bind:open={askOpen} title={t('library.deleted.takenTitle')} size="sm">
	<div class="ask" data-testid="restore-taken">
		<p class="ask-why">{t('library.deleted.takenBody', { name: asking?.name ?? '' })}</p>
		<div class="ask-rows">
			{#if asking?.canSwap}
				<button type="button" class="ask-row" disabled={busy} onclick={() => answer('swap')} data-testid="restore-swap">
					<b>{t('library.deleted.swap')}</b><small>{t('library.deleted.swapHint')}</small>
				</button>
			{/if}
			<button type="button" class="ask-row" disabled={busy} onclick={() => answer('both')} data-testid="restore-both">
				<b>{t('library.deleted.keepBoth')}</b><small>{t('library.deleted.keepBothHint')}</small>
			</button>
		</div>
		<div class="ask-actions"><Button variant="plain" onclick={() => answer('cancel')} data-testid="restore-cancel">{t('library.deleted.cancel')}</Button></div>
	</div>
</Sheet>

<style>
	.note { display: flex; align-items: center; flex-wrap: wrap; gap: 8px 12px; margin: 0 0 12px; color: var(--text-muted); font: var(--t-footnote); }
	.note p { margin: 0; flex: 1; min-width: 0; }
	.note a { color: var(--accent-text); font-weight: 600; text-decoration: none; }
	.note a:hover { text-decoration: underline; text-underline-offset: 3px; }
	.all { display: flex; align-items: center; gap: 10px; min-height: 44px; padding: 0 20px; color: var(--text-muted); font: 600 11px/1 var(--font-ui); letter-spacing: .06em; text-transform: uppercase; background: color-mix(in srgb, var(--text) 3%, transparent); border-bottom: 1px solid var(--line); cursor: pointer; }
	input[type='checkbox'] { width: 20px; height: 20px; margin: 0; accent-color: var(--accent); }
	.row-item { display: flex; align-items: center; gap: 12px; }
	.row-item.picked { background: color-mix(in srgb, var(--accent) 8%, transparent); }
	.pick { display: inline-flex; align-items: center; justify-content: center; flex: none; min-width: 44px; min-height: 44px; margin: -12px; cursor: pointer; }
	.top { display: flex; align-items: center; flex-wrap: wrap; gap: 4px 8px; }
	.name { color: var(--text); font-weight: 600; text-decoration: none; overflow-wrap: anywhere; }
	a.name:hover { text-decoration: underline; text-underline-offset: 3px; }
	.tag { padding: 3px 7px; border-radius: 6px; background: color-mix(in srgb, var(--text) 8%, transparent); color: var(--text-muted); font: 600 11px/1 var(--font-ui); }
	.tag.rep { background: color-mix(in srgb, var(--accent) 14%, transparent); color: var(--accent-text); }
	.grow { flex: 1; min-width: 0; font: var(--t-callout); font-variant-numeric: tabular-nums; }
	.path { overflow-wrap: anywhere; }
	.left { flex: none; font: var(--t-footnote); color: var(--ok-text); font-variant-numeric: tabular-nums; }
	.left.soon { color: var(--fail-text); }
	.ask { display: grid; gap: 16px; }
	.ask-why { margin: 0; color: var(--text-muted); }
	.ask-rows { display: grid; border-radius: 14px; background: var(--surface-2); overflow: hidden; }
	.ask-row { display: grid; gap: 3px; padding: 14px 16px; border: 0; background: none; color: var(--text); text-align: start; cursor: pointer; font: inherit; }
	.ask-row:hover { background: color-mix(in srgb, var(--text) 4%, transparent); }
	.ask-row:focus-visible { outline: none; box-shadow: inset 0 0 0 2px var(--accent); }
	.ask-row + .ask-row { box-shadow: inset 0 .5px 0 color-mix(in srgb, var(--text) 10%, transparent); }
	.ask-row b { font: 600 15px/1.2 var(--font-ui); }
	.ask-row small { color: var(--text-muted); font: var(--t-footnote); }
	.ask-actions { display: flex; justify-content: flex-end; }
	@media (max-width: 639px) {
		.all { padding: 0 16px; }
		.row-item { flex-wrap: wrap; align-items: flex-start; }
		.left { flex-basis: 100%; padding-left: 44px; }
	}
	@media (pointer: coarse) { a.name { display: inline-block; padding-block: 12px; margin-block: -12px; } }
</style>
