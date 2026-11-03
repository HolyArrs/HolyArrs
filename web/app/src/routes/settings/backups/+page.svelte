<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import AdminLinkButton from '#lib/ui/AdminLinkButton.svelte';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { api, message, ApiError } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { bytes, dateTime, words } from '#lib/format.ts';
	import Icon from '#lib/Icon.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import type { BackupInfo } from '#lib/types.ts';

	type Preset = 'balanced' | 'light' | 'careful';
	const presets: { key: Preset; name: string; about: string }[] = [
		{ key: 'balanced', name: 'Balanced', about: 'Every day. Keeps the last 7 days and 4 weekly copies.' },
		{ key: 'light', name: 'Light', about: 'Once a week. Keeps 4 weeks. Uses the least space.' },
		{ key: 'careful', name: 'Careful', about: 'Every day. Keeps every copy for 30 days.' }
	];

	let backups = $state<BackupInfo[]>([]);
	let preset = $state<Preset>('balanced');
	let waiting = $state(false);
	let keyNote = $state('');
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
			const r = await api.get<{ backups: BackupInfo[]; preset: Preset; restore_waiting: boolean; key_note: string }>('/backups');
			backups = r.backups;
			preset = r.preset;
			waiting = r.restore_waiting;
			keyNote = r.key_note;
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

	async function choose(p: Preset) {
		if (savingPreset) return;
		const before = preset;
		preset = p;
		savingPreset = true;
		try {
			await api.put('/backups/settings', { preset: p });
			toasts.push(`Backups: ${presets.find((x) => x.key === p)?.name}.`, 'ok');
		} catch (e) {
			preset = before;
			toasts.push(message(e), 'fail');
		} finally {
			savingPreset = false;
		}
	}

	async function makeNow() {
		making = true;
		try {
			const b = await api.post<BackupInfo>('/backups');
			backups = [b, ...backups.filter((x) => x.name !== b.name)];
			toasts.push('Backup made.', 'ok');
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			making = false;
		}
	}

	async function remove(b: BackupInfo) {
		if (!confirm(`Delete the backup ${b.name}? This cannot be undone.`)) return;
		try {
			await api.del(`/backups/${encodeURIComponent(b.name)}`);
			backups = backups.filter((x) => x.name !== b.name);
			toasts.push('Backup deleted.', 'ok');
		} catch (e) {
			toasts.push(message(e), 'fail');
		}
	}

	async function restore(e: Event) {
		const file = (e.currentTarget as HTMLInputElement).files?.[0];
		if (!file) return;
		if (!confirm(`Restore from ${file.name}? HolyArrs checks the file, then restarts and puts it in place. Changes made since that backup are lost.`)) {
			(e.currentTarget as HTMLInputElement).value = '';
			return;
		}
		restoring = true;
		try {
			const r = await api.request<{ note: string }>('/backups/restore', { method: 'POST', raw: file });
			toasts.push(r.note, 'ok');
			waiting = true;
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			restoring = false;
			if (fileInput) fileInput.value = '';
		}
	}
</script>

<svelte:head><title>Backups and restore · HolyArrs</title></svelte:head>

<PageHeader title="Backups and restore">
	{#snippet actions()}
		{#if loaded && !missing}
		<Button icon="archive" variant="primary" type="button" onclick={makeNow} disabled={making} aria-busy={making}>Make a backup now</Button>
		{/if}
	{/snippet}
</PageHeader>

{#if !loaded}
	<SkSettings rows={2} />
{:else if missing}
	<EmptyState icon="archive" title="Backups are off in this build" text="This HolyArrs runs without its backup manager." />
{:else if !hasData}
	<LoadError hasData={false} message={error} onretry={load} />
{:else}
	{#if error}<LoadError hasData={true} message={error} onretry={load} />{/if}
	{#if waiting}
		<p class="notice warn" role="status" style="margin-bottom: var(--s-4)">
			<Icon name="info" size={18} /><span>A restore is checked and waiting. HolyArrs puts it in place the next time it starts.</span>
		</p>
	{/if}

	<SettingsCard id="backup-often" title="How often">
		<div class="presets" role="radiogroup" aria-label="How often" aria-busy={savingPreset}>
			{#each presets as p (p.key)}
				<label class="preset" class:on={preset === p.key}>
					<input type="radio" name="preset" value={p.key} checked={preset === p.key} disabled={savingPreset} onchange={() => choose(p.key)} />
					<span class="pn">{p.name}</span>
					<span class="xs muted">{p.about}</span>
				</label>
			{/each}
		</div>
	</SettingsCard>

	<section class="card" id="backup-list" tabindex="-1" aria-labelledby="h-list">
		<div class="card-head">
			<h2 id="h-list" class="h2">Saved backups</h2>
			<label class="restore" aria-busy={restoring}>
				<Icon name="upload" size={18} /> Restore from a file
				<input class="sr-only" type="file" accept=".zip,application/zip" bind:this={fileInput} onchange={restore} disabled={restoring} />
			</label>
		</div>
		{#if backups.length === 0}
			<p class="small muted">No backups yet. The first one is made on schedule, or press Make a backup now.</p>
		{:else}
			<ul class="rows">
				{#each backups as b (b.name)}
					<li>
						<div class="grow">
							<div class="mono small ellipsis">{b.name}</div>
							<div class="xs muted">{dateTime(b.created_at)} · {bytes(b.size)} · {b.detail || words(b.kind)}</div>
						</div>
						<AdminLinkButton variant="plain" href={to(`/api/v1/backups/${encodeURIComponent(b.name)}`)} download>Download</AdminLinkButton>
						<IconButton icon="trash" label="Delete {b.name}" tipSide="below-end" type="button" onclick={() => remove(b)} />
					</li>
				{/each}
			</ul>
		{/if}
		{#if keyNote}<p class="xs muted keynote">{keyNote}</p>{/if}
	</section>
{/if}

<style>
	/* A file picker label, shaped like the kit secondary Button. */
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
	@media (max-width: 700px) {
		.presets {
			grid-template-columns: 1fr;
		}
	}
</style>
