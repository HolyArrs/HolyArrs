<script lang="ts">
	// Sheet that lists the releases found for a title and lets the user pick or reject one.
	import Sheet from '#lib/ui/Sheet.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { keyReasons, verdict, verdictTone, sortReleases, releaseSorts, resolutionsOf, resolutionWord, nameMatches, sourcesOf, languagesOf, type ReleaseSort } from './releases.ts';
	import ReleaseTags from './ReleaseTags.svelte';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { api, message, ApiError } from '#lib/core/api.ts';
	import { bytes, agoLong } from '#lib/core/format.ts';
	import type { Choice, LastSearch, ReleasePage, ReleaseView } from '#lib/core/types.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import { chosenToast } from './chosen-toast.ts';
	import { releasePage } from './release-link.ts';
	import type { ReleaseCheck } from './releases.ts';
	import type { Snippet } from 'svelte';

	// check: releases of a movie not in the Library yet; it loads, searches and picks in place of the title's own list, and head holds its choices.
	let { open = $bindable(false), titleId, name, scope = null, show = false, onchosen, check, head }: { open?: boolean; titleId: number; name: string; scope?: { label: string; query: string; body: Record<string, unknown> } | null; show?: boolean; onchosen?: () => void; check?: ReleaseCheck; head?: Snippet } = $props();
	// A show is searched by season or episode: the whole-show sheet lists what was found and says where to search.
	const partOnly = $derived(show && !scope);

	let releases = $state<ReleaseView[]>([]);
	let total = $state(0);
	let last = $state<LastSearch | null>(null);
	let running = $state<number[]>([]);
	let rulesChanged = $state(false);
	let loading = $state(false);
	let busyId = $state(0);
	let confirmId = $state(0);
	let filter = $state<'good' | 'all'>('good');
	// Sort is remembered on this device; resolution and name filters reset with the sheet.
	const sortKey = 'holyarrs.releaseSort';
	let sortBy = $state<ReleaseSort>(remembered());
	let res = $state(0);
	let q = $state('');
	let src = $state('');
	let lang = $state('');
	let whyId = $state(0);
	let limit = $state(100);
	function remembered(): ReleaseSort {
		try {
			const v = localStorage.getItem(sortKey) as ReleaseSort | null;
			return v && releaseSorts.includes(v) ? v : 'best';
		} catch {
			return 'best';
		}
	}
	function setSort(v: ReleaseSort) {
		sortBy = v;
		try {
			localStorage.setItem(sortKey, v);
		} catch {
			/* private window: the sort still works for this visit */
		}
	}

	// Good view hides failed and turned down ones; the resolution chip and name text narrow it further.
	const sorted = $derived(
		sortReleases(
			releases.filter((r) => (filter === 'all' || (r.passed && r.state !== 'turned_down')) && (!res || r.read?.resolution === res) && (!src || r.source_name === src) && (!lang || !!r.read?.languages?.includes(lang)) && (!q.trim() || nameMatches(r.name, q))),
			sortBy
		)
	);
	const resolutions = $derived(resolutionsOf(releases));
	const sources = $derived(sourcesOf(releases));
	const languages = $derived(languagesOf(releases));
	const hidden = $derived(releases.length - releases.filter((r) => r.passed && r.state !== 'turned_down').length);

	$effect(() => {
		if (open) void load();
	});
	// Poll while a search runs so results show as they arrive.
	$effect(() => {
		if (!open || !running.length) return;
		const t = setInterval(load, 3000);
		return () => clearInterval(t);
	});

	// Pick rules are often changed in another tab: look again on coming back so the verdicts are not stale.
	$effect(() => {
		if (!open) return;
		const back = () => document.visibilityState === 'visible' && void load();
		document.addEventListener('visibilitychange', back);
		return () => document.removeEventListener('visibilitychange', back);
	});

	// Counts searches started, so a list asked for before one started does not drop its jobs.
	let searches = 0;
	async function load() {
		loading = true;
		const seen = searches;
		try {
			// Demo data only shows in dev builds.
			const p = check ? await check.load() : import.meta.env.DEV && new URLSearchParams(location.search).get('releases') === 'demo' ? demoPage(name) : await api.get<ReleasePage>(`/titles/${titleId}/releases?${scope ? scope.query + '&' : ''}limit=${limit}&offset=0`);
			releases = p.releases;
			total = p.total;
			last = p.last_search ?? null;
			rulesChanged = !!p.rules_changed;
			// Jobs are written before the search answers, so the server's list is the truth
			// unless a search started while this list was on its way.
			running = seen === searches ? (p.running_job_ids ?? []) : [...new Set([...(p.running_job_ids ?? []), ...running])];
		} catch (e) {
			toasts.push(t('mediafiles.releaseSheet.loadFailed'), 'fail', { body: message(e) });
		} finally {
			loading = false;
		}
	}
	async function search() {
		try {
			searches++;
			const ids = check ? await check.search() : (await api.post<{ job_ids: number[] }>(`/titles/${titleId}/search`, { ...(scope?.body ?? {}), choose: false })).job_ids;
			running = [...new Set([...running, ...ids])];
		} catch (e) {
			toasts.push(t('mediafiles.title.startFailed'), 'fail', { body: message(e) });
		}
	}
	async function choose(r: ReleaseView) {
		// A forced pick needs a second click to confirm.
		if (r.needs_force && confirmId !== r.id) return void (confirmId = r.id);
		busyId = r.id;
		try {
			if (check) {
				await check.pick(r);
				open = false;
				return;
			}
			// Send the Slot the sheet was opened for; with none, the server picks the Version whose rules the Release fits.
			const body: Record<string, unknown> = { force: r.needs_force };
			if (typeof scope?.body.slot_id === 'number') body.slot_id = scope.body.slot_id;
			const ch = await api.post<Choice>(`/releases/${r.id}/choose`, body);
			chosenToast(ch);
			open = false;
			onchosen?.();
		} catch (e) {
			// The server can demand force even when the row did not say so.
			if (e instanceof ApiError && e.status === 409 && e.body.needs_force) {
				r.needs_force = true;
				confirmId = r.id;
			} else toasts.push(t('mediafiles.releaseSheet.sendFailed'), 'fail', { body: message(e) });
		} finally {
			busyId = 0;
		}
	}
	async function turnDown(r: ReleaseView) {
		busyId = r.id;
		try {
			await api.post(`/releases/${r.id}/turn-down`);
			toasts.push(t('mediafiles.title.turnedDown'), 'ok', { body: t('mediafiles.releaseSheet.neverPickedTitle') });
			await load();
		} catch (e) {
			toasts.push(t('mediafiles.releaseSheet.turnDownFailed'), 'fail', { body: message(e) });
		} finally {
			busyId = 0;
		}
	}

	// Blocks the release group in the Pick rules this title uses and turns this release down.
	async function blockGroup(r: ReleaseView, group: string) {
		busyId = r.id;
		try {
			await api.post(`/releases/${r.id}/turn-down`, { block_group: true });
			toasts.push(t('mediafiles.releaseSheet.groupBlocked', { group }), 'ok', { body: t('mediafiles.releaseSheet.groupBlockedHint') });
			await load();
		} catch (e) {
			toasts.push(t('mediafiles.releaseSheet.blockGroupFailed'), 'fail', { body: message(e) });
		} finally {
			busyId = 0;
		}
	}

	// Sample releases for the dev demo view.
	function demoPage(t: string): ReleasePage {
		const n = t.replace(/[^A-Za-z0-9]+/g, '.');
		const day = 864e5;
		const mk = (id: number, name: string, gb: number, seeders: number, src: string, days: number, o: Partial<ReleaseView>, why: { effect: string; text: string }[] = []): ReleaseView =>
			({ id: -id, name, size: gb * 1024 ** 3, seeders, protocol: 'torrent', state: 'judged', found_by: 'search', source_id: 1, source_name: src, published_at: new Date(Date.now() - days * day).toISOString(), outcome: 'passed_over', passed: true, rank: id, points: 100 - id * 7, good_enough: true, needs_force: false, reasons: why, ...o }) as ReleaseView;
		const releases = [
			mk(1, `${n}.1979.2160p.UHD.BluRay.REMUX.DV.HDR.HEVC.Atmos-FGT`, 68.4, 41, 'Torrent Hub', 210, { outcome: 'chosen' }, [{ effect: 'rank', text: 'Best match for your quality: 4K Remux with Dolby Vision' }]),
			mk(2, `${n}.1979.2160p.BluRay.x265.10bit.HDR.DTS-HD.MA-SWTYBLZ`, 22.1, 118, 'Torrent Hub', 340, {}, [{ effect: 'rank', text: 'Smaller 4K copy, ranked second' }]),
			mk(3, `${n}.1979.1080p.BluRay.x264-SPARKS`, 12.3, 402, 'Usenet One', 1900, {}, [{ effect: 'rank', text: 'HD, ranked below the 4K copies' }]),
			mk(4, `${n}.1979.1080p.WEB-DL.DDP5.1.H.264-NTb`, 7.9, 66, 'Usenet One', 820, {}),
			mk(5, `${n}.1979.720p.BluRay.x264-AMIABLE`, 4.4, 3, 'Torrent Hub', 3100, {}, [{ effect: 'warn', text: 'Only 3 seeders, may be slow' }]),
			mk(6, `${n}.1979.2160p.WEB.H265-NAISU`, 15.2, 0, 'Torrent Hub', 2, { outcome: 'held', passed: true }, [{ effect: 'hold', text: 'Waiting 2 more days in case a better copy shows up' }]),
			mk(7, `${n}.1979.1080p.HDCAM.x264-RARBG`, 2.1, 950, 'Torrent Hub', 400, { outcome: 'refused', passed: false, needs_force: true }, [{ effect: 'fail', text: 'Camera recording, never picked' }]),
			mk(8, `${n}.1979.MULTi.1080p.BluRay.x264-UKDHD`, 14.6, 12, 'Usenet One', 1200, { outcome: 'refused', passed: false, needs_force: true }, [{ effect: 'fail', text: 'Audio is French first, you asked for English' }])
		];
		return { releases, total: releases.length, limit: 60, offset: 0, running_job_ids: [] };
	}

	// Rough age words for a release row.
	function age(iso: string): string {
		const d = Math.max(0, (Date.now() - Date.parse(iso)) / 864e5);
		return d < 1 ? t('mediafiles.releaseSheet.today') : d < 60 ? t('mediafiles.releaseSheet.days', { n: Math.round(d) }) : d < 365 ? t('mediafiles.releaseSheet.months', { n: Math.round(d / 30) }) : t('mediafiles.releaseSheet.years', { n: Math.round(d / 365) });
	}

	async function more() {
		limit = Math.min(500, limit + 100);
		await load();
	}
	// Lower weight shows first, so blockers come before ranking notes.
	const weight: Record<string, number> = { fail: 0, hold: 1, rank: 2, warn: 3 };
	const deciding = (r: ReleaseView) => keyReasons(r.reasons).sort((x, y) => (weight[x.effect] ?? 9) - (weight[y.effect] ?? 9)).slice(0, 2);
