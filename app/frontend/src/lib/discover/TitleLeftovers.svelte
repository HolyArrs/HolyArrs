<!-- Rescan folder and the title's leftover files: match one by hand, move a shifted season in one go, or leave a file alone. Nothing moves on disk. -->
<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { api, message } from '#lib/core/api.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { bytes } from '#lib/core/format.ts';
	import { followJob, type JobView } from '#lib/mediafiles/index.ts';
	import type { OwnedTitle } from './title-owned.svelte.ts';
	import { fileName, reasonKey, shiftGuess, type Leftover, type MatchEpisode, type ShiftGuess } from './leftovers.ts';

	let { own }: { own: OwnedTitle } = $props();
	const o = $derived(own.owned);

	let list = $state<Leftover[]>([]);
	let episodes = $state<MatchEpisode[]>([]);
	let listOpen = $state(false);
	let matchOpen = $state(false);
	let previewOpen = $state(false);
	let showAlone = $state(false);
	let busy = $state('');
	let picked = $state<Leftover | null>(null);
	let renameAsk = $state(false);
	let season = $state(1);
	const chosen = $state<number[]>([]);

	const open = $derived(list.filter((l) => !l.dismissed));
	const alone = $derived(list.filter((l) => l.dismissed));
	const guess = $derived<ShiftGuess | null>(o?.kind === 'show' ? shiftGuess(list, episodes) : null);
	const seasons = $derived([...new Set(episodes.map((e) => e.season))].sort((a, b) => a - b));
	const replacing = $derived(episodes.some((e) => e.hasFile && chosen.includes(e.id)));
	const inSeason = $derived(episodes.filter((e) => e.season === season).sort((a, b) => a.episode - b.episode));

	async function load(id: number) {
		try {
			list = (await api.get<{ leftovers: Leftover[] }>(`/titles/${id}/leftovers`)).leftovers ?? [];
		} catch {
			list = [];
		}
	}

	async function loadEpisodes() {
		const x = o;
		if (!x || x.kind !== 'show') return;
		const all: MatchEpisode[] = [];
		for (const se of x.show?.seasons ?? []) {
			try {
				const r = await api.get<{ episodes: { id: number; episode: number; name?: string; slots: { file?: unknown }[] }[] }>(`/titles/${x.id}/episodes?season=${se.number}&files=1`);
				for (const e of r.episodes ?? []) all.push({ id: e.id, season: se.number, episode: e.episode, name: e.name, hasFile: e.slots.some((s) => !!s.file) });
			} catch {
				// A season that fails to load is left out of the pickers.
			}
		}
		episodes = all;
	}

	$effect(() => {
		const id = o?.id;
		if (id && own.manage) void load(id);
	});

	async function openList() {
		listOpen = true;
		if (o?.kind === 'show' && !episodes.length) await loadEpisodes();
	}

	async function rescan() {
		const x = o;
		if (!x) return;
		busy = 'rescan';
		try {
			const r = await api.post<{ job_id: number }>(`/titles/${x.id}/rescan`);
			toasts.push(t('discover.leftovers.rescanning', { name: x.name }), 'info');
			const end = await followJob((id) => api.get<JobView>(`/jobs/${id}`), r.job_id, () => {});
			if (end.State !== 'done') throw new Error(end.Error?.message || t('discover.leftovers.rescanFailed'));
			await Promise.all([load(x.id), own.loadOwned(x.id)]);
			episodes = [];
			const n = Number(end.Result?.leftovers ?? 0);
			toasts.push(t('discover.leftovers.rescanDone'), 'ok', { body: n ? t(n === 1 ? 'discover.leftovers.countOne' : 'discover.leftovers.countOther', { n }) : t('discover.leftovers.none') });
		} catch (e) {
			toasts.push(t('discover.leftovers.rescanFailed'), 'fail', { body: message(e) });
		} finally {
			busy = '';
		}
	}

	type Outcome = { file_id: number; linked: boolean; refused?: string };

	// Sends a match; refused items are counted, and Undo turns the linked ones back into leftovers.
	async function send(items: { file_id: number; episode_ids: number[] }[], replace: boolean) {
		const x = o;
		if (!x) return;
		busy = 'match';
		try {
			const r = await api.post<{ results: Outcome[] }>(`/titles/${x.id}/leftovers/match`, { items, replace });
			const linked = r.results.filter((y) => y.linked).map((y) => y.file_id);
			const refused = r.results.length - linked.length;
			matchOpen = previewOpen = false;
			await Promise.all([load(x.id), own.loadOwned(x.id)]);
			episodes = [];
			if (o?.kind === 'show') await loadEpisodes();
			if (linked.length) {
				renameAsk = true;
				toasts.push(t(linked.length === 1 ? 'discover.leftovers.matchedOne' : 'discover.leftovers.matchedOther', { n: linked.length }), 'ok', {
					body: refused ? t('discover.leftovers.notMatched', { n: refused }) : t('discover.leftovers.renameNote'),
					action: { label: t('discover.leftovers.undo'), run: () => void undo(linked) }
				});
			} else {
				toasts.push(t('discover.leftovers.matchFailed'), 'fail', { body: t('discover.leftovers.notMatched', { n: refused }) });
			}
		} catch (e) {
			toasts.push(t('discover.leftovers.matchFailed'), 'fail', { body: message(e) });
		} finally {
			busy = '';
		}
	}

	async function undo(ids: number[]) {
		const x = o;
		if (!x) return;
		try {
			await api.post(`/titles/${x.id}/leftovers/unmatch`, { file_ids: ids });
			await Promise.all([load(x.id), own.loadOwned(x.id)]);
			episodes = [];
		} catch (e) {
			toasts.push(t('discover.leftovers.matchFailed'), 'fail', { body: message(e) });
		}
	}

	async function dismiss(l: Leftover, on: boolean) {
		const x = o;
		if (!x) return;
		try {
			await api.put(`/titles/${x.id}/leftovers/${l.file_id}`, { dismissed: on });
			list = list.map((y) => (y.file_id === l.file_id ? { ...y, dismissed: on } : y));
		} catch (e) {
			toasts.push(t('discover.leftovers.matchFailed'), 'fail', { body: message(e) });
		}
	}

	function startMatch(l: Leftover) {
		picked = l;
		chosen.length = 0;
		if (o?.kind !== 'show') return void send([{ file_id: l.file_id, episode_ids: [] }], false);
		season = l.guess && seasons.includes(l.guess.season) ? l.guess.season : (seasons.find((s) => s > 0) ?? seasons[0] ?? 1);
		matchOpen = true;
	}

	function toggle(id: number) {
		const i = chosen.indexOf(id);
		if (i >= 0) chosen.splice(i, 1);
		else chosen.push(id);
	}

	const seasonWord = (n: number) => (n === 0 ? t('discover.leftovers.specials') : t('discover.leftovers.seasonN', { n }));
	const se = (p: [number, number]) => `S${String(p[0]).padStart(2, '0')}E${String(p[1]).padStart(2, '0')}`;
