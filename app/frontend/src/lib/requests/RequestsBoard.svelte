<script lang="ts">
	// The Requests page for approvers. One toolbar like Downloads: the status tabs as a pill group on the left, the Filter button (status and type) on the right. Each card is a small window: a dark card with the title's wide art and a frosted sheet holding who asked and the answers.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon, { type IconName } from '#lib/ui/Icon.svelte';
	import Avatar from '#lib/ui/Avatar.svelte';
	import StatusIcon, { statusKey, type StatusKey } from '#lib/ui/StatusIcon.svelte';
	import KidMark from './KidMark.svelte';
	import RequestsFilter from './RequestsFilter.svelte';
	import RequestWindow from './RequestWindow.svelte';
	import { askHref, type BoardCtx } from './board.ts';
	import { artistPhoto, askArtist, storedParts } from './details.svelte.ts';
	import { shortDate, whenDate } from './inbox.ts';
	import { lifecycle, upcoming, type Life } from './lifecycle.ts';
	import { approverTabs, askTab, approverTab as tabOfAsk, type ApproverTab } from './ask-words.ts';
	import { askedFor } from './asked-for.ts';
	import { watchlistLine } from '../accounts/index.ts';
	import { onDestroy, tick } from 'svelte';
	import { flip } from 'svelte/animate';
	import { fade, fly, slide } from 'svelte/transition';
	import { cubicIn, cubicOut } from 'svelte/easing';
	import { agoLong, dateTime } from '../core/format.ts';
	import { to } from '../core/paths.ts';
	import { proxied } from '../core/tmdb-image.ts';
	import type { Ask } from '../core/types.ts';

	let { ctx }: { ctx: BoardCtx } = $props();

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
		// The first windows open with their seasons and albums already here (a window reads them at first paint).
		for (const a of list.slice(0, 4)) storedParts(a);
	});

	const types: { k: 'all' | 'movie' | 'show' | 'artist'; label: string; icon: IconName }[] = [
		{ k: 'all', get label() { return t('requests.board.kindAll'); }, icon: 'library' },
		{ k: 'movie', get label() { return t('requests.cards.kind.movie'); }, icon: 'film' },
		{ k: 'show', get label() { return t('requests.cards.kind.show'); }, icon: 'tv' },
		{ k: 'artist', get label() { return t('requests.cards.kind.artist'); }, icon: 'music' }
	];

	// What an empty tab says.
	const calm = (tab: string): { title: string; text: string } | undefined =>
		['pending', 'processing', 'done'].includes(tab) ? { title: t(`requests.board.calm.${tab}.title`), text: t(`requests.board.calm.${tab}.text`) } : undefined;

	// The status line with what the page knows: not out yet (from the release and air dates). Partly in the Library ("1 of 2 ready") needs which asked seasons and albums are in the Library, which the asks list does not carry yet.
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
		// A known date first; else the server's "not out yet".
		return lifecycle(a, out ? { upcoming: shortDate(out) } : live.upcoming !== undefined ? { upcoming: live.upcoming } : {});
	};
	// Worked out once per ask; the cards, the status filter and its counts all read it.
	const lives = $derived(new Map(ctx.viewed.map((a) => [a.id, lifeNow(a)])));
	const lifeOf = (a: Ask): Life => lives.get(a.id) ?? lifeNow(a);

	// The status row of answered cards. Processing and Failed: a button-shaped capsule that fills in its tone (the live download share when known, else the stage's place on the line). Done: an icon chip like the who row.
	const stTone = (t: Life['tone']) => (t === 'bad' ? 'bad' : t === 'go' ? 'go' : t === 'ok' ? 'ok' : 'soft');
	const stKey = (l: Life): StatusKey => (l.stage === 'asked' ? 'pending' : l.stage === 'getting' ? 'processing' : l.stage);
	/** The status word: "Downloading · 48%" becomes "Downloading". */
	const stWord = (l: Life) => l.label.split(' · ')[0];
	/** The live download share, only when the label carries one. */
	const stPct = (l: Life) => {
		const m = /(\d+)%$/.exec(l.label);
		return m ? Number(m[1]) : null;
	};
	/** Declined by whom, only when the page knows a display name (an account name alone is not said). */
	const stDeclinedBy = (a: Ask) => {
		if (!a.decided_by || a.state !== 'turned_down') return '';
		const name = ctx.decidedBy(a);
		return name && name !== a.decided_by.name ? name : '';
	};
	/** The detail after the word, from what the ask really carries. */
	const stDetail = (a: Ask, l: Life) => {
		if (l.stage === 'ready' && a.available_at) return whenDate(a.available_at);
		if (l.stage === 'declined') {
			const by = stDeclinedBy(a);
			return by ? t('requests.board.by', { name: by }) : a.decided_at ? whenDate(a.decided_at) : '';
		}
		// The server's reason, as a short tail on the capsule (the window says it in full).
		if (l.stage === 'failed') return (l.reason ?? '').replace(/\.$/, '');
		return l.label.split(' · ').slice(1).join(' · ');
	};
	/** How full the capsule is, 0 to 1: the live share when known, else the stage's place (failed stops where it failed). */
	const stFill = (l: Life) => Math.max(0, Math.min(100, stPct(l) ?? l.pct)) / 100;
	const stChip = (l: Life) => l.stage === 'ready' || l.stage === 'declined';

	// Filter by status within the tab: the status word without its detail.
	const statusOf = (a: Ask) => lifeOf(a).label.split(' · ')[0];
	// The picked status belongs to one tab: it clears when the tab changes by any path, and when it no longer exists in that tab.
	let statusPick = $state<{ tab: ApproverTab | ''; word: string }>({ tab: '', word: '' });
	/** The statuses in one tab, in the page's order (Processing: failed first). */
	const flow = () => ['waiting', 'upcoming', 'searching', 'downloading', 'importing', 'processing', 'ready'].map((k) => t(`requests.life.${k}`));
	const statusesIn = (t: ApproverTab) => {
		const n = new Map<string, { count: number; tone: Life['tone']; key: string }>();
		for (const a of ctx.viewed) {
			if (tabOfAsk(a) !== t) continue;
			const w = statusOf(a);
			const x = n.get(w);
			n.set(w, { count: (x?.count ?? 0) + 1, tone: x?.tone ?? lifeOf(a).tone, key: x?.key ?? lifeOf(a).status });
		}
		// Failed first (it needs you), then the order an ask moves through, Declined last.
		const order = flow();
		const rank = (x: { word: string; tone: Life['tone'] }) => (x.tone === 'bad' ? 0 : x.tone === 'off' ? 100 : order.indexOf(x.word) + 1 || 50);
		return [...n].map(([word, x]) => ({ word, ...x })).sort((x, y) => rank(x) - rank(y));
	};
	// Kept even when a Type pick leaves no match: the empty state says so and offers Show all, so the user's filter never changes by itself.
	const status = $derived(statusPick.word && statusPick.tab === ctx.approverTab ? statusPick.word : '');
	function showAll() {
		statusPick = { tab: '', word: '' };
		ctx.setKind('all');
	}
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

	let sfMenu = $state(false);
	let sfBtn = $state<HTMLButtonElement>();
	const filterCount = $derived((status ? 1 : 0) + (ctx.kind !== 'all' ? 1 : 0));
	// A download that failed (known only from the live stages) joins the failed asks at the front of Processing.
	const isFailed = (a: Ask) => lifeOf(a).stage === 'failed';
	const ordered = $derived(ctx.approverTab === 'processing' ? [...list].sort((x, y) => Number(isFailed(y)) - Number(isFailed(x))) : list);
	const cardsShown = $derived(status ? ordered.filter((a) => statusOf(a) === status) : ordered);
	const failedCount = $derived(ctx.viewed.filter((a) => tabOfAsk(a) === 'processing' && isFailed(a)).length);
	// Approve all: the waiting cards, minus any already in their Approved beat.
	const toApprove = $derived(cardsShown.filter((x) => waiting(x) && !approving.has(x.id)));

	// The card's banner: the title's wide art (show or movie backdrop, the artist's wide photo).
	const banner = (a: Ask) => {
		if (a.kind === 'artist') return askArtist(a)?.backgrounds[0] ?? '';
		const d = ctx.details(a);
		return d?.backdrop ? to(d.backdrop) : '';
	};

	// What the ask is for, in words that fit a card.
	const kindOf = (a: Ask) => t(a.kind === 'show' ? 'requests.kind.show' : a.kind === 'artist' ? 'requests.kind.artist' : 'requests.kind.movie');
	const wants = (a: Ask) => askedFor(a, { seasons: ctx.info(a)?.seasons.length });

	// Approving shows its answer on the card for a beat, then the card leaves (Undo stays in the toast).
	let approving = $state(new Set<number>());
	const beats = new Map<ReturnType<typeof setTimeout>, Ask>();
	// Leaving mid-beat still approves: hand the ask on now, and the page's own leave step sends it (a child is torn down before its page).
	onDestroy(() => {
		for (const [t, a] of beats) {
			clearTimeout(t);
			if (waiting(a)) ctx.approve([a]);
		}
	});
	function approveOne(a: Ask) {
		if (approving.has(a.id)) return;
		// Read before the foot turns inert (inert drops the focus).
		const had = !!document.querySelector(`.bc[data-ask="${a.id}"]`)?.contains(document.activeElement);
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
			// Focus goes on to the next card, so the keyboard does not fall back to the page top.
			const card = document.querySelector<HTMLElement>(`.bc[data-ask="${a.id}"]`);
			const after = card?.nextElementSibling?.querySelector<HTMLElement>('.open') ?? null;
			ctx.approve([a]);
			const next = new Set(approving);
			next.delete(a.id);
			approving = next;
			if (had) void tick().then(() => (after?.isConnected ? after : (document.querySelector<HTMLElement>('.bc .open') ?? sfBtn))?.focus({ preventScroll: true }));
		}, ms(560));
		beats.set(t, a);
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
	// ?ask=<id> (History's "Go to Requests") opens that ask's window once the asks are here.
	let wanted = typeof location === 'undefined' ? 0 : Number(new URLSearchParams(location.search).get('ask')) || 0;
	$effect(() => {
		const a = wanted ? ctx.viewed.find((x) => x.id === wanted) : undefined;
		if (!a) return;
		wanted = 0;
		void show(a, document.body);
	});
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
				const i = cardsShown.findIndex((x) => x.id === id);
				const ok = (x: Ask) => x.id !== id && waiting(x);
				const next = (i >= 0 ? cardsShown.slice(i + 1).find(ok) : undefined) ?? cardsShown.slice(0, Math.max(i, 0)).find(ok) ?? cardsShown.find(ok);
				if (next) openId = next.id;
				else {
					// Undo can still bring it back: reopen on it then, so the approver lands where they were.
					reopen = { id, until: Date.now() + 6500 };
					hide();
				}
			}, ms(650));
		}
		// Answered and then undone (the toast's Undo): stay on it.
		if (w && wasWaiting !== id) clearTimeout(advance);
		wasWaiting = w ? id : 0;
	});
	$effect(() => () => clearTimeout(advance));
	let reopen = $state<{ id: number; until: number } | null>(null);
	$effect(() => {
		const r = reopen;
		if (!r) return;
		const a = ctx.viewed.find((x) => x.id === r.id);
		if (!a || !waiting(a) || openId || closing) return;
		reopen = null;
		if (Date.now() > r.until) return;
		const el = document.querySelector<HTMLElement>(`.bc[data-ask="${r.id}"] .open`);
		if (el) void show(a, el);
	});
	// The ask left the list (filter, tab or a refresh): do not leave an empty window open.
	$effect(() => {
		if (openId && !openAsk && win?.open) hide();
	});
	// Arrow keys move to the ask before or after, without closing the window.
	const at = $derived(cardsShown.findIndex((a) => a.id === openId));
	// "2 of 9: Name", read out when stepping.
	let said = $state('');
	function step(by: number) {
		// The open ask is not listed (it was just answered): stay put until the window moves on.
		if (at < 0) return;
		const next = cardsShown[at + by];
		if (!next) return;
		openId = next.id;
		// Read back from the window's own position, so the words match what it shows.
		void tick().then(() => (said = t('requests.board.said', { pos: at + 1, total: cardsShown.length, name: next.name })));
	}
	function winKeys(e: KeyboardEvent) {
		if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
		// Controls inside the sheet keep their arrows; the sheet itself (focused when it opens) steps.
		if (e.target instanceof Element && (e.target.closest('input, textarea, select, video, [contenteditable], [role="group"], button[aria-pressed]') || e.target.closest('.sheet *'))) return;
		if (document.querySelectorAll('dialog[open]').length > 1) return;
		e.preventDefault();
		step(e.key === 'ArrowRight' ? 1 : -1);
	}
	// Esc closes the whole window in one press, with the closing motion.
	function cancel(e: Event) {
		e.preventDefault();
		hide();
	}
