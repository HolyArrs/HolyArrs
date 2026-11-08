<script lang="ts">
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { MetadataNotice } from '#lib/metadata/index.ts';
	import { pickId } from '../mediafiles/index.ts';
	import { albumChoices, extraKinds, mainKinds, newAlbumChoices, pressingStatuses, toggle } from '../music/index.ts';
	import type { LookupArtist, MusicSettings } from '../music/index.ts';
	import { to } from '../core/paths.ts';
	import { api, message } from '../core/api.ts';
	import type { LibraryFolder, PickRulesView, TitleView } from '../core/types.ts';

	// The add-an-artist dialog: Library folder, Pick rules, which albums to keep up to date, new albums, album kinds and Pressing statuses.

	let {
		folders,
		rules,
		defaults,
		onadded
	}: {
		folders: LibraryFolder[];
		rules: PickRulesView[];
		defaults: MusicSettings | null;
		onadded: (artist: LookupArtist, title: TitleView) => void;
	} = $props();

	let sheetOpen = $state(false);
	let picked = $state<LookupArtist | null>(null);
	// Follow puts it on my calendar (ticked by default).
	let follow = $state(true);
	let folderId = $state(0);
	let rulesId = $state(0);
	let albums = $state('all');
	let newAlbums = $state('all');
	let types = $state<string[]>(['album', 'ep']);
	let extras = $state<string[]>(['studio']);
	let statuses = $state<string[]>(['official']);
	let keep = $state(true);
	let adding = $state(false);
	let error = $state('');

	const musicFolders = $derived(folders.filter((f) => f.kind === 'music'));
	const musicRules = $derived(rules.filter((r) => !r.kinds.length || r.kinds.includes('music')));

	/** Shows the dialog for one looked-up artist. */
	export function open(a: LookupArtist) {
		follow = true;
		picked = a;
		error = '';
		folderId = musicFolders.find((f) => f.health === 'ok')?.id ?? musicFolders[0]?.id ?? 0;
		chooseRules(folderId);
		albums = defaults?.album_choice ?? 'all';
		newAlbums = defaults?.new_albums ?? 'all';
		types = [...(defaults?.album_types ?? ['album', 'ep'])];
		extras = [...(defaults?.secondary_types ?? ['studio'])];
		statuses = [...(defaults?.release_statuses ?? ['official'])];
		keep = true;
		sheetOpen = true;
	}

	function chooseRules(fid: number) {
		const own = musicFolders.find((f) => f.id === fid)?.default_pick_rules_id ?? 0;
		rulesId = pickId(own, musicRules, musicRules[0]?.id ?? 0);
	}

	async function add(e: SubmitEvent) {
		e.preventDefault();
		if (!picked) return;
		adding = true;
		error = '';
		try {
			const r = await api.post<{ title: TitleView }>('/artists', {
				id: `mbid:${picked.mbid}`,
				follow,
				library_folder_id: folderId,
				pick_rules_id: rulesId,
				keep_up_to_date: keep,
				albums,
				new_albums: newAlbums,
				album_types: types,
				secondary_types: extras,
				release_statuses: statuses
			});
			(sheetOpen = false);
			onadded(picked, r.title);
		} catch (err) {
			error = message(err);
		} finally {
			adding = false;
		}
	}
</script>

