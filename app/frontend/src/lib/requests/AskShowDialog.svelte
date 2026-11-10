<script lang="ts">
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	// A member asks for a show: all seasons, the seasons they pick, or the future seasons only (what airs from now on).
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { seasonName, seasonWords } from '../shows/index.ts';
	import { api, message, ApiError } from '../core/api.ts';
	import type { Ask } from '../core/types.ts';
	import type { LookupShow } from '../core/types.ts';

	let {
		onasked,
		onrefused
	}: {
		onasked: (show: LookupShow, r: { ask: Ask; joined: boolean; already: boolean }) => void;
		onrefused?: (show: LookupShow, body: Record<string, unknown>) => void;
	} = $props();

	let sheetOpen = $state(false);
	let picked = $state<LookupShow | null>(null);
	// Follow puts it on my calendar (ticked by default).
	let follow = $state(true);
	let mode = $state<'all' | 'choose' | 'future'>('all');
	let chosen = $state<number[]>([]);
	// Single episodes picked, by season (seasons not ticked whole).
	let epPicks = $state<Record<number, number[]>>({});
	let openSeason = $state<number | null>(null);
	type Ep = { number: number; name?: string };
	let eps = $state<Record<number, Ep[] | 'error'>>({});
	const pickedEps = $derived(Object.entries(epPicks).filter(([, l]) => l.length).map(([k, l]) => ({ season: Number(k), episodes: [...l].sort((a, b) => a - b) })).sort((a, b) => a.season - b.season));
	const epCount = $derived(pickedEps.reduce((n, e) => n + e.episodes.length, 0));
	let busy = $state(false);
	let error = $state('');

	const seasons = $derived((picked?.seasons ?? []).map((s) => s.number).sort((a, b) => a - b));

	/** Shows the dialog for one looked-up show. */
	export function open(s: LookupShow) {
		follow = true;
		picked = s;
		mode = 'all';
		chosen = [];
		epPicks = {};
		openSeason = null;
		eps = {};
		error = '';
		sheetOpen = true;
	}

	function idOf(s: LookupShow): string {
		for (const k of [s.source, 'tmdb', 'tvdb', 'tvmaze', 'imdb']) if (s.ids[k]) return `${k}:${s.ids[k]}`;
		return '';
	}

	function toggle(n: number, on: boolean) {
		chosen = on ? [...chosen, n].sort((a, b) => a - b) : chosen.filter((x) => x !== n);
		if (on) epPicks = { ...epPicks, [n]: [] };
	}

	function toggleEp(season: number, n: number, on: boolean) {
		const cur = epPicks[season] ?? [];
		epPicks = { ...epPicks, [season]: on ? [...cur, n] : cur.filter((x) => x !== n) };
	}

	function expand(n: number) {
		openSeason = openSeason === n ? null : n;
		if (openSeason === null || eps[n] || !picked) return;
		const id = picked.ids.tmdb;
		if (!id) {
			eps[n] = 'error';
			return;
		}
		api.get<{ episodes?: Ep[] }>(`/metadata/show/${encodeURIComponent(id)}/seasons/${n}`)
			.then((r) => (eps[n] = r.episodes ?? []))
			.catch(() => (eps[n] = 'error'));
	}

	const summary = $derived([chosen.length ? seasonWords(chosen) : '', epCount ? t(epCount === 1 ? 'requests.parts.episodes.one' : 'requests.parts.episodes.other', { n: epCount }) : ''].filter(Boolean).join(' + '));

	async function ask(e: SubmitEvent) {
		e.preventDefault();
		if (!picked) return;
		busy = true;
		error = '';
		try {
			const r = await api.post<{ ask: Ask; joined: boolean; already: boolean }>('/asks', mode === 'future' ? { kind: 'show', id: idOf(picked), future_only: true, follow } : { kind: 'show', id: idOf(picked), seasons: mode === 'all' ? [] : chosen, ...(mode === 'choose' && pickedEps.length ? { episodes: pickedEps } : {}), follow });
			(sheetOpen = false);
			onasked(picked, r);
		} catch (err) {
			error = message(err);
			if (err instanceof ApiError && err.status === 409) onrefused?.(picked, err.body);
		} finally {
			busy = false;
		}
	}
</script>