</script>

<div class="desktop" data-testid="requests-desktop">
	<div class="toolbar">
		<div class="tcap">
		<div class="tabs" role="group" aria-label={t('requests.board.status')}>
			{#each approverTabs as tb (tb.tab)}
				{@const n = ctx.approverCounts[tb.tab] ?? 0}
				{@const bad = tb.tab === 'processing' ? failedCount : 0}
				{@const here = ctx.approverTab === tb.tab}
				<div class="pw" class:on={here} role="presentation">
					<button type="button" class="tab" class:zero={!n} aria-pressed={here} aria-label={t('requests.board.tabLabel', { label: tb.label, n }) + (bad ? t('requests.board.tabFailed', { n: bad }) : '') + (here && status ? t('requests.board.tabShowing', { status }) : '')} onclick={() => pickTab(tb.tab)}>{tb.label}<span class="n" class:wait={tb.tab === 'pending' && n}>{n}</span>{#if bad}<span class="fail-dot" aria-hidden="true"></span>{/if}</button>
				</div>
			{/each}
		</div>
		</div>
		<div class="right">
			{#if ctx.reports >= 0}
				<a class="rp-btn" href={to('/requests/reports')} data-testid="reports-link" aria-label={ctx.reports ? t('requests.board.reportsOpen', { n: ctx.reports }) : t('requests.reports.title')}><Icon name="alert" size={16} /><span class="rp-v">{t('requests.reports.title')}</span>{#if ctx.reports}<span class="rp-n" aria-hidden="true">{ctx.reports}</span>{/if}</a>
			{/if}
			{#if ctx.approverTab === 'pending' && toApprove.length > 1}
				<button type="button" class="approve-all" in:fade={{ duration: ms(200) }} onclick={() => ctx.approve(toApprove)}><Icon name="check" size={16} />{t('requests.board.approveAll')}<span class="aa-n">{toApprove.length}</span></button>
			{/if}
			<RequestsFilter {ctx} {types} {subs} {hasSubs} {subsTotal} {filterCount} {status} {toneOf} {setStatus} {ms} bind:open={sfMenu} bind:btn={sfBtn} />
		</div>
	</div>
		{#snippet stDecline(a: Ask)}
			<button type="button" class="b no" aria-label={t('requests.window.declineName', { name: a.name })} data-testid="ask-decline" onclick={() => ctx.turnDown([a])}><Icon name="x" size={15} /><span class="bl">{t('requests.window.decline')}</span></button>
		{/snippet}
		{#snippet stIc(l: Life, size: number)}
			<span class="st-i" aria-hidden="true"><StatusIcon status={stKey(l)} {size} /></span>
		{/snippet}
		{#snippet stRow(a: Ask, l: Life)}
			{@const d = stDetail(a, l)}
			{@const can = l.stage === 'failed' && failed(a)}
			{#if stChip(l)}
				<span class="st-chip st-{stTone(l.tone)}">
					<span class="st-sq">{@render stIc(l, 16)}</span>
					<span class="st-id"><b>{stWord(l)}</b>{#if d}<span class="st-d">{d}</span>{/if}</span>
				</span>
			{:else}
				<span class="st-cap st-{stTone(l.tone)}" class:st-half={can} class:st-live={l.stage === 'searching' || l.stage === 'downloading'} style:--st-fill={stFill(l)}>
					<span class="st-meter" aria-hidden="true"></span>
					{@render stIc(l, 16)}<span class="st-tx" title={l.stage === 'failed' && d ? d : undefined}><b>{stWord(l)}</b>{#if d}<span class="st-d">{d}</span>{/if}</span>
				</span>
				{#if can}{@render stDecline(a)}{/if}
			{/if}
		{/snippet}
		<ul class="cards" aria-label={t('requests.board.requests')} in:fade={{ duration: ms(260) }}>
			{#each cardsShown as a, i (a.id)}
				{@const w = ctx.who(a)}
				{@const isKid = !!w.kid || /\(kid\)\s*$/i.test(w.name)}
				{@const life = lifeOf(a)}
				{@const photo = a.kind === 'artist' ? proxied(artistPhoto(a)) || null : null}
				{@const wide = proxied(banner(a)) || null}
				<li class="bc" data-ask={a.id} class:music={a.kind === 'artist'} class:answered={!waiting(a)} class:st-row={!waiting(a) && !ctx.isPending(a.id)} animate:flip={{ duration: ms(340), easing: cubicOut }} in:fly={{ y: 14, duration: ms(420), delay: ms(Math.min(i, 8) * 45), easing: cubicOut }} out:fade={{ duration: ms(220) }}>
					<!-- The whole card opens the window; the answers sit above this layer. -->
					<button type="button" class="open" aria-label={t('requests.board.open', { name: a.name, who: w.name })} aria-haspopup="dialog" onclick={(e) => void show(a, e.currentTarget)}></button>
					{#if askHref(a)}<a class="cl-go" href={to(askHref(a))} aria-label={t(a.kind === 'artist' ? 'requests.board.pageForArtist' : a.kind === 'show' ? 'requests.board.pageForShow' : 'requests.board.pageForMovie', { name: a.name })} title={t(a.kind === 'artist' ? 'requests.window.pageArtist' : a.kind === 'show' ? 'requests.window.pageShow' : 'requests.window.pageMovie')}><Icon name="arrowUpRight" size={16} /></a>{/if}
					<div class="ban" class:noart={!wide && !photo && !a.poster}>
						{#if wide}<img class="ban-img" src={wide} alt="" loading="lazy" decoding="async" />{:else if photo}<span class="ban-img" aria-hidden="true" style:background-image={`url("${photo}")`}></span>{:else if a.poster}<span class="ban-img soft" aria-hidden="true" style:background-image={`url("${proxied(a.poster)}")`}></span>{/if}
					</div>
					<div class="top">
						<div class="ban-cap">{#if ctx.isNew(a.id)}<span class="new-mark" data-testid="ask-new">{t('requests.board.new')}</span>{/if}<h2 class="ban-t">{a.name}</h2><span class="ban-f">{#if a.four_k}<span class="ap4k-tag" title={t('requests.board.fourKTitle')}>4K</span>{/if}{[kindOf(a), a.year, wants(a)].filter(Boolean).join(' · ')}</span></div>
					</div>
					<div class="sheet-b">
					<div class="body">
						<span class="who"><Avatar name={w.name} src={w.avatar} size={26} /><span class="gl-id"><span class="gl-n"><b>{w.name.replace(/\s*\(kid\)\s*$/i, '')}</b>{#if isKid}<KidMark label={t('requests.window.kid')} size="sm" />{/if}</span><span class="gl-when">{t('requests.board.asked')} <time datetime={a.asked_at} title={dateTime(a.asked_at)}>{agoLong(a.asked_at)}</time>{#if watchlistLine(a.people, undefined)}<span data-testid="ask-from-watchlist">{` · ${t('requests.board.fromPlex')}`}</span>{/if}</span></span></span>
					</div>
					<div class="foot" inert={approving.has(a.id)}>
						{#if waiting(a)}
							<button type="button" class="b no" aria-label={t('requests.window.declineName', { name: a.name })} data-testid="ask-decline" onclick={() => ctx.turnDown([a])}><Icon name="x" size={15} /><span class="bl">{t('requests.window.decline')}</span></button>
							<button type="button" class="b yes" aria-label={t('requests.window.approveName', { name: a.name })} data-testid="ask-approve" onclick={() => approveOne(a)}><Icon name="check" size={15} /><span class="bl">{t('requests.window.approve')}</span></button>
						{:else if !ctx.isPending(a.id)}
							{@render stRow(a, life)}
						{:else}
							<span class="said">
								{#if ctx.isPending(a.id)}
									<span class="state">{t('requests.window.sending')}</span>
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
						<span class="done" in:fade={{ duration: ms(180) }}><span class="done-ic" in:fly={{ y: 6, duration: ms(320), easing: cubicOut }}><Icon name="check" size={22} /></span>{t('requests.parts.approved')}</span>
					{/if}
				</li>
			{:else}
					<li class="calm">
						<span class="calm-ic"><Icon name="check" size={24} /></span>
						{#if filterCount}
							<h2>{status ? t('requests.board.nothingStatus', { status: status.toLowerCase() }) : t('requests.board.noKind', { kind: (types.find((ty) => ty.k === ctx.kind)?.label ?? '').toLowerCase() })}</h2>
							<p>{t('requests.board.hidden')}</p>
							<button type="button" class="calm-all" onclick={showAll}>{t('requests.board.showAll')}</button>
						{:else}
							<h2>{calm(ctx.approverTab)?.title ?? t('requests.board.nothingNow')}</h2>
							<p>{calm(ctx.approverTab)?.text ?? ''}</p>
						{/if}
					</li>
			{/each}
		</ul>
</div>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
<dialog class="win" tabindex="-1" onkeydown={winKeys} bind:this={win} aria-labelledby="stage-title-{openId}" oncancel={cancel} onclose={closed} onclick={(e) => e.target === win && hide()}>
	{#if openAsk}
		<button type="button" class="close" aria-label={t('requests.board.close')} onclick={hide}><Icon name="x" size={18} /></button>
		{#if cardsShown.length > 1 && at >= 0}
			<div class="steps-nav">
				<button type="button" class="nav" aria-label={t('requests.board.previous')} disabled={at <= 0} onclick={() => step(-1)}><Icon name="chevronLeft" size={18} /></button>
				<span class="pos">{t('requests.board.pos', { pos: at + 1, total: cardsShown.length })}</span>
				<button type="button" class="nav" aria-label={t('requests.board.next')} disabled={at >= cardsShown.length - 1} onclick={() => step(1)}><Icon name="chevronRight" size={18} /></button>
			</div>
		{/if}
		<div class="win-in"><RequestWindow {ctx} single={openAsk} choose /></div>
		<span class="vh" aria-live="polite">{said}</span>
	{/if}
</dialog>

<style>
	.desktop { --ez: cubic-bezier(0.32, 0.72, 0, 1); display: flex; flex-direction: column; gap: 16px; }
	button { font: inherit; color: inherit; }
	button:focus-visible { outline: none; box-shadow: var(--focus-ring); }

	.tcap { position: relative; z-index: 6; }
	/* Status bar: slim pills. Bar 40 = 1 border + 3 inset + 32 pill + 3 + 1; pill radius 9 = bar radius 12 - inset 3. Only the picked tab is raised; its statuses reveal out from behind it onto the track. */
	.pw { position: relative; display: flex; align-items: center; flex: none; height: 32px; }
	.pw .tab { height: 32px; }
	.pw.on .tab { background: var(--seg-thumb); box-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.12); }
	/* Coarse pointers keep a 44px target without fattening the bar. */
	@media (pointer: coarse) { .tab::after { content: ''; position: absolute; inset: -6px 0; } .right .approve-all, .right :global(.sf-btn), .right .rp-btn { height: 44px; } }
	/* Phones: the tab track grows so each tab's hit area (::after, inset -6px 0) reaches 44px. */
	@media (max-width: 767px) and (pointer: coarse) { .toolbar .tabs { height: 46px; padding-block: 6px; border-radius: 15px; } }
	.dd { width: 6px; height: 6px; border-radius: 50%; }
	.toolbar { display: flex; flex-wrap: nowrap; align-items: center; justify-content: space-between; gap: 12px; }
	.toolbar .tcap { flex: 0 1 auto; min-width: 0; }
	.toolbar .tabs { max-width: 100%; overflow-x: auto; overflow-y: hidden; scrollbar-width: none; overscroll-behavior-x: contain; }
	.toolbar .tabs::-webkit-scrollbar { display: none; }
	.toolbar .right { flex: none; }
	.right { display: flex; align-items: center; gap: 10px; margin-left: auto; }
	.tabs { position: relative; display: flex; align-items: center; gap: 2px; height: 40px; box-sizing: border-box; padding: 3px; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); backdrop-filter: blur(18px) saturate(1.4); -webkit-backdrop-filter: blur(18px) saturate(1.4); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); }
	.tab { position: relative; z-index: 1; display: inline-flex; align-items: center; gap: 7px; height: 32px; padding: 0 13px; border: 0; border-radius: 9px; background: none; color: color-mix(in srgb, var(--text) 88%, var(--bg)); font: 500 13.5px/1 var(--font-ui); white-space: nowrap; cursor: pointer; transition: color 200ms var(--ease-out), background-color 200ms var(--ease-out), box-shadow 200ms var(--ease-out); }
	.tab:hover, .tab[aria-pressed='true'] { color: var(--text); }
	.tab:hover:not([aria-pressed='true']) { background: color-mix(in srgb, var(--text) 6%, transparent); }
	.tab.zero:not([aria-pressed='true']):not(:hover) { color: var(--text-muted); }
	.n { font-weight: 600; font-variant-numeric: tabular-nums; }
	.n.wait { color: var(--wait-text, var(--wait)); opacity: 1; }
	/* On the raised thumb (dark) the amber needs to be lighter to read. */
	:global(:root[data-theme='dark']) .pw.on .n.wait { color: #ffd08a; }
	@media (prefers-color-scheme: dark) { :global(:root:not([data-theme='light'])) .pw.on .n.wait { color: #ffd08a; } }
	.tab .fail-dot { width: 6px; height: 6px; margin-left: -2px; border-radius: 50%; background: var(--fail, #e5484d); }
	.approve-all { display: inline-flex; align-items: center; gap: 6px; height: 40px; padding: 0 14px; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); color: var(--text); font: 600 13px/1 var(--font-ui); cursor: pointer; backdrop-filter: blur(18px) saturate(1.4); -webkit-backdrop-filter: blur(18px) saturate(1.4); transition: transform 160ms var(--ease-out), background 200ms var(--ease-out); }
	.approve-all:hover { background: var(--surface-2); }
	.rp-btn { display: inline-flex; align-items: center; gap: 7px; height: 40px; padding: 0 14px; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); color: var(--text); font: 500 13px/1 var(--font-ui); white-space: nowrap; text-decoration: none; -webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4); transition: background 160ms var(--ease-out); }
	.rp-btn:hover { background: var(--surface-2); }
	.rp-btn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.rp-n { min-width: 18px; height: 18px; padding: 0 5px; box-sizing: border-box; border-radius: 9px; background: var(--accent-fill); color: var(--text-on-red); font: 600 11px/18px var(--font-ui); text-align: center; font-variant-numeric: tabular-nums; }
	/* Narrow phones: the three toolbar buttons drop their icons together so each word fits. */
	@media (max-width: 379px) { .right .approve-all :global(svg), .right .rp-btn :global(svg), .right :global(.sf-btn .sf-funnel) { display: none; } }
	.new-mark { align-self: flex-start; margin-bottom: 4px; padding: 2px 8px; border-radius: var(--r-pill); background: var(--accent-fill); color: var(--text-on-red); font: 600 11px/16px var(--font-ui); }
	.approve-all:active { transform: scale(0.96); }
	.aa-n { margin-left: -2px; font-variant-numeric: tabular-nums; }

	.calm-all:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.calm-all { margin-top: 8px; min-height: 36px; padding: 0 16px; border: 0; border-radius: var(--r-pill, 999px); background: var(--surface-2); color: var(--text); box-shadow: 0 0 0 1px var(--line); font: 600 14px/1 var(--font-ui); cursor: pointer; }
	@media (pointer: coarse) { .calm-all { min-height: 44px; } }

	.cards { position: relative; display: grid; grid-template-columns: repeat(auto-fill, minmax(min(290px, 100%), 1fr)); gap: 16px; margin: 0; padding: 0; list-style: none; }
	/* The card takes the sheet's own colour (dark in dark mode, pale in light), so no band shows under the sheet. */
	.bc { --top: 170px; --gap: 10px; position: relative; isolation: isolate; overflow: hidden; display: flex; flex-direction: column; border-radius: 20px; background: #17181d; box-shadow: 0 0 0 1px var(--line), 0 12px 32px -16px rgb(0 0 0 / 0.6); transition: box-shadow 380ms cubic-bezier(0.4, 0, 0.2, 1); }
	/* Desktop: a small "open its page" link fades into the card's top-right corner on hover; touch uses the window's link. */
	.cl-go { position: absolute; top: 10px; right: 10px; z-index: 5; display: grid; place-items: center; width: 32px; height: 32px; border-radius: 10px; color: #fff; background: rgb(20 20 24 / 0.42); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.18), 0 0 0 1px rgb(255 255 255 / 0.14), 0 6px 16px -6px rgb(0 0 0 / 0.5); backdrop-filter: blur(14px) saturate(1.4); -webkit-backdrop-filter: blur(14px) saturate(1.4); opacity: 0; transform: translateY(-3px) scale(0.96); transition: opacity 280ms cubic-bezier(0.22, 1, 0.36, 1), transform 280ms cubic-bezier(0.22, 1, 0.36, 1), background 160ms ease; }
	.cl-go:hover { background: rgb(20 20 24 / 0.62); }
	.cl-go:focus-visible { opacity: 1; transform: none; outline: none; box-shadow: var(--focus-ring); }
	@media (hover: hover) and (pointer: fine) { .bc:hover .cl-go { opacity: 1; transform: none; } }
	@media (hover: none), (pointer: coarse) { .cl-go { display: none; } }
	@media (prefers-reduced-motion: reduce) { .cl-go { transition: opacity 120ms linear; transform: none; } }
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
	.ban::after { content: ''; position: absolute; inset: 0; background: linear-gradient(to bottom, rgb(0 0 0 / 0.22), transparent calc(var(--top) * 0.22), rgb(0 0 0 / 0.6) calc(var(--top) * 0.6), rgb(0 0 0 / 0.8) var(--top), rgb(0 0 0 / 0.4)); }
	.ban-cap { min-width: 0; display: flex; flex-direction: column; gap: 3px; color: #fff; }
	.ban-t { margin: 0; font: 800 22px/1.05 var(--font-display, var(--font-ui)); letter-spacing: -0.02em; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
	.ban-f { font: 600 12px/1.3 var(--font-ui); color: rgb(255 255 255 / 0.85); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.ap4k-tag { display: inline-block; margin-right: 6px; padding: 0 5px; border: 1px solid rgb(255 255 255 / 0.7); border-radius: 4px; background: rgb(0 0 0 / 0.35); color: #fff; font: 800 10.5px/1.5 var(--font-ui); letter-spacing: 0.04em; vertical-align: 1px; }
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

	.win { width: min(1180px, calc(100vw - 48px)); max-width: none; height: min(760px, calc(100dvh - 48px)); max-height: none; margin: auto; padding: 0; border: 0; border-radius: 24px; background: #0b0b0c; overflow: hidden; box-shadow: 0 0 0 1px rgb(255 255 255 / 0.08), 0 50px 120px -30px rgb(0 0 0 / 0.85); }
	.win[open] { animation: win-in 520ms cubic-bezier(0.22, 1, 0.36, 1) both; }
	.win::backdrop { background: rgb(0 0 0 / 0.6); -webkit-backdrop-filter: blur(8px); backdrop-filter: blur(8px); animation: fade-in 300ms var(--ease-out) both; }
	.win-in { height: 100%; }
	.win:focus { outline: none; }
	.close { position: absolute; top: 16px; right: 18px; z-index: 10; display: grid; place-items: center; width: 36px; height: 36px; border: 0; border-radius: 50%; background: rgb(20 20 24 / 0.55); color: #fff; box-shadow: inset 0 0 0 1px rgb(255 255 255 / 0.18); -webkit-backdrop-filter: blur(14px); backdrop-filter: blur(14px); cursor: pointer; transition: background 200ms var(--ease-out); }
	.close:hover { background: rgb(20 20 24 / 0.8); }
	.steps-nav { position: absolute; top: 13px; left: 18px; z-index: 10; display: flex; align-items: center; gap: 4px; padding: 3px; border-radius: 999px; background: rgb(20 20 24 / 0.55); box-shadow: inset 0 0 0 1px rgb(255 255 255 / 0.18); -webkit-backdrop-filter: blur(14px); backdrop-filter: blur(14px); color: #fff; }
	.nav { display: grid; place-items: center; width: 36px; height: 36px; border: 0; border-radius: 50%; background: none; color: #fff; cursor: pointer; transition: background 200ms var(--ease-out); }
	.nav:hover:not(:disabled) { background: rgb(255 255 255 / 0.16); }
	.nav:disabled { opacity: 0.35; cursor: default; }
	.pos { min-width: 52px; text-align: center; font: 600 12px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.win :global(.ctl.top) { right: 68px !important; }
	/* One close at a time: while the sheet is up, its own header closes it. */
	.win:has(:global(.frame.choosing)) .close { display: none; }
	.vh { position: absolute; width: 1px; height: 1px; margin: -1px; padding: 0; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; border: 0; }
	@media (pointer: coarse) { .nav, .close { width: 44px; height: 44px; } .b { height: 44px; } }
	@keyframes win-in { from { opacity: 0; transform: translate(var(--ox, 0), var(--oy, 16px)) scale(0.85); border-radius: 20px; } }
	@keyframes fade-in { from { opacity: 0; } }
	.win[open]:global(.win-out) { animation: win-out 320ms cubic-bezier(0.32, 0.72, 0, 1) both; }
	.win[open]:global(.win-out)::backdrop { animation: fade-out 320ms var(--ease-out) both; }
	@keyframes win-out { to { opacity: 0; transform: translateY(10px) scale(0.96); } }
	@keyframes fade-out { to { opacity: 0; } }
	@media (max-width: 767px) {
		.desktop { gap: 12px; padding-bottom: calc(var(--tabbar, 64px) + 16px + env(safe-area-inset-bottom, 0px)); }
		.toolbar { flex-direction: column; align-items: stretch; gap: 10px; }
		.tcap { width: 100%; }
		.tabs { width: 100%; overflow-x: auto; scrollbar-width: none; overscroll-behavior-x: contain; }
		.tabs::-webkit-scrollbar { display: none; }
		.tab { flex: none; padding: 0 12px; }
		.right { display: grid; grid-template-columns: repeat(auto-fit, minmax(0, 1fr)); gap: 10px; margin-left: 0; }
		.approve-all, .rp-btn { justify-content: center; }
		/* Three buttons share the phone row: the count is on the Pending tab already. */
		.approve-all { white-space: nowrap; }
		.aa-n { display: none; }
		.approve-all, .rp-btn { padding-inline: 10px; gap: 5px; }
		.right :global(.sf-anchor), .right :global(.sf-anchor .sf-btn) { width: 100%; }
		.cards { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
		.bc { --top: 124px; --gap: 8px; }
		.top { padding: 0 10px 8px; }
		.ban-t { font-size: 17px; }
		.ban-f { font-size: 12px; white-space: normal; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; }
		.sheet-b { border-radius: 12px; }
		.gl-n { font-size: 12.5px; gap: 4px; }
		.gl-when { font-size: 12px; white-space: normal; text-overflow: clip; }
		.body { padding: 10px 10px 0; }
		.who { gap: 7px; font: var(--t-caption); }
		.foot { gap: 6px; padding: 10px; }
		.b { padding: 0 8px; font: var(--t-footnote); font-weight: 600; }
		.b .bl { display: none; }
		.win { width: 100vw; height: 100dvh; border-radius: 0; }
		.close { top: calc(12px + env(safe-area-inset-top, 0px)); right: 12px; width: 44px; height: 44px; }
		.steps-nav { top: calc(12px + env(safe-area-inset-top, 0px)); padding: 0; }
		.win :global(.ctl.top) { right: 64px !important; }
	}
	@media (prefers-reduced-motion: reduce) {
		.bc, .fill, .win[open], .win::backdrop, .ban-img, img.ban-img, .win[open]:global(.win-out), .win[open]:global(.win-out)::backdrop { transition: none; animation: none; }
		.b:active, .approve-all:active { transform: none; }
	}

	/* The glass panel redefines --text, --text-muted and --bg so the ink reads on the blurred art. */
	.sheet-b { --blur: blur(22px) saturate(1.3); --text: light-dark(#14161c, #f5f5f7); --text-muted: light-dark(#1c2029, rgb(235 235 245 / 0.8)); --bg: light-dark(#eef0f5, #17171b); --gl-fail: light-dark(#86160b, #ff8f8a); --g1: light-dark(rgb(255 255 255 / 0.72), rgb(34 34 40 / 0.7)); --g2: light-dark(rgb(246 248 252 / 0.7), rgb(10 10 13 / 0.8)); --g3: light-dark(rgb(255 255 255 / 0.06), transparent); --edge: inset 0 1px 0 light-dark(rgb(255 255 255 / 0.75), rgb(255 255 255 / 0.16)), inset 0 0 0 1px light-dark(rgb(30 36 58 / 0.07), rgb(255 255 255 / 0.05)), inset 0 14px 22px -16px light-dark(rgb(255 255 255 / 0.6), transparent), 0 18px 34px -14px light-dark(rgb(10 14 30 / 0.32), rgb(0 0 0 / 0.8)); }
	:global(:root[data-theme='light']) .sheet-b { --blur: blur(30px) saturate(1.3) brightness(1.14); }
	@media (prefers-color-scheme: light) {
		:global(:root:not([data-theme])) .sheet-b { --blur: blur(30px) saturate(1.3) brightness(1.14); }
	}
	.b.no { box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--text) 10%, transparent); }
	.b.yes { box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.2), 0 6px 14px -8px rgb(0 0 0 / 0.6); }
	.b:focus-visible { box-shadow: var(--focus-ring); }

	/* Hover (mouse only; touch screens and reduced motion get none): the art slowly pushes in about 4% over 5s while a soft glint crosses once. It lives in the art layer, under the glass and the title. */
	.ban::before { content: ''; position: absolute; inset: 0; z-index: 1; pointer-events: none; opacity: 0; background: linear-gradient(115deg, transparent 35%, rgb(255 255 255 / 0.22) 47%, rgb(255 250 235 / 0.34) 50%, rgb(255 255 255 / 0.22) 53%, transparent 65%) no-repeat; background-size: 250% 100%; background-position: 130% 0; transition: opacity 350ms ease-out; }
	@media (hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference) {
		.bc .ban-img:not(.soft) { transition: transform 350ms cubic-bezier(0.25, 0.1, 0.25, 1), opacity 500ms var(--ease-out); }
		.bc:hover .ban-img:not(.soft) { transform: scale(1.0608); transition: transform 5s cubic-bezier(0.42, 0, 0.3, 1), opacity 500ms var(--ease-out); }
		.bc:hover .ban::before { opacity: 1; transition-duration: 500ms; animation: fx-sweep 2.4s cubic-bezier(0.4, 0, 0.2, 1) both, fx-shimmer 3.2s ease-in-out 2.4s infinite alternate; }
	}
	@keyframes fx-sweep { from { background-position: 130% 0; } to { background-position: -30% 0; } }
	@keyframes fx-shimmer { from { background-position: -30% 0; opacity: 1; } 50% { opacity: 0.35; } to { background-position: -10% 0; opacity: 0.55; } }
	/* The status row fills the same foot as Decline and Approve (40px in 12px padding: 64 desktop, 60 phone), so every tab lines up. */
	.sheet-b { --st-h: 40px; --st-r: 12px; --st-bad: light-dark(#a3170c, #ff6b63); --st-bad-ink: light-dark(#5e0e06, #ffb3ae); --st-go: light-dark(#1d5fd6, #4b8dff); --st-go-ink: light-dark(#072a6b, #b5d1ff); --st-ok: light-dark(#18843f, #34c26a); --st-ok-ink: light-dark(#073d1b, #a6efc0); --st-soft: light-dark(#5a6070, #9aa0ad); --st-soft-ink: var(--text); }
	.st-bad { --st-c: var(--st-bad); --st-ink: var(--st-bad-ink); }
	.st-go { --st-c: var(--st-go); --st-ink: var(--st-go-ink); }
	.st-ok { --st-c: var(--st-ok); --st-ink: var(--st-ok-ink); }
	.st-soft { --st-c: var(--st-soft); --st-ink: var(--st-soft-ink); }
	/* The capsule and the chip carry the status, so the thin line under answered cards steps aside on every tab. */
	.bc.st-row .life { display: none; }
	.st-i { display: inline-grid; place-items: center; flex: none; }
	.st-d { min-width: 0; font-weight: 500; }
	.st-cap { grid-column: 1 / -1; position: relative; overflow: hidden; display: flex; align-items: center; justify-content: center; gap: 7px; min-width: 0; height: var(--st-h); padding: 0 12px; border-radius: var(--st-r); background: light-dark(color-mix(in srgb, var(--st-c) 8%, rgb(255 255 255 / 0.5)), color-mix(in srgb, var(--st-c) 9%, transparent)); box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--st-c) 34%, transparent); color: var(--st-ink); font: var(--t-subhead); font-weight: 600; }
	.st-cap.st-half { grid-column: 1; }
	.st-tx { position: relative; display: flex; flex-wrap: wrap; align-items: baseline; justify-content: center; column-gap: 6px; min-width: 0; line-height: 1.15; text-align: center; }
	.st-tx > b { flex: none; font-weight: 700; }
	.st-tx > .st-d::before { content: '·'; margin-right: 6px; opacity: 0.7; }
	/* A failed reason can be long: one line, cut short, so the capsule keeps its height. */
	.st-cap.st-bad .st-tx { flex-wrap: nowrap; }
	.st-cap.st-bad .st-tx > .st-d { max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.st-cap > .st-i { position: relative; }
	.st-meter { position: absolute; inset: 0; overflow: hidden; background: light-dark(color-mix(in srgb, var(--st-c) 15%, transparent), color-mix(in srgb, var(--st-c) 30%, transparent)); transform: scaleX(var(--st-fill, 0)); transform-origin: left; transition: transform 600ms cubic-bezier(0.22, 1, 0.36, 1); }
	.st-soft .st-meter { background: light-dark(color-mix(in srgb, var(--st-c) 14%, transparent), color-mix(in srgb, var(--st-c) 22%, transparent)); }
	.st-live .st-meter::after { content: ''; position: absolute; inset: 0; background: linear-gradient(90deg, transparent, rgb(255 255 255 / 0.22), transparent) no-repeat; background-size: 40% 100%; animation: st-shim 2.2s ease-in-out infinite; }
	@keyframes st-shim { from { background-position: -60% 0; } to { background-position: 160% 0; } }
	.st-chip { grid-column: 1 / -1; display: flex; align-items: center; gap: 9px; min-width: 0; height: var(--st-h); }
	.st-sq { display: grid; place-items: center; flex: none; width: 26px; height: 26px; border-radius: 8px; background: linear-gradient(180deg, rgb(255 255 255 / 0.1), transparent 60%), color-mix(in srgb, var(--st-c) 20%, transparent); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.14), inset 0 0 0 1px color-mix(in srgb, var(--st-c) 42%, transparent); color: var(--st-ink); }
	.st-id { display: flex; flex-direction: column; gap: 1px; min-width: 0; }
	.st-id > b { color: var(--st-ink); font: 700 13.5px/1.3 var(--font-ui); }
	.st-id > .st-d { color: var(--text-muted); font: 500 12px/1.3 var(--font-ui); }
	@media (pointer: coarse) { .sheet-b { --st-h: 44px; } }
	@media (max-width: 767px) {
		.st-cap { padding: 0 8px; gap: 5px; font: 600 12px/1.15 var(--font-ui); }
		.st-cap.st-half { padding: 0 6px; gap: 0; }
		.st-cap.st-half > .st-i { display: none; }
		.st-tx { flex-direction: column; flex-wrap: nowrap; align-items: flex-start; text-align: left; }
		.st-tx > .st-d { font-size: 12px; }
		.st-tx > .st-d::before { display: none; }
		.st-cap.st-half .st-tx > .st-d { display: none; }
		.st-chip { gap: 7px; }
		.st-id > b { font-size: 12.5px; }
		.st-id > .st-d { font-size: 12px; }
	}
	@media (prefers-reduced-motion: reduce) { .st-live .st-meter::after { display: none; } .st-meter { transition: none; } }
</style>