</script>

{#if own.manage && o}
	<button type="button" class="lo-row" data-testid="rescan-row" disabled={busy === 'rescan'} onclick={rescan}>
		<span><b>{t('discover.leftovers.rescanFolder')}</b><small>{busy === 'rescan' ? t('discover.leftovers.rescanningShort') : t('discover.leftovers.rescanHint')}</small></span><Icon name="refresh" size={16} />
	</button>
	{#if open.length || alone.length}
		<button type="button" class="lo-row" data-testid="leftovers-row" onclick={openList}>
			<span><b>{t('discover.leftovers.title')}</b><small>{open.length ? t(open.length === 1 ? 'discover.leftovers.countOne' : 'discover.leftovers.countOther', { n: open.length }) : t('discover.leftovers.aloneCount', { n: alone.length })}</small></span><Icon name="chevronRight" size={16} />
		</button>
	{/if}

	<Sheet bind:open={listOpen} title={t('discover.leftovers.title')} description={t('discover.leftovers.intro')} size="lg">
		{#if renameAsk}
			<div class="lo-bulk" data-testid="leftovers-rename">
				<span><b>{t('discover.leftovers.renameAfter')}</b><small>{t('discover.leftovers.renameNote')}</small></span>
				<Button size="sm" variant="secondary" onclick={() => ((renameAsk = false), (listOpen = false), (own.renameOpen = true))}>{t('discover.leftovers.renameAfter')}</Button>
			</div>
		{/if}
		{#if guess}
			<div class="lo-bulk" data-testid="leftovers-bulk">
				<span><b>{t('discover.leftovers.bulkTitle', { from: guess.from, to: guess.to, shift: guess.shift })}</b><small>{t('discover.leftovers.bulkBody', { n: guess.pairs.length })}</small></span>
				<Button size="sm" variant="primary" onclick={() => (previewOpen = true)}>{t('discover.leftovers.preview')}</Button>
			</div>
		{/if}
		<ul class="lo-list">
			{#each open as l (l.file_id)}
				<li class="lo-item" data-testid="leftover">
					<span class="lo-text">
						<b title={l.relative_path}>{fileName(l.relative_path)}</b>
						<small>{bytes(l.size)}{l.resolution ? ` · ${l.resolution}p` : ''} · {t(reasonKey(l), { n: l.dropped_season ?? 0 })}</small>
					</span>
					<span class="lo-acts">
						<Button size="sm" variant="secondary" onclick={() => dismiss(l, true)}>{t('discover.leftovers.leaveAlone')}</Button>
						<Button size="sm" variant="primary" busy={busy === 'match' && picked?.file_id === l.file_id} onclick={() => startMatch(l)}>{t('discover.leftovers.match')}</Button>
					</span>
				</li>
			{/each}
		</ul>
		{#if alone.length}
			<button type="button" class="lo-alone" aria-expanded={showAlone} onclick={() => (showAlone = !showAlone)}>{t('discover.leftovers.aloneCount', { n: alone.length })}<Icon name={showAlone ? 'chevronUp' : 'chevronDown'} size={14} /></button>
			{#if showAlone}
				<ul class="lo-list">
					{#each alone as l (l.file_id)}
						<li class="lo-item">
							<span class="lo-text"><b title={l.relative_path}>{fileName(l.relative_path)}</b><small>{bytes(l.size)}</small></span>
							<span class="lo-acts"><Button size="sm" variant="secondary" onclick={() => dismiss(l, false)}>{t('discover.leftovers.listAgain')}</Button></span>
						</li>
					{/each}
				</ul>
			{/if}
		{/if}
	</Sheet>

	<Sheet bind:open={matchOpen} title={t('discover.leftovers.matchTitle')} description={picked ? fileName(picked.relative_path) : undefined}>
		<div class="lo-seasons" role="radiogroup" aria-label={t('discover.leftovers.pickSeason')}>
			{#each seasons as n (n)}
				<button type="button" role="radio" aria-checked={season === n} class:on={season === n} onclick={() => ((season = n), (chosen.length = 0))}>{seasonWord(n)}</button>
			{/each}
		</div>
		<p class="lo-hint">{t('discover.leftovers.pickEpisodes')}</p>
		<ul class="lo-eps">
			{#each inSeason as e (e.id)}
				<li>
					<label class:on={chosen.includes(e.id)}>
						<input type="checkbox" checked={chosen.includes(e.id)} onchange={() => toggle(e.id)} />
						<span class="lo-text"><b dir="auto">{se([e.season, e.episode])}{e.name ? ` · ${e.name}` : ''}</b>{#if e.hasFile}<small>{t('discover.leftovers.hasFile')}</small>{/if}</span>
					</label>
				</li>
			{/each}
		</ul>
		{#if replacing}<p class="lo-hint warn">{t('discover.leftovers.replaceAsk')}</p>{/if}
		{#snippet footer()}
			<Button variant={replacing ? 'destructive' : 'primary'} full disabled={!chosen.length || !picked} busy={busy === 'match'} onclick={() => picked && send([{ file_id: picked.file_id, episode_ids: [...chosen] }], replacing)}>{replacing ? t('discover.leftovers.replace') : t('discover.leftovers.match')}</Button>
		{/snippet}
	</Sheet>

	{#if guess}
		<Sheet bind:open={previewOpen} title={t('discover.leftovers.bulkTitle', { from: guess.from, to: guess.to, shift: guess.shift })} description={t('discover.leftovers.renameNote')}>
			<table class="lo-preview">
				<thead><tr><th>{t('discover.leftovers.now')}</th><th>{t('discover.leftovers.becomes')}</th></tr></thead>
				<tbody>
					{#each guess.pairs as p (p.file_id)}
						<tr><td>{se(p.from)}</td><td>{se(p.to)}</td></tr>
					{/each}
				</tbody>
			</table>
			{#snippet footer()}
				<Button variant="primary" full busy={busy === 'match'} onclick={() => guess && send(guess.pairs.map((p) => ({ file_id: p.file_id, episode_ids: [p.episode_id] })), false)}>{t('discover.leftovers.matchAll', { n: guess.pairs.length })}</Button>
			{/snippet}
		</Sheet>
	{/if}
{/if}

<style>
	.lo-row { display: flex; align-items: center; justify-content: space-between; gap: 16px; width: 100%; padding: 14px 20px; border: 0; background: none; color: var(--text); text-align: start; cursor: pointer; font: inherit; box-shadow: inset 0 .5px 0 color-mix(in srgb, var(--text) 10%, transparent); }
	.lo-row:hover { background: color-mix(in srgb, var(--text) 4%, transparent); }
	.lo-row:focus-visible { outline: none; box-shadow: inset 0 0 0 2px var(--accent); }
	.lo-row:disabled { cursor: progress; opacity: .7; }
	.lo-row span, .lo-text { display: grid; gap: 3px; min-width: 0; }
	.lo-row b, .lo-text b { font: 600 15px/1.2 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.lo-row small, .lo-text small { color: var(--text-muted); font: 500 13px/1.3 var(--font-ui); overflow-wrap: anywhere; }
	.lo-bulk { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 14px 16px; margin-bottom: 12px; border-radius: 12px; background: var(--surface-2); }
	.lo-bulk span { display: grid; gap: 3px; min-width: 0; }
	.lo-bulk b { font: 600 15px/1.2 var(--font-ui); }
	.lo-bulk small { color: var(--text-muted); font: 500 13px/1.3 var(--font-ui); }
	.lo-list, .lo-eps { list-style: none; margin: 0; padding: 0; display: grid; }
	.lo-item { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 0; }
	.lo-item + .lo-item { box-shadow: inset 0 .5px 0 color-mix(in srgb, var(--text) 10%, transparent); }
	.lo-acts { display: flex; gap: 8px; flex: none; }
	.lo-alone { display: inline-flex; align-items: center; gap: 6px; margin-top: 12px; padding: 8px 0; border: 0; background: none; color: var(--text-muted); font: 600 13px/1.2 var(--font-ui); cursor: pointer; }
	.lo-seasons { display: flex; flex-wrap: wrap; gap: 8px; }
	.lo-seasons button { padding: 8px 14px; border: 0; border-radius: 999px; background: var(--surface-2); color: var(--text); font: 600 14px/1.2 var(--font-ui); cursor: pointer; }
	.lo-seasons button.on { background: var(--accent); color: var(--on-accent, #fff); }
	.lo-hint { margin: 14px 0 6px; color: var(--text-muted); font: 500 13px/1.3 var(--font-ui); }
	.lo-hint.warn { color: var(--warn-text, var(--text)); }
	.lo-eps label { display: flex; align-items: center; gap: 12px; padding: 10px 4px; border-radius: 10px; cursor: pointer; }
	.lo-eps label.on { background: color-mix(in srgb, var(--accent) 12%, transparent); }
	.lo-eps input { width: 18px; height: 18px; flex: none; accent-color: var(--accent); }
	.lo-preview { width: 100%; border-collapse: collapse; font: 500 14px/1.3 var(--font-ui); }
	.lo-preview th { text-align: start; color: var(--text-muted); font-weight: 600; padding: 6px 0; }
	.lo-preview td { padding: 6px 0; box-shadow: inset 0 .5px 0 color-mix(in srgb, var(--text) 10%, transparent); font-variant-numeric: tabular-nums; }
	@media (max-width: 560px) {
		.lo-item { flex-direction: column; align-items: stretch; }
		.lo-acts { display: grid; grid-template-columns: 1fr 1fr; }
		.lo-acts :global(.ui-btn) { width: 100%; }
		.lo-bulk { flex-direction: column; align-items: stretch; }
	}
</style>
