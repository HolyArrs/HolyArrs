<script lang="ts">
	// Approvals on a desktop (design-notes/approvals-stage.md): the picked ask
	// as a big banner (Explore's Billboard: moving backdrop, a muted trailer by
	// itself) with the status tabs built in, and the inbox list on the right.
	// "Choose what to get" lifts the title into the corner and raises a sheet
	// inside the same frame: the seasons and episodes (or albums and songs) to
	// keep, the Pick rule, then Approve. Every ask carries its life-cycle line.
	import { tick } from 'svelte';
	import { fade, fly } from 'svelte/transition';
	import { flip } from 'svelte/animate';
	import { cubicOut } from 'svelte/easing';
	import Billboard from '../ui/Billboard.svelte';
	import TrailerPlayer from '../ui/TrailerPlayer.svelte';
	import { artistVideo, autoplayHere, loadUISettings } from '../autoplay.svelte.ts';
	import { heroVideoStart, pickedSources, type HeroVideo } from '../music/hero-video.ts';
	import Poster from '../Poster.svelte';
	import Icon, { type IconName } from '../Icon.svelte';
	import Avatar from '../ui/Avatar.svelte';
	import KidMark from './KidMark.svelte';
	import AskParts from './AskParts.svelte';
	import { ago, dateTime } from '../format.ts';
	import { to } from '../paths.ts';
	import { kindWord, type ApprovalsCtx } from './approvals.ts';
	import { artistIdOf, artistPhoto, askArtist } from './details.svelte.ts';
	import { askSummary, inboxTabs } from './inbox.ts';
	import { lifecycle } from './lifecycle.ts';
	import { askedFor } from './asked-for.ts';
	import type { PartsPick } from './ask-parts.ts';
	import type { Ask } from '../types.ts';

	let { ctx, single = null, embedded = false }: { ctx: ApprovalsCtx; /** One ask only, filling its box (the cards' window): no list, no tabs. */ single?: Ask | null; /** Under the desktop toolbar, which already has the tabs and the type filter. */ embedded?: boolean } = $props();

	const waiting = (a: Ask) => ctx.approver && a.state === 'asked' && !ctx.isPending(a.id);
	const list = $derived(single ? [ctx.viewed.find((a) => a.id === single.id) ?? single] : [...ctx.shown].sort((x, y) => Date.parse(y.asked_at) - Date.parse(x.asked_at)));

	// The picked ask: the newest one waiting unless one was clicked; it stays put while the list changes.
	let pickedId = $state(0);
	const picked = $derived(list.find((a) => a.id === pickedId) ?? list.find(waiting) ?? list[0] ?? null);
	$effect(() => {
		for (const a of list.slice(0, 12)) ctx.want(a);
	});

	const d = $derived(picked ? ctx.details(picked) : null);
	const music = $derived(picked?.kind === 'artist');
	const artistBg = $derived(picked && music ? (askArtist(picked)?.backgrounds[0] ?? artistPhoto(picked) ?? '') : '');
	const pictures = $derived(!picked || music ? [] : d?.backdrop ? [to(d.backdrop)] : picked.poster ? [to(picked.poster)] : []);
	// Music: the artist's music video plays muted over the photo, as on the artist page (same Autoplay setting).
	let mvideo = $state<HeroVideo | null>(null);
	$effect(() => {
		const a = picked;
		mvideo = null;
		if (!a || a.kind !== 'artist') return;
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
	const year = $derived(d?.year ?? picked?.year ?? null);
	/** What Approve would get, only where it narrows the choice: seasons for a show, albums for music. */
	const count = (a: Ask) => askedFor(a, { seasons: ctx.info(a)?.seasons.length });
	const kindOf = (a: Ask) => (a.kind === 'show' ? 'TV show' : a.kind === 'artist' ? 'Music' : 'Movie');

	// What an empty tab says.
	const calm: Record<string, { icon: IconName; title: string; text: string }> = {
		waiting: { icon: 'check', title: 'You’re all caught up', text: 'New asks land here the moment someone asks.' },
		approved: { icon: 'download', title: 'Nothing on its way', text: 'Approved asks wait here while HolyArrs gets them.' },
		declined: { icon: 'declined', title: 'Nothing turned down', text: 'Asks you say no to stay here, with your reason.' },
		done: { icon: 'film', title: 'Nothing finished yet', text: 'Asks move here once they’re in the Library.' },
		failed: { icon: 'alert', title: 'Nothing needs a look', text: 'Asks that HolyArrs couldn’t finish show up here.' }
	};

	// Status tabs, built into the banner.
	const tabs = $derived(inboxTabs.filter((t) => ctx.approver || t.tab !== 'failed'));
	const types: { k: 'all' | 'movie' | 'show' | 'artist'; label: string; icon: IconName }[] = [
		{ k: 'all', label: 'Everything', icon: 'grid' },
		{ k: 'movie', label: 'Movies', icon: 'film' },
		{ k: 'show', label: 'Shows', icon: 'tv' },
		{ k: 'artist', label: 'Music', icon: 'music' }
	];

	// The underline under the picked tab slides to the next one.
	let tabsEl = $state<HTMLElement>();
	let ink = $state({ x: 0, w: 0 });
	$effect(() => {
		void ctx.tab;
		void ctx.counts;
		void picked?.id;
		void tick().then(() => {
			const on = tabsEl?.querySelector<HTMLElement>('[aria-selected="true"]');
			if (on) ink = { x: on.offsetLeft + 12, w: on.offsetWidth - 24 };
		});
	});

	// Choose what to get: the title lifts into the corner, the sheet rises inside the frame.
	let choosing = $state(false);
	let frame = $state<HTMLElement>();
	let lift = $state(0);
	// Where the sheet starts: just under the lifted title block.
	let sheetTop = $state(180);
	let pick = $state<PartsPick>({ seasons: null, albums: null, empty: false });
	let rulesId = $state(0);
	const rules = $derived(picked ? ctx.rulesFor(picked) : []);
	$effect(() => {
		void picked?.id;
		choosing = false;
		rulesId = 0;
	});
	$effect(() => {
		if (choosing && picked && !waiting(picked)) choosing = false;
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
	const changed = $derived(!!(pick.seasons || pick.episodes || pick.albums || pick.songs));
	function approveChosen() {
		if (!picked || pick.empty) return;
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
			return;
		}
		// ↑ and ↓ move through the list (not while typing or choosing).
		if ((e.key !== 'ArrowDown' && e.key !== 'ArrowUp') || choosing || e.metaKey || e.ctrlKey || e.altKey) return;
		if (e.target instanceof Element && e.target.closest('input, textarea, select, [contenteditable]')) return;
		const i = list.findIndex((a) => a.id === picked?.id);
		const next = list[Math.min(Math.max(i + (e.key === 'ArrowDown' ? 1 : -1), 0), list.length - 1)];
		if (!next || next.id === picked?.id) return;
		e.preventDefault();
		pickedId = next.id;
		void tick().then(() => document.querySelector<HTMLElement>(`[data-row="${next.id}"]`)?.scrollIntoView({ block: 'nearest', behavior: 'smooth' }));
	}
</script>

<svelte:window onkeydown={keys} />

{#snippet tabsTop()}{@render tabBar()}{/snippet}

{#snippet tabBar()}
<div class="tabbar">
							<div class="tabs" role="tablist" aria-label="Show asks that are" bind:this={tabsEl}>
								{#each tabs as t (t.tab)}
									<button type="button" role="tab" class="tab" class:wait={t.tab === 'waiting'} aria-selected={ctx.tab === t.tab} data-tab={t.tab} onclick={() => ctx.setTab(t.tab)}>
										{t.label}{#if ctx.counts[t.tab]}<span class="n">{ctx.counts[t.tab]}</span>{/if}
									</button>
								{/each}
								<span class="ink" aria-hidden="true" style:transform="translateX({ink.x}px)" style:width="{ink.w}px"></span>
							</div>
						</div>
{/snippet}

{#snippet line(a: Ask, cls = '')}
	{@const life = lifecycle(a)}
	<span class="life {cls} tone-{life.tone}" role="progressbar" aria-label="{a.name}: {life.label}" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(life.pct)}><span class="fill" style:width="{life.pct}%"></span></span>
{/snippet}

{#snippet artistArt()}
	{#if artistBg}<div class="artist-bg" style:background-image={`url("${artistBg}")`}></div>{/if}
	{#if mvideo && picked}<div class="mv">{#key mvideo.key}<TrailerPlayer sources={pickedSources(mvideo.key, mvideo)} title={picked.name} sharp pausable top repeat start={heroVideoStart(mvideo)} />{/key}</div>{/if}
{/snippet}

<div class="desk" class:embedded class:single data-testid={single ? 'ask-window-stage' : 'approvals-desk'}>
	<div class="frame" class:choosing bind:this={frame} style:--lift="{lift}px" style:--sheet-top="{sheetTop}px">
		{#if picked}
			{#key picked.id}
			<div class="swap" in:fade={{ duration: 520 }} out:fade={{ duration: 360 }}>
				<Billboard
					label="{picked.name}, asked by {ctx.who(picked).name}"
					testid="ask-banner"
					kind={picked.kind === 'movie' || picked.kind === 'show' ? picked.kind : undefined}
					tmdb={picked.kind === 'artist' ? null : picked.tmdb_id}
					title={picked.name}
					{pictures}
					position="60% 25%"
					art={music && (artistBg || mvideo) ? artistArt : undefined}
					top={single || embedded ? undefined : tabsTop}
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
						{#if waiting(picked)}
							<div class="acts" class:gone={choosing}>
								<button type="button" class="act primary" data-testid="ask-approve" onclick={() => ctx.approve([picked])}><Icon name="check" size={20} />Approve</button>
								<button type="button" class="act glass" onclick={() => void choose()}><Icon name="tools" size={20} />Choose what to get</button>
								<button type="button" class="act glass round" aria-label="Turn down {picked.name}" title="Turn down" data-testid="ask-decline" onclick={() => ctx.turnDown([picked])}><Icon name="x" size={20} /></button>
							</div>
						{:else if ctx.isPending(picked.id)}
							<p class="state">Sending your answer…</p>
						{:else}
							<p class="state">{ctx.decided(picked) || lifecycle(picked).label}{#if lifecycle(picked).stage === 'getting'}<span class="sep">·</span>getting it now{/if}</p>
						{/if}
					{/snippet}
				</Billboard>
			</div>
			{/key}
			{@render line(picked, 'edge')}

			{#if choosing}
				<section class="sheet" tabindex="-1" aria-labelledby="sheet-h" transition:fly={{ y: 40, duration: 420, easing: cubicOut, opacity: 0 }}>
					<header class="sh-h">
						<h3 id="sheet-h">What to get</h3>
						<button type="button" class="x" aria-label="Back to the banner" onclick={() => (choosing = false)}><Icon name="x" size={18} /></button>
					</header>
					<div class="sh-body">
						{#if picked.kind === 'movie'}
							<div class="one"><span class="one-pic"><Poster src={picked.poster} name={picked.name} /></span><div><strong>{picked.name}</strong><span>The movie{year ? ` · ${year}` : ''}{d?.runtime ? ` · ${Math.floor(d.runtime / 60) ? `${Math.floor(d.runtime / 60)}h ` : ''}${d.runtime % 60}m` : ''}</span></div><Icon name="check" size={18} /></div>
						{:else}
							<AskParts {ctx} a={picked} editable unfolded bind:pick />
						{/if}
						{#if rules.length}
							<div class="rules" role="radiogroup" aria-label="Pick rule">
								<span class="rl">Pick rule</span>
								<button type="button" role="radio" class="rule" aria-checked={rulesId === 0} onclick={() => (rulesId = 0)}>{#if ctx.defaultRule(picked)}{ctx.defaultRule(picked)}<span class="dim">· default</span>{:else}Default rule{/if}</button>
								{#each rules as r (r.id)}
									<button type="button" role="radio" class="rule" aria-checked={rulesId === r.id} onclick={() => (rulesId = r.id)}>{r.name}</button>
								{/each}
							</div>
						{/if}
					</div>
					<footer class="sh-f">
						<span class="sum">{pick.empty ? 'Nothing ticked yet' : changed ? 'Your picks, not all of what was asked' : 'Everything that was asked'}</span>
						<button type="button" class="act ghost" onclick={() => (choosing = false)}>Back</button>
						<button type="button" class="act primary" disabled={pick.empty} onclick={approveChosen}><Icon name="check" size={18} />Approve</button>
					</footer>
				</section>
			{/if}
		{:else}
			<div class="calm">
				<div class="calm-top">{@render tabBar()}</div>
				<div class="calm-in" in:fade={{ duration: 400 }}>
					<span class="calm-ic"><Icon name={calm[ctx.tab].icon} size={26} /></span>
					<h2>{calm[ctx.tab].title}</h2>
					<p>{calm[ctx.tab].text}</p>
				</div>
			</div>
		{/if}
	</div>

	{#if !single}
	<aside class="inbox" aria-label="Asks">
		{#if !embedded}
		<div class="types" role="group" aria-label="Type">
			{#each types as t (t.k)}
				<button type="button" class="ty" aria-pressed={ctx.kind === t.k} title={t.label} onclick={() => ctx.setKind(t.k)}><Icon name={t.icon} size={15} /><span>{t.label}</span></button>
			{/each}
		</div>
		{/if}
		<ul class="rows">
			{#each list as a (a.id)}
				{@const w = ctx.who(a)}
				<li animate:flip={{ duration: 340, easing: cubicOut }} in:fly={{ y: 8, duration: 300, easing: cubicOut }} out:fade={{ duration: 220 }}>
					<button type="button" class="row" data-row={a.id} class:on={picked?.id === a.id} class:unread={waiting(a)} aria-current={picked?.id === a.id ? 'true' : undefined} aria-controls="stage-title" onclick={() => (pickedId = a.id)}>
						<span class="pic">{#if a.kind === 'artist' && artistPhoto(a)}<img src={artistPhoto(a)} alt="" loading="lazy" decoding="async" />{:else}<Poster src={a.kind === 'artist' ? null : a.poster} name={a.name} />{/if}</span>
						<span class="txt">
							<strong>{a.name}</strong>
							<span class="who"><Avatar name={w.name} src={w.avatar} size={16} />{w.name}{#if w.kid}<KidMark label="Kid" size="sm" />{/if}<span class="dot">·</span><time datetime={a.asked_at} title={dateTime(a.asked_at)}>{ago(a.asked_at)}</time></span>
						</span>
						{@render line(a, 'row-line')}
					</button>
				</li>
			{:else}
				<li class="none">Nothing here right now.</li>
			{/each}
		</ul>
	</aside>
	{/if}
</div>

<style>
	/* The desk fills the screen under the page title without scrolling the page. */
	.desk.embedded { --h: clamp(480px, calc(100dvh - var(--topbar, 56px) - 220px), 1120px); }
	.desk.single { --h: 100%; grid-template-columns: minmax(0, 1fr); height: 100%; }
	.desk.single .frame { border-radius: 0; box-shadow: none; }
	.desk.single .frame :global(.ui-billboard) { border-radius: 0; }
	.desk { --h: clamp(520px, calc(100dvh - var(--topbar, 56px) - 156px), 1180px); display: grid; grid-template-columns: minmax(0, 1fr) 340px; gap: 18px; align-items: start; }
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
	.frame :global(.ui-billboard .content) { transform-origin: left top; transition: transform 560ms cubic-bezier(0.22, 1, 0.36, 1), opacity 300ms var(--ease-out); }
	.frame.choosing :global(.ui-billboard .content) { transform: translateY(calc(-1 * var(--lift))) scale(0.72); }
	.frame.choosing :global(.ui-billboard .shade) { opacity: 1; background: linear-gradient(to bottom, rgb(0 0 0 / 0.65), rgb(0 0 0 / 0.35) 40%, rgb(0 0 0 / 0.75)); }
	.facts { display: flex; flex-wrap: nowrap; align-items: center; gap: 0; white-space: nowrap; }
	.facts span + span::before { content: '·'; margin: 0 10px; opacity: 0.6; }
	.state { color: rgb(255 255 255 / 0.85); font: var(--t-callout); }
	.asker { flex-basis: 100%; display: inline-flex; align-items: center; gap: 8px; margin-top: 2px; color: rgb(255 255 255 / 0.82); font: 500 14px/1.3 var(--font-ui) !important; }
	.asker b { color: #fff; font-weight: 700; }
	.said { flex-basis: 100%; max-width: 520px; padding-left: 12px; border-left: 2px solid rgb(255 255 255 / 0.45); color: rgb(255 255 255 / 0.9); font: italic 400 15px/1.45 var(--font-ui) !important; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
	.sep { margin: 0 8px; opacity: 0.6; }

	/* Tabs as a glass control at the top of the banner. */
	.tabbar { display: flex; justify-content: flex-start; }
	.tabs { position: relative; display: inline-flex; align-items: center; height: auto; gap: 4px; }
	.tab { position: relative; display: inline-flex; align-items: baseline; gap: 6px; min-height: 40px; padding: 0 12px; border: 0; border-radius: 10px; background: none; color: rgb(255 255 255 / 0.62); font: 600 15px/1 var(--font-ui); letter-spacing: -0.01em; text-shadow: 0 1px 12px rgb(0 0 0 / 0.55) !important; cursor: pointer; transition: color 260ms var(--ease-out); }
	.tab:hover { color: rgb(255 255 255 / 0.9); }
	.tab[aria-selected='true'] { color: #fff; }
	.n { color: rgb(255 255 255 / 0.5); font: 600 12px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.tab.wait .n { color: var(--wait); }
	.ink { position: absolute; left: 0; bottom: 2px; height: 2px; border-radius: 2px; background: #fff; box-shadow: 0 0 10px rgb(255 255 255 / 0.5); transition: transform 420ms cubic-bezier(0.22, 1, 0.36, 1), width 420ms cubic-bezier(0.22, 1, 0.36, 1); }

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

	/* The life-cycle line: along the banner's bottom edge and under each row. */
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
	.sheet { position: absolute; left: 14px; right: 14px; top: var(--sheet-top, 180px); z-index: 4; display: flex; flex-direction: column; max-height: calc(100% - var(--sheet-top, 180px) - 14px); border-radius: 20px; background: color-mix(in srgb, var(--bg) 84%, transparent); color: var(--text); box-shadow: 0 0 0 1px rgb(255 255 255 / 0.08), 0 30px 60px -20px rgb(0 0 0 / 0.7); -webkit-backdrop-filter: blur(30px) saturate(1.5); backdrop-filter: blur(30px) saturate(1.5); }
	.sh-h { display: flex; align-items: center; justify-content: space-between; padding: 14px 14px 10px 20px; }
	.sh-h h3 { font: var(--t-title3); }
	.sheet:focus { outline: none; }
	.x { display: grid; place-items: center; width: 34px; height: 34px; border: 0; border-radius: 50%; background: var(--surface-2); cursor: pointer; }
	.x:hover { background: var(--surface-3); }
	.sh-body { flex: 1; min-height: 0; overflow-y: auto; overscroll-behavior: contain; padding: 0 20px 12px; display: flex; flex-direction: column; gap: 16px; }
	.one { display: flex; align-items: center; gap: 14px; padding: 10px; border-radius: 14px; background: var(--surface-2); }
	.one-pic { width: 44px; border-radius: 6px; overflow: hidden; --poster-ratio: 2 / 3; }
	.one div { flex: 1; display: flex; flex-direction: column; font: var(--t-callout); }
	.one div span { color: var(--text-muted); font: var(--t-footnote); }
	.rules { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
	.rl { margin-right: 6px; color: var(--text-muted); font: var(--t-footnote); font-weight: 600; }
	.rule { display: inline-flex; align-items: center; gap: 6px; min-height: 32px; padding: 0 14px; border: 0; border-radius: var(--r-pill); background: var(--surface-2); font: var(--t-subhead); font-weight: 600; cursor: pointer; transition: background 200ms var(--ease-out), color 200ms var(--ease-out); }
	.rule:hover { background: var(--surface-3); }
	.rule[aria-checked='true'] { background: var(--text); color: var(--bg); }
	.dim { opacity: 0.6; font-weight: 400; }
	.sh-f { display: flex; align-items: center; gap: 8px; padding: 12px 14px 14px 20px; border-top: 1px solid var(--line); }
	.sum { flex: 1; color: var(--text-muted); font: var(--t-footnote); }
	.sh-f .act.primary { background: var(--text); color: var(--bg); height: 44px; }

	.swap { position: absolute; inset: 0; }
	.calm { position: relative; height: 100%; background: radial-gradient(120% 90% at 80% 0%, #24262d 0%, transparent 60%), linear-gradient(160deg, #18191e, #0b0b0c 75%); color: rgb(255 255 255 / 0.66); }
	.calm-top { position: absolute; top: clamp(14px, 2vw, 24px); left: clamp(20px, 2vw, 40px); }
	.calm-in { display: grid; place-content: center; justify-items: center; gap: 8px; height: 100%; text-align: center; padding: 24px; }
	.calm-ic { display: grid; place-items: center; width: 64px; height: 64px; margin-bottom: 6px; border-radius: 50%; background: rgb(255 255 255 / 0.08); color: #fff; box-shadow: inset 0 0 0 1px rgb(255 255 255 / 0.1); }
	.calm h2 { color: #fff; font: var(--t-title2); }
	.calm p { max-width: 340px; font: var(--t-callout); }

	/* The inbox on the right. */
	.inbox { display: flex; flex-direction: column; gap: 10px; height: var(--h); min-width: 0; }
	.types { display: grid; grid-template-columns: repeat(4, 1fr); gap: 2px; padding: 3px; border-radius: 12px; background: var(--surface-2); }
	.ty { display: inline-flex; align-items: center; justify-content: center; gap: 5px; min-height: 30px; border: 0; border-radius: 9px; background: none; color: var(--text-muted); font: var(--t-caption); font-weight: 600; cursor: pointer; transition: background 200ms var(--ease-out), color 200ms var(--ease-out); }
	.ty[aria-pressed='true'] { background: var(--surface-1); color: var(--text); box-shadow: 0 1px 3px rgb(0 0 0 / 0.18); }
	.rows { flex: 1; min-height: 0; overflow-y: auto; overscroll-behavior: contain; margin: 0; padding: 6px; display: flex; flex-direction: column; gap: 2px; list-style: none; border-radius: 20px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); scrollbar-width: thin; }
	.row { position: relative; display: flex; align-items: center; gap: 12px; width: 100%; padding: 10px 10px 13px; border: 0; border-radius: 14px; background: none; text-align: left; cursor: pointer; overflow: hidden; transition: background 200ms var(--ease-out), transform 160ms var(--ease-out); }
	.row:hover { background: var(--surface-2); }
	.row:active { transform: scale(0.99); }
	.row.on { background: var(--surface-3); box-shadow: inset 0 0 0 1px var(--line-strong, var(--line)); }
	.row.on::before { content: ''; position: absolute; left: 0; top: 14px; bottom: 16px; width: 3px; border-radius: 0 3px 3px 0; background: var(--text); }
	.pic { flex: none; width: 42px; aspect-ratio: 2 / 3; --poster-ratio: 2 / 3; border-radius: 6px; overflow: hidden; background: var(--surface-3); box-shadow: 0 6px 14px -6px rgb(0 0 0 / 0.5); transition: transform 300ms var(--ease-out); }
	.pic img { width: 100%; height: 100%; object-fit: cover; object-position: center 25%; display: block; }
	.row:hover .pic, .row.on .pic { transform: scale(1.05); }
	.txt { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
	.txt strong { font: var(--t-subhead); font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.unread .txt strong { font-weight: 700; }
	.who { display: flex; align-items: center; gap: 5px; color: var(--text-muted); font: var(--t-caption); white-space: nowrap; overflow: hidden; }
	.dot { opacity: 0.6; }
	.row-line { left: 12px; right: 12px; bottom: 4px; height: 2px; border-radius: 2px; }
	.none { padding: 30px 12px; color: var(--text-muted); font: var(--t-callout); text-align: center; }

	@media (max-width: 767px) {
		.facts { flex-wrap: wrap; white-space: normal; row-gap: 2px; }
		.act { height: 46px; padding: 0 18px; }
	}
	@media (prefers-reduced-motion: reduce) {
		.frame :global(.ui-billboard .content), .fill, .acts, .row, .pic, .act, .ink { transition: none; }
		.tone-go .fill, .artist-bg { animation: none; }
	}
</style>
