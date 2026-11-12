<!-- Confirm sheet for deleting a title, with a dry-run preview of what will happen. -->
<script lang="ts">
	import { untrack } from 'svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import { deleteSummary, type RemovePlan } from './organise.ts';
	import { api, message, ApiError } from '#lib/core/api.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	let { open = $bindable(false), titleId, name, kind, ondone }: { open?: boolean; titleId: number; name: string; kind: 'movie' | 'show' | 'artist'; ondone?: (p: RemovePlan) => void } = $props();

	let files = $state(false);
	let never = $state(false);
	let plan = $state<RemovePlan | null>(null);
	let error = $state('');
	let busy = $state(false);
	// Counts previews so a slow older answer cannot overwrite a newer one.
	let ask = 0;

	const kindWord = $derived(t(`mediafiles.kind.${kind}`));

	// Each time the sheet opens it starts clean; untrack so edits inside do not rerun this.
	$effect(() => {
		if (!open) return;
		untrack(() => {
			files = false;
			never = false;
			plan = null;
			void planIt();
		});
	});

	async function planIt() {
		const n = ++ask;
		error = '';
		try {
			const p = await api.post<RemovePlan>(`/titles/${titleId}/delete`, { delete_files: files, never_add: never, apply: false });
			if (n === ask) plan = p;
		} catch (e) {
			if (n === ask) error = message(e);
		}
	}

	// Held files get an Undo that restores them from the holding folder.
	async function run() {
		busy = true;
		error = '';
		try {
			const p = await api.post<RemovePlan>(`/titles/${titleId}/delete`, { delete_files: files, never_add: never, apply: true });
			const held = p.held.map((h) => h.file_id);
			const kept = p.held.length ? (p.holding_days ? t('mediafiles.delete.keptDays', { n: p.holding_days }) : t('mediafiles.delete.keptFolder')) : t('mediafiles.delete.stay');
			toasts.push(t('mediafiles.delete.deleted', { name }), 'ok',
				held.length ? { body: kept, ms: 10000, action: { label: t('mediafiles.delete.undo'), run: () => void api.post<{ results: { result: string }[] }>('/holding/restore', { ids: held }).then((r) => { const bad = r.results.find((x) => x.result !== 'restored'); if (bad) toasts.push(t('mediafiles.delete.restoreFailed'), 'fail', { body: bad.result }); else toasts.push(t('mediafiles.delete.restored', { name }), 'ok'); }, (e) => toasts.push(t('mediafiles.delete.restoreFailed'), 'fail', { body: message(e) })) } } : { body: kept });
			open = false;
			ondone?.(p);
		} catch (e) {
			error = message(e);
			if (e instanceof ApiError && e.body.plan) plan = e.body.plan as RemovePlan;
		} finally {
			busy = false;
		}
	}
	// Changing an option makes the preview stale.
	function flip(which: 'files' | 'never') {
		if (which === 'files') files = !files;
		else never = !never;
		void planIt();
	}
</script>

