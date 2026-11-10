<script lang="ts">
	// Sheet that shows what deleting a show or artist (or part of it) will do, then applies it.
	import Button from '#lib/ui/Button.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import { deleteBody, deleteSummary, unit, wholeScope, type DeleteScope, type RemovePlan } from './organise.ts';
	import { api, ApiError, message } from '../core/api.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	let {
		titleId,
		name,
		kind = 'show',
		ondone
	}: { titleId: number; name: string; kind?: 'show' | 'artist'; ondone: (plan: RemovePlan) => void } = $props();

	let isOpen = $state(false);
	let scope = $state<DeleteScope>({});
	let files = $state(false);
	let never = $state(false);
	let plan = $state<RemovePlan | null>(null);
	let error = $state('');
	let busy = $state(false);
	// Counts plan requests so a slow older answer cannot replace a newer one.
	let ask = 0;

	// Whole means no season or album is picked, so the switches cover the entire title.
	const whole = $derived(wholeScope(scope));
	const what = $derived(whole ? name : kind === 'artist' ? `${name} – ${scope.label ?? ''}` : `${name} ${scope.label ?? ''}`.trim());
	const kindWord = $derived(t(`mediafiles.kind.${kind}`));

	function flip(which: 'files' | 'never') {
		if (which === 'files') files = !files;
		else never = !never;
		void planIt();
	}

	// Asks the server for a dry run so the summary matches the switches.
	async function planIt() {
		const mine = ++ask;
		error = '';
		try {
			const p = await api.post<RemovePlan>(`/titles/${titleId}/delete`, body(false));
			if (mine === ask) plan = p;
		} catch (e) {
			if (mine === ask) error = message(e);
		}
	}

	// Called by the parent to start with a fresh scope and a first plan.
	export function open(s: DeleteScope = {}) {
		scope = s;
		files = false;
		never = false;
		plan = null;
		isOpen = true;
		void planIt();
	}

	async function go() {
		busy = true;
		error = '';
		try {
			const p = await api.post<RemovePlan>(`/titles/${titleId}/delete`, body(true));
			isOpen = false;
			const kept = p.held.length ? (p.holding_days ? t('mediafiles.delete.keptDays', { n: p.holding_days }) : t('mediafiles.delete.keptFolder')) : t('mediafiles.delete.stay');
			toasts.push(t('mediafiles.delete.deleted', { name: what }), 'ok', { body: kept });
			ondone(p);
		} catch (e) {
			error = message(e);
			// A refused delete returns the current plan, so show it.
			if (e instanceof ApiError && e.body.plan) plan = e.body.plan as RemovePlan;
		} finally {
			busy = false;
		}
	}

	function body(apply: boolean): Record<string, unknown> {
		return deleteBody(scope, files, never, apply);
	}
</script>

