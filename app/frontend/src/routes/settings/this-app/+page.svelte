<script lang="ts">
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
	const keepWord = (d: number) => (d === 0 ? 'Forever' : d === 365 ? '1 year' : `${d} days`);
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
			toasts.push('Picture cache saved', 'ok', { body: `Up to ${cacheGB} GB` });
		} catch (e) {
			if (el) el.value = String(was);
			toasts.push("Couldn't save", 'fail', { body: message(e) });
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
			toasts.push('History keep time saved', 'ok');
		} catch (e) {
			if (el) el.value = String(was);
			toasts.push("Couldn't save", 'fail', { body: message(e) });
			await loadUi();
		} finally {
			uiBusy = false;
		}
	}
	async function saveCountInstall(on: boolean, el?: HTMLInputElement) {
		uiBusy = true;
		try {
			countInstall = (await api.put<{ count_install: boolean }>('/settings/ui', { count_install: on })).count_install;
			toasts.push(countInstall ? 'Install counted' : 'Install not counted', 'ok');
		} catch (e) {
			if (el) el.checked = !on;
			toasts.push("Couldn't save", 'fail', { body: message(e) });
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
	const freeWords = (n: number) => (n < 0 ? 'free space unknown' : `${bytes(n)} free`);
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
			toasts.push('URL base saved', 'ok', urlBase.restart_needed ? { body: 'Restart to use it' } : undefined);
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
		const t = setInterval(load, 30000);
		const offs = [live.on('health', load)];
		return () => {
			clearInterval(t);
			offs.forEach((off) => off());
		};
	});
</script>

<svelte:head><title>This app · HolyArrs</title></svelte:head>

<PageHeader title="This app" />

