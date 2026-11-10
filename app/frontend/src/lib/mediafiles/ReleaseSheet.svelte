<script lang="ts">
	// Sheet that lists the releases found for a title and lets the user pick or reject one.
	import Sheet from '#lib/ui/Sheet.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { keyReasons, verdict, verdictTone } from './releases.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { api, message, ApiError } from '#lib/core/api.ts';
	import { bytes } from '#lib/core/format.ts';
	import type { Choice, ReleasePage, ReleaseView } from '#lib/core/types.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	let { open = $bindable(false), titleId, name, scope = null, onchosen }: { open?: boolean; titleId: number; name: string; scope?: { label: string; query: string; body: Record<string, unknown> } | null; onchosen?: () => void } = $props();

	let releases = $state<ReleaseView[]>([]);
	let total = $state(0);
	let running = $state<number[]>([]);
	let loading = $state(false);
	let busyId = $state(0);
	let confirmId = $state(0);
	let filter = $state<'good' | 'all'>('good');
	// Time of the last search start, so an empty list is not read as done too early.
	let startedAt = 0;

	// Chosen first, then passed, then best rank. Good view hides failed and turned down ones.
	const sorted = $derived(
		[...releases]
			.filter((r) => filter === 'all' || (r.passed && r.state !== 'turned_down'))
			.sort((a, b) => Number(b.outcome === 'chosen') - Number(a.outcome === 'chosen') || Number(b.passed) - Number(a.passed) || (a.rank || 999) - (b.rank || 999) || b.points - a.points)
	);
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

	async function load() {
		loading = true;
		try {
			// Demo data only shows in dev builds.
			const p = import.meta.env.DEV && new URLSearchParams(location.search).get('releases') === 'demo' ? demoPage(name) : await api.get<ReleasePage>(`/titles/${titleId}/releases?${scope ? scope.query + '&' : ''}limit=60&offset=0`);
			releases = p.releases;
			total = p.total;
			// Keep our own job ids for a minute: the server may not list them yet.
			const mine = Date.now() - startedAt < 60_000 && !p.releases.length ? running : [];
			running = [...new Set([...(p.running_job_ids ?? []), ...mine])];
		} catch (e) {
			toasts.push(t('mediafiles.releaseSheet.loadFailed'), 'fail', { body: message(e) });
		} finally {
			loading = false;
		}
	}
	async function search() {
		try {
			startedAt = Date.now();
			const r = await api.post<{ job_ids: number[] }>(`/titles/${titleId}/search`, { ...(scope?.body ?? {}), choose: false });
			running = [...new Set([...running, ...r.job_ids])];
		} catch (e) {
			toasts.push(t('mediafiles.title.startFailed'), 'fail', { body: message(e) });
		}
	}
	async function choose(r: ReleaseView) {
		// A forced pick needs a second click to confirm.
		if (r.needs_force && confirmId !== r.id) return void (confirmId = r.id);
		busyId = r.id;
		try {
			const ch = await api.post<Choice>(`/releases/${r.id}/choose`, { force: r.needs_force });
			toasts.push(t('mediafiles.title.sent'), 'ok', ch.warnings.length ? { body: ch.warnings.length === 1 ? ch.warnings[0] : t('mediafiles.title.warnings', { n: ch.warnings.length }) } : undefined);
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

	// Pull the quality tags out of a release name.
	function tags(n: string): string[] {
		const s = n.toLowerCase();
		const out: string[] = [];
		if (/2160p|\b4k\b|uhd/.test(s)) out.push('4K');
		else if (/1080p/.test(s)) out.push('1080p');
		else if (/720p/.test(s)) out.push('720p');
		if (/dolby.?vision|\bdv\b|dovi/.test(s)) out.push('Dolby Vision');
		else if (/hdr/.test(s)) out.push('HDR');
		if (/remux/.test(s)) out.push('Remux');
		else if (/blu-?ray|bdrip/.test(s)) out.push('BluRay');
		else if (/web-?dl|webrip|\bweb\b/.test(s)) out.push('WEB');
		if (/x265|h\.?265|hevc/.test(s)) out.push('x265');
		if (/atmos/.test(s)) out.push('Atmos');
		return out;
	}
	// Lower weight shows first, so blockers come before ranking notes.
	const weight: Record<string, number> = { fail: 0, hold: 1, rank: 2, warn: 3 };
	const deciding = (r: ReleaseView) => keyReasons(r.reasons).sort((x, y) => (weight[x.effect] ?? 9) - (weight[y.effect] ?? 9)).slice(0, 2);
</script>

<Sheet bind:open title={t('mediafiles.releaseSheet.choose')} description={scope ? `${name} · ${scope.label}` : name} size="lg">
	<div class="rs" data-testid="release-sheet">
		<div class="rs-bar">
			<div class="rs-seg" role="tablist">
				<button type="button" role="tab" class:on={filter === 'good'} aria-selected={filter === 'good'} onclick={() => (filter = 'good')}>{t('mediafiles.releaseSheet.good')}</button>
				<button type="button" role="tab" class:on={filter === 'all'} aria-selected={filter === 'all'} onclick={() => (filter = 'all')}>{t('mediafiles.releaseSheet.all')} {total || ''}</button>
			</div>
			<button type="button" class="rs-search" disabled={running.length > 0} onclick={search}><Icon name="search" size={15} />{running.length ? t('mediafiles.releaseSheet.searchingDots') : t('mediafiles.releaseSheet.searchAgain')}</button>
		</div>
		{#if running.length}<div class="rs-run" aria-live="polite"><span class="rs-spin" aria-hidden="true"></span>{t('mediafiles.releaseSheet.asking')}</div>{/if}

		{#if loading && !releases.length}
			<ul class="rs-list">{#each [0, 1, 2, 3] as i (i)}<li class="rs-row skel"></li>{/each}</ul>
		{:else if !releases.length && !running.length}
			<div class="rs-empty">
				<Icon name="grid" size={28} />
				<p><b>{t('mediafiles.releaseSheet.noSearch')}</b></p>
				<p class="muted">{t('mediafiles.releaseSheet.noSearchHint')}</p>
				<button type="button" class="rs-primary" onclick={search}><Icon name="search" size={16} />{t('mediafiles.select.searchNow')}</button>
			</div>
		{:else}
			<ul class="rs-list">
				{#each sorted as r (r.id)}
					{@const why = deciding(r)}
					{@const best = r.outcome === 'chosen' && r.state !== 'chosen' && r.state !== 'turned_down'}
					<li class="rs-row" class:best class:off={!r.passed || r.state === 'turned_down'} data-testid="release">
						<div class="rs-main">
							<p class="rs-tags">
								{#if best}<span class="rs-rec">{t('mediafiles.releaseSheet.recommended')}</span>{/if}
								{#each tags(r.name) as t (t)}<span class="rs-tag">{t}</span>{/each}
								{#if !best && !(r.passed && r.outcome === 'passed_over' && r.state !== 'turned_down')}<span class="rs-verdict {verdictTone(r)}">{verdict(r)}</span>{/if}
							</p>
							<p class="rs-name" title={r.name}>{r.name}</p>
							<p class="rs-facts">
								<span>{bytes(r.size)}</span>
								{#if r.seeders != null}<span class:low={r.seeders < 5}>{t('mediafiles.release.seeders', { n: r.seeders })}</span>{/if}
								<span>{r.source_name}</span>
								{#if r.published_at}<span>{age(r.published_at)}</span>{/if}
							</p>
							{#each why as w, i (i)}<p class="rs-why {w.effect}">{w.text}</p>{/each}
						</div>
						<div class="rs-act">
							{#if r.state === 'chosen'}
								<span class="rs-sent"><Icon name="check" size={15} />{t('mediafiles.releaseSheet.sent')}</span>
							{:else}
								<button type="button" class="rs-choose" class:quiet={!best} class:warn={confirmId === r.id} disabled={busyId === r.id} onclick={() => choose(r)}>{confirmId === r.id ? t('mediafiles.releaseSheet.pickAnyway') : t('mediafiles.releaseSheet.pick')}</button>
								{#if r.state !== 'turned_down'}<button type="button" class="rs-x" aria-label={t('mediafiles.releaseSheet.neverPick', { name: r.name })} title={t('mediafiles.releaseSheet.neverPickThis')} disabled={busyId === r.id} onclick={() => turnDown(r)}><Icon name="x" size={14} /></button>{/if}
							{/if}
						</div>
					</li>
				{:else}
					<li class="rs-none muted">{t('mediafiles.releaseSheet.noMatches')}{#if hidden} <button type="button" class="rs-link" onclick={() => (filter = 'all')}>{t('mediafiles.releaseSheet.showHidden', { n: hidden })}</button>{/if}</li>
				{/each}
			</ul>
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
	.rs-tag, .rs-rec, .rs-verdict { display: inline-flex; align-items: center; height: 20px; padding: 0 7px; border-radius: 5px; font: 700 11px/1 var(--font-ui); letter-spacing: .03em; }
	.rs-tag { border: 1px solid color-mix(in srgb, var(--text) 28%, transparent); color: var(--text-muted); }
	.rs-rec { background: var(--ok-fill); color: #fff; }
	.rs-verdict { color: var(--text); background: color-mix(in srgb, var(--text) 7%, transparent); }
	.rs-verdict.fail { color: var(--fail-text); background: color-mix(in srgb, var(--fail) 12%, transparent); }
	.rs-verdict.wait { color: var(--wait-text); background: color-mix(in srgb, var(--wait) 14%, transparent); }
	.rs-verdict.red { color: #fff; background: var(--accent-fill); }
	.rs-name { font: 600 14px/1.35 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
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
	.rs-link { padding: 0; border: 0; background: none; color: var(--accent); font: inherit; cursor: pointer; }
	@media (max-width: 600px) { .rs-row { padding: 12px 12px 12px 14px; gap: 10px; } .rs-choose { padding: 0 14px; } }
	@media (prefers-reduced-motion: reduce) { .rs-spin, .rs-row.skel { animation: none; } }
</style>
