<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Billboard from '#lib/ui/Billboard.svelte';
	import TrailerPlayer from '#lib/ui/TrailerPlayer.svelte';
	import Poster from '#lib/ui/Poster.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Avatar from '#lib/ui/Avatar.svelte';
	import KidMark from './KidMark.svelte';
	import AskParts from './AskParts.svelte';
	import type { BoardCtx } from './board.ts';
	import { artistIdOf, artistPhoto, askArtist } from './details.svelte.ts';
	import { lifecycle, upcoming, type Life, type LifeFacts } from './lifecycle.ts';
	import { partialFacts, realAlbum } from './live.ts';
	import { nextWhen, shortDate, whenDate } from './inbox.ts';
	import { askTab } from './ask-words.ts';
	import { askHref } from './board.ts';
	import { askedFor } from './asked-for.ts';
	import type { PartsPick } from './ask-parts.ts';
	import { watchlistLine } from '../accounts/index.ts';
	import { tick, untrack } from 'svelte';
	import { fade, fly } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { proxied } from '../core/tmdb-image.ts';
	import { artistVideo, autoplayHere, loadUISettings } from '../core/trailer/autoplay.svelte.ts';
	import { heroVideoStart, pickedSources, type HeroVideo } from '../core/trailer/hero-video.ts';
	import { agoLong, dateTime } from '../core/format.ts';
	import { to } from '../core/paths.ts';
	import { session } from '../core/session.svelte.ts';
	import { api } from '../core/api.ts';
	import type { AlbumSummary, TitleView } from '../core/types.ts';
	import type { Ask } from '../core/types.ts';

	// One ask in the cards' window (RequestsBoard): the trailer as a big banner (Explore's Billboard; for music, the artist's video over the photo), type · year · what was asked, who asked and their note, then Decline and Approve.

	let { ctx, single, choose: startChoosing = false }: { ctx: BoardCtx; /** The ask this window shows. */ single: Ask; /** Open straight into What to get. */ choose?: boolean } = $props();

	// Reduced motion: Svelte transitions run in no time.
	const reduced = () => typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
	const ms = (d: number) => (reduced() ? 0 : d);
	const waiting = (a: Ask) => ctx.approver && a.state === 'asked' && !ctx.isPending(a.id);
	const picked = $derived(ctx.viewed.find((a) => a.id === single.id) ?? single);
	$effect(() => {
		ctx.want(picked);
	});

	const d = $derived(ctx.details(picked));
	const music = $derived(picked.kind === 'artist');
	const artistBg = $derived(music ? (askArtist(picked)?.backgrounds[0] ?? artistPhoto(picked) ?? '') : '');
	const pictures = $derived(music ? [] : d?.backdrop ? [to(d.backdrop)] : picked.poster ? [to(picked.poster)] : []);
	// Music: the artist's music video plays muted over the photo, as on the artist page (same Autoplay setting).
	let mvideo = $state<HeroVideo | null>(null);
	// Keyed on the artist only, so a list refresh does not restart the playing video.
	const mvArtist = $derived(picked.kind === 'artist' ? artistIdOf(picked) : null);
	$effect(() => {
		const id = mvArtist;
		mvideo = null;
		if (!id) return;
		const ctl = new AbortController();
		void loadUISettings().then(() => {
			if (ctl.signal.aborted || !autoplayHere()) return;
			void artistVideo(id, ctl.signal).then((v) => {
				if (!ctl.signal.aborted && v?.proxy_url) mvideo = v;
			});
		});
		return () => ctl.abort();
	});
	// Not pending (Processing or Done): a big status under the banner instead of the answer buttons.
	const status = $derived(ctx.approver && picked.state !== 'asked');
	// How much of what was asked is in the Library (this window only: one title read).
	let partial = $state<{ id: number; facts: LifeFacts } | null>(null);
	// Read again only when the ask, its title or its state changes, not on every list refresh.
	const partKey = $derived(`${picked.id}|${picked.title_id ?? 0}|${picked.state}`);
	$effect(() => {
		void partKey;
		const a = untrack(() => picked);
		if (!a.title_id || (a.state !== 'approved' && a.state !== 'available') || a.kind === 'movie') return;
		const id = a.title_id;
		const ctl = new AbortController();
		const load =
			a.kind === 'show'
				? api.get<TitleView>(`/titles/${id}`, ctl.signal).then((tv) =>
						partialFacts(
							'seasons',
							(tv.show?.seasons ?? []).filter((s) => s.number > 0 && !s.dropped).map((s) => ({ key: s.number, complete: s.aired > 0 && s.in_library >= s.aired, real: s.aired > 0, size: s.aired, have: s.in_library })),
							a.seasons ?? [],
							(a.episodes ?? []).filter((e) => e.episodes?.length).map((e) => ({ key: e.season, count: new Set(e.episodes).size }))
						)
					)
				: api.get<{ albums: AlbumSummary[] }>(`/titles/${id}/albums`, ctl.signal).then((r) =>
						partialFacts(
							'albums',
							r.albums.filter((x) => !x.dropped).map((x) => ({ key: x.mbid, complete: x.tracks > 0 && x.in_library >= x.tracks, real: realAlbum(x), size: x.tracks, have: x.in_library })),
							(a.albums ?? []).filter((x) => !x.songs?.length).map((x) => x.mbid),
							(a.albums ?? []).filter((x) => x.songs?.length).map((x) => ({ key: x.mbid, count: new Set(x.songs).size }))
						)
					);
		load.then((facts) => !ctl.signal.aborted && (partial = { id, facts })).catch(() => {});
		return () => ctl.abort();
	});
	const life = $derived.by(() => {
		const a = picked;
		if (a.state === 'available') return lifecycle(a, partial && partial.id === a.title_id ? partial.facts : {});
		if (a.state !== 'approved' || !a.title_id) return lifecycle(a);
		const live = ctx.live(a);
		const part: LifeFacts = partial && partial.id === a.title_id ? partial.facts : {};
		if (live.phase) return lifecycle(a, { ...part, ...live });
		const out = a.kind === 'artist' ? '' : upcoming(a, ctx.info(a), (season) => ctx.episodes(a.tmdb_id, season)?.map((e) => e.air_day ?? '').filter(Boolean).sort()[0] ?? null);
		return lifecycle(a, out ? { ...part, upcoming: shortDate(out) } : live.upcoming !== undefined ? { ...part, upcoming: live.upcoming } : part);
	});
	const failed = $derived(status && askTab(picked) === 'failed');
	// An approved ask with a download in play links to its row on Downloads.
	const dlId = $derived(status && picked.state === 'approved' && picked.title_id && session.can('manage_library') ? ctx.live(picked).downloadId : undefined);
	// A declined ask had nothing approved, so its list is what was asked for.
	const listName = $derived(picked.state === 'turned_down' ? t('requests.window.listAsked') : t('requests.window.listApproved'));
	const sentence = $derived.by(() => {
		const who = ctx.who(picked).name;
		if (life.stage === 'ready') {
			const at = picked.available_at ?? picked.decided_at;
			const line = at ? t('requests.window.inLibrarySince', { date: whenDate(at) }) : t('requests.window.inLibrary');
			return life.more ? line.replace(/[.。]$/, '') : line;
		}
		if (life.stage === 'declined') {
			// Same rule as the card: a display name only, never a bare account name, and the status word is not said twice.
			const by = picked.state === 'turned_down' ? ctx.decidedBy(picked) : '';
			// Others who said no to the same title before, after this ask's own decider.
			const also = ctx.alsoDeclined(picked);
			const first = by && by !== picked.decided_by?.name ? t('requests.window.by', { name: by }) : picked.decided_at ? t('requests.window.on', { date: whenDate(picked.decided_at) }) : '';
			return [first, also].filter(Boolean).join(' ');
		}
		if (life.stage === 'failed') {
			const why = life.reason ? ` ${life.reason.charAt(0).toUpperCase()}${life.reason.slice(1)}${/[.!?]$/.test(life.reason) ? '' : '.'}` : '';
			return (picked.title_id ? t('requests.window.couldNotDownload') : t('requests.cards.couldNotAdd')) + why;
		}
		// Nothing in flight and nothing found yet: say why and when it last looked.
		if (life.status === 'looking for a release') {
			// Never looked for: when the search for missing comes round to it.
			if (!life.searchedAt) return life.nextSearch ? t('requests.window.notLookedNext', { when: nextWhen(life.nextSearch) }) : t('requests.window.notLooked');
			const why = life.reason ? `${life.reason.charAt(0).toUpperCase()}${life.reason.slice(1)}${/[.!?]$/.test(life.reason) ? '' : '.'}` : t('requests.window.nothingYet');
			return t('requests.window.lastLooked', { why, date: whenDate(life.searchedAt) });
		}
		return life.stage === 'upcoming' ? t('requests.window.gettingNotOut', { who }) : t('requests.window.getting', { who });
	});
	const year = $derived(d?.year ?? picked.year ?? null);
	/** What Approve would get, only where it narrows the choice: seasons for a show, albums for music. */
	const count = (a: Ask) => askedFor(a, { seasons: ctx.info(a)?.seasons.length });
	const pageHref = $derived(askHref(picked));
	const pageLabel = $derived(t(picked.kind === 'show' ? 'requests.window.pageShow' : picked.kind === 'artist' ? 'requests.window.pageArtist' : 'requests.window.pageMovie'));
	const kindOf = (a: Ask) => t(a.kind === 'show' ? 'requests.kind.show' : a.kind === 'artist' ? 'requests.kind.artist' : 'requests.kind.movie');

	// Choose what to get: the title lifts into the corner, the sheet rises inside the frame.
	let choosing = $state(false);
	// Opened by the window itself on Processing or Done: everything is already in place (no lift, no rise, no fade).
	let instant = $state(false);
	let frame = $state<HTMLElement>();
	let lift = $state(0);
	// Where the sheet starts: just under the lifted title block.
	let sheetTop = $state(180);
	let pick = $state<PartsPick>({ seasons: null, albums: null, empty: false });
	let rulesId = $state(0);
	const rules = $derived(ctx.rulesFor(picked));
	// Keyed on the id value only: a list refresh hands a new object for the same ask and must not throw away the ticks and the Quality pick.
	const pid = $derived(picked.id);
	$effect(() => {
		void pid;
		untrack(() => {
			pick = { seasons: null, albums: null, empty: false };
			rulesId = 0;
			// Stepping from one waiting ask to the next keeps What to add open (no half close).
			if (!(choosing && waiting(picked) && !status)) choosing = false;
		});
	});
	$effect(() => {
		if (choosing && !waiting(picked) && !status) choosing = false;
	});
	async function choose(now = false) {
		instant = now;
		// Processing and Done windows change the banner's layout when the list opens, so measure the title in that layout (list open, no lift yet) rather than before it. The title moves into the list's own header.
		if (status) {
			lift = 0;
			sheetTop = 72;
			choosing = true;
			await tick();
			frame?.querySelector<HTMLElement>('.sheet')?.focus({ preventScroll: true });
			return;
		}
		const content = frame?.querySelector<HTMLElement>('.ui-billboard .content');
		// How far the title travels: from where it sits to just under the tabs. Measured against the window itself, so it holds whatever the banner's own layout is.
		const fr = frame?.getBoundingClientRect();
		const k = fr && frame?.offsetWidth ? fr.width / frame.offsetWidth || 1 : 1;
		const top = fr?.top ?? 0;
		const box = content?.getBoundingClientRect();
		lift = box ? Math.max((box.top - top) / k - 64, 0) : 0;
		// The sheet starts 20 px under the lifted block's last line (the block keeps 72% of its size).
		const lines = content?.querySelectorAll<HTMLElement>('.meta > *, .facts, .asker, .said');
		let bottom = content?.querySelector<HTMLElement>('.meta')?.getBoundingClientRect().bottom ?? 0;
		for (const el of lines ?? []) bottom = Math.max(bottom, el.getBoundingClientRect().bottom);
		sheetTop = box && bottom ? Math.round(64 + ((bottom - box.top) / k) * 0.72 + 20) : 180;
		choosing = true;
		await tick();
		frame?.querySelector<HTMLElement>('.sheet')?.focus({ preventScroll: true });
	}
	// The cards' window opens straight into What to get, once per ask.
	let autoFor = 0;
	$effect(() => {
		const a = picked;
		// Processing and Done open on what was approved (read only); a movie has nothing to list.
		if (!startChoosing || autoFor === a.id || (!waiting(a) && !(status && a.kind !== 'movie'))) return;
		// Processing and Done are already open on the first paint, so nothing moves.
		if (status) {
			autoFor = a.id;
			void choose(true);
			return;
		}
		// Cleared when the window steps on first, so the sheet never opens on the next ask unasked.
		const timer = setTimeout(() => {
			autoFor = a.id;
			void choose();
		}, 120);
		return () => clearTimeout(timer);
	});
	const changed = $derived(!!(pick.seasons || pick.episodes || pick.albums || pick.songs));
	function approveChosen() {
		if (pick.empty) return;
		ctx.approveWith(picked, {
			rulesId,
			langs: [],
			note: '',
			...(pick.seasons ? { seasons: pick.seasons } : {}),
			...(pick.episodes ? { episodes: pick.episodes } : {}),
			...(pick.albums ? { albums: pick.albums } : {}),
			...(pick.songs ? { songs: pick.songs } : {})
		});
		closeSheet();
	}
	// Closing the sheet hands focus back to the button that opens it (or the window).
	function closeSheet() {
		instant = false;
		// Hide drops the ticks with the list, so the Quality pick goes with them: reopening always starts from what was asked, never half of an earlier choice.
		if (!status) {
			pick = { seasons: null, albums: null, empty: false };
			rulesId = 0;
		}
		choosing = false;
		void tick().then(() => {
			const opener = frame?.querySelector<HTMLElement>('[data-opens-sheet]');
			const to = opener ?? frame?.closest<HTMLElement>('dialog') ?? null;
			to?.focus({ preventScroll: true });
		});
	}
