<script lang="ts">
	import { onDestroy, onMount, type Snippet } from 'svelte';
	import { api } from '../api.ts';
	import { to } from '../paths.ts';
	import Icon from '../Icon.svelte';
	import IconButton from './IconButton.svelte';
	import PersonCircle from './PersonCircle.svelte';
	import TrailerPlayer from './TrailerPlayer.svelte';
	import { richMetadata, type RichMeta } from './TitleHero.svelte';
	import { autoplayHere, loadUISettings, trailerSources, type TrailerSource } from '../autoplay.svelte.ts';
	import { sized } from '../tmdb-image.ts';
	import { detailPath, remember } from '../title-details.ts';
	import { discoverCard, type ExploreCard } from '../explore.ts';
	import { holdScroll, openTheater } from '../theater.svelte.ts';
	import { reducedMotion } from '../motion.ts';
	import { facts, firstSeason, minutes, readEpisodes, readInfo, seasonLabel, type InfoDetails, type InfoEpisode, type InfoRef } from '../more-info.ts';

	// The More info panel, Netflix style: a large centred modal over Explore
	// (the page keeps its place behind it). A 16:9 trailer area with the
	// title logo and the one-tap Ask/Add, then the facts, overview, cast and
	// genres, where to stream, a show's seasons and episodes, More like this
	// and a movie's collection. It grows from the card it was opened from.
	let {
		ref,
		from = null,
		region = 'US',
		live,
		action,
		card,
		cardOpens,
		onCardOpens,
		onpick,
		onclose
	}: {
		ref: InfoRef;
		/** The card's rect it opened from (FLIP); null opens in place. */
		from?: DOMRect | null;
		region?: string;
		/** The page's view of a card (asked here, owned marks). */
		live: (c: ExploreCard) => ExploreCard;
		/** The one-tap Ask/Add for this title (null when it is in the Library or not allowed). */
		action: Snippet<[ExploreCard]>;
		/** A wide card for More like this and Collection, the same as the rows draw. */
		card: Snippet<[ExploreCard]>;
		cardOpens: 'page' | 'panel';
		onCardOpens: (v: 'page' | 'panel') => void;
		onpick: (r: InfoRef, from: DOMRect | null) => void;
		onclose: () => void;
	} = $props();

	let dialog = $state<HTMLDialogElement>();
	let panel = $state<HTMLElement>();
	let d = $state<InfoDetails | null>(null);
	let meta = $state<RichMeta | null>(null);
	let failed = $state(false);
	let sources = $state<TrailerSource[]>([]);
	let playing = $state(false);
	let season = $state<number | null>(null);
	let episodes = $state<InfoEpisode[] | null>(null);
	let episodesFailed = $state(false);
	let moreText = $state(false);
	let logoFailed = $state(false);
	let opener: HTMLElement | null = null;
	let leaving = false;
	const OPEN_MS = 300;

	const head = $derived(d ? live(discoverCard(d.head, 'info')) : null);
	const art = $derived(sized(meta?.backdrop ?? d?.backdrop ?? null, 'backdrop') || null);
	const logo = $derived(logoFailed ? null : sized(meta?.logo ?? d?.logo ?? null, 'poster') || null);
	const line = $derived(d ? facts(d) : []);
	const pageHref = $derived(d?.titleId ? to(`/library/${d.titleId}`) : to(detailPath(ref.kind, ref.tmdb)));
	const owned = $derived(!!head?.titleId);
	const directors = $derived(d?.crew.filter((p) => p.role !== 'actor').map((p) => p.name) ?? []);
	const cards = (list: InfoDetails['similar']) => list.map((s) => live(discoverCard(s, 'info-more')));

	// Load (again when another title is picked inside the panel).
	$effect(() => {
		const r = ref;
		const ctl = new AbortController();
		d = null;
		meta = null;
		failed = false;
		sources = [];
		playing = false;
		season = null;
		episodes = null;
		moreText = false;
		logoFailed = false;
		dialog?.scrollTo({ top: 0 });
		void (async () => {
			try {
				const raw = await api.get<Record<string, unknown>>(`/metadata/${r.kind}/${encodeURIComponent(r.tmdb)}?region=${encodeURIComponent(region)}`, ctl.signal);
				if (ctl.signal.aborted) return;
				d = readInfo(r.kind, r.tmdb, raw);
				season = firstSeason(d.seasons);
				remember(r.kind, r.tmdb, { name: d.name, year: d.year, poster: d.poster });
			} catch {
				if (!ctl.signal.aborted) failed = true;
				return;
			}
			meta = await richMetadata(api.get, r.kind, r.tmdb);
			// The trailer waits for the open to settle, then plays muted.
			await new Promise((ok) => setTimeout(ok, OPEN_MS + 200));
			await loadUISettings();
			if (ctl.signal.aborted || !autoplayHere()) return;
			const s = await trailerSources(r.kind, r.tmdb, meta?.videos, ctl.signal);
			if (!ctl.signal.aborted) sources = s;
		})();
		return () => ctl.abort();
	});

	// A show's season: its episodes.
	$effect(() => {
		const n = season;
		const tmdb = ref.tmdb;
		if (ref.kind !== 'show' || n === null) return;
		const ctl = new AbortController();
		episodes = null;
		episodesFailed = false;
		api.get<Record<string, unknown>>(`/metadata/show/${encodeURIComponent(tmdb)}/seasons/${n}`, ctl.signal)
			.then((raw) => !ctl.signal.aborted && (episodes = readEpisodes(raw)))
			.catch(() => !ctl.signal.aborted && (episodesFailed = true));
		return () => ctl.abort();
	});

	let release: (() => void) | null = null;

	onMount(() => {
		opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
		release = holdScroll();
		dialog?.showModal();
		if (!panel) return;
		if (reducedMotion()) {
			panel.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 160, easing: 'linear' });
			return;
		}
		const end = panel.getBoundingClientRect();
		if (from && from.width > 0 && end.width > 0) {
			// FLIP: start exactly on the card, grow into the panel.
			const sx = from.width / end.width;
			const dx = from.left - end.left;
			const dy = from.top - end.top;
			// Only the picture grows at first (the card is that picture); the rest opens under it as it lands.
			const media = panel.querySelector<HTMLElement>('.media')?.offsetHeight ?? end.width * 9 / 16;
			const below = Math.max(0, panel.offsetHeight - media);
			panel.animate(
				[
					{ transform: `translate(${dx}px, ${dy}px) scale(${sx})`, clipPath: `inset(0 0 ${below}px 0 round ${12 / sx}px)`, offset: 0 },
					{ transform: 'none', clipPath: `inset(0 0 ${below}px 0 round 20px)`, offset: 0.7 },
					{ transform: 'none', clipPath: 'inset(0 0 0 0 round 20px)', offset: 1 }
				],
				{ duration: OPEN_MS + 80, easing: 'cubic-bezier(.3, .1, .25, 1)' }
			);
		} else {
			panel.animate([{ transform: 'translateY(24px) scale(.96)', opacity: 0 }, { transform: 'none', opacity: 1 }], { duration: OPEN_MS, easing: 'cubic-bezier(.2, .8, .2, 1)' });
		}
		dialog?.animate([{ backgroundColor: 'rgb(0 0 0 / 0)' }, { backgroundColor: getComputedStyle(dialog).backgroundColor }], { duration: OPEN_MS });
	});

	onDestroy(() => {
		release?.();
		if (dialog?.open) dialog.close();
		if (opener?.isConnected) opener.focus({ preventScroll: true });
	});

	async function close() {
		if (leaving) return;
		leaving = true;
		if (panel && !reducedMotion()) {
			await panel.animate([{ transform: 'none', opacity: 1 }, { transform: 'translateY(16px) scale(.97)', opacity: 0 }], { duration: 180, easing: 'cubic-bezier(.4, 0, 1, 1)', fill: 'forwards' }).finished.catch(() => {});
		}
		onclose();
	}

	// A click on the dim area outside the panel closes it.
	function backdrop(e: MouseEvent) {
		if (e.target === dialog) void close();
	}

	function pick(e: MouseEvent) {
		const t = e.target as Element | null;
		const a = t?.closest?.('a') ? t.closest('[data-info]') : null;
		if (!a || e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
		const [kind, tmdb] = (a.getAttribute('data-info') ?? '').split(':');
		if ((kind !== 'movie' && kind !== 'show') || !tmdb) return;
		e.preventDefault();
		onpick({ kind, tmdb }, null);
	}

	function expand(o: { source: TrailerSource; time: number; rect: DOMRect }) {
		if (!d) return;
		openTheater({ kind: ref.kind, tmdb: ref.tmdb, title: d.name, source: o.source, time: o.time, rect: { left: o.rect.left, top: o.rect.top, width: o.rect.width, height: o.rect.height }, from: 'panel' });
	}

	const day = (s?: string) => {
		if (!s) return '';
		const t = new Date(`${s}T12:00:00`);
		return Number.isNaN(t.getTime()) ? '' : t.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
	};
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
<dialog
	bind:this={dialog}
	class="ui-info"
	aria-labelledby="info-title"
	data-testid="more-info"
	oncancel={(e) => {
		e.preventDefault();
		void close();
	}}
	onclick={backdrop}
>
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<article class="panel" bind:this={panel} onclick={pick} tabindex="-1" autofocus>
		<div class="media">
			{#if art}<img class="art" src={art} alt="" decoding="async" />{:else}<span class="ph" aria-hidden="true"></span>{/if}
			{#if sources.length}
				<TrailerPlayer {sources} title={d?.name ?? ''} repeat onplaying={() => (playing = true)} onfail={() => (playing = false)} onended={() => (playing = false)} onexpand={expand} />
			{/if}
			<span class="fade" aria-hidden="true"></span>
			<div class="close"><IconButton icon="x" label="Close" variant="glass" size="lg" tipSide="below" onclick={() => void close()} /></div>
			<div class="over" class:playing>
				<h2 id="info-title" class="logo-wrap">
					{#if logo}<img class="logo" src={logo} alt={d?.name ?? ''} onerror={() => (logoFailed = true)} />{:else}<span class="word">{d?.name ?? ''}</span>{/if}
				</h2>
				<div class="buttons">
					{#if head}{@render action(head)}{/if}
					<a class="page" href={pageHref} aria-label="Open the {owned ? 'Library' : 'title'} page for {d?.name ?? 'this title'}">
						<Icon name={owned ? 'library' : 'info'} size={18} /><span>{owned ? 'Open in Library' : 'Title page'}</span>
					</a>
				</div>
			</div>
		</div>

		{#if failed}
			<div class="body"><p class="muted">Could not load this title. <a href={pageHref}>Open its page</a>.</p></div>
		{:else if !d}
			<div class="body loading" aria-busy="true" aria-label="Loading">
				<span class="sk w40"></span><span class="sk w90"></span><span class="sk w80"></span><span class="sk w60"></span>
			</div>
		{:else}
			<div class="body">
				<div class="cols">
					<div class="main">
						<p class="facts">
							{#if owned}<span class="have"><Icon name="owned" size={14} stroke={2.4} />In your Library</span>{:else if head?.status && head.status !== 'in_library'}<span class="have ask">{head.ask?.state_label ?? 'Asked'}</span>{/if}
							{#each line as f, i (f)}{#if i}<i aria-hidden="true">·</i>{/if}<span class:rating={f === d.rating}>{f}</span>{/each}
						</p>
						{#if d.overview}
							<p class="overview" class:open={moreText}>{d.overview}</p>
							{#if d.overview.length > 320}<button type="button" class="more-text" aria-expanded={moreText} onclick={() => (moreText = !moreText)}>{moreText ? 'Less' : 'More'}</button>{/if}
						{/if}
					</div>
					<aside class="side">
						{#if d.cast.length}<p><span class="k">Cast:</span> {d.cast.slice(0, 4).map((p) => p.name).join(', ')}</p>{/if}
						{#if d.genres.length}<p><span class="k">Genres:</span> {d.genres.join(', ')}</p>{/if}
						{#if directors.length}<p><span class="k">{ref.kind === 'show' ? 'Created by:' : 'Director:'}</span> {directors.slice(0, 2).join(', ')}</p>{/if}
					</aside>
				</div>

				{#if d.streaming.length}
					<section class="sec" aria-labelledby="info-stream">
						<h3 id="info-stream">Where to stream</h3>
						<ul class="stream">
							{#each d.streaming as sv (sv.id)}
								<li title={sv.name}>{#if sv.logo}<img src={sv.logo} alt="" width="36" height="36" loading="lazy" />{/if}<span>{sv.name}</span></li>
							{/each}
						</ul>
					</section>
				{/if}

				{#if d.cast.length}
					<section class="sec" aria-labelledby="info-cast">
						<h3 id="info-cast">Cast</h3>
						<div class="cast">
							{#each d.cast as p (p.name + (p.character ?? ''))}
								<PersonCircle name={p.name} role={p.character} image={p.profile ? sized(p.profile, 'poster') : null} size={72} href={to(`/search?q=${encodeURIComponent(p.name)}`)} />
							{/each}
						</div>
					</section>
				{/if}

				{#if ref.kind === 'show' && d.seasons.length}
					<section class="sec" aria-labelledby="info-eps">
						<div class="sec-head">
							<h3 id="info-eps">Episodes</h3>
							{#if d.seasons.length > 1}
								<label class="season">
									<span class="sr">Season</span>
									<select bind:value={season} data-testid="season-picker">
										{#each d.seasons as s (s.number)}<option value={s.number}>{seasonLabel(s.number)} ({s.episodes})</option>{/each}
									</select>
									<Icon name="chevronDown" size={16} />
								</label>
							{:else if season !== null}<span class="one">{seasonLabel(season)}</span>{/if}
						</div>
						{#if episodesFailed}
							<p class="muted">Could not load this season.</p>
						{:else if !episodes}
							<div class="loading" aria-busy="true" aria-label="Loading episodes"><span class="sk ep"></span><span class="sk ep"></span><span class="sk ep"></span></div>
						{:else}
							<ol class="eps">
								{#each episodes as ep (ep.number)}
									<li class="ep">
										<span class="num">{ep.number}</span>
										<span class="still">{#if ep.still}<img src={sized(ep.still, 'backdrop')} alt="" loading="lazy" decoding="async" />{/if}</span>
										<span class="ep-t">{ep.name ?? `Episode ${ep.number}`}</span>
										<span class="ep-m">{[minutes(ep.runtime), day(ep.airDay)].filter(Boolean).join(' · ')}</span>
										{#if ep.overview}<span class="ep-o">{ep.overview}</span>{/if}
									</li>
								{/each}
							</ol>
						{/if}
					</section>
				{/if}

				{#if d.similar.length}
					<section class="sec" aria-labelledby="info-more">
						<h3 id="info-more">More like this</h3>
						<div class="grid">{#each cards(d.similar) as c (c.key)}<div data-info="{c.kind}:{c.ref?.slice(5)}">{@render card(c)}</div>{/each}</div>
					</section>
				{/if}

				{#if d.collection}
					<section class="sec" aria-labelledby="info-coll">
						<h3 id="info-coll">{d.collection.name}</h3>
						<div class="grid">{#each cards(d.collection.parts) as c (c.key)}<div data-info="{c.kind}:{c.ref?.slice(5)}" class:here={c.ref === `tmdb:${ref.tmdb}`}>{@render card(c)}</div>{/each}</div>
					</section>
				{/if}

				<footer class="foot">
					<label class="pref">
						<input type="checkbox" checked={cardOpens === 'panel'} onchange={(e) => onCardOpens((e.currentTarget as HTMLInputElement).checked ? 'panel' : 'page')} />
						<span>Open cards on Explore here instead of the title page</span>
					</label>
					<a class="all" href={pageHref}>{owned ? 'Open in Library' : 'Full title page'} <Icon name="chevronRight" size={16} /></a>
				</footer>
			</div>
		{/if}
	</article>
</dialog>

<style>
	.ui-info {
		--info-dim: rgb(0 0 0 / 0.62);
		position: fixed; inset: 0; width: 100%; height: 100dvh; max-width: none; max-height: none; margin: 0; padding: 32px 16px 48px;
		border: 0; background: var(--info-dim); color: var(--text); overflow-y: auto; overscroll-behavior: contain;
	}
	.ui-info::backdrop { background: transparent; }
	.panel:focus { outline: none; }
	.panel { position: relative; width: min(940px, 100%); margin: 0 auto; border-radius: var(--r-modal); overflow: hidden; background: var(--surface-1); box-shadow: var(--shadow-modal), 0 0 0 1px var(--line); transform-origin: 0 0; }
	.media { position: relative; aspect-ratio: 16 / 9; background: var(--surface-2); overflow: hidden; color: var(--on-art); }
	.art, .ph { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
	.ph { background: radial-gradient(120% 90% at 0% 0%, var(--accent-soft), transparent 60%), linear-gradient(160deg, var(--surface-2), var(--surface-3)); }
	.fade { position: absolute; inset: 0; pointer-events: none; z-index: 1; background: linear-gradient(to top, var(--surface-1) 0%, color-mix(in srgb, var(--surface-1) 0%, transparent) 12%), linear-gradient(to top, var(--on-art-scrim-mid) 0%, var(--on-art-scrim-none) 55%), linear-gradient(to right, var(--on-art-scrim-mid) 0%, var(--on-art-scrim-none) 55%); }
	.media :global(.ctl) { z-index: 3; right: 20px; bottom: 24px; }
	.media :global(.expand) { z-index: 1; }
	.close { position: absolute; top: 16px; right: 16px; z-index: 4; }
	.over { position: absolute; left: clamp(20px, 5%, 48px); right: 30%; bottom: clamp(32px, 10%, 64px); z-index: 2; display: flex; flex-direction: column; gap: 18px; pointer-events: none; }
	.over > * { pointer-events: auto; }
	h2 { margin: 0; }
	.logo-wrap { transform-origin: left bottom; transition: transform 800ms var(--ease-out); }
	.over.playing .logo-wrap { transform: scale(.8); }
	.logo { display: block; max-width: min(380px, 100%); max-height: 130px; width: auto; height: auto; object-fit: contain; object-position: left bottom; filter: drop-shadow(0 4px 20px var(--on-art-scrim-mid)); }
	.word { display: block; font: var(--t-large-title); letter-spacing: var(--t-large-title-track); color: var(--on-art); text-shadow: 0 2px 20px var(--on-art-scrim-mid); text-wrap: balance; }
	.buttons { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; }
	.buttons :global(.info-act) { display: inline-flex; align-items: center; gap: 8px; min-height: 44px; padding: 0 22px 0 16px; border: 0; border-radius: var(--r-pill); background: var(--on-art); color: var(--on-art-ink); font: var(--t-headline); cursor: pointer; transition: background-color var(--dur-fast) ease, transform var(--dur-instant) var(--ease-out); }
	.buttons :global(.info-act:hover) { background: var(--on-art-fill-strong); }
	.buttons :global(.info-act:active) { transform: scale(var(--press)); }
	.buttons :global(.info-act:focus-visible), .page:focus-visible { outline: none; box-shadow: 0 0 0 3px var(--accent); }
	.page { display: inline-flex; align-items: center; gap: 8px; min-height: 44px; padding: 0 18px 0 14px; border-radius: var(--r-pill); background: var(--on-art-fill); color: var(--on-art); font: var(--t-headline); box-shadow: inset 0 0 0 1px var(--on-art-ring); -webkit-backdrop-filter: blur(16px) saturate(160%); backdrop-filter: blur(16px) saturate(160%); transition: background-color var(--dur-fast) ease; }
	.page:hover { background: var(--on-art-fill-hover); }

	.body { display: flex; flex-direction: column; gap: 32px; padding: 8px clamp(20px, 5%, 48px) 36px; }
	.cols { display: grid; grid-template-columns: minmax(0, 2fr) minmax(0, 1fr); gap: 32px; }
	.main { display: flex; flex-direction: column; gap: 14px; min-width: 0; }
	.facts { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 10px; margin: 0; font: var(--t-callout); font-weight: 500; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.facts i { font-style: normal; color: var(--text-faint, var(--text-muted)); }
	.rating { padding: 0 6px; border: 1px solid color-mix(in srgb, var(--text) 40%, transparent); border-radius: 4px; color: var(--text); font: var(--t-footnote); line-height: 18px; }
	.have { display: inline-flex; align-items: center; gap: 5px; padding: 2px 10px 2px 8px; border-radius: var(--r-pill); background: color-mix(in srgb, var(--mark-have) 16%, transparent); color: var(--text); font: var(--t-footnote); font-weight: 600; }
	.have :global(svg) { color: var(--mark-have); }
	.have.ask { background: color-mix(in srgb, var(--mark-ask) 18%, transparent); }
	.overview { margin: 0; font: var(--t-body); font-size: 16px; color: var(--text); max-width: 62ch; display: -webkit-box; -webkit-line-clamp: 5; line-clamp: 5; -webkit-box-orient: vertical; overflow: hidden; }
	.overview.open { display: block; }
	.more-text { align-self: flex-start; min-height: 32px; padding: 0; color: var(--text-muted); font: var(--t-subhead); font-weight: 600; }
	.more-text:hover { color: var(--text); text-decoration: underline; text-underline-offset: 3px; }
	.side { display: flex; flex-direction: column; gap: 10px; font: var(--t-callout); color: var(--text); min-width: 0; }
	.side p { margin: 0; }
	.k { color: var(--text-muted); }

	.sec { display: flex; flex-direction: column; gap: 14px; min-width: 0; }
	h3 { margin: 0; font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	.sec-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
	.stream { display: flex; flex-wrap: wrap; gap: 10px; margin: 0; padding: 0; list-style: none; }
	.stream li { display: inline-flex; align-items: center; gap: 10px; padding: 6px 14px 6px 6px; border-radius: var(--r-pill); background: var(--surface-2); font: var(--t-callout); font-weight: 500; box-shadow: inset 0 0 0 1px var(--line); }
	.stream img { width: 36px; height: 36px; border-radius: 9px; object-fit: cover; }
	.cast { display: flex; gap: 8px; overflow-x: auto; scrollbar-width: none; margin-inline: -8px; padding: 4px 0 4px; }
	.cast::-webkit-scrollbar { display: none; }
	.season { position: relative; display: inline-flex; align-items: center; }
	.season select { appearance: none; -webkit-appearance: none; min-height: 44px; padding: 0 40px 0 16px; border-radius: var(--r-btn); border: 0; background: var(--surface-2); color: var(--text); font: var(--t-headline); box-shadow: inset 0 0 0 1px var(--line); cursor: pointer; }
	.season select:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.season :global(svg) { position: absolute; right: 14px; pointer-events: none; color: var(--text-muted); }
	.one { font: var(--t-headline); color: var(--text-muted); }
	.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
	.eps { display: flex; flex-direction: column; margin: 0; padding: 0; list-style: none; }
	.ep { display: grid; grid-template-columns: 36px 168px minmax(0, 1fr) auto; grid-template-areas: 'num still t m' 'num still o o'; column-gap: 18px; row-gap: 6px; align-items: start; padding: 18px 12px; border-top: 1px solid var(--line); border-radius: 2px; }
	.ep:first-child { border-top: 0; }
	.num { grid-area: num; align-self: center; font: var(--t-title2); color: var(--text-muted); text-align: center; font-variant-numeric: tabular-nums; }
	.still { grid-area: still; align-self: center; position: relative; aspect-ratio: 16 / 9; border-radius: var(--r-sm); overflow: hidden; background: linear-gradient(160deg, var(--surface-2), var(--surface-3)); }
	.still img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
	.ep-t { grid-area: t; font: var(--t-headline); min-width: 0; }
	.ep-m { grid-area: m; font: var(--t-footnote); color: var(--text-muted); white-space: nowrap; font-variant-numeric: tabular-nums; padding-top: 2px; }
	.ep-o { grid-area: o; font: var(--t-callout); color: var(--text-muted); display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
	.grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px; }
	.grid > div { min-width: 0; }
	.grid .here { opacity: .55; pointer-events: none; }
	.foot { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 12px; padding-top: 20px; border-top: 1px solid var(--line); }
	.pref { display: inline-flex; align-items: center; gap: 10px; min-height: 44px; font: var(--t-callout); color: var(--text-muted); cursor: pointer; }
	.pref input { width: 18px; height: 18px; accent-color: var(--accent); }
	.all { display: inline-flex; align-items: center; gap: 4px; min-height: 44px; font: var(--t-headline); color: var(--accent); }
	.all:hover { text-decoration: underline; text-underline-offset: 3px; }
	.muted { color: var(--text-muted); font: var(--t-callout); margin: 0; }
	.muted a { color: var(--accent); }
	.loading { display: flex; flex-direction: column; gap: 12px; }
	.sk { display: block; height: 16px; border-radius: 6px; background: var(--surface-2); animation: sk 1.2s ease-in-out infinite alternate; }
	.sk.w40 { width: 40%; } .sk.w90 { width: 90%; } .sk.w80 { width: 80%; } .sk.w60 { width: 60%; }
	.sk.ep { height: 96px; }
	@keyframes sk { to { opacity: .5; } }

	@media (max-width: 767px) {
		.ui-info { padding: 0; }
		.panel { width: 100%; min-height: 100dvh; border-radius: 0; box-shadow: none; }
		.over { right: 16px; left: 16px; bottom: 14px; gap: 12px; }
		.logo { max-width: 62%; max-height: 72px; }
		.word { font: var(--t-title1); letter-spacing: var(--t-title1-track); }
		.media { aspect-ratio: 4 / 3; }
		.body { padding: 4px 16px 32px; gap: 26px; }
		.cols { grid-template-columns: 1fr; gap: 14px; }
		.grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
		.ep { grid-template-columns: 128px minmax(0, 1fr); grid-template-areas: 'still t' 'still m' 'o o'; column-gap: 12px; padding: 14px 0; }
		.num { display: none; }
		.ep-m { padding-top: 0; }
		.media :global(.ctl) { right: 12px; bottom: auto; top: 16px; right: 72px; }
	}
	@media (prefers-reduced-motion: reduce) {
		.logo-wrap { transition: none; }
		.over.playing .logo-wrap { transform: none; }
		.sk { animation: none; }
	}
</style>
