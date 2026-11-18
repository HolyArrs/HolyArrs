<script lang="ts">
	import { ask } from '#lib/ui/ask.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import Isolated from '#lib/ui/Isolated.svelte';
	import Button from '#lib/ui/Button.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import Badge from '#lib/ui/Badge.svelte';
	import { LibraryToolPage, adoptParts, blocker, episodeCode, itemsFor, pickLine, resultLine, startPick, startTicked, oneTickPerTrack, type ByHandFile, type ByHandResult, type ByHandScan, type Pick } from '#lib/mediafiles/index.ts';
	import { to } from '#lib/core/paths.ts';
	import { api, message } from '#lib/core/api.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { bytes } from '#lib/core/format.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import type { AlbumSummary, EpisodeView, TitleView } from '#lib/core/types.ts';
	import type { TrackView } from '#lib/music/index.ts';
	import { appLocale, ltr, t as tr } from '#lib/core/i18n/i18n.svelte.ts';
	import { page } from '$app/state';
	import { onMount } from 'svelte';

	let path = $state('');
	let scanning = $state(false);
	let scan = $state<ByHandScan | null>(null);
	let picks = $state<Record<string, Pick | null>>({});
	let ticked = $state<Record<string, boolean>>({});
	let held = $state<Record<string, boolean>>({});
	let results = $state<Record<string, ByHandResult>>({});
	let importing = $state(false);
	let error = $state('');

	// The match editor: one file at a time.
	let editFor = $state<string | null>(null);
	let term = $state('');
	let found = $state<TitleView[]>([]);
	let looking = $state(false);
	let lookError = $state('');
	let chosen = $state<TitleView | null>(null);
	let episodes = $state<EpisodeView[]>([]);
	let versionId = $state(0);
	let season = $state<number | undefined>(undefined);
	let episode = $state<number | undefined>(undefined);
	// Music: the artist's albums and the picked album's tracks.
	let albums = $state<AlbumSummary[]>([]);
	let albumId = $state<number | undefined>(undefined);
	let tracks = $state<TrackView[]>([]);
	let trackId = $state<number | undefined>(undefined);

	const files = $derived(scan?.files ?? []);
	const items = $derived(itemsFor(files, picks, ticked));
	const seasons = $derived([...new Set(episodes.map((e) => e.season))].sort((a, b) => a - b));
	const seasonEps = $derived(episodes.filter((e) => e.season === season));
	const pickedTrack = $derived(tracks.find((x) => x.id === trackId));
	const discs = $derived(Math.max(1, ...tracks.map((x) => x.disc)));

	// Review's Import files by hand sends ?path= (the download's folder): look in it straight away.
	onMount(() => {
		const asked = page.url.searchParams.get('path')?.trim();
		if (asked) {
			path = asked;
			void doScan();
		}
	});

	async function doScan(e?: SubmitEvent) {
		e?.preventDefault();
		if (!path.trim()) return;
		scanning = true;
		error = '';
		results = {};
		editFor = null;
		try {
			const s = await api.post<ByHandScan>('/manual-import/scan', { path: path.trim() });
			const p: Record<string, Pick | null> = {};
			const t: Record<string, boolean> = {};
			for (const f of s.files) {
				p[f.path] = startPick(f);
				t[f.path] = startTicked(f, p[f.path]);
			}
			held = oneTickPerTrack(s.files, p, t);
			picks = p;
			ticked = t;
			scan = s;
		} catch (err) {
			error = message(err);
			scan = null;
		} finally {
			scanning = false;
		}
	}

	function openEditor(f: ByHandFile) {
		editFor = f.path;
		const p = picks[f.path];
		term = p?.title_name ?? f.name.split('/').pop()?.replace(/\.[^.]+$/, '').replace(/[._]/g, ' ') ?? '';
		found = [];
		lookError = '';
		chosen = null;
		episodes = [];
		albums = [];
		tracks = [];
		if (p) void choose(p.title_id, p);
	}

	async function lookup(e: SubmitEvent) {
		e.preventDefault();
		if (!term.trim()) return;
		looking = true;
		lookError = '';
		try {
			const r = await api.get<{ titles: TitleView[] }>(`/library/search?q=${encodeURIComponent(term.trim())}&kind=all`);
			found = r.titles.filter((t) => t.kind === 'movie' || t.kind === 'show' || t.kind === 'artist');
			if (!found.length) lookError = tr('library.byHand.nothingByNameAll');
		} catch (err) {
			lookError = message(err);
		} finally {
			looking = false;
		}
	}

	async function choose(id: number, from?: Pick | null) {
		lookError = '';
		try {
			const t = await api.get<TitleView>(`/titles/${id}`);
			episodes = t.kind === 'show' ? (await api.get<{ episodes: EpisodeView[] }>(`/titles/${id}/episodes`)).episodes : [];
			albums = t.kind === 'artist' ? (await api.get<{ albums: AlbumSummary[] }>(`/titles/${id}/albums`)).albums.filter((a) => !a.dropped) : [];
			tracks = [];
			trackId = undefined;
			albumId = t.kind === 'artist' ? from?.album_id : undefined;
			if (albumId) await pickAlbum(albumId, from?.slot_id);
			chosen = t;
			versionId = from?.version_id ?? t.versions.find((v) => v.main)?.id ?? t.versions[0]?.id ?? 0;
			season = from?.season ?? seasons[0];
			episode = from?.episode;
		} catch (err) {
			lookError = message(err);
		}
	}

	async function pickAlbum(id: number | undefined, slot?: number) {
		tracks = [];
		trackId = undefined;
		if (!id) return;
		try {
			tracks = (await api.get<{ tracks: TrackView[] }>(`/albums/${id}/tracks`)).tracks.filter((x) => !x.dropped);
			trackId = slot ? tracks.find((x) => x.slots.some((s) => s.id === slot))?.id : undefined;
		} catch (err) {
			lookError = message(err);
		}
	}

	function trackSlot(x: TrackView | undefined) {
		return x?.slots.find((s) => s.version_id === versionId) ?? x?.slots[0];
	}

	function hasFile(): boolean {
		if (!chosen) return false;
		if (chosen.kind === 'artist') return !!trackSlot(pickedTrack)?.file_id;
		if (chosen.kind === 'show') {
			const ep = episodes.find((e) => e.season === season && e.episode === episode);
			return !!ep?.slots.find((s) => s.version_id === versionId)?.file_id;
		}
		return !!chosen.slots.find((s) => s.version_id === versionId)?.file;
	}

	function useMatch() {
		if (!editFor || !chosen) return;
		const show = chosen.kind === 'show';
		const album = albums.find((a) => a.id === albumId);
		const music =
			chosen.kind === 'artist' && album && pickedTrack
				? {
						album_id: album.id,
						album_name: album.name,
						album_year: album.release_date ? Number(album.release_date.slice(0, 4)) : undefined,
						disc: pickedTrack.disc,
						track: pickedTrack.number,
						track_name: pickedTrack.name,
						slot_id: trackSlot(pickedTrack)?.id
					}
				: {};
		picks[editFor] = {
			...music,
			title_id: chosen.id,
			title_name: chosen.name,
			title_kind: chosen.kind,
			year: chosen.year,
			version_id: versionId || undefined,
			season: show ? season : undefined,
			episode: show ? episode : undefined,
			has_file: hasFile()
		};
		const f = files.find((x) => x.path === editFor);
		ticked[editFor] = !!f && !blocker(f, picks[editFor]);
		held[editFor] = false;
		editFor = null;
	}

	function setReplace(f: ByHandFile, on: boolean) {
		const p = picks[f.path];
		if (!p) return;
		picks[f.path] = { ...p, replace: on };
		ticked[f.path] = !blocker(f, picks[f.path]);
	}

	async function doImport() {
		if (!items.length) return;
		const swaps = items.filter((x) => x.replace).length;
		if (swaps && !(await ask(tr(swaps === 1 ? 'library.byHand.askReplaceOne' : 'library.byHand.askReplaceMany', { n: swaps }), { yes: tr('ui.ask.import') }))) return;
		importing = true;
		error = '';
		const next = { ...results };
		let n = 0;
		try {
			// Many files go in parts, so no request comes near the server's size limit.
			for (const part of adoptParts(items)) {
				const r = await api.post<{ results: ByHandResult[]; placed: number }>('/manual-import', { items: part });
				for (const x of r.results) {
					next[x.path] = x;
					if (x.outcome === 'placed') {
						ticked[x.path] = false;
						const p = picks[x.path];
						if (p) picks[x.path] = { ...p, has_file: true, replace: false };
					}
				}
				n += r.placed;
			}
		} catch (err) {
			error = message(err);
		} finally {
			results = next;
			if (n || !error) toasts.push(tr(n === 1 ? 'library.byHand.importedOne' : 'library.byHand.importedMany', { n }), n ? 'ok' : 'info');
			importing = false;
		}
	}
