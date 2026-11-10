<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import AdminLinkButton from '#lib/ui/AdminLinkButton.svelte';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import type { BackupInfo } from '#lib/backups/index.ts';
	import { onMount } from 'svelte';
	import { to } from '#lib/core/paths.ts';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { bytes, dateTime, words } from '#lib/core/format.ts';
	import { session } from '#lib/core/session.svelte.ts';

	type Preset = 'balanced' | 'light' | 'careful';
	const presets = $derived<{ key: Preset; name: string; about: string }[]>([
		{ key: 'balanced', name: t('settings.backups.balanced'), about: t('settings.backups.balancedAbout') },
		{ key: 'light', name: t('settings.backups.light'), about: t('settings.backups.lightAbout') },
		{ key: 'careful', name: t('settings.backups.careful'), about: t('settings.backups.carefulAbout') }
	]);

	let backups = $state<BackupInfo[]>([]);
	let preset = $state<Preset>('balanced');
	let waiting = $state(false);
	let keyNote = $state('');
	let keyKit = $state<{ needed: boolean; fingerprint: string } | null>(null);
	let gettingKit = $state(false);
	let loaded = $state(false);
	let missing = $state(false);
	let error = $state('');
	let hasData = $state(false);
	let making = $state(false);
	let savingPreset = $state(false);
	let restoring = $state(false);
	let fileInput: HTMLInputElement | undefined = $state();

	async function load() {
		try {
			const r = await api.get<{ backups: BackupInfo[]; preset: Preset; restore_waiting: boolean; key_note: string; key_kit?: { needed: boolean; fingerprint: string } }>('/backups');
			backups = r.backups;
			preset = r.preset;
			waiting = r.restore_waiting;
			keyNote = r.key_note;
			keyKit = r.key_kit ?? null;
			hasData = true;
			error = '';
		} catch (e) {
			if (e instanceof ApiError && e.status === 404) missing = true;
			else error = message(e);
		} finally {
			loaded = true;
		}
	}
	onMount(load);

	// A backup file is the whole database: downloading it is a risky action, so a sign-in older than 10 minutes is asked for the password first (the box above the page), then Download works.
	function download(e: MouseEvent) {
		if (session.s.via !== 'session' || session.s.localRecent) return;
		e.preventDefault();
		window.dispatchEvent(new CustomEvent('holyarrs:reauth', { detail: { method: 'POST' } }));
	}

	// The key kit is the master key as a file (owner only, fresh password). It is a POST, so it is fetched and handed to the browser as a download.
	async function downloadKeyKit() {
		if (gettingKit) return;
		if (session.s.via === 'session' && !session.s.localRecent) {
			window.dispatchEvent(new CustomEvent('holyarrs:reauth', { detail: { method: 'POST' } }));
			return;
		}
		gettingKit = true;
		try {
			const headers: Record<string, string> = {};
			const csrf = api.csrf();
			if (csrf) headers['X-CSRF-Token'] = csrf;
			const res = await fetch(to('/api/v1/system/key-kit'), { method: 'POST', credentials: 'same-origin', headers });
			if (!res.ok) {
				const data = await res.json().catch(() => ({}));
				if (res.status === 403 && data.code === 'reauth') window.dispatchEvent(new CustomEvent('holyarrs:reauth', { detail: { method: 'POST' } }));
				throw new Error(data.error || t('settings.backups.requestFailed', { status: res.status }));
			}
			const name = /filename="([^"]+)"/.exec(res.headers.get('Content-Disposition') ?? '')?.[1] ?? 'holyarrs-key-kit.key';
			const a = document.createElement('a');
			a.href = URL.createObjectURL(await res.blob());
			a.download = name;
			a.click();
			setTimeout(() => URL.revokeObjectURL(a.href), 1000);
			if (keyKit) keyKit = { ...keyKit, needed: false };
			toasts.push(t('settings.backups.kitDownloaded'), 'ok', { body: t('settings.backups.kitApart') });
		} catch (e) {
			toasts.push(t('settings.backups.kitFailed'), 'fail', { body: message(e) });
		} finally {
			gettingKit = false;
		}
	}

	async function choose(p: Preset) {
		if (savingPreset) return;
		const before = preset;
		preset = p;
		savingPreset = true;
		try {
			await api.put('/backups/settings', { preset: p });
			toasts.push(t('settings.backups.planSaved'), 'ok', { body: presets.find((x) => x.key === p)?.name });
		} catch (e) {
			preset = before;
			toasts.push(t('settings.backups.couldntSave'), 'fail', { body: message(e) });
		} finally {
			savingPreset = false;
		}
	}

	async function makeNow() {
		making = true;
		try {
			const b = await api.post<BackupInfo>('/backups');
			backups = [b, ...backups.filter((x) => x.name !== b.name)];
			toasts.push(t('settings.backups.made'), 'ok');
		} catch (e) {
			toasts.push(t('settings.backups.makeFailed'), 'fail', { body: message(e) });
		} finally {
			making = false;
		}
	}

	async function remove(b: BackupInfo) {
		if (!confirm(t('settings.backups.confirmRemove', { name: b.name }))) return;
		try {
			await api.del(`/backups/${encodeURIComponent(b.name)}`);
			backups = backups.filter((x) => x.name !== b.name);
			toasts.push(t('settings.backups.deleted'), 'ok');
		} catch (e) {
			toasts.push(t('settings.backups.deleteFailed'), 'fail', { body: message(e) });
		}
	}

	async function restore(e: Event) {
		const file = (e.currentTarget as HTMLInputElement).files?.[0];
		if (!file) return;
		if (!confirm(t('settings.backups.confirmRestore', { name: file.name }))) {
			(e.currentTarget as HTMLInputElement).value = '';
			return;
		}
		restoring = true;
		try {
			const r = await api.request<{ note: string }>('/backups/restore', { method: 'POST', raw: file });
			toasts.push(r.note, 'ok');
			waiting = true;
		} catch (err) {
			toasts.push(t('settings.backups.restoreFailed'), 'fail', { body: message(err) });
		} finally {
			restoring = false;
			if (fileInput) fileInput.value = '';
		}
	}