{#if error && !health}
	<LoadError hasData={false} message={error} onretry={load} />
{:else if !health}
	<SkSettings rows={3} />
{:else}
	{#if error}<LoadError hasData message={error} onretry={load} />{/if}
	{#if uiLoaded}
		<section class="card" id="history-keep-card" aria-labelledby="h-history-keep" data-testid="history-keep-setting">
			<div class="card-head"><h2 id="h-history-keep" class="h2">History</h2></div>
			<label class="checkline">
				<select class="input" aria-label="Keep routine Events" value={keepRoutine} disabled={uiBusy} onchange={(e) => saveKeep('history_routine_days', Number(e.currentTarget.value), e.currentTarget)}>
					{#each keepChoices.includes(keepRoutine) ? keepChoices : [...keepChoices, keepRoutine] as d (d)}<option value={d}>{keepWord(d)}</option>{/each}
				</select>
				<span><b>Keep routine Events</b><br /><span class="hint">Searches, feed checks, refreshes, scans</span></span>
			</label>
			<label class="checkline">
				<select class="input" aria-label="Keep other Events" value={keepOther} disabled={uiBusy} onchange={(e) => saveKeep('history_other_days', Number(e.currentTarget.value), e.currentTarget)}>
					{#each keepChoices.includes(keepOther) ? keepChoices : [...keepChoices, keepOther] as d (d)}<option value={d}>{keepWord(d)}</option>{/each}
				</select>
				<span><b>Keep other Events</b><br /><span class="hint">Downloads, imports, better copies, problems, sign-ins, settings changes</span></span>
			</label>
		</section>
	{/if}

	{#if session.can('settings')}
		<MoreOptions>
			<SettingsCard id="this-server-card" title="This server">
				{#if uiLoaded}
					<label class="checkline" data-testid="artwork-cache-setting">
						<select id="picture-storage" class="input" aria-label="Picture storage" value={cacheGB} disabled={uiBusy} onchange={(e) => saveCacheGB(Number(e.currentTarget.value), e.currentTarget)}>
							{#each [1, 2, 5, 10, 20] as gb (gb)}<option value={gb}>{gb} GB</option>{/each}
						</select>
						<span><b>Picture storage</b><br /><span class="hint">Oldest pictures are removed past this size and reload when needed</span></span>
					</label>
					<label class="checkline" data-testid="count-install-setting">
						<input id="count-install" type="checkbox" checked={countInstall} disabled={uiBusy} onchange={(e) => saveCountInstall(e.currentTarget.checked, e.currentTarget)} />
						<span><b>Count this install</b><br /><span class="hint">Adds this install to a per-country total. Nothing else is sent.</span></span>
					</label>
				{/if}
			</SettingsCard>
		<div data-testid="storage">
			<SettingsCard id="storage-card" title="Storage" description="Paths inside the container. HolyArrs data lives in config.">
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
								<span class="grow small"><b>{p.name}</b> <span class="xs muted">Library folder</span><br /><code class="xs">{p.path}</code></span>
								<span class="xs muted">{freeWords(p.free_bytes)}</span>
							</li>
						{/each}
						{#if storage.save_path}
							<li>
								<span class="grow small"><b>Downloader save folder</b><br /><code class="xs">{storage.save_path}</code></span>
							</li>
						{/if}
					</ul>
				{:else if storageNeedsProof}
					<p class="small muted" style="margin-top: var(--s-2)">Confirm your password in My account to see paths</p>
					<AdminLinkButton href={to('/settings/account')} style="margin-top: var(--s-3)">Open My account</AdminLinkButton>
				{:else if storageError}
					<p class="notice bad small" role="alert" style="margin-top: var(--s-3)">{storageError}</p>
				{:else}
					<p class="small muted" style="margin-top: var(--s-2)">Needs the Settings permission</p>
				{/if}
			</SettingsCard>
		</div>

		{#if session.can('settings') && !urlBaseGone}
			<SettingsCard id="url-base-card" title="URL base" description="For reverse proxies, e.g. /holyarrs. Empty for none.">
				<form class="sc-urlbase" onsubmit={saveURLBase} data-testid="url-base-form">
					<Field id="url-base" label="URL base" hint={urlBase ? `In use now: ${urlBase.in_use || 'none'}` : undefined}>
						<input id="url-base" class="input mono" bind:value={urlBaseTyped} placeholder="/holyarrs" autocomplete="off" spellcheck="false" maxlength="200" />
					</Field>
					{#if urlBase?.restart_needed}<p class="notice warn" role="status">Needs a restart. HOLYARRS_URL_BASE or -url-base overrides it.</p>{/if}
					{#if urlBaseErr}<p class="form-error" role="alert">{urlBaseErr}</p>{/if}
					<div class="row acts"><Button variant="primary" type="submit" disabled={!urlBase || urlBaseTyped.trim() === urlBase.saved} aria-busy={urlBaseBusy}>Save</Button></div>
				</form>
			</SettingsCard>
		{/if}
		</MoreOptions>
	{:else}
		<div data-testid="storage">
			<SettingsCard id="storage-card" title="Storage" description="Paths inside the container. HolyArrs data lives in config.">
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
								<span class="grow small"><b>{p.name}</b> <span class="xs muted">Library folder</span><br /><code class="xs">{p.path}</code></span>
								<span class="xs muted">{freeWords(p.free_bytes)}</span>
							</li>
						{/each}
						{#if storage.save_path}
							<li>
								<span class="grow small"><b>Downloader save folder</b><br /><code class="xs">{storage.save_path}</code></span>
							</li>
						{/if}
					</ul>
				{:else if storageNeedsProof}
					<p class="small muted" style="margin-top: var(--s-2)">Confirm your password in My account to see paths</p>
					<AdminLinkButton href={to('/settings/account')} style="margin-top: var(--s-3)">Open My account</AdminLinkButton>
				{:else if storageError}
					<p class="notice bad small" role="alert" style="margin-top: var(--s-3)">{storageError}</p>
				{:else}
					<p class="small muted" style="margin-top: var(--s-2)">Needs the Settings permission</p>
				{/if}
			</SettingsCard>
		</div>
	{/if}

	<p class="xs muted" style="margin-top: var(--s-5)">
		HolyArrs is free software under the AGPL-3.0.
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
