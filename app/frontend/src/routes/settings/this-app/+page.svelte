<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import AdminLinkButton from '#lib/ui/AdminLinkButton.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Field from '#lib/ui/Field.svelte';
	import MoreOptions from '#lib/ui/MoreOptions.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import type { Health } from '#lib/health/index.ts';
	import { onMount } from 'svelte';
	import { to } from '#lib/core/paths.ts';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { ago, bytes, dateTime, duration, words } from '#lib/core/format.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';

	// App-wide settings kept here: picture storage, the install count and History keep times. Autoplay trailers and the sign-in art live on Appearance; the Trakt keys on Outside lists.
	let uiBusy = $state(false);
	let uiLoaded = $state(false);
	// Count this install in the metadata server's per-country install total.
	let countInstall = $state(true);
	// Picture cache size (GB): covers and posters kept on this server.
	let cacheGB = $state(5);
	// History keep times in days (0 keeps forever).
	let keepRoutine = $state(30);
	let keepOther = $state(0);
	const keepChoices = [0, 7, 14, 30, 90, 180, 365];
	const keepWord = (d: number) => (d === 0 ? t('settings.thisApp.forever') : d === 365 ? t('settings.thisApp.oneYear') : t('settings.thisApp.days', { n: d }));
	async function loadUi() {
		try {
			const s = await api.get<{ count_install?: boolean; artwork_cache_gb?: number; history_routine_days?: number; history_other_days?: number }>('/settings/ui');
			keepRoutine = s.history_routine_days ?? 30;
			keepOther = s.history_other_days ?? 0;
			cacheGB = s.artwork_cache_gb ?? 5;
			countInstall = s.count_install !== false;
			uiLoaded = true;
		} catch {
			uiLoaded = false;
		}
	}
	async function saveCacheGB(gb: number, el?: HTMLSelectElement) {
		const was = cacheGB;
		uiBusy = true;
		try {
			cacheGB = (await api.put<{ artwork_cache_gb: number }>('/settings/ui', { artwork_cache_gb: gb })).artwork_cache_gb;
			toasts.push(t('settings.thisApp.cacheSaved'), 'ok', { body: t('settings.thisApp.upTo', { gb: cacheGB }) });
		} catch (e) {
			if (el) el.value = String(was);
			toasts.push(t('settings.thisApp.couldntSave'), 'fail', { body: message(e) });
			await loadUi();
		} finally {
			uiBusy = false;
		}
	}
	async function saveKeep(field: 'history_routine_days' | 'history_other_days', days: number, el?: HTMLSelectElement) {
		const was = field === 'history_routine_days' ? keepRoutine : keepOther;
		uiBusy = true;
		try {
			const s = await api.put<{ history_routine_days: number; history_other_days: number }>('/settings/ui', { [field]: days });
			keepRoutine = s.history_routine_days;
			keepOther = s.history_other_days;
			toasts.push(t('settings.thisApp.keepSaved'), 'ok');
		} catch (e) {
			if (el) el.value = String(was);
			toasts.push(t('settings.thisApp.couldntSave'), 'fail', { body: message(e) });
			await loadUi();
		} finally {
			uiBusy = false;
		}
	}
	async function saveCountInstall(on: boolean, el?: HTMLInputElement) {
		uiBusy = true;
		try {
			countInstall = (await api.put<{ count_install: boolean }>('/settings/ui', { count_install: on })).count_install;
			toasts.push(countInstall ? t('settings.thisApp.counted') : t('settings.thisApp.notCounted'), 'ok');
		} catch (e) {
			if (el) el.checked = !on;
			toasts.push(t('settings.thisApp.couldntSave'), 'fail', { body: message(e) });
			await loadUi();
		} finally {
			uiBusy = false;
		}
	}
	let health = $state<Health | null>(null);
	interface PathInfo {
		name: string;
		path: string;
		bytes?: number;
		free_bytes: number;
		kind?: string;
	}
	let storage = $state<{ app: PathInfo[]; library_folders: PathInfo[]; save_path: string } | null>(null);
	let storageNeedsProof = $state(false);
	let storageError = $state('');
	const freeWords = (n: number) => (n < 0 ? t('settings.thisApp.freeUnknown') : t('settings.thisApp.free', { size: bytes(n) }));
	let error = $state('');

	async function loadStorage() {
		if (!session.can('settings')) return;
		if (storageNeedsProof) return;
		try {
			storage = await api.get('/app/storage');
			storageError = '';
			storageNeedsProof = false;
		} catch (e) {
			storage = null;
			if (e instanceof ApiError && e.status === 403) {
				storageError = '';
				storageNeedsProof = true;
			} else storageError = message(e);
		}
	}

	async function load() {
		try {
			health = await api.get<Health>('/health');
			await loadStorage();
			error = '';
		} catch (e) {
			error = message(e);
		}
	}

	// URL base: the path HolyArrs lives under behind a reverse proxy. A saved base applies on the next start, unless HOLYARRS_URL_BASE or -url-base is set.
	let urlBase = $state<{ saved: string; in_use: string; restart_needed: boolean } | null>(null);
	let urlBaseTyped = $state('');
	let urlBaseBusy = $state(false);
	let urlBaseErr = $state('');
	let urlBaseGone = $state(false);
	async function loadURLBase() {
		try {
			urlBase = await api.get<{ saved: string; in_use: string; restart_needed: boolean }>('/settings/url-base');
			urlBaseTyped = urlBase.saved;
		} catch (err) {
			// An older server without the setting: no card rather than an error.
			if (err instanceof ApiError && err.status === 404) urlBaseGone = true;
			else urlBaseErr = message(err);
		}
	}
	async function saveURLBase(e: SubmitEvent) {
		e.preventDefault();
		urlBaseBusy = true;
		urlBaseErr = '';
		try {
			urlBase = await api.put<{ saved: string; in_use: string; restart_needed: boolean }>('/settings/url-base', { url_base: urlBaseTyped.trim() });
			urlBaseTyped = urlBase.saved;
			toasts.push(t('settings.thisApp.urlSaved'), 'ok', urlBase.restart_needed ? { body: t('settings.thisApp.restartToUse') } : undefined);
		} catch (err) {
			urlBaseErr = message(err);
		} finally {
			urlBaseBusy = false;
		}
	}

	onMount(() => {
		void load();
		if (session.can('settings')) void loadUi();
		if (session.can('settings')) void loadURLBase();
		const timer = setInterval(load, 30000);
		const offs = [live.on('health', load)];
		return () => {
			clearInterval(timer);
			offs.forEach((off) => off());
		};
	});
