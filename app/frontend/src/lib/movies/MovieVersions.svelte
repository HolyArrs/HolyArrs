<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import Menu from '#lib/ui/Menu.svelte';
	import TitleSection from '#lib/ui/TitleSection.svelte';
	import { shortFolder } from '#lib/ui/TitleHero.svelte';
	import { api, message } from '../core/api.ts';
	import { bytes, date, tone, words } from '../core/format.ts';
	import { secondVersion } from '../mediafiles/index.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import type { LibraryFolder, PickRulesView, TitleView, VersionView } from '../core/types.ts';

	let { list = true, title, onchange }: { list?: boolean; title: TitleView; onchange: () => void | Promise<void> } = $props();

	let dialog: HTMLDialogElement | undefined = $state();
	let all = $state(false);
	let rules = $state<PickRulesView[]>([]);
	let folders = $state<LibraryFolder[]>([]);
	let vLabel = $state('4K');
	let vEdition = $state('');
	let vAudio = $state('');
	let vFolderName = $state('');
	let vRules = $state(0);
	let vFolder = $state(0);
	let vError = $state('');
	let vBusy = $state(false);

	const lead = $derived((title.versions.find((v) => v.main) ?? title.versions[0])?.id);
	const movieFolders = $derived(folders.filter((f) => f.kind === 'movie'));

	export async function openAdd() {
		vError = '';
		try {
			if (!rules.length) rules = (await api.get<{ pick_rules: PickRulesView[] }>('/pick-rules')).pick_rules.filter((r) => !r.kinds.length || r.kinds.includes('movie'));
			if (!folders.length) folders = (await api.get<{ library_folders: LibraryFolder[] }>('/library-folders')).library_folders;
			const used = new Set(title.versions.map((v) => v.label.toLowerCase()));
			const sv = secondVersion(rules, movieFolders, title.library_folder_id ?? 0);
			vLabel = used.has(sv.label.toLowerCase()) ? '' : sv.label;
			vRules = sv.pick_rules_id;
			vEdition = '';
			vAudio = '';
			vFolderName = '';
			vFolder = sv.library_folder_id || movieFolders[0]?.id || 0;
		} catch (e) {
			vError = message(e);
		}
		// Show the dialog only after the defaults are set, so they can't overwrite typing.
		dialog?.showModal();
	}

	async function add(e: SubmitEvent) {
		e.preventDefault();
		vBusy = true;
		vError = '';
		try {
			await api.post(`/titles/${title.id}/versions`, { label: vLabel.trim(), pick_rules_id: vRules, library_folder_id: vFolder, edition: vEdition.trim(), audio_languages: vAudio.split(/[,\s]+/).filter(Boolean), folder_name: vFolderName });
			dialog?.close();
			all = true;
			await onchange();
		} catch (err) {
			vError = message(err);
		} finally {
			vBusy = false;
		}
	}

	export async function remove(v: VersionView) {
		if (!confirm(`Stop keeping the ${v.label} Version of this movie? Files on disk are not touched.`)) return;
		try {
			await api.del(`/titles/${title.id}/versions/${v.id}`);
			await onchange();
		} catch (e) {
			toasts.push(`Couldn't remove Version`, 'fail', { body: message(e) });
		}
	}

	const slotOf = (v: VersionView) => title.slots.find((s) => s.version_id === v.id);
</script>

