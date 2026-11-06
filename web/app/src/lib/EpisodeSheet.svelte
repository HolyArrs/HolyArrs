<script lang="ts">
	import Sheet from '#lib/ui/Sheet.svelte';
	import { goto } from '$app/navigation';
	import { to } from '#lib/paths.ts';
	import Icon from '#lib/Icon.svelte';
	import { api, message } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { ago, bytes } from '#lib/format.ts';
	import { qualityBadges } from '#lib/title-page.ts';
	import SubtitlePanel from '#lib/SubtitlePanel.svelte';
	import CompactRow from '#lib/history-looks/CompactRow.svelte';
	import { isRoutine } from '#lib/history-looks/dirs.ts';
	import { groupDownloads } from '#lib/history.ts';
	import type { EventRecord, TitleView } from '#lib/types.ts';

	// One episode, from the Library tab: its picture and story, where it stands,
	// its file (quality, size, languages), keep up to date, and what happened to it.
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
	// History in the compact rows of the title pages, routine work left out.
	const rows = $derived(groupDownloads(events.filter((e) => !isRoutine(e.Kind))));
	const titles = $derived(new Map(show ? [[show.id, show]] : []));
	let busy = $state('');
	let keep = $state(true);

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

	const slot = $derived(ep?.slots[0]);
	const file = $derived(slot?.file ?? null);
	const tone = $derived.by(() => {
		const st = slot?.state ?? '';
		return ['have', 'imported', 'in_library', 'kept'].includes(st) ? 'have' : ['downloading', 'importing', 'searching'].includes(st) ? 'downloading' : st === 'failed' ? 'failed' : 'missing';
	});
	const langs = (v: unknown) => (Array.isArray(v) ? v : []).map((s) => String(s).toUpperCase()).filter(Boolean).slice(0, 8);
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
			addedOn: ((iso?: string) => { if (!iso) return ''; const d = new Date(iso); return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', ...(d.getFullYear() !== new Date().getFullYear() ? { year: 'numeric' } : {}) }); })(file.AddedAt),
			audio: langs(m.audio_languages),
			subs: langs(m.subtitle_languages ?? m.subtitles)
		};
	});
	const date = (iso?: string) => (iso ? new Date(iso).toLocaleDateString(undefined, { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' }) : '');

	async function look() {
		if (!slot) return;
		busy = 'look';
		try {
			await api.post(`/titles/${titleId}/search`, { slot_id: slot.id });
			toasts.push(`Looking for episode ${ep?.episode}.`, 'ok');
		} catch (e) {
			toasts.push(message(e), 'fail');
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
			toasts.push(message(e), 'fail');
		} finally {
			busy = '';
		}
	}
</script>

<Sheet bind:open title={ep ? `${ep.season != null ? `Season ${ep.season} · ` : ''}Episode ${ep.episode}` : 'Episode'} placement="center" size="lg">
	{#if ep}
		<div class="es" data-testid="episode-sheet">
			<div class="es-main">
				<div class="es-still">
					{#if ep.still}<img src={ep.still} alt="" decoding="async" />{/if}
					<span class="es-shade" aria-hidden="true"></span>
					<div class="es-cap">
						<h3>{ep.name ?? `Episode ${ep.episode}`}</h3>
						<p>{[date(ep.air_date), ep.runtime_minutes ? `${ep.runtime_minutes} min` : ''].filter(Boolean).join(' · ')}</p>
					</div>
				</div>
				{#if story}<p class="es-story">{story}</p>{/if}

				<section class="es-status {tone}">
					<span class="es-st"><span class="es-dot" aria-hidden="true"></span>{slot?.state_label ?? 'Missing'}</span>
					{#if manage && slot && tone !== 'have' && tone !== 'downloading'}<button type="button" class="es-btn primary" disabled={busy === 'look'} onclick={look}><Icon name="search" size={15} />Look for it</button>{/if}
				</section>

				{#if fileFacts}
					<article class="es-file">
						<p class="es-badges"><b>File</b>{#each fileFacts.badges as b (b)}<span class="es-badge">{b}</span>{/each}</p>
						<div class="es-tiles"><div><span>Quality</span><b>{fileFacts.res || '—'}</b></div><div><span>Size</span><b>{fileFacts.size || '—'}</b></div><div><span>Added</span><b>{fileFacts.addedOn || '—'}</b></div></div>
						{#if fileFacts.audio.length || fileFacts.subs.length}
							<p class="es-langs">{#if fileFacts.audio.length}<span class="muted">Audio</span>{#each fileFacts.audio as l (l)}<span class="es-lang">{l}</span>{/each}{/if}{#if fileFacts.subs.length}<span class="muted">Subtitles</span>{#each fileFacts.subs as l (l)}<span class="es-lang">{l}</span>{/each}{/if}</p>
						{/if}
						<p class="es-name" title={fileFacts.name}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 3h8l4 4v14H6z"/><path d="M14 3v4h4"/></svg><span>{fileFacts.name}</span></p>
					</article>
				{/if}
			</div>

			<div class="es-side">
				<h4 class="es-h">History</h4>
				<ol class="es-tl">
					{#each rows as r (r.key)}<li><CompactRow row={r} ctx={{ titles }} titleById={titles} onclick={() => goto(to(`/downloads/history?title=${titleId}`))} /><span class="sr-only">{r.events.map((x) => x.Summary).join(' · ')}</span></li>
					{:else}<li class="muted">Nothing yet.</li>{/each}
				</ol>

				{#if manage}
					<h4 class="es-h">Settings</h4>
					<div class="es-rows">
						<button type="button" class="es-row" role="switch" aria-checked={keep} disabled={busy === 'keep'} onclick={flipKeep}>
							<span><b>Keep up to date</b><small>Better copies</small></span>
							<span class="es-switch" class:on={keep} aria-hidden="true"></span>
						</button>
						{#if ondelete}<button type="button" class="es-row danger" onclick={ondelete}><span><b>Delete episode</b></span><Icon name="trash" size={16} /></button>{/if}
					</div>
				{/if}

				{#if manage && ep.season != null}
					<!-- This episode's subtitles: its languages, what is missing, choose one by hand. -->
					<div class="es-subs" data-testid="episode-subtitles"><p class="es-h" aria-hidden="true">Subtitles</p>{#key ep.id}<SubtitlePanel {titleId} movie={false} itemId={ep.id} bare label={`S${String(ep.season).padStart(2, '0')}E${String(ep.episode).padStart(2, '0')}`} />{/key}</div>
				{/if}
			</div>
		</div>
	{/if}
</Sheet>

<style>
	.es { display: grid; grid-template-columns: minmax(0, 1.25fr) minmax(0, 1fr); gap: 24px; align-items: start; }
	.es-main, .es-side { display: grid; gap: 14px; min-width: 0; }
	.es-subs :global(.card) { padding: 0; background: none; box-shadow: none; border: 0; }
	.es-subs { position: relative; display: grid; gap: 16px; }
	.es-subs > .es-h { display: flex; align-items: center; min-height: 34px; margin: 0; }
	.es-subs :global(.card-head.bare) { position: absolute; top: 0; right: 0; margin: 0; }
	@media (max-width: 760px) { .es { grid-template-columns: minmax(0, 1fr); } }
	.es-still { position: relative; aspect-ratio: 16 / 9; border-radius: 14px; overflow: hidden; background: var(--surface-2); }
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
