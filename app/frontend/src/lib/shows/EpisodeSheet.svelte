<script lang="ts">
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	// Side sheet for one episode: status, file facts, subtitles and history.
	import Sheet from '#lib/ui/Sheet.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import { qualityBadges } from '#lib/mediafiles/index.ts';
	import { SubtitlePanel } from '#lib/subtitles/index.ts';
	import type { TitleSubtitles } from '#lib/subtitles/index.ts';
	import { HistoryLine as CompactRow, isRoutine, groupDownloads } from '#lib/downloads/index.ts';
	import { goto } from '$app/navigation';
	import { to } from '#lib/core/paths.ts';
	import { api, message } from '#lib/core/api.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { ago, bytes } from '#lib/core/format.ts';
	import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
	import type { EventRecord, TitleView } from '#lib/core/types.ts';

	export type SheetEpisode = {
		id: number;
		season?: number;
		episode: number;
		name?: string;
		air_date?: string;
		runtime_minutes?: number;
		still?: string;
		keep_up_to_date?: boolean;
		slots: { id: number; state: string; state_label: string; file?: Record<string, any> | null }[];
	};

	let {
		open = $bindable(false),
		ep,
		titleId,
		tmdb,
		manage = false,
		show = null,
		onchange,
		ondelete
	}: { open?: boolean; ep: SheetEpisode | null; titleId: number; tmdb?: string; manage?: boolean; show?: TitleView | null; onchange?: () => void; ondelete?: () => void } = $props();

	let story = $state('');
	let events = $state<EventRecord[]>([]);
	let busy = $state('');
	let keep = $state(true);
	let subs = $state<TitleSubtitles | null>(null);
	// Custom opens the full subtitle list.
	let subsMore = $state(false);

	// Routine events are left out of the short history.
	const rows = $derived(groupDownloads(events.filter((e) => !isRoutine(e.Kind))));
	const titles = $derived(new Map(show ? [[show.id, show]] : []));
	const subsOwn = $derived(subs?.wishes.find((w) => w.item_id === ep?.id));
	// No wish means the show default; a wish with no languages means off.
	const subsMode = $derived<'default' | 'off' | 'custom'>(!subsOwn ? 'default' : subsOwn.languages.length ? 'custom' : 'off');
	const subsFile = $derived(subs?.files.find((f) => f.item_id === ep?.id));
	const subsHint = $derived.by(() => {
		if (subsMode === 'off') return t('shows.sheet.noSubs');
		const wanted = subsFile?.wish?.languages ?? subsOwn?.languages ?? [];
		const have = new Set((subsFile?.subtitles ?? []).filter((r) => r.state === 'present').map((r) => r.language));
		const miss = wanted.filter((w) => !have.has(w.language));
		const head = subsMode === 'custom' ? codes(subsOwn?.languages ?? []) : t('shows.sheet.default');
		if (!wanted.length) return t('shows.sheet.noneSet', { head });
		return miss.length ? t('shows.sheet.missingLangs', { head, langs: codes(miss) }) : t('shows.sheet.allHere', { head });
	});
	const subsMenu = $derived<MenuItem[]>([
		{ label: t('shows.sheet.chooseLanguages'), icon: 'tools', onselect: () => (subsMore = true) },
		{ label: t('shows.sheet.lookSubs'), icon: 'search', onselect: () => void findSubs() },
		{ label: t('shows.sheet.chooseSub'), icon: 'copy', onselect: () => (subsMore = true) }
	]);
	const slot = $derived(ep?.slots[0]);
	const file = $derived(slot?.file ?? null);
	// Collapses the many slot states into the four the sheet styles.
	const tone = $derived.by(() => {
		const st = slot?.state ?? '';
		return ['have', 'imported', 'in_library', 'kept'].includes(st) ? 'have' : ['downloading', 'importing', 'searching'].includes(st) ? 'downloading' : st === 'failed' ? 'failed' : 'missing';
	});
	const fileFacts = $derived.by(() => {
		if (!file) return null;
		const q = (file.Quality ?? {}) as Record<string, any>;
		const m = (file.MediaInfo ?? {}) as Record<string, any>;
		const src: Record<string, string> = { web_dl: 'WEB', webrip: 'WEB', bluray: 'BluRay', remux: 'Remux', hdtv: 'TV', dvd: 'DVD' };
		return {
			badges: qualityBadges([file]),
			quality: [q.resolution ? `${q.resolution}p` : '', src[q.source] ?? '', m.video_codec ?? ''].filter(Boolean).join(' · '),
			size: file.Size ? bytes(file.Size) : '',
			name: String(file.RelativePath ?? '').split(/[\\/]/).pop() ?? '',
			added: file.AddedAt ? ago(file.AddedAt) : '',
			res: q.resolution ? `${q.resolution}p` : '',
			addedOn: ((iso?: string) => { if (!iso) return ''; const d = new Date(iso); return d.toLocaleDateString(appLocale(), { month: 'short', day: 'numeric', ...(d.getFullYear() !== new Date().getFullYear() ? { year: 'numeric' } : {}) }); })(file.AddedAt),
			audio: langs(m.audio_languages),
			subs: langs(m.subtitle_languages ?? m.subtitles)
		};
	});

	// Reloads history and the synopsis each time the sheet opens on an episode.
	$effect(() => {
		const e = ep;
		if (!open || !e) return;
		keep = e.keep_up_to_date !== false;
		story = '';
		events = [];
		void api.get<{ events?: typeof events }>(`/events?episode=${e.id}&limit=20`).then((r) => (events = r.events ?? []), () => {});
		if (tmdb && e.season != null)
			void api
				.get<{ episodes?: { number: number; overview?: string }[] }>(`/metadata/show/${encodeURIComponent(tmdb)}/seasons/${e.season}`)
				.then((r) => (story = r.episodes?.find((x) => x.number === e.episode)?.overview ?? ''), () => {});
	});
	$effect(() => { if (open && ep && manage) { subsMore = false; void loadSubs(); } });

	async function loadSubs() {
		if (!ep) return;
		try { subs = await api.get<TitleSubtitles>(`/titles/${titleId}/subtitles`); } catch { subs = null; }
	}
	async function setSubs(mode: 'default' | 'off') {
		if (!ep || mode === subsMode) return;
		busy = 'subs';
		try {
			await api.put(`/titles/${titleId}/subtitle-wish`, mode === 'default' ? { inherit: true, item_id: ep.id } : { languages: [], item_id: ep.id });
			await loadSubs();
		} catch (e) {
			toasts.push(t('shows.sheet.saveSubsFailed'), 'fail', { body: message(e) });
		} finally {
			busy = '';
		}
	}
	async function findSubs() {
		if (!ep) return;
		try {
			const r = await api.post<{ queued: number }>('/subtitles/search', { title_id: titleId, item_id: ep.id });
			toasts.push(r.queued ? t('shows.sheet.searchingSubs') : t('shows.sheet.noFileToSearch'), r.queued ? 'info' : 'ok');
		} catch (e) {
			toasts.push(t('shows.sheet.searchSubsFailed'), 'fail', { body: message(e) });
		}
	}
	async function look() {
		if (!slot) return;
		busy = 'look';
		try {
			await api.post(`/titles/${titleId}/search`, { slot_id: slot.id });
			toasts.push(t('shows.sheet.searchingEpisode', { n: ep?.episode ?? '' }), 'info');
		} catch (e) {
			toasts.push(t('shows.sheet.startSearchFailed'), 'fail', { body: message(e) });
		} finally {
			busy = '';
		}
	}
	async function flipKeep() {
		if (!ep) return;
		busy = 'keep';
		const next = !keep;
		try {
			await api.put(`/episodes/${ep.id}`, { keep_up_to_date: next });
			keep = next;
			onchange?.();
		} catch (e) {
			toasts.push(t('shows.sheet.updateFailed'), 'fail', { body: message(e) });
		} finally {
			busy = '';
		}
	}

	const codes = (ws: { language: string }[]) => [...new Set(ws.map((w) => w.language.toUpperCase()))].join(', ');
	// Eight tags is all the file card has room for.
	const langs = (v: unknown) => (Array.isArray(v) ? v : []).map((s) => String(s).toUpperCase()).filter(Boolean).slice(0, 8);
	// Air dates are plain days, so UTC keeps them from shifting a day.
	const date = (iso?: string) => (iso ? new Date(iso).toLocaleDateString(appLocale(), { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' }) : '');
</script>

<Sheet bind:open title={ep ? `${ep.season != null ? `${t('shows.season.named', { n: ep.season })} · ` : ''}${t('shows.list.episodeFallback', { n: ep.episode })}` : t('shows.sheet.episode')} size="md">
	{#if ep}
		<div class="es" data-testid="episode-sheet">
			<div class="es-still">
				{#if ep.still}<img src={ep.still} alt="" decoding="async" />{/if}
				<span class="es-shade" aria-hidden="true"></span>
				<div class="es-cap">
					<h3>{ep.name ?? t('shows.list.episodeFallback', { n: ep.episode })}</h3>
					<p>{[date(ep.air_date), ep.runtime_minutes ? t('shows.sheet.minutes', { n: ep.runtime_minutes }) : ''].filter(Boolean).join(' · ')}</p>
				</div>
				{#if tone === 'have'}<span class="es-chip"><span class="es-dot" aria-hidden="true"></span>{serverWords(slot?.state_label) ?? t('shows.sheet.inLibrary')}</span>{/if}
			</div>
			{#if story}<p class="es-story">{story}</p>{/if}

			{#if tone !== 'have'}
				<section class="es-status {tone}">
					<span class="es-st"><span class="es-dot" aria-hidden="true"></span>{serverWords(slot?.state_label) ?? t('shows.sheet.missing')}</span>
					{#if manage && slot && tone !== 'downloading'}<button type="button" class="es-btn primary" disabled={busy === 'look'} onclick={look}><Icon name="search" size={15} />{t('shows.sheet.lookForIt')}</button>{/if}
				</section>
			{/if}

			{#if fileFacts}
				<article class="es-file">
					<p class="es-badges"><b>{t('shows.sheet.file')}</b>{#each fileFacts.badges as b (b)}<span class="es-badge">{b}</span>{/each}</p>
					<div class="es-tiles"><div><span>{t('shows.sheet.quality')}</span><b>{fileFacts.res || '—'}</b></div><div><span>{t('shows.sheet.size')}</span><b>{fileFacts.size || '—'}</b></div><div><span>{t('shows.sheet.added')}</span><b>{fileFacts.addedOn || '—'}</b></div></div>
					{#if fileFacts.audio.length || fileFacts.subs.length}
						<p class="es-langs">{#if fileFacts.audio.length}<span class="muted">{t('shows.sheet.audio')}</span>{#each fileFacts.audio as l (l)}<span class="es-lang">{l}</span>{/each}{/if}{#if fileFacts.subs.length}<span class="muted">{t('shows.sheet.subtitles')}</span>{#each fileFacts.subs as l (l)}<span class="es-lang">{l}</span>{/each}{/if}</p>
					{/if}
					<p class="es-name" title={fileFacts.name}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 3h8l4 4v14H6z"/><path d="M14 3v4h4"/></svg><span>{fileFacts.name}</span></p>
				</article>
			{/if}

			{#if manage}
				<div class="es-rows">
					<button type="button" class="es-row" role="switch" aria-checked={keep} disabled={busy === 'keep'} onclick={flipKeep}>
						<span><b>{t('shows.sheet.keep')}</b><small>{t('shows.sheet.keepHint')}</small></span>
						<span class="es-switch" class:on={keep} aria-hidden="true"></span>
					</button>
					{#if ep.season != null}
						<div class="es-row static es-subq" data-testid="episode-subtitles-row">
							<span><b>{t('shows.sheet.subtitles')}</b><small>{subs ? subsHint : ' '}</small></span>
							<div class="q-seg" role="radiogroup" aria-label={t('shows.sheet.subtitles')}>
								<button type="button" role="radio" aria-checked={subsMode === 'default'} class:on={subsMode === 'default'} disabled={busy === 'subs'} onclick={() => setSubs('default')}>{t('shows.sheet.default')}</button>
								<button type="button" role="radio" aria-checked={subsMode === 'off'} class:on={subsMode === 'off'} disabled={busy === 'subs'} onclick={() => setSubs('off')}>{t('shows.sheet.off')}</button>
								<div class="q-more" class:on={subsMode === 'custom'}><Menu label={t('shows.sheet.subtitleOptions')} variant="button" width={220} items={subsMenu}>{#snippet trigger()}<i>{subsMode === 'custom' ? codes(subsOwn?.languages ?? []) : t('shows.sheet.custom')}</i><Icon name="chevronDown" size={13} />{/snippet}</Menu></div>
							</div>
						</div>
					{/if}
					{#if ondelete}<button type="button" class="es-row danger" onclick={ondelete}><span><b>{t('shows.sheet.delete')}</b></span><Icon name="trash" size={16} /></button>{/if}
				</div>
				{#if subsMore && ep.season != null}
					<div class="es-subs" data-testid="episode-subtitles">{#key ep.id}<SubtitlePanel {titleId} movie={false} itemId={ep.id} bare label={`S${String(ep.season).padStart(2, '0')}E${String(ep.episode).padStart(2, '0')}`} />{/key}</div>
				{/if}
			{/if}

			<h4 class="es-h">{t('shows.tools.history')}</h4>
			<ol class="es-tl">
				{#each rows as r (r.key)}<li><CompactRow row={r} ctx={{ titles }} titleById={titles} onclick={() => goto(to(`/downloads/history?title=${titleId}`))} /><span class="sr-only">{r.events.map((x) => x.Summary).join(' · ')}</span></li>
				{:else}<li class="muted">{t('shows.sheet.nothingYet')}</li>{/each}
			</ol>
		</div>
	{/if}
</Sheet>

<style>
	.es { display: grid; grid-template-columns: minmax(0, 1fr); gap: 14px; }
	.es-chip { position: absolute; right: 16px; bottom: 16px; display: inline-flex; align-items: center; gap: 7px; height: 28px; padding: 0 12px; border-radius: 999px; background: rgb(0 0 0 / 45%); color: #fff; font: 600 12.5px/1 var(--font-ui); backdrop-filter: blur(10px); -webkit-backdrop-filter: blur(10px); }
	.es-chip .es-dot { background: var(--ok); }
	.es-subs :global(.card) { padding: 0; background: none; box-shadow: none; border: 0; }
	.es-subs { position: relative; display: grid; gap: 16px; padding-top: 34px; }
	.es-row.static { cursor: default; }
	.es-row.static:hover { background: none; }
	.es-row.es-subq { display: grid; gap: 10px; justify-content: stretch; }
	.es-subq .q-seg { max-width: 420px; }
	.q-seg { display: flex; gap: 2px; padding: 3px; border-radius: 10px; background: color-mix(in srgb, var(--text) 7%, transparent); }
	.q-seg > button, .q-more :global(.trigger) { flex: 1 1 auto; min-width: 0; min-height: 0; display: inline-flex; align-items: center; justify-content: center; gap: 3px; height: 32px; padding: 0 8px; border: 0; border-radius: 8px; background: none; color: color-mix(in srgb, var(--text) 82%, transparent); font: 600 13px/1 var(--font-ui); white-space: nowrap; cursor: pointer; transition: background var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	.q-seg > button.on, .q-more.on :global(.trigger) { background: var(--surface-1, var(--bg)); color: var(--text); box-shadow: 0 0 0 1px color-mix(in srgb, var(--text) 10%, transparent), 0 1px 2px rgb(0 0 0 / .2); }
	.q-seg > button:focus-visible, .q-more :global(.trigger:focus-visible) { box-shadow: var(--focus-ring); }
	.q-more { flex: 0 0 auto; display: flex; }
	.q-more :global(.ui-pop) { display: flex; align-self: stretch; }
	.q-more i { font-style: normal; }
	@media (pointer: coarse) { .q-seg > button, .q-more :global(.trigger) { height: 40px; } }
	.es-subs :global(.card-head.bare) { position: absolute; top: 0; right: 0; margin: 0; }
	@media (max-width: 760px) { .es { grid-template-columns: minmax(0, 1fr); } }
	.es-still { position: relative; aspect-ratio: 21 / 9; border-radius: 14px; overflow: hidden; background: var(--surface-2); }
	@media (max-width: 760px) { .es-still { aspect-ratio: 16 / 9; } }

	.es-still img { width: 100%; height: 100%; object-fit: cover; display: block; }
	.es-shade { position: absolute; inset: 0; background: linear-gradient(to top, rgb(0 0 0 / 78%) 0%, rgb(0 0 0 / 20%) 45%, transparent 70%); }
	.es-cap { position: absolute; left: 22px; right: 22px; bottom: 16px; color: #fff; }
	.es-cap h3 { margin: 0; font: 700 22px/1.2 var(--font-ui); letter-spacing: -.01em; text-shadow: 0 1px 6px rgb(0 0 0 / 40%); }
	.es-cap p { margin: 4px 0 0; color: rgb(255 255 255 / 80%); font: 500 13px/1.3 var(--font-ui); }
	.es-story { margin: 0; color: var(--text-muted); font: 500 14px/1.5 var(--font-ui); display: -webkit-box; -webkit-line-clamp: 4; line-clamp: 4; -webkit-box-orient: vertical; overflow: hidden; }
	.es-status { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 14px; border-radius: 14px; background: var(--surface-2); }
	.es-st { display: inline-flex; align-items: center; gap: 8px; font: 600 15px/1 var(--font-ui); }
	.es-dot { width: 9px; height: 9px; border-radius: 99px; background: var(--text-muted); }
	.es-status.have .es-dot { background: var(--ok); }
	.es-status.downloading .es-dot { background: var(--info); }
	.es-status.failed .es-dot { background: var(--fail); }
	.es-status.failed { background: color-mix(in srgb, var(--fail) 10%, var(--surface-2)); }
	.es-btn { display: inline-flex; align-items: center; gap: 7px; height: 36px; padding: 0 16px; border: 0; border-radius: 999px; background: color-mix(in srgb, var(--text) 8%, transparent); color: var(--text); font: 600 13px/1 var(--font-ui); cursor: pointer; white-space: nowrap; }
	.es-btn.primary { background: var(--text); color: var(--bg); }
	.es-h { margin: 4px 0 -4px; font: 700 17px/1.2 var(--font-ui); }
	.es-file { display: grid; gap: 8px; padding: 14px 16px; border-radius: 14px; background: var(--surface-2); }
	.es-file p { margin: 0; }
	.es-badges { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
	.es-badges b { margin-right: 4px; font: 700 15px/1.2 var(--font-ui); }
	.es-badge, .es-lang { display: inline-flex; align-items: center; height: 20px; padding: 0 7px; border-radius: 5px; border: 1px solid color-mix(in srgb, var(--text) 30%, transparent); color: var(--text-muted); font: 700 11px/1 var(--font-ui); letter-spacing: .03em; }
	.es-lang { border-radius: 99px; }
	.es-q { font: 600 14px/1 var(--font-ui); }
	.es-meta { color: var(--text-muted); font: 500 13px/1.3 var(--font-ui); }
	.es-langs { display: flex; flex-wrap: wrap; align-items: center; gap: 5px; font: 500 12px/1 var(--font-ui); }
	.es-langs .muted:not(:first-child) { margin-left: 8px; }
	.es-tiles { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px; }
	.es-tiles > div { display: grid; gap: 2px; min-width: 0; padding: 9px 12px; border-radius: 10px; background: color-mix(in srgb, var(--text) 5%, transparent); }
	.es-tiles span { color: var(--text-muted); font: 500 11.5px/1.2 var(--font-ui); }
	.es-tiles b { font: 600 14px/1.3 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.es-name svg { flex: none; width: 15px; height: 15px; stroke: currentColor; fill: none; stroke-width: 1.8; }
	.es-name span { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
	.es-name { display: flex; align-items: center; gap: 8px; padding-top: 8px; border-top: 1px solid color-mix(in srgb, var(--text) 8%, transparent); color: var(--text-muted); font: 500 12px/1.4 var(--font-mono, ui-monospace, monospace); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; opacity: .8; }
	.es-rows { display: grid; border-radius: 14px; background: var(--surface-2); overflow: hidden; }
	.es-row { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 14px 16px; border: 0; background: none; color: var(--text); text-align: left; cursor: pointer; font: inherit; }
	.es-row span:first-child { display: grid; gap: 3px; }
	.es-row b { font: 600 15px/1.2 var(--font-ui); }
	.es-row.danger b, .es-row.danger :global(svg) { color: var(--fail-text); }
	.es-row:hover { background: color-mix(in srgb, var(--text) 4%, transparent); }
	.es-row:focus-visible { outline: none; box-shadow: inset 0 0 0 2px var(--accent); }
	.es-rows > .es-row + .es-row { border-top: 1px solid color-mix(in srgb, var(--text) 8%, transparent); }
	.es-row small { color: var(--text-muted); font: 500 13px/1.3 var(--font-ui); }
	.es-switch { position: relative; flex: none; width: 44px; height: 26px; border-radius: 99px; background: color-mix(in srgb, var(--text) 18%, transparent); transition: background var(--dur-fast) var(--ease-out); }
	.es-switch::after { content: ''; position: absolute; top: 3px; left: 3px; width: 20px; height: 20px; border-radius: 99px; background: #fff; box-shadow: 0 1px 3px rgb(0 0 0 / 25%); transition: transform var(--dur-fast) var(--ease-out); }
	.es-switch.on { background: var(--ok); }
	.es-switch.on::after { transform: translateX(18px); }
	.es-tl { display: grid; gap: 2px; margin: 0 -12px; padding: 0; list-style: none; font: 500 14px/1.35 var(--font-ui); }
	.es-tl li.muted { padding: 0 12px; }
</style>
