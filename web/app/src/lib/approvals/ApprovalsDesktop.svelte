<script lang="ts">
	// Approvals for approvers. One toolbar the Downloads way: the status tabs as a
	// pill group on the left, the Filter button (status and type) on the right. Each card is
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

	// Five hover effects for the cards. ?hover shows a pill bar to switch; without it, effect 1.
	const HOVERS = ['Lean in', 'Lift', 'Glow', 'Parallax', 'Spotlight'];
	const hoverOn = typeof window !== 'undefined' && new URLSearchParams(window.location.search).has('hover');
	let hv = $state(1);
	if (hoverOn) {
		try { hv = Math.min(5, Math.max(1, Number(localStorage.getItem('approvals-hover')) || 1)); } catch { hv = 1; }
	}
	function pickHover(n: number) {
		hv = n;
		try { localStorage.setItem('approvals-hover', String(n)); } catch { /* storage blocked: the pick lasts this visit */ }
	}
	// Parallax and Spotlight follow the pointer: write its place on the card as CSS variables (no re-render).
	function track(e: PointerEvent) {
		if (hv < 4 || e.pointerType === 'touch') return;
		const el = e.currentTarget as HTMLElement;
		const r = el.getBoundingClientRect();
		const x = e.clientX - r.left, y = e.clientY - r.top;
		el.style.setProperty('--mx', `${x.toFixed(1)}px`);
		el.style.setProperty('--my', `${y.toFixed(1)}px`);
		el.style.setProperty('--px', String(Math.max(-1, Math.min(1, (x / r.width) * 2 - 1)).toFixed(3)));
		el.style.setProperty('--py', String(Math.max(-1, Math.min(1, (y / r.height) * 2 - 1)).toFixed(3)));
	}
	// "16 d ago" read out in full: "16 days ago".
	const longAgo = (s: string) => ago(s).replace(/^(\d+) (d|h|min) ago$/, (_, n: string, u: string) => `${n} ${{ d: 'day', h: 'hour', min: 'minute' }[u]}${n === '1' ? '' : 's'} ago`);

	const types: { k: 'all' | 'movie' | 'show' | 'artist'; label: string; icon: IconName }[] = [
		{ k: 'all', label: 'All', icon: 'library' },
		{ k: 'movie', label: 'Movies', icon: 'film' },
		{ k: 'show', label: 'TV shows', icon: 'tv' },
		{ k: 'artist', label: 'Music', icon: 'music' }
	];

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
	// The current tab's statuses; the status control shows only when there are two or more.
	const subs = $derived(statusesIn(ctx.approverTab));
	const hasSubs = $derived(subs.length > 1);
	const subsTotal = $derived(subs.reduce((n, x) => n + x.count, 0));
	const toneOf = (t: Life['tone']) => (t === 'bad' ? 'fail' : t === 'go' ? 'info' : t === 'ok' ? 'ok' : t === 'wait' ? 'wait' : 'off');
	function pickTab(t: ApproverTab) {
		sfMenu = false;
		if (ctx.approverTab !== t) {
			statusPick = { tab: '', word: '' };
			ctx.setApproverTab(t);
		}
	}
	function setStatus(word: string) {
		statusPick = word ? { tab: ctx.approverTab, word } : { tab: '', word: '' };
	}

	// The Filter menu (Status for this tab, then Type). On phones it opens as a bottom sheet.
	let phone = $state(false);
	$effect(() => {
		const mq = window.matchMedia('(max-width: 767px)');
		phone = mq.matches;
		const on = () => (phone = mq.matches);
		mq.addEventListener('change', on);
		return () => mq.removeEventListener('change', on);
	});
	// One open menu at a time: the status menu (1, 5) or the filter menu (3). Variant 5 on a phone opens it as a sheet.
	let sfMenu = $state(false);
	let sfBtn = $state<HTMLButtonElement>();
	let sfPop = $state<HTMLElement>();
	const sfItems = () => [...(sfPop?.querySelectorAll<HTMLElement>('[role^="menuitem"]') ?? [])];
	function sfToggle() {
		sfMenu = !sfMenu;
		if (sfMenu) void tick().then(() => (sfPop?.querySelector<HTMLElement>('[aria-checked="true"]') ?? sfItems()[0])?.focus());
	}
	function sfClose(back = true) {
		if (!sfMenu) return;
		sfMenu = false;
		if (back) sfBtn?.focus();
	}
	function sfKeys(e: KeyboardEvent) {
		if (e.key === 'Escape' || e.key === 'Tab') {
			if (e.key === 'Escape') {
				e.preventDefault();
				e.stopPropagation();
			}
			sfClose(e.key === 'Escape');
			return;
		}
		if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(e.key)) return;
		const items = sfItems();
		const i = items.indexOf(document.activeElement as HTMLElement);
		const n = e.key === 'Home' ? 0 : e.key === 'End' ? items.length - 1 : (i + (e.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length;
		e.preventDefault();
		items[n]?.focus();
	}
	$effect(() => {
		if (!sfMenu) return;
		const out = (e: PointerEvent) => {
			const t = e.target as Node;
			if (!sfPop?.contains(t) && !sfBtn?.contains(t)) sfClose(false);
		};
		document.addEventListener('pointerdown', out, true);
		return () => document.removeEventListener('pointerdown', out, true);
	});
	const filterCount = $derived((status ? 1 : 0) + (ctx.kind !== 'all' ? 1 : 0));
	const cardsShown = $derived(status ? list.filter((a) => statusOf(a) === status) : list);
	// Approve all: the waiting cards, minus any already in their Approved beat.
	const toApprove = $derived(cardsShown.filter((x) => waiting(x) && !approving.has(x.id)));

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
	// Closing eases out (scale down and fade), then the dialog closes.
	let closing: ReturnType<typeof setTimeout> | undefined;
	function hide() {
		if (!win?.open || closing) return;
		if (reduced()) return win.close();
		win.classList.add('win-out');
		closing = setTimeout(() => {
			closing = undefined;
			win?.classList.remove('win-out');
			win?.close();
		}, 320);
	}
	onDestroy(() => clearTimeout(closing));
	// Focus goes back to the card of the ask last shown (stepping moves it), else the card it opened from.
	function closed() {
		const id = openId;
		openId = 0;
		const card = document.querySelector<HTMLElement>(`.bc[data-ask="${id}"] .open`);
		(card ?? (from?.isConnected ? from : document.querySelector<HTMLElement>('.bc .open')))?.focus({ preventScroll: true });
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
		// The open ask isn't listed (it was just answered): stay put until the window moves on.
		if (at < 0) return;
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
	// Esc closes the whole window in one press (with the closing motion).
	function cancel(e: Event) {
		e.preventDefault();
		hide();
	}
</script>

<div class="desktop" data-testid="approvals-desktop">
	{#snippet statusItems(close: boolean)}
		<button type="button" role="menuitemradio" aria-checked={!status} class="sf-mi" onclick={() => { setStatus(''); if (close) sfClose(); }}><span class="sf-tick" aria-hidden="true"><Icon name="check" size={14} /></span><span class="sf-w">All statuses</span><span class="sf-c">{subsTotal}</span></button>
		{#each subs as x (x.word)}
			<button type="button" role="menuitemradio" aria-checked={status === x.word} class="sf-mi sf-t-{toneOf(x.tone)}" onclick={() => { setStatus(x.word); if (close) sfClose(); }}><span class="sf-tick" aria-hidden="true"><Icon name="check" size={14} /></span><span class="sf-d" aria-hidden="true"></span><span class="sf-w">{x.word}</span><span class="sf-c">{x.count}</span></button>
		{/each}
	{/snippet}
	{#snippet filterItems()}
		{#if hasSubs}
				<div class="sf-mh" role="presentation">Status</div>
				{@render statusItems(false)}
				<div class="sf-sep" role="separator"></div>
			{/if}
			<div class="sf-mh" role="presentation">Type</div>
			{#each types as t (t.k)}
				<button type="button" role="menuitemradio" aria-checked={ctx.kind === t.k} class="sf-mi" onclick={() => ctx.setKind(t.k)}><span class="sf-tick" aria-hidden="true"><Icon name="check" size={14} /></span><span class="sf-ti" aria-hidden="true"><Icon name={t.icon} size={14} /></span><span class="sf-w">{t.label}</span></button>
			{/each}
	{/snippet}
	<div class="toolbar">
		<div class="tcap">
		<div class="tabs" role="group" aria-label="Status">
			{#each approverTabs as t (t.tab)}
				{@const n = ctx.approverCounts[t.tab] ?? 0}
				{@const bad = t.tab === 'processing' ? ctx.approverCounts.failed : 0}
				{@const here = ctx.approverTab === t.tab}
				<div class="pw" class:on={here} role="presentation">
					<button type="button" class="tab" class:zero={!n} aria-pressed={here} aria-label="{t.label} {n}{bad ? `, ${bad} failed` : ''}{here && status ? `, showing ${status}` : ''}" onclick={() => pickTab(t.tab)}>{t.label}<span class="n" class:wait={t.tab === 'pending' && n}>{n}</span>{#if bad}<span class="fail-dot" aria-hidden="true"></span>{/if}</button>
				</div>
			{/each}
		</div>
		</div>
		<div class="right">
			{#if ctx.approverTab === 'pending' && toApprove.length > 1}
				<button type="button" class="approve-all" in:fade={{ duration: ms(200) }} onclick={() => ctx.approve(toApprove)}><Icon name="check" size={16} />Approve all {toApprove.length}</button>
			{/if}
			<div class="sf-anchor">
					<button type="button" class="sf-btn" bind:this={sfBtn} aria-haspopup={phone ? 'dialog' : 'menu'} aria-expanded={sfMenu} aria-label="Filter{filterCount ? `, ${filterCount} on` : ''}" onclick={sfToggle}>
						<svg class="sf-funnel" viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M2.5 3.5h11l-4.2 5v4l-2.6 1.2V8.5z" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" /></svg>
						<span class="sf-v">Filter</span>
						{#if filterCount}<span class="sf-badge" aria-hidden="true">{filterCount}</span>{/if}
					</button>
					{#if sfMenu && !phone}
						<div class="sf-menu" in:fly={{ y: -8, duration: ms(280), easing: cubicOut }} out:fade={{ duration: ms(180) }} role="menu" aria-label="Filter" tabindex="-1" bind:this={sfPop} onkeydown={sfKeys}>
							{@render filterItems()}
						</div>
					{/if}
				</div>
		</div>
	</div>
	{#if sfMenu && phone}
		<div class="sf-scrim" aria-hidden="true" transition:fade={{ duration: ms(300) }}></div>
		<div class="sf-sheet" role="dialog" aria-modal="true" aria-label="Filter" tabindex="-1" bind:this={sfPop} onkeydown={sfKeys} in:fly={{ y: 80, duration: ms(420), easing: cubicOut }} out:fly={{ y: 80, duration: ms(260), easing: cubicOut }}>
			<div class="sf-grab" aria-hidden="true"></div>
			<div role="menu" aria-label="Filter options">{@render filterItems()}</div>
		</div>
	{/if}

		<ul class="cards" aria-label="Asks" in:fade={{ duration: ms(260) }}>
			{#each cardsShown as a, i (a.id)}
				{@const w = ctx.who(a)}
				{@const life = lifeOf(a)}
				{@const photo = a.kind === 'artist' ? artistPhoto(a) : null}
				{@const wide = banner(a)}
				<li class="bc" data-ask={a.id} data-hover={hv} onpointermove={track} class:music={a.kind === 'artist'} class:answered={!waiting(a)} animate:flip={{ duration: ms(340), easing: cubicOut }} in:fly={{ y: 14, duration: ms(420), delay: ms(Math.min(i, 8) * 45), easing: cubicOut }} out:fade={{ duration: ms(220) }}>
					<!-- The whole card opens the window; the answers sit above this layer. -->
					<button type="button" class="open" aria-label="Open {a.name}, asked by {w.name}" aria-haspopup="dialog" onclick={(e) => void show(a, e.currentTarget)}></button>
					<div class="ban" class:noart={!wide && !photo && !a.poster}>
						{#if wide}<img class="ban-img" src={wide} alt="" loading="lazy" decoding="async" />{:else if photo}<span class="ban-img" aria-hidden="true" style:background-image={`url("${photo}")`}></span>{:else if a.poster}<span class="ban-img soft" aria-hidden="true" style:background-image={`url("${a.poster}")`}></span>{/if}
					</div>
					<div class="top">
						<div class="ban-cap"><h3 class="ban-t">{a.name}</h3><span class="ban-f">{[kindOf(a), a.year, wants(a)].filter(Boolean).join(' · ')}</span></div>
					</div>
					<div class="sheet-b">
					<div class="body">
						<span class="who"><Avatar name={w.name} src={w.avatar} size={26} /><span class="gl-id"><span class="gl-n"><b>{w.kid ? w.name.replace(/\s*\(kid\)\s*$/i, '') : w.name}</b>{#if w.kid}<KidMark label="Kid" size="sm" />{/if}</span><span class="gl-when">Asked <time datetime={a.asked_at} title={dateTime(a.asked_at)}>{longAgo(a.asked_at)}</time></span></span></span>
					</div>
					<div class="foot" inert={approving.has(a.id)}>
						{#if waiting(a)}
							<button type="button" class="b no" aria-label="Decline {a.name}" data-testid="ask-decline" onclick={() => ctx.turnDown([a])}><Icon name="x" size={15} /><span class="bl">Decline</span></button>
							<button type="button" class="b yes" aria-label="Approve {a.name}" data-testid="ask-approve" onclick={() => approveOne(a)}><Icon name="check" size={15} /><span class="bl">Approve</span></button>
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
					<span class="life tone-{approving.has(a.id) ? 'ok' : life.tone}" aria-hidden="true"><span class="fill" style:transform="scaleX({(approving.has(a.id) ? 100 : life.pct) / 100})"></span></span>
					{#if approving.has(a.id)}
						<span class="done" in:fade={{ duration: ms(180) }}><span class="done-ic" in:fly={{ y: 6, duration: ms(320), easing: cubicOut }}><Icon name="check" size={22} /></span>Approved</span>
					{/if}
				</li>
			{:else}
					<li class="calm">
						<span class="calm-ic"><Icon name="check" size={24} /></span>
						<h2>{calm[ctx.approverTab]?.title ?? 'Nothing here right now.'}</h2>
						<p>{ctx.kind !== 'all' ? 'Nothing of this type here. Try All.' : (calm[ctx.approverTab]?.text ?? '')}</p>
					</li>
			{/each}
		</ul>
</div>

{#if hoverOn}
	<div class="hv-bar" role="group" aria-label="Card hover (preview)">
		{#each HOVERS as name, i (name)}<button type="button" class="hv-pill" aria-pressed={hv === i + 1} onclick={() => pickHover(i + 1)}>{i + 1} · {name}</button>{/each}
	</div>
{/if}

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
<dialog class="win" tabindex="-1" onkeydown={winKeys} bind:this={win} aria-labelledby="stage-title" oncancel={cancel} onclose={closed} onclick={(e) => e.target === win && hide()}>
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

	/* The toolbar: tabs left, the Filter button right. */
	.tcap { position: relative; z-index: 6; }
	/* Status bar: slim pills. Bar 40 = 1 border + 3 inset + 32 pill + 3 + 1; pill radius 9 = bar radius 12 - inset 3.
	   Only the picked tab is raised; its statuses reveal out from behind it onto the track (clip + width + x + fade). */
	.pw { position: relative; display: flex; align-items: center; flex: none; height: 32px; }
	.pw .tab { height: 32px; }
	.pw.on .tab { background: var(--seg-thumb); box-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.12); }
	/* Coarse pointers keep a 44px target without fattening the bar. */
	@media (pointer: coarse) { .tab::after, .sf-btn::after { content: ''; position: absolute; inset: -6px 0; } }
	.dd { width: 6px; height: 6px; border-radius: 50%; }
	/* One row that never wraps: tabs left, the status control and the type filter right. */
	.toolbar { display: flex; flex-wrap: nowrap; align-items: center; justify-content: space-between; gap: 12px; }
	.toolbar .tcap { flex: 0 1 auto; min-width: 0; }
	.toolbar .tabs { max-width: 100%; overflow-x: auto; overflow-y: hidden; scrollbar-width: none; overscroll-behavior-x: contain; }
	.toolbar .tabs::-webkit-scrollbar { display: none; }
	.toolbar .right { flex: none; }
	.right { display: flex; align-items: center; gap: 10px; margin-left: auto; }
	.tabs { position: relative; display: flex; align-items: center; gap: 2px; height: 40px; box-sizing: border-box; padding: 3px; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); backdrop-filter: blur(18px) saturate(1.4); -webkit-backdrop-filter: blur(18px) saturate(1.4); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); }
	.tab { position: relative; z-index: 1; display: inline-flex; align-items: center; gap: 7px; height: 32px; padding: 0 13px; border: 0; border-radius: 9px; background: none; color: color-mix(in srgb, var(--text) 76%, var(--bg)); font: 500 13.5px/1 var(--font-ui); white-space: nowrap; cursor: pointer; transition: color 200ms var(--ease-out), background-color 200ms var(--ease-out), box-shadow 200ms var(--ease-out); }
	.tab:hover, .tab[aria-pressed='true'] { color: var(--text); }
	.tab:hover:not([aria-pressed='true']) { background: color-mix(in srgb, var(--text) 6%, transparent); }
	.tab.zero:not([aria-pressed='true']):not(:hover) { color: var(--text-muted); }
	.n { opacity: 0.75; font-weight: 600; font-variant-numeric: tabular-nums; }
	.n.wait { color: var(--wait-text, var(--wait)); opacity: 1; }
	.tab .fail-dot { width: 6px; height: 6px; margin-left: -2px; border-radius: 50%; background: var(--fail, #e5484d); }
	.approve-all { display: inline-flex; align-items: center; gap: 6px; height: 40px; padding: 0 14px; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); color: var(--text); font: 600 13px/1 var(--font-ui); cursor: pointer; backdrop-filter: blur(18px) saturate(1.4); -webkit-backdrop-filter: blur(18px) saturate(1.4); transition: transform 160ms var(--ease-out), background 200ms var(--ease-out); }
	.approve-all:hover { background: var(--surface-2); }
	.approve-all:active { transform: scale(0.96); }

	/* The Filter button and its menu (Status for this tab, then Type). Tones: failed red, live steps blue, ready green, waiting amber, the rest muted. */
	.sf-t-fail { --sc: var(--fail, #ff6961); --st: var(--fail-text, var(--fail, #ff6961)); }
	.sf-t-info { --sc: var(--info, #409cff); --st: var(--info-text, var(--info, #409cff)); }
	.sf-t-ok { --sc: var(--ok, #30d158); --st: var(--ok-text, var(--ok, #30d158)); }
	.sf-t-wait { --sc: var(--wait, #ffb340); --st: var(--wait-text, var(--wait, #ffb340)); }
	.sf-t-off { --sc: var(--text-muted); --st: var(--text); }
	.sf-d { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--sc, var(--text-muted)); }
	.sf-anchor { position: relative; }
	.sf-btn { position: relative; display: inline-flex; align-items: center; gap: 7px; height: 40px; padding: 0 14px; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); color: var(--text); font: 500 13px/1 var(--font-ui); white-space: nowrap; cursor: pointer; -webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4); transition: background 160ms var(--ease-out); }
	.sf-btn:hover, .sf-btn[aria-expanded='true'] { background: var(--surface-2); }
	.sf-funnel { color: var(--text-muted); }
	.sf-badge { min-width: 18px; height: 18px; padding: 0 5px; box-sizing: border-box; border-radius: 9px; background: var(--info, #409cff); color: #fff; font: 600 11px/18px var(--font-ui); text-align: center; }
	.sf-menu { position: absolute; right: 0; top: calc(100% + 6px); z-index: 20; min-width: 230px; max-height: min(420px, 70vh); overflow-y: auto; padding: 4px; border: 1px solid var(--line-strong); border-radius: 12px; background: var(--surface-2); box-shadow: 0 18px 40px -12px rgb(0 0 0 / 0.55); }
	.sf-menu:focus { outline: none; }
	.sf-mh { padding: 8px 10px 4px; color: var(--text-muted); font: 600 11px/1 var(--font-ui); letter-spacing: 0.06em; text-transform: uppercase; }
	.sf-sep { height: 1px; margin: 4px 6px; background: var(--line); }
	.sf-mi { display: flex; align-items: center; gap: 10px; width: 100%; height: 34px; padding: 0 10px; border: 0; border-radius: 8px; background: none; color: var(--text); font: 500 13px/1 var(--font-ui); text-align: left; white-space: nowrap; cursor: pointer; }
	.sf-mi:hover, .sf-mi:focus-visible, .sf-mi[aria-checked='true'] { background: color-mix(in srgb, var(--text) 8%, transparent); }
	.sf-mi:focus-visible { box-shadow: var(--focus-ring); }
	.sf-mi.sf-t-fail .sf-w { color: var(--st); }
	.sf-tick { display: grid; place-items: center; width: 14px; color: var(--info-text, var(--info, #409cff)); visibility: hidden; }
	.sf-mi[aria-checked='true'] .sf-tick { visibility: visible; }
	.sf-ti { display: grid; place-items: center; color: var(--text-muted); }
	.sf-c { margin-left: auto; padding-left: 16px; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	/* 5 on phones: a bottom sheet above the tab bar. */
	.sf-scrim { position: fixed; inset: 0; z-index: 96; background: rgb(0 0 0 / 0.45); }
	.sf-sheet { position: fixed; left: 0; right: 0; bottom: calc(var(--tabbar, 64px) + env(safe-area-inset-bottom, 0px)); z-index: 97; max-height: calc(100dvh - var(--tabbar, 64px) - env(safe-area-inset-bottom, 0px) - 48px); overflow-y: auto; box-sizing: border-box; padding: 10px max(16px, env(safe-area-inset-right, 0px)) 12px max(16px, env(safe-area-inset-left, 0px)); border-bottom: 1px solid var(--line); border-top: 1px solid var(--line-strong); border-radius: 18px 18px 0 0; background: var(--surface-2); box-shadow: 0 -20px 50px rgb(0 0 0 / 0.45); }
	.sf-sheet:focus { outline: none; }
	.sf-sheet .sf-mi { height: 48px; font-size: 15px; }
	.sf-grab { width: 40px; height: 4px; margin: 0 auto 6px; border-radius: 2px; background: var(--line-strong); }
	@media (pointer: coarse) { .sf-mi { height: 44px; } }
	@media (max-width: 767px) {
		.sf-btn { width: 100%; justify-content: flex-start; }
		.sf-badge { margin-left: auto; }
		.sf-menu { left: 0; right: 0; }
	}

	/* Window cards: a dark card (in both themes, it is led by the art), the title's wide art
	   with the title and its facts on it, then a frosted sheet over the art's foot holding
	   who asked and the answers. The life-cycle line runs along the bottom edge. */
	.cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(290px, 100%), 1fr)); gap: 16px; margin: 0; padding: 0; list-style: none; }
	/* The card takes the sheet's own colour (dark in dark mode, pale in light), so no band shows under the sheet. */
	.bc { --top: 170px; --gap: 10px; position: relative; isolation: isolate; overflow: hidden; display: flex; flex-direction: column; border-radius: 20px; background: #17181d; box-shadow: 0 0 0 1px var(--line), 0 12px 32px -16px rgb(0 0 0 / 0.6); transition: transform 380ms cubic-bezier(0.4, 0, 0.2, 1), box-shadow 380ms cubic-bezier(0.4, 0, 0.2, 1); }
	.open { position: absolute; inset: 0; z-index: 1; padding: 0; border: 0; border-radius: inherit; background: none; cursor: pointer; }
	.open:focus-visible { box-shadow: none; }
	.bc:has(.open:focus-visible) { box-shadow: 0 0 0 2px var(--text), 0 24px 48px -24px rgb(0 0 0 / 0.6); }
	.ban { position: absolute; inset: 0; z-index: -1; overflow: hidden; background: #17181d; border-radius: inherit; transform-origin: 50% 50%; transition: transform 380ms cubic-bezier(0.4, 0, 0.2, 1); will-change: transform; }
	.ban.noart { background: radial-gradient(120% 90% at 15% 0%, #3b4263 0%, transparent 60%), radial-gradient(90% 80% at 100% 100%, #583a58 0%, transparent 58%), #17181d; }
	.top { flex: none; height: var(--top); display: flex; align-items: flex-end; box-sizing: border-box; padding: 0 14px 10px; }
	.ban-img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; object-position: center 28%; background-size: cover; background-position: center 28%; transform: scale(1.02); transition: transform 400ms var(--ez), opacity 500ms var(--ease-out); }
	img.ban-img { animation: ban-in 600ms var(--ease-out) both; }
	@keyframes ban-in { from { opacity: 0; } }
	.ban-img.soft { filter: blur(18px) saturate(1.3); transform: scale(1.3); opacity: 0.8; }
	.ban::after { content: ''; position: absolute; inset: 0; background: linear-gradient(to bottom, rgb(0 0 0 / 0.18), transparent calc(var(--top) * 0.35), rgb(0 0 0 / 0.66) var(--top), rgb(0 0 0 / 0.4)); }
	.ban-cap { min-width: 0; display: flex; flex-direction: column; gap: 3px; color: #fff; text-shadow: 0 1px 12px rgb(0 0 0 / 0.5); }
	.ban-t { margin: 0; font: 800 22px/1.05 var(--font-display, var(--font-ui)); letter-spacing: -0.02em; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
	.ban-f { font: 600 12px/1.3 var(--font-ui); color: rgb(255 255 255 / 0.85); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	/* Clicks pass through the sheet to the card's open layer; only its buttons take their own. */
	.sheet-b, .top { pointer-events: none; }
	.sheet-b button { pointer-events: auto; }
	.sheet-b { position: relative; z-index: 1; display: flex; flex-direction: column; flex: 1; margin: 0 var(--gap) var(--gap); border-radius: calc(20px - var(--gap)); background: linear-gradient(180deg, var(--g1), var(--g2)), linear-gradient(var(--g3), var(--g3)); color: var(--text); -webkit-backdrop-filter: var(--blur); backdrop-filter: var(--blur); box-shadow: var(--edge); }
	.body { display: flex; flex-direction: column; gap: 4px; padding: 12px 12px 0; min-width: 0; }
	.who { display: flex; align-items: center; gap: 9px; min-width: 0; color: var(--text-muted); font: var(--t-footnote); white-space: nowrap; }
	.who > :global(*) { flex: none; }
	.gl-id { display: flex; flex: 1 1 auto; flex-direction: column; gap: 1px; min-width: 0; }
	.gl-n { display: flex; align-items: center; gap: 6px; min-width: 0; color: var(--text); font: 600 13.5px/1.3 var(--font-ui); }
	.gl-n > :global(*) { flex: none; }
	.gl-n > b { flex: 0 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.gl-when { overflow: hidden; color: var(--text-muted); font: 500 12px/1.3 var(--font-ui); text-overflow: ellipsis; white-space: nowrap; }
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
	.state.tone-bad { color: var(--gl-fail); }
	.state.tone-bad .dot { background: var(--fail, #e5484d); }
	.foot > .state.tone-bad { grid-column: 1; }
	.state.tone-soon .dot { background: var(--text-muted); }
	/* Inset from the card's rounded corners, so the line is never clipped. */
	.life { position: absolute; left: 20px; right: 20px; bottom: 3px; z-index: 2; height: 3px; overflow: hidden; border-radius: 3px; background: rgb(255 255 255 / 0.22); }
	.fill { display: block; width: 100%; height: 100%; border-radius: 3px; transform-origin: left; transition: transform 900ms cubic-bezier(0.22, 1, 0.36, 1); }
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
	.win[open]:global(.win-out) { animation: win-out 320ms cubic-bezier(0.32, 0.72, 0, 1) both; }
	.win[open]:global(.win-out)::backdrop { animation: fade-out 320ms var(--ease-out) both; }
	@keyframes win-out { to { opacity: 0; transform: translateY(10px) scale(0.96); } }
	@keyframes fade-out { to { opacity: 0; } }
	/* Phones: the tabs swipe, the filter fills the width, two cards per row, the window fills the screen. */
	@media (max-width: 767px) {
		.desktop { gap: 12px; padding-bottom: calc(var(--tabbar, 64px) + 16px + env(safe-area-inset-bottom, 0px)); }
		.toolbar { flex-direction: column; align-items: stretch; gap: 10px; }
		.tcap { width: 100%; }
		.tabs { width: 100%; overflow-x: auto; scrollbar-width: none; overscroll-behavior-x: contain; }
		.tabs::-webkit-scrollbar { display: none; }
		.tab { flex: none; padding: 0 12px; }
		/* Approve all and Filter share one row, equal halves, so the art starts sooner. */
		.right { display: grid; grid-template-columns: repeat(auto-fit, minmax(0, 1fr)); gap: 10px; margin-left: 0; }
		.approve-all { justify-content: center; }
		.right .sf-anchor, .right .sf-anchor :global(.sf-btn) { width: 100%; }
		.cards { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
		.bc { --top: 124px; --gap: 8px; }
		.top { padding: 0 10px 8px; }
		.ban-t { font-size: 17px; }
		.ban-f { font-size: 11px; }
		.sheet-b { border-radius: 12px; }
		.gl-n { font-size: 12.5px; gap: 4px; }
		.gl-when { font-size: 11px; }
		.body { padding: 10px 10px 0; }
		.who { gap: 7px; font: var(--t-caption); }
		/* Decline and Approve: equal halves, icons only (each has its full name for screen readers). */
		.foot { gap: 6px; padding: 10px; }
		.b { padding: 0 8px; font: var(--t-footnote); font-weight: 600; }
		.b .bl { display: none; }
		.win { width: 100vw; height: 100dvh; border-radius: 0; }
		.close { top: calc(12px + env(safe-area-inset-top, 0px)); right: 12px; width: 44px; height: 44px; }
		.win :global(.ctl.top) { right: 64px !important; }
	}
	@media (prefers-reduced-motion: reduce) {
		.bc, .fill, .win[open], .win::backdrop, .ban-img, img.ban-img, .win[open]:global(.win-out), .win[open]:global(.win-out)::backdrop { transition: none; animation: none; }
		.b:active, .approve-all:active { transform: none; }
		.bc[data-hover], .bc[data-hover]:hover, .bc[data-hover] .ban, .bc[data-hover] .sheet-b { transform: none !important; transition: none !important; }
	.bc::after, .sheet-b::after { display: none; }
	}

	/* The glass panel: smoked glass. Dark: deep smoke with a soft inner highlight. Light: a pale grey-blue smoke,
	   dark ink. The full-frame art shows around it and, blurred, through it. --text, --text-muted and --bg are
	   redefined on the panel so the answers and the status read on it. */
	.sheet-b { --blur: blur(22px) saturate(1.3); --text: light-dark(#14161c, #f5f5f7); --text-muted: light-dark(#3b4150, rgb(235 235 245 / 0.8)); --bg: light-dark(#eef0f5, #17171b); --gl-fail: light-dark(#86160b, #ff8f8a); --g1: light-dark(rgb(228 233 242 / 0.74), rgb(34 34 40 / 0.7)); --g2: light-dark(rgb(205 212 226 / 0.7), rgb(10 10 13 / 0.8)); --g3: light-dark(rgb(232 236 244 / 0.34), transparent); --edge: inset 0 1px 0 light-dark(rgb(255 255 255 / 0.3), rgb(255 255 255 / 0.16)), inset 0 0 0 1px light-dark(rgb(30 36 58 / 0.07), rgb(255 255 255 / 0.05)), 0 18px 34px -14px light-dark(rgb(10 14 30 / 0.5), rgb(0 0 0 / 0.8)); }
	.b.no { box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--text) 10%, transparent); }
	.b.yes { box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.2), 0 6px 14px -8px rgb(0 0 0 / 0.6); }

	/* Hover effects (mouse only; touch screens and reduced motion get none). Each moves only paint, never layout,
	   so the grid and the neighbours stay still. In is slow and eased, out is a little quicker. */
	.bc::after { content: ''; position: absolute; inset: 0; z-index: 1; border-radius: inherit; background: radial-gradient(circle 170px at var(--mx, 50%) var(--my, 40%), rgb(255 255 255 / 0.2), rgb(255 255 255 / 0.06) 45%, transparent 75%); opacity: 0; pointer-events: none; transition: opacity 300ms ease-out; }
	.sheet-b::after { content: ''; position: absolute; inset: 0; border-radius: inherit; background: rgb(255 255 255); opacity: 0; pointer-events: none; transition: opacity 380ms ease-out; }
	@media (hover: hover) and (pointer: fine) {
		.bc:hover, .bc:hover .ban, .bc:hover .sheet-b, .bc:hover::after, .bc:hover .sheet-b::after { transition-duration: 650ms; transition-timing-function: cubic-bezier(0.4, 0, 0.2, 1); }
		/* 1 Lean in: the art slowly zooms, the shadow deepens a little. */
		.bc[data-hover='1']:hover .ban { transform: scale(1.04); }
		.bc[data-hover='1']:hover { box-shadow: 0 0 0 1px var(--line), 0 18px 38px -16px rgb(0 0 0 / 0.78); }
		/* 2 Lift: the card rises, a softer larger shadow, the art zooms a touch, the glass brightens. */
		.bc[data-hover='2']:hover { transform: translateY(-4px); box-shadow: 0 0 0 1px var(--line), 0 30px 60px -22px rgb(0 0 0 / 0.6); }
		.bc[data-hover='2']:hover .ban { transform: scale(1.02); }
		.bc[data-hover='2']:hover .sheet-b::after { opacity: 0.08; }
		/* 3 Glow: a soft light rim fades in, the glass brightens, the art zooms a little. */
		.bc[data-hover='3']:hover { box-shadow: 0 0 0 1px light-dark(rgb(60 110 255 / 0.55), rgb(255 255 255 / 0.5)), 0 0 30px 2px light-dark(rgb(60 110 255 / 0.28), rgb(255 255 255 / 0.22)), 0 12px 32px -16px rgb(0 0 0 / 0.6); }
		.bc[data-hover='3']:hover .ban { transform: scale(1.03); }
		.bc[data-hover='3']:hover .sheet-b::after { opacity: 0.1; }
		/* 4 Parallax: the art drifts with the pointer, the glass drifts the other way. */
		.bc[data-hover='4']:hover .ban { transform: translate(calc(var(--px, 0) * 5.5px), calc(var(--py, 0) * 5.5px)) scale(1.04); }
		.bc[data-hover='4']:hover .sheet-b { transform: translate(calc(var(--px, 0) * -2px), calc(var(--py, 0) * -2px)); }
		.bc[data-hover='4'] .sheet-b { transition: transform 380ms cubic-bezier(0.4, 0, 0.2, 1); }
		/* 5 Spotlight: a soft light follows the pointer over the art and the glass. */
		.bc[data-hover='5']:hover .ban { transform: scale(1.02); }
		.bc[data-hover='5']:hover::after { opacity: 1; }
	}
	.hv-bar { position: fixed; left: 50%; bottom: calc(16px + env(safe-area-inset-bottom, 0px)); z-index: 2000; display: flex; gap: 4px; max-width: calc(100vw - 24px); box-sizing: border-box; overflow-x: auto; scrollbar-width: none; padding: 4px; border-radius: 999px; transform: translateX(-50%); background: color-mix(in srgb, var(--surface-2) 88%, transparent); -webkit-backdrop-filter: blur(18px); backdrop-filter: blur(18px); box-shadow: 0 0 0 1px var(--line-strong), 0 12px 30px -10px rgb(0 0 0 / 0.5); }
	.hv-bar::-webkit-scrollbar { display: none; }
	.hv-pill { flex: none; height: 34px; padding: 0 14px; border: 0; border-radius: 999px; background: none; color: var(--text); font: 600 12.5px/1 var(--font-ui); white-space: nowrap; cursor: pointer; }
	.hv-pill:hover { background: color-mix(in srgb, var(--text) 8%, transparent); }
	.hv-pill[aria-pressed='true'] { background: var(--text); color: var(--bg); }
	@media (max-width: 767px) {
		.hv-bar { bottom: calc(var(--tabbar, 64px) + 12px + env(safe-area-inset-bottom, 0px)); }
		.hv-pill { height: 32px; padding: 0 11px; font-size: 12px; }
	}
</style>
