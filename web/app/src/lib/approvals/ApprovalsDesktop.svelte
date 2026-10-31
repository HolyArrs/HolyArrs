<script lang="ts">
	// Approvals for approvers. One toolbar the Downloads way: the status tabs as a
	// pill group on the left, the type filter in a capsule on the right. Each card is
	// a small window: a dark card with the title's wide art, the title and its facts
	// on the art, then a frosted sheet over the art's foot with who asked and the
	// answers (Approve, Decline). A card opens the full window with that ask
	// (trailer banner, What to get).
	import { onDestroy, tick } from 'svelte';
	import { flip } from 'svelte/animate';
	import { fade, fly, slide } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import Icon, { type IconName } from '../Icon.svelte';
	import Avatar from '../ui/Avatar.svelte';
	import KidMark from './KidMark.svelte';
	import ApprovalsStage from './ApprovalsStage.svelte';
	import { ago, dateTime } from '../format.ts';
	import type { ApprovalsCtx } from './approvals.ts';
	import { artistPhoto, askArtist } from './details.svelte.ts';
	import { to } from '../paths.ts';
	import { shortDate } from './inbox.ts';
	import { lifecycle, upcoming, type Life } from './lifecycle.ts';
	import { approverTabs, askTab, approverTab as tabOfAsk, type ApproverTab } from '../wishlist.ts';
	import { askedFor } from './asked-for.ts';
	import type { Ask } from '../types.ts';

	let { ctx }: { ctx: ApprovalsCtx } = $props();

	// Reduced motion: Svelte transitions and the cards' reflow run in no time.
	const reduced = () => typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
	const ms = (d: number) => (reduced() ? 0 : d);
	const waiting = (a: Ask) => ctx.approver && a.state === 'asked' && !ctx.isPending(a.id);
	// A failed ask can still be declined, so the person who asked hears back.
	const failed = (a: Ask) => ctx.approver && askTab(a) === 'failed' && !ctx.isPending(a.id);
	// The page sorts each tab (Processing: failed first; Done: newest decision first).
	const list = $derived(ctx.shown);
	$effect(() => {
		for (const a of list.slice(0, 16)) ctx.want(a);
	});

	const types: { k: 'all' | 'movie' | 'show' | 'artist'; label: string; icon: IconName }[] = [
		{ k: 'all', label: 'All', icon: 'library' },
		{ k: 'movie', label: 'Movies', icon: 'film' },
		{ k: 'show', label: 'TV shows', icon: 'tv' },
		{ k: 'artist', label: 'Music', icon: 'music' }
	];

	// The raised plate slides to the picked item in each group (as on Downloads).
	function plate(node: HTMLElement, _key: unknown) {
		const place = () => {
			const on = node.querySelector<HTMLElement>(':scope > [aria-pressed="true"]');
			if (!on) return node.style.setProperty('--io', '0');
			node.style.setProperty('--ix', `${on.offsetLeft}px`);
			node.style.setProperty('--iw', `${on.offsetWidth}px`);
			node.style.setProperty('--ih', `${on.offsetHeight}px`);
			node.style.setProperty('--iy', `${on.offsetTop}px`);
			node.style.setProperty('--io', '1');
		};
		void tick().then(place);
		const ro = new ResizeObserver(place);
		ro.observe(node);
		return { update: () => void tick().then(place), destroy: () => ro.disconnect() };
	}

	// What an empty tab says.
	const calm: Record<string, { title: string; text: string }> = {
		pending: { title: 'You’re all caught up.', text: 'New asks show up here.' },
		processing: { title: 'Nothing in progress.', text: 'Approved asks stay here until they’re ready.' },
		done: { title: 'Nothing done yet.', text: 'Ready and declined asks end up here.' }
	};

	// The status line, with what the page knows: not out yet (from the release and air dates).
	// Partly in the Library ("1 of 2 ready") needs which asked seasons and albums are in the
	// Library, which the asks list doesn't carry yet, so it isn't said (Processing instead).
	const firstAir = (a: Ask) => (season: number) => {
		const eps = ctx.episodes(a.tmdb_id, season);
		if (!eps) return null;
		return eps.map((e) => e.air_day ?? '').filter(Boolean).sort()[0] ?? null;
	};
	const lifeNow = (a: Ask): Life => {
		if (a.state !== 'approved' || !a.title_id) return lifecycle(a);
		const live = ctx.live(a);
		if (live.phase || a.kind === 'artist') return lifecycle(a, live);
		const out = upcoming(a, ctx.info(a), firstAir(a));
		return lifecycle(a, out ? { upcoming: shortDate(out) } : {});
	};
	// Worked out once per ask; the cards, the status filter and its counts all read it.
	const lives = $derived(new Map(ctx.viewed.map((a) => [a.id, lifeNow(a)])));
	const lifeOf = (a: Ask): Life => lives.get(a.id) ?? lifeNow(a);

	// Filter by status within the tab: the status word without its detail (Downloading · 40% → Downloading).
	const statusOf = (a: Ask) => lifeOf(a).label.split(' · ')[0];
	// The picked status belongs to one tab: it clears when the tab changes by any path,
	// and when it no longer exists in that tab.
	let statusPick = $state<{ tab: ApproverTab | ''; word: string }>({ tab: '', word: '' });
	/** The statuses in one tab, in the page's order (Processing: failed first). */
	const FLOW = ['Pending', 'Upcoming', 'Searching', 'Downloading', 'Importing', 'Processing', 'Ready'];
	const statusesIn = (t: ApproverTab) => {
		const n = new Map<string, { count: number; tone: Life['tone'] }>();
		for (const a of ctx.viewed) {
			if (tabOfAsk(a) !== t) continue;
			const w = statusOf(a);
			const x = n.get(w);
			n.set(w, { count: (x?.count ?? 0) + 1, tone: x?.tone ?? lifeOf(a).tone });
		}
		// Failed first (it needs you), then the order an ask moves through, Declined last.
		const rank = (x: { word: string; tone: Life['tone'] }) => (x.tone === 'bad' ? 0 : x.tone === 'off' ? 100 : FLOW.indexOf(x.word) + 1 || 50);
		return [...n].map(([word, x]) => ({ word, ...x })).sort((x, y) => rank(x) - rank(y));
	};
	const status = $derived(statusPick.word && statusPick.tab === ctx.approverTab && statusesIn(ctx.approverTab).some((x) => x.word === statusPick.word) ? statusPick.word : '');
	$effect(() => {
		if (statusPick.word && !status) statusPick = { tab: '', word: '' };
	});
	// A tab's statuses slide out from behind it only when the tab is clicked (never on hover);
	// leaving the bar folds them shortly after, and coming back does not reopen them.
	let openTab = $state<ApproverTab | ''>('');
	let foldT: ReturnType<typeof setTimeout> | undefined;
	const subsOf = (t: ApproverTab) => {
		const x = statusesIn(t);
		return x.length > 1 ? x : [];
	};
	function unfold(t: ApproverTab) {
		clearTimeout(foldT);
		openTab = subsOf(t).length ? t : '';
	}
	function foldSoon(ms = 380) {
		clearTimeout(foldT);
		foldT = setTimeout(() => (openTab = ''), ms);
	}
	function pickTab(t: ApproverTab) {
		if (ctx.approverTab !== t) {
			statusPick = { tab: '', word: '' };
			ctx.setApproverTab(t);
		}
		unfold(t);
	}
	function pickStatus(t: ApproverTab, word: string, e?: MouseEvent) {
		if (ctx.approverTab !== t) ctx.setApproverTab(t);
		// Touch has no "leave": fold a moment after the pick.
		if ((e as PointerEvent | undefined)?.pointerType && (e as PointerEvent).pointerType !== 'mouse') foldSoon(600);
		statusPick = status === word && ctx.approverTab === t ? { tab: '', word: '' } : { tab: t, word };
	}
	function capKeys(e: KeyboardEvent) {
		if (e.key === 'Escape' && openTab) {
			e.stopPropagation();
			openTab = '';
		}
	}
	// Touch and keyboard have no "leave": a tap or click outside the bar, or a page scroll, folds it.
	let capEl = $state<HTMLElement>();
	$effect(() => {
		if (!openTab) return;
		const out = (e: Event) => {
			if (capEl && !capEl.contains(e.target as Node)) foldSoon(0);
		};
		document.addEventListener('pointerdown', out, true);
		window.addEventListener('scroll', out, { passive: true, capture: true });
		return () => {
			document.removeEventListener('pointerdown', out, true);
			window.removeEventListener('scroll', out, { capture: true });
		};
	});
	const cardsShown = $derived(status ? list.filter((a) => statusOf(a) === status) : list);

	// The card's banner: the title's wide art (show or movie backdrop, the artist's wide photo).
	const banner = (a: Ask) => {
		if (a.kind === 'artist') return askArtist(a)?.backgrounds[0] ?? '';
		const d = ctx.details(a);
		return d?.backdrop ? to(d.backdrop) : '';
	};

	// What the ask is for, in words that fit a card.
	const kindOf = (a: Ask) => (a.kind === 'show' ? 'TV show' : a.kind === 'artist' ? 'Music' : 'Movie');
	const wants = (a: Ask) => askedFor(a, { seasons: ctx.info(a)?.seasons.length });

	// Approving shows its answer on the card for a beat, then the card leaves (Undo stays in the toast).
	let approving = $state(new Set<number>());
	const beats = new Set<ReturnType<typeof setTimeout>>();
	onDestroy(() => {
		for (const t of beats) clearTimeout(t);
		clearTimeout(foldT);
	});
	function approveOne(a: Ask) {
		if (approving.has(a.id)) return;
		approving = new Set(approving).add(a.id);
		const t = setTimeout(() => {
			beats.delete(t);
			// Answered elsewhere meanwhile (Approve all, the window): only clear the beat.
			if (!waiting(a)) {
				const next = new Set(approving);
				next.delete(a.id);
				approving = next;
				return;
			}
			// Focus goes on to the next card, so the keyboard doesn't fall back to the page top.
			const card = document.querySelector<HTMLElement>(`.bc[data-ask="${a.id}"]`);
			const after = card?.nextElementSibling?.querySelector<HTMLElement>('.open') ?? null;
			const had = !!card?.contains(document.activeElement);
			ctx.approve([a]);
			const next = new Set(approving);
			next.delete(a.id);
			approving = next;
			if (had && after) void tick().then(() => after.isConnected && after.focus({ preventScroll: true }));
		}, ms(560));
		beats.add(t);
	}

	// The window: one ask on the Stage.
	let win = $state<HTMLDialogElement>();
	let openId = $state(0);
	const openAsk = $derived(ctx.viewed.find((a) => a.id === openId) ?? null);
	let from: HTMLElement | null = null;
	async function show(a: Ask, el: HTMLElement) {
		openId = a.id;
		from = el;
		await tick();
		if (!win) return;
		// The window grows out of the card it came from.
		const card = (el.closest('.bc') ?? el).getBoundingClientRect();
		win.style.setProperty('--ox', `${card.left + card.width / 2 - innerWidth / 2}px`);
		win.style.setProperty('--oy', `${card.top + card.height / 2 - innerHeight / 2}px`);
		if (!win.open) win.showModal();
		// Focus the window itself, so no button opens wearing a focus ring.
		win.focus({ preventScroll: true });
	}
	function hide() {
		if (win?.open) win.close();
	}
	// Only an answer to the same ask moves on; stepping to an answered ask must not.
	let wasWaiting = 0;
	let advance: ReturnType<typeof setTimeout> | undefined;
	$effect(() => {
		const id = openId;
		const w = !!openAsk && waiting(openAsk);
		if (wasWaiting === id && id && !w && win?.open) {
			// Answered here: go on to the next one still waiting, or close when none is left.
			clearTimeout(advance);
			advance = setTimeout(() => {
				if (openId !== id || !win?.open) return;
				const next = cardsShown.find((x) => x.id !== id && waiting(x));
				if (next) openId = next.id;
				else hide();
			}, ms(650));
		}
		wasWaiting = w ? id : 0;
	});
	$effect(() => () => clearTimeout(advance));
	// The ask left the list (filter, tab or a refresh): don't leave an empty window open.
	$effect(() => {
		if (openId && !openAsk && win?.open) hide();
	});
	// ← and → (or the arrows) move to the ask before or after, without closing the window.
	const at = $derived(cardsShown.findIndex((a) => a.id === openId));
	// "2 of 9: Name", read out when stepping.
	let said = $state('');
	function step(by: number) {
		const next = cardsShown[at + by];
		if (!next) return;
		openId = next.id;
		// Read back from the window's own position, so the words match what it shows.
		void tick().then(() => (said = `${at + 1} of ${cardsShown.length}: ${next.name}`));
	}
	function winKeys(e: KeyboardEvent) {
		if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
		// Controls inside the sheet keep their arrows; the sheet itself (focused when it opens) steps.
		if (e.target instanceof Element && (e.target.closest('input, textarea, select, video, [contenteditable], [role="group"], button[aria-pressed]') || e.target.closest('.sheet *'))) return;
		if (document.querySelectorAll('dialog[open]').length > 1) return;
		e.preventDefault();
		step(e.key === 'ArrowRight' ? 1 : -1);
	}
	function cancel(e: Event) {
		const back = win?.querySelector<HTMLElement>('.frame.choosing .sh-h .x');
		if (back) {
			e.preventDefault();
			back.click();
		}
	}
