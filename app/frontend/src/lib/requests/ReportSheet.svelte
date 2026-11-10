<script lang="ts">
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { episodeWord, hasFile, kindHint, kindIcon, makeReport, noteMax, partWords, reportKinds, type ReportKind, type ReportView } from './reports.ts';
	import { seasonName } from '#lib/shows/index.ts';
	import { untrack } from 'svelte';
	import { fade } from 'svelte/transition';
	import { api, message } from '#lib/core/api.ts';
	import { to } from '#lib/core/paths.ts';

	// Report a problem with a title in the Library: what is wrong, and for a show the season or episode, for an album the track, and the Version when there is more than one. Centred on wide screens, a bottom sheet on phones.
	type Part = { id: number; episode?: number; number?: number; disc?: number; name?: string; slots?: { file?: unknown; file_id?: number; state?: string }[] };
	let {
		open = $bindable(false),
		title,
		album = null,
		season = null,
		itemId = null,
		onsent
	}: {
		open?: boolean;
		title: { id: number; name: string; kind: string; versions?: { id: number; label: string }[]; show?: { seasons: { number: number; name?: string; in_library: number }[] } | null };
		/** The album page: its tracks to pick from. */
		album?: { id: number; name: string; tracks: Part[] } | null;
		/** A season or episode to start on. */
		season?: number | null;
		itemId?: number | null;
		onsent?: (r: ReportView) => void;
	} = $props();

	const id = $props.id();
	let kind = $state<ReportKind | ''>('');
	let note = $state('');
	let pickSeason = $state('');
	let pickItem = $state('');
	let pickVersion = $state('');
	let episodes = $state<Part[]>([]);
	let loadingEps = $state(false);
	let busy = $state(false);
	let error = $state('');
	let sent = $state<ReportView | null>(null);

	const seasons = $derived((title.show?.seasons ?? []).filter((s) => s.in_library > 0).sort((a, b) => (a.number || 1e6) - (b.number || 1e6)));
	const versions = $derived(title.versions ?? []);
	const tracks = $derived((album?.tracks ?? []).filter(hasFile));
	const discs = $derived(new Set(tracks.map((tr) => tr.disc ?? 1)).size);
	const what = $derived(album ? album.name : title.name);

	// Each opening starts fresh, on the season or episode it was opened from.
	$effect(() => {
		if (!open) return;
		untrack(() => {
			kind = '';
			note = '';
			error = '';
			sent = null;
			pickVersion = '';
			pickSeason = season !== null && season !== undefined ? String(season) : '';
			pickItem = itemId ? String(itemId) : '';
			episodes = [];
			if (title.show && pickSeason) void loadEpisodes(Number(pickSeason));
		});
	});

	async function loadEpisodes(n: number) {
		loadingEps = true;
		try {
			const r = await api.get<{ episodes: Part[] | null }>(`/titles/${title.id}/episodes?season=${n}&files=1`);
			if (String(n) !== pickSeason) return;
			const all = r.episodes ?? [];
			const with_ = all.filter(hasFile);
			episodes = with_.length ? with_ : all;
		} catch {
			episodes = [];
		} finally {
			loadingEps = false;
		}
	}
	function seasonPicked() {
		pickItem = '';
		episodes = [];
		if (pickSeason) void loadEpisodes(Number(pickSeason));
	}
	function trackWord(tr: Part): string {
		const n = String(tr.number ?? 0).padStart(2, '0');
		return `${discs > 1 ? `${tr.disc ?? 1}-` : ''}${n}${tr.name ? ` · ${tr.name}` : ''}`;
	}

	async function send() {
		if (!kind) {
			error = t('requests.reportSheet.pickFirst');
			return;
		}
		busy = true;
		error = '';
		try {
			sent = await makeReport({
				title_id: title.id,
				kind,
				note,
				season: title.show && pickSeason !== '' ? Number(pickSeason) : null,
				item_id: pickItem ? Number(pickItem) : null,
				album_id: album?.id ?? null,
				version_id: pickVersion ? Number(pickVersion) : null
			});
			onsent?.(sent);
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
		}
	}
</script>

