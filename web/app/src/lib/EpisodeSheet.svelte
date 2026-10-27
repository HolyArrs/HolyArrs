<script lang="ts">
	import Sheet from '#lib/ui/Sheet.svelte';
	import Icon from '#lib/Icon.svelte';
	import { api, message } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { ago, bytes } from '#lib/format.ts';
	import { qualityBadges } from '#lib/title-page.ts';

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
		onchange
	}: { open?: boolean; ep: SheetEpisode | null; titleId: number; tmdb?: string; manage?: boolean; onchange?: () => void } = $props();

	let story = $state('');
	let events = $state<{ ID: number; At?: string; Kind?: string; Summary?: string }[]>([]);
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

<Sheet bind:open title={ep ? `${ep.season != null ? `Season ${ep.season} · ` : ''}Episode ${ep.episode}` : 'Episode'} placement="right" size="md">
	{#if ep}
		<div class="es" data-testid="episode-sheet">
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
				<p class="es-h">File</p>
				<article class="es-file">
					<p class="es-badges">{#each fileFacts.badges as b (b)}<span class="es-badge">{b}</span>{/each}{#if fileFacts.quality}<span class="es-q">{fileFacts.quality}</span>{/if}</p>
					<p class="es-meta">{[fileFacts.size, fileFacts.added && `added ${fileFacts.added}`].filter(Boolean).join(' · ')}</p>
					{#if fileFacts.audio.length || fileFacts.subs.length}
						<p class="es-langs">{#if fileFacts.audio.length}<span class="muted">Audio</span>{#each fileFacts.audio as l (l)}<span class="es-lang">{l}</span>{/each}{/if}{#if fileFacts.subs.length}<span class="muted">Subtitles</span>{#each fileFacts.subs as l (l)}<span class="es-lang">{l}</span>{/each}{/if}</p>
					{/if}
					<p class="es-name" title={fileFacts.name}>{fileFacts.name}</p>
				</article>
			{/if}

			{#if manage}
				<div class="es-rows">
					<button type="button" class="es-row" role="switch" aria-checked={keep} disabled={busy === 'keep'} onclick={flipKeep}>
						<span><b>Keep up to date</b><small>Grab it when it shows up and upgrade it later</small></span>
						<span class="es-switch" class:on={keep} aria-hidden="true"></span>
					</button>
				</div>
			{/if}

			<p class="es-h">History</p>
			<ol class="es-tl">
				{#each events as e (e.ID)}<li><span>{e.Summary ?? e.Kind}</span><span class="muted">{e.At ? ago(e.At) : ''}</span></li>
				{:else}<li class="muted">Nothing has happened to this episode yet.</li>{/each}
			</ol>
		</div>
	{/if}
</Sheet>

<style>
	.es { display: grid; gap: 16px; }
	.es-still { position: relative; aspect-ratio: 16 / 9; margin: 0 -22px; overflow: hidden; background: var(--surface-2); }
	.es-still img { width: 100%; height: 100%; object-fit: cover; display: block; }
	.es-shade { position: absolute; inset: 0; background: linear-gradient(to top, rgb(0 0 0 / 78%) 0%, rgb(0 0 0 / 20%) 45%, transparent 70%); }
	.es-cap { position: absolute; left: 22px; right: 22px; bottom: 16px; color: #fff; }
	.es-cap h3 { margin: 0; font: 700 22px/1.2 var(--font-ui); letter-spacing: -.01em; text-shadow: 0 1px 6px rgb(0 0 0 / 40%); }
	.es-cap p { margin: 4px 0 0; color: rgb(255 255 255 / 80%); font: 500 13px/1.3 var(--font-ui); }
	.es-story { margin: 0; color: var(--text-muted); font: 500 14px/1.5 var(--font-ui); }
	.es-status { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 14px; border-radius: 14px; background: var(--surface-2); }
	.es-st { display: inline-flex; align-items: center; gap: 8px; font: 600 15px/1 var(--font-ui); }
	.es-dot { width: 9px; height: 9px; border-radius: 99px; background: var(--text-muted); }
	.es-status.have .es-dot { background: #30a46c; }
	.es-status.downloading .es-dot { background: #3e8ef7; }
	.es-status.failed .es-dot { background: var(--danger, #e5484d); }
	.es-status.failed { background: color-mix(in srgb, var(--danger, #e5484d) 10%, var(--surface-2)); }
	.es-btn { display: inline-flex; align-items: center; gap: 7px; height: 36px; padding: 0 16px; border: 0; border-radius: 999px; background: color-mix(in srgb, var(--text) 8%, transparent); color: var(--text); font: 600 13px/1 var(--font-ui); cursor: pointer; white-space: nowrap; }
	.es-btn.primary { background: var(--text); color: var(--bg); }
	.es-h { margin: 6px 0 -6px; color: var(--text-muted); font: 700 12px/1 var(--font-ui); letter-spacing: .06em; text-transform: uppercase; }
	.es-file { display: grid; gap: 8px; padding: 14px 16px; border-radius: 14px; background: var(--surface-2); }
	.es-file p { margin: 0; }
	.es-badges { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
	.es-badge, .es-lang { display: inline-flex; align-items: center; height: 20px; padding: 0 7px; border-radius: 5px; border: 1px solid color-mix(in srgb, var(--text) 30%, transparent); color: var(--text-muted); font: 700 11px/1 var(--font-ui); letter-spacing: .03em; }
	.es-lang { border-radius: 99px; }
	.es-q { font: 600 14px/1 var(--font-ui); }
	.es-meta { color: var(--text-muted); font: 500 13px/1.3 var(--font-ui); }
	.es-langs { display: flex; flex-wrap: wrap; align-items: center; gap: 5px; font: 500 12px/1 var(--font-ui); }
	.es-langs .muted:not(:first-child) { margin-left: 8px; }
	.es-name { color: var(--text-muted); font: 500 12px/1.4 var(--font-mono, ui-monospace, monospace); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; opacity: .8; }
	.es-rows { display: grid; border-radius: 14px; background: var(--surface-2); overflow: hidden; }
	.es-row { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 14px 16px; border: 0; background: none; color: var(--text); text-align: left; cursor: pointer; font: inherit; }
	.es-row span:first-child { display: grid; gap: 3px; }
	.es-row b { font: 600 15px/1.2 var(--font-ui); }
	.es-row small { color: var(--text-muted); font: 500 13px/1.3 var(--font-ui); }
	.es-switch { position: relative; flex: none; width: 44px; height: 26px; border-radius: 99px; background: color-mix(in srgb, var(--text) 18%, transparent); transition: background .2s ease; }
	.es-switch::after { content: ''; position: absolute; top: 3px; left: 3px; width: 20px; height: 20px; border-radius: 99px; background: #fff; box-shadow: 0 1px 3px rgb(0 0 0 / 25%); transition: transform .2s ease; }
	.es-switch.on { background: #30a46c; }
	.es-switch.on::after { transform: translateX(18px); }
	.es-tl { display: grid; gap: 10px; margin: 0; padding: 0; list-style: none; font: 500 14px/1.35 var(--font-ui); }
	.es-tl li { display: flex; justify-content: space-between; gap: 12px; }
</style>