</script>

<svelte:head><title>{tr('library.byHand.title')} · HolyArrs</title></svelte:head>

<LibraryToolPage title={tr('library.byHand.title')}>
{#if !session.can('settings')}
	<EmptyState icon="shield" title={tr('library.byHand.owner')} />
{:else}

	<section class="tp-sheet tp-pad" aria-labelledby="h-path">
		<h2 id="h-path" class="h2">{tr('library.byHand.step1')}</h2>
		<form class="row pick" onsubmit={doScan}>
			<div class="field grow">
				<label for="mi-path">{tr('library.byHand.folderOrFile')}</label>
				<input id="mi-path" class="input mono" bind:value={path} placeholder="/downloads/complete/Some.Show.S01" autocomplete="off" spellcheck="false" aria-describedby="mi-where" />
			</div>
			<Button type="submit" variant="primary" icon="search" busy={scanning} disabled={importing || !path.trim()}>{tr('library.byHand.scan')}</Button>
		</form>
		<p class="xs muted where" id="mi-where">{tr('library.byHand.where')} <a href={to('/settings/library-folders#import-folders')}>{tr('library.byHand.whereLink')}</a> {tr('library.byHand.whereEnd')}</p>
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
	</section>

	{#if scan}
		<section class="tp-sheet tp-pad" aria-labelledby="h-files" data-testid="import-files">
			<h2 id="h-files" class="h2">{tr('library.byHand.step2')}</h2>
			{#if !files.length}
				<EmptyState compact icon="folder" title={tr('library.byHand.noFiles')} text={tr('library.byHand.noFilesHint')} />
			{:else}
				<p class="small muted">
					<Isolated text={tr(files.length === 1 ? 'library.byHand.filesInOne' : 'library.byHand.filesInMany', { n: files.length.toLocaleString(appLocale()), path: ltr(scan.path) })} />
				</p>
				{#if scan.more}<p class="notice">{tr('library.byHand.onlyFirst', { n: files.length.toLocaleString(appLocale()) })}</p>{/if}
				<ul class="list">
					{#each files as f (f.path)}
						{@const p = picks[f.path] ?? null}
						{@const why = blocker(f, p)}
						{@const done = results[f.path]}
						<li data-testid="import-file">
							<div class="top">
								<label class="tick">
									<input type="checkbox" bind:checked={ticked[f.path]} disabled={!!why || importing} aria-label={tr('library.byHand.importName', { name: ltr(f.name) })} />
									<span class="mono small path">{f.name}</span>
								</label>
								<span class="meta xs muted">{bytes(f.size)}{#if f.quality}{' · '}{f.quality}{/if}</span>
							</div>
							{#if f.kind === 'video' || p || f.guess.title_id || f.guess.reason}
								<div class="match">
									<b class="small">{pickLine(p)}</b>
									{#if p && f.guess.title_id === p.title_id && (p.title_kind !== 'artist' || p.slot_id === f.guess.slot_id) && f.guess.confidence > 0 && f.guess.confidence < 1}<Badge label={f.guess.confidence >= 0.7 ? tr('library.byHand.likely') : tr('library.byHand.guess')} />{/if}
									{#if !f.in_library && editFor !== f.path}<Button variant="plain" size="sm" onclick={() => openEditor(f)} disabled={importing}>{tr('shows.versions.change')}</Button>{/if}
								</div>
							{/if}
							{#if p?.has_file && !f.in_library && !done}
								<label class="tick swap">
									<input type="checkbox" checked={!!p.replace} onchange={(e) => setReplace(f, e.currentTarget.checked)} disabled={importing} />
									<span class="small">{tr('mediafiles.byHand.replace')}</span>
								</label>
							{/if}
							{#if done?.outcome !== 'placed'}{#if why}<p class="xs muted">{why}</p>{:else if f.note}<p class="xs muted">{f.note}</p>{:else if held[f.path] && !ticked[f.path] && !done}<p class="xs muted">{tr('mediafiles.byHand.music.sameTrack')}</p>{/if}{/if}
							{#if done}
								<p class="xs" class:ok-text={done.outcome === 'placed'} class:bad-text={done.outcome !== 'placed'} role="status">
									{resultLine(done)}{#if done.title_id && done.outcome === 'placed'}{' · '}<a href={to(`/library/${done.title_id}`)}>{tr('mediafiles.poster.open')}</a>{/if}
								</p>
							{/if}
							{#if editFor === f.path}
								<div class="editor">
									<form class="look" onsubmit={lookup}>
										<label class="sr-only" for="look-{f.path}"><Isolated text={tr('library.byHand.lookFor', { name: ltr(f.name) })} /></label>
										<input id="look-{f.path}" class="input" bind:value={term} placeholder={tr('library.byHand.placeholderAll')} autocomplete="off" />
										<Button type="submit" busy={looking}>{tr('library.byHand.lookUp')}</Button>
										<Button variant="plain" onclick={() => (editFor = null)}>{tr('shows.page.close')}</Button>
									</form>
									{#if lookError}<p class="form-error" role="alert">{lookError}</p>{/if}
									{#if found.length}
										<ul class="found">
											{#each found as t (t.id)}
												<li>
													<span class="small">{t.name}{t.year ? ` (${t.year})` : ''} <span class="muted xs">{tr(t.kind === 'show' ? 'mediafiles.kindCap.show' : t.kind === 'artist' ? 'mediafiles.kindCap.artist' : 'mediafiles.kindCap.movie')}</span></span>
													<Button variant="plain" size="sm" onclick={() => choose(t.id)}>{chosen?.id === t.id ? tr('library.byHand.picked') : tr('mediafiles.releaseSheet.pick')}</Button>
												</li>
											{/each}
										</ul>
									{/if}
									{#if chosen}
										<div class="row picks">
											{#if chosen.versions.length > 1 && chosen.kind !== 'artist'}
												<div class="field">
													<label class="xs" for="ver-{f.path}">{tr('library.byHand.version')}</label>
													<select id="ver-{f.path}" class="input" bind:value={versionId}>
														{#each chosen.versions as v (v.id)}<option value={v.id}>{v.label}</option>{/each}
													</select>
												</div>
											{/if}
											{#if chosen.kind === 'show'}
												<div class="field">
													<label class="xs" for="sea-{f.path}">{tr('shows.picker.season')}</label>
													<select id="sea-{f.path}" class="input" bind:value={season} onchange={() => (episode = undefined)}>
														{#each seasons as s (s)}<option value={s}>{s === 0 ? tr('shows.season.specials') : tr('shows.season.named', { n: s })}</option>{/each}
													</select>
												</div>
												<div class="field">
													<label class="xs" for="ep-{f.path}">{tr('library.byHand.episode')}</label>
													<select id="ep-{f.path}" class="input" bind:value={episode}>
														<option value={undefined}>{tr('library.byHand.pickOne')}</option>
														{#each seasonEps as e (e.id)}<option value={e.episode}>{episodeCode(e.season, e.episode)}{e.name ? ` · ${e.name}` : ''}</option>{/each}
													</select>
												</div>
											{/if}
											{#if chosen.kind === 'artist'}
												<div class="field">
													<label class="xs" for="alb-{f.path}">{tr('library.byHand.album')}</label>
													<select id="alb-{f.path}" class="input" bind:value={albumId} onchange={() => pickAlbum(albumId)}>
														<option value={undefined}>{tr('library.byHand.pickOne')}</option>
														{#each albums as a (a.id)}<option value={a.id}>{a.name}{a.release_date ? ` (${a.release_date.slice(0, 4)})` : ''}</option>{/each}
													</select>
												</div>
												<div class="field">
													<label class="xs" for="trk-{f.path}">{tr('library.byHand.track')}</label>
													<select id="trk-{f.path}" class="input" bind:value={trackId} disabled={!tracks.length}>
														<option value={undefined}>{tr('library.byHand.pickOne')}</option>
														{#each tracks as x (x.id)}<option value={x.id}>{discs > 1 ? `${x.disc}-` : ''}{String(x.number).padStart(2, '0')}{x.name ? ` · ${x.name}` : ''}</option>{/each}
													</select>
												</div>
											{/if}
										</div>
										<div class="row end tight">
											<span class="small muted">{chosen.name}{chosen.kind === 'show' && episode !== undefined ? ` · ${episodeCode(season, episode)}` : ''}{chosen.kind === 'artist' && pickedTrack ? ` · ${String(pickedTrack.number).padStart(2, '0')}${pickedTrack.name ? ` ${pickedTrack.name}` : ''}` : ''}</span>
											<Button variant="primary" size="sm" onclick={useMatch} disabled={(chosen.kind === 'show' && (season === undefined || episode === undefined)) || (chosen.kind === 'artist' && !trackSlot(pickedTrack))}>{tr('library.byHand.useThis')}</Button>
										</div>
									{/if}
								</div>
							{/if}
						</li>
					{/each}
				</ul>
				<p class="xs muted" data-testid="where-placed">{tr('library.byHand.wherePlaced')}</p>
				<div class="row end">
					<span class="small muted">{tr('library.byHand.ticked', { n: items.length.toLocaleString(appLocale()) })}</span>
					<Button variant="primary" icon="download" onclick={doImport} disabled={!items.length} busy={importing}>
						{tr(items.length === 1 ? 'library.byHand.importOne' : 'library.byHand.importMany', { n: items.length.toLocaleString(appLocale()) })}
					</Button>
				</div>
			{/if}
		</section>
	{:else if !scanning}
		<p class="tp-msg good"><Icon name="shield" size={18} /><span>{tr('library.byHand.nothingMoves')}</span></p>
	{/if}
{/if}
</LibraryToolPage>

<style>
	:global(.tp) .h2 { margin: 0 0 12px; font: var(--t-title3); }
	.notice { display: flex; gap: 10px; align-items: flex-start; margin: var(--s-2) 0 0; padding: 12px 14px; border-radius: var(--r-btn); background: var(--surface-2); color: var(--text); font: var(--t-callout); }
	.pick { align-items: flex-end; gap: var(--s-3); flex-wrap: wrap; padding: var(--s-3); border-radius: var(--r-btn); background: var(--surface-2); }
	.grow { flex: 1 1 260px; margin-bottom: 0; min-width: 0; }
	.where { margin: var(--s-2) 0 0; overflow-wrap: anywhere; }
	/* On touch phones the link in the sentence gets a 44px tall box (inline padding moves nothing). */
	@media (max-width: 767px) and (pointer: coarse) { .where a { padding-block: 14px; } }
	.list, .found { list-style: none; padding: 0; margin: var(--s-2) 0 0; }
	.list { border: 1px solid var(--line); border-radius: var(--r-btn); overflow: hidden; }
	.list > li { display: flex; flex-direction: column; gap: var(--s-1); padding: var(--s-3) var(--s-4); }
	.list > li + li { border-top: 1px solid var(--line); }
	.top, .match { display: flex; align-items: center; gap: var(--s-2); min-width: 0; }
	.top { justify-content: space-between; }
	.match { flex-wrap: wrap; }
	.meta { flex: none; }
	.tick { display: flex; align-items: center; gap: var(--s-2); min-height: 40px; min-width: 0; }
	.tick input { width: 20px; height: 20px; flex: none; }
	.path { overflow-wrap: anywhere; }
	.editor { display: flex; flex-direction: column; gap: var(--s-2); margin-top: var(--s-2); padding: var(--s-3); border-radius: var(--r-btn); background: var(--surface-2); }
	.look { display: flex; flex-wrap: wrap; gap: var(--s-2); }
	.look .input { flex: 1 1 200px; min-width: 0; }
	.found li { display: flex; justify-content: space-between; align-items: center; gap: var(--s-2); padding: var(--s-1) 0; }
	.picks { flex-wrap: wrap; gap: var(--s-3); }
	.picks .field { margin-bottom: 0; min-width: 0; flex: 1 1 140px; }
	.end { justify-content: flex-end; align-items: center; flex-wrap: wrap; gap: var(--s-3); margin-top: var(--s-5); }
	.end.tight { margin-top: 0; }
	.ok-text { color: var(--ok); }
	.bad-text { color: var(--fail); }
	@media (max-width: 600px) {
		.pick > :global(.ui-btn), .end > :global(.ui-btn) { width: 100%; }
		.top, .found li { align-items: flex-start; flex-direction: column; }
	}
</style>
