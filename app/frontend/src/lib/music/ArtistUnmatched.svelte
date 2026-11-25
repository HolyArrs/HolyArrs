<!-- An artist's unmatched files: music in its folder that fills no track. Match one to a track by hand (the tracklist beside the file), use a pressing the files fit better, or leave a file alone. Nothing moves on disk. -->
<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { api, message } from '#lib/core/api.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { bytes } from '#lib/core/format.ts';
	import { followJob, type JobView } from '#lib/mediafiles/index.ts';
	import type { AlbumSummary } from '#lib/core/types.ts';
	import type { TrackView } from './types.ts';
	import { fileName, unmatchedReason, type MusicLeftover, type PressingHint } from './unmatched.ts';

	let { titleId, albums, onchange }: { titleId: number; albums: AlbumSummary[]; onchange?: () => void } = $props();

	let list = $state<MusicLeftover[]>([]);
	let hints = $state<PressingHint[]>([]);
	let listOpen = $state(false);
	let matchOpen = $state(false);
	let showAlone = $state(false);
	let busy = $state('');
	let picked = $state<MusicLeftover | null>(null);
	let albumId = $state(0);
	let tracks = $state<TrackView[]>([]);
	let trackId = $state(0);

	const open = $derived(list.filter((l) => !l.dismissed));
	const alone = $derived(list.filter((l) => l.dismissed));
	const chosenTrack = $derived(tracks.find((x) => x.id === trackId));
	const replacing = $derived(!!chosenTrack?.slots.some((s) => !!s.file_id));
	const pickable = $derived(albums.filter((a) => a.included));

	// The row needs only the count; the reasons, closest tracks and pressings are read when the list opens.
	async function load(full = listOpen) {
		try {
			const r = await api.get<{ leftovers: MusicLeftover[]; pressings?: PressingHint[] }>(`/titles/${titleId}/leftovers${full ? '' : '?brief=1'}`);
			list = r.leftovers ?? [];
			hints = r.pressings ?? [];
		} catch {
			list = [];
			hints = [];
		}
	}

	$effect(() => {
		void titleId;
		void load(false);
	});

	async function loadTracks(id: number) {
		tracks = [];
		if (!id) return;
		try {
			tracks = ((await api.get<{ tracks: TrackView[] }>(`/albums/${id}/tracks`)).tracks ?? []).filter((x) => !x.dropped);
		} catch {
			tracks = [];
		}
	}

	async function startMatch(l: MusicLeftover) {
		picked = l;
		albumId = l.music?.album_id && pickable.some((a) => a.id === l.music?.album_id) ? l.music.album_id : (pickable[0]?.id ?? 0);
		trackId = l.music?.album_id === albumId ? (l.music?.track_id ?? 0) : 0;
		matchOpen = true;
		await loadTracks(albumId);
	}

	async function pickAlbum(id: number) {
		albumId = id;
		trackId = 0;
		await loadTracks(id);
	}

	async function refresh() {
		await load();
		onchange?.();
	}

	async function send(fileId: number, ids: number[], replace: boolean) {
		busy = 'match';
		try {
			const r = await api.post<{ results: { file_id: number; linked: boolean; refused?: string }[] }>(`/titles/${titleId}/leftovers/match`, { items: [{ file_id: fileId, episode_ids: ids }], replace });
			const ok = r.results.some((y) => y.linked);
			matchOpen = false;
			await refresh();
			if (ok) toasts.push(t('discover.leftovers.matchedOne'), 'ok', { action: { label: t('discover.leftovers.undo'), run: () => void undo(fileId) } });
			else toasts.push(t('discover.leftovers.matchFailed'), 'fail', { body: t('discover.leftovers.notMatched', { n: 1 }) });
		} catch (e) {
			toasts.push(t('discover.leftovers.matchFailed'), 'fail', { body: message(e) });
		} finally {
			busy = '';
		}
	}

	async function undo(fileId: number) {
		try {
			await api.post(`/titles/${titleId}/leftovers/unmatch`, { file_ids: [fileId] });
			await refresh();
		} catch (e) {
			toasts.push(t('discover.leftovers.matchFailed'), 'fail', { body: message(e) });
		}
	}

	async function dismiss(l: MusicLeftover, on: boolean) {
		try {
			await api.put(`/titles/${titleId}/leftovers/${l.file_id}`, { dismissed: on });
			list = list.map((y) => (y.file_id === l.file_id ? { ...y, dismissed: on } : y));
			onchange?.();
		} catch (e) {
			toasts.push(t('discover.leftovers.matchFailed'), 'fail', { body: message(e) });
		}
	}

	// Only ever on the person's word: the chosen pressing changes, then the folder is looked at again.
	async function usePressing(h: PressingHint) {
		busy = `p${h.album_id}`;
		try {
			await api.put(`/albums/${h.album_id}`, { pressing_id: h.pressing_id });
			toasts.push(t('music.unmatched.pressingChanged'), 'ok', { body: t('music.unmatched.pressingRescan') });
			const r = await api.post<{ job_id: number }>(`/titles/${titleId}/rescan`);
			await followJob((id) => api.get<JobView>(`/jobs/${id}`), r.job_id, () => {});
			await refresh();
		} catch (e) {
			toasts.push(t('music.unmatched.pressingFailed'), 'fail', { body: message(e) });
		} finally {
			busy = '';
		}
	}

	const trackNo = (x: TrackView) => `${tracks.some((y) => y.disc > 1) ? `${x.disc}-` : ''}${String(x.number).padStart(2, '0')}`;
	const length = (ms?: number) => (ms ? `${Math.floor(ms / 60000)}:${String(Math.round(ms / 1000) % 60).padStart(2, '0')}` : '');