<Sheet bind:open title={t('mediafiles.delete.title', { name })} size="sm">
	<div class="ds" data-testid="delete-sheet">
		<div class="ds-rows">
			<button type="button" class="ds-row" role="switch" aria-checked={files} onclick={() => flip('files')}>
				<span><b>{t('mediafiles.delete.alsoFiles')}</b><small>{t('mediafiles.delete.alsoFilesHint')}</small></span>
				<span class="ds-switch" class:on={files} aria-hidden="true"></span>
			</button>
			<button type="button" class="ds-row" role="switch" aria-checked={never} onclick={() => flip('never')}>
				<span><b>{t('mediafiles.delete.never')}</b><small>{t('mediafiles.delete.neverHint')}</small></span>
				<span class="ds-switch" class:on={never} aria-hidden="true"></span>
			</button>
		</div>

		<section class="ds-plan" aria-live="polite" data-testid="delete-plan">
			<p class="ds-h">{t('mediafiles.delete.willHappen')}</p>
			{#if !plan}
				<div class="ds-skel"></div>
			{:else}
				<ul>{#each deleteSummary(plan, kind) as line (line)}<li>{line}</li>{/each}</ul>
			{/if}
		</section>

		{#if error}<p class="ds-err" role="alert">{error}</p>{/if}

		<div class="ds-actions">
			<button type="button" class="ds-btn" onclick={() => (open = false)}>{t('mediafiles.delete.cancel')}</button>
			<button type="button" class="ds-btn danger" disabled={busy || !plan} onclick={run}>{busy ? t('mediafiles.delete.deleting') : files ? t('mediafiles.delete.kindAndFiles', { kind: kindWord }) : t('mediafiles.delete.kind', { kind: kindWord })}</button>
		</div>
	</div>
</Sheet>

<style>
	.ds { display: grid; gap: 16px; }
	.ds-rows { display: grid; border-radius: 14px; background: var(--surface-2); overflow: hidden; }
	.ds-row { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 14px 16px; border: 0; background: none; color: var(--text); text-align: left; cursor: pointer; font: inherit; }
	.ds-row:hover { background: color-mix(in srgb, var(--text) 4%, transparent); }
	.ds-row:focus-visible { outline: none; box-shadow: inset 0 0 0 2px var(--accent); }
	.ds-row + .ds-row { box-shadow: inset 0 .5px 0 color-mix(in srgb, var(--text) 10%, transparent); }
	.ds-row span:first-child { display: grid; gap: 3px; }
	.ds-row b { font: 600 15px/1.2 var(--font-ui); }
	.ds-row small { color: var(--text-muted); font: 500 13px/1.35 var(--font-ui); }
	.ds-switch { position: relative; flex: none; width: 44px; height: 26px; border-radius: 99px; background: color-mix(in srgb, var(--text) 18%, transparent); transition: background var(--dur-fast) var(--ease-out); }
	.ds-switch::after { content: ''; position: absolute; top: 3px; left: 3px; width: 20px; height: 20px; border-radius: 99px; background: #fff; box-shadow: 0 1px 3px rgb(0 0 0 / 25%); transition: transform var(--dur-fast) var(--ease-out); }
	.ds-switch.on { background: var(--accent-fill); }
	.ds-switch.on::after { transform: translateX(18px); }
	.ds-plan { padding: 14px 16px; border-radius: 14px; background: color-mix(in srgb, var(--text) 5%, transparent); }
	.ds-h { margin: 0 0 8px; color: var(--text-muted); font: 700 12px/1 var(--font-ui); letter-spacing: .06em; text-transform: uppercase; }
	.ds-plan ul { display: grid; gap: 6px; margin: 0; padding-left: 18px; font: 500 14px/1.4 var(--font-ui); }
	/* Placeholder while the preview loads. */
	.ds-skel { height: 44px; border-radius: 8px; background: color-mix(in srgb, var(--text) 8%, transparent); animation: pulse 1.2s ease-in-out infinite alternate; }
	@keyframes pulse { to { opacity: .5; } }
	.ds-err { margin: 0; color: var(--fail-text); font: 500 14px/1.4 var(--font-ui); }
	.ds-actions { display: flex; justify-content: flex-end; gap: 8px; }
	.ds-btn { height: 40px; padding: 0 18px; border: 0; border-radius: 999px; background: color-mix(in srgb, var(--text) 8%, transparent); color: var(--text); font: 600 14px/1 var(--font-ui); cursor: pointer; white-space: nowrap; }
	.ds-btn.danger { background: var(--accent-fill); color: var(--text-on-red); }
	.ds-btn:disabled { opacity: var(--disabled-opacity); cursor: default; }
	@media (max-width: 600px) { .ds-actions .ds-btn { flex: 1; } }
	@media (prefers-reduced-motion: reduce) { .ds-skel { animation: none; } }
</style>
