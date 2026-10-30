<script lang="ts">
	// Ten Approvals layouts Luis is comparing (pill bar, ?view=1…10). The plan and
	// the references behind each are in design-notes/approvals-round-3.md.
	// Every layout answers through ctx (one click, 6 s Undo), and the full ask
	// opens in a window that closes with ✕, Esc or a click outside.
	import { tick } from 'svelte';
	import { flip } from 'svelte/animate';
	import { slide } from 'svelte/transition';
	import Poster from '../Poster.svelte';
	import Icon, { type IconName } from '../Icon.svelte';
	import Avatar from '../ui/Avatar.svelte';
	import Button from '../ui/Button.svelte';
	import StatusPill from '../ui/StatusPill.svelte';
	import KidMark from './KidMark.svelte';
	import { ago, dateTime } from '../format.ts';
	import { kindWord, type ApprovalsCtx } from './approvals.ts';
	import { artistPhoto } from './details.svelte.ts';
	import { askTab, type AskTab } from '../wishlist.ts';
	import type { Ask } from '../types.ts';

	let {
		ctx,
		look,
		line,
		open
	}: {
		ctx: ApprovalsCtx;
		look: number;
		/** What the ask is for, in one short line. */
		line: (a: Ask) => string;
		/** Opens the ask in the page's window (seasons, folder, rules, note). */
		open: (a: Ask, el?: HTMLElement | null) => void;
	} = $props();

	const waiting = (a: Ask) => ctx.approver && a.state === 'asked' && !ctx.isPending(a.id);
	const art = (a: Ask) => (a.kind === 'artist' ? artistPhoto(a) : a.poster) ?? null;
	const byNewest = (l: Ask[]) => [...l].sort((x, y) => Date.parse(y.asked_at) - Date.parse(x.asked_at));
	const shown = $derived(byNewest(ctx.shown));
	const all = $derived(byNewest(ctx.viewed));
	const open_ = $derived(all.filter(waiting));
	const typeOf = (a: Ask) => (a.kind === 'show' ? 'show' : a.kind === 'artist' ? 'artist' : 'movie');
	const plural = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;
	const days = (iso: string) => Math.max(0, Math.floor((Date.now() - Date.parse(iso)) / 86_400_000));
	/** "2 days" / "today": how long the oldest waiting ask has waited. */
	const oldestWait = $derived.by(() => {
		if (!open_.length) return '';
		const d = days(open_[open_.length - 1].asked_at);
		return d === 0 ? 'today' : plural(d, 'day');
	});
	const answeredWeek = $derived(all.filter((a) => a.decided_at && Date.now() - Date.parse(a.decided_at) < 7 * 86_400_000).length);
	/** The answer in words the asker would hear. */
	const verdict = (a: Ask) => (a.state === 'turned_down' ? 'Turned down' : a.state === 'available' ? 'Ready to watch' : askTab(a) === 'failed' ? 'Needs a look' : 'Approved');

	// 1 Queue: rail filters (type, person) and one open row.
	let railType = $state<'' | 'movie' | 'show' | 'artist'>('');
	let railWho = $state(0);
	const queue = $derived(shown.filter((a) => (!railType || typeOf(a) === railType) && (!railWho || ctx.who(a).id === railWho)));
	let openRow = $state(0);

	// 2 Mod queue: its own queue rail drives the page's status tabs.
	const queues: { tab: AskTab; label: string; icon: IconName }[] = [
		{ tab: 'waiting', label: 'Waiting on you', icon: 'inbox' },
		{ tab: 'approved', label: 'On its way', icon: 'download' },
		{ tab: 'done', label: 'Ready to watch', icon: 'check' },
		{ tab: 'declined', label: 'Turned down', icon: 'declined' },
		{ tab: 'failed', label: 'Needs a look', icon: 'alert' }
	];

	// 3 Table + side sheet, 6 Poster sheet, 9 Review window: their own dialogs.
	let sheetEl: HTMLDialogElement | undefined = $state();
	let sheetId = $state(0);
	const sheetAsk = $derived(all.find((a) => a.id === sheetId) ?? null);
	let sheetFrom: HTMLElement | null = null;
	async function showSheet(a: Ask, el?: HTMLElement | null) {
		sheetId = a.id;
		sheetFrom = el ?? null;
		await tick();
		if (sheetEl && !sheetEl.open) sheetEl.showModal();
	}
	function hideSheet() {
		if (sheetEl?.open) sheetEl.close();
	}
	function sheetClosed() {
		sheetId = 0;
		if (sheetFrom?.isConnected) sheetFrom.focus({ preventScroll: true });
	}
	/** Answer in a sheet, then either close it or (review window) move to the next waiting ask. */
	function sheetAnswer(a: Ask, mode: 'approve' | 'turn_down') {
		const next = look === 9 ? shown.filter(waiting).find((x) => x.id !== a.id) : null;
		if (mode === 'approve') ctx.approve([a]);
		else ctx.turnDown([a]);
		if (next) sheetId = next.id;
		else hideSheet();
	}
	function details(a: Ask) {
		const el = document.activeElement as HTMLElement | null;
		hideSheet();
		open(a, el);
	}

	// 7 Pick and act: chosen asks, answered from the sticky bar.
	const chosen = $derived(shown.filter((a) => waiting(a) && ctx.isChosen(a.id)));

	// 8 People rows: Waiting or Answered.
	let seg = $state<'waiting' | 'answered'>('waiting');
	const segList = $derived(seg === 'waiting' ? open_ : all.filter((a) => !waiting(a)).slice(0, 40));

	// 11 Desk: the people waiting, once each, for the faces in the header.
	const waiters = $derived([...new Map(open_.map((a) => [ctx.who(a).id, ctx.who(a)])).values()]);

	// 12 Screening Room · 13 Up Next: one picked ask, the first waiting one by default.
	let pickedId = $state(0);
	const pickedAsk = $derived(shown.find((a) => a.id === pickedId) ?? shown.find(waiting) ?? shown[0] ?? null);
	function roomKeys(e: KeyboardEvent) {
		if (look !== 12 || (e.target as HTMLElement).closest('input, textarea, select, dialog') || e.metaKey || e.ctrlKey || e.altKey) return;
		const i = shown.findIndex((a) => a.id === pickedAsk?.id);
		const cur = pickedAsk;
		if (e.key === 'j' || e.key === 'k') {
			const n = shown[Math.min(Math.max(i + (e.key === 'j' ? 1 : -1), 0), shown.length - 1)];
			if (n) { pickedId = n.id; void tick().then(() => document.querySelector<HTMLElement>(`[data-sr="${n.id}"]`)?.scrollIntoView({ block: 'nearest' })); }
		} else if (e.key === 'a' && cur && waiting(cur)) ctx.approve([cur]);
		else if (e.key === 'd' && cur && waiting(cur)) ctx.turnDown([cur]);
		else return;
		e.preventDefault();
	}

	// 14 Split stacks: what needs you most, first.
	const stacks = $derived.by(() => {
		const kid = (a: Ask) => !!ctx.who(a).kid;
		const w = shown.filter(waiting);
		const kids = w.filter(kid);
		const rest = w.filter((a) => !kid(a));
		const fresh = rest.filter((a) => days(a.asked_at) < 3);
		const old = rest.filter((a) => days(a.asked_at) >= 3);
		const done = all.filter((a) => !waiting(a) && a.decided_at && Date.now() - Date.parse(a.decided_at) < 7 * 86_400_000).slice(0, 8);
		return [
			{ id: 'new', title: 'New', hint: 'Asked in the last few days.', icon: 'sparkle' as IconName, color: 'var(--accent)', asks: fresh },
			{ id: 'kids', title: 'From the kids', hint: 'Kid accounts; check the rating before you say yes.', icon: 'people' as IconName, color: 'var(--info, var(--accent))', asks: kids },
			{ id: 'old', title: 'Waiting a while', hint: 'Three days or more. Someone is still hoping.', icon: 'clock' as IconName, color: 'var(--wait)', asks: old },
			{ id: 'done', title: 'Handled this week', hint: 'Your answers from the last seven days.', icon: 'check' as IconName, color: 'var(--ok)', asks: done }
		];
	});

	// 15–19: the ask that needs you most (the oldest waiting), shelves by state,
	// triage (reason and Later), and asks by person.
	const mostNeeded = $derived(shown.filter(waiting).at(-1) ?? null);
	const shelves = $derived([
		{ id: 'waiting', title: 'Waiting on you', asks: all.filter(waiting).filter((a) => a.id !== mostNeeded?.id) },
		{ id: 'approved', title: 'On its way', asks: all.filter((a) => !waiting(a) && askTab(a) === 'approved') },
		{ id: 'done', title: 'Ready to watch', asks: all.filter((a) => askTab(a) === 'done') },
		{ id: 'declined', title: 'Turned down', asks: all.filter((a) => askTab(a) === 'declined') }
	]);
	let later = $state(new Set<number>());
	let reasonFor = $state(0);
	let reasonText = $state('');
	const triage = $derived(shown.filter((a) => !later.has(a.id)));
	const persons = $derived.by(() => {
		const by = new Map<number, Ask[]>();
		for (const a of shown) by.set(ctx.who(a).id, [...(by.get(ctx.who(a).id) ?? []), a]);
		return [...by.values()].map((asks) => ({ who: ctx.who(asks[0]), asks })).sort((x, y) => y.asks.filter(waiting).length - x.asks.filter(waiting).length);
	});

	// 10 Tracker: where each ask is on its way.
	const steps = ['Asked', 'Approved', 'Getting it', 'Ready'];
	const stepOf = (a: Ask) => (a.state === 'asked' ? 0 : a.state === 'available' ? 3 : askTab(a) === 'approved' ? 2 : 1);

	// 4 Overview: the latest answers and the most wanted.
	const lately = $derived([...all].filter((a) => a.decided_at).sort((x, y) => Date.parse(y.decided_at!) - Date.parse(x.decided_at!)).slice(0, 6));
	const wanted = $derived([...all].sort((x, y) => (y.people?.length ?? 1) - (x.people?.length ?? 1) || Date.parse(y.asked_at) - Date.parse(x.asked_at)).slice(0, 8));
</script>