</script>

{#if open.length || alone.length}
	<button type="button" class="um-row" data-testid="unmatched-row" onclick={() => ((listOpen = true), void load(true))}>
		<span><b>{t('music.unmatched.title')}</b><small>{open.length ? t(open.length === 1 ? 'music.unmatched.countOne' : 'music.unmatched.countOther', { n: open.length }) : t('discover.leftovers.aloneCount', { n: alone.length })}</small></span><Icon name="chevronRight" size={16} />
	</button>
{/if}

<Sheet bind:open={listOpen} title={t('music.unmatched.title')} description={t('music.unmatched.intro')} size="lg">
	{#each hints as h (h.album_id)}
		<div class="um-bulk" data-testid="unmatched-pressing">
			<span><b dir="auto">{t('music.unmatched.pressingTitle', { album: h.album_name })}</b><small dir="auto">{t('music.unmatched.pressingBody', { fits: h.fits, files: h.files, pressing: h.pressing_name, tracks: h.track_count, chosen: h.chosen })}</small></span>
			<Button size="sm" variant="secondary" busy={busy === `p${h.album_id}`} onclick={() => usePressing(h)}>{t('music.unmatched.usePressing')}</Button>
		</div>
	{/each}
	<ul class="um-list">
		{#each open as l (l.file_id)}
			<li class="um-item" data-testid="unmatched-file">
				<span class="um-text">
					<b title={l.relative_path} dir="auto">{fileName(l.relative_path)}</b>
					<small>{bytes(l.size)} · {t(unmatchedReason(l.reason), { name: l.music?.owner ?? '' })}</small>
					{#if l.music?.album_name}<small dir="auto">{t('music.unmatched.closest', { album: l.music.album_name })}</small>{/if}
				</span>
				<span class="um-acts">
					<Button size="sm" variant="secondary" onclick={() => dismiss(l, true)}>{t('discover.leftovers.leaveAlone')}</Button>
					<Button size="sm" variant="primary" disabled={!pickable.length} onclick={() => startMatch(l)}>{t('discover.leftovers.match')}</Button>
				</span>
			</li>
		{/each}
	</ul>
	{#if alone.length}
		<button type="button" class="um-alone" aria-expanded={showAlone} onclick={() => (showAlone = !showAlone)}>{t('discover.leftovers.aloneCount', { n: alone.length })}<Icon name={showAlone ? 'chevronUp' : 'chevronDown'} size={14} /></button>
		{#if showAlone}
			<ul class="um-list">
				{#each alone as l (l.file_id)}
					<li class="um-item">
						<span class="um-text"><b title={l.relative_path} dir="auto">{fileName(l.relative_path)}</b><small>{bytes(l.size)}</small></span>
						<span class="um-acts"><Button size="sm" variant="secondary" onclick={() => dismiss(l, false)}>{t('discover.leftovers.listAgain')}</Button></span>
					</li>
				{/each}
			</ul>
		{/if}
	{/if}
</Sheet>

<Sheet bind:open={matchOpen} title={t('music.unmatched.matchTitle')} size="lg">
	{#if picked}
		<div class="um-side">
			<div class="um-file" data-testid="unmatched-match-file">
				<b dir="auto" title={picked.relative_path}>{fileName(picked.relative_path)}</b>
				<small>{bytes(picked.size)}</small>
				<small>{t(unmatchedReason(picked.reason), { name: picked.music?.owner ?? '' })}</small>
				<small class="um-path" dir="auto">{picked.relative_path}</small>
			</div>
			<div class="um-pick">
				<label class="um-label">
					<span>{t('music.unmatched.album')}</span>
					<select value={albumId} onchange={(e) => pickAlbum(Number(e.currentTarget.value))}>
						{#each pickable as a (a.id)}<option value={a.id}>{a.name}</option>{/each}
					</select>
				</label>
				<p class="um-hint">{tracks.length ? t('music.unmatched.pickTrack') : t('music.unmatched.noTracks')}</p>
				<ul class="um-tracks" role="radiogroup" aria-label={t('music.unmatched.pickTrack')}>
					{#each tracks as x (x.id)}
						<li>
							<label class:on={trackId === x.id}>
								<input type="radio" name="um-track" checked={trackId === x.id} onchange={() => (trackId = x.id)} />
								<span class="um-num">{trackNo(x)}</span>
								<span class="um-text"><b dir="auto">{x.name ?? ''}</b>{#if x.slots.some((s) => !!s.file_id)}<small>{t('discover.leftovers.hasFile')}</small>{/if}</span>
								<span class="um-len">{length(x.length_ms)}</span>
							</label>
						</li>
					{/each}
				</ul>
			</div>
		</div>
		{#if replacing}<p class="um-hint warn">{t('music.unmatched.replaceAsk')}</p>{/if}
	{/if}
	{#snippet footer()}
		<Button variant={replacing ? 'destructive' : 'primary'} full disabled={!trackId || !picked} busy={busy === 'match'} onclick={() => picked && send(picked.file_id, [trackId], replacing)}>{replacing ? t('discover.leftovers.replace') : t('discover.leftovers.match')}</Button>
	{/snippet}
</Sheet>

<style>
	.um-row { display: flex; align-items: center; justify-content: space-between; gap: 16px; width: 100%; padding: 14px 20px; border: 0; border-radius: 14px; background: var(--surface-2); color: var(--text); text-align: start; cursor: pointer; font: inherit; }
	.um-row:hover { background: color-mix(in srgb, var(--text) 8%, var(--surface-2)); }
	.um-row:focus-visible { outline: none; box-shadow: inset 0 0 0 2px var(--accent); }
	.um-row span, .um-text, .um-file { display: grid; gap: 3px; min-width: 0; }
	.um-row b, .um-text b, .um-file b { font: 600 15px/1.2 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.um-row small, .um-text small, .um-file small { color: var(--text-muted); font: 500 13px/1.3 var(--font-ui); overflow-wrap: anywhere; }
	.um-bulk { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 14px 16px; margin-bottom: 12px; border-radius: 12px; background: var(--surface-2); }
	.um-bulk span { display: grid; gap: 3px; min-width: 0; }
	.um-bulk b { font: 600 15px/1.2 var(--font-ui); }
	.um-bulk small { color: var(--text-muted); font: 500 13px/1.3 var(--font-ui); }
	.um-list, .um-tracks { list-style: none; margin: 0; padding: 0; display: grid; }
	.um-item { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 0; }
	.um-item + .um-item { box-shadow: inset 0 .5px 0 color-mix(in srgb, var(--text) 10%, transparent); }
	.um-acts { display: flex; gap: 8px; flex: none; }
	.um-alone { display: inline-flex; align-items: center; gap: 6px; margin-top: 12px; padding: 8px 0; border: 0; background: none; color: var(--text-muted); font: 600 13px/1.2 var(--font-ui); cursor: pointer; }
	/* The file beside the tracklist; stacked on a phone. */
	.um-side { display: grid; grid-template-columns: minmax(0, 2fr) minmax(0, 3fr); gap: 20px; align-items: start; }
	.um-file { padding: 14px 16px; border-radius: 12px; background: var(--surface-2); position: sticky; top: 0; }
	.um-file .um-path { font-size: 12px; }
	.um-label { display: grid; gap: 6px; color: var(--text-muted); font: 600 13px/1.2 var(--font-ui); }
	.um-label select { min-height: 40px; padding: 0 12px; border: 0; border-radius: 10px; background: var(--surface-2); color: var(--text); font: 500 15px/1.2 var(--font-ui); }
	.um-hint { margin: 14px 0 6px; color: var(--text-muted); font: 500 13px/1.3 var(--font-ui); }
	.um-hint.warn { color: var(--warn-text, var(--text)); }
	.um-tracks label { display: flex; align-items: center; gap: 12px; padding: 10px 4px; border-radius: 10px; cursor: pointer; }
	.um-tracks label.on { background: color-mix(in srgb, var(--accent) 12%, transparent); }
	.um-tracks input { width: 18px; height: 18px; flex: none; accent-color: var(--accent); }
	.um-tracks .um-text { flex: 1; }
	.um-num, .um-len { flex: none; color: var(--text-muted); font: 500 13px/1.2 var(--font-ui); font-variant-numeric: tabular-nums; }
	@media (max-width: 560px) {
		.um-item { flex-direction: column; align-items: stretch; }
		.um-acts { display: grid; grid-template-columns: 1fr 1fr; }
		.um-acts :global(.ui-btn) { width: 100%; }
		.um-bulk { flex-direction: column; align-items: stretch; }
		.um-side { grid-template-columns: minmax(0, 1fr); gap: 12px; }
		.um-file { position: static; }
	}
</style>
