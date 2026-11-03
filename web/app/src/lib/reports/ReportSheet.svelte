<script lang="ts">
	import { untrack } from 'svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { api, message } from '#lib/api.ts';
	import { to } from '#lib/paths.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { seasonName } from '#lib/shows.ts';
	import { episodeWord, hasFile, makeReport, noteMax, reportKinds, type ReportKind, type ReportView } from '#lib/reports.ts';

	// Report a problem (#95) with a title in the Library: what is wrong, and
	// for a show the season or episode, for an album the track, and the
	// Version when there is more than one. Centred on wide screens, a bottom
	// sheet on phones. The owner and admins see it on /reports.
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
	const discs = $derived(new Set(tracks.map((t) => t.disc ?? 1)).size);
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
	function trackWord(t: Part): string {
		const n = String(t.number ?? 0).padStart(2, '0');
		return `${discs > 1 ? `${t.disc ?? 1}-` : ''}${n}${t.name ? ` · ${t.name}` : ''}`;
	}

	async function send() {
		if (!kind) {
			error = 'Pick what is wrong first.';
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
			toasts.push('Thanks, your report was sent.', 'ok');
			onsent?.(sent);
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
		}
	}
</script>

<Sheet bind:open title={sent ? 'Report sent' : 'Report a problem'} description={sent ? undefined : what} size="sm" placement="auto">
	{#if sent}
		<div class="rs-done" data-testid="report-sent">
			<p>Thanks. The owner and admins can see your report now, and you get a message when it is fixed.</p>
			<p class="rs-what"><b>{sent.kind_label}</b>{#if sent.where}{' · '}{sent.where}{/if}</p>
		</div>
	{:else}
		<form class="rs" data-testid="report-sheet" onsubmit={(e) => { e.preventDefault(); void send(); }}>
			<fieldset class="rs-kinds">
				<legend>What is wrong?</legend>
				<div class="rs-chips">
					{#each reportKinds as k (k.kind)}
						<label class="rs-chip" class:on={kind === k.kind} title={k.hint}>
							<input type="radio" name="{id}-kind" value={k.kind} bind:group={kind} />
							<span>{k.label}</span>
						</label>
					{/each}
				</div>
				{#if kind}<p class="rs-hint">{reportKinds.find((k) => k.kind === kind)?.hint}</p>{/if}
			</fieldset>

			{#if title.show && seasons.length}
				<div class="rs-row">
					<label class="rs-field">
						<span>Season</span>
						<select bind:value={pickSeason} onchange={seasonPicked}>
							<option value="">The whole show</option>
							{#each seasons as s (s.number)}<option value={String(s.number)}>{s.name || seasonName(s.number)}</option>{/each}
						</select>
					</label>
					{#if pickSeason !== ''}
						<label class="rs-field">
							<span>Episode</span>
							<select bind:value={pickItem} disabled={loadingEps} aria-busy={loadingEps}>
								<option value="">{loadingEps ? 'Loading…' : 'The whole season'}</option>
								{#each episodes as e (e.id)}<option value={String(e.id)}>{episodeWord({ episode: e.episode ?? 0, name: e.name })}</option>{/each}
							</select>
						</label>
					{/if}
				</div>
			{/if}

			{#if album && tracks.length}
				<label class="rs-field">
					<span>Track</span>
					<select bind:value={pickItem}>
						<option value="">The whole album</option>
						{#each tracks as t (t.id)}<option value={String(t.id)}>{trackWord(t)}</option>{/each}
					</select>
				</label>
			{/if}

			{#if versions.length > 1}
				<label class="rs-field">
					<span>Version</span>
					<select bind:value={pickVersion}>
						<option value="">Not sure</option>
						{#each versions as v (v.id)}<option value={String(v.id)}>{v.label}</option>{/each}
					</select>
				</label>
			{/if}

			<label class="rs-field">
				<span>Anything else? <small>(optional)</small></span>
				<textarea bind:value={note} rows="3" maxlength={noteMax} placeholder="For example: the sound drifts after about 20 minutes"></textarea>
				{#if note.length > noteMax - 60}<small class="rs-count">{noteMax - note.length} left</small>{/if}
			</label>

			{#if error}<p class="rs-err" role="alert">{error}</p>{/if}
		</form>
	{/if}
	{#snippet footer()}
		{#if sent}
			<Button variant="plain" href={to('/reports')}>See your reports</Button>
			<Button variant="primary" onclick={() => (open = false)}>Done</Button>
		{:else}
			<Button variant="plain" onclick={() => (open = false)}>Cancel</Button>
			<Button variant="primary" {busy} disabled={!kind} onclick={() => void send()}>Send report</Button>
		{/if}
	{/snippet}
</Sheet>

<style>
	.rs { display: grid; gap: 14px; min-width: 0; }
	.rs-kinds { margin: 0; padding: 0; border: 0; min-width: 0; }
	.rs-kinds legend { margin-bottom: 8px; font: 600 15px/1.2 var(--font-ui); }
	.rs-chips { display: flex; flex-wrap: wrap; gap: 8px; }
	.rs-chip { position: relative; display: inline-flex; align-items: center; min-height: 40px; padding: 0 14px; border-radius: 999px; border: 1px solid var(--line-strong, var(--line)); background: var(--surface-2); color: var(--text); font: 600 14px/1 var(--font-ui); cursor: pointer; }
	.rs-chip input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; }
	.rs-chip.on { background: var(--text); border-color: var(--text); color: var(--bg); }
	.rs-chip:has(input:focus-visible) { box-shadow: var(--focus-ring); }
	.rs-hint { margin: 8px 0 0; color: var(--text-muted); font: 500 13px/1.35 var(--font-ui); }
	.rs-row { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 10px; }
	.rs-field { display: grid; gap: 6px; min-width: 0; }
	.rs-field > span { font: 600 14px/1.2 var(--font-ui); }
	.rs-field small { color: var(--text-muted); font-weight: 500; }
	.rs-field select, .rs-field textarea { width: 100%; min-width: 0; padding: 0 12px; border: 1px solid color-mix(in srgb, var(--text) 14%, transparent); border-radius: 10px; background: var(--surface-1, var(--bg)); color: var(--text); font: 500 14px/1.4 var(--font-ui); }
	.rs-field select { height: 40px; }
	.rs-field textarea { padding: 10px 12px; resize: vertical; min-height: 76px; }
	.rs-count { justify-self: end; }
	.rs-err { margin: 0; color: var(--fail-text, #e5484d); font: 500 14px/1.4 var(--font-ui); }
	.rs-done { display: grid; gap: 10px; }
	.rs-done p { margin: 0; font: var(--t-callout); }
	.rs-what { color: var(--text-muted); }
	@media (pointer: coarse) { .rs-chip { min-height: 44px; } .rs-field select { height: 44px; } }
</style>
