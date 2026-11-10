<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { MetadataNotice } from '#lib/metadata/index.ts';
	import { secondVersion, builtInDefaults, startChoices, savedDefaults } from '../mediafiles/index.ts';
	import { detailsSources, episodeOrders, seasonChoices } from '../shows/index.ts';
	import type { ShowSettings } from '../shows/index.ts';
	import type { TitleDefaults } from '../mediafiles/index.ts';
	import { to } from '../core/paths.ts';
	import { api, message } from '../core/api.ts';
	import type { LibraryFolder, PickRulesView, TitleView } from '../core/types.ts';
	import type { LookupShow } from '../core/types.ts';

	// The add-a-show dialog: Library folder, Pick rules, which seasons to keep up to date, new seasons, episode order and details source. It starts from the Movies and TV defaults and the show settings.

	let {
		folders,
		rules,
		defaults,
		onadded
	}: {
		folders: LibraryFolder[];
		rules: PickRulesView[];
		defaults: ShowSettings | null;
		onadded: (show: LookupShow, title: TitleView) => void;
	} = $props();

	let sheetOpen = $state(false);
	let picked = $state<LookupShow | null>(null);
	// Follow puts it on my calendar (ticked by default).
	let follow = $state(true);
	let folderId = $state(0);
	let rulesId = $state(0);
	let seasons = $state('all');
	let newSeasons = $state('all');
	let order = $state('aired');
	let source = $state('tmdb');
	let keep = $state(true);
	let searchOnAdd = $state(false);
	// A second Version, such as 4K, next to the main one.
	let more = $state(false);
	let moreLabel = $state('4K');
	let moreRules = $state(0);
	let moreFolder = $state(0);
	let adding = $state(false);
	let error = $state('');
	// The saved defaults once they are in; the sheet shows a placeholder until then.
	let titleDefaults = $state<TitleDefaults | null>(null);
	let ready = $state(false);

	const showFolders = $derived(folders.filter((f) => f.kind === 'show'));
	const showRules = $derived(rules.filter((r) => !r.kinds.length || r.kinds.includes('show')));

	/** Shows the dialog for one looked-up show. */
	export async function open(s: LookupShow) {
		follow = true;
		picked = s;
		error = '';
		ready = false;
		sheetOpen = true;
		const d = await savedDefaults.load();
		if (picked !== s) return;
		titleDefaults = d;
		const start = startChoices(titleDefaults, folders, rules, 'show');
		folderId = start.folderId;
		rulesId = start.rulesId;
		keep = start.keep;
		searchOnAdd = start.searchOnAdd;
		seasons = defaults?.season_choice ?? 'all';
		newSeasons = defaults?.new_seasons ?? 'all';
		order = defaults?.episode_order ?? 'aired';
		source = s.source || defaults?.details_source || 'tmdb';
		more = false;
		const sv = secondVersion(showRules, showFolders, folderId);
		moreLabel = sv.label;
		moreRules = sv.pick_rules_id;
		moreFolder = sv.library_folder_id;
		ready = true;
	}

	// Another folder: the saved default Pick rules, else that folder's own, else the first offered.
	function chooseRules(fid: number) {
		const d = titleDefaults ?? builtInDefaults;
		rulesId = startChoices({ ...d, show: { ...d.show, library_folder_id: fid } }, folders, rules, 'show').rulesId;
	}

	/** Picks the id to add by: the source's own id first. */
	function idOf(s: LookupShow): string {
		for (const k of [s.source, 'tmdb', 'tvdb', 'tvmaze', 'imdb']) if (s.ids[k]) return `${k}:${s.ids[k]}`;
		return '';
	}

	async function add(e: SubmitEvent) {
		e.preventDefault();
		if (!picked) return;
		adding = true;
		error = '';
		try {
			const r = await api.post<{ title: TitleView }>('/shows', {
				id: idOf(picked),
				follow,
				library_folder_id: folderId,
				pick_rules_id: rulesId,
				keep_up_to_date: keep,
				search_on_add: searchOnAdd,
				seasons,
				new_seasons: seasons === 'future' ? 'all' : newSeasons,
				episode_order: order,
				details_source: source,
				more_versions: more ? [{ label: moreLabel.trim(), pick_rules_id: moreRules, library_folder_id: moreFolder }] : []
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

<Sheet bind:open={sheetOpen} title={picked ? t('requests.add.title', { name: picked.name }) : t('requests.add.titleBare')}>
	<MetadataNotice />
	{#if picked && !ready}
		<div class="form-sk" aria-busy="true" aria-label={t('requests.add.loadingDefaults')}>{#each [0, 1, 2] as i (i)}<span class="sk sk-line" style="width: 30%; height: 11px"></span><span class="sk" style="height: 44px; border-radius: var(--r-btn); margin-bottom: 8px"></span>{/each}</div>
	{:else if picked}
		<form id="add-show-title" class="sheet-form" onsubmit={add}>
			{#if showFolders.length === 0}
				<p class="notice warn" >
					<Words key="requests.add.noFolder.show" links={{ a: to('/settings/library-folders') }} linkClass="ul" />
				</p>
			{:else}
				<div class="field">
					<label for="show-folder">{t('requests.add.folder')}</label>
					<select id="show-folder" class="input" bind:value={folderId} onchange={() => chooseRules(folderId)}>
						{#each showFolders as f (f.id)}
							<option value={f.id}>{f.label || f.path} ({f.path})</option>
						{/each}
					</select>
				</div>
				<div class="field">
					<label for="show-rules">{t('requests.add.rules')}</label>
					<select id="show-rules" class="input" bind:value={rulesId}>
						{#each showRules as r (r.id)}
							<option value={r.id}>{r.name}</option>
						{/each}
					</select>
					<span class="hint">{t('requests.add.rulesHint')}</span>
				</div>
				<div class="field">
					<label for="show-seasons">{t('requests.addShow.seasons')}</label>
					<select id="show-seasons" class="input" bind:value={seasons}>
						{#each seasonChoices as [v, l] (v)}<option value={v}>{l}</option>{/each}
					</select>
					<span class="hint">{seasonChoices.find(([v]) => v === seasons)?.[2]}</span>
				</div>
				<div class="field">
					<label for="show-new">{t('requests.addShow.newSeasons')}</label>
					<select id="show-new" class="input" bind:value={newSeasons} disabled={seasons === 'future'}>
						<option value="all">{t('requests.addShow.newKeep')}</option>
						<option value="none">{t('requests.addShow.newOff')}</option>
					</select>
					<span class="hint">{seasons === 'future' ? t('requests.addShow.futureHint') : t('requests.addShow.newHint')}</span>
				</div>
				<div class="field">
					<label for="show-order">{t('requests.addShow.order')}</label>
					<select id="show-order" class="input" bind:value={order}>
						{#each episodeOrders as [v, l] (v)}<option value={v}>{l}</option>{/each}
					</select>
					<span class="hint">{episodeOrders.find(([v]) => v === order)?.[2]}</span>
				</div>
				<div class="field">
					<label for="show-source">{t('requests.addShow.source')}</label>
					<select id="show-source" class="input" bind:value={source}>
						{#each detailsSources as [v, l] (v)}<option value={v}>{l}</option>{/each}
					</select>
					<span class="hint">{t('requests.addShow.sourceHint')}</span>
				</div>
				<label class="checkline">
					<input type="checkbox" bind:checked={keep} />
					<span><b>{t('requests.add.keep')}</b><br /><span class="hint">{t('requests.addShow.keepHint')}</span></span>
				</label>
				<label class="checkline">
					<input type="checkbox" bind:checked={searchOnAdd} />
					<span><b>{t('requests.add.search')}</b><br /><span class="hint">{t('requests.addShow.searchHint')}</span></span>
				</label>
				<label class="checkline">
					<input type="checkbox" bind:checked={more} />
					<span><b>{t('requests.add.more')}</b><br /><span class="hint">{t('requests.addShow.moreHint')}</span></span>
				</label>
				{#if more}
					<fieldset class="more">
						<legend class="sr-only">{t('requests.add.moreLegend')}</legend>
						<div class="field">
							<label for="show-more-label">{t('requests.add.moreName')}</label>
							<input id="show-more-label" class="input" bind:value={moreLabel} maxlength="40" required />
						</div>
						<div class="field">
							<label for="show-more-rules">{t('requests.add.moreRules')}</label>
							<select id="show-more-rules" class="input" bind:value={moreRules}>
								{#each showRules as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
							</select>
						</div>
						<div class="field">
							<label for="show-more-folder">{t('requests.add.moreFolder')}</label>
							<select id="show-more-folder" class="input" bind:value={moreFolder}>
								{#each showFolders as f (f.id)}<option value={f.id}>{f.label || f.path} ({f.path})</option>{/each}
							</select>
							{#if moreFolder === folderId}<span class="hint">{t('requests.addShow.sameFolder', { label: moreLabel.trim() || '4K' })}</span>{/if}
						</div>
					</fieldset>
				{/if}
				{#if !searchOnAdd}<p class="hint" data-testid="no-show-search">{t('requests.addShow.noSearch')}</p>{/if}
			{/if}
			<label class="checkline" data-testid="follow-box">
				<input type="checkbox" bind:checked={follow} />
				<span><b>{t('requests.add.follow')}</b><br /><span class="hint">{t('requests.add.followHint')}</span></span>
			</label>
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
		</form>
	{/if}
	{#snippet footer()}
				<Button variant="plain" onclick={() => (sheetOpen = false)}>{t('requests.add.cancel')}</Button>
				<Button variant="primary" type="submit" form="add-show-title" disabled={showFolders.length === 0 || !ready} busy={adding}>{t('requests.add.submit')}</Button>
	{/snippet}
</Sheet>

<style>
	.sheet-form { display: flex; flex-direction: column; gap: 4px; }
	.form-error { margin: 8px 0 0; color: var(--fail); font: var(--t-callout); }
	.notice :global(.ul) {
		color: var(--text);
		text-decoration: underline;
	}
	.more {
		margin: var(--s-3) 0 0;
		padding: var(--s-3) 0 0 var(--s-4);
		border: 0;
		border-left: 2px solid var(--line-strong);
	}
	.hint[data-testid='no-show-search'] {
		display: block;
		margin-top: var(--s-3);
	}
	.form-sk { display: flex; flex-direction: column; gap: 8px; }
</style>