</script>

{#snippet line(a: Ask, life: Life)}
	<span class="life edge tone-{life.tone}" role="progressbar" aria-label={t('requests.window.progress', { name: a.name, label: life.label })} aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(life.pct)}><span class="fill" style:width="{life.pct}%"></span></span>
{/snippet}

{#snippet artistArt()}
	{#if artistBg}<div class="artist-bg" style:background-image={`url("${proxied(artistBg)}")`}></div>{/if}
	{#if mvideo}<div class="mv">{#key mvideo.key}<TrailerPlayer sources={pickedSources(mvideo.key, mvideo)} title={picked.name} sharp pausable top repeat start={heroVideoStart(mvideo)} />{/key}</div>{/if}
{/snippet}

<div class="desk" data-testid="ask-window-stage">
	<div class="frame" class:choosing class:status class:instant bind:this={frame} style:--lift="{lift}px" style:--sheet-top="{sheetTop}px">
			{#key picked.id}
			<div class="swap" in:fade={{ duration: status && startChoosing ? 0 : ms(520) }} out:fade={{ duration: ms(360) }}>
				<Billboard
					label={t('requests.window.bannerLabel', { name: picked.name, who: ctx.who(picked).name })}
					testid="ask-banner"
					kind={picked.kind === 'movie' || picked.kind === 'show' ? picked.kind : undefined}
					tmdb={picked.kind === 'artist' ? null : picked.tmdb_id}
					title={picked.name}
					{pictures}
					thumb={music ? (artistPhoto(picked) ? proxied(artistPhoto(picked)!) : null) : picked.poster ? to(picked.poster) : null}
					thumbSquare={music}
					position="60% 25%"
					art={music && (artistBg || mvideo) ? artistArt : undefined}
				>
					{#snippet heading()}<h2 class="bb-name" id="stage-title-{picked.id}">{picked.name}</h2>{/snippet}
					{#snippet meta()}
						<p class="facts"><span>{kindOf(picked)}</span>{#if picked.four_k}<span>{t('requests.askMovie.fourK')}</span>{/if}{#if year}<span>{year}</span>{/if}{#if count(picked)}<span>{count(picked)}</span>{/if}</p>
						{#each [ctx.who(picked)] as w (w.id)}
							<p class="asker"><Avatar name={w.name} src={w.avatar} size={22} /><span><b>{w.name}</b> {t('requests.window.asked')} <time datetime={picked.asked_at} title={dateTime(picked.asked_at)}>{agoLong(picked.asked_at)}</time>{#if watchlistLine(picked.people, undefined)}{' '}{t('requests.window.fromPlex')}{/if}</span>{#if w.kid}<KidMark label={t('requests.window.kid')} size="sm" />{/if}</p>
						{/each}
						{#if picked.note}<p class="said">&ldquo;{picked.note}&rdquo;</p>{/if}
					{/snippet}
					{#snippet actions()}
						<div class="tl-row">
						{#if status}
							<span></span>
						{:else if waiting(picked)}
							<div class="acts" class:gone={choosing} inert={choosing}>
								<button type="button" class="act glass st-half" aria-label={t('requests.window.declineName', { name: picked.name })} data-testid="ask-decline" onclick={() => ctx.turnDown([picked])}><Icon name="x" size={20} />{t('requests.window.decline')}</button>
								<button type="button" class="act primary st-half" aria-label={t('requests.window.approveName', { name: picked.name })} data-testid="ask-approve" data-opens-sheet onclick={() => void choose()}><Icon name="check" size={20} />{t('requests.window.approve')}</button>
							</div>
						{:else}
							<div role="status" aria-live="polite">
								{#if ctx.isPending(picked.id)}
									<p class="state">{t('requests.window.sending')}</p>
								{:else}
									<p class="state">{ctx.decided(picked) || lifecycle(picked).label}{#if lifecycle(picked).stage === 'getting'}<span class="sep">·</span>{t('requests.window.gettingNow')}{/if}</p>
								{/if}
							</div>
						{/if}
						</div>
					{/snippet}
				</Billboard>
			</div>
			{/key}
			{#if pageHref && !choosing}<a class="tl-link tl-float" class:tl-wait={!status && waiting(picked)} href={to(pageHref)} data-testid="ask-title-page">{pageLabel}<Icon name="chevronRight" size={16} /></a>{/if}
			{#if status}
				<section class="big" data-testid="ask-status" aria-label={t('requests.window.status')}>
					<span class="life inline tone-{life.tone}" role="progressbar" aria-label={t('requests.window.progress', { name: picked.name, label: life.label })} aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(life.pct)}><span class="fill" style:width="{life.pct}%"></span></span>
					<div class="says">
						<p class="huge tone-{life.tone}">{life.label}</p>
						<p class="sent">{sentence}{#if life.more}<span class="more">{` · ${life.more}`}</span>{/if}</p>
						{#if dlId}<a class="tl-link" href={to(`/downloads?open=${dlId}`)} data-testid="ask-download">{t('requests.window.seeDownload')}<Icon name="chevronRight" size={16} /></a>{/if}
					</div>
					{#if picked.kind !== 'movie' && !choosing}
						<button type="button" class="act glass peek" data-opens-sheet onclick={() => void choose()}><Icon name="chevronUp" size={18} />{listName}</button>
					{/if}
					{#if failed}
						<div class="acts"><button type="button" class="act glass" aria-label={t('requests.window.declineName', { name: picked.name })} data-testid="ask-decline" onclick={() => ctx.turnDown([picked])}><Icon name="x" size={20} /><span class="bl">{t('requests.window.decline')}</span></button></div>
					{/if}
				</section>
			{:else}
				{@render line(picked, life)}
			{/if}

			{#if choosing}
				<section class="sheet" tabindex="-1" aria-labelledby="sheet-h" transition:fly={{ y: 40, duration: instant ? 0 : ms(420), easing: cubicOut, opacity: 0 }}>
					<header class="sh-h">
						<h3 id="sheet-h">{#if status}<span class="sh-title">{picked.name}</span><span class="sh-kind">{listName}</span>{:else}{t('requests.window.whatToAdd')}{/if}</h3>
						{#if pageHref}<a class="tl-link tl-sheet" href={to(pageHref)} data-testid="ask-title-page-sheet">{pageLabel}<Icon name="chevronRight" size={16} /></a>{/if}
						<button type="button" class="st-hide" aria-label={t('requests.window.hide', { what: status ? listName : t('requests.window.whatToAdd') })} title={t('requests.window.hideShort')} onclick={closeSheet}><span>{t('requests.window.hideShort')}</span><Icon name="chevronDown" size={18} /></button>
					</header>
					<div class="sh-body">
						{#if picked.kind === 'movie'}
							<div class="one"><span class="one-pic"><Poster src={picked.poster} name={picked.name} /></span><div><strong>{picked.name}</strong><span>{t('requests.window.theMovie')}{year ? ` · ${year}` : ''}{d?.runtime ? ` · ${Math.floor(d.runtime / 60) ? `${t('requests.window.hoursShort', { n: Math.floor(d.runtime / 60) })} ` : ''}${t('requests.window.minutesShort', { n: d.runtime % 60 })}` : ''}</span></div><Icon name="check" size={18} /></div>
						{:else}
							{#if status}
								<AskParts {ctx} a={picked} unfolded layout="split" />
							{:else}
								<AskParts {ctx} a={picked} editable unfolded layout="split" bind:pick />
							{/if}
						{/if}
						{#if rules.length && !status}
							<div class="rules" role="group" aria-label={t('requests.window.quality')}>
								<span class="rl">{t('requests.window.quality')}</span>
								<button type="button" class="rule" aria-pressed={rulesId === 0} onclick={() => (rulesId = 0)}>{#if ctx.defaultRule(picked)}{ctx.defaultRule(picked)}<span class="dim">{t('requests.window.defaultParen')}</span>{:else}{t('requests.window.default')}{/if}</button>
								{#each rules as r (r.id)}
									<button type="button" class="rule" aria-pressed={rulesId === r.id} onclick={() => (rulesId = r.id)}>{r.name}</button>
								{/each}
							</div>
						{/if}
					</div>
					{#if !status}
					<footer class="sh-f">
						<span class="sum">{pick.empty ? t('requests.window.tickOne') : changed ? t('requests.window.addsTicked') : picked.kind === 'movie' ? t('requests.window.addsMovie') : t('requests.window.addsWhat', { what: (count(picked) || t('requests.window.everything')).toLowerCase() })}</span>
						<button type="button" class="act ghost" onclick={closeSheet}>{t('requests.window.back')}</button>
						<button type="button" class="act primary" disabled={pick.empty} onclick={approveChosen}><Icon name="check" size={18} />{t('requests.window.approve')}</button>
					</footer>
					{/if}
				</section>
			{/if}
	</div>
</div>

<style>
	.desk { --h: 100%; display: grid; grid-template-columns: minmax(0, 1fr); height: 100%; }
	.desk .frame { border-radius: 0; box-shadow: none; }
	.desk .frame :global(.ui-billboard) { border-radius: 0; }
	button { font: inherit; color: inherit; }
	button:focus-visible { outline: none; box-shadow: var(--focus-ring); }

	.frame { position: relative; height: var(--h); border-radius: 24px; overflow: hidden; isolation: isolate; box-shadow: 0 0 0 1px var(--line), 0 40px 80px -40px rgb(0 0 0 / 0.7); }
	.frame :global(.ui-billboard) { height: 100%; min-height: 0; margin: 0; border-radius: 24px; }
	/* Trailers ship letterboxed (wide films in a 16:9 file) and the tall frame would show the bands, so zoom past them. */
	.frame :global(.ui-billboard .art video), .frame :global(.ui-billboard .art iframe) { scale: 1.34; }
	.mv { position: absolute; inset: 0; }
	.artist-bg { position: absolute; inset: 0; background-size: cover; background-position: center 25%; animation: drift 30s ease-in-out infinite alternate; }
	@keyframes drift { to { transform: scale(1.08) translate(-1.5%, -1%); } }

	.frame :global(.ui-billboard .content) { width: min(760px, 92%); transform-origin: left top; transition: transform 560ms cubic-bezier(0.22, 1, 0.36, 1), opacity 300ms var(--ease-out); }
	.frame.choosing :global(.ui-billboard .content) { transform: translateY(calc(-1 * var(--lift))) scale(0.72); }
	.frame.choosing :global(.ui-billboard .shade) { opacity: 1; background: linear-gradient(to bottom, rgb(0 0 0 / 0.65), rgb(0 0 0 / 0.35) 40%, rgb(0 0 0 / 0.75)); }
	.facts { display: flex; flex-wrap: nowrap; align-items: center; gap: 0; white-space: nowrap; }
	.facts span + span::before { content: '·'; margin: 0 10px; opacity: 0.6; }
	.state { color: rgb(255 255 255 / 0.85); font: var(--t-callout); }
	.asker { flex-basis: 100%; display: inline-flex; align-items: center; gap: 8px; margin-top: 2px; color: rgb(255 255 255 / 0.82); font: 500 14px/1.3 var(--font-ui) !important; }
	.asker b { color: #fff; font-weight: 700; }
	/* The banner art is always dark, so the Kid mark keeps a light ink in both themes. */
	.asker :global(.kid.kid) { color: #fff; background: rgb(0 0 0 / 0.55); box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--mark-ask) 70%, transparent); }
	.said { flex-basis: 100%; max-width: 520px; padding-left: 12px; border-left: 2px solid rgb(255 255 255 / 0.45); color: rgb(255 255 255 / 0.9); font: italic 400 15px/1.45 var(--font-ui) !important; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
	.sep { margin: 0 8px; opacity: 0.6; }

	.acts { display: flex; flex-wrap: wrap; gap: 10px; transition: opacity 220ms var(--ease-out), transform 300ms var(--ease-out); }
	.acts.gone { opacity: 0; transform: translateY(8px); pointer-events: none; }
	.act { display: inline-flex; align-items: center; justify-content: center; gap: 8px; height: 50px; padding: 0 22px; border: 0; border-radius: var(--r-pill); font: var(--t-headline); cursor: pointer; transition: transform 160ms var(--ease-out), background 200ms var(--ease-out), box-shadow 200ms var(--ease-out); }
	.act:active { transform: scale(0.96); }
	.act.primary { background: #fff; color: #0b0b0c; box-shadow: 0 10px 30px -10px rgb(0 0 0 / 0.6); }
	.act.primary:hover { transform: translateY(-1px); box-shadow: 0 14px 34px -10px rgb(0 0 0 / 0.7); }
	.act.primary:disabled { opacity: 0.4; cursor: default; transform: none; }
	.act.glass { background: rgb(255 255 255 / 0.18); color: #fff; box-shadow: inset 0 0 0 1px rgb(255 255 255 / 0.16); -webkit-backdrop-filter: blur(18px); backdrop-filter: blur(18px); }
	.act.glass:hover { background: rgb(255 255 255 / 0.28); }
	.act.round { width: 50px; padding: 0; }
	.act.ghost { background: none; color: var(--text-muted); height: 44px; }
	.act.ghost:hover { color: var(--text); background: var(--surface-2); }

	.life { position: absolute; left: 0; right: 0; bottom: 0; height: 3px; background: rgb(127 127 127 / 0.08); overflow: hidden; pointer-events: none; }
	.life.edge { z-index: 3; height: 4px; background: rgb(255 255 255 / 0.12); }
	.fill { display: block; height: 100%; border-radius: 0 3px 3px 0; transition: width 900ms cubic-bezier(0.22, 1, 0.36, 1), background 400ms var(--ease-out); }
	.tone-wait .fill { background: var(--wait); }
	.tone-go .fill { background: linear-gradient(90deg, var(--info, #3b82f6), color-mix(in srgb, var(--info, #3b82f6) 55%, #fff)); background-size: 200% 100%; animation: flow 2.4s linear infinite; }
	.tone-ok .fill { background: var(--ok); }
	.tone-off .fill { background: rgb(127 127 127 / 0.45); }
	.tone-bad .fill { background: var(--fail, #e5484d); }
	@keyframes flow { to { background-position: -200% 0; } }

	.sheet { position: absolute; left: 14px; right: 14px; top: var(--sheet-top, 180px); bottom: 14px; z-index: 4; display: flex; flex-direction: column; border-radius: 20px; background: color-mix(in srgb, var(--bg) 84%, transparent); color: var(--text); box-shadow: 0 0 0 1px rgb(255 255 255 / 0.08), 0 30px 60px -20px rgb(0 0 0 / 0.7); -webkit-backdrop-filter: blur(30px) saturate(1.5); backdrop-filter: blur(30px) saturate(1.5); }
	.sh-h { display: flex; align-items: center; justify-content: space-between; padding: 10px 10px 6px 16px; }
	.sh-h h3 { font: var(--t-headline); }
	.sh-h h3:has(.sh-title) { display: flex; align-items: baseline; gap: 10px; min-width: 0; }
	.sh-title { font: 800 20px/1.1 var(--font-display, var(--font-ui)); letter-spacing: -0.01em; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.sh-kind { color: var(--text-muted); font: 500 13px/1 var(--font-ui); white-space: nowrap; }
	.frame.status.choosing :global(.ui-billboard .content) { opacity: 0; pointer-events: none; }
	/* Minimal look: the poster fades with the words while choosing; on phones the words keep clear of the close button and pager. */
	.frame :global(.ui-billboard.minimal .thumb) { transition: opacity 300ms var(--ease-out); }
	.frame.choosing :global(.ui-billboard.minimal .thumb) { opacity: 0; }
	@media (max-width: 767px) {
		.frame :global(.ui-billboard.minimal) { padding-top: 64px; }
		.sh-h:has(.sh-title) { flex-wrap: wrap; row-gap: 6px; }
		.sh-h h3:has(.sh-title) { flex: 1 0 100%; flex-direction: column; align-items: flex-start; gap: 2px; }
		.sh-h:has(.sh-title) .tl-sheet { margin-left: 0; }
	}
	.sheet :global(.parts) { background: none; box-shadow: inset 0 0 0 1px var(--line); border-radius: 12px; }
	.sheet :global(.head) { min-height: 38px; font: var(--t-footnote); font-weight: 600; color: var(--text-muted); }
	.sheet :global(.line) { min-height: 36px; gap: 10px; }
	.sheet :global(.cover) { width: 26px; height: 26px; }
	.sheet :global(.list > ul) { max-height: none; }
	.sheet :global(.season) { min-height: 36px; }
	.sheet:focus { outline: none; }
	.st-hide { display: inline-flex; align-items: center; gap: 4px; height: 36px; padding: 0 10px 0 14px; border: 1px solid var(--line); border-radius: var(--r-pill); background: var(--surface-2); color: var(--ink-2, inherit); font: 600 13px/1 var(--font-ui); cursor: pointer; transition: background 160ms var(--ease-out); }
	.st-hide:hover { background: var(--surface-3); }
	.tl-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; width: 100%; }
	.tl-link { display: inline-flex; align-items: center; gap: 4px; flex: none; margin-left: auto; height: 36px; padding: 0 10px 0 14px; border-radius: var(--r-pill); background: rgb(255 255 255 / 0.18); color: #fff; box-shadow: inset 0 0 0 1px rgb(255 255 255 / 0.16); -webkit-backdrop-filter: blur(18px); backdrop-filter: blur(18px); font: 600 13px/1 var(--font-ui); text-decoration: none; white-space: nowrap; }
	.tl-link:hover { background: rgb(255 255 255 / 0.28); }
	.tl-float { position: absolute; z-index: 3; right: 28px; bottom: 44px; margin: 0; }
	.frame.status .tl-float { bottom: calc(var(--big-h) + 22px); }
	/* On bright art the white glass alone is too faint: use the same smoke as the window controls. */
	.tl-float { background: rgb(20 20 24 / 0.55); }
	.tl-float:hover { background: rgb(20 20 24 / 0.7); }
	/* Phones: the answer row spans the banner, so the link waits in the sheet header instead. */
	@media (max-width: 767px) { .tl-float { right: 16px; bottom: 28px; } .tl-float.tl-wait { display: none; } }
	.tl-link:focus-visible { outline: 2px solid #fff; outline-offset: 2px; }
	.tl-link.tl-sheet { margin-left: auto; margin-right: 8px; background: var(--surface-2); color: var(--ink-2, inherit); box-shadow: none; border: 1px solid var(--line); -webkit-backdrop-filter: none; backdrop-filter: none; }
	.tl-link.tl-sheet:hover { background: var(--surface-3); }
	.st-half { flex: 1 1 0; min-width: 0; }
	.acts:has(.st-half) { flex-wrap: nowrap; width: min(420px, 100%); }
	.frame.instant :global(.ui-billboard .content) { transition: none; }
	.frame.status :global(.ui-kb img), .frame.status :global(.ui-kb .frame), .frame.status .artist-bg { animation: none; transition: none; }
	.sh-body { flex: 1; min-height: 0; overflow: hidden; padding: 0 16px 10px; display: flex; flex-direction: column; gap: 10px; }
	.sh-body > :global(.split) { flex: 1; min-height: 0; }
	.sh-body > :global(.parts) { flex: none; }
	.rules { flex: none; }
	.one { display: flex; align-items: center; gap: 14px; padding: 10px; border-radius: 14px; background: var(--surface-2); }
	.one-pic { width: 44px; border-radius: 6px; overflow: hidden; --poster-ratio: 2 / 3; }
	.one div { flex: 1; display: flex; flex-direction: column; font: var(--t-callout); }
	.one div span { color: var(--text-muted); font: var(--t-footnote); }
	.rules { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
	.rl { margin-right: 6px; color: var(--text-muted); font: var(--t-footnote); font-weight: 600; }
	.rule { display: inline-flex; align-items: center; gap: 6px; min-height: 36px; padding: 0 12px; border: 0; border-radius: var(--r-pill); background: var(--surface-2); font: var(--t-subhead); font-weight: 600; cursor: pointer; transition: background 200ms var(--ease-out), color 200ms var(--ease-out); }
	.rule:hover { background: var(--surface-3); }
	.rule[aria-pressed='true'] { background: var(--text); color: var(--bg); }
	.dim { opacity: 0.6; font-weight: 400; }
	.sh-f { display: flex; align-items: center; gap: 8px; padding: 10px 10px 10px 16px; border-top: 1px solid var(--line); }
	.sum { flex: 1; color: var(--text-muted); font: var(--t-footnote); }
	.sh-f .act.primary { background: var(--text); color: var(--bg); height: 40px; padding: 0 18px; font: var(--t-subhead); font-weight: 600; }
	.sh-f .act.ghost { height: 40px; font: var(--t-subhead); }

	.swap { position: absolute; inset: 0; }

	.frame.status:not(.choosing) .swap { bottom: auto; height: calc(100% - var(--big-h)); }
	.frame.status { --big-h: 132px; }
	.frame.status :global(.ui-billboard .shade) { opacity: 1; }
	.frame.status .sheet { bottom: calc(var(--big-h) + 14px); }
	.big { position: absolute; left: 0; right: 0; bottom: 0; height: var(--big-h); display: flex; align-items: center; gap: 20px; padding: 0 28px; background: #0b0b0c; color: #fff; }
	.says { display: flex; flex-direction: column; gap: 4px; min-width: 0; flex: 1; }
	.huge { font: 800 34px/1.1 var(--font-ui); letter-spacing: -0.02em; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	/* The status band is dark in both themes, so it keeps dark-theme tones (light ones fall under 3:1 here). */
	.huge.tone-wait { color: #ffb340; }
	.huge.tone-go { color: #6aa6ff; }
	.huge.tone-ok { color: #4ad27a; }
	.huge.tone-bad { color: #ff6961; }
	.huge.tone-soon, .huge.tone-off { color: rgb(255 255 255 / 0.7); }
	.sent { color: rgb(255 255 255 / 0.8); font: var(--t-callout); }
	.more { color: rgb(255 255 255 / 0.5); }
	.life.inline { position: absolute; left: 0; right: 0; top: 0; height: 3px; background: rgb(255 255 255 / 0.1); }
	.big .acts { flex: none; }
	.peek { flex: none; }
	@media (max-width: 767px) {
		.frame.status { --big-h: 112px; }
		.huge { font-size: 26px; }
		.big { padding: 0 20px; gap: 12px; }
		.frame.status:has(.big .peek), .frame.status:has(.big .acts) { --big-h: 156px; }
		.big { flex-wrap: wrap; align-content: center; row-gap: 12px; }
		.says { flex: 1 0 100%; }
		.peek { flex: 1 1 0; min-width: 0; }
		.big .acts { flex: none; margin-left: auto; }
		.big .acts .act { width: 46px; padding: 0; }
		.big .acts .bl { display: none; }
	}

	@media (max-width: 767px) {
		.facts { flex-wrap: wrap; white-space: normal; row-gap: 2px; }
		.act { height: 46px; padding: 0 18px; }
		.tl-link { height: 44px; }
		.rules { flex-wrap: nowrap; overflow-x: auto; scrollbar-width: none; overscroll-behavior-x: contain; margin: 0 -16px; padding: 0 16px; }
		.rules::-webkit-scrollbar { display: none; }
		.rule, .rl { flex: none; white-space: nowrap; }
		.sh-f { flex-wrap: wrap; }
		.sum { flex: 1 0 100%; }
		.sh-f .act.ghost { margin-left: auto; }
	}
	@media (pointer: coarse) {
		.rule { min-height: 44px; }
		.st-hide, .tl-link { height: 44px; }
		.sheet :global(.line), .sheet :global(.season) { min-height: 44px; }
		.sh-f .act.primary, .sh-f .act.ghost { height: 44px; }
	}
	@media (prefers-reduced-motion: reduce) {
		.frame :global(.ui-billboard .content), .fill, .acts, .act { transition: none; }
		.tone-go .fill, .artist-bg { animation: none; }
	}
</style>