</script>

<svelte:head><title>{t('settings.thisApp.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.thisApp.title')} />

{#if error && !health}
	<LoadError hasData={false} message={error} onretry={load} />
{:else if !health}
	<SkSettings rows={3} />
{:else}
	{#if error}<LoadError hasData message={error} onretry={load} />{/if}
	{#if uiLoaded}
		<section class="card" id="history-keep-card" aria-labelledby="h-history-keep" data-testid="history-keep-setting">
			<div class="card-head"><h2 id="h-history-keep" class="h2">{t('settings.thisApp.history')}</h2></div>
			<label class="checkline">
				<select class="input" aria-label={t('settings.thisApp.keepRoutine')} value={keepRoutine} disabled={uiBusy} onchange={(e) => saveKeep('history_routine_days', Number(e.currentTarget.value), e.currentTarget)}>
					{#each keepChoices.includes(keepRoutine) ? keepChoices : [...keepChoices, keepRoutine] as d (d)}<option value={d}>{keepWord(d)}</option>{/each}
				</select>
				<span><b>{t('settings.thisApp.keepRoutine')}</b><br /><span class="hint">{t('settings.thisApp.keepRoutineHint')}</span></span>
			</label>
			<label class="checkline">
				<select class="input" aria-label={t('settings.thisApp.keepOther')} value={keepOther} disabled={uiBusy} onchange={(e) => saveKeep('history_other_days', Number(e.currentTarget.value), e.currentTarget)}>
					{#each keepChoices.includes(keepOther) ? keepChoices : [...keepChoices, keepOther] as d (d)}<option value={d}>{keepWord(d)}</option>{/each}
				</select>
				<span><b>{t('settings.thisApp.keepOther')}</b><br /><span class="hint">{t('settings.thisApp.keepOtherHint')}</span></span>
			</label>
		</section>
	{/if}

	{#if session.can('settings')}
		<MoreOptions>
			<SettingsCard id="this-server-card" title={t('settings.thisApp.server')}>
				{#if uiLoaded}
					<label class="checkline" data-testid="artwork-cache-setting">
						<select id="picture-storage" class="input" aria-label={t('settings.thisApp.pictures')} value={cacheGB} disabled={uiBusy} onchange={(e) => saveCacheGB(Number(e.currentTarget.value), e.currentTarget)}>
							{#each [1, 2, 5, 10, 20] as gb (gb)}<option value={gb}>{gb} GB</option>{/each}
						</select>
						<span><b>{t('settings.thisApp.pictures')}</b><br /><span class="hint">{t('settings.thisApp.picturesHint')}</span></span>
					</label>
					<label class="checkline" data-testid="count-install-setting">
						<input id="count-install" type="checkbox" checked={countInstall} disabled={uiBusy} onchange={(e) => saveCountInstall(e.currentTarget.checked, e.currentTarget)} />
						<span><b>{t('settings.thisApp.count')}</b><br /><span class="hint">{t('settings.thisApp.countHint')}</span></span>
					</label>
				{/if}
			</SettingsCard>
		<div data-testid="storage">
			<SettingsCard id="storage-card" title={t('settings.thisApp.storage')} description={t('settings.thisApp.storageDesc')}>
				{#if storage}
					<ul class="rows checks">
						{#each storage.app as p (p.name)}
							<li data-testid="storage-path">
								<span class="grow small">
									<b>{p.name}</b>{#if p.bytes} <span class="xs muted">· {bytes(p.bytes)}</span>{/if}
									<br /><code class="xs">{p.path}</code>
								</span>
								<span class="xs muted">{freeWords(p.free_bytes)}</span>
							</li>
						{/each}
						{#each storage.library_folders as p (p.path)}
							<li data-testid="storage-media">
								<span class="grow small"><b>{p.name}</b> <span class="xs muted">{t('settings.thisApp.libraryFolder')}</span><br /><code class="xs">{p.path}</code></span>
								<span class="xs muted">{freeWords(p.free_bytes)}</span>
							</li>
						{/each}
						{#if storage.save_path}
							<li>
								<span class="grow small"><b>{t('settings.thisApp.saveFolder')}</b><br /><code class="xs">{storage.save_path}</code></span>
							</li>
						{/if}
					</ul>
				{:else if storageNeedsProof}
					<p class="small muted" style="margin-top: var(--s-2)">{t('settings.thisApp.confirmPasswordLabel')}</p>
					<AdminLinkButton href={to('/settings/account')} style="margin-top: var(--s-3)">{t('settings.thisApp.openAccount')}</AdminLinkButton>
				{:else if storageError}
					<p class="notice bad small" role="alert" style="margin-top: var(--s-3)">{storageError}</p>
				{:else}
					<p class="small muted" style="margin-top: var(--s-2)">{t('settings.thisApp.needsPermission')}</p>
				{/if}
			</SettingsCard>
		</div>

		{#if session.can('settings') && !urlBaseGone}
			<SettingsCard id="url-base-card" title={t('settings.thisApp.urlBase')} description={t('settings.thisApp.urlBaseDesc')}>
				<form class="sc-urlbase" onsubmit={saveURLBase} data-testid="url-base-form">
					<Field id="url-base" label={t('settings.thisApp.urlBase')} hint={urlBase ? t('settings.thisApp.inUse', { value: urlBase.in_use || t('settings.thisApp.none') }) : undefined}>
						<input id="url-base" class="input mono" bind:value={urlBaseTyped} placeholder="/holyarrs" autocomplete="off" spellcheck="false" maxlength="200" />
					</Field>
					{#if urlBase?.restart_needed}<p class="notice warn" role="status">{t('settings.thisApp.needsRestart')}</p>{/if}
					{#if urlBaseErr}<p class="form-error" role="alert">{urlBaseErr}</p>{/if}
					<div class="row acts"><Button variant="primary" type="submit" disabled={!urlBase || urlBaseTyped.trim() === urlBase.saved} aria-busy={urlBaseBusy}>{t('settings.thisApp.save')}</Button></div>
				</form>
			</SettingsCard>
		{/if}
		</MoreOptions>
	{:else}
		<div data-testid="storage">
			<SettingsCard id="storage-card" title={t('settings.thisApp.storage')} description={t('settings.thisApp.storageDesc')}>
				{#if storage}
					<ul class="rows checks">
						{#each storage.app as p (p.name)}
							<li data-testid="storage-path">
								<span class="grow small">
									<b>{p.name}</b>{#if p.bytes} <span class="xs muted">· {bytes(p.bytes)}</span>{/if}
									<br /><code class="xs">{p.path}</code>
								</span>
								<span class="xs muted">{freeWords(p.free_bytes)}</span>
							</li>
						{/each}
						{#each storage.library_folders as p (p.path)}
							<li data-testid="storage-media">
								<span class="grow small"><b>{p.name}</b> <span class="xs muted">{t('settings.thisApp.libraryFolder')}</span><br /><code class="xs">{p.path}</code></span>
								<span class="xs muted">{freeWords(p.free_bytes)}</span>
							</li>
						{/each}
						{#if storage.save_path}
							<li>
								<span class="grow small"><b>{t('settings.thisApp.saveFolder')}</b><br /><code class="xs">{storage.save_path}</code></span>
							</li>
						{/if}
					</ul>
				{:else if storageNeedsProof}
					<p class="small muted" style="margin-top: var(--s-2)">{t('settings.thisApp.confirmPasswordLabel')}</p>
					<AdminLinkButton href={to('/settings/account')} style="margin-top: var(--s-3)">{t('settings.thisApp.openAccount')}</AdminLinkButton>
				{:else if storageError}
					<p class="notice bad small" role="alert" style="margin-top: var(--s-3)">{storageError}</p>
				{:else}
					<p class="small muted" style="margin-top: var(--s-2)">{t('settings.thisApp.needsPermission')}</p>
				{/if}
			</SettingsCard>
		</div>
	{/if}

	<p class="xs muted" style="margin-top: var(--s-5)">
		{t('settings.thisApp.licence')}
	</p>
{/if}

<style>
	.checkline > select.input { width: auto; flex: none; min-width: 112px; }
	.checks li {
		align-items: flex-start;
	}
	.checks .grow {
		min-width: 0;
		overflow-wrap: anywhere;
	}
	@media (max-width: 600px) {
		.checks li {
			align-items: stretch;
			flex-direction: column;
			gap: var(--s-2);
		}
		.checks .grow {
			width: 100%;
		}
		.checks li > :global(.chip) {
			align-self: flex-start;
		}
	}
	code {
		font-family: var(--font-mono);
		color: var(--text);
		overflow-wrap: anywhere;
	}
</style>