<Sheet bind:open={sheetOpen} title={picked ? t('requests.ask.title', { name: picked.name }) : t('requests.ask.titleBare')}>
	{#if picked}
		<form id="ask-show-title" class="sheet-form" onsubmit={ask}>
			{#if picked.ask}
				<p class="small muted" style="margin-top: var(--s-2)">{t('requests.askShow.already', { what: seasonWords(picked.ask.seasons), state: serverWords(picked.ask.state_label) })}</p>
			{/if}
			<fieldset class="pick">
				<legend class="small">{t('requests.ask.what')}</legend>
				<label class="checkline">
					<input type="radio" name="what" checked={mode === 'all'} onchange={() => (mode = 'all')} />
					<span><b>{t('requests.askShow.all')}</b><br /><span class="hint">{t('requests.askShow.allHint')}</span></span>
				</label>
				<label class="checkline">
					<input type="radio" name="what" checked={mode === 'choose'} onchange={() => (mode = 'choose')} disabled={seasons.length === 0} />
					<span><b>{t('requests.askShow.choose')}</b><br /><span class="hint">{t('requests.askShow.chooseHint')}</span></span>
				</label>
				<label class="checkline">
					<input type="radio" name="what" checked={mode === 'future'} onchange={() => (mode = 'future')} />
					<span><b>{t('requests.askShow.future')}</b><br /><span class="hint">{t('requests.askShow.futureHint')}</span></span>
				</label>
			</fieldset>
			{#if mode === 'choose'}
				<fieldset class="seasons" data-testid="ask-seasons">
					<legend class="sr-only">{t('requests.askShow.legend')}</legend>
					{#each seasons as n (n)}
						{@const whole = chosen.includes(n)}
						{@const list = eps[n]}
						<div class="row" class:on={whole || (epPicks[n]?.length ?? 0) > 0}>
							<label class="pickrow">
								<input type="checkbox" checked={whole} onchange={(e) => toggle(n, e.currentTarget.checked)} />
								<span class="nm">{seasonName(n)}</span>
								{#if epPicks[n]?.length}<span class="xs muted">{t('requests.askShow.picked', { n: epPicks[n].length })}</span>{/if}
							</label>
							<button type="button" class="more" aria-expanded={openSeason === n} aria-label={t('requests.askShow.episodesOf', { season: seasonName(n) })} disabled={whole} onclick={() => expand(n)}>{openSeason === n ? t('requests.askShow.hide') : t('requests.askShow.episodes')}</button>
						</div>
						{#if openSeason === n && !whole}
							{#if list === 'error'}
								<p class="small muted sub">{t('requests.askShow.readError')}</p>
							{:else if !list}
								<p class="small muted sub">{t('requests.askShow.reading')}</p>
							{:else if list.length === 0}
								<p class="small muted sub">{t('requests.askShow.none')}</p>
							{:else}
								{#each list as ep (ep.number)}
									<div class="row sub" class:on={epPicks[n]?.includes(ep.number)}>
										<label class="pickrow">
											<input type="checkbox" checked={epPicks[n]?.includes(ep.number) ?? false} onchange={(e) => toggleEp(n, ep.number, e.currentTarget.checked)} />
											<span class="nm">{ep.number}. {ep.name || t('requests.askShow.episodeN', { n: ep.number })}</span>
										</label>
									</div>
								{/each}
							{/if}
						{/if}
					{/each}
				</fieldset>
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
				<Button variant="primary" type="submit" form="ask-show-title" disabled={mode === 'choose' && chosen.length === 0 && epCount === 0} busy={busy}>
					{mode === 'all' ? t('requests.askShow.submitAll') : mode === 'future' ? t('requests.askShow.submitFuture') : summary ? t('requests.ask.title', { name: summary }) : t('requests.askShow.pick')}
				</Button>
	{/snippet}
</Sheet>

<style>
	.sheet-form { display: flex; flex-direction: column; gap: 4px; }
	.form-error { margin: 8px 0 0; color: var(--fail); font: var(--t-callout); }
	.pick,
	.seasons {
		border: 0;
		padding: 0;
		margin: var(--s-4) 0 0;
	}
	.seasons {
		display: flex;
		flex-direction: column;
		gap: var(--s-2);
		max-height: 50vh;
		overflow-y: auto;
	}
	.row {
		display: flex;
		align-items: center;
		gap: var(--s-2);
		min-height: 44px;
		padding: 0 var(--s-3);
		border: 1px solid var(--line-strong);
		border-radius: var(--r-btn);
	}
	.row.sub { margin: calc(var(--s-2) * -1) 0 0 var(--s-4); min-height: 36px; border: 0; border-bottom: 1px solid var(--line); border-radius: 0; }
	.row.sub .nm { font: var(--t-subhead); }
	.row.sub.on { background: var(--accent-soft); border-color: var(--line); }
	p.sub { margin: 0 0 0 var(--s-4); }
	.row.on {
		border-color: var(--accent);
		background: var(--accent-soft);
	}
	.pickrow {
		flex: 1;
		min-width: 0;
		display: flex;
		align-items: center;
		gap: var(--s-2);
		min-height: 44px;
		cursor: pointer;
	}
	.nm { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.more {
		min-height: 32px;
		padding: 0 var(--s-3);
		border: 1px solid var(--line-strong);
		border-radius: var(--r-btn);
		background: transparent;
		color: inherit;
		font: var(--t-callout);
		cursor: pointer;
	}
	.more:disabled { opacity: 0.4; cursor: default; }
</style>