</script>

<svelte:head><title>{t('settings.backups.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.backups.title')}>
	{#snippet actions()}
		{#if loaded && !missing}
		<Button icon="archive" variant="primary" type="button" onclick={makeNow} disabled={making} aria-busy={making}>{t('settings.backups.backUpNow')}</Button>
		{/if}
	{/snippet}
</PageHeader>

{#if !loaded}
	<SkSettings rows={2} />
{:else if missing}
	<EmptyState icon="archive" title={t('settings.backups.off')} text={t('settings.backups.offText')} />
{:else if !hasData}
	<LoadError hasData={false} message={error} onretry={load} />
{:else}
	{#if error}<LoadError hasData={true} message={error} onretry={load} />{/if}
	{#if waiting}
		<p class="notice warn" role="status" style="margin-bottom: var(--s-4)">
			<Icon name="info" size={18} /><span>{t('settings.backups.restoreReady')}</span>
		</p>
	{/if}

	<section class="card" id="backup-list" tabindex="-1" aria-labelledby="h-list">
		<div class="card-head">
			<h2 id="h-list" class="h2">{t('settings.backups.saved')}</h2>
			<label class="restore" aria-busy={restoring}>
				<Icon name="upload" size={18} /> {t('settings.backups.restoreFile')}
				<input class="sr-only" type="file" accept=".zip,application/zip" bind:this={fileInput} onchange={restore} disabled={restoring} />
			</label>
		</div>
		{#if backups.length === 0}
			<p class="small muted">{t('settings.backups.none')}</p>
		{:else}
			<ul class="rows">
				{#each backups as b (b.name)}
					<li>
						<div class="grow">
							<div class="mono small ellipsis">{b.name}</div>
							<div class="xs muted">{dateTime(b.created_at)} · {bytes(b.size)} · {b.detail || words(b.kind)}</div>
						</div>
						<AdminLinkButton variant="plain" href={to(`/api/v1/backups/${encodeURIComponent(b.name)}`)} download onclick={download}>{t('settings.backups.download')}</AdminLinkButton>
						<IconButton icon="trash" label={t('settings.backups.delete', { name: b.name })} tipSide="below-end" type="button" onclick={() => remove(b)} />
					</li>
				{/each}
			</ul>
		{/if}
		{#if keyNote}<p class="xs muted keynote">{keyNote}</p>{/if}
	</section>

	<SettingsCard id="backup-often" title={t('settings.backups.often')}>
		<div class="presets" role="radiogroup" aria-label={t('settings.backups.often')} aria-busy={savingPreset}>
			{#each presets as p (p.key)}
				<label class="preset" class:on={preset === p.key}>
					<input type="radio" name="preset" value={p.key} checked={preset === p.key} disabled={savingPreset} onchange={() => choose(p.key)} />
					<span class="pn">{p.name}</span>
					<span class="xs muted">{p.about}</span>
				</label>
			{/each}
		</div>
	</SettingsCard>

	{#if keyKit?.fingerprint}
		<SettingsCard id="backup-key" title={t('settings.backups.key')} description={t('settings.backups.keyDesc')}>
			{#if keyKit.needed}
				<p class="notice warn" role="status">
					<Icon name="info" size={18} /><span
						>{t('settings.backups.noKit')}{#if session.s.role !== 'owner'}
							{t('settings.backups.ownerOnlyKit')}{/if}</span
					>
				</p>
			{/if}
			<div class="keyrow">
				<span class="xs muted">{t('settings.backups.fingerprint')} <span class="mono">{keyKit.fingerprint}</span></span>
				{#if session.s.role === 'owner'}
					<Button icon="key" type="button" onclick={downloadKeyKit} disabled={gettingKit} aria-busy={gettingKit}>{t('settings.backups.downloadKit')}</Button>
				{/if}
			</div>
			{#if session.s.role === 'owner'}
				<p class="xs muted keynote">{t('settings.backups.restoreKey')}</p>
			{/if}
		</SettingsCard>
	{/if}

{/if}

<style>
	/* A file picker label, shaped like the secondary Button. */
	.restore { display: inline-flex; align-items: center; gap: 8px; min-height: 36px; padding: 0 16px; border-radius: var(--r-btn); background: var(--surface-2); color: var(--text); font: var(--t-headline); font-size: 15px; cursor: pointer; transition: background var(--dur-fast) var(--ease-out); }
	.restore:hover { background: var(--surface-3); }
	.restore:has(input:focus-visible) { box-shadow: var(--focus-ring); }
	.restore[aria-busy='true'] { opacity: .6; }
	@media (pointer: coarse) { .restore { min-height: 44px; } }
	.presets {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: var(--s-3);
		margin-top: var(--s-4);
	}
	.preset {
		position: relative;
		display: flex;
		flex-direction: column;
		gap: 4px;
		padding: var(--s-4);
		border: 1px solid var(--line-strong);
		border-radius: var(--r-btn);
		cursor: pointer;
		transition:
			border-color var(--dur) var(--ease-out),
			background var(--dur-fast) var(--ease-out);
	}
	.preset:hover {
		background: var(--surface-3);
	}
	.preset.on {
		border-color: var(--ha-red);
		background: var(--ha-red-soft);
	}
	/* Muted grey is too faint on the tinted and hover backgrounds (axe contrast). */
	.preset.on .muted,
	.preset:hover .muted {
		color: var(--text);
	}
	.preset input {
		position: absolute;
		opacity: 0;
		inset: 0;
		cursor: pointer;
	}
	.preset:has(input:focus-visible) {
		box-shadow: var(--focus-ring);
	}
	.pn {
		font-weight: var(--weight-semibold);
	}
	.keynote {
		margin-top: var(--s-4);
	}
	.keyrow {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: var(--s-3);
		margin-top: var(--s-4);
	}
	@media (max-width: 700px) {
		.presets {
			grid-template-columns: 1fr;
		}
	}
</style>