<Sheet bind:open title={sent ? t('requests.reportSheet.sentTitle') : t('requests.reportSheet.title')} description={sent ? undefined : what} size="sm" placement="auto">
	{#if sent}
		<div class="rs-done" data-testid="report-sent">
			<span class="rs-ok" aria-hidden="true"><Icon name="check" size={26} /></span>
			<p class="rs-what"><span class="rs-tag"><Icon name={kindIcon(sent.kind)} size={13} />{serverWords(sent.kind_label)}</span>{#if partWords(sent)}<span>{partWords(sent)}</span>{/if}</p>
			<p>{t('requests.reportSheet.thanks')}</p>
		</div>
	{:else}
		<form class="rs" data-testid="report-sheet" onsubmit={(e) => { e.preventDefault(); void send(); }}>
			<fieldset class="rs-kinds">
				<legend>{t('requests.reportSheet.what')}</legend>
				<div class="rs-tiles">
					{#each reportKinds as k (k.kind)}
						<label class="rs-tile" class:on={kind === k.kind} title={k.hint}>
							<input type="radio" name="{id}-kind" value={k.kind} bind:group={kind} />
							<Icon name={kindIcon(k.kind)} size={20} />
							<span>{k.label}</span>
						</label>
					{/each}
				</div>
				<p class="rs-hint" aria-live="polite">{#key kind}<span in:fade={{ duration: 180 }}>{kind ? kindHint(kind) : t('requests.reportSheet.closest')}</span>{/key}</p>
			</fieldset>

			{#if title.show && seasons.length}
				<div class="rs-row">
					<label class="rs-field">
						<span>{t('requests.reportSheet.season')}</span>
						<select bind:value={pickSeason} onchange={seasonPicked}>
							<option value="">{t('requests.reportSheet.wholeShow')}</option>
							{#each seasons as s (s.number)}<option value={String(s.number)}>{s.name || seasonName(s.number)}</option>{/each}
						</select>
					</label>
					{#if pickSeason !== ''}
						<label class="rs-field">
							<span>{t('requests.reportSheet.episode')}</span>
							<select bind:value={pickItem} disabled={loadingEps} aria-busy={loadingEps}>
								<option value="">{loadingEps ? t('requests.reportSheet.loading') : t('requests.reportSheet.wholeSeason')}</option>
								{#each episodes as e (e.id)}<option value={String(e.id)}>{episodeWord({ episode: e.episode ?? 0, name: e.name })}</option>{/each}
							</select>
						</label>
					{/if}
				</div>
			{/if}

			{#if album && tracks.length}
				<label class="rs-field">
					<span>{t('requests.reportSheet.track')}</span>
					<select bind:value={pickItem}>
						<option value="">{t('requests.reportSheet.wholeAlbum')}</option>
						{#each tracks as tr (tr.id)}<option value={String(tr.id)}>{trackWord(tr)}</option>{/each}
					</select>
				</label>
			{/if}

			{#if versions.length > 1}
				<label class="rs-field">
					<span>{t('requests.reportSheet.version')}</span>
					<select bind:value={pickVersion}>
						<option value="">{t('requests.reportSheet.notSure')}</option>
						{#each versions as v (v.id)}<option value={String(v.id)}>{v.label}</option>{/each}
					</select>
				</label>
			{/if}

			<label class="rs-field">
				<span>{t('requests.reportSheet.else')} <small>{t('requests.reportSheet.optional')}</small></span>
				<textarea bind:value={note} rows="3" maxlength={noteMax} placeholder={t('requests.reportSheet.placeholder')}></textarea>
				{#if note.length > noteMax - 60}<small class="rs-count">{t('requests.reportSheet.left', { n: noteMax - note.length })}</small>{/if}
			</label>

			{#if error}<p class="rs-err" role="alert">{error}</p>{/if}
		</form>
	{/if}
	{#snippet footer()}
		{#if sent}
			<Button variant="plain" href={to('/requests/reports')}>{t('requests.reportSheet.seeReports')}</Button>
			<Button variant="primary" onclick={() => (open = false)}>{t('requests.reportSheet.done')}</Button>
		{:else}
			<Button variant="plain" onclick={() => (open = false)}>{t('requests.add.cancel')}</Button>
			<Button variant="primary" {busy} disabled={!kind} onclick={() => void send()}>{t('requests.reportSheet.send')}</Button>
		{/if}
	{/snippet}
</Sheet>

<style>
	.rs { display: grid; gap: 14px; min-width: 0; }
	.rs-kinds { margin: 0; padding: 0; border: 0; min-width: 0; }
	.rs-kinds legend { margin-bottom: 8px; padding: 0; font: 600 15px/1.2 var(--font-ui); }
	.rs-tiles { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 8px; }
	.rs-tile { position: relative; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 7px; min-height: 74px; padding: 10px 2px 9px; border-radius: 14px; background: var(--surface-2); box-shadow: inset 0 0 0 1px var(--line); color: var(--text-muted); font: 600 12px/1.15 var(--font-ui); text-align: center; cursor: pointer; transition: transform 160ms var(--ease-out), background-color 200ms var(--ease-out), color 200ms var(--ease-out), box-shadow 200ms var(--ease-out); }
	.rs-tile input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; }
	.rs-tile > span { color: var(--text); transition: color 200ms var(--ease-out); }
	.rs-tile:hover:not(.on) { background: var(--surface-3); color: var(--text); }
	.rs-tile:active { transform: scale(0.96); }
	.rs-tile.on { background: var(--text); box-shadow: 0 8px 18px -10px rgb(0 0 0 / 0.5); color: var(--bg); }
	.rs-tile.on > span { color: var(--bg); }
	.rs-tile:has(input:focus-visible) { box-shadow: var(--focus-ring); }
	.rs-hint { min-height: 18px; margin: 9px 0 0; color: var(--text-muted); font: 500 13px/1.35 var(--font-ui); }
	.rs-row { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 10px; }
	.rs-field { display: grid; gap: 6px; min-width: 0; }
	.rs-field > span { font: 600 14px/1.2 var(--font-ui); }
	.rs-field small { color: var(--text-muted); font-weight: 500; }
	.rs-field select, .rs-field textarea { width: 100%; min-width: 0; padding: 0 12px; border: 1px solid color-mix(in srgb, var(--text) 14%, transparent); border-radius: 10px; background: var(--surface-1, var(--bg)); color: var(--text); font: 500 14px/1.4 var(--font-ui); }
	.rs-field select { height: 40px; }
	.rs-field textarea { padding: 10px 12px; resize: vertical; min-height: 76px; }
	.rs-count { justify-self: end; }
	.rs-err { margin: 0; color: var(--fail-text, #e5484d); font: 500 14px/1.4 var(--font-ui); }
	.rs-done { display: grid; justify-items: center; gap: 12px; padding: 6px 0 2px; text-align: center; }
	.rs-done p { margin: 0; max-width: 40ch; font: var(--t-callout); }
	.rs-ok { display: grid; place-items: center; width: 54px; height: 54px; border-radius: 50%; background: var(--ok); color: #fff; box-shadow: 0 12px 26px -12px color-mix(in srgb, var(--ok) 85%, transparent); animation: rs-pop 420ms cubic-bezier(0.34, 1.56, 0.64, 1) both; }
	.rs-ok :global(svg) { animation: rs-tick 300ms var(--ease-out) 160ms both; }
	@keyframes rs-pop { from { opacity: 0; transform: scale(0.6); } }
	@keyframes rs-tick { from { opacity: 0; transform: scale(0.7); } }
	.rs-what { display: flex; flex-wrap: wrap; align-items: center; justify-content: center; gap: 6px 10px; color: var(--text-muted); }
	.rs-tag { display: inline-flex; align-items: center; gap: 5px; height: 26px; padding: 0 11px 0 9px; border-radius: 999px; background: var(--surface-2); box-shadow: inset 0 0 0 1px var(--line); color: var(--text); font: 600 13px/1 var(--font-ui); }
	@media (pointer: coarse) { .rs-field select { height: 44px; } }
	@media (max-width: 400px) { .rs-tiles { gap: 6px; } .rs-tile { font-size: 11.5px; } }
	@media (prefers-reduced-motion: reduce) { .rs-ok, .rs-ok :global(svg) { animation: none; } .rs-tile { transition: none; } .rs-tile:active { transform: none; } }
</style>