{#snippet pic(a: Ask, cls: string)}
	{@const src = art(a)}
	<span class="pic {cls}">{#if a.kind === 'artist' && src}<img {src} alt="" loading="lazy" decoding="async" />{:else}<Poster src={a.kind === 'artist' ? null : src} name={a.name} />{/if}</span>
{/snippet}
{#snippet fill(a: Ask, cls: string)}
	{@const src = art(a)}
	<span class="fill {cls}" aria-hidden="true" style:background-image={src ? `url("${src}")` : undefined}></span>
{/snippet}
{#snippet who(a: Ask, size = 20, strong = true)}
	{@const w = ctx.who(a)}
	<span class="who" class:strong><Avatar name={w.name} src={w.avatar} {size} /><span class="wn">{w.name}</span>{#if w.kid}<KidMark label="Kid" size="sm" />{/if}</span>
{/snippet}
{#snippet answer(a: Ask, size: 'sm' | 'md' | 'lg' = 'md', full = false)}
	{#if waiting(a)}
		<span class="answer" class:full>
			<Button {size} variant="primary" icon="check" type="button" data-testid="ask-approve" onclick={() => ctx.approve([a])}>Approve</Button>
			<Button {size} icon="x" type="button" data-testid="ask-decline" onclick={() => ctx.turnDown([a])}>Turn down</Button>
		</span>
	{:else if ctx.isPending(a.id)}
		<span class="sending">Sending your answer…</span>
	{:else}
		<StatusPill kind={ctx.pill(a).kind} label={ctx.pill(a).label} />
	{/if}
{/snippet}
{#snippet ticks(a: Ask)}
	{#if waiting(a)}
		<span class="ticks">
			<button class="tk no" type="button" aria-label="Turn down {a.name}" data-testid="ask-decline" onclick={() => ctx.turnDown([a])}><Icon name="x" size={16} /></button>
			<button class="tk yes" type="button" aria-label="Approve {a.name}" data-testid="ask-approve" onclick={() => ctx.approve([a])}><Icon name="check" size={16} /></button>
		</span>
	{:else if ctx.isPending(a.id)}
		<span class="sending">Sending…</span>
	{:else}
		<StatusPill kind={ctx.pill(a).kind} label={ctx.pill(a).label} />
	{/if}
{/snippet}
{#snippet facts(a: Ask)}
	<dl class="facts">
		<dt>Asked by</dt><dd>{@render who(a, 20)}</dd>
		<dt>Asked</dt><dd><time datetime={a.asked_at} title={dateTime(a.asked_at)}>{ago(a.asked_at)}</time></dd>
		<dt>For</dt><dd>{a.kind === 'movie' ? 'The movie' : line(a)}</dd>
		{#if ctx.decided(a)}<dt>Answer</dt><dd>{ctx.decided(a)}</dd>{/if}
	</dl>
	{#if a.note}<p class="note">&ldquo;{a.note}&rdquo;</p>{/if}
{/snippet}
{#snippet empty(text: string)}
	<p class="empty"><Icon name="check" size={18} />{text}</p>
{/snippet}

{#if look === 1}
	<!-- 1 Queue: Downloads' own scheme for asks. -->
	<div class="q" data-look="queue">
		<div class="q-strip">
			<span><strong class="wait-n">{open_.length}</strong> waiting on you</span>
			{#if oldestWait}<span><span class="k">Oldest waits</span> {oldestWait}</span>{/if}
			<span><span class="k">Answered this week</span> {answeredWeek}</span>
		</div>
		<div class="q-bar" role="toolbar" aria-label="Answer asks">
			<button type="button" class="qb" disabled={!open_.length} onclick={() => ctx.approve(open_)}><Icon name="check" size={20} /><span>Approve all</span></button>
			<button type="button" class="qb" disabled={!open_.length} onclick={() => ctx.turnDown(open_)}><Icon name="x" size={20} /><span>Turn down all</span></button>
			<button type="button" class="qb" disabled={!open_.length} onclick={() => open(open_[open_.length - 1], document.activeElement as HTMLElement)}><Icon name="clock" size={20} /><span>Oldest first</span></button>
		</div>
		<div class="q-grid">
			<nav class="q-rail" aria-label="Narrow the queue">
				<p class="rh">Type</p>
				{#each [['', 'Everything', 'grid'], ['movie', 'Movies', 'film'], ['show', 'TV shows', 'tv'], ['artist', 'Music', 'music']] as [k, label, icon] (k)}
					<button type="button" class="rr" aria-pressed={railType === k} onclick={() => (railType = k as typeof railType)}><Icon name={icon as IconName} size={16} /><span>{label}</span><span class="rn">{k ? shown.filter((a) => typeOf(a) === k).length : shown.length}</span></button>
				{/each}
				{#if ctx.people.length > 1}
					<p class="rh">Asked by</p>
					<button type="button" class="rr" aria-pressed={!railWho} onclick={() => (railWho = 0)}><Icon name="people" size={16} /><span>Everyone</span></button>
					{#each ctx.people as p (p.id)}
						<button type="button" class="rr" aria-pressed={railWho === p.id} onclick={() => (railWho = p.id)}><Icon name="user" size={16} /><span>{p.name}</span><span class="rn">{p.n}</span></button>
					{/each}
				{/if}
			</nav>
			<div class="q-list">
				<div class="q-head" aria-hidden="true"><span></span><span></span><span>Title</span><span>Asked by</span><span>Waiting</span><span></span></div>
				<ul>
					{#each queue as a (a.id)}
						{@const on = openRow === a.id}
						<li class="q-row" class:on>
							<div class="q-line">
								<button class="caret" type="button" aria-expanded={on} aria-label="Details for {a.name}" onclick={() => (openRow = on ? 0 : a.id)}><Icon name="chevronDown" size={14} /></button>
								{@render pic(a, 'q-pic')}
								<span class="q-name"><strong>{a.name}</strong><span>{kindWord(a.kind)} · {line(a)}</span></span>
								<span class="q-who">{@render who(a, 18, false)}</span>
								<time class="q-when" datetime={a.asked_at} title={dateTime(a.asked_at)}>{ago(a.asked_at)}</time>
								<span class="q-act">{@render ticks(a)}</span>
							</div>
							{#if on}
								<div class="q-more" transition:slide={{ duration: 200 }}>
									{@render pic(a, 'q-big')}
									<div>{@render facts(a)}<div class="row-gap"><Button size="sm" variant="plain" type="button" onclick={(e: MouseEvent) => open(a, e.currentTarget as HTMLElement)}>Seasons, folder and rules</Button></div></div>
								</div>
							{/if}
						</li>
					{:else}
						<li>{@render empty('Nothing in this part of the queue.')}</li>
					{/each}
				</ul>
			</div>
		</div>
	</div>
{:else if look === 2}
	<!-- 2 Mod queue: a queue rail; each ask a post with an action strip. -->
	<div class="mq" data-look="mod-queue">
		<nav class="mq-rail" aria-label="Queues">
			{#each queues.filter((q) => ctx.approver || q.tab !== 'failed') as q (q.tab)}
				<button type="button" class="mq-q" aria-current={ctx.tab === q.tab ? 'page' : undefined} onclick={() => ctx.setTab(q.tab)}><Icon name={q.icon} size={18} /><span>{q.label}</span>{#if ctx.counts[q.tab]}<span class="rn">{ctx.counts[q.tab]}</span>{/if}</button>
			{/each}
		</nav>
		<div class="mq-feed">
			<h2 class="mq-title">{queues.find((q) => q.tab === ctx.tab)?.label}<span>{plural(shown.length, 'ask')}</span></h2>
			<ul>
				{#each shown as a (a.id)}
					<li class="post" animate:flip={{ duration: 240 }}>
						<header class="post-h">{@render who(a, 22)}<span class="dotsep">·</span><time datetime={a.asked_at} title={dateTime(a.asked_at)}>{ago(a.asked_at)}</time><span class="dotsep">·</span><span>{kindWord(a.kind)}</span></header>
						<div class="post-b">
							<div class="post-t">
								<button class="title-btn" type="button" onclick={(e) => open(a, e.currentTarget)}>{a.name}{#if a.year}<span class="yr">{a.year}</span>{/if}</button>
								{#if a.kind !== 'movie'}<p class="muted">{line(a)}</p>{/if}
								{#if a.note}<p class="note">&ldquo;{a.note}&rdquo;</p>{/if}
							</div>
							{@render pic(a, 'post-pic')}
						</div>
						<div class="strip-act">
							{#if waiting(a)}
								<Button size="sm" variant="primary" icon="check" type="button" data-testid="ask-approve" onclick={() => ctx.approve([a])}>Approve</Button>
								<Button size="sm" variant="plain" icon="x" type="button" data-testid="ask-decline" onclick={() => ctx.turnDown([a])}>Turn down</Button>
							{:else}{@render answer(a, 'sm')}{/if}
							<Button size="sm" variant="plain" type="button" onclick={(e: MouseEvent) => open(a, e.currentTarget as HTMLElement)}>Seasons and folder</Button>
						</div>
					</li>
				{:else}
					<li>{@render empty(ctx.tab === 'waiting' ? 'You’ve answered everyone.' : 'Nothing in this queue.')}</li>
				{/each}
			</ul>
		</div>
	</div>
{:else if look === 3 || look === 9}
	<!-- 3 Table + side sheet · 9 Review window: one table, two ways to decide. -->
	<div class="tbl" data-look={look === 3 ? 'table-sheet' : 'review-window'}>
		{#if look === 9 && shown.some(waiting)}
			<div class="tbl-top"><span>{plural(shown.filter(waiting).length, 'ask')} waiting on you</span><Button size="sm" variant="primary" type="button" onclick={(e: MouseEvent) => showSheet(shown.filter(waiting)[0], e.currentTarget as HTMLElement)}>Start reviewing</Button></div>
		{/if}
		<div class="tbl-head" aria-hidden="true"><span>Title</span><span>Asked by</span><span>Status</span><span>Asked</span><span></span></div>
		<ul>
			{#each shown as a (a.id)}
				<li class="tbl-row" class:cur={sheetId === a.id} animate:flip={{ duration: 220 }}>
					<button class="tbl-title" type="button" onclick={(e) => showSheet(a, e.currentTarget)}>{@render pic(a, 'tbl-pic')}<span><strong>{a.name}</strong><span>{kindWord(a.kind)}{a.kind !== 'movie' ? ` · ${line(a)}` : ''}</span></span></button>
					<span class="tbl-who">{@render who(a, 18, false)}</span>
					<span class="tbl-st"><StatusPill kind={ctx.pill(a).kind} label={ctx.pill(a).label} /></span>
					<time class="tbl-when" datetime={a.asked_at} title={dateTime(a.asked_at)}>{ago(a.asked_at)}</time>
					<span class="tbl-go">
						{#if look === 9 && waiting(a)}<Button size="sm" type="button" onclick={(e: MouseEvent) => showSheet(a, e.currentTarget as HTMLElement)}>Review</Button>
						{:else}<Icon name="chevronRight" size={16} />{/if}
					</span>
				</li>
			{:else}
				<li>{@render empty('Nothing here right now.')}</li>
			{/each}
		</ul>
	</div>
{:else if look === 4}
	<!-- 4 Overview: what waits, what happened lately, what's wanted most. -->
	<div class="ov" data-look="overview">
		<section class="card ov-wait" aria-labelledby="ov-w">
			<header class="card-h"><h2 id="ov-w">Waiting on you</h2><span class="badge">{open_.length}</span>{#if open_.length > 1}<Button size="sm" variant="plain" icon="check" type="button" onclick={() => ctx.approve(open_)}>Approve all {open_.length}</Button>{/if}</header>
			<ul>
				{#each open_.slice(0, 6) as a (a.id)}
					<li class="ov-row" animate:flip={{ duration: 220 }}>
						<button class="ov-t" type="button" onclick={(e) => open(a, e.currentTarget)}>{@render pic(a, 'ov-pic')}<span><strong>{ctx.who(a).name}</strong> wants <strong>{a.name}</strong><span class="muted">{ago(a.asked_at)} · {kindWord(a.kind)}</span></span></button>
						<span class="answer"><Button size="sm" type="button" onclick={() => ctx.turnDown([a])}>Turn down</Button><Button size="sm" variant="primary" type="button" onclick={() => ctx.approve([a])}>Approve</Button></span>
					</li>
				{:else}
					<li>{@render empty('You’ve answered everyone.')}</li>
				{/each}
			</ul>
			{#if open_.length > 6}<p class="more">{plural(open_.length - 6, 'more ask')} below the fold. Switch to Queue to see them all.</p>{/if}
		</section>
		<section class="card ov-late" aria-labelledby="ov-l">
			<header class="card-h"><h2 id="ov-l">Lately</h2></header>
			<ol class="log">
				{#each lately as a (a.id)}
					<li><span class="log-dot" class:no={a.state === 'turned_down'} aria-hidden="true"></span><span><strong>{a.name}</strong> · {verdict(a).toLowerCase()} for {ctx.who(a).name}</span><time datetime={a.decided_at} title={a.decided_at ? dateTime(a.decided_at) : ''}>{a.decided_at ? ago(a.decided_at) : ''}</time></li>
				{:else}
					<li class="muted">Answers you give show up here.</li>
				{/each}
			</ol>
		</section>
		<section class="ov-most" aria-labelledby="ov-m">
			<h2 id="ov-m">Most wanted in the house</h2>
			<ul class="posters">
				{#each wanted as a (a.id)}
					<li><button type="button" class="pst" onclick={(e) => open(a, e.currentTarget)}>{@render pic(a, 'pst-pic')}<span class="pst-n">{a.name}</span><span class="muted">{(a.people?.length ?? 1) > 1 ? `${a.people.length} people want it` : `${ctx.who(a).name} wants it`}</span></button></li>
				{/each}
			</ul>
		</section>
	</div>
{:else if look === 5}
	<!-- 5 Ask cards: the asker heads the card, their words lead. -->
	<ul class="acards" data-look="ask-cards">
		{#each shown as a (a.id)}
			<li class="ac" animate:flip={{ duration: 260 }}>
				<header class="ac-h">{@render who(a, 32)}<time datetime={a.asked_at} title={dateTime(a.asked_at)}>{ago(a.asked_at)}</time>{#if !waiting(a)}<span class="ac-st"><StatusPill kind={ctx.pill(a).kind} label={ctx.pill(a).label} /></span>{/if}</header>
				<p class="ac-say">{a.note ? `“${a.note}”` : a.kind === 'artist' ? 'Can we get this artist?' : a.kind === 'show' ? 'Can we get this show?' : 'Can we get this movie?'}</p>
				<button class="ac-t" type="button" onclick={(e) => open(a, e.currentTarget)}>{@render pic(a, 'ac-pic')}<span><strong>{a.name}</strong><span class="muted">{kindWord(a.kind)}{a.year ? ` · ${a.year}` : ''}{a.kind !== 'movie' ? ` · ${line(a)}` : ''}</span></span><Icon name="chevronRight" size={16} /></button>
				{#if waiting(a) || ctx.isPending(a.id)}<div class="ac-f">{@render answer(a, 'md', true)}</div>{/if}
			</li>
		{:else}
			<li>{@render empty('Nothing here right now.')}</li>
		{/each}
	</ul>
{:else if look === 6}
	<!-- 6 Poster sheet: Explore's posters; a sheet rises over the art to answer. -->
	<ul class="pgrid" data-look="poster-sheet">
		{#each shown as a (a.id)}
			{@const w = ctx.who(a)}
			<li animate:flip={{ duration: 260 }}>
				<button type="button" class="pg" onclick={(e) => showSheet(a, e.currentTarget)}>
					<span class="pg-art">{@render pic(a, 'pg-pic')}<span class="pg-av"><Avatar name={w.name} src={w.avatar} size={26} /></span>{#if waiting(a)}<span class="pg-new">Waiting</span>{/if}</span>
					<span class="pg-n">{a.name}</span>
					<span class="muted">{w.name} · {ago(a.asked_at)}</span>
				</button>
			</li>
		{:else}
			<li>{@render empty('Nothing here right now.')}</li>
		{/each}
	</ul>
{:else if look === 7}
	<!-- 7 Pick and act: tick circles; a sticky bar answers the chosen ones. -->
	<div class="pk" data-look="pick-act">
		<div class="pk-top">
			<span>{shown.filter(waiting).length ? `Tap the asks you want to answer` : 'Nothing waits for an answer'}</span>
			{#if shown.filter(waiting).length > 1}<button type="button" class="link" onclick={() => ctx.chooseAll(shown.filter(waiting))}>{chosen.length === shown.filter(waiting).length ? 'Clear' : 'Choose all'}</button>{/if}
		</div>
		<ul class="pk-list">
			{#each shown as a (a.id)}
				{@const can = waiting(a)}
				<li class="pk-row" class:on={ctx.isChosen(a.id)} animate:flip={{ duration: 220 }}>
					<button type="button" class="pk-hit" disabled={!can} aria-pressed={can ? ctx.isChosen(a.id) : undefined} onclick={() => ctx.toggleChosen(a.id)}>
						<span class="circle" aria-hidden="true">{#if ctx.isChosen(a.id)}<Icon name="check" size={14} />{/if}</span>
						{@render pic(a, 'pk-pic')}
						<span class="pk-t"><strong>{a.name}</strong><span class="muted">{ctx.who(a).name} · {kindWord(a.kind)} · {ago(a.asked_at)}</span></span>
					</button>
					{#if !can}<StatusPill kind={ctx.pill(a).kind} label={ctx.pill(a).label} />{:else}<button type="button" class="pk-i" aria-label="Details for {a.name}" onclick={(e) => open(a, e.currentTarget)}><Icon name="info" size={18} /></button>{/if}
				</li>
			{/each}
		</ul>
		<div class="pk-bar" class:on={chosen.length > 0} aria-live="polite">
			<button type="button" class="pk-b no" disabled={!chosen.length} onclick={() => ctx.turnDown(chosen)}><Icon name="x" size={16} />Turn down</button>
			<span class="pk-n">{chosen.length ? `${chosen.length} chosen` : 'None chosen'}</span>
			<button type="button" class="pk-b yes" disabled={!chosen.length} onclick={() => ctx.approve(chosen)}><Icon name="check" size={16} />Approve</button>
		</div>
	</div>
{:else if look === 8}
	<!-- 8 People rows: who wants what, round ✕ ✓, Approve all. -->
	<div class="pr" data-look="people-rows">
		<div class="pr-top">
			<div class="seg" role="tablist" aria-label="Show">
				<button role="tab" type="button" aria-selected={seg === 'waiting'} onclick={() => (seg = 'waiting')}>Waiting <span class="rn">{open_.length}</span></button>
				<button role="tab" type="button" aria-selected={seg === 'answered'} onclick={() => (seg = 'answered')}>Answered</button>
			</div>
			{#if seg === 'waiting' && open_.length > 1}<button type="button" class="pill-btn" onclick={() => ctx.approve(open_)}>Approve all</button>{/if}
		</div>
		<ul class="pr-list">
			{#each segList as a (a.id)}
				{@const w = ctx.who(a)}
				<li class="pr-row" animate:flip={{ duration: 220 }}>
					<Avatar name={w.name} src={w.avatar} size={44} />
					<button type="button" class="pr-t" onclick={(e) => open(a, e.currentTarget)}>
						<span class="pr-l1"><strong>{w.name}</strong>{#if w.kid}<KidMark label="Kid" size="sm" />{/if} wants</span>
						<span class="pr-l2">{a.name}<span class="muted"> · {kindWord(a.kind)} · {ago(a.asked_at)}</span></span>
					</button>
					{@render pic(a, 'pr-pic')}
					{@render ticks(a)}
				</li>
			{:else}
				<li>{@render empty(seg === 'waiting' ? 'Nobody is waiting on you.' : 'Answers you give show up here.')}</li>
			{/each}
		</ul>
	</div>
{:else if look === 11}
	<!-- 11 Desk: one list that reads the same at 1 ask and at 20. Front: poster,
	     title, who asked, when, the answer. Details open in place, lit by the art. -->
	<div class="desk" data-look="desk">
		<header class="dk-hero">
			{#if open_[0] && art(open_[0])}{@render fill(open_[0], 'dk-hero-bg')}{/if}
			<div class="dk-sum">
				<span class="dk-eyebrow">{open_.length ? 'Waiting on you' : 'All caught up'}</span>
				<p class="dk-line"><span class="dk-num">{open_.length}</span><span class="dk-words">{open_.length === 1 ? 'ask to answer' : open_.length ? 'asks to answer' : 'nobody is waiting'}{#if oldestWait}<span class="muted">The oldest has waited {oldestWait}.</span>{:else}<span class="muted">New asks land here the moment someone asks.</span>{/if}</span></p>
			</div>
			<div class="dk-side">
				{#if waiters.length}
					<span class="faces" aria-label="Asked by {waiters.map((w) => w.name).join(', ')}">
						{#each waiters.slice(0, 5) as w (w.id)}<span class="face"><Avatar name={w.name} src={w.avatar} size={34} /></span>{/each}
						{#if waiters.length > 5}<span class="face more-n">+{waiters.length - 5}</span>{/if}
					</span>
				{/if}
				<span class="muted dk-week">{answeredWeek} answered this week</span>
				{#if ctx.tab === 'waiting' && shown.filter(waiting).length > 1}
					<Button variant="primary" icon="check" type="button" onclick={() => ctx.approve(shown.filter(waiting))}>Approve all {shown.filter(waiting).length}</Button>
				{/if}
			</div>
		</header>
		<ul class="dk-list">
			{#each shown as a, i (a.id)}
				{@const on = openRow === a.id}
				{@const w = ctx.who(a)}
				<li class="dk" class:on class:answered={!waiting(a)} style="--i: {i}" animate:flip={{ duration: 300 }} out:slide={{ duration: 260 }}>
					{#if art(a)}{@render fill(a, 'dk-glow')}{/if}
					<div class="dk-row">
						<button class="dk-hit" type="button" aria-expanded={on} onclick={() => (openRow = on ? 0 : a.id)}>
							<span class="dk-art">{@render pic(a, 'dk-pic')}{#if waiting(a)}<span class="dk-dot" aria-hidden="true"></span>{/if}</span>
							<span class="dk-t">
								<span class="dk-kind">{kindWord(a.kind)}{a.year ? ` · ${a.year}` : ''}</span>
								<strong>{a.name}</strong>
								{#if a.kind !== 'movie'}<span class="muted">{line(a)}</span>{/if}
							</span>
						</button>
						<span class="dk-who"><Avatar name={w.name} src={w.avatar} size={32} /><span><strong>{w.name}{#if w.kid}<KidMark label="Kid" size="sm" />{/if}</strong><time class="muted" datetime={a.asked_at} title={dateTime(a.asked_at)}>asked {ago(a.asked_at)}</time></span></span>
						<span class="dk-act">
							{#if waiting(a)}
								<button type="button" class="dk-b no" data-testid="ask-decline" onclick={() => ctx.turnDown([a])}><Icon name="x" size={16} /><span>Turn down</span></button>
								<button type="button" class="dk-b yes" data-testid="ask-approve" onclick={() => ctx.approve([a])}><Icon name="check" size={16} /><span>Approve</span></button>
							{:else if ctx.isPending(a.id)}<span class="sending">Sending your answer…</span>
							{:else}<StatusPill kind={ctx.pill(a).kind} label={ctx.pill(a).label} />{/if}
						</span>
						<button class="caret dk-caret" type="button" aria-expanded={on} aria-label="{on ? 'Hide' : 'Show'} details for {a.name}" onclick={() => (openRow = on ? 0 : a.id)}><Icon name="chevronDown" size={16} /></button>
					</div>
					{#if on}
						<div class="dk-more" transition:slide={{ duration: 260 }}>
							<span class="dk-more-art">{@render pic(a, 'dk-big')}</span>
							<div class="dk-more-txt">
								{@render facts(a)}
								<div class="dk-more-act">
									<Button size="sm" type="button" onclick={(e: MouseEvent) => open(a, e.currentTarget as HTMLElement)}>Choose seasons, folder and rules</Button>
								</div>
							</div>
						</div>
					{/if}
				</li>
			{:else}
				<li class="dk-empty">{@render empty(ctx.tab === 'waiting' ? 'Nobody is waiting on you.' : 'Nothing here right now.')}</li>
			{/each}
		</ul>
	</div>
{:else if look === 12 || look === 13}
	{@const cur = pickedAsk}
	{#if look === 12}
		<!-- 12 Screening Room: tabs · a calm list · the picked ask large, lit by its art. J/K move, A approves, D turns down. -->
		<div class="sr" data-look="screening-room">
			<nav class="sr-tabs" aria-label="Show">
				{#each queues.filter((q) => ctx.approver || q.tab !== 'failed') as q (q.tab)}
					<button type="button" class="sr-tab" aria-current={ctx.tab === q.tab ? 'page' : undefined} onclick={() => ctx.setTab(q.tab)}><Icon name={q.icon} size={17} /><span>{q.label}</span>{#if ctx.counts[q.tab]}<span class="rn">{ctx.counts[q.tab]}</span>{/if}</button>
				{/each}
				<p class="sr-keys"><kbd>J</kbd><kbd>K</kbd> move<br /><kbd>A</kbd> approve · <kbd>D</kbd> turn down</p>
			</nav>
			<ul class="sr-list" aria-label="Asks">
				{#each shown as a (a.id)}
					<li animate:flip={{ duration: 220 }}>
						<button type="button" class="sr-row" class:cur={cur?.id === a.id} aria-current={cur?.id === a.id ? 'true' : undefined} data-sr={a.id} onclick={() => (pickedId = a.id)}>
							{@render pic(a, 'sr-pic')}
							<span class="sr-t"><strong>{a.name}</strong><span class="muted">{ctx.who(a).name} · {ago(a.asked_at)}</span></span>
							{#if waiting(a)}<span class="dk-dot sr-dot" aria-hidden="true"></span>{/if}
						</button>
					</li>
				{:else}
					<li>{@render empty('Nothing in this list.')}</li>
				{/each}
			</ul>
			<section class="sr-stage" aria-label="The picked ask">
				{#if cur}
					{#key cur.id}
						{@render fill(cur, 'sr-bg')}
						<div class="sr-in">
							<div class="sr-head">
								{@render pic(cur, 'sr-big')}
								<div class="sr-ht">
									<span class="dk-kind">{kindWord(cur.kind)}{cur.year ? ` · ${cur.year}` : ''}</span>
									<h2>{cur.name}</h2>
									{#if cur.kind !== 'movie'}<p class="muted">{line(cur)}</p>{/if}
								</div>
							</div>
							<div class="glass">{@render facts(cur)}</div>
							<div class="sr-act">
								{#if waiting(cur)}
									<button type="button" class="dk-b yes" onclick={() => ctx.approve([cur])}><Icon name="check" size={16} /><span>Approve</span></button>
									<button type="button" class="dk-b no" onclick={() => ctx.turnDown([cur])}><Icon name="x" size={16} /><span>Turn down</span></button>
								{:else}{@render answer(cur, 'md')}{/if}
								<Button size="md" variant="plain" type="button" onclick={(e: MouseEvent) => open(cur, e.currentTarget as HTMLElement)}>Seasons, folder and rules</Button>
							</div>
						</div>
					{/key}
				{:else}
					<div class="sr-zero"><Icon name="check" size={28} /><h2>All caught up</h2><p class="muted">New asks land here the moment someone asks.</p></div>
				{/if}
			</section>
		</div>
	{:else}
		<!-- 13 Up Next: a cinematic hero for the picked ask, a shelf of every ask below. -->
		<div class="un" data-look="up-next">
			{#if cur}
				<section class="un-hero" aria-label="The picked ask">
					{#key cur.id}
						{@render fill(cur, 'un-bg')}
						<div class="un-in">
							{@render pic(cur, 'un-pic')}
							<div class="un-t">
								<span class="dk-eyebrow">{waiting(cur) ? 'Up next' : verdict(cur)}</span>
								<h2>{cur.name}</h2>
								<p class="un-meta">{kindWord(cur.kind)}{cur.year ? ` · ${cur.year}` : ''}{cur.kind !== 'movie' ? ` · ${line(cur)}` : ''}</p>
								<p class="un-who">{@render who(cur, 28)}<span class="muted">asked {ago(cur.asked_at)}</span></p>
								{#if cur.note}<p class="un-note">&ldquo;{cur.note}&rdquo;</p>{/if}
								<div class="sr-act">
									{#if waiting(cur)}
										<button type="button" class="dk-b yes big-b" onclick={() => ctx.approve([cur])}><Icon name="check" size={18} /><span>Approve</span></button>
										<button type="button" class="dk-b no big-b glassy" onclick={() => ctx.turnDown([cur])}><Icon name="x" size={18} /><span>Turn down</span></button>
									{:else}{@render answer(cur, 'md')}{/if}
									<button type="button" class="dk-b no big-b glassy" onclick={(e) => open(cur, e.currentTarget)}><Icon name="info" size={18} /><span>Details</span></button>
								</div>
							</div>
						</div>
					{/key}
				</section>
				<h3 class="un-h">{ctx.tab === 'waiting' ? `Waiting on you · ${shown.length}` : `${shown.length} in this list`}</h3>
				<ul class="un-shelf" aria-label="Asks">
					{#each shown as a (a.id)}
						<li animate:flip={{ duration: 260 }}>
							<button type="button" class="un-card" class:cur={cur.id === a.id} aria-current={cur.id === a.id ? 'true' : undefined} onclick={() => (pickedId = a.id)}>
								<span class="un-art">{@render pic(a, 'un-cpic')}<span class="pg-av"><Avatar name={ctx.who(a).name} src={ctx.who(a).avatar} size={24} /></span></span>
								<span class="pg-n">{a.name}</span>
								<span class="muted">{ctx.who(a).name} · {ago(a.asked_at)}</span>
							</button>
						</li>
					{/each}
				</ul>
			{:else}
				<section class="un-zero">
					<div class="collage" aria-hidden="true">{#each all.filter((a) => a.state !== 'asked' && art(a)).slice(0, 12) as a (a.id)}{@render pic(a, 'col-pic')}{/each}</div>
					<div class="un-zt"><h2>All caught up</h2><p class="muted">Everything the house asked for has an answer. Here’s what you said yes to.</p></div>
				</section>
			{/if}
		</div>
	{/if}
{:else if look === 14}
	<!-- 14 Split stacks: the queue split by what needs you most. -->
	<div class="ss" data-look="split-stacks">
		{#each stacks as st (st.id)}
			{#if st.asks.length}
				<section class="ss-sec" aria-labelledby="ss-{st.id}">
					<header class="ss-h">
						<span class="ss-ic" style="--c: {st.color}"><Icon name={st.icon} size={16} /></span>
						<div><h2 id="ss-{st.id}">{st.title}<span class="rn">{st.asks.length}</span></h2><p class="muted">{st.hint}</p></div>
						{#if st.id !== 'done' && st.asks.filter(waiting).length > 1}<Button size="sm" variant="plain" icon="check" type="button" onclick={() => ctx.approve(st.asks.filter(waiting))}>Approve all {st.asks.filter(waiting).length}</Button>{/if}
					</header>
					<ul class="ss-list">
						{#each st.asks as a (a.id)}
							{@const w = ctx.who(a)}
							<li class="ss-row" class:answered={!waiting(a)} animate:flip={{ duration: 240 }}>
								{#if art(a)}{@render fill(a, 'dk-glow')}{/if}
								<button type="button" class="dk-hit" onclick={(e) => open(a, e.currentTarget)}>
									{@render pic(a, 'ss-pic')}
									<span class="dk-t"><span class="dk-kind">{kindWord(a.kind)}{a.year ? ` · ${a.year}` : ''}</span><strong>{a.name}</strong>{#if a.kind !== 'movie'}<span class="muted">{line(a)}</span>{/if}</span>
								</button>
								<span class="dk-who"><Avatar name={w.name} src={w.avatar} size={28} /><span><strong>{w.name}{#if w.kid}<KidMark label="Kid" size="sm" />{/if}</strong><time class="muted" datetime={a.asked_at}>asked {ago(a.asked_at)}</time></span></span>
								<span class="dk-act">
									{#if waiting(a)}
										<button type="button" class="dk-b no" onclick={() => ctx.turnDown([a])}><Icon name="x" size={16} /><span>Turn down</span></button>
										<button type="button" class="dk-b yes" onclick={() => ctx.approve([a])}><Icon name="check" size={16} /><span>Approve</span></button>
									{:else}<span class="ss-done">{verdict(a)}{#if a.decided_at} · {ago(a.decided_at)}{/if}</span>{/if}
								</span>
							</li>
						{/each}
					</ul>
				</section>
			{/if}
		{/each}
		{#if !stacks.some((s) => s.asks.length)}{@render empty('Nothing here right now.')}{/if}
	</div>
{:else if look >= 15 && look <= 19}
	<!-- 15–19: the best of what we learned. Desk rows (liked), QuickBox's banner and
	     shelves, Linear's reason and "later", HEY's per-person answer, Downloads' side panel. -->
	{#snippet banner(a: Ask)}
		{@const w = ctx.who(a)}
		<section class="bn" aria-label="Needs you most">
			{#key a.id}
				{@render fill(a, 'bn-bg')}
				<div class="bn-in">
					<button type="button" class="bn-art" aria-label="Open {a.name}" onclick={(e) => open(a, e.currentTarget)}>{@render pic(a, 'bn-pic')}</button>
					<div class="bn-t">
						<span class="dk-eyebrow">Needs you most · waiting {days(a.asked_at) ? plural(days(a.asked_at), 'day') : 'since today'}</span>
						<h2>{a.name}</h2>
						<p class="bn-meta">{kindWord(a.kind)}{a.year ? ` · ${a.year}` : ''}{a.kind !== 'movie' ? ` · ${line(a)}` : ''}</p>
						<p class="bn-who"><Avatar name={w.name} src={w.avatar} size={26} /><span><strong>{w.name}</strong> asked {ago(a.asked_at)}</span>{#if w.kid}<KidMark label="Kid" size="sm" />{/if}</p>
						{#if a.note}<p class="bn-note">&ldquo;{a.note}&rdquo;</p>{/if}
						<div class="sr-act">
							<button type="button" class="dk-b yes big-b" onclick={() => ctx.approve([a])}><Icon name="check" size={18} /><span>Approve</span></button>
							<button type="button" class="dk-b no big-b glassy" onclick={() => ctx.turnDown([a])}><Icon name="x" size={18} /><span>Turn down</span></button>
							<button type="button" class="dk-b no big-b glassy" onclick={(e) => open(a, e.currentTarget)}><Icon name="info" size={18} /><span>Details</span></button>
						</div>
					</div>
				</div>
			{/key}
		</section>
	{/snippet}
	{#snippet row(a: Ask, i: number, opts: { who?: boolean; later?: boolean; reason?: boolean; pick?: boolean } = {})}
		{@const w = ctx.who(a)}
		{@const on = openRow === a.id}
		<li class="dk" class:on class:answered={!waiting(a)} class:cur={opts.pick && pickedAsk?.id === a.id} style="--i: {i}">
			{#if art(a)}{@render fill(a, 'dk-glow')}{/if}
			<div class="dk-row" class:nowho={opts.who === false}>
				<button class="dk-hit" type="button" aria-expanded={opts.pick ? undefined : on} onclick={() => (opts.pick ? (pickedId = a.id) : (openRow = on ? 0 : a.id))}>
					<span class="dk-art">{@render pic(a, 'dk-pic')}{#if waiting(a)}<span class="dk-dot" aria-hidden="true"></span>{/if}</span>
					<span class="dk-t"><span class="dk-kind">{kindWord(a.kind)}{a.year ? ` · ${a.year}` : ''}</span><strong>{a.name}</strong>{#if a.kind !== 'movie'}<span class="muted">{line(a)}</span>{/if}</span>
				</button>
				{#if opts.who !== false}<span class="dk-who"><Avatar name={w.name} src={w.avatar} size={30} /><span><strong>{w.name}{#if w.kid}<KidMark label="Kid" size="sm" />{/if}</strong><time class="muted" datetime={a.asked_at} title={dateTime(a.asked_at)}>asked {ago(a.asked_at)}</time></span></span>{/if}
				<span class="dk-act">
					{#if waiting(a)}
						{#if opts.later}<button type="button" class="dk-b no ghost" title="Hide it until you come back" onclick={() => (later = new Set(later).add(a.id))}><Icon name="clock" size={16} /><span>Later</span></button>{/if}
						<button type="button" class="dk-b no" data-testid="ask-decline" onclick={() => (opts.reason ? (reasonFor = reasonFor === a.id ? 0 : a.id) : ctx.turnDown([a]))}><Icon name="x" size={16} /><span>Turn down</span></button>
						<button type="button" class="dk-b yes" data-testid="ask-approve" onclick={() => ctx.approve([a])}><Icon name="check" size={16} /><span>Approve</span></button>
					{:else if ctx.isPending(a.id)}<span class="sending">Sending your answer…</span>
					{:else}<StatusPill kind={ctx.pill(a).kind} label={ctx.pill(a).label} />{/if}
				</span>
				{#if !opts.pick}<button class="caret dk-caret" type="button" aria-expanded={on} aria-label="{on ? 'Hide' : 'Show'} details for {a.name}" onclick={() => (openRow = on ? 0 : a.id)}><Icon name="chevronDown" size={16} /></button>{/if}
			</div>
			{#if opts.reason && reasonFor === a.id && waiting(a)}
				<form class="why" transition:slide={{ duration: 200 }} onsubmit={(e) => { e.preventDefault(); ctx.turnDownWith(a, reasonText.trim()); reasonFor = 0; reasonText = ''; }}>
					<label for="why-{a.id}">Tell {w.name} why <span class="muted">(optional, they’ll see it)</span></label>
					<div class="why-row">
						<input id="why-{a.id}" class="input" bind:value={reasonText} maxlength="300" placeholder="We already have it on another service" />
						<button type="submit" class="dk-b no">Turn down</button>
					</div>
					<div class="why-chips">{#each ['Already have it', 'Not for kids', 'Not out yet', 'Too big right now'] as c (c)}<button type="button" class="chip-s" onclick={() => (reasonText = c)}>{c}</button>{/each}</div>
				</form>
			{/if}
			{#if !opts.pick && on}
				<div class="dk-more" transition:slide={{ duration: 260 }}>
					<span class="dk-more-art">{@render pic(a, 'dk-big')}</span>
					<div class="dk-more-txt">{@render facts(a)}<div class="dk-more-act"><Button size="sm" type="button" onclick={(e: MouseEvent) => open(a, e.currentTarget as HTMLElement)}>Choose seasons, folder and rules</Button></div></div>
				</div>
			{/if}
		</li>
	{/snippet}

	{#if look === 15}
		<!-- 15 Banner + Desk: the one that needs you most up top, the rest as Desk rows. -->
		<div class="desk" data-look="banner-desk">
			{#if mostNeeded}{@render banner(mostNeeded)}{:else}{@render empty('You’re all caught up. New asks land here the moment someone asks.')}{/if}
			{#if shown.filter((a) => a.id !== mostNeeded?.id).length}
				<h3 class="un-h">{ctx.tab === 'waiting' ? 'Also waiting' : 'In this list'}</h3>
				<ul class="dk-list">{#each shown.filter((a) => a.id !== mostNeeded?.id) as a, i (a.id)}{@render row(a, i)}{/each}</ul>
			{/if}
		</div>
	{:else if look === 16}
		<!-- 16 Banner + shelves by state (QuickBox): poster shelves you scroll sideways. -->
		<div class="desk wide" data-look="banner-shelves">
			{#if mostNeeded}{@render banner(mostNeeded)}{/if}
			{#each shelves as sh (sh.id)}
				{#if sh.asks.length}
					<section class="shelf-s" aria-labelledby="sh-{sh.id}">
						<h3 class="un-h" id="sh-{sh.id}">{sh.title}<span class="rn">{sh.asks.length}</span></h3>
						<ul class="un-shelf">
							{#each sh.asks as a (a.id)}
								<li animate:flip={{ duration: 260 }}>
									<div class="un-card">
										<button type="button" class="un-art" aria-label="Open {a.name}" onclick={(e) => open(a, e.currentTarget)}>{@render pic(a, 'un-cpic')}<span class="pg-av"><Avatar name={ctx.who(a).name} src={ctx.who(a).avatar} size={24} /></span></button>
										<span class="pg-n">{a.name}</span>
										<span class="muted">{ctx.who(a).name} · {ago(a.asked_at)}</span>
										{#if waiting(a)}
											<span class="shelf-act">
												<button type="button" class="tk no" aria-label="Turn down {a.name}" onclick={() => ctx.turnDown([a])}><Icon name="x" size={15} /></button>
												<button type="button" class="tk yes" aria-label="Approve {a.name}" onclick={() => ctx.approve([a])}><Icon name="check" size={15} /></button>
											</span>
										{/if}
									</div>
								</li>
							{/each}
						</ul>
					</section>
				{/if}
			{/each}
		</div>
	{:else if look === 17}
		<!-- 17 Triage: Desk rows with a reason on turn-down and "Later" (Linear). -->
		<div class="desk" data-look="triage">
			<header class="tri-top">
				<div><h2>{triage.length ? `${plural(triage.length, 'ask')} to triage` : later.size ? 'Only the ones you set aside are left' : 'Inbox zero'}</h2><p class="muted">Approve, turn down with a reason, or set it aside for later.</p></div>
				{#if later.size}<button type="button" class="pill-btn" onclick={() => (later = new Set())}>Show {plural(later.size, 'ask')} set aside</button>{/if}
			</header>
			<ul class="dk-list">{#each triage as a, i (a.id)}{@render row(a, i, { later: true, reason: true })}{:else}<li class="dk-empty">{@render empty('Nothing left to triage.')}</li>{/each}</ul>
		</div>
	{:else if look === 18}
		<!-- 18 By person (HEY): answer per person, or one ask at a time. -->
		<div class="desk" data-look="by-person">
			{#each persons as p (p.who.id)}
				<section class="per" aria-labelledby="pe-{p.who.id}">
					<header class="per-h">
						<Avatar name={p.who.name} src={p.who.avatar} size={44} />
						<div><h2 id="pe-{p.who.id}">{p.who.name}{#if p.who.kid}<KidMark label="Kid" size="sm" />{/if}</h2><p class="muted">{p.asks.filter(waiting).length ? `${plural(p.asks.filter(waiting).length, 'ask')} waiting · last asked ${ago(p.asks[0].asked_at)}` : 'Nothing waiting'}</p></div>
						{#if p.asks.filter(waiting).length > 1}<button type="button" class="dk-b yes" onclick={() => ctx.approve(p.asks.filter(waiting))}><Icon name="check" size={16} /><span>Approve all from {p.who.name}</span></button>{/if}
					</header>
					<ul class="dk-list">{#each p.asks as a, i (a.id)}{@render row(a, i, { who: false })}{/each}</ul>
				</section>
			{:else}
				{@render empty('Nothing here right now.')}
			{/each}
		</div>
	{:else if look === 19}
		<!-- 19 Desk + panel: Desk rows on the left, the picked ask large on the right (Downloads' side panel). -->
		<div class="dp" data-look="desk-panel">
			<ul class="dk-list">{#each shown as a, i (a.id)}{@render row(a, i, { pick: true, who: true })}{:else}<li class="dk-empty">{@render empty('Nothing here right now.')}</li>{/each}</ul>
			<aside class="sr-stage dp-panel" aria-label="The picked ask">
				{#if pickedAsk}
					{@const cur = pickedAsk}
					{#key cur.id}
						{@render fill(cur, 'sr-bg')}
						<div class="sr-in">
							<div class="sr-head">{@render pic(cur, 'sr-big')}<div class="sr-ht"><span class="dk-kind">{kindWord(cur.kind)}{cur.year ? ` · ${cur.year}` : ''}</span><h2>{cur.name}</h2>{#if cur.kind !== 'movie'}<p class="muted">{line(cur)}</p>{/if}</div></div>
							<div class="glass">{@render facts(cur)}</div>
							<div class="sr-act">
								{#if waiting(cur)}
									<button type="button" class="dk-b yes" onclick={() => ctx.approve([cur])}><Icon name="check" size={16} /><span>Approve</span></button>
									<button type="button" class="dk-b no" onclick={() => ctx.turnDown([cur])}><Icon name="x" size={16} /><span>Turn down</span></button>
								{:else}{@render answer(cur, 'md')}{/if}
								<Button size="md" variant="plain" type="button" onclick={(e: MouseEvent) => open(cur, e.currentTarget as HTMLElement)}>Seasons, folder and rules</Button>
							</div>
						</div>
					{/key}
				{:else}
					<div class="sr-zero"><Icon name="check" size={28} /><h2>All caught up</h2></div>
				{/if}
			</aside>
		</div>
	{/if}
{:else if look === 10}
	<!-- 10 Tracker: every ask shows where it is on its way. -->
	<ul class="trk" data-look="tracker">
		{#each shown as a (a.id)}
			{@const s = stepOf(a)}
			{@const no = a.state === 'turned_down'}
			<li class="tr" animate:flip={{ duration: 240 }}>
				<button class="tr-t" type="button" onclick={(e) => open(a, e.currentTarget)}>{@render pic(a, 'tr-pic')}<span><strong>{a.name}</strong><span class="muted">{kindWord(a.kind)} · for {ctx.who(a).name} · asked {ago(a.asked_at)}</span></span></button>
				<ol class="steps" class:no aria-label="Where {a.name} is">
					{#each steps as label, i (label)}
						<li class:done={!no && i <= s} class:now={!no && i === s} aria-current={!no && i === s ? 'step' : undefined}><span class="sd" aria-hidden="true"></span><span class="sl">{no && i === 1 ? 'Turned down' : label}</span></li>
					{/each}
				</ol>
				<div class="tr-act">{#if waiting(a) || ctx.isPending(a.id)}{@render answer(a, 'sm')}{:else}<span class="muted">{ctx.decided(a)}</span>{/if}</div>
			</li>
		{:else}
			<li>{@render empty('Nothing here right now.')}</li>
		{/each}
	</ul>
{/if}

<svelte:window onkeydown={roomKeys} />

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
<dialog class="sheet" class:side={look === 3} class:rise={look === 6} class:review={look === 9} bind:this={sheetEl} aria-label={sheetAsk?.name ?? 'Ask'} onclose={sheetClosed} onclick={(e) => e.target === sheetEl && hideSheet()}>
	{#if sheetAsk}
		{@const a = sheetAsk}
		{#if look === 6}{@render fill(a, 'sh-bg')}{/if}
		<div class="sh-in">
			<header class="sh-h">
				<span class="sh-k">{look === 9 ? `${Math.max(shown.filter(waiting).findIndex((x) => x.id === a.id) + 1, 1)} of ${Math.max(shown.filter(waiting).length, 1)} waiting` : kindWord(a.kind)}</span>
				<button class="sh-x" type="button" aria-label="Close" onclick={hideSheet}><Icon name="x" size={18} /></button>
			</header>
			<div class="sh-body">
				<div class="sh-top">{@render pic(a, 'sh-pic')}<div><h2 class="sh-t">{a.name}{#if a.year}<span class="yr">{a.year}</span>{/if}</h2><p class="muted">{kindWord(a.kind)}{a.kind !== 'movie' ? ` · ${line(a)}` : ''}</p></div></div>
				{@render facts(a)}
				<button type="button" class="link sh-more" onclick={() => details(a)}>Choose seasons, folder and rules</button>
			</div>
			<footer class="sh-f">
				{#if waiting(a)}
					<button type="button" class="big no" onclick={() => sheetAnswer(a, 'turn_down')}><Icon name="x" size={18} />Turn down</button>
					<button type="button" class="big yes" onclick={() => sheetAnswer(a, 'approve')}><Icon name="check" size={18} />Approve</button>
				{:else}
					<span class="sh-done">{verdict(a)}{#if ctx.decided(a)} · {ctx.decided(a)}{/if}</span>
				{/if}
			</footer>
		</div>
	{/if}
</dialog>

<style>
	/* Shared: one picture frame, one answer pair, one note, one empty line. */
	button { font: inherit; color: inherit; }
	button:focus-visible, .rr:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	ul, ol { margin: 0; padding: 0; list-style: none; }
	h2 { font: var(--t-headline); }
	.muted { color: var(--text-muted); }
	.pic { display: block; flex: none; aspect-ratio: 2 / 3; --poster-ratio: 2 / 3; overflow: hidden; border-radius: 6px; background: var(--surface-3); }
	.pic > img { width: 100%; height: 100%; object-fit: cover; object-position: center 25%; display: block; }
	.fill { display: block; background-size: cover; background-position: center 25%; background-color: var(--surface-3); }
	.who { display: inline-flex; align-items: center; gap: 6px; min-width: 0; }
	.who.strong .wn { font-weight: 600; }
	.wn { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.answer { display: inline-flex; flex-wrap: wrap; gap: 8px; }
	.answer.full { display: grid; grid-template-columns: 1fr 1fr; width: 100%; }
	.sending { color: var(--text-muted); font: var(--t-footnote); }
	.note { margin-top: 8px; padding: 8px 12px; border-radius: 10px; background: var(--surface-2); font: var(--t-footnote); font-style: italic; overflow-wrap: anywhere; }
	.empty { display: flex; align-items: center; justify-content: center; gap: 8px; padding: 28px 16px; color: var(--text-muted); font: var(--t-callout); }
	.rn { min-width: 20px; margin-left: auto; padding: 0 6px; border-radius: var(--r-pill); background: var(--surface-3); color: var(--text-muted); font: var(--t-caption); font-weight: 600; text-align: center; font-variant-numeric: tabular-nums; }
	.yr { margin-left: 8px; color: var(--text-muted); font-weight: 400; }
	.link { padding: 0; border: 0; background: none; color: var(--accent-text); font: var(--t-subhead); font-weight: 600; cursor: pointer; border-radius: 4px; }
	.facts { display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: 8px 16px; margin: 0; font: var(--t-callout); }
	.facts dt { color: var(--text-muted); }
	.facts dd { margin: 0; min-width: 0; }
	.ticks { display: inline-flex; gap: 8px; }
	.tk { display: grid; place-items: center; width: 32px; height: 32px; border: 0; border-radius: 50%; cursor: pointer; transition: transform var(--dur-fast) var(--ease-out), background var(--dur-fast) var(--ease-out); }
	.tk:active { transform: scale(0.92); }
	.tk.no { background: var(--surface-3); color: var(--text); }
	.tk.no:hover { background: color-mix(in srgb, var(--accent) 18%, var(--surface-3)); }
	.tk.yes { background: var(--ok); color: var(--ok-on-art, #fff); }
	.tk.yes:hover { background: var(--ok-fill-hover, var(--ok)); }
	.card { border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); overflow: hidden; }
	.card-h { display: flex; align-items: center; gap: 8px; min-height: 52px; padding: 0 12px 0 16px; border-bottom: 1px solid var(--line); }
	.card-h h2 { margin-right: auto; }
	.badge { order: -1; display: none; }

	/* 1 Queue */
	.q-strip { display: flex; flex-wrap: wrap; align-items: baseline; gap: 6px 22px; padding: 8px 0; margin-bottom: 10px; border-bottom: 1px solid var(--line); font: var(--t-callout); font-variant-numeric: tabular-nums; }
	.k { color: var(--text-muted); font: var(--t-footnote); }
	.wait-n { color: var(--wait-text, var(--text)); font-size: 1.15em; }
	.q-bar { display: flex; gap: 4px; margin-bottom: 12px; }
	.qb { display: flex; flex-direction: column; align-items: center; gap: 3px; min-width: 76px; padding: 6px 10px; border: 0; border-radius: var(--r-btn); background: none; color: var(--text); font: var(--t-caption); font-weight: 500; cursor: pointer; transition: background var(--dur-fast) var(--ease-out); }
	.qb:hover:not(:disabled) { background: var(--surface-2); }
	.qb:disabled { opacity: 0.4; cursor: default; }
	.q-grid { display: grid; grid-template-columns: 196px minmax(0, 1fr); gap: 16px; align-items: start; }
	.q-rail { position: sticky; top: calc(var(--topbar, 56px) + 12px); display: flex; flex-direction: column; gap: 1px; }
	.rh { margin: 12px 8px 4px; color: var(--text-muted); font: var(--t-caption); font-weight: 600; }
	.rh:first-child { margin-top: 0; }
	.rr { display: flex; align-items: center; gap: 10px; min-height: 32px; padding: 0 8px; border: 0; border-radius: 6px; background: none; color: var(--text); font: var(--t-footnote); text-align: left; cursor: pointer; }
	.rr:hover { background: var(--surface-2); }
	.rr[aria-pressed='true'] { background: var(--surface-3); font-weight: 600; }
	.rr span:nth-child(2) { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.q-head, .q-line { display: grid; grid-template-columns: 24px 26px minmax(0, 2.2fr) minmax(0, 1fr) 84px 80px; align-items: center; column-gap: 10px; }
	.q-head { height: 28px; border-bottom: 1px solid var(--line); color: var(--text-muted); font: var(--t-caption); font-weight: 600; }
	.q-row { border-bottom: 1px solid var(--line); }
	.q-line { height: 48px; }
	.q-row:hover .q-line, .q-row.on .q-line { background: color-mix(in srgb, var(--surface-2) 70%, transparent); }
	.caret { display: grid; place-items: center; width: 24px; height: 24px; padding: 0; border: 0; border-radius: 5px; background: none; color: var(--text-muted); cursor: pointer; transition: transform var(--dur-fast) var(--ease-out); transform: rotate(-90deg); }
	.on .caret { transform: none; color: var(--text); }
	.q-pic { width: 26px; border-radius: 3px; }
	.q-name { display: flex; flex-direction: column; min-width: 0; line-height: 1.25; }
	.q-name strong { font: var(--t-callout); font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.q-name span { color: var(--text-muted); font: var(--t-caption); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.q-who { font: var(--t-footnote); min-width: 0; }
	.q-when { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.q-act { display: flex; justify-content: flex-end; }
	.q-act .tk { width: 28px; height: 28px; }
	.q-more { display: grid; grid-template-columns: 72px minmax(0, 1fr); gap: 16px; padding: 10px 12px 16px 34px; }
	.q-big { width: 72px; border-radius: 6px; box-shadow: 0 4px 14px rgb(0 0 0 / 0.18); }
	.row-gap { margin-top: 10px; }

	/* 2 Mod queue */
	.mq { display: grid; grid-template-columns: 220px minmax(0, 1fr); gap: 24px; align-items: start; }
	.mq-rail { position: sticky; top: calc(var(--topbar, 56px) + 12px); display: flex; flex-direction: column; gap: 2px; }
	.mq-q { position: relative; display: flex; align-items: center; gap: 10px; min-height: 40px; padding: 0 12px; border: 0; border-radius: 10px; background: none; color: var(--text-muted); font: var(--t-subhead); text-align: left; cursor: pointer; }
	.mq-q:hover { background: var(--surface-2); color: var(--text); }
	.mq-q[aria-current='page'] { background: var(--surface-2); color: var(--text); font-weight: 600; }
	.mq-q[aria-current='page']::before { content: ''; position: absolute; left: 0; top: 8px; bottom: 8px; width: 3px; border-radius: 0 3px 3px 0; background: var(--accent); }
	.mq-title { display: flex; align-items: baseline; gap: 10px; margin-bottom: 10px; font: var(--t-title2); }
	.mq-title span { color: var(--text-muted); font: var(--t-footnote); }
	.post { padding: 14px 0 12px; border-top: 1px solid var(--line); }
	.post-h { display: flex; align-items: center; gap: 6px; color: var(--text-muted); font: var(--t-footnote); }
	.dotsep { opacity: 0.6; }
	.post-b { display: flex; gap: 16px; margin-top: 6px; }
	.post-t { flex: 1; min-width: 0; }
	.title-btn { padding: 0; border: 0; background: none; font: var(--t-title3); text-align: left; cursor: pointer; overflow-wrap: anywhere; border-radius: 4px; }
	.title-btn:hover { text-decoration: underline; text-underline-offset: 4px; text-decoration-color: color-mix(in srgb, currentColor 35%, transparent); }
	.post-pic { width: 64px; }
	.strip-act { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; margin-top: 10px; }

	/* 3 Table · 9 Review */
	.tbl { border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); overflow: hidden; }
	.tbl-top { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 16px; border-bottom: 1px solid var(--line); background: color-mix(in srgb, var(--wait) 10%, transparent); font: var(--t-subhead); font-weight: 600; }
	.tbl-head, .tbl-row { display: grid; grid-template-columns: minmax(0, 2.4fr) minmax(0, 1.1fr) 150px 96px 90px; align-items: center; gap: 12px; padding: 0 16px; }
	.tbl-head { height: 34px; border-bottom: 1px solid var(--line); background: var(--surface-2); color: var(--text-muted); font: var(--t-caption); font-weight: 600; letter-spacing: 0.04em; text-transform: uppercase; }
	.tbl-row { min-height: 56px; transition: background var(--dur-fast) var(--ease-out); }
	.tbl-row + .tbl-row { border-top: 1px solid var(--line); }
	.tbl-row:hover, .tbl-row.cur { background: var(--surface-2); }
	.tbl-title { display: flex; align-items: center; gap: 12px; min-width: 0; padding: 6px 0; border: 0; background: none; text-align: left; cursor: pointer; border-radius: 6px; }
	.tbl-title::after { content: ''; position: absolute; inset: 0; }
	.tbl-row { position: relative; }
	.tbl-title > span { display: flex; flex-direction: column; min-width: 0; }
	.tbl-title strong { font: var(--t-subhead); font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.tbl-title > span > span { color: var(--text-muted); font: var(--t-caption); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.tbl-pic { width: 30px; border-radius: 4px; }
	.tbl-who { font: var(--t-footnote); min-width: 0; }
	.tbl-when { color: var(--text-muted); font: var(--t-footnote); }
	.tbl-go { position: relative; z-index: 1; display: flex; justify-content: flex-end; color: var(--text-muted); }

	/* 4 Overview */
	.ov { display: grid; grid-template-columns: minmax(0, 1.5fr) minmax(0, 1fr); gap: 16px; align-items: start; }
	.ov-row { display: flex; align-items: center; gap: 12px; padding: 10px 12px 10px 16px; }
	.ov-row + .ov-row { border-top: 1px solid var(--line); }
	.ov-t { flex: 1; min-width: 0; display: flex; align-items: center; gap: 12px; padding: 0; border: 0; background: none; text-align: left; cursor: pointer; font: var(--t-callout); }
	.ov-t > span { display: flex; flex-direction: column; min-width: 0; }
	.ov-t .muted { font: var(--t-caption); }
	.ov-pic { width: 36px; }
	.more { padding: 10px 16px; border-top: 1px solid var(--line); color: var(--text-muted); font: var(--t-footnote); }
	.log { padding: 8px 16px 12px; }
	.log li { display: grid; grid-template-columns: 10px minmax(0, 1fr) auto; align-items: baseline; gap: 10px; padding: 8px 0; font: var(--t-footnote); }
	.log li + li { border-top: 1px solid var(--line); }
	.log time { color: var(--text-muted); font: var(--t-caption); }
	.log-dot { width: 8px; height: 8px; border-radius: 50%; background: var(--ok); }
	.log-dot.no { background: var(--text-muted); }
	.ov-most { grid-column: 1 / -1; margin-top: 8px; }
	.ov-most h2 { margin: 0 4px 10px; font: var(--t-title3); }
	.posters { display: flex; gap: 14px; overflow-x: auto; padding-bottom: 6px; scroll-snap-type: x proximity; scrollbar-width: none; }
	.posters li { flex: none; width: 140px; scroll-snap-align: start; }
	.pst { display: flex; flex-direction: column; gap: 2px; width: 100%; padding: 0; border: 0; background: none; text-align: left; cursor: pointer; font: var(--t-caption); border-radius: 8px; }
	.pst-pic { width: 100%; border-radius: 10px; margin-bottom: 6px; box-shadow: 0 0 0 1px var(--line); transition: transform var(--dur-fast) var(--ease-out); }
	.pst:hover .pst-pic { transform: translateY(-2px); }
	.pst-n { font: var(--t-footnote); font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

	/* 5 Ask cards */
	.acards { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(340px, 100%), 1fr)); gap: 14px; }
	.ac { display: flex; flex-direction: column; gap: 12px; padding: 16px; border-radius: var(--r-card); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.ac-h { display: flex; align-items: center; gap: 8px; font: var(--t-subhead); }
	.ac-h time { color: var(--text-muted); font: var(--t-footnote); }
	.ac-st { margin-left: auto; }
	.ac-say { padding: 10px 14px; border-radius: 4px 16px 16px 16px; background: color-mix(in srgb, var(--accent) 10%, var(--surface-2)); font: var(--t-callout); overflow-wrap: anywhere; }
	.ac-t { display: flex; align-items: center; gap: 12px; padding: 8px; border: 0; border-radius: 12px; background: var(--surface-2); text-align: left; cursor: pointer; transition: background var(--dur-fast) var(--ease-out); }
	.ac-t:hover { background: var(--surface-3); }
	.ac-t > span { flex: 1; display: flex; flex-direction: column; min-width: 0; font: var(--t-subhead); }
	.ac-t .muted { font: var(--t-caption); }
	.ac-pic { width: 44px; }

	/* 6 Poster sheet */
	.pgrid { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: 20px 14px; }
	.pg { display: flex; flex-direction: column; gap: 2px; width: 100%; padding: 0; border: 0; background: none; text-align: left; cursor: pointer; font: var(--t-caption); border-radius: 10px; }
	.pg-art { position: relative; display: block; margin-bottom: 6px; border-radius: 10px; box-shadow: 0 0 0 1px var(--line); transition: transform var(--dur-fast) var(--ease-out); }
	.pg:hover .pg-art { transform: translateY(-3px); }
	.pg:active .pg-art { transform: scale(0.98); }
	.pg-pic { width: 100%; border-radius: 10px; }
	.pg-av { position: absolute; right: 6px; bottom: 6px; display: inline-flex; border-radius: 50%; box-shadow: 0 0 0 2px var(--bg); }
	.pg-new { position: absolute; left: 6px; top: 6px; padding: 2px 8px; border-radius: var(--r-pill); background: var(--wait); color: var(--wait-on-art, #111); font: var(--t-caption); font-weight: 700; }
	.pg-n { font: var(--t-footnote); font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

	/* 7 Pick and act */
	.pk { padding-bottom: 84px; }
	.pk-top { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 8px; color: var(--text-muted); font: var(--t-footnote); }
	.pk-list { border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); overflow: hidden; }
	.pk-row { display: flex; align-items: center; gap: 10px; padding-right: 12px; transition: background var(--dur-fast) var(--ease-out); }
	.pk-row + .pk-row { border-top: 1px solid var(--line); }
	.pk-row.on { background: color-mix(in srgb, var(--accent) 9%, transparent); }
	.pk-hit { flex: 1; min-width: 0; display: flex; align-items: center; gap: 12px; padding: 10px 0 10px 14px; border: 0; background: none; text-align: left; cursor: pointer; }
	.pk-hit:disabled { cursor: default; }
	.circle { display: grid; place-items: center; flex: none; width: 22px; height: 22px; border-radius: 50%; box-shadow: inset 0 0 0 1.5px var(--line-strong, var(--text-muted)); color: #fff; transition: background var(--dur-fast) var(--ease-out); }
	.pk-hit:disabled .circle { opacity: 0.3; }
	.on .circle { background: var(--accent); box-shadow: none; }
	.pk-pic { width: 36px; }
	.pk-t { display: flex; flex-direction: column; min-width: 0; font: var(--t-subhead); }
	.pk-t .muted { font: var(--t-caption); }
	.pk-i { display: grid; place-items: center; width: 36px; height: 36px; border: 0; border-radius: 50%; background: none; color: var(--text-muted); cursor: pointer; }
	.pk-i:hover { background: var(--surface-2); color: var(--text); }
	.pk-bar { position: sticky; bottom: calc(var(--tabbar, 0px) + 12px); z-index: 5; display: grid; grid-template-columns: 1fr auto 1fr; align-items: center; gap: 8px; margin-top: 14px; padding: 8px; border-radius: var(--r-pill); background: color-mix(in srgb, var(--surface-1) 88%, transparent); box-shadow: 0 0 0 1px var(--line), var(--shadow-panel, 0 8px 24px rgb(0 0 0 / 0.2)); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); }
	.pk-b { display: inline-flex; align-items: center; justify-content: center; gap: 6px; min-height: 40px; border: 0; border-radius: var(--r-pill); background: none; font: var(--t-subhead); font-weight: 600; cursor: pointer; transition: background var(--dur-fast) var(--ease-out), opacity var(--dur-fast) var(--ease-out); }
	.pk-b:disabled { opacity: 0.35; cursor: default; }
	.pk-b.yes:not(:disabled) { background: var(--accent); color: var(--text-on-red, #fff); }
	.pk-b.no:not(:disabled):hover { background: var(--surface-2); }
	.pk-n { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }

	/* 8 People rows */
	.pr { max-width: 720px; }
	.pr-top { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
	.seg { display: inline-flex; padding: 3px; border-radius: var(--r-pill); background: var(--surface-2); }
	.seg button { display: inline-flex; align-items: center; gap: 6px; min-height: 34px; padding: 0 16px; border: 0; border-radius: var(--r-pill); background: none; color: var(--text-muted); font: var(--t-subhead); font-weight: 600; cursor: pointer; transition: background var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	.seg button[aria-selected='true'] { background: var(--surface-1); color: var(--text); box-shadow: 0 1px 3px rgb(0 0 0 / 0.15); }
	.seg .rn { margin-left: 0; }
	.pill-btn { min-height: 34px; padding: 0 16px; border: 0; border-radius: var(--r-pill); background: var(--surface-2); color: var(--accent-text); font: var(--t-subhead); font-weight: 600; cursor: pointer; }
	.pill-btn:hover { background: var(--surface-3); }
	.pr-row { display: flex; align-items: center; gap: 12px; padding: 12px 4px; border-bottom: 1px solid var(--line); }
	.pr-t { flex: 1; min-width: 0; display: flex; flex-direction: column; padding: 0; border: 0; background: none; text-align: left; cursor: pointer; }
	.pr-l1 { display: flex; align-items: center; gap: 6px; color: var(--text-muted); font: var(--t-footnote); }
	.pr-l1 strong { color: var(--text); font: var(--t-subhead); font-weight: 600; }
	.pr-l2 { font: var(--t-callout); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.pr-l2 .muted { font: var(--t-footnote); }
	.pr-pic { width: 30px; border-radius: 4px; }

	/* 10 Tracker */
	.trk { display: flex; flex-direction: column; gap: 10px; }
	.tr { display: grid; grid-template-columns: minmax(0, 1.3fr) minmax(0, 1.6fr) auto; align-items: center; gap: 16px; padding: 12px 16px; border-radius: var(--r-card); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.tr-t { display: flex; align-items: center; gap: 12px; min-width: 0; padding: 0; border: 0; background: none; text-align: left; cursor: pointer; }
	.tr-t > span { display: flex; flex-direction: column; min-width: 0; font: var(--t-subhead); }
	.tr-t strong { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.tr-t .muted { font: var(--t-caption); }
	.tr-pic { width: 40px; }
	.steps { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); }
	.steps li { position: relative; display: flex; flex-direction: column; align-items: center; gap: 6px; color: var(--text-muted); font: var(--t-caption); text-align: center; }
	.steps li::before { content: ''; position: absolute; top: 5px; right: 50%; width: 100%; height: 2px; background: var(--line); z-index: 0; }
	.steps li:first-child::before { display: none; }
	.steps li.done::before { background: var(--ok); }
	.sd { position: relative; z-index: 1; width: 12px; height: 12px; border-radius: 50%; background: var(--surface-3); box-shadow: 0 0 0 3px var(--surface-1); }
	.done .sd { background: var(--ok); }
	.now .sd { box-shadow: 0 0 0 3px var(--surface-1), 0 0 0 6px color-mix(in srgb, var(--ok) 30%, transparent); }
	.now .sl { color: var(--text); font-weight: 600; }
	.steps.no li:nth-child(2) .sd { background: var(--text-muted); }
	.steps.no li:nth-child(2) .sl { color: var(--text); font-weight: 600; }
	.steps.no li:nth-child(n + 3) { opacity: 0.35; }
	.tr-act { display: flex; justify-content: flex-end; font: var(--t-footnote); }

	/* 11 Desk: depth from the art itself. A hero lit by the newest poster, rows that
	   carry a faint glow of their own poster, posters with real shadow, soft glass
	   edges, and motion only where something changes. */
	.desk { max-width: 1120px; }
	.dk-hero { position: relative; isolation: isolate; overflow: hidden; display: flex; align-items: flex-end; justify-content: space-between; gap: 24px; margin-bottom: 18px; padding: 26px 28px; border-radius: var(--r-modal, 20px); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line), 0 1px 0 rgb(255 255 255 / 0.04) inset; }
	.dk-hero-bg { position: absolute; inset: -60px; z-index: -1; filter: blur(70px) saturate(1.5); opacity: 0.42; transform: scale(1.1); }
	.dk-hero::after { content: ''; position: absolute; inset: 0; z-index: -1; background: linear-gradient(100deg, var(--surface-1) 25%, color-mix(in srgb, var(--surface-1) 40%, transparent)); }
	.dk-eyebrow { color: var(--text-muted); font: var(--t-footnote); font-weight: 600; letter-spacing: 0.06em; text-transform: uppercase; }
	.dk-line { display: flex; align-items: center; gap: 14px; margin-top: 4px; }
	.dk-num { font-size: 56px; font-weight: 700; line-height: 1; letter-spacing: -0.03em; font-variant-numeric: tabular-nums; }
	.dk-words { display: flex; flex-direction: column; font: var(--t-title3); }
	.dk-words .muted { font: var(--t-footnote); }
	.dk-side { display: flex; flex-direction: column; align-items: flex-end; gap: 10px; }
	.faces { display: flex; }
	.face { display: inline-flex; margin-left: -10px; border-radius: 50%; box-shadow: 0 0 0 3px var(--surface-1); }
	.face:first-child { margin-left: 0; }
	.more-n { display: grid; place-items: center; width: 34px; height: 34px; background: var(--surface-3); color: var(--text-muted); font: var(--t-caption); font-weight: 700; }
	.dk-week { font: var(--t-footnote); }

	.dk-list { display: flex; flex-direction: column; gap: 8px; }
	.dk { position: relative; isolation: isolate; overflow: hidden; border-radius: 16px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); transition: box-shadow var(--dur) var(--ease-out), transform var(--dur) var(--ease-out); animation: dk-in 420ms var(--ease-out) both; animation-delay: calc(min(var(--i), 8) * 40ms); }
	.dk:hover { transform: translateY(-1px); box-shadow: 0 0 0 1px var(--line-strong, var(--line)), 0 10px 30px -12px rgb(0 0 0 / 0.35); }
	.dk.on { box-shadow: 0 0 0 1px var(--line-strong, var(--line)), 0 18px 44px -18px rgb(0 0 0 / 0.45); }
	.dk-glow { position: absolute; inset: 0 auto 0 -40px; width: 340px; z-index: -1; filter: blur(42px) saturate(1.6); opacity: 0.22; transition: opacity var(--dur) var(--ease-out); mask-image: linear-gradient(90deg, #000 30%, transparent); }
	.dk:hover .dk-glow, .dk.on .dk-glow { opacity: 0.34; }
	.dk.answered .dk-glow { opacity: 0.08; }
	.dk-row { display: grid; grid-template-columns: minmax(0, 1fr) 220px auto 36px; align-items: center; gap: 18px; padding: 12px 14px 12px 12px; }
	.dk-hit { display: flex; align-items: center; gap: 16px; min-width: 0; padding: 0; border: 0; background: none; text-align: left; cursor: pointer; border-radius: 10px; }
	.dk-art { position: relative; flex: none; }
	.dk-pic { width: 54px; border-radius: 8px; box-shadow: 0 0 0 1px rgb(255 255 255 / 0.06), 0 8px 20px -6px rgb(0 0 0 / 0.5); transition: transform var(--dur) var(--ease-out); }
	.dk-hit:hover .dk-pic { transform: scale(1.04) rotate(-1deg); }
	.dk-dot { position: absolute; right: -3px; top: -3px; width: 12px; height: 12px; border-radius: 50%; background: var(--wait); box-shadow: 0 0 0 3px var(--surface-1); }
	.dk-t { display: flex; flex-direction: column; gap: 1px; min-width: 0; }
	.dk-kind { color: var(--text-muted); font: var(--t-caption); font-weight: 600; letter-spacing: 0.04em; text-transform: uppercase; }
	.dk-t strong { font: var(--t-title3); letter-spacing: -0.01em; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.dk-t .muted { font: var(--t-footnote); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.dk-who { display: flex; align-items: center; gap: 10px; min-width: 0; }
	.dk-who > span { display: flex; flex-direction: column; min-width: 0; }
	.dk-who strong { display: inline-flex; align-items: center; gap: 6px; font: var(--t-subhead); font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.dk-who time { font: var(--t-caption); }
	.dk-act { display: flex; justify-content: flex-end; gap: 8px; }
	.dk-b { display: inline-flex; align-items: center; justify-content: center; gap: 6px; min-height: 38px; padding: 0 16px; border: 0; border-radius: var(--r-pill); font: var(--t-subhead); font-weight: 600; cursor: pointer; transition: transform var(--dur-fast) var(--ease-out), background var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out); }
	.dk-b:active { transform: scale(0.96); }
	.dk-b.no { background: color-mix(in srgb, var(--text) 7%, transparent); color: var(--text); }
	.dk-b.no:hover { background: color-mix(in srgb, var(--text) 12%, transparent); }
	.dk-b.yes { background: var(--accent); color: var(--text-on-red, #fff); box-shadow: 0 6px 18px -6px color-mix(in srgb, var(--accent) 70%, transparent); }
	.dk-b.yes:hover { box-shadow: 0 8px 22px -6px color-mix(in srgb, var(--accent) 85%, transparent); transform: translateY(-1px); }
	.dk.answered .dk-hit, .dk.answered .dk-who { opacity: 0.66; }
	.dk-caret { transform: rotate(-90deg); width: 32px; height: 32px; border-radius: 50%; }
	.dk-caret:hover { background: var(--surface-2); color: var(--text); }
	.dk.on .dk-caret { transform: none; color: var(--text); }
	.dk-more { display: grid; grid-template-columns: 120px minmax(0, 1fr); gap: 24px; padding: 6px 24px 22px 12px; border-top: 1px solid color-mix(in srgb, var(--line) 60%, transparent); }
	.dk-big { width: 120px; margin-top: 16px; border-radius: 12px; box-shadow: 0 18px 40px -12px rgb(0 0 0 / 0.55); }
	.dk-more-txt { padding-top: 16px; }
	.dk-more .facts { max-width: 560px; }
	.dk-more-act { margin-top: 14px; }
	.dk-empty { border-radius: 16px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	@keyframes dk-in { from { opacity: 0; transform: translateY(8px); } }
	@media (prefers-reduced-motion: reduce) { .dk, .dk-pic, .dk-b { animation: none; transition: none; } }

	/* 12 Screening Room */
	.sr { display: grid; grid-template-columns: 200px minmax(260px, 340px) minmax(0, 1fr); gap: 16px; align-items: start; }
	.sr-tabs { position: sticky; top: calc(var(--topbar, 56px) + 12px); display: flex; flex-direction: column; gap: 2px; }
	.sr-tab { display: flex; align-items: center; gap: 10px; min-height: 38px; padding: 0 10px; border: 0; border-radius: 10px; background: none; color: var(--text-muted); font: var(--t-subhead); text-align: left; cursor: pointer; transition: background var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	.sr-tab:hover { background: var(--surface-2); color: var(--text); }
	.sr-tab[aria-current='page'] { background: var(--surface-2); color: var(--text); font-weight: 600; }
	.sr-keys { margin: 16px 10px 0; color: var(--text-muted); font: var(--t-caption); line-height: 2; }
	kbd { margin-right: 3px; padding: 1px 6px; border-radius: 4px; background: var(--surface-3); box-shadow: inset 0 -1px 0 var(--line); font: var(--t-caption); font-weight: 600; }
	.sr-list { position: sticky; top: calc(var(--topbar, 56px) + 12px); max-height: calc(100dvh - var(--topbar, 56px) - 120px); overflow-y: auto; padding: 6px; border-radius: 18px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.sr-row { position: relative; display: flex; align-items: center; gap: 12px; width: 100%; padding: 8px; border: 0; border-radius: 12px; background: none; text-align: left; cursor: pointer; transition: background var(--dur-fast) var(--ease-out); }
	.sr-row:hover { background: var(--surface-2); }
	.sr-row.cur { background: color-mix(in srgb, var(--accent) 14%, var(--surface-2)); }
	.sr-pic { width: 38px; border-radius: 6px; box-shadow: 0 4px 12px -4px rgb(0 0 0 / 0.5); }
	.sr-t { display: flex; flex-direction: column; min-width: 0; font: var(--t-subhead); }
	.sr-t strong { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.sr-t .muted { font: var(--t-caption); }
	.sr-dot { position: static; margin-left: auto; flex: none; width: 8px; height: 8px; box-shadow: none; }
	.sr-stage { position: relative; isolation: isolate; overflow: hidden; min-height: 560px; border-radius: 22px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line), 0 30px 60px -30px rgb(0 0 0 / 0.6); }
	.sr-bg { position: absolute; inset: -80px; z-index: -1; filter: blur(80px) saturate(1.6); opacity: 0.55; animation: fade 500ms var(--ease-out) both; }
	.sr-stage::after { content: ''; position: absolute; inset: 0; z-index: -1; background: linear-gradient(180deg, transparent, var(--surface-1) 85%); }
	.sr-in { display: flex; flex-direction: column; gap: 20px; padding: 32px; animation: pop 360ms var(--ease-out) both; }
	.sr-head { display: flex; gap: 24px; align-items: flex-end; }
	.sr-big { width: 170px; border-radius: 14px; box-shadow: 0 0 0 1px rgb(255 255 255 / 0.08), 0 30px 60px -18px rgb(0 0 0 / 0.7); }
	.sr-ht h2 { margin-top: 4px; font: var(--t-title1); letter-spacing: -0.02em; overflow-wrap: anywhere; }
	.glass { padding: 16px 18px; border-radius: 16px; background: color-mix(in srgb, var(--surface-1) 55%, transparent); box-shadow: inset 0 0 0 1px rgb(255 255 255 / 0.07); -webkit-backdrop-filter: blur(20px) saturate(1.4); backdrop-filter: blur(20px) saturate(1.4); }
	.sr-act { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; }
	.sr-zero { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 6px; min-height: 560px; color: var(--text-muted); text-align: center; }
	.sr-zero h2 { color: var(--text); font: var(--t-title3); }
	@keyframes fade { from { opacity: 0; } }

	/* 13 Up Next */
	.un-hero { position: relative; isolation: isolate; overflow: hidden; min-height: 440px; display: flex; align-items: flex-end; border-radius: 24px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.un-bg { position: absolute; inset: -40px; z-index: -1; filter: blur(30px) saturate(1.3); opacity: 0.75; transform: scale(1.1); animation: fade 600ms var(--ease-out) both; }
	.un-hero::after { content: ''; position: absolute; inset: 0; z-index: -1; background: linear-gradient(90deg, rgb(0 0 0 / 0.78) 0%, rgb(0 0 0 / 0.45) 55%, rgb(0 0 0 / 0.1)), linear-gradient(0deg, rgb(0 0 0 / 0.5), transparent 50%); }
	.un-in { display: flex; gap: 32px; align-items: flex-end; padding: 40px; color: #fff; animation: pop 420ms var(--ease-out) both; }
	.un-pic { width: 200px; border-radius: 14px; box-shadow: 0 0 0 1px rgb(255 255 255 / 0.12), 0 30px 60px -16px rgb(0 0 0 / 0.8); }
	.un-t { display: flex; flex-direction: column; gap: 8px; max-width: 620px; }
	.un-t .dk-eyebrow { color: rgb(255 255 255 / 0.7); }
	.un-t h2 { font-size: clamp(30px, 4vw, 48px); font-weight: 700; line-height: 1.05; letter-spacing: -0.03em; }
	.un-meta { color: rgb(255 255 255 / 0.75); font: var(--t-callout); }
	.un-who { display: flex; align-items: center; gap: 10px; font: var(--t-callout); }
	.un-who .muted { color: rgb(255 255 255 / 0.7); }
	.un-note { padding-left: 12px; border-left: 2px solid rgb(255 255 255 / 0.4); font: var(--t-body); font-style: italic; }
	.big-b { min-height: 46px; padding: 0 22px; font: var(--t-headline); }
	.glassy { background: rgb(255 255 255 / 0.16); color: #fff; -webkit-backdrop-filter: blur(16px); backdrop-filter: blur(16px); }
	.glassy:hover { background: rgb(255 255 255 / 0.26); }
	.un-h { margin: 28px 4px 12px; font: var(--t-title3); }
	.un-shelf { display: flex; gap: 16px; overflow-x: auto; padding: 4px 4px 12px; scroll-snap-type: x proximity; scrollbar-width: none; }
	.un-shelf::-webkit-scrollbar { display: none; }
	.un-shelf li { flex: none; width: 168px; scroll-snap-align: start; }
	.un-card { display: flex; flex-direction: column; gap: 2px; width: 100%; padding: 0; border: 0; background: none; text-align: left; cursor: pointer; font: var(--t-caption); border-radius: 12px; }
	.un-art { position: relative; display: block; margin-bottom: 8px; border-radius: 12px; transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out); box-shadow: 0 0 0 1px var(--line); }
	.un-card:hover .un-art { transform: translateY(-4px) scale(1.02); box-shadow: 0 0 0 1px var(--line), 0 18px 34px -14px rgb(0 0 0 / 0.6); }
	.un-card.cur .un-art { box-shadow: 0 0 0 3px var(--accent), 0 18px 34px -14px rgb(0 0 0 / 0.6); transform: translateY(-4px); }
	.un-cpic { width: 100%; border-radius: 12px; }
	.un-zero { position: relative; overflow: hidden; min-height: 420px; display: grid; place-items: center; border-radius: 24px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.collage { position: absolute; inset: -20px; display: grid; grid-template-columns: repeat(6, 1fr); gap: 10px; opacity: 0.35; transform: rotate(-6deg) scale(1.15); }
	.col-pic { width: 100%; border-radius: 10px; }
	.un-zt { position: relative; padding: 28px 32px; border-radius: 20px; text-align: center; background: color-mix(in srgb, var(--surface-1) 70%, transparent); -webkit-backdrop-filter: blur(20px); backdrop-filter: blur(20px); box-shadow: inset 0 0 0 1px rgb(255 255 255 / 0.08); }
	.un-zt h2 { font: var(--t-title2); }

	/* 14 Split stacks */
	.ss { display: flex; flex-direction: column; gap: 28px; max-width: 1120px; }
	.ss-h { display: flex; align-items: center; gap: 12px; margin: 0 4px 12px; }
	.ss-h > div { flex: 1; min-width: 0; }
	.ss-h h2 { display: flex; align-items: center; gap: 8px; font: var(--t-title3); }
	.ss-h .rn { margin-left: 0; }
	.ss-h p { font: var(--t-footnote); }
	.ss-ic { display: grid; place-items: center; width: 34px; height: 34px; border-radius: 10px; background: color-mix(in srgb, var(--c) 18%, transparent); color: var(--c); }
	.ss-list { display: flex; flex-direction: column; gap: 8px; }
	.ss-row { position: relative; isolation: isolate; overflow: hidden; display: grid; grid-template-columns: minmax(0, 1fr) 220px auto; align-items: center; gap: 18px; padding: 12px 14px 12px 12px; border-radius: 16px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out); }
	.ss-row:hover { transform: translateY(-1px); box-shadow: 0 0 0 1px var(--line-strong, var(--line)), 0 10px 30px -12px rgb(0 0 0 / 0.35); }
	.ss-row.answered .dk-hit, .ss-row.answered .dk-who { opacity: 0.66; }
	.ss-pic { width: 48px; border-radius: 8px; box-shadow: 0 8px 20px -6px rgb(0 0 0 / 0.5); }
	.ss-done { color: var(--text-muted); font: var(--t-footnote); }

	/* 15–19 */
	.desk.wide { max-width: none; }
	.bn { position: relative; isolation: isolate; overflow: hidden; margin-bottom: 8px; border-radius: 24px; background: #000; box-shadow: 0 0 0 1px var(--line), 0 30px 60px -30px rgb(0 0 0 / 0.6); }
	.bn-bg { position: absolute; inset: -40px; z-index: -1; filter: blur(28px) saturate(1.35); opacity: 0.8; transform: scale(1.1); animation: fade 600ms var(--ease-out) both; }
	.bn::after { content: ''; position: absolute; inset: 0; z-index: -1; background: linear-gradient(90deg, rgb(0 0 0 / 0.82) 0%, rgb(0 0 0 / 0.5) 60%, rgb(0 0 0 / 0.15)), linear-gradient(0deg, rgb(0 0 0 / 0.45), transparent 60%); }
	.bn-in { display: flex; gap: 32px; align-items: flex-end; padding: 36px; color: #fff; animation: pop 420ms var(--ease-out) both; }
	.bn-art { flex: none; padding: 0; border: 0; background: none; cursor: pointer; border-radius: 14px; }
	.bn-pic { width: 180px; border-radius: 14px; box-shadow: 0 0 0 1px rgb(255 255 255 / 0.12), 0 30px 60px -16px rgb(0 0 0 / 0.8); transition: transform var(--dur) var(--ease-out); }
	.bn-art:hover .bn-pic { transform: translateY(-3px) rotate(-1deg); }
	.bn-t { display: flex; flex-direction: column; gap: 8px; max-width: 620px; }
	.bn-t .dk-eyebrow { color: rgb(255 255 255 / 0.72); }
	.bn-t h2 { font-size: clamp(28px, 3.6vw, 44px); font-weight: 700; line-height: 1.05; letter-spacing: -0.03em; }
	.bn-meta { color: rgb(255 255 255 / 0.75); font: var(--t-callout); }
	.bn-who { display: flex; align-items: center; gap: 10px; font: var(--t-callout); }
	.bn-note { padding-left: 12px; border-left: 2px solid rgb(255 255 255 / 0.4); font: var(--t-body); font-style: italic; }
	.dk.cur { box-shadow: 0 0 0 2px var(--accent), 0 18px 44px -18px rgb(0 0 0 / 0.45); }
	.dk-row.nowho { grid-template-columns: minmax(0, 1fr) auto 36px; }
	.ghost { background: none !important; color: var(--text-muted) !important; }
	.ghost:hover { color: var(--text) !important; background: color-mix(in srgb, var(--text) 7%, transparent) !important; }
	.shelf-s { margin-top: 8px; }
	.shelf-s .un-h { display: flex; align-items: center; gap: 8px; }
	.shelf-s .rn { margin-left: 0; }
	.un-card { position: relative; }
	button.un-art { padding: 0; border: 0; background: none; cursor: pointer; width: 100%; }
	.shelf-act { display: flex; gap: 6px; margin-top: 6px; }
	.shelf-act .tk { width: 30px; height: 30px; }
	.tri-top { display: flex; align-items: flex-end; justify-content: space-between; gap: 16px; margin-bottom: 14px; }
	.tri-top h2 { font: var(--t-title2); }
	.tri-top p { font: var(--t-footnote); }
	.why { display: flex; flex-direction: column; gap: 8px; padding: 4px 16px 16px 82px; }
	.why label { font: var(--t-footnote); font-weight: 600; }
	.why-row { display: flex; gap: 8px; }
	.why-row .input { flex: 1; min-width: 0; }
	.why-chips { display: flex; flex-wrap: wrap; gap: 6px; }
	.chip-s { min-height: 28px; padding: 0 12px; border: 0; border-radius: var(--r-pill); background: var(--surface-2); color: var(--text-muted); font: var(--t-caption); font-weight: 600; cursor: pointer; }
	.chip-s:hover { background: var(--surface-3); color: var(--text); }
	.per { margin-bottom: 26px; }
	.per-h { display: flex; align-items: center; gap: 14px; margin: 0 4px 12px; }
	.per-h > div { flex: 1; min-width: 0; }
	.per-h h2 { display: flex; align-items: center; gap: 8px; font: var(--t-title3); }
	.per-h p { font: var(--t-footnote); }
	.dp { display: grid; grid-template-columns: minmax(0, 1fr) minmax(360px, 440px); gap: 18px; align-items: start; }
	.dp .dk-row { grid-template-columns: minmax(0, 1fr) 170px auto; }
	.dp-panel { position: sticky; top: calc(var(--topbar, 56px) + 12px); min-height: 0; }
	.dp-panel .sr-in { padding: 24px; }
	.dp-panel .sr-big { width: 130px; }
	.dp-panel .sr-ht h2 { font: var(--t-title2); }

	/* Sheets: side (3), rising over art (6), review window (9). */
	.sheet { padding: 0; border: 0; color: var(--text); background: var(--bg); box-shadow: 0 0 0 1px var(--line), var(--shadow-modal); overflow: hidden; }
	.sheet::backdrop { background: rgb(0 0 0 / 0.5); -webkit-backdrop-filter: blur(4px); backdrop-filter: blur(4px); }
	.sheet[open] { animation: pop var(--dur) var(--ease-out) both; }
	.sheet.side { margin: 0 0 0 auto; width: min(460px, 100vw); max-width: none; height: 100dvh; max-height: none; border-radius: var(--r-modal) 0 0 var(--r-modal); }
	.sheet.side[open] { animation: from-right var(--dur) var(--ease-out) both; }
	.sheet.rise { margin: auto auto 0; width: min(560px, 100vw); max-width: none; max-height: 88dvh; border-radius: 24px 24px 0 0; }
	.sheet.rise[open] { animation: from-below var(--dur) var(--ease-out) both; }
	.sheet.review { width: min(560px, calc(100vw - 32px)); border-radius: var(--r-modal); }
	.sh-bg { position: absolute; inset: -40px; filter: blur(40px) saturate(1.3); opacity: 0.45; }
	.sh-in { position: relative; display: flex; flex-direction: column; height: 100%; max-height: inherit; }
	.sh-h { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 12px 12px 20px; border-bottom: 1px solid var(--line); }
	.sh-k { color: var(--text-muted); font: var(--t-footnote); font-weight: 600; }
	.sh-x { display: grid; place-items: center; width: 36px; height: 36px; border: 0; border-radius: 50%; background: var(--surface-2); cursor: pointer; }
	.sh-x:hover { background: var(--surface-3); }
	.sh-body { flex: 1; overflow-y: auto; display: flex; flex-direction: column; gap: 16px; padding: 20px; }
	.sh-top { display: flex; gap: 16px; align-items: flex-end; }
	.sh-pic { width: 96px; border-radius: 10px; box-shadow: 0 10px 30px rgb(0 0 0 / 0.3); }
	.sh-t { font: var(--t-title2); overflow-wrap: anywhere; }
	.sh-more { align-self: flex-start; }
	.sh-f { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; padding: 14px 20px calc(14px + env(safe-area-inset-bottom, 0px)); border-top: 1px solid var(--line); background: color-mix(in srgb, var(--bg) 85%, transparent); }
	.sh-done { grid-column: 1 / -1; color: var(--text-muted); font: var(--t-callout); text-align: center; }
	.big { display: inline-flex; align-items: center; justify-content: center; gap: 8px; min-height: 48px; border: 0; border-radius: 14px; font: var(--t-headline); cursor: pointer; transition: transform var(--dur-fast) var(--ease-out), filter var(--dur-fast) var(--ease-out); }
	.big:active { transform: scale(0.98); }
	.big.yes { background: color-mix(in srgb, var(--ok) 22%, transparent); color: var(--ok-text, var(--ok)); }
	.big.no { background: color-mix(in srgb, var(--accent) 16%, transparent); color: var(--accent-text); }
	.big:hover { filter: brightness(1.08); }
	@keyframes pop { from { opacity: 0; transform: translateY(10px) scale(0.985); } }
	@keyframes from-right { from { transform: translateX(40px); opacity: 0; } }
	@keyframes from-below { from { transform: translateY(60px); opacity: 0; } }

	@media (max-width: 1023px) {
		.sr { grid-template-columns: minmax(0, 1fr); }
		.sr-tabs, .sr-list { position: static; max-height: none; }
		.sr-tabs { flex-direction: row; overflow-x: auto; scrollbar-width: none; }
		.sr-tab { flex: none; }
		.sr-keys { display: none; }
		.ss-row { grid-template-columns: minmax(0, 1fr); }
		.un-in { flex-direction: column; align-items: flex-start; padding: 24px; }
		.un-pic { width: 130px; }
		.q-grid, .mq { grid-template-columns: minmax(0, 1fr); }
		.q-rail, .mq-rail { position: static; flex-direction: row; overflow-x: auto; scrollbar-width: none; gap: 6px; }
		.q-rail .rh { display: none; }
		.rr, .mq-q { flex: none; border-radius: var(--r-pill); background: var(--surface-2); }
		.mq-q[aria-current='page']::before { display: none; }
		.mq-q[aria-current='page'] { background: var(--accent); color: var(--text-on-red, #fff); }
		.ov { grid-template-columns: minmax(0, 1fr); }
	}
	@media (max-width: 767px) {
		.q-head { display: none; }
		.q-line { grid-template-columns: 24px 26px minmax(0, 1fr) auto; height: 56px; }
		.q-who, .q-when { display: none; }
		.q-more { padding-left: 12px; }
		.tbl-head { display: none; }
		.tbl-row { grid-template-columns: minmax(0, 1fr) auto; grid-template-areas: 't go' 'w st'; padding: 10px 14px; gap: 4px 10px; }
		.tbl-title { grid-area: t; }
		.tbl-who { grid-area: w; padding-left: 42px; color: var(--text-muted); }
		.tbl-st { grid-area: st; justify-self: end; }
		.tbl-go { grid-area: go; }
		.tbl-when { display: none; }
		.ov-row { flex-wrap: wrap; }
		.ov-row .answer { width: 100%; display: grid; grid-template-columns: 1fr 1fr; padding-left: 48px; }
		.pgrid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.tr { grid-template-columns: minmax(0, 1fr); gap: 12px; }
		.tr-act { justify-content: stretch; }
		.tr-act :global(.answer) { display: grid; grid-template-columns: 1fr 1fr; width: 100%; }
		.post-pic { width: 52px; }
		.pr-pic { display: none; }
		.dk-hero { flex-direction: column; align-items: stretch; padding: 20px; }
		.dk-num { font-size: 44px; }
		.dk-row { grid-template-columns: minmax(0, 1fr) 36px; gap: 10px; padding: 12px; }
		.dk-who { grid-column: 1; padding-left: 70px; }
		.dk-act { grid-column: 1 / -1; display: grid; grid-template-columns: 1fr 1fr; }
		.dk-caret { grid-row: 1; grid-column: 2; }
		.dk-more { grid-template-columns: minmax(0, 1fr); padding: 16px; }
		.dk-more-art { display: none; }
		.bn-in { flex-direction: column; align-items: flex-start; padding: 22px; gap: 18px; }
		.bn-pic { width: 120px; }
		.dp { grid-template-columns: minmax(0, 1fr); }
		.dp-panel { display: none; }
		.why { padding: 4px 12px 14px; }
		.tri-top, .per-h { flex-wrap: wrap; }
		.sheet.side { width: 100vw; border-radius: 0; }
		.sheet.review { width: 100vw; height: 100dvh; max-height: none; border-radius: 0; margin: 0; }
		.sheet.rise { width: 100vw; }
	}
	@media (prefers-reduced-motion: reduce) {
		.sheet[open], .sheet.side[open], .sheet.rise[open] { animation: none; }
		.tk, .pg-art, .pst-pic, .caret { transition: none; }
	}
</style>
