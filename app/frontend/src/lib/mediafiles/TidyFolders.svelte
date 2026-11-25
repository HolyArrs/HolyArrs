<script lang="ts">
	// Tidy my folders: loose movie files get a folder of their own. A preview first; nothing moves until the person confirms, and the last tidy can be undone.
	import Button from '#lib/ui/Button.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import Isolated from '#lib/ui/Isolated.svelte';
	import { fileName, type TidyMove, type TidyPlan, type TidyResult } from './organise.ts';
	import { ApiError, api, message } from '../core/api.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import { appLocale, ltr, t } from '#lib/core/i18n/i18n.svelte.ts';
	import { serverWords } from '#lib/core/i18n/server-words.ts';

	// loose: movies in genre folders a scan leaves out; looseTop: movies that are files right in the Library folder,
	// which is fine, so tidying them is only an option; suggestFlat: the folder looks flat but is not set so.
	let { folderId, loose = 0, looseTop = 0, suggestFlat = false, onDone }: { folderId: number; loose?: number; looseTop?: number; suggestFlat?: boolean; onDone?: () => void } = $props();

	let open = $state(false);
	let plan = $state<TidyPlan | null>(null);
	let error = $state('');
	let busy = $state(false);
	let failed = $state<TidyResult['failed']>([]);
	// What the last tidy of this folder left to undo, read on the way in.
	let undoFiles = $state(0);

	const n = (v: number) => v.toLocaleString(appLocale());

	async function load() {
		error = '';
		try {
			plan = await api.get<TidyPlan>(`/library-folders/${folderId}/tidy`);
			undoFiles = plan.undo_files;
		} catch (e) {
			error = message(e);
		}
	}
	$effect(() => {
		if (folderId) void api.get<TidyPlan>(`/library-folders/${folderId}/tidy`).then((p) => (undoFiles = p.undo_files), () => {});
	});

	// Keep the movies as files: only the setting changes, no file moves.
	async function keepFlat() {
		busy = true;
		try {
			await api.put(`/library-folders/${folderId}`, { flat: true });
			toasts.push(t('library.tidy.keptFlat'), 'ok');
			onDone?.();
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = false;
		}
	}

	function start() {
		plan = null;
		failed = [];
		open = true;
		void load();
	}

	function why(m: TidyMove): string {
		const path = ltr(m.path ?? '');
		if (m.why === 'exists') return t('library.tidy.whyExists', { path });
		if (m.why === 'two') return t('library.tidy.whyTwo', { path });
		if (m.why === 'has_movie') return t('library.tidy.whyHasMovie', { path });
		return serverWords(m.problem ?? '') ?? m.problem ?? '';
	}

	async function move() {
		if (!plan) return;
		busy = true;
		error = '';
		try {
			const r = await api.post<TidyResult>(`/library-folders/${folderId}/tidy`, { hash: plan.hash });
			failed = r.failed;
			toasts.push(t('library.tidy.moved', { n: n(r.moved) }), r.failed.length ? 'warn' : 'ok');
			if (!r.failed.length) open = false;
			await load();
			onDone?.();
		} catch (e) {
			if (e instanceof ApiError && e.status === 409) {
				error = t('library.tidy.changed');
				await load();
			} else error = message(e);
		} finally {
			busy = false;
		}
	}

	async function undo() {
		busy = true;
		error = '';
		try {
			const r = await api.post<TidyResult>(`/library-folders/${folderId}/tidy/undo`, {});
			failed = r.failed;
			toasts.push(t('library.tidy.undone', { n: n(r.moved) }), r.failed.length ? 'warn' : 'ok');
			const p = await api.get<TidyPlan>(`/library-folders/${folderId}/tidy`);
			undoFiles = p.undo_files;
			if (open) plan = p;
			onDone?.();
		} catch (e) {
			error = message(e);
			toasts.push(message(e), 'fail');
		} finally {
			busy = false;
		}
	}
</script>

