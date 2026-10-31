<script lang="ts">
	// One ask in the cards' window (ApprovalsDesktop): the trailer as a big
	// banner (Explore's Billboard; for music, the artist's video over the
	// photo), type · year · what was asked, who asked and their note, then
	// Approve / Choose what to get / Turn down. "Choose what to get" lifts the
	// title into the corner and raises the What to get sheet inside the same
	// frame: the two-pane picker (seasons and episodes, or albums and songs),
	// the Pick rule, then Approve. The life-cycle line runs along the edge.
	import { tick } from 'svelte';
	import { fade, fly } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import Billboard from '../ui/Billboard.svelte';
	import TrailerPlayer from '../ui/TrailerPlayer.svelte';
	import { artistVideo, autoplayHere, loadUISettings } from '../autoplay.svelte.ts';
	import { heroVideoStart, pickedSources, type HeroVideo } from '../music/hero-video.ts';
	import Poster from '../Poster.svelte';
	import Icon from '../Icon.svelte';
	import Avatar from '../ui/Avatar.svelte';
	import KidMark from './KidMark.svelte';
	import AskParts from './AskParts.svelte';
	import { ago, dateTime } from '../format.ts';
	import { to } from '../paths.ts';
	import type { ApprovalsCtx } from './approvals.ts';
	import { artistIdOf, artistPhoto, askArtist } from './details.svelte.ts';
	import { lifecycle, upcoming, type Life, type LifeFacts } from './lifecycle.ts';
	import { partialFacts } from './live.ts';
	import { api } from '../api.ts';
	import type { AlbumSummary, TitleView } from '../types.ts';
	import { shortDate } from './inbox.ts';
	import { askTab } from '../wishlist.ts';
	import { askedFor } from './asked-for.ts';
	import type { PartsPick } from './ask-parts.ts';
	import type { Ask } from '../types.ts';

	let { ctx, single, choose: startChoosing = false }: { ctx: ApprovalsCtx; /** The ask this window shows. */ single: Ask; /** Open straight into What to get. */ choose?: boolean } = $props();

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
	$effect(() => {
		const a = picked;
		mvideo = null;
		if (a.kind !== 'artist') return;
		const id = artistIdOf(a);
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
	$effect(() => {
		const a = picked;
		if (!a.title_id || (a.state !== 'approved' && a.state !== 'available') || a.kind === 'movie') return;
		const id = a.title_id;
		const ctl = new AbortController();
		const load =
			a.kind === 'show'
				? api.get<TitleView>(`/titles/${id}`, ctl.signal).then((t) =>
						partialFacts('seasons', (t.show?.seasons ?? []).filter((s) => s.number > 0 && !s.dropped).map((s) => ({ key: s.number, complete: s.aired > 0 && s.in_library >= s.aired })), a.seasons ?? [])
					)
				: api.get<{ albums: AlbumSummary[] }>(`/titles/${id}/albums`, ctl.signal).then((r) =>
						partialFacts('albums', r.albums.filter((x) => !x.dropped).map((x) => ({ key: x.mbid, complete: x.tracks > 0 && x.in_library >= x.tracks })), (a.albums ?? []).map((x) => x.mbid))
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
		return lifecycle(a, out ? { ...part, upcoming: shortDate(out) } : part);
	});
	const failed = $derived(status && askTab(picked) === 'failed');
	const sentence = $derived.by(() => {
		const who = ctx.who(picked).name;
		if (life.stage === 'ready') {
			const at = picked.available_at ?? picked.decided_at;
			return `In the library${at ? ` since ${shortDate(at)}` : ''}.`;
		}
		if (life.stage === 'declined') return ctx.decided(picked) ? `${ctx.decided(picked)}.` : 'Declined.';
		if (life.stage === 'failed') return 'Couldn’t find it.';
		return life.stage === 'upcoming' ? `Getting it for ${who}. It isn’t out yet.` : `Getting it for ${who}.`;
	});
	const year = $derived(d?.year ?? picked.year ?? null);
	/** What Approve would get, only where it narrows the choice: seasons for a show, albums for music. */
	const count = (a: Ask) => askedFor(a, { seasons: ctx.info(a)?.seasons.length });
	const kindOf = (a: Ask) => (a.kind === 'show' ? 'TV show' : a.kind === 'artist' ? 'Music' : 'Movie');

	// Choose what to get: the title lifts into the corner, the sheet rises inside the frame.
	let choosing = $state(false);
	let frame = $state<HTMLElement>();
	let lift = $state(0);
	// Where the sheet starts: just under the lifted title block.
	let sheetTop = $state(180);
	let pick = $state<PartsPick>({ seasons: null, albums: null, empty: false });
	let rulesId = $state(0);
	const rules = $derived(ctx.rulesFor(picked));
	$effect(() => {
		void picked.id;
		choosing = false;
		rulesId = 0;
	});
	$effect(() => {
		if (choosing && !waiting(picked) && !status) choosing = false;
	});
	async function choose() {
		const content = frame?.querySelector<HTMLElement>('.ui-billboard .content');
		// How far the title travels: from where it sits to just under the tabs.
		lift = content ? Math.max(content.offsetTop - 64, 0) : 0;
		// The sheet starts 20 px under the lifted meta line (the block keeps 72 % of its size).
		const meta = content?.querySelector<HTMLElement>('.meta');
		sheetTop = meta ? Math.round(64 + (meta.offsetTop + meta.offsetHeight) * 0.72 + 20) : 180;
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
		// Cleared when the window steps on first, so the sheet never opens on the next ask unasked.
		const t = setTimeout(() => {
			autoFor = a.id;
			void choose();
		}, 120);
		return () => clearTimeout(t);
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
		choosing = false;
	}
	function keys(e: KeyboardEvent) {
		if (document.querySelector('dialog[open]')) return;
		if (e.key === 'Escape' && choosing) {
			e.preventDefault();
			choosing = false;
		}
	}
</script>

<svelte:window onkeydown={keys} />

<!-- The life-cycle line along the banner's bottom edge. -->
{#snippet line(a: Ask, life: Life)}
	<span class="life edge tone-{life.tone}" role="progressbar" aria-label="{a.name}: {life.label}" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(life.pct)}><span class="fill" style:width="{life.pct}%"></span></span>
{/snippet}

{#snippet artistArt()}
	{#if artistBg}<div class="artist-bg" style:background-image={`url("${artistBg}")`}></div>{/if}
	{#if mvideo}<div class="mv">{#key mvideo.key}<TrailerPlayer sources={pickedSources(mvideo.key, mvideo)} title={picked.name} sharp pausable top repeat start={heroVideoStart(mvideo)} />{/key}</div>{/if}
{/snippet}

<div class="desk" data-testid="ask-window-stage">
	<div class="frame" class:choosing class:status bind:this={frame} style:--lift="{lift}px" style:--sheet-top="{sheetTop}px">
			{#key picked.id}
			<div class="swap" in:fade={{ duration: ms(520) }} out:fade={{ duration: ms(360) }}>
				<Billboard
					label="{picked.name}, asked by {ctx.who(picked).name}"
					testid="ask-banner"
					kind={picked.kind === 'movie' || picked.kind === 'show' ? picked.kind : undefined}
					tmdb={picked.kind === 'artist' ? null : picked.tmdb_id}
					title={picked.name}
					{pictures}
					position="60% 25%"
					art={music && (artistBg || mvideo) ? artistArt : undefined}
				>
					{#snippet heading()}<h2 class="bb-name" id="stage-title">{picked.name}</h2>{/snippet}
					{#snippet meta()}
						<p class="facts"><span>{kindOf(picked)}</span>{#if year}<span>{year}</span>{/if}{#if count(picked)}<span>{count(picked)}</span>{/if}</p>
						{#each [ctx.who(picked)] as w (w.id)}
							<p class="asker"><Avatar name={w.name} src={w.avatar} size={22} /><span><b>{w.name}</b> asked <time datetime={picked.asked_at} title={dateTime(picked.asked_at)}>{ago(picked.asked_at)}</time></span>{#if w.kid}<KidMark label="Kid" size="sm" />{/if}</p>
						{/each}
						{#if picked.note}<p class="said">&ldquo;{picked.note}&rdquo;</p>{/if}
					{/snippet}
					{#snippet actions()}
						{#if status}
							<span></span>
						{:else if waiting(picked)}
							<div class="acts" class:gone={choosing}>
								<button type="button" class="act primary" data-testid="ask-approve" onclick={() => ctx.approve([picked])}><Icon name="check" size={20} />Approve</button>
								<button type="button" class="act glass" onclick={() => void choose()}><Icon name="tools" size={20} />Choose what to add</button>
								<button type="button" class="act glass" data-testid="ask-decline" onclick={() => ctx.turnDown([picked])}>Decline</button>
							</div>
						{:else}
							<div role="status" aria-live="polite">
								{#if ctx.isPending(picked.id)}
									<p class="state">Sending…</p>
								{:else}
									<p class="state">{ctx.decided(picked) || lifecycle(picked).label}{#if lifecycle(picked).stage === 'getting'}<span class="sep">·</span>getting it now{/if}</p>
								{/if}
							</div>
						{/if}
					{/snippet}
				</Billboard>
			</div>
			{/key}
			{#if status}
				<section class="big" data-testid="ask-status" aria-label="Status">
					<span class="life inline tone-{life.tone}" role="progressbar" aria-label="{picked.name}: {life.label}" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(life.pct)}><span class="fill" style:width="{life.pct}%"></span></span>
					<div class="says">
						<p class="huge tone-{life.tone}" role="status" aria-live="polite">{life.label}</p>
						<p class="sent">{sentence}{#if life.more}<span class="more"> · {life.more}</span>{/if}</p>
					</div>
					{#if picked.kind !== 'movie' && !choosing}
						<button type="button" class="act glass peek" onclick={() => void choose()}><Icon name="chevronUp" size={18} />What was approved</button>
					{/if}
					{#if failed}
						<div class="acts"><button type="button" class="act glass" data-testid="ask-decline" onclick={() => ctx.turnDown([picked])}><Icon name="x" size={20} />Decline</button></div>
					{/if}
				</section>
			{:else}
				{@render line(picked, life)}
			{/if}

			{#if choosing}
				<section class="sheet" tabindex="-1" aria-labelledby="sheet-h" transition:fly={{ y: 40, duration: ms(420), easing: cubicOut, opacity: 0 }}>
					<header class="sh-h">
						<h3 id="sheet-h">{status ? 'What was approved' : 'What to add'}</h3>
						<button type="button" class="x" aria-label={status ? 'Collapse' : 'Close'} onclick={() => (choosing = false)}><Icon name={status ? 'chevronDown' : 'x'} size={18} /></button>
					</header>
					<div class="sh-body">
						{#if picked.kind === 'movie'}
							<div class="one"><span class="one-pic"><Poster src={picked.poster} name={picked.name} /></span><div><strong>{picked.name}</strong><span>The movie{year ? ` · ${year}` : ''}{d?.runtime ? ` · ${Math.floor(d.runtime / 60) ? `${Math.floor(d.runtime / 60)}h ` : ''}${d.runtime % 60}m` : ''}</span></div><Icon name="check" size={18} /></div>
						{:else}
							{#if status}
								<AskParts {ctx} a={picked} unfolded layout="split" />
							{:else}
								<AskParts {ctx} a={picked} editable unfolded layout="split" bind:pick />
							{/if}
						{/if}
						{#if rules.length && !status}
							<div class="rules" role="group" aria-label="Pick rules">
								<span class="rl">Pick rules</span>
								<button type="button" class="rule" aria-pressed={rulesId === 0} onclick={() => (rulesId = 0)}>{#if ctx.defaultRule(picked)}{ctx.defaultRule(picked)}<span class="dim">· default</span>{:else}Default pick rules{/if}</button>
								{#each rules as r (r.id)}
									<button type="button" class="rule" aria-pressed={rulesId === r.id} onclick={() => (rulesId = r.id)}>{r.name}</button>
								{/each}
							</div>
						{/if}
					</div>
					{#if !status}
					<footer class="sh-f">
						<span class="sum">{pick.empty ? 'Tick at least one to approve' : changed ? 'Approving adds only what you ticked' : picked.kind === 'movie' ? `Approving adds the movie` : `Approving adds ${(count(picked) || 'everything they asked for').toLowerCase()}`}</span>
						<button type="button" class="act ghost" onclick={() => (choosing = false)}>Back</button>
						<button type="button" class="act primary" disabled={pick.empty} onclick={approveChosen}><Icon name="check" size={18} />Approve</button>
					</footer>
					{/if}
				</section>
			{/if}
	</div>
</div>

<style>
	/* The desk fills its window box. */
	.desk { --h: 100%; display: grid; grid-template-columns: minmax(0, 1fr); height: 100%; }
	.desk .frame { border-radius: 0; box-shadow: none; }
	.desk .frame :global(.ui-billboard) { border-radius: 0; }
	button { font: inherit; color: inherit; }
	button:focus-visible { outline: none; box-shadow: var(--focus-ring); }

	/* The frame: Billboard at a fixed height; nothing in it changes the frame's size. */
	.frame { position: relative; height: var(--h); border-radius: 24px; overflow: hidden; isolation: isolate; box-shadow: 0 0 0 1px var(--line), 0 40px 80px -40px rgb(0 0 0 / 0.7); }
	.frame :global(.ui-billboard) { height: 100%; min-height: 0; margin: 0; border-radius: 24px; }
	/* Trailers ship letterboxed (wide films in a 16:9 file); the tall frame would show the bands, so zoom past them. */
	.frame :global(.ui-billboard .art video), .frame :global(.ui-billboard .art iframe) { scale: 1.34; }
	.mv { position: absolute; inset: 0; }
	.artist-bg { position: absolute; inset: 0; background-size: cover; background-position: center 25%; animation: drift 30s ease-in-out infinite alternate; }
	@keyframes drift { to { transform: scale(1.08) translate(-1.5%, -1%); } }

	/* The title block lifts and shrinks into the corner; the frame stays. */
	.frame :global(.ui-billboard .content) { width: min(760px, 92%); transform-origin: left top; transition: transform 560ms cubic-bezier(0.22, 1, 0.36, 1), opacity 300ms var(--ease-out); }
	.frame.choosing :global(.ui-billboard .content) { transform: translateY(calc(-1 * var(--lift))) scale(0.72); }
	.frame.choosing :global(.ui-billboard .shade) { opacity: 1; background: linear-gradient(to bottom, rgb(0 0 0 / 0.65), rgb(0 0 0 / 0.35) 40%, rgb(0 0 0 / 0.75)); }
	.facts { display: flex; flex-wrap: nowrap; align-items: center; gap: 0; white-space: nowrap; }
	.facts span + span::before { content: '·'; margin: 0 10px; opacity: 0.6; }
	.state { color: rgb(255 255 255 / 0.85); font: var(--t-callout); }
	.asker { flex-basis: 100%; display: inline-flex; align-items: center; gap: 8px; margin-top: 2px; color: rgb(255 255 255 / 0.82); font: 500 14px/1.3 var(--font-ui) !important; }
	.asker b { color: #fff; font-weight: 700; }
	.said { flex-basis: 100%; max-width: 520px; padding-left: 12px; border-left: 2px solid rgb(255 255 255 / 0.45); color: rgb(255 255 255 / 0.9); font: italic 400 15px/1.45 var(--font-ui) !important; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
	.sep { margin: 0 8px; opacity: 0.6; }

	/* Answers on the art. */
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

	/* The life-cycle line along the banner's bottom edge. */
	.life { position: absolute; left: 0; right: 0; bottom: 0; height: 3px; background: rgb(127 127 127 / 0.08); overflow: hidden; pointer-events: none; }
	.life.edge { z-index: 3; height: 4px; background: rgb(255 255 255 / 0.12); }
	.fill { display: block; height: 100%; border-radius: 0 3px 3px 0; transition: width 900ms cubic-bezier(0.22, 1, 0.36, 1), background 400ms var(--ease-out); }
	.tone-wait .fill { background: var(--wait); }
	.tone-go .fill { background: linear-gradient(90deg, var(--info, #3b82f6), color-mix(in srgb, var(--info, #3b82f6) 55%, #fff)); background-size: 200% 100%; animation: flow 2.4s linear infinite; }
	.tone-ok .fill { background: var(--ok); }
	.tone-off .fill { background: rgb(127 127 127 / 0.45); }
	.tone-bad .fill { background: var(--fail, #e5484d); }
	@keyframes flow { to { background-position: -200% 0; } }

	/* The sheet rises inside the frame. */
	.sheet { position: absolute; left: 14px; right: 14px; top: var(--sheet-top, 180px); bottom: 14px; z-index: 4; display: flex; flex-direction: column; border-radius: 20px; background: color-mix(in srgb, var(--bg) 84%, transparent); color: var(--text); box-shadow: 0 0 0 1px rgb(255 255 255 / 0.08), 0 30px 60px -20px rgb(0 0 0 / 0.7); -webkit-backdrop-filter: blur(30px) saturate(1.5); backdrop-filter: blur(30px) saturate(1.5); }
	.sh-h { display: flex; align-items: center; justify-content: space-between; padding: 10px 10px 6px 16px; }
	.sh-h h3 { font: var(--t-headline); }
	/* The picker, slimmer inside the sheet: one hairline box, shorter rows, the sheet does the scrolling. */
	.sheet :global(.parts) { background: none; box-shadow: inset 0 0 0 1px var(--line); border-radius: 12px; }
	.sheet :global(.head) { min-height: 38px; font: var(--t-footnote); font-weight: 600; color: var(--text-muted); }
	.sheet :global(.line) { min-height: 36px; gap: 10px; }
	.sheet :global(.cover) { width: 26px; height: 26px; }
	.sheet :global(.list > ul) { max-height: none; }
	.sheet :global(.season) { min-height: 36px; }
	.sheet:focus { outline: none; }
	.x { display: grid; place-items: center; width: 36px; height: 36px; border: 0; border-radius: 50%; background: var(--surface-2); cursor: pointer; }
	.x:hover { background: var(--surface-3); }
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

	/* Big status: the banner keeps the top, the status sits under it. */
	.frame.status:not(.choosing) .swap { bottom: auto; height: calc(100% - var(--big-h)); }
	.frame.status { --big-h: 132px; }
	.frame.status :global(.ui-billboard .shade) { opacity: 1; }
	.frame.status .sheet { bottom: calc(var(--big-h) + 14px); }
	.big { position: absolute; left: 0; right: 0; bottom: 0; height: var(--big-h); display: flex; align-items: center; gap: 20px; padding: 0 28px; background: #0b0b0c; color: #fff; }
	.says { display: flex; flex-direction: column; gap: 4px; min-width: 0; flex: 1; }
	.huge { font: 800 34px/1.1 var(--font-ui); letter-spacing: -0.02em; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.huge.tone-wait { color: var(--wait-text, var(--wait)); }
	.huge.tone-go { color: var(--info, #3b82f6); }
	.huge.tone-ok { color: var(--ok-text, var(--ok)); }
	.huge.tone-bad { color: var(--fail-text, var(--fail, #e5484d)); }
	.huge.tone-soon, .huge.tone-off { color: rgb(255 255 255 / 0.7); }
	.sent { color: rgb(255 255 255 / 0.8); font: var(--t-callout); }
	.more { color: rgb(255 255 255 / 0.5); }
	/* The lifecycle line runs the full width along the seam between art and status. */
	.life.inline { position: absolute; left: 0; right: 0; top: 0; height: 3px; background: rgb(255 255 255 / 0.1); }
	.big .acts { flex: none; }
	.peek { flex: none; }
	@media (max-width: 767px) {
		.frame.status { --big-h: 112px; }
		.huge { font-size: 26px; }
		.big { padding: 0 20px; gap: 12px; }
	}

	@media (max-width: 767px) {
		.facts { flex-wrap: wrap; white-space: normal; row-gap: 2px; }
		.act { height: 46px; padding: 0 18px; }
	}
	/* Touch screens: every small control gets a full finger-sized target. */
	@media (pointer: coarse) {
		.rule { min-height: 44px; }
		.x { width: 44px; height: 44px; }
	}
	@media (prefers-reduced-motion: reduce) {
		.frame :global(.ui-billboard .content), .fill, .acts, .act { transition: none; }
		.tone-go .fill, .artist-bg { animation: none; }
	}
</style>