</script>

<div class="desktop" data-testid="approvals-desktop">
	<div class="toolbar">
		<div class="tcap" role="presentation" bind:this={capEl} onkeydown={capKeys} onpointerenter={() => openTab && clearTimeout(foldT)} onpointerleave={(e) => e.pointerType === 'mouse' && openTab && foldSoon()}>
		<div class="tabs" role="group" aria-label="Status">
			{#each approverTabs as t (t.tab)}
				{@const n = ctx.approverCounts[t.tab] ?? 0}
				{@const bad = t.tab === 'processing' ? ctx.approverCounts.failed : 0}
				{@const here = ctx.approverTab === t.tab}
				{@const subs = subsOf(t.tab)}
				{@const open = openTab === t.tab && subs.length > 0}
				<div class="pw" class:on={here} class:pw-open={open} role="presentation">
					<button type="button" class="tab" class:zero={!n} aria-pressed={here} aria-expanded={subs.length ? open : undefined} aria-label="{t.label} {n}{bad ? `, ${bad} failed` : ''}{here && status ? `, showing ${status}` : ''}" onclick={() => pickTab(t.tab)}>{t.label}{#if here && status && !open}<span class="pick">· {status}</span>{/if}<span class="n" class:wait={t.tab === 'pending' && n}>{n}</span>{#if bad}<span class="fail-dot" aria-hidden="true"></span>{/if}</button>
					{#if subs.length}
						<div class="subwrap" inert={!open}>
							<div class="subin" role="group" aria-label="{t.label} by status">
								{#each subs as x, xi (x.word)}
									<button type="button" class="stab tn-{x.tone}" style:--i={xi} class:stab-on={here && status === x.word} aria-pressed={here && status === x.word} aria-label="{x.word}, {x.count}" onclick={(e) => pickStatus(t.tab, x.word, e)}><span class="dd tone-{x.tone}" aria-hidden="true"></span>{x.word}<b>{x.count}</b></button>
								{/each}
							</div>
						</div>
					{/if}
				</div>
			{/each}
		</div>
		</div>
		<div class="right">
			{#if ctx.approverTab === 'pending' && cardsShown.filter(waiting).length > 1}
				<button type="button" class="approve-all" in:fade={{ duration: ms(200) }} onclick={() => ctx.approve(cardsShown.filter((x) => waiting(x) && !approving.has(x.id)))}><Icon name="check" size={16} />Approve all {cardsShown.filter(waiting).length}</button>
			{/if}
			<div class="cap">
				<div class="seg" role="group" aria-label="Type" use:plate={ctx.kind}>
					<span class="plate" aria-hidden="true"></span>
					{#each types as t (t.k)}
						<button type="button" class="it" aria-pressed={ctx.kind === t.k} onclick={() => ctx.setKind(t.k)}><span class="ic"><Icon name={t.icon} size={16} /></span><span class="lb">{t.label}</span></button>
					{/each}
				</div>
			</div>
		</div>
	</div>


		<ul class="cards" aria-label="Asks" in:fade={{ duration: ms(260) }}>
			{#each cardsShown as a, i (a.id)}
				{@const w = ctx.who(a)}
				{@const life = lifeOf(a)}
				{@const photo = a.kind === 'artist' ? artistPhoto(a) : null}
				{@const wide = banner(a)}
				<li class="bc" data-ask={a.id} class:music={a.kind === 'artist'} class:answered={!waiting(a)} animate:flip={{ duration: ms(340), easing: cubicOut }} in:fly={{ y: 14, duration: ms(420), delay: ms(Math.min(i, 8) * 45), easing: cubicOut }} out:fade={{ duration: ms(220) }}>
					<!-- The whole card opens the window; the answers sit above this layer. -->
					<button type="button" class="open" aria-label="Open {a.name}, asked by {w.name}" aria-haspopup="dialog" onclick={(e) => void show(a, e.currentTarget)}></button>
					<div class="ban">
						{#if wide}<img class="ban-img" src={wide} alt="" loading="lazy" decoding="async" />{:else}<span class="ban-img soft" aria-hidden="true" style:background-image={photo || a.poster ? `url("${photo ?? a.poster}")` : undefined}></span>{/if}
						<div class="ban-cap"><h3 class="ban-t">{a.name}</h3><span class="ban-f">{[kindOf(a), a.year, wants(a)].filter(Boolean).join(' · ')}</span></div>
					</div>
					<div class="sheet-b">
					<div class="body">
						<span class="who"><Avatar name={w.name} src={w.avatar} size={20} /><b>{w.name}</b>{#if w.kid}<KidMark label="Kid" size="sm" />{/if}<span class="when">· <time datetime={a.asked_at} title={dateTime(a.asked_at)}>{ago(a.asked_at)}</time></span></span>
					</div>
					<div class="foot">
						{#if waiting(a)}
							<button type="button" class="b no" aria-label="Decline {a.name}" data-testid="ask-decline" onclick={() => ctx.turnDown([a])}><Icon name="x" size={15} /><span class="bl">Decline</span></button>
							<button type="button" class="b yes" data-testid="ask-approve" onclick={() => approveOne(a)}><Icon name="check" size={15} />Approve</button>
						{:else if life.stage === 'failed' && !ctx.isPending(a.id)}
							<!-- Every failed ask: the red label on the left, Decline on the right when it can still be declined. -->
							<span class="state tone-bad"><span class="dot"></span>{life.label}</span>
							{#if failed(a)}<button type="button" class="b no" aria-label="Decline {a.name}" data-testid="ask-decline" onclick={() => ctx.turnDown([a])}><Icon name="x" size={15} /><span class="bl">Decline</span></button>{/if}
						{:else}
							<span class="said">
								{#if ctx.isPending(a.id)}
									<span class="state">Sending…</span>
								{:else}
									<!-- The status word only; the window says who declined. -->
									<span class="state tone-{life.tone}"><span class="dot"></span>{life.label}</span>
								{/if}
							</span>
						{/if}
					</div>
					</div>
					<span class="life tone-{approving.has(a.id) ? 'ok' : life.tone}" aria-hidden="true"><span class="fill" style:width="{approving.has(a.id) ? 100 : life.pct}%"></span></span>
					{#if approving.has(a.id)}
						<span class="done" in:fade={{ duration: ms(180) }}><span class="done-ic" in:fly={{ y: 6, duration: ms(320), easing: cubicOut }}><Icon name="check" size={22} /></span>Approved</span>
					{/if}
				</li>
			{:else}
				{#if status}
					<li class="calm">
						<h2>Nothing here with this status.</h2>
						<button type="button" class="b no show-all" onclick={() => (statusPick = { tab: '', word: '' })}>Show all</button>
					</li>
				{:else}
					<li class="calm">
						<span class="calm-ic"><Icon name="check" size={24} /></span>
						<h2>{calm[ctx.approverTab]?.title ?? 'Nothing here right now.'}</h2>
						<p>{ctx.kind !== 'all' ? 'Nothing of this type here. Try All.' : (calm[ctx.approverTab]?.text ?? '')}</p>
					</li>
				{/if}
			{/each}
		</ul>
</div>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
<dialog class="win" tabindex="-1" onkeydown={winKeys} bind:this={win} aria-labelledby="stage-title" oncancel={cancel} onclose={() => { openId = 0; from?.focus({ preventScroll: true }); }} onclick={(e) => e.target === win && hide()}>
	{#if openAsk}
		<button type="button" class="close" aria-label="Close" onclick={hide}><Icon name="x" size={18} /></button>
		{#if cardsShown.length > 1 && at >= 0}
			<div class="steps-nav">
				<button type="button" class="nav" aria-label="Previous ask" disabled={at <= 0} onclick={() => step(-1)}><Icon name="chevronLeft" size={18} /></button>
				<span class="pos">{at + 1} of {cardsShown.length}</span>
				<button type="button" class="nav" aria-label="Next ask" disabled={at >= cardsShown.length - 1} onclick={() => step(1)}><Icon name="chevronRight" size={18} /></button>
			</div>
		{/if}
		<div class="win-in"><ApprovalsStage {ctx} single={openAsk} choose /></div>
		<span class="vh" aria-live="polite">{said}</span>
	{/if}
</dialog>

<style>
	.desktop { --ez: cubic-bezier(0.32, 0.72, 0, 1); display: flex; flex-direction: column; gap: 16px; }
	button { font: inherit; color: inherit; }
	button:focus-visible { outline: none; box-shadow: var(--focus-ring); }

	/* The toolbar: tabs left, filter and view right. */
	.tcap { position: relative; z-index: 6; }
	/* Status bar: slim pills. Bar 40 = 1 border + 3 inset + 32 pill + 3 + 1; pill radius 9 = bar radius 12 - inset 3.
	   Only the picked tab is raised; its statuses reveal out from behind it onto the track (clip + width + x + fade). */
	.pw { position: relative; display: flex; align-items: center; flex: none; height: 32px; }
	.pw .tab { height: 32px; }
	.pw.on .tab { background: var(--seg-thumb); box-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.12); }
	/* Glide, not snap: the width eases in and out over ~0.5s (gentle start, soft landing), the chips drift in one after another; folding eases both ways and is a little quicker. */
	.subwrap { display: grid; grid-template-columns: 0fr; overflow: hidden; transition: grid-template-columns 380ms cubic-bezier(0.65, 0, 0.35, 1); }
	.pw.pw-open .subwrap { grid-template-columns: 1fr; transition: grid-template-columns 500ms cubic-bezier(0.45, 0, 0.2, 1); }
	.subin { display: flex; align-items: center; gap: 2px; min-width: 0; white-space: nowrap; opacity: 0; transform: translateX(-24px); transition: opacity 220ms cubic-bezier(0.65, 0, 0.35, 1), transform 380ms cubic-bezier(0.65, 0, 0.35, 1); }
	.pw.pw-open .subin { opacity: 1; transform: none; transition: opacity 360ms cubic-bezier(0.45, 0, 0.2, 1) 80ms, transform 500ms cubic-bezier(0.45, 0, 0.2, 1); }
	.subin .stab { opacity: 0; transform: translateX(-8px); transition: opacity 180ms ease, transform 300ms cubic-bezier(0.65, 0, 0.35, 1), background-color 160ms var(--ease-out), color 160ms var(--ease-out), box-shadow 160ms var(--ease-out); }
	.pw.pw-open .subin .stab { opacity: 1; transform: none; transition: opacity 320ms cubic-bezier(0.45, 0, 0.2, 1) calc(120ms + var(--i, 0) * 45ms), transform 420ms cubic-bezier(0.45, 0, 0.2, 1) calc(80ms + var(--i, 0) * 45ms), background-color 160ms var(--ease-out), color 160ms var(--ease-out), box-shadow 160ms var(--ease-out); }
	/* Chips: quiet until picked; the picked one takes a tint of its own tone with readable text in that tone. */
	.stab { --tc: var(--info, #409cff); --tt: var(--info-text, #409cff); position: relative; display: inline-flex; align-items: center; flex: none; gap: 6px; height: 32px; padding: 0 11px; border: 0; border-radius: 9px; background: none; color: var(--text-muted); font: 500 13px/1 var(--font-ui); white-space: nowrap; cursor: pointer; transition: background-color 160ms var(--ease-out), color 160ms var(--ease-out), box-shadow 160ms var(--ease-out); }
	.stab.tn-bad { --tc: var(--fail, #ff6961); --tt: var(--fail-text, #ff6961); }
	.stab.tn-ok { --tc: var(--ok, #30d158); --tt: var(--ok-text, #30d158); }
	.stab.tn-wait { --tc: var(--wait, #ffb340); --tt: var(--wait-text, #ffb340); }
	.stab.tn-soon, .stab.tn-off { --tc: var(--text-muted); --tt: var(--text); }
	.stab:first-child { margin-left: 4px; }
	.stab b { font-weight: 600; font-variant-numeric: tabular-nums; opacity: 0.7; }
	.stab:hover { color: var(--text); background: color-mix(in srgb, var(--text) 6%, transparent); }
	.stab:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.stab.stab-on { color: var(--tt); background: color-mix(in srgb, var(--tc) 15%, transparent); box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--tc) 30%, transparent); }
	.stab.stab-on b { opacity: 1; }
	.stab .dd { background: var(--tc); }
	@media (prefers-reduced-motion: reduce) { .subwrap, .pw.pw-open .subwrap { transition: none; } .subin, .pw.pw-open .subin, .subin .stab, .pw.pw-open .subin .stab { transform: none; transition: opacity 120ms linear; } }
	/* Coarse pointers keep a 44px target without fattening the bar. */
	@media (pointer: coarse) { .tab::after, .stab::after, .it::after { content: ''; position: absolute; inset: -6px 0; } }
	.dd { width: 6px; height: 6px; border-radius: 50%; }
	.pick { margin-left: 4px; color: color-mix(in srgb, var(--text) 70%, transparent); font-weight: 500; }
	/* One row that never wraps: opening a tab's statuses must not push anything to a new line. */
	.toolbar { display: flex; flex-wrap: nowrap; align-items: center; justify-content: space-between; gap: 12px; }
	.toolbar .tcap { flex: 0 1 auto; min-width: 0; }
	.toolbar .tabs { max-width: 100%; overflow-x: auto; overflow-y: hidden; scrollbar-width: none; overscroll-behavior-x: contain; }
	.toolbar .tabs::-webkit-scrollbar { display: none; }
	.toolbar .right { flex: none; }
	/* While statuses are open, the type filter steps back to icons to give them room. */
	.seg .lb { display: inline-block; max-width: 90px; overflow: hidden; transition: max-width 500ms cubic-bezier(0.45, 0, 0.2, 1), opacity 300ms cubic-bezier(0.45, 0, 0.2, 1); }
	.toolbar:has(.pw-open) .seg .lb { max-width: 0; opacity: 0; }
	.toolbar:has(.pw-open) .seg .it { gap: 0; }
	.right { display: flex; align-items: center; gap: 10px; margin-left: auto; }
	.plate { position: absolute; z-index: 0; left: 0; top: 0; width: var(--iw, 0); height: var(--ih, 0); background: var(--seg-thumb); box-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.12), 0 2px 8px rgb(0 0 0 / 0.08); opacity: var(--io, 0); transform: translate(var(--ix, 0), var(--iy, 0)); transition: transform 320ms var(--ez), width 320ms var(--ez), opacity 160ms var(--ease-out); pointer-events: none; }
	.tabs { position: relative; display: flex; align-items: center; gap: 2px; height: 40px; box-sizing: border-box; padding: 3px; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); backdrop-filter: blur(18px) saturate(1.4); -webkit-backdrop-filter: blur(18px) saturate(1.4); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); }
	.tab { position: relative; z-index: 1; display: inline-flex; align-items: center; gap: 7px; height: 32px; padding: 0 13px; border: 0; border-radius: 9px; background: none; color: color-mix(in srgb, var(--text) 76%, var(--bg)); font: 500 13.5px/1 var(--font-ui); white-space: nowrap; cursor: pointer; transition: color 200ms var(--ease-out), background-color 200ms var(--ease-out), box-shadow 200ms var(--ease-out); }
	.tab:hover, .tab[aria-pressed='true'] { color: var(--text); }
	.tab:hover:not([aria-pressed='true']) { background: color-mix(in srgb, var(--text) 6%, transparent); }
	.tab.zero:not([aria-pressed='true']):not(:hover) { color: var(--text-muted); }
	.n { opacity: 0.75; font-weight: 600; font-variant-numeric: tabular-nums; }
	.n.wait { color: var(--wait-text, var(--wait)); opacity: 1; }
	.tab .fail-dot { width: 6px; height: 6px; margin-left: -2px; border-radius: 50%; background: var(--fail, #e5484d); }
	.cap { display: flex; align-items: center; height: 40px; box-sizing: border-box; padding: 3px; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); backdrop-filter: blur(18px) saturate(1.4); -webkit-backdrop-filter: blur(18px) saturate(1.4); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); }
	.seg { position: relative; display: flex; align-items: stretch; gap: 2px; height: 32px; }
	.seg .plate { border-radius: 9px; }
	.it { position: relative; z-index: 1; display: inline-flex; align-items: center; justify-content: center; gap: 6px; height: 32px; padding: 0 12px; border: 0; border-radius: 9px; background: none; color: var(--text); cursor: pointer; transition: background 160ms var(--ease-out), transform 180ms var(--ez); }
	.ic { display: grid; place-items: center; width: 16px; height: 16px; }
	.lb { white-space: nowrap; font: 500 13px/1 var(--font-ui); color: var(--text-muted); transition: color 160ms var(--ease-out); }
	.it:hover .lb, .it[aria-pressed='true'] .lb { color: var(--text); }
	.it:hover:not([aria-pressed='true']) { background: color-mix(in srgb, var(--text) 7%, transparent); }
	.it:active { transform: scale(0.97); }
	.approve-all { display: inline-flex; align-items: center; gap: 6px; height: 40px; padding: 0 14px; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); color: var(--text); font: 600 13px/1 var(--font-ui); cursor: pointer; backdrop-filter: blur(18px) saturate(1.4); -webkit-backdrop-filter: blur(18px) saturate(1.4); transition: transform 160ms var(--ease-out), background 200ms var(--ease-out); }
	.approve-all:hover { background: var(--surface-2); }
	.approve-all:active { transform: scale(0.96); }

	/* Window cards: a dark card (in both themes, it is led by the art), the title's wide art
	   with the title and its facts on it, then a frosted sheet over the art's foot holding
	   who asked and the answers. The life-cycle line runs along the bottom edge. */
	.cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(290px, 100%), 1fr)); gap: 16px; margin: 0; padding: 0; list-style: none; }
	/* The card takes the sheet's own colour (dark in dark mode, pale in light), so no band shows under the sheet. */
	.bc { --sheet: color-mix(in srgb, var(--bg) 88%, transparent); position: relative; isolation: isolate; overflow: hidden; display: flex; flex-direction: column; border-radius: 20px; background: var(--bg); box-shadow: 0 0 0 1px var(--line), 0 12px 32px -16px rgb(0 0 0 / 0.6); transition: transform 300ms var(--ez), box-shadow 300ms var(--ease-out); }
	.bc:hover { transform: translateY(-3px); box-shadow: 0 0 0 1px rgb(255 255 255 / 0.1), 0 2px 4px rgb(0 0 0 / 0.08), 0 24px 48px -24px rgb(0 0 0 / 0.65); }
	.open { position: absolute; inset: 0; z-index: 1; padding: 0; border: 0; border-radius: inherit; background: none; cursor: pointer; }
	.open:focus-visible { box-shadow: none; }
	.bc:has(.open:focus-visible) { box-shadow: 0 0 0 2px var(--text), 0 24px 48px -24px rgb(0 0 0 / 0.6); }
	.ban { position: relative; height: 190px; overflow: hidden; background: #1a1a1d; border-radius: 20px 20px 0 0; }
	.ban-img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; object-position: center 28%; background-size: cover; background-position: center 28%; transform: scale(1.02); transition: transform 400ms var(--ez), opacity 500ms var(--ease-out); }
	img.ban-img { animation: ban-in 600ms var(--ease-out) both; }
	@keyframes ban-in { from { opacity: 0; } }
	.ban-img.soft { filter: blur(18px) saturate(1.3); transform: scale(1.3); opacity: 0.8; }
	.bc:hover .ban-img:not(.soft) { transform: scale(1.04); }
	.ban::after { content: ''; position: absolute; inset: 0; background: linear-gradient(to bottom, rgb(0 0 0 / 0.15), transparent 30%, rgb(0 0 0 / 0.72)); }
	.ban-cap { position: absolute; left: 14px; right: 14px; bottom: 30px; z-index: 1; display: flex; flex-direction: column; gap: 3px; color: #fff; text-shadow: 0 1px 12px rgb(0 0 0 / 0.5); }
	.ban-t { margin: 0; font: 800 22px/1.05 var(--font-display, var(--font-ui)); letter-spacing: -0.02em; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
	.ban-f { font: 600 12px/1.3 var(--font-ui); color: rgb(255 255 255 / 0.85); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	/* Clicks pass through the sheet to the card's open layer; only its buttons take their own. */
	.sheet-b, .ban, .ban-cap { pointer-events: none; }
	.sheet-b button { pointer-events: auto; }
	.sheet-b { position: relative; z-index: 1; display: flex; flex-direction: column; flex: 1; margin: -20px 8px 8px; border-radius: 16px; background: var(--sheet); color: var(--text); -webkit-backdrop-filter: blur(24px) saturate(1.4); backdrop-filter: blur(24px) saturate(1.4); box-shadow: 0 0 0 1px rgb(255 255 255 / 0.08), 0 14px 30px -14px rgb(0 0 0 / 0.6); }
	.body { display: flex; flex-direction: column; gap: 4px; padding: 12px 12px 0; min-width: 0; }
	.who { display: flex; align-items: center; gap: 7px; min-width: 0; color: var(--text-muted); font: var(--t-footnote); white-space: nowrap; }
	.who > :global(*) { flex: none; }
	.who b { max-width: 60%; overflow: hidden; text-overflow: ellipsis; color: var(--text); font-weight: 600; }
	.who .when { flex: 0 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; }
	.foot { position: relative; z-index: 2; display: grid; grid-template-columns: 1fr 1fr; gap: 8px; margin-top: auto; padding: 12px; }
	.b { display: inline-flex; align-items: center; justify-content: center; gap: 6px; height: 40px; padding: 0 12px; border: 0; border-radius: 12px; font: var(--t-subhead); font-weight: 600; cursor: pointer; transition: transform 140ms var(--ease-out), background 200ms var(--ease-out);  white-space: nowrap; }
	.b:active { transform: scale(0.96); }
	.b.no { background: color-mix(in srgb, var(--text) 8%, transparent); color: var(--text); }
	.b.no:hover { background: color-mix(in srgb, var(--text) 13%, transparent); }
	.b.yes { background: var(--text); color: var(--bg); }
	.said { grid-column: 1 / -1; display: flex; }
	.state { grid-column: 1 / -1; display: inline-flex; align-items: center; gap: 7px; min-height: 40px; color: var(--text-muted); font: var(--t-footnote); }
	.dot { width: 7px; height: 7px; border-radius: 50%; background: currentColor; }
	.state.tone-wait .dot { background: var(--wait); }
	.state.tone-go .dot { background: var(--info, #3b82f6); }
	.state.tone-ok .dot { background: var(--ok); }
	.state.tone-bad { color: var(--fail-text, var(--fail, #e5484d)); }
	.state.tone-bad .dot { background: var(--fail, #e5484d); }
	.foot > .state.tone-bad { grid-column: 1; }
	.state.tone-soon .dot { background: var(--text-muted); }
	/* Inset from the card's rounded corners, so the line is never clipped. */
	.life { position: absolute; left: 20px; right: 20px; bottom: 3px; z-index: 2; height: 3px; overflow: hidden; border-radius: 3px; background: rgb(127 127 127 / 0.14); }
	.fill { display: block; height: 100%; border-radius: 3px; transition: width 900ms cubic-bezier(0.22, 1, 0.36, 1); }
	.tone-wait .fill { background: var(--wait); }
	.tone-go .fill { background: var(--info, #3b82f6); }
	.tone-ok .fill { background: var(--ok); }
	.tone-soon .fill { background: var(--text-muted); }
	.tone-off .fill { background: rgb(127 127 127 / 0.45); }
	.tone-bad .fill { background: var(--fail, #e5484d); }
	.answered .ban-img { filter: saturate(0.7); }

	.done { position: absolute; inset: 0; z-index: 2; display: flex; align-items: center; justify-content: center; gap: 10px; background: color-mix(in srgb, var(--surface-1) 78%, transparent); -webkit-backdrop-filter: blur(6px); backdrop-filter: blur(6px); color: var(--text); font: var(--t-headline); }
	.done-ic { display: grid; place-items: center; width: 40px; height: 40px; border-radius: 50%; background: var(--ok); color: #fff; box-shadow: 0 8px 20px -8px color-mix(in srgb, var(--ok) 80%, transparent); }
	.calm { grid-column: 1 / -1; display: grid; justify-items: center; gap: 6px; padding: 64px 20px; border-radius: 18px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); color: var(--text-muted); text-align: center; }
	.calm-ic { display: grid; place-items: center; width: 52px; height: 52px; margin-bottom: 4px; border-radius: 50%; background: var(--surface-2); color: var(--text); }
	.calm h2 { color: var(--text); font: var(--t-title3); }
	.show-all { margin-top: 6px; }

	/* The window: one ask on the Stage. */
	.win { width: min(1180px, calc(100vw - 48px)); max-width: none; height: min(760px, calc(100dvh - 48px)); max-height: none; margin: auto; padding: 0; border: 0; border-radius: 24px; background: #0b0b0c; overflow: hidden; box-shadow: 0 0 0 1px rgb(255 255 255 / 0.08), 0 50px 120px -30px rgb(0 0 0 / 0.85); }
	.win[open] { animation: win-in 520ms cubic-bezier(0.22, 1, 0.36, 1) both; }
	.win::backdrop { background: rgb(0 0 0 / 0.6); -webkit-backdrop-filter: blur(8px); backdrop-filter: blur(8px); animation: fade-in 300ms var(--ease-out) both; }
	.win-in { height: 100%; }
	.win:focus { outline: none; }
	.close { position: absolute; top: 18px; right: 18px; z-index: 10; display: grid; place-items: center; width: 38px; height: 38px; border: 0; border-radius: 50%; background: rgb(20 20 24 / 0.55); color: #fff; box-shadow: inset 0 0 0 1px rgb(255 255 255 / 0.18); -webkit-backdrop-filter: blur(14px); backdrop-filter: blur(14px); cursor: pointer; transition: background 200ms var(--ease-out); }
	.close:hover { background: rgb(20 20 24 / 0.8); }
	.steps-nav { position: absolute; top: 18px; left: 18px; z-index: 10; display: flex; align-items: center; gap: 4px; padding: 3px; border-radius: 999px; background: rgb(20 20 24 / 0.55); box-shadow: inset 0 0 0 1px rgb(255 255 255 / 0.18); -webkit-backdrop-filter: blur(14px); backdrop-filter: blur(14px); color: #fff; }
	.nav { display: grid; place-items: center; width: 36px; height: 36px; border: 0; border-radius: 50%; background: none; color: #fff; cursor: pointer; transition: background 200ms var(--ease-out); }
	.nav:hover:not(:disabled) { background: rgb(255 255 255 / 0.16); }
	.nav:disabled { opacity: 0.35; cursor: default; }
	.pos { min-width: 52px; text-align: center; font: 600 12px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.win :global(.ctl.top) { right: 68px !important; }
	/* One close at a time: while the sheet is up, its own header closes it. */
	.win:has(:global(.frame.choosing)) .close { display: none; }
	.vh { position: absolute; width: 1px; height: 1px; margin: -1px; padding: 0; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; border: 0; }
	/* Touch screens: every control gets a full finger-sized target, at any width. */
	@media (pointer: coarse) { .nav, .close { width: 44px; height: 44px; } .b { height: 44px; } }
	@keyframes win-in { from { opacity: 0; transform: translate(var(--ox, 0), var(--oy, 16px)) scale(0.85); border-radius: 20px; } }
	@keyframes fade-in { from { opacity: 0; } }
	/* Phones: the tabs swipe, the filter fills the width, two cards per row, the window fills the screen. */
	@media (max-width: 767px) {
		.desktop { gap: 12px; padding-bottom: calc(var(--tabbar, 64px) + 16px + env(safe-area-inset-bottom, 0px)); }
		.toolbar { flex-direction: column; align-items: stretch; gap: 10px; }
		.tcap { width: 100%; }
		.tabs { width: 100%; overflow-x: auto; scrollbar-width: none; overscroll-behavior-x: contain; }
		.tabs::-webkit-scrollbar { display: none; }
		.tab { flex: none; padding: 0 12px; }
		.right { flex-direction: column-reverse; align-items: stretch; gap: 10px; margin-left: 0; }
		.cap { width: 100%; }
		.cap .seg { flex: 1; }
		.it { flex: 1; width: auto; gap: 5px; padding: 0 6px; }
		.approve-all { justify-content: center; height: 48px; }
		.cards { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
		.ban { height: 140px; }
		.ban-cap { left: 10px; right: 10px; bottom: 24px; }
		.ban-t { font-size: 17px; }
		.ban-f { font-size: 11px; }
		.sheet-b { margin: -16px 6px 6px; border-radius: 14px; }
		.body { padding: 10px 10px 0; }
		.who { font: var(--t-caption); }
		.foot { grid-template-columns: 44px minmax(0, 1fr); gap: 6px; padding: 10px; }
		.b { padding: 0 8px; font: var(--t-footnote); font-weight: 600; }
		.b.no .bl { display: none; }
		.win { width: 100vw; height: 100dvh; border-radius: 0; }
		.close { top: calc(12px + env(safe-area-inset-top, 0px)); right: 12px; width: 44px; height: 44px; }
		.win :global(.ctl.top) { right: 64px !important; }
	}
	@media (prefers-reduced-motion: reduce) {
		.plate, .bc, .fill, .it, .win[open], .win::backdrop, .ban-img, img.ban-img { transition: none; animation: none; }
		.bc:hover, .bc:hover .ban-img:not(.soft) { transform: none; }
	}
</style>