{#if loose > 0 || looseTop > 0 || undoFiles > 0}
	<div class="notice tidy" data-testid="tidy-notice">
		{#if looseTop > 0}<p class="small" data-testid="tidy-flat-note">{suggestFlat ? t('library.tidy.suggestFlat') : t('library.tidy.looseTop', { n: n(looseTop) })} {t('library.tidy.optional')}</p>{/if}
		{#if loose > 0}<p class="small">{t('library.tidy.loose', { n: n(loose) })}</p>{/if}
		{#if undoFiles > 0}<p class="small muted" data-testid="tidy-undo-note">{t('library.tidy.undoNote', { n: n(undoFiles) })}</p>{/if}
		<div class="row">
			{#if looseTop > 0}<Button variant="primary" onclick={keepFlat} disabled={busy} data-testid="tidy-keep-flat">{t('library.tidy.keepFlat')}</Button>{/if}
			{#if loose > 0 || looseTop > 0}<Button variant={looseTop > 0 ? 'secondary' : 'primary'} onclick={start} disabled={busy} data-testid="tidy-open">{t('library.tidy.open')}</Button>{/if}
			{#if undoFiles > 0}<Button onclick={undo} disabled={busy} data-testid="tidy-undo">{t('library.tidy.undo')}</Button>{/if}
		</div>
	</div>
{/if}

{#if open}
	<Sheet bind:open title={t('library.tidy.open')} description={t('library.tidy.intro')} size="lg" onclose={() => (open = false)}>
		<div class="tidy-body" data-testid="tidy-sheet">
			{#if error}<p class="tp-msg bad" role="alert">{error}</p>{/if}
			{#if !plan}
				{#if !error}<Skeleton lines={4} />{/if}
			{:else}
				{#if plan.moves.length}
					<h3 class="h3">{t('library.tidy.ready', { n: n(plan.moves.length) })}</h3>
					<ul class="tp-rows steps">
						{#each plan.moves as m (m.from)}
							<li data-testid="tidy-move">
								<div class="pair">
									<span class="lab">{t('mediafiles.rename.now')}</span><span class="mono path old"><Isolated text={ltr(m.from)} /></span>
									<span class="lab">{t('mediafiles.rename.new')}</span><span class="mono path new"><Isolated text={ltr(m.to)} /></span>
								</div>
								{#if m.files.length}<p class="tp-sub">{t('library.tidy.also', { files: m.files.map((f) => fileName(f.from)).join(', ') })}</p>{/if}
							</li>
						{/each}
					</ul>
				{:else}
					<p class="tp-note" data-testid="tidy-nothing">{t('library.tidy.nothing')}</p>
				{/if}
				{#if plan.blocked.length}
					<details open={!plan.moves.length}>
						<summary>{t('library.tidy.blocked', { n: n(plan.blocked.length) })}</summary>
						<ul class="tp-rows steps">
							{#each plan.blocked as m (m.from)}
								<li data-testid="tidy-blocked"><span class="mono path"><Isolated text={ltr(m.from)} /></span><span class="tp-sub">{why(m)}</span></li>
							{/each}
						</ul>
					</details>
				{/if}
				{#if plan.unmatched.length}
					<details>
						<summary>{t('library.tidy.unmatched', { n: n(plan.unmatched.length) })}</summary>
						<ul class="tp-rows steps">
							{#each plan.unmatched as u (u)}<li data-testid="tidy-unmatched"><span class="mono path"><Isolated text={ltr(u)} /></span></li>{/each}
						</ul>
					</details>
				{/if}
				{#if failed.length}
					<div class="tp-msg bad" role="alert">
						<p>{t('library.tidy.failed', { n: n(failed.length) })}</p>
						<ul>{#each failed as f (f.from)}<li class="small"><span class="mono"><Isolated text={ltr(f.from)} /></span>: {serverWords(f.why) ?? f.why}</li>{/each}</ul>
					</div>
				{/if}
			{/if}
		</div>
		{#snippet footer()}
			<Button onclick={() => (open = false)} disabled={busy}>{t('library.tidy.cancel')}</Button>
			<Button variant="primary" onclick={move} disabled={busy || !plan?.moves.length} data-testid="tidy-confirm">{t('library.tidy.move')}</Button>
		{/snippet}
	</Sheet>
{/if}

<style>
	.tidy { display: flex; flex-direction: column; gap: 8px; }
	.tidy .row { display: flex; flex-wrap: wrap; gap: 8px; }
	.tidy-body { display: flex; flex-direction: column; gap: 12px; min-width: 0; }
	.pair { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 2px 10px; align-items: baseline; }
	.lab { color: var(--text-muted); font: var(--t-caption); }
	.path { overflow-wrap: anywhere; min-width: 0; }
	.path.new { color: var(--text); }
	.path.old { color: var(--text-muted); }
	.steps { list-style: none; margin: 0; padding: 0; }
	.steps li { display: flex; flex-direction: column; gap: 4px; padding: 10px 0; border-bottom: 1px solid var(--line); min-width: 0; }
	summary { cursor: pointer; font: var(--t-callout); padding: 6px 0; }
	@media (max-width: 600px) {
		.tidy .row :global(button) { flex: 1 1 auto; }
		.pair { grid-template-columns: minmax(0, 1fr); }
	}
</style>