<Sheet bind:open={sheetOpen} title={picked ? `Add ${picked.name}` : 'Add'}>
	<MetadataNotice />
	{#if picked}
		<form id="add-artist-title" class="sheet-form" onsubmit={add}>
			{#if picked.disambiguation}<p class="small muted">{picked.disambiguation}</p>{/if}
			{#if musicFolders.length === 0}
				<p class="notice warn" >
					There is no Library folder for music yet. <a href={to(`/settings/library-folders`)} class="ul">Add one</a> first.
				</p>
			{:else}
				<div class="field">
					<label for="artist-folder">Library folder</label>
					<select id="artist-folder" class="input" bind:value={folderId} onchange={() => chooseRules(folderId)}>
						{#each musicFolders as f (f.id)}
							<option value={f.id}>{f.label || f.path} ({f.path})</option>
						{/each}
					</select>
				</div>
				<div class="field">
					<label for="artist-rules">Pick rules</label>
					<select id="artist-rules" class="input" bind:value={rulesId}>
						{#each musicRules as r (r.id)}
							<option value={r.id}>{r.name}</option>
						{/each}
					</select>
					<span class="hint">Which Releases count, and which one is best.</span>
				</div>
				<div class="field">
					<label for="artist-albums">Albums to keep up to date</label>
					<select id="artist-albums" class="input" bind:value={albums}>
						{#each albumChoices as [v, l] (v)}<option value={v}>{l}</option>{/each}
					</select>
					<span class="hint">{albumChoices.find(([v]) => v === albums)?.[2]}</span>
				</div>
				<div class="field">
					<label for="artist-new">New albums</label>
					<select id="artist-new" class="input" bind:value={newAlbums}>
						{#each newAlbumChoices as [v, l] (v)}<option value={v}>{l}</option>{/each}
					</select>
					<span class="hint">What happens to an album that comes out later.</span>
				</div>
				<fieldset class="kinds">
					<legend class="small"><b>Album kinds</b> <span class="hint">Albums of other kinds are listed but left out.</span></legend>
					<div class="boxes">
						{#each mainKinds as [v, l] (v)}
							<label class="box"><input type="checkbox" checked={types.includes(v)} onchange={(e) => (types = toggle(types, v, e.currentTarget.checked, mainKinds))} /> {l}</label>
						{/each}
					</div>
				</fieldset>
				<fieldset class="kinds">
					<legend class="small"><b>Extra kinds</b> <span class="hint">An album with an extra kind, such as Live, needs it ticked.</span></legend>
					<div class="boxes">
						{#each extraKinds as [v, l] (v)}
							<label class="box"><input type="checkbox" checked={extras.includes(v)} onchange={(e) => (extras = toggle(extras, v, e.currentTarget.checked, extraKinds))} /> {l}</label>
						{/each}
					</div>
				</fieldset>
				<fieldset class="kinds">
					<legend class="small"><b>Pressings that count</b></legend>
					<div class="boxes">
						{#each pressingStatuses as [v, l] (v)}
							<label class="box"><input type="checkbox" checked={statuses.includes(v)} onchange={(e) => (statuses = toggle(statuses, v, e.currentTarget.checked, pressingStatuses))} /> {l}</label>
						{/each}
					</div>
				</fieldset>
				<label class="checkline">
					<input type="checkbox" bind:checked={keep} />
					<span><b>Keep up to date</b><br /><span class="hint">HolyArrs looks after the albums picked above.</span></span>
				</label>
				<p class="hint note" data-testid="music-search-note">Albums you keep up to date are searched once their tracks are in.</p>
			{/if}
			<label class="checkline" data-testid="follow-box">
				<input type="checkbox" bind:checked={follow} />
				<span><b>Follow</b><br /><span class="hint">Show it on my calendar.</span></span>
			</label>
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
		</form>
	{/if}
	{#snippet footer()}
				<Button variant="plain" onclick={() => (sheetOpen = false)}>Cancel</Button>
				<Button variant="primary" type="submit" form="add-artist-title" disabled={musicFolders.length === 0 || !types.length || !extras.length || !statuses.length} busy={adding}>Add to Library</Button>
	{/snippet}
</Sheet>

<style>
	.sheet-form { display: flex; flex-direction: column; gap: 4px; }
	.form-error { margin: 8px 0 0; color: var(--fail); font: var(--t-callout); }
	.ul {
		color: var(--text);
		text-decoration: underline;
	}
	.kinds {
		border: 0;
		padding: 0;
		margin: var(--s-4) 0 0;
	}
	.kinds legend {
		margin-bottom: var(--s-2);
	}
	.boxes {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
		gap: var(--s-2);
	}
	.box {
		display: flex;
		align-items: center;
		gap: var(--s-2);
		min-height: var(--tap);
		font-size: var(--text-sm);
	}
	.note {
		display: block;
		margin-top: var(--s-3);
	}
</style>
