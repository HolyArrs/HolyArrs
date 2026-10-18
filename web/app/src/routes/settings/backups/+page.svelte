<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { api, message, ApiError } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { bytes, dateTime, words } from '#lib/format.ts';
	import Icon from '#lib/Icon.svelte';
	import EmptyState from '#lib/EmptyState.svelte';
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

<svelte:head><title>Backups · HolyArrs</title></svelte:head>

<header class="page-head">
	<div>
		<h1>Backups</h1>
		<p>HolyArrs saves both of its databases on a schedule, and before every update. Media files are never in a backup.</p>
	</div>
	{#if loaded && !missing}
		<button class="btn btn-red" type="button" onclick={makeNow} disabled={making} aria-busy={making}><Icon name="archive" size={18} /> Make a backup now</button>
	{/if}
</header>

{#if !loaded}
	<div class="sk" style="height: 200px"></div>
{:else if missing}
	<EmptyState icon="archive" title="Backups are off in this build">
		<p>This HolyArrs runs without its backup manager.</p>
	</EmptyState>
{:else if error}
	<p class="notice bad" role="alert">{error}</p>
{:else}
	{#if waiting}
		<p class="notice warn" role="status" style="margin-bottom: var(--s-4)">
			<Icon name="info" size={18} /><span>A restore is checked and waiting. HolyArrs puts it in place the next time it starts.</span>
		</p>
	{/if}

	<section class="card" aria-labelledby="h-plan">
		<h2 id="h-plan" class="h2">How often</h2>
		<div class="presets" role="radiogroup" aria-labelledby="h-plan" aria-busy={savingPreset}>
			{#each presets as p (p.key)}
				<label class="preset" class:on={preset === p.key}>
					<input type="radio" name="preset" value={p.key} checked={preset === p.key} disabled={savingPreset} onchange={() => choose(p.key)} />
					<span class="pn">{p.name}</span>
					<span class="xs muted">{p.about}</span>
				</label>
			{/each}
		</div>
	</section>

	<section class="card" aria-labelledby="h-list">
		<div class="card-head">
			<h2 id="h-list" class="h2">Saved backups</h2>
			<label class="btn btn-ghost" aria-busy={restoring}>
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
							<div class="xs muted">{dateTime(b.created_at)} · {bytes(b.size)} · {words(b.kind)}</div>
						</div>
						<a class="btn btn-quiet" href={to(`/api/v1/backups/${encodeURIComponent(b.name)}`)} download>Download</a>
						<button class="icon-btn" type="button" aria-label="Delete {b.name}" onclick={() => remove(b)}><Icon name="trash" size={18} /></button>
					</li>
				{/each}
			</ul>
		{/if}
		{#if keyNote}<p class="xs muted keynote">{keyNote}</p>{/if}
	</section>
{/if}

<style>
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
			border-color var(--dur),
			background var(--dur-fast);
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
	label.btn {
		cursor: pointer;
	}
	label.btn:has(input:focus-visible) {
		box-shadow: var(--focus-ring);
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
