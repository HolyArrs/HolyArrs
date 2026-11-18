<!-- In-app updates: the mode, a downloaded version ready to install, progress, and going back one version. -->
<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import Button from '#lib/ui/Button.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import { api, message } from '../core/api.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import { plainVersion } from './update-notice.ts';

	type Mode = 'off' | 'ask' | 'auto';
	type View = {
		available: boolean;
		mode: Mode;
		mode_locked: boolean;
		source: 'image' | 'update';
		previous?: string;
		launch: { rollback?: string; exec_error?: string };
		progress: { phase: string; version?: string; message?: string };
		ready?: string;
		one_way?: boolean;
		running: string;
	};

	let { canAct }: { canAct: boolean } = $props();

	let view = $state<View | null>(null);
	let busy = $state(false);
	let mode = $state<Mode>('off');

	async function load() {
		try {
			view = await api.get<View>('/app/update/install');
			mode = view.mode;
		} catch {
			view = null;
		}
	}
	$effect(() => {
		if (canAct) load();
	});

	// While an update runs the page follows it; after the restart the server
	// answers again with the new version and the page reloads.
	const moving = $derived(['downloading', 'checking', 'waiting', 'copying', 'restarting'].includes(view?.progress.phase ?? ''));
	$effect(() => {
		if (!moving) return;
		const was = view?.running;
		const id = setInterval(async () => {
			try {
				const v = await api.get<View>('/app/update/install');
				if (v.running !== was) location.reload();
				view = v;
			} catch {
				// Restarting: the server is away for a few seconds.
			}
		}, 2000);
		return () => clearInterval(id);
	});

	async function setMode(m: Mode) {
		try {
			view = await api.put<View>('/app/update/mode', { mode: m });
			toasts.push(t('updates.inApp.saved'), 'ok');
		} catch (e) {
			mode = view?.mode ?? 'off';
			toasts.push(t('updates.inApp.saveFailed'), 'fail', { body: message(e) });
		}
	}

	async function act(path: string, failed: string) {
		busy = true;
		try {
			view = await api.post<View>(path);
		} catch (e) {
			toasts.push(failed, 'fail', { body: message(e) });
		} finally {
			busy = false;
		}
	}

	const phaseSays = $derived.by(() => {
		const p = view?.progress;
		if (!p) return '';
		const version = plainVersion(p.version ?? '');
		switch (p.phase) {
			case 'downloading': return t('updates.inApp.downloading', { version });
			case 'checking': return t('updates.inApp.checking', { version });
			case 'waiting': return t('updates.inApp.waiting', { what: serverWords(p.message ?? '') });
			case 'copying': return t('updates.inApp.copying');
			case 'restarting': return t('updates.inApp.restarting');
			case 'failed': return t('updates.inApp.failed', { message: serverWords(p.message ?? '') });
			default: return '';
		}
	});
</script>

{#if view?.available && canAct}
	<section class="st-plate in-app" aria-labelledby="s-inapp" data-testid="in-app-updates">
		<h2 id="s-inapp">{t('updates.inApp.title')}</h2>
		<div class="row">
			<SegmentedControl
				options={[
					{ value: 'off', label: t('updates.inApp.off') },
					{ value: 'ask', label: t('updates.inApp.ask') },
					{ value: 'auto', label: t('updates.inApp.auto') }
				]}
				bind:value={mode}
				label={t('updates.inApp.mode')}
				testid="in-app-mode"
				onchange={setMode}
			/>
			{#if view.mode_locked}<span class="note">{t('updates.inApp.locked')}</span>{/if}
		</div>
		<p class="hint">{mode === 'auto' ? t('updates.inApp.autoHint') : mode === 'ask' ? t('updates.inApp.askHint') : t('updates.inApp.offHint')}</p>
		<p class="hint">{view.source === 'update' ? t('updates.inApp.fromUpdate') : t('updates.inApp.fromImage')}</p>

		{#if view.launch.rollback}<p class="warn" data-testid="in-app-rollback">{t('updates.inApp.rolledBack', { text: view.launch.rollback })}</p>{/if}
		{#if view.launch.exec_error}<p class="warn">{t('updates.inApp.execError', { message: view.launch.exec_error })}</p>{/if}
		{#if phaseSays}<p class="phase" class:warn={view.progress.phase === 'failed'} aria-live="polite" data-testid="in-app-phase">{phaseSays}</p>{/if}

		{#if view.ready && !moving}
			<div class="ready">
				<p>{t('updates.inApp.ready', { version: plainVersion(view.ready) })}</p>
				{#if view.one_way}<p class="warn">{t('updates.inApp.oneWay')}</p>{/if}
				<Button {busy} onclick={() => act('/app/update/install', t('updates.inApp.updateFailed'))} variant="primary" data-testid="in-app-update-now">{t('updates.inApp.updateNow')}</Button>
			</div>
		{/if}
		{#if view.previous && !moving}
			<div class="back">
				<Button variant="plain" {busy} onclick={() => act('/app/update/go-back', t('updates.inApp.goBackFailed'))} data-testid="in-app-go-back">{t('updates.inApp.goBack', { version: plainVersion(view.previous) })}</Button>
			</div>
		{/if}
	</section>
{/if}

<style>
	.in-app { display: flex; flex-direction: column; gap: 10px; }
	.row { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
	.note, .hint { margin: 0; font-size: 13.5px; line-height: 1.5; color: var(--text-muted); }
	.phase { margin: 0; font-size: 14.5px; color: var(--text); }
	.warn { margin: 0; font-size: 14px; line-height: 1.5; color: var(--wait-text, var(--text)); }
	.ready { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
	.ready p { margin: 0; font-size: 14.5px; color: var(--text); }
	@media (max-width: 767px) {
		.ready { flex-direction: column; align-items: flex-start; }
	}
</style>