<Sheet bind:open={isOpen} title={t('mediafiles.delete.title', { name: what })} size="md">
	<div class="sd" data-testid="show-delete">
		<div class="sd-rows">
			<button type="button" class="sd-row" role="switch" aria-checked={files} onclick={() => flip('files')}>
				<span><b>{t(whole ? 'mediafiles.showDelete.alsoIts' : 'mediafiles.showDelete.alsoTheir')}</b><small>{t('mediafiles.delete.alsoFilesHint')}</small></span>
				<span class="sd-switch" class:on={files} aria-hidden="true"></span>
			</button>
			{#if whole}
				<button type="button" class="sd-row" role="switch" aria-checked={never} onclick={() => flip('never')}>
					<span><b>{t('mediafiles.delete.never')}</b><small>{t('mediafiles.delete.neverHint')}</small></span>
					<span class="sd-switch" class:on={never} aria-hidden="true"></span>
				</button>
			{/if}
		</div>
		<section class="sd-plan" aria-live="polite" data-testid="delete-plan">
			<p class="sd-h">{t('mediafiles.delete.willHappen')}</p>
			{#if !plan}
				{#if !error}<div class="sd-skel"></div>{/if}
			{:else}
				<ul>{#each deleteSummary(plan, kind) as line (line)}<li>{line}</li>{/each}</ul>
				{#if plan.held.length}
					<details class="sd-files"><summary>{t('mediafiles.delete.held', { files: unit('file', plan.held.length) })}</summary>
						<ul>{#each plan.held as f (f.file_id)}<li class="mono">{f.path}</li>{/each}</ul>
					</details>
				{/if}
				{#if plan.kept.length}
					<details class="sd-files"><summary>{t(plan.kept.length === 1 ? 'mediafiles.showDelete.staysOne' : 'mediafiles.showDelete.staysMany', { n: plan.kept.length })}</summary>
						<ul>{#each plan.kept as f (f.file_id)}<li class="mono">{f.path} <span class="muted">({f.why})</span></li>{/each}</ul>
					</details>
				{/if}
			{/if}
		</section>
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
		<div class="sd-actions">
			<Button variant="plain" onclick={() => (isOpen = false)}>{t('mediafiles.delete.cancel')}</Button>
			<Button variant="destructive" onclick={go} disabled={busy || !plan} busy={busy}>
				{whole ? t('mediafiles.delete.kind', { kind: kindWord }) : t('mediafiles.delete.kind', { kind: scope.label || t(kind === 'artist' ? 'mediafiles.showDelete.theseTracks' : 'mediafiles.showDelete.theseEpisodes') })}
			</Button>
		</div>
	</div>
</Sheet>

<style>
	.sd { display: grid; gap: 14px; }
	.sd-rows { display: grid; border-radius: var(--r-panel); background: var(--surface-2); overflow: hidden; }
	.sd-row { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 14px 20px; border: 0; background: none; color: var(--text); text-align: left; cursor: pointer; font: inherit; }
	.sd-row + .sd-row { box-shadow: inset 0 .5px 0 color-mix(in srgb, var(--text) 10%, transparent); }
	.sd-row:hover { background: color-mix(in srgb, var(--text) 4%, transparent); }
	.sd-row:focus-visible { outline: none; box-shadow: inset 0 0 0 2px var(--accent); }
	.sd-row span:first-child { display: grid; gap: 3px; }
	.sd-row b { font: 600 15px/1.2 var(--font-ui); }
	.sd-row small { color: color-mix(in srgb, var(--text) 40%, var(--text-muted)); font: 500 13px/1.3 var(--font-ui); }
	.sd-switch { position: relative; flex: none; width: 44px; height: 26px; border-radius: 99px; background: color-mix(in srgb, var(--text) 18%, transparent); transition: background var(--dur-fast) var(--ease-out); }
	.sd-switch::after { content: ''; position: absolute; top: 3px; left: 3px; width: 20px; height: 20px; border-radius: 99px; background: #fff; box-shadow: 0 1px 3px rgb(0 0 0 / 25%); transition: transform var(--dur-fast) var(--ease-out); }
	.sd-switch.on { background: var(--accent-fill); }
	.sd-switch.on::after { transform: translateX(18px); }
	.sd-plan { display: grid; gap: 8px; padding: 14px 20px; border-radius: var(--r-panel); background: var(--surface-2); }
	.sd-h { margin: 0; color: var(--text-muted); font: 600 13px/1 var(--font-ui); }
	.sd-plan ul { margin: 0; padding-left: 18px; display: grid; gap: 4px; font: 500 14px/1.4 var(--font-ui); }
	.sd-files summary { cursor: pointer; color: var(--text-muted); font: 600 13px/1.3 var(--font-ui); }
	.sd-files ul { margin-top: 6px; padding-left: 0; list-style: none; max-height: 160px; overflow: auto; }
	.sd-files .mono { font: 500 12px/1.5 var(--font-mono, ui-monospace, monospace); color: var(--text-muted); overflow-wrap: anywhere; }
	.sd-skel { height: 48px; border-radius: 10px; background: color-mix(in srgb, var(--text) 6%, transparent); }
	.sd-actions { display: flex; justify-content: flex-end; gap: 8px; }
</style>
