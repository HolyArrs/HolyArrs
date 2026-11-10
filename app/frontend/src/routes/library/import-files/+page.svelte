<script lang="ts">
	import Icon from '#lib/ui/Icon.svelte';
	import Button from '#lib/ui/Button.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import Badge from '#lib/ui/Badge.svelte';
	import { LibraryToolPage, blocker, episodeCode, itemsFor, pickLine, resultLine, startPick, startTicked, type ByHandFile, type ByHandResult, type ByHandScan, type Mode, type Pick } from '#lib/mediafiles/index.ts';
	import { to } from '#lib/core/paths.ts';
	import { api, message } from '#lib/core/api.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { bytes } from '#lib/core/format.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import type { EpisodeView, TitleView } from '#lib/core/types.ts';
	import { t as tr } from '#lib/core/i18n/i18n.svelte.ts';

	let path = $state('');
	let scanning = $state(false);
	let scan = $state<ByHandScan | null>(null);
	let picks = $state<Record<string, Pick | null>>({});
	let ticked = $state<Record<string, boolean>>({});
	let results = $state<Record<string, ByHandResult>>({});
	let mode = $state<Mode>('copy');
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

	const files = $derived(scan?.files ?? []);
	const items = $derived(itemsFor(files, picks, ticked, mode));
	const seasons = $derived([...new Set(episodes.map((e) => e.season))].sort((a, b) => a - b));
	const seasonEps = $derived(episodes.filter((e) => e.season === season));

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
		if (p) void choose(p.title_id, p);
	}

	async function lookup(e: SubmitEvent) {
		e.preventDefault();
		if (!term.trim()) return;
		looking = true;
		lookError = '';
		try {
			const r = await api.get<{ titles: TitleView[] }>(`/library/search?q=${encodeURIComponent(term.trim())}&kind=all`);
			found = r.titles.filter((t) => t.kind === 'movie' || t.kind === 'show');
			if (!found.length) lookError = tr('library.byHand.nothingByName');
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
			chosen = t;
			versionId = from?.version_id ?? t.versions.find((v) => v.main)?.id ?? t.versions[0]?.id ?? 0;
			season = from?.season ?? seasons[0];
			episode = from?.episode;
		} catch (err) {
			lookError = message(err);
		}
	}

	function hasFile(): boolean {
		if (!chosen) return false;
		if (chosen.kind === 'show') {
			const ep = episodes.find((e) => e.season === season && e.episode === episode);
			return !!ep?.slots.find((s) => s.version_id === versionId)?.file_id;
		}
		return !!chosen.slots.find((s) => s.version_id === versionId)?.file;
	}

	function useMatch() {
		if (!editFor || !chosen) return;
		const show = chosen.kind === 'show';
		picks[editFor] = {
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
		editFor = null;
	}

	async function doImport() {
		if (!items.length) return;
		importing = true;
		error = '';
		try {
			const r = await api.post<{ results: ByHandResult[]; placed: number }>('/manual-import', { items });
			const next = { ...results };
			for (const x of r.results) {
				next[x.path] = x;
				if (x.outcome === 'placed') {
					ticked[x.path] = false;
					const p = picks[x.path];
					if (p) picks[x.path] = { ...p, has_file: true };
				}
			}
			results = next;
			const n = r.placed;
			toasts.push(tr(n === 1 ? 'library.byHand.importedOne' : 'library.byHand.importedMany', { n }), n ? 'ok' : 'info');
		} catch (err) {
			error = message(err);
		} finally {
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
					{tr(files.length === 1 ? 'library.byHand.filesInOne' : 'library.byHand.filesInMany', { n: files.length, path: scan.path })}
				</p>
				{#if scan.more}<p class="notice">{tr('library.byHand.onlyFirst', { n: files.length })}</p>{/if}
				<ul class="list">
					{#each files as f (f.path)}
						{@const p = picks[f.path] ?? null}
						{@const why = blocker(f, p)}
						{@const done = results[f.path]}
						<li data-testid="import-file">
							<div class="top">
								<label class="tick">
									<input type="checkbox" bind:checked={ticked[f.path]} disabled={!!why || importing} aria-label={tr('library.byHand.importName', { name: f.name })} />
									<span class="mono small path">{f.name}</span>
								</label>
								<span class="meta xs muted">{bytes(f.size)}{#if f.quality}{' · '}{f.quality}{/if}</span>
							</div>
							{#if f.kind === 'video'}
								<div class="match">
									<b class="small">{pickLine(p)}</b>
									{#if p && f.guess.title_id === p.title_id && f.guess.confidence > 0 && f.guess.confidence < 1}<Badge label={f.guess.confidence >= 0.7 ? tr('library.byHand.likely') : tr('library.byHand.guess')} />{/if}
									{#if !f.in_library && editFor !== f.path}<Button variant="plain" size="sm" onclick={() => openEditor(f)} disabled={importing}>{tr('shows.versions.change')}</Button>{/if}
								</div>
							{/if}
							{#if why}<p class="xs muted">{why}</p>{:else if f.note}<p class="xs muted">{f.note}</p>{/if}
							{#if done}
								<p class="xs" class:ok-text={done.outcome === 'placed'} class:bad-text={done.outcome !== 'placed'} role="status">
									{resultLine(done)}{#if done.title_id && done.outcome === 'placed'}{' · '}<a href={to(`/library/${done.title_id}`)}>{tr('mediafiles.poster.open')}</a>{/if}
								</p>
							{/if}
							{#if editFor === f.path}
								<div class="editor">
									<form class="look" onsubmit={lookup}>
										<label class="sr-only" for="look-{f.path}">{tr('library.byHand.lookFor', { name: f.name })}</label>
										<input id="look-{f.path}" class="input" bind:value={term} placeholder={tr('library.byHand.placeholder')} autocomplete="off" />
										<Button type="submit" busy={looking}>{tr('library.byHand.lookUp')}</Button>
										<Button variant="plain" onclick={() => (editFor = null)}>{tr('shows.page.close')}</Button>
									</form>
									{#if lookError}<p class="form-error" role="alert">{lookError}</p>{/if}
									{#if found.length}
										<ul class="found">
											{#each found as t (t.id)}
												<li>
													<span class="small">{t.name}{t.year ? ` (${t.year})` : ''} <span class="muted xs">{tr(t.kind === 'show' ? 'mediafiles.kindCap.show' : 'mediafiles.kindCap.movie')}</span></span>
													<Button variant="plain" size="sm" onclick={() => choose(t.id)}>{chosen?.id === t.id ? tr('library.byHand.picked') : tr('mediafiles.releaseSheet.pick')}</Button>
												</li>
											{/each}
										</ul>
									{/if}
									{#if chosen}
										<div class="row picks">
											{#if chosen.versions.length > 1}
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
										</div>
										<div class="row end tight">
											<span class="small muted">{chosen.name}{chosen.kind === 'show' && episode !== undefined ? ` · ${episodeCode(season, episode)}` : ''}</span>
											<Button variant="primary" size="sm" onclick={useMatch} disabled={chosen.kind === 'show' && (season === undefined || episode === undefined)}>{tr('library.byHand.useThis')}</Button>
										</div>
									{/if}
								</div>
							{/if}
						</li>
					{/each}
				</ul>
				<div class="row end">
					<fieldset class="modes">
						<legend class="xs muted">{tr('library.byHand.how')}</legend>
						<label class="checkline"><input type="radio" bind:group={mode} value="copy" /> <span><b>{tr('library.byHand.copy')}</b><br /><span class="hint">{tr('library.byHand.copyHint')}</span></span></label>
						<label class="checkline"><input type="radio" bind:group={mode} value="move" /> <span><b>{tr('library.byHand.move')}</b><br /><span class="hint">{tr('library.byHand.moveHint')}</span></span></label>
					</fieldset>
					<span class="small muted">{tr('library.byHand.ticked', { n: items.length })}</span>
					<Button variant="primary" icon="download" onclick={doImport} disabled={!items.length} busy={importing}>
						{tr(items.length === 1 ? 'library.byHand.importOne' : 'library.byHand.importMany', { n: items.length })}
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
	.modes { display: flex; flex-wrap: wrap; gap: var(--s-3); margin: 0 auto 0 0; padding: 0; border: 0; min-width: 0; }
	.ok-text { color: var(--ok); }
	.bad-text { color: var(--fail); }
	@media (max-width: 600px) {
		.pick > :global(.ui-btn), .end > :global(.ui-btn) { width: 100%; }
		.top, .found li { align-items: flex-start; flex-direction: column; }
		.modes { flex-direction: column; }
	}
</style>
