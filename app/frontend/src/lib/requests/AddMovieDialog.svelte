<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { MetadataNotice } from '#lib/metadata/index.ts';
	import { loadChoices, saveChoices, secondVersion, builtInDefaults, gateChoices as gates, startChoices, savedDefaults } from '../mediafiles/index.ts';
	import type { TitleDefaults } from '../mediafiles/index.ts';
	import { to } from '../core/paths.ts';
	import { api, message } from '../core/api.ts';
	import { plainReason } from '../core/plain.ts';
	import type { LibraryFolder, LookupMovie, PickRulesView, TitleView } from '../core/types.ts';

	// Adds a movie to the Library: folder, Pick rules, Release gate, Keep up to date, Search when added and an optional second Version. Used by Search and the title detail page; open(movie) shows it. It starts from the Movies and TV defaults.
	let {
		folders,
		rules,
		gateDefault = 'released',
		onadded
	}: {
		folders: LibraryFolder[];
		rules: PickRulesView[];
		gateDefault?: string;
		onadded: (movie: LookupMovie, title: TitleView) => void;
	} = $props();

	let addOpen = $state(false);
	let picked = $state<LookupMovie | null>(null);
	// Follow puts it on my calendar (ticked by default).
	let follow = $state(true);
	let folderId = $state(0);
	let rulesId = $state(0);
	let gate = $state('released');
	let keep = $state(true);
	let searchOnAdd = $state(true);
	// A second Version, such as 4K, next to the main one.
	let more = $state(false);
	let moreLabel = $state('4K');
	let moreRules = $state(0);
	let moreFolder = $state(0);
	let adding = $state(false);
	let addError = $state('');
	// The saved defaults once they are in; the sheet shows a placeholder until then.
	let titleDefaults = $state<TitleDefaults | null>(null);
	let ready = $state(false);

	const movieFolders = $derived(folders.filter((f) => f.kind === 'movie'));
	const movieRules = $derived(rules.filter((r) => !r.kinds.length || r.kinds.includes('movie')));

	export async function open(m: LookupMovie) {
		follow = true;
		picked = m;
		addError = '';
		ready = false;
		addOpen = true;
		const d = await savedDefaults.load();
		if (picked !== m) return;
		titleDefaults = d;
		const start = startChoices(titleDefaults, folders, rules, 'movie');
		folderId = start.folderId;
		rulesId = start.rulesId;
		keep = start.keep;
		searchOnAdd = start.searchOnAdd;
		// The Release gate this browser picked last, while it is still offered.
		const last = loadChoices();
		gate = last.release_gate && gates.some(([g]) => g === last.release_gate) ? last.release_gate : gateDefault;
		more = false;
		const sv = secondVersion(movieRules, movieFolders, folderId);
		moreLabel = sv.label;
		moreRules = sv.pick_rules_id;
		moreFolder = sv.library_folder_id;
		ready = true;
	}

	// Another folder: the saved default Pick rules, else that folder's own, else the first offered.
	function chooseRulesFor(fid: number) {
		const d = titleDefaults ?? builtInDefaults;
		rulesId = startChoices({ ...d, movie: { ...d.movie, library_folder_id: fid } }, folders, rules, 'movie').rulesId;
	}

	async function add(e: SubmitEvent) {
		e.preventDefault();
		if (!picked) return;
		adding = true;
		addError = '';
		try {
			const r = await api.post<{ title: TitleView }>('/titles', {
				id: `tmdb:${picked.ids.tmdb}`,
				follow,
				library_folder_id: folderId,
				pick_rules_id: rulesId,
				release_gate: gate,
				keep_up_to_date: keep,
				search_on_add: searchOnAdd,
				more_versions: more ? [{ label: moreLabel.trim(), pick_rules_id: moreRules, library_folder_id: moreFolder }] : []
			});
			saveChoices({ release_gate: gate });
			addOpen = false;
			onadded(picked, r.title);
		} catch (err) {
			addError = message(err);
		} finally {
			adding = false;
		}
	}
</script>