{#if list}
<TitleSection id="h-versions" title="Versions" testid="movie-versions" primary>
	{#snippet head()}
		<div class="tp-head-end">
			{#if title.versions.length > 1}
				<Button variant="plain" aria-expanded={all} aria-controls="movie-versions-list" onclick={() => (all = !all)}>{all ? 'Show less' : `See all ${title.versions.length}`}</Button>
			{/if}
			<Button icon="plus" onclick={openAdd}>Add a Version</Button>
		</div>
	{/snippet}
	<ul class="tp-list tp-vrow" id="movie-versions-list" aria-label="Versions">
		{#each title.versions as v (v.id)}
			{@const s = slotOf(v)}
			{@const f = s?.file}
			<li class="tp-row" data-testid="version" hidden={!all && v.id !== lead}>
				<div class="tp-row-main">
					<b class="v-name">{v.label}{#if v.main && v.label.trim().toLowerCase() !== 'main'}<span class="v-main" data-testid="version-main">Main</span>{:else if v.main}<span class="sr-only" data-testid="version-main">, the main Version</span>{/if}</b>
					<span class="xs muted">{v.edition || 'Any edition'} · {v.audio_languages?.length ? `audio ${v.audio_languages.join(', ')}` : 'any audio'} · {v.pick_rules_name}{v.keep_up_to_date ? '' : ' · not kept up to date'}</span>
					{#if f}
						<span class="xs muted">{bytes(f.Size)}{#if f.Quality?.resolution} · {f.Quality.resolution}p{f.Quality.source ? ` ${words(String(f.Quality.source))}` : ''}{/if} · Added {date(f.AddedAt)}</span>
					{:else}
						<span class="xs muted">No file yet.</span>
					{/if}
					{#if f || v.folder_path}
						<details class="tp-details">
							<summary>Details</summary>
							<ul>
								{#if f}<li class="path">File: {shortFolder(f.RelativePath)}</li>{/if}
								{#if f?.ReleaseName}<li class="path">From the release {f.ReleaseName}</li>{/if}
								{#if v.folder_path}<li class="path">Folder: {shortFolder(v.folder_path)}</li>{/if}
							</ul>
						</details>
					{/if}
				</div>
				<div class="tp-row-end">
					{#if s}<span class="chip {tone(s.state)}">{s.state_label}</span>{/if}
					{#if !v.main}
						<Menu label="Version actions for {v.label}" items={[{ label: `Remove ${v.label}`, icon: 'trash', destructive: true, onselect: () => remove(v) }]} width={220} />
					{/if}
				</div>
			</li>
		{/each}
	</ul>
</TitleSection>
{/if}

<dialog bind:this={dialog} aria-labelledby="movie-version-title" class="modal">
	<form onsubmit={add}>
		<h2 id="movie-version-title" class="h2">Add a Version of {title.name}</h2>
		<p class="small muted">Another copy, like 4K, with its own Pick rules and folder.</p>
		<div class="field" style="margin-top: var(--s-5)">
			<label for="mv-label">Version name</label>
			<input id="mv-label" class="input" bind:value={vLabel} maxlength="40" required />
		</div>
		<div class="field">
			<label for="mv-edition">Movie edition</label>
			<input id="mv-edition" class="input" bind:value={vEdition} maxlength="40" placeholder="For example, Theatrical or Directors Cut" />
			<span class="hint">Leave empty for the existing quality-only choice. An unknown edition needs review before it can fill an edition choice.</span>
		</div>
		<div class="field">
			<label for="mv-audio">Required audio languages</label>
			<input id="mv-audio" class="input" bind:value={vAudio} placeholder="For example, en, ja" />
			<span class="hint">Audio must have every language. Subtitles don’t count.</span>
		</div>
		<div class="field">
			<label for="mv-existing-folder">Existing movie folder (optional)</label>
			<input id="mv-existing-folder" class="input" bind:value={vFolderName} maxlength="200" />
			<span class="hint">To use a copy you have, name its folder. Leave empty for a new one.</span>
		</div>
		<div class="field">
			<label for="mv-rules">Pick rules for this Version</label>
			<select id="mv-rules" class="input" bind:value={vRules}>
				{#each rules as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
			</select>
		</div>
		<div class="field">
			<label for="mv-folder">Library folder for this Version</label>
			<select id="mv-folder" class="input" bind:value={vFolder}>
				{#each movieFolders as f (f.id)}<option value={f.id}>{f.label || f.path} ({f.path})</option>{/each}
			</select>
		</div>
		{#if vError}<p class="form-error" role="alert">{vError}</p>{/if}
		<div class="row modal-actions">
			<Button variant="plain" onclick={() => dialog?.close()}>Cancel</Button>
			<Button variant="primary" type="submit" disabled={vBusy || !vLabel.trim()} busy={vBusy}>Add version</Button>
		</div>
	</form>
</dialog>

<style>
	.path { overflow-wrap: anywhere; }
	.tp-row[hidden] { display: none; }
	.modal { width: min(480px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow: auto; padding: var(--s-6); border: 0; border-radius: var(--r-modal); background: var(--surface-1); color: var(--text); box-shadow: var(--shadow-modal); }
	.modal::backdrop { background: var(--scrim); }
	.modal-actions { justify-content: flex-end; margin-top: var(--s-6); }
	.v-name { display: inline-flex; align-items: center; gap: 8px; }
	.v-main { display: inline-flex; align-items: center; height: 20px; padding: 0 7px; border-radius: 5px; border: 1px solid color-mix(in srgb, var(--text) 30%, transparent); color: var(--text-muted); font: 700 11px/1 var(--font-ui); }
</style>