</script>

<Sheet bind:open title={check ? t('discover.titleOwned.checkReleases') : t('mediafiles.releaseSheet.choose')} description={scope ? `${name} · ${scope.label}` : name} size="lg" steady>
	<div class="rs" data-testid="release-sheet">
		{@render head?.()}
		<div class="rs-bar">
			<div class="rs-seg" role="tablist">
				<button type="button" role="tab" class:on={filter === 'good'} aria-selected={filter === 'good'} onclick={() => (filter = 'good')}>{t('mediafiles.releaseSheet.good')}</button>
				<button type="button" role="tab" class:on={filter === 'all'} aria-selected={filter === 'all'} onclick={() => (filter = 'all')}>{t('mediafiles.releaseSheet.all')} {total || ''}</button>
			</div>
			{#if !partOnly}<button type="button" class="rs-search" disabled={running.length > 0} onclick={search}><Icon name="search" size={15} />{running.length ? t('mediafiles.releaseSheet.searchingDots') : t('mediafiles.releaseSheet.searchAgain')}</button>{/if}
		</div>
		{#if releases.length}
			<div class="rs-tools">
				<label class="rs-sort"><span>{t('mediafiles.releaseSheet.sortLabel')}</span>
					<select value={sortBy} onchange={(e) => setSort(e.currentTarget.value as ReleaseSort)} data-testid="release-sort">
						{#each releaseSorts as k (k)}<option value={k}>{t(`mediafiles.releaseSheet.sort.${k}`)}</option>{/each}
					</select>
				</label>
				{#if resolutions.length > 1}
					<div class="rs-chips" role="group" aria-label={t('mediafiles.releaseSheet.sort.quality')}>
						<button type="button" class:on={!res} aria-pressed={!res} onclick={() => (res = 0)}>{t('mediafiles.releaseSheet.anyQuality')}</button>
						{#each resolutions as n (n)}<button type="button" class:on={res === n} aria-pressed={res === n} onclick={() => (res = res === n ? 0 : n)}>{resolutionWord(n)}</button>{/each}
					</div>
				{/if}
				{#if sources.length > 1}
					<label class="rs-sort"><span class="sr-only">{t('mediafiles.releaseSheet.sourceLabel')}</span>
						<select bind:value={src} data-testid="release-source">
							<option value="">{t('mediafiles.releaseSheet.anySource')}</option>
							{#each sources as n (n)}<option value={n}>{n}</option>{/each}
						</select>
					</label>
				{/if}
				{#if languages.length}
					<label class="rs-sort"><span class="sr-only">{t('mediafiles.releaseSheet.languageLabel')}</span>
						<select bind:value={lang} data-testid="release-language">
							<option value="">{t('mediafiles.releaseSheet.anyLanguage')}</option>
							{#each languages as l (l)}<option value={l}>{l.toUpperCase()}</option>{/each}
						</select>
					</label>
				{/if}
				<input class="rs-q" type="search" bind:value={q} placeholder={t('mediafiles.releaseSheet.filterName')} aria-label={t('mediafiles.releaseSheet.filterName')} data-testid="release-filter" />
			</div>
		{/if}
		{#if rulesChanged && releases.length && !running.length}<div class="rs-run" role="status" data-testid="release-rules-changed"><Icon name="info" size={15} /><span>{t('mediafiles.releaseSheet.rulesChanged')}</span></div>{/if}
		{#if running.length}<div class="rs-run" aria-live="polite"><span class="rs-spin" aria-hidden="true"></span>{t('mediafiles.releaseSheet.asking')}</div>{/if}

		{#if loading && !releases.length}
			<ul class="rs-list">{#each [0, 1, 2, 3] as i (i)}<li class="rs-row skel"></li>{/each}</ul>
		{:else if !releases.length && !running.length}
			<div class="rs-empty">
				<Icon name="grid" size={28} />
				{#if last?.failed}
					<p><b>{t('mediafiles.releaseSheet.searchFailed')}</b></p>
					<p class="muted" data-testid="release-sheet-last">{t('mediafiles.releaseSheet.searchFailedHint', { when: agoLong(last.at) })}</p>
				{:else if last?.dropped}
					<p><b>{t('mediafiles.releaseSheet.allDropped')}</b></p>
					<p class="muted" data-testid="release-sheet-last">{t('mediafiles.releaseSheet.allDroppedHint', { when: agoLong(last.at), sources: (last.dropped_sources ?? []).join(', '), n: last.dropped })}</p>
				{:else if last}
					<p><b>{t('mediafiles.releaseSheet.foundNone')}</b></p>
					<p class="muted" data-testid="release-sheet-last">{t('mediafiles.releaseSheet.foundNoneHint', { when: agoLong(last.at) })}</p>
				{:else}
					<p><b>{t('mediafiles.releaseSheet.noSearch')}</b></p>
				{/if}
				{#if partOnly}
					<p class="muted" data-testid="release-sheet-part">{t('mediafiles.releaseSheet.showPickPart', { choose: t('discover.titleOwned.chooseRelease') })}</p>
				{:else}
					{#if !last}<p class="muted">{check ? t('mediafiles.releaseSheet.checkHint') : t('mediafiles.releaseSheet.noSearchHint')}</p>{/if}
					<button type="button" class="rs-primary" onclick={search} data-testid="release-sheet-search"><Icon name="search" size={16} />{check ? t('discover.titleOwned.checkReleases') : t('mediafiles.select.searchNow')}</button>
				{/if}
			</div>
		{:else}
			<ul class="rs-list">
				{#each sorted as r (r.id)}
					{@const why = deciding(r)}
					{@const page = releasePage(r.info_url)}
					{@const best = r.outcome === 'chosen' && r.state !== 'chosen' && r.state !== 'turned_down'}
					<li class="rs-row" class:best class:off={!r.passed || r.state === 'turned_down'} data-testid="release">
						<div class="rs-main">
							<p class="rs-tags">
								{#if best}<span class="rs-rec">{t('mediafiles.releaseSheet.recommended')}</span>{/if}
								<ReleaseTags release={r} />
								{#if !best}<span class="rs-verdict {verdictTone(r)}" data-testid="verdict">{verdict(r)}</span>{/if}
							</p>
							<p class="rs-name" title={r.name}>{#if page}<a class="rs-link" href={page} target="_blank" rel="noopener noreferrer" aria-label={t('mediafiles.releaseSheet.openPage', { name: r.name, source: r.source_name })} data-testid="release-page-link">{r.name}<Icon name="arrowUpRight" size={13} /></a>{:else}{r.name}{/if}</p>
							<p class="rs-facts">
								<span>{bytes(r.size)}</span>
								{#if r.seeders != null}<span class:low={r.seeders < 5}>{t('mediafiles.release.seeders', { n: r.seeders })}</span>{/if}
								<span>{r.source_name}</span>
								<span>{r.protocol === 'usenet' ? 'Usenet' : 'Torrent'}</span>
								{#if r.passed || r.points}<span data-testid="release-points">{t('mediafiles.release.points', { n: r.points })}</span>{/if}
								{#if r.published_at}<span>{age(r.published_at)}</span>{/if}
							</p>
							{#if whyId === r.id || confirmId === r.id}
								<ul class="rs-all" data-testid="release-reasons">{#each r.reasons as w, i (i)}<li class="rs-why {w.effect}">{serverWords(w.text)}</li>{/each}</ul>
							{:else}
								{#each why as w, i (i)}<p class="rs-why {w.effect}">{serverWords(w.text)}</p>{/each}
							{/if}
							{#if confirmId === r.id}<p class="rs-warn" role="alert">{t('mediafiles.releaseSheet.anywayWarn')}</p>{/if}
							{#if r.reasons.length > why.length && confirmId !== r.id}<button type="button" class="rs-link rs-whybtn" aria-expanded={whyId === r.id} onclick={() => (whyId = whyId === r.id ? 0 : r.id)}>{whyId === r.id ? t('mediafiles.releaseSheet.whyHide') : t('mediafiles.releaseSheet.whyAll', { n: r.reasons.length })}</button>{/if}
						</div>
						<div class="rs-act">
							{#if r.state === 'chosen'}
								<span class="rs-sent"><Icon name="check" size={15} />{t('mediafiles.releaseSheet.sent')}</span>
							{:else}
								<button type="button" class="rs-choose" class:quiet={!best} class:warn={confirmId === r.id} disabled={busyId === r.id} onclick={() => choose(r)}>{confirmId === r.id ? t('mediafiles.releaseSheet.pickAnyway') : t('mediafiles.releaseSheet.pick')}</button>
								{#if r.state !== 'turned_down' && !check}<button type="button" class="rs-x" aria-label={t('mediafiles.releaseSheet.neverPick', { name: r.name })} title={t('mediafiles.releaseSheet.neverPickThis')} disabled={busyId === r.id} onclick={() => turnDown(r)}><Icon name="x" size={14} /></button>{/if}
								{#if r.state !== 'turned_down' && !check && r.read?.group}<button type="button" class="rs-x" aria-label={t('mediafiles.releaseSheet.blockGroup', { group: r.read.group })} title={t('mediafiles.releaseSheet.blockGroup', { group: r.read.group })} disabled={busyId === r.id} onclick={() => blockGroup(r, r.read?.group ?? '')} data-testid="release-block-group"><Icon name="declined" size={14} /></button>{/if}
							{/if}
						</div>
					</li>
				{:else}
					<li class="rs-none muted">{#if filter === 'good'}{t('mediafiles.releaseSheet.noMatches')}{' '}{#if hidden}<button type="button" class="rs-link" onclick={() => (filter = 'all')}>{t('mediafiles.releaseSheet.showHidden', { n: hidden })}</button>{/if}{:else}{t('mediafiles.releaseSheet.noFilterMatches')}{/if}</li>
				{/each}
			</ul>
			{#if total > releases.length && limit < 500}<button type="button" class="rs-search rs-more" disabled={loading} onclick={more}>{t('mediafiles.releaseSheet.showMore', { shown: releases.length, total })}</button>{/if}
		{/if}
	</div>
</Sheet>

<style>
	.rs { display: grid; gap: 14px; }
	.rs-bar { display: flex; align-items: center; justify-content: space-between; gap: 10px; flex-wrap: wrap; row-gap: 8px; }
	.rs-seg { display: inline-flex; padding: 3px; border-radius: 999px; background: color-mix(in srgb, var(--text) 7%, transparent); }
	.rs-seg button { height: 30px; padding: 0 14px; border: 0; border-radius: 999px; background: none; color: var(--text); font: 600 13px/1 var(--font-ui); cursor: pointer; white-space: nowrap; }
	.rs-seg button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (pointer: coarse) { .rs-seg button { height: 40px; } }
	.rs-seg button.on { background: var(--surface-1, var(--bg)); color: var(--text); box-shadow: 0 1px 3px rgb(0 0 0 / 18%); }
	.rs-search, .rs-primary { display: inline-flex; align-items: center; gap: 7px; height: 34px; padding: 0 14px; border: 0; border-radius: 999px; background: color-mix(in srgb, var(--text) 8%, transparent); color: var(--text); font: 600 13px/1 var(--font-ui); cursor: pointer; white-space: nowrap; }
	.rs-primary { height: 40px; padding: 0 18px; background: var(--text); color: var(--bg); font-size: 14px; }
	.rs-run { display: flex; align-items: center; gap: 10px; padding: 10px 14px; border-radius: 12px; background: color-mix(in srgb, var(--info) 12%, transparent); font: 500 13px/1.3 var(--font-ui); }
	.rs-spin { width: 14px; height: 14px; border-radius: 99px; border: 2px solid color-mix(in srgb, var(--info) 30%, transparent); border-top-color: var(--info); animation: spin .8s linear infinite; }
	@keyframes spin { to { transform: rotate(360deg); } }
	.rs-list { display: grid; gap: 8px; margin: 0; padding: 0; list-style: none; }
	.rs-row { display: flex; align-items: center; gap: 14px; padding: 14px 16px; border-radius: 14px; background: var(--surface-2); }
	.rs-row.best { box-shadow: inset 0 0 0 1.5px color-mix(in srgb, var(--ok) 70%, transparent); }
	.rs-row.off { color: var(--text-muted); }
	.rs-row.skel { height: 92px; animation: pulse 1.2s ease-in-out infinite alternate; }
	@keyframes pulse { to { opacity: .5; } }
	.rs-main { flex: 1; min-width: 0; display: grid; gap: 6px; }
	.rs-main p { margin: 0; }
	.rs-tags { display: flex; flex-wrap: wrap; gap: 5px; }
	.rs-rec, .rs-verdict { display: inline-flex; align-items: center; height: 20px; padding: 0 7px; border-radius: 5px; font: 700 11px/1 var(--font-ui); letter-spacing: .03em; }
	.rs-rec { background: var(--ok-fill); color: #fff; }
	.rs-verdict { color: var(--text); background: color-mix(in srgb, var(--text) 7%, transparent); }
	.rs-verdict.fail { color: var(--fail-text); background: color-mix(in srgb, var(--fail) 12%, transparent); }
	.rs-verdict.wait { color: var(--wait-text); background: color-mix(in srgb, var(--wait) 14%, transparent); }
	.rs-verdict.red { color: #fff; background: var(--accent-fill); }
	.rs-link { color: inherit; text-decoration: none; }
	.rs-link :global(svg) { display: inline-block; vertical-align: -1px; margin-inline-start: 4px; opacity: 0.55; }
	.rs-link:hover { text-decoration: underline; text-underline-offset: 3px; }
	.rs-link:hover :global(svg) { opacity: 1; }
	.rs-link:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 4px; }
	.rs-name { font: 600 14px/1.35 var(--font-ui); overflow: hidden; overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; }
	.rs-tools { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 10px; }
	.rs-sort { display: inline-flex; align-items: center; gap: 6px; color: var(--text-muted); font: 600 12.5px/1 var(--font-ui); }
	.rs-sort select, .rs-q { height: 34px; padding: 0 12px; border: 0; border-radius: 999px; background: color-mix(in srgb, var(--text) 7%, transparent); color: var(--text); font: 600 13px/1 var(--font-ui); }
	.rs-q { flex: 1 1 160px; min-width: 0; }
	.rs-chips { display: inline-flex; flex-wrap: wrap; gap: 6px; }
	.rs-chips button { height: 30px; padding: 0 12px; border: 1px solid color-mix(in srgb, var(--text) 18%, transparent); border-radius: 999px; background: none; color: var(--text); font: 600 12.5px/1 var(--font-ui); cursor: pointer; }
	.rs-chips button.on { background: var(--text); color: var(--bg); border-color: transparent; }
	.rs-all { display: grid; gap: 4px; margin: 0; padding: 0; list-style: none; }
	.rs-warn { color: var(--wait-text); font: 600 12.5px/1.35 var(--font-ui); }
	.rs-whybtn { justify-self: start; font: 600 12.5px/1.35 var(--font-ui); }
	.rs-more { justify-self: center; }
	@media (pointer: coarse) { .rs-x { width: 44px; height: 44px; } .rs-choose, .rs-sort select, .rs-q { height: 44px; } .rs-chips button { height: 40px; } }
	.rs-facts { display: flex; flex-wrap: wrap; gap: 4px 12px; color: var(--text-muted); font: 500 12.5px/1.2 var(--font-ui); }
	.rs-facts .low { color: var(--wait-text); }
	.rs-why { color: var(--text-muted); font: 500 12.5px/1.35 var(--font-ui); }
	.rs-why.fail { color: var(--fail-text); }
	.rs-why.hold, .rs-why.warn { color: var(--wait-text); }
	.rs-act { display: flex; align-items: center; gap: 8px; flex: none; }
	.rs-choose { height: 34px; padding: 0 16px; border: 0; border-radius: 999px; background: var(--text); color: var(--bg); font: 600 13px/1 var(--font-ui); cursor: pointer; white-space: nowrap; }
	.rs-choose.quiet { background: color-mix(in srgb, var(--text) 10%, transparent); color: var(--text); }
	.rs-choose.warn { background: var(--wait); color: #111; }
	.rs-x { display: grid; place-items: center; width: 32px; height: 32px; border: 0; border-radius: 999px; background: none; color: var(--text-muted); cursor: pointer; }
	.rs-x:hover { background: color-mix(in srgb, var(--text) 8%, transparent); }
	.rs-sent { display: inline-flex; align-items: center; gap: 5px; color: var(--ok-text); font: 600 13px/1 var(--font-ui); }
	.rs-empty { display: grid; justify-items: center; gap: 6px; padding: 48px 16px; text-align: center; color: var(--text); }
	.rs-empty p { margin: 0; max-width: 34ch; }
	.rs-empty .rs-primary { margin-top: 12px; }
	.rs-none { padding: 24px 4px; font: 500 14px/1.4 var(--font-ui); }
	.rs-link { padding: 0; border: 0; background: none; color: var(--accent-text); font: inherit; cursor: pointer; }
	.rs-link.rs-whybtn { color: var(--text-muted); }
	.rs-link.rs-whybtn:hover { color: var(--text); }
	@media (max-width: 600px) { .rs-row { flex-direction: column; align-items: stretch; padding: 12px 12px 12px 14px; gap: 10px; } .rs-act { justify-content: flex-end; } .rs-name { -webkit-line-clamp: 4; line-clamp: 4; } .rs-choose { padding: 0 14px; } }
	@media (prefers-reduced-motion: reduce) { .rs-spin, .rs-row.skel { animation: none; } }
</style>