<Sheet bind:open={addOpen} title={picked ? t('requests.add.title', { name: picked.name }) : t('requests.addMovie.titleBare')}>
	<MetadataNotice />
	{#if picked && !ready}
		<div class="form-sk" aria-busy="true" aria-label={t('requests.add.loadingDefaults')}>{#each [0, 1, 2] as i (i)}<span class="sk sk-line" style="width: 30%; height: 11px"></span><span class="sk" style="height: 44px; border-radius: var(--r-btn); margin-bottom: 8px"></span>{/each}</div>
	{:else if picked}
		<form id="add-movie" class="sheet-form" onsubmit={add}>
			{#if movieFolders.length === 0}
				<p class="notice warn">
					<Words key="requests.add.noFolder.movie" links={{ a: to('/settings/library-folders') }} linkClass="ul" />
				</p>
			{:else}
				<div class="field">
					<label for="folder">{t('requests.add.folder')}</label>
					<select id="folder" class="input" bind:value={folderId} onchange={() => chooseRulesFor(folderId)}>
						{#each movieFolders as f (f.id)}
							<option value={f.id}>{f.label || f.path} ({f.path})</option>
						{/each}
					</select>
				</div>
				<div class="field">
					<label for="rules">{t('requests.add.rules')}</label>
					<select id="rules" class="input" bind:value={rulesId}>
						{#each movieRules as r (r.id)}
							<option value={r.id}>{r.name}</option>
						{/each}
					</select>
					<span class="hint">{t('requests.add.rulesHint')}</span>
				</div>
				<div class="field">
					<label for="gate">{t('requests.addMovie.gate')}</label>
					<select id="gate" class="input" bind:value={gate}>
						{#each gates as [g, label] (g)}
							<option value={g}>{label}</option>
						{/each}
					</select>
					<span class="hint">{gates.find(([g]) => g === gate)?.[2]}</span>
				</div>
				<label class="checkline">
					<input type="checkbox" bind:checked={keep} />
					<span><b>{t('requests.add.keep')}</b><br /><span class="hint">{t('requests.addMovie.keepHint')}</span></span>
				</label>
				<label class="checkline">
					<input type="checkbox" bind:checked={searchOnAdd} />
					<span><b>{t('requests.add.search')}</b><br /><span class="hint">{t('requests.addMovie.searchHint')}</span></span>
				</label>
				<label class="checkline">
					<input type="checkbox" bind:checked={more} />
					<span><b>{t('requests.add.more')}</b><br /><span class="hint">{t('requests.addMovie.moreHint')}</span></span>
				</label>
				{#if more}
					<fieldset class="more">
						<legend class="sr-only">{t('requests.add.moreLegend')}</legend>
						<div class="field">
							<label for="more-label">{t('requests.add.moreName')}</label>
							<input id="more-label" class="input" bind:value={moreLabel} maxlength="40" required />
						</div>
						<div class="field">
							<label for="more-rules">{t('requests.add.moreRules')}</label>
							<select id="more-rules" class="input" bind:value={moreRules}>
								{#each movieRules as r (r.id)}
									<option value={r.id}>{r.name}</option>
								{/each}
							</select>
						</div>
						<div class="field">
							<label for="more-folder">{t('requests.add.moreFolder')}</label>
							<select id="more-folder" class="input" bind:value={moreFolder}>
								{#each movieFolders as f (f.id)}
									<option value={f.id}>{f.label || f.path} ({f.path})</option>
								{/each}
							</select>
							{#if moreFolder === folderId}<span class="hint">{t('requests.addMovie.sameFolder', { label: moreLabel.trim() || '4K' })}</span>{/if}
						</div>
					</fieldset>
				{/if}
			{/if}
			<label class="checkline" data-testid="follow-box">
				<input type="checkbox" bind:checked={follow} />
				<span><b>{t('requests.add.follow')}</b><br /><span class="hint">{t('requests.add.followHint')}</span></span>
			</label>
			{#if addError}<p class="form-error" role="alert">{plainReason(addError).text}</p>{/if}
		</form>
	{/if}
	{#snippet footer()}
		<Button variant="plain" onclick={() => (addOpen = false)}>{t('requests.add.cancel')}</Button>
		<Button variant="primary" type="submit" form="add-movie" disabled={movieFolders.length === 0 || !ready} busy={adding}>{t('requests.add.submit')}</Button>
	{/snippet}
</Sheet>

<style>
	.sheet-form { display: flex; flex-direction: column; gap: 4px; }
	.notice { display: flex; gap: 8px; margin: 0; padding: 10px 12px; border: 0; border-radius: var(--r-btn); background: var(--surface-2); font: var(--t-callout); }
	.notice.warn { color: var(--text); }
	.form-error { margin: 8px 0 0; color: var(--fail); font: var(--t-callout); }
	.more { margin: 12px 0 0; padding: 12px 0 0 16px; border: 0; border-left: 2px solid var(--line-strong); }
	.notice :global(.ul) { text-decoration: underline; color: var(--text); }
	.form-sk { display: flex; flex-direction: column; gap: 8px; }
</style>
