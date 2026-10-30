<script lang="ts">
	// Five Approvals layouts Luis is comparing (pill bar, ?view=1…5):
	// 1 Conversation, 2 Spotlight, 3 Board, 4 Tickets, 5 Ledger.
	// Every layout answers the same asks through ctx, so Undo, Choose and the
	// full approve screen behave the same in each.
	import { tick } from 'svelte';
	import { flip } from 'svelte/animate';
	import Poster from '../Poster.svelte';
	import Avatar from '../ui/Avatar.svelte';
	import Button from '../ui/Button.svelte';
	import IconButton from '../ui/IconButton.svelte';
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
		/** Opens the full ask (seasons, folder, rules, note). */
		open: (a: Ask, el?: HTMLElement | null) => void;
	} = $props();

	const waiting = (a: Ask) => ctx.approver && a.state === 'asked' && !ctx.isPending(a.id);
	const art = (a: Ask) => (a.kind === 'artist' ? artistPhoto(a) : a.poster) ?? null;
	const newest = (list: Ask[]) => [...list].sort((x, y) => Date.parse(y.asked_at) - Date.parse(x.asked_at));
	const shown = $derived(newest(ctx.shown));
	const waitingCount = $derived(ctx.shown.filter(waiting).length);

	/** A plain sentence for what was asked: "Dune: Part Two" or "Severance, season 2". */
	const askedFor = (a: Ask) => (a.kind === 'movie' ? a.name : `${a.name}, ${line(a)}`);

	// 1 Conversation: one thread per person, newest person first.
	const threads = $derived.by(() => {
		const by = new Map<number, Ask[]>();
		for (const a of shown) {
			const id = ctx.who(a).id;
			by.set(id, [...(by.get(id) ?? []), a]);
		}
		return [...by.values()].map((asks) => ({ who: ctx.who(asks[0]), asks: [...asks].reverse() }));
	});

	// 2 Spotlight: the picked ask large, the rest as a filmstrip.
	let at = $state(0);
	const spot = $derived(shown[Math.min(at, Math.max(shown.length - 1, 0))] ?? null);
	function go(by: number) {
		if (!shown.length) return;
		at = (((at + by) % shown.length) + shown.length) % shown.length;
		void tick().then(() => document.querySelector<HTMLElement>(`[data-strip="${shown[at]?.id}"]`)?.scrollIntoView({ block: 'nearest', inline: 'center', behavior: 'smooth' }));
	}
	function spotKeys(e: KeyboardEvent) {
		if ((e.target as HTMLElement).closest('input, textarea, select, dialog')) return;
		if (e.key === 'ArrowRight') { e.preventDefault(); go(1); }
		if (e.key === 'ArrowLeft') { e.preventDefault(); go(-1); }
	}

	// 3 Board: every ask on its column, whatever the status tab says.
	const columns: { tab: AskTab; title: string; hint: string }[] = [
		{ tab: 'waiting', title: 'Waiting on you', hint: 'Nobody has answered yet.' },
		{ tab: 'approved', title: 'On its way', hint: 'Approved; HolyArrs is getting it.' },
		{ tab: 'done', title: 'Ready to watch', hint: 'In the Library now.' },
		{ tab: 'declined', title: 'Turned down', hint: 'The asker was told why.' }
	];
	const boardAsks = $derived(newest(ctx.viewed.filter((a) => (ctx.kind === 'all' || (ctx.kind === 'movie' ? a.kind !== 'show' && a.kind !== 'artist' : a.kind === ctx.kind)) && (!ctx.person || ctx.who(a).id === ctx.person))));
	const column = (t: AskTab) => boardAsks.filter((a) => askTab(a) === t);

	// 5 Ledger: j/k to move, a approves, x turns down, Enter opens.
	let cursor = $state(0);
	function ledgerKeys(e: KeyboardEvent) {
		if ((e.target as HTMLElement).closest('input, textarea, select, button[aria-haspopup]')) return;
		const a = shown[cursor];
		if (e.key === 'j' || e.key === 'ArrowDown') { e.preventDefault(); cursor = Math.min(cursor + 1, shown.length - 1); }
		else if (e.key === 'k' || e.key === 'ArrowUp') { e.preventDefault(); cursor = Math.max(cursor - 1, 0); }
		else if (e.key === 'a' && a && waiting(a)) ctx.approve([a]);
		else if (e.key === 'x' && a && waiting(a)) ctx.turnDown([a]);
		else if (e.key === 'Enter' && a) open(a, e.target as HTMLElement);
		else return;
		void tick().then(() => document.querySelector<HTMLElement>(`[data-led="${shown[cursor]?.id}"]`)?.focus({ preventScroll: false }));
	}
</script>

{#snippet pic(a: Ask, cls: string)}
	{@const src = art(a)}
	<span class={cls}>{#if a.kind === 'artist' && src}<img {src} alt="" loading="lazy" decoding="async" />{:else}<Poster src={a.kind === 'artist' ? null : src} name={a.name} />{/if}</span>
{/snippet}
{#snippet answer(a: Ask, size: 'sm' | 'md' | 'lg' = 'md', words = true)}
	{#if waiting(a)}
		<span class="answer">
			<Button {size} variant="primary" icon="check" type="button" data-testid="ask-approve" onclick={() => ctx.approve([a])}>{words ? 'Approve' : 'Yes'}</Button>
			<Button {size} icon="x" type="button" data-testid="ask-decline" onclick={() => ctx.turnDown([a])}>{words ? 'Turn down' : 'No'}</Button>
		</span>
	{:else if ctx.isPending(a.id)}
		<span class="sending">Sending your answer…</span>
	{:else}
		<StatusPill kind={ctx.pill(a).kind} label={ctx.pill(a).label} />
	{/if}
{/snippet}

<svelte:window onkeydown={(e) => look === 2 && spotKeys(e)} />

{#if look === 1}
	<!-- 1 Conversation -->
	<div class="talk" data-look="conversation">
		{#each threads as t (t.who.id)}
			<section class="thread" aria-labelledby="t-{t.who.id}">
				<header class="th-head">
					<Avatar name={t.who.name} src={t.who.avatar} size={40} />
					<div class="th-who">
						<h2 id="t-{t.who.id}">{t.who.name}{#if t.who.kid}<KidMark label="Kid" size="sm" />{/if}</h2>
						<span>{t.asks.filter(waiting).length ? `${t.asks.filter(waiting).length} waiting on your answer` : 'All answered'}</span>
					</div>
					{#if ctx.approver && t.asks.filter(waiting).length > 1}
						<Button size="sm" variant="plain" icon="check" type="button" onclick={() => ctx.approve(t.asks.filter(waiting))}>Approve all {t.asks.filter(waiting).length}</Button>
					{/if}
				</header>
				<ol class="msgs">
					{#each t.asks as a (a.id)}
						<li class="msg" animate:flip={{ duration: 260 }}>
							<time class="stamp" datetime={a.asked_at} title={dateTime(a.asked_at)}>{ago(a.asked_at)}</time>
							<div class="bubble them">
								<p>Can we get <button class="link" type="button" onclick={(e) => open(a, e.currentTarget)}>{askedFor(a)}</button>?</p>
								{#if a.note}<p class="said">&ldquo;{a.note}&rdquo;</p>{/if}
								<button class="attach" type="button" aria-label="Open {a.name}" onclick={(e) => open(a, e.currentTarget)}>
									{@render pic(a, 'att-pic')}
									<span class="att-txt"><strong>{a.name}</strong><span>{kindWord(a.kind)}{a.year ? ` · ${a.year}` : ''}</span></span>
								</button>
							</div>
							{#if waiting(a) || ctx.isPending(a.id)}
								<div class="replies">{@render answer(a, 'md', false)}</div>
							{:else}
								<div class="bubble me" class:no={a.state === 'turned_down'}>
									<p>{a.state === 'turned_down' ? 'Not this time.' : a.state === 'available' ? 'It’s ready. Enjoy.' : 'Yes. It’s on its way.'}</p>
									{#if ctx.decided(a)}<span class="by">{ctx.decided(a)}</span>{/if}
								</div>
							{/if}
						</li>
					{/each}
				</ol>
			</section>
		{/each}
	</div>
{:else if look === 2 && spot}
	<!-- 2 Spotlight -->
	<div class="spot" data-look="spotlight">
		<section class="sp-stage" aria-label="{spot.name}, {at + 1} of {shown.length}">
			{#key spot.id}
				{#if art(spot)}<div class="sp-light" aria-hidden="true" style:background-image={`url("${art(spot)}")`}></div>{/if}
				<div class="sp-in">
					<button class="sp-art" type="button" aria-label="Open {spot.name}" onclick={(e) => open(spot, e.currentTarget)}>{@render pic(spot, 'cover')}</button>
					<div class="sp-txt">
						<span class="eyebrow">{at === 0 && waiting(spot) ? 'Newest ask' : `${at + 1} of ${shown.length}`}</span>
						<h2>{spot.name}{#if spot.year}<span class="yr">{spot.year}</span>{/if}</h2>
						<p class="sp-what">{kindWord(spot.kind)} · {line(spot)}</p>
						{#each [ctx.who(spot)] as w (w.id)}
							<p class="sp-who"><Avatar name={w.name} src={w.avatar} size={28} /><span><strong>{w.name}</strong> asked <time datetime={spot.asked_at} title={dateTime(spot.asked_at)}>{ago(spot.asked_at)}</time></span>{#if w.kid}<KidMark label="Kid" size="sm" />{/if}</p>
						{/each}
						{#if spot.note}<blockquote>&ldquo;{spot.note}&rdquo;</blockquote>{/if}
						<div class="sp-act">
							{@render answer(spot, 'lg')}
							<Button size="lg" variant="plain" type="button" onclick={(e: MouseEvent) => open(spot, e.currentTarget as HTMLElement)}>Choose seasons and folder</Button>
						</div>
					</div>
				</div>
			{/key}
			{#if shown.length > 1}
				<div class="sp-nav">
					<IconButton icon="chevronLeft" variant="glass" label="The ask before" onclick={() => go(-1)} />
					<IconButton icon="chevronRight" variant="glass" label="The next ask" onclick={() => go(1)} />
				</div>
			{/if}
		</section>
		{#if shown.length > 1}
			<ol class="strip" aria-label="Every ask in this list">
				{#each shown as a, i (a.id)}
					<li><button type="button" class="st" class:on={i === at} data-strip={a.id} aria-current={i === at ? 'true' : undefined} aria-label="{a.name}, asked by {ctx.who(a).name}" onclick={() => (at = i)}>{@render pic(a, 'cover')}{#if !waiting(a) && a.state !== 'asked'}<span class="st-done" aria-hidden="true"></span>{/if}</button></li>
				{/each}
			</ol>
		{/if}
		<p class="hint">Use ← and → to move between asks.</p>
	</div>
{:else if look === 3}
	<!-- 3 Board -->
	<div class="board" data-look="board">
		{#each columns as c (c.tab)}
			{@const list = column(c.tab)}
			<section class="col col-{c.tab}" aria-labelledby="c-{c.tab}">
				<header class="col-head"><span class="dot" aria-hidden="true"></span><h2 id="c-{c.tab}">{c.title}</h2><span class="n">{list.length}</span></header>
				<p class="col-hint">{c.hint}</p>
				<ul class="cards">
					{#each list as a (a.id)}
						{@const w = ctx.who(a)}
						<li class="bc" animate:flip={{ duration: 300 }}>
							<button class="bc-main" type="button" onclick={(e) => open(a, e.currentTarget)}>
								{@render pic(a, 'bc-pic')}
								<span class="bc-txt"><strong>{a.name}</strong><span>{line(a)}</span><span class="bc-who"><Avatar name={w.name} src={w.avatar} size={16} />{w.name} · {ago(a.asked_at)}</span></span>
							</button>
							{#if waiting(a) || ctx.isPending(a.id)}<div class="bc-act">{@render answer(a, 'sm')}</div>{/if}
						</li>
					{:else}
						<li class="col-empty">Nothing here right now.</li>
					{/each}
				</ul>
			</section>
		{/each}
	</div>
{:else if look === 4}
	<!-- 4 Tickets -->
	<ul class="tickets" data-look="tickets">
		{#each shown as a (a.id)}
			{@const w = ctx.who(a)}
			<li class="tk" class:stamped={!waiting(a) && a.state !== 'asked'} class:no={a.state === 'turned_down'} animate:flip={{ duration: 280 }}>
				<button class="tk-art" type="button" aria-label="Open {a.name}" onclick={(e) => open(a, e.currentTarget)}>{@render pic(a, 'cover')}</button>
				<div class="tk-body">
					<span class="eyebrow">{kindWord(a.kind)}{a.year ? ` · ${a.year}` : ''}</span>
					<h2>{a.name}</h2>
					{#if a.kind !== 'movie'}<p class="tk-what">{line(a)}</p>{/if}
					<p class="tk-who"><Avatar name={w.name} src={w.avatar} size={22} /><span>For <strong>{w.name}</strong>, asked {ago(a.asked_at)}</span></p>
					{#if a.note}<p class="tk-note">&ldquo;{a.note}&rdquo;</p>{/if}
					<div class="tk-foot">{@render answer(a, 'md')}</div>
				</div>
				{#if !waiting(a) && a.state !== 'asked' && !ctx.isPending(a.id)}
					<span class="stamp-mark" aria-hidden="true">{a.state === 'turned_down' ? 'Turned down' : a.state === 'available' ? 'Ready' : 'Approved'}</span>
				{/if}
			</li>
		{/each}
	</ul>
{:else if look === 5}
	<!-- 5 Ledger -->
	<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
	<div class="ledger" data-look="ledger" role="grid" aria-label="Asks" aria-rowcount={shown.length} tabindex="-1" onkeydown={ledgerKeys}>
		<div class="lg-head" role="row">
			<span role="columnheader">Title</span><span role="columnheader">Asked by</span><span role="columnheader">When</span><span role="columnheader">Answer</span>
		</div>
		{#each shown as a, i (a.id)}
			{@const w = ctx.who(a)}
			<div class="lg-row" role="row" class:cur={i === cursor} animate:flip={{ duration: 220 }}>
				<span role="gridcell" class="lg-title">
					{@render pic(a, 'lg-pic')}
					<button type="button" data-led={a.id} tabindex={i === cursor ? 0 : -1} onfocus={() => (cursor = i)} onclick={(e) => open(a, e.currentTarget)}><strong>{a.name}</strong><span>{kindWord(a.kind)} · {line(a)}</span></button>
				</span>
				<span role="gridcell" class="lg-who"><Avatar name={w.name} src={w.avatar} size={20} />{w.name}{#if w.kid}<KidMark label="Kid" size="sm" />{/if}</span>
				<span role="gridcell" class="lg-when"><time datetime={a.asked_at} title={dateTime(a.asked_at)}>{ago(a.asked_at)}</time></span>
				<span role="gridcell" class="lg-act">
					{#if waiting(a)}
						<IconButton icon="check" variant="tinted" size="sm" label="Approve {a.name}" data-testid="ask-approve" onclick={() => ctx.approve([a])} />
						<IconButton icon="x" variant="tinted" size="sm" label="Turn down {a.name}" data-testid="ask-decline" onclick={() => ctx.turnDown([a])} />
					{:else}{@render answer(a, 'sm')}{/if}
				</span>
			</div>
		{/each}
		<p class="hint lg-keys"><kbd>J</kbd> <kbd>K</kbd> move · <kbd>A</kbd> approve · <kbd>X</kbd> turn down · <kbd>Enter</kbd> open</p>
	</div>
{/if}
{#if look !== 3 && waitingCount === 0 && ctx.tab === 'waiting'}
	<p class="clear">You’ve answered everyone. New asks show up here the moment someone asks.</p>
{/if}

<style>
	/* Shared: one poster frame, one answer pair, one quiet hint line. */
	.cover { display: block; aspect-ratio: 2 / 3; --poster-ratio: 2 / 3; overflow: hidden; background: var(--surface-3); }
	.cover > img, .att-pic > img, .bc-pic > img, .lg-pic > img { width: 100%; height: 100%; object-fit: cover; object-position: center 25%; display: block; }
	.answer { display: inline-flex; flex-wrap: wrap; gap: 8px; }
	.sending { color: var(--text-muted); font: var(--t-footnote); }
	.eyebrow { color: var(--text-muted); font: var(--t-footnote); font-weight: 600; letter-spacing: 0.02em; }
	.hint { margin-top: 12px; color: var(--text-faint, var(--text-muted)); font: var(--t-caption); }
	.clear { margin-top: 20px; padding: 18px 20px; border-radius: var(--r-card); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); color: var(--text-muted); font: var(--t-callout); text-align: center; }
	button { font: inherit; color: inherit; }
	button:focus-visible { outline: none; box-shadow: var(--focus-ring); }

	/* 1 Conversation: threads as cards, their bubbles left, yours right. */
	.talk { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(420px, 100%), 1fr)); gap: 16px; align-items: start; }
	.thread { border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); overflow: hidden; }
	.th-head { display: flex; align-items: center; gap: 12px; padding: 14px 16px; border-bottom: 1px solid var(--line); background: color-mix(in srgb, var(--surface-2) 60%, transparent); }
	.th-who { flex: 1; min-width: 0; display: flex; flex-direction: column; }
	.th-who h2 { display: flex; align-items: center; gap: 6px; font: var(--t-headline); }
	.th-who span { color: var(--text-muted); font: var(--t-footnote); }
	.msgs { display: flex; flex-direction: column; gap: 14px; margin: 0; padding: 16px; list-style: none; }
	.msg { display: flex; flex-direction: column; gap: 6px; }
	.stamp { align-self: center; color: var(--text-faint, var(--text-muted)); font: var(--t-caption); }
	.bubble { max-width: 86%; padding: 10px 14px; font: var(--t-callout); overflow-wrap: anywhere; animation: pop var(--dur) var(--ease-spring, var(--ease-out)) both; }
	.them { align-self: flex-start; border-radius: 18px 18px 18px 6px; background: var(--surface-2); }
	.me { align-self: flex-end; border-radius: 18px 18px 6px 18px; background: var(--accent); color: var(--text-on-red, #fff); }
	.me.no { background: var(--surface-3); color: var(--text); }
	.me .by { display: block; margin-top: 2px; opacity: 0.8; font: var(--t-caption); }
	.said { margin-top: 4px; color: var(--text-muted); font-style: italic; }
	.link { padding: 0; border: 0; background: none; font-weight: 600; text-decoration: underline; text-decoration-color: color-mix(in srgb, currentColor 35%, transparent); text-underline-offset: 3px; cursor: pointer; border-radius: 4px; }
	.attach { display: flex; align-items: center; gap: 10px; width: 100%; margin-top: 8px; padding: 6px; border: 0; border-radius: 12px; background: var(--surface-1); text-align: left; cursor: pointer; transition: background var(--dur-fast) var(--ease-out); }
	.attach:hover { background: var(--surface-3); }
	.att-pic { flex: none; width: 40px; aspect-ratio: 2 / 3; --poster-ratio: 2 / 3; border-radius: 6px; overflow: hidden; background: var(--surface-3); }
	.att-txt { display: flex; flex-direction: column; min-width: 0; font: var(--t-footnote); }
	.att-txt span { color: var(--text-muted); }
	.replies { align-self: flex-end; }
	@keyframes pop { from { opacity: 0; transform: translateY(6px) scale(0.98); } }

	/* 2 Spotlight: the ask lit by its own poster, a filmstrip below. */
	.spot { min-width: 0; }
	.sp-stage { position: relative; isolation: isolate; overflow: hidden; min-height: 420px; padding: 32px; border-radius: var(--r-modal, var(--r-panel)); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line), var(--shadow-panel, none); }
	.sp-light { position: absolute; inset: -60px; z-index: -1; background-size: cover; background-position: center 30%; filter: blur(60px) saturate(1.4); opacity: 0.5; animation: fade var(--dur-slow, 400ms) var(--ease-out) both; }
	.sp-in { display: flex; gap: 32px; align-items: flex-end; animation: rise var(--dur-page, 360ms) var(--ease-glide, var(--ease-out)) both; }
	.sp-art { flex: none; width: 220px; padding: 0; border: 0; border-radius: 14px; overflow: hidden; background: none; cursor: pointer; box-shadow: 0 0 0 1px rgb(255 255 255 / 0.08), 0 24px 48px rgb(0 0 0 / 0.4); transition: transform var(--dur) var(--ease-out); }
	.sp-art:hover { transform: translateY(-3px); }
	.sp-txt { display: flex; flex-direction: column; gap: 8px; min-width: 0; padding-bottom: 4px; }
	.sp-txt h2 { font: var(--t-title1); overflow-wrap: anywhere; }
	.yr { margin-left: 10px; color: var(--text-muted); font-weight: 400; }
	.sp-what { color: var(--text-muted); font: var(--t-callout); }
	.sp-who { display: flex; align-items: center; gap: 8px; font: var(--t-callout); }
	blockquote { margin: 4px 0 0; padding-left: 12px; border-left: 2px solid color-mix(in srgb, var(--text) 25%, transparent); font: var(--t-body); font-style: italic; }
	.sp-act { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 14px; }
	.sp-nav { position: absolute; top: 16px; right: 16px; display: flex; gap: 6px; }
	.strip { display: flex; gap: 10px; margin: 14px 0 0; padding: 4px 2px 8px; list-style: none; overflow-x: auto; scroll-snap-type: x proximity; scrollbar-width: none; }
	.strip::-webkit-scrollbar { display: none; }
	.st { position: relative; display: block; width: 64px; padding: 0; border: 0; border-radius: 8px; overflow: hidden; background: none; cursor: pointer; scroll-snap-align: center; opacity: 0.55; transition: opacity var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out); }
	.st:hover { opacity: 0.85; }
	.st.on { opacity: 1; transform: translateY(-3px); box-shadow: 0 0 0 2px var(--accent); }
	.st-done { position: absolute; right: 4px; top: 4px; width: 8px; height: 8px; border-radius: 50%; background: var(--ok); box-shadow: 0 0 0 2px var(--bg); }
	@keyframes rise { from { opacity: 0; transform: translateY(10px); } }
	@keyframes fade { from { opacity: 0; } }

	/* 3 Board: four lanes; each lane's dot carries its colour, nothing else does. */
	.board { display: grid; grid-template-columns: repeat(4, minmax(240px, 1fr)); gap: 14px; align-items: start; overflow-x: auto; scroll-snap-type: x mandatory; padding-bottom: 8px; }
	.col { scroll-snap-align: start; min-width: 0; padding: 12px; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.col-head { display: flex; align-items: center; gap: 8px; padding: 2px 4px; }
	.col-head h2 { flex: 1; font: var(--t-subhead); font-weight: 600; }
	.dot { width: 8px; height: 8px; border-radius: 50%; background: var(--text-muted); }
	.col-waiting .dot { background: var(--wait); }
	.col-approved .dot { background: var(--accent); }
	.col-done .dot { background: var(--ok); }
	.n { min-width: 22px; padding: 0 7px; border-radius: var(--r-pill); background: var(--surface-2); color: var(--text-muted); font: var(--t-caption); font-weight: 600; text-align: center; font-variant-numeric: tabular-nums; }
	.col-hint { margin: 2px 4px 10px; color: var(--text-muted); font: var(--t-caption); }
	.cards { display: flex; flex-direction: column; gap: 8px; margin: 0; padding: 0; list-style: none; }
	.bc { border-radius: var(--r-card); background: var(--surface-2); box-shadow: 0 0 0 1px var(--line); transition: transform var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out); }
	.bc:hover { transform: translateY(-1px); box-shadow: 0 0 0 1px var(--line-strong, var(--line)), var(--shadow-card, none); }
	.bc-main { display: flex; gap: 10px; width: 100%; padding: 10px; border: 0; background: none; text-align: left; cursor: pointer; border-radius: inherit; }
	.bc-pic { flex: none; width: 44px; aspect-ratio: 2 / 3; --poster-ratio: 2 / 3; border-radius: 6px; overflow: hidden; background: var(--surface-3); }
	.bc-txt { display: flex; flex-direction: column; gap: 2px; min-width: 0; font: var(--t-footnote); }
	.bc-txt strong { font: var(--t-subhead); font-weight: 600; overflow-wrap: anywhere; }
	.bc-txt > span { color: var(--text-muted); }
	.bc-who { display: inline-flex; align-items: center; gap: 5px; font: var(--t-caption); }
	.bc-act { padding: 0 10px 10px; }
	.bc-act .answer { display: grid; grid-template-columns: 1fr 1fr; }
	.col-empty { padding: 18px 8px; border: 1px dashed var(--line); border-radius: var(--r-card); color: var(--text-muted); font: var(--t-footnote); text-align: center; }

	/* 4 Tickets: a stub with a torn edge; the answer lands as a stamp. */
	.tickets { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(380px, 100%), 1fr)); gap: 16px; margin: 0; padding: 0; list-style: none; }
	.tk { position: relative; display: flex; overflow: hidden; border-radius: var(--r-card); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); transition: transform var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out); }
	.tk:hover { transform: translateY(-2px); box-shadow: 0 0 0 1px var(--line-strong, var(--line)), var(--shadow-lift, none); }
	.tk-art { flex: none; width: 128px; padding: 0; border: 0; background: none; cursor: pointer; }
	.tk-art .cover { height: 100%; }
	.tk-body { position: relative; flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 4px; padding: 16px 18px 16px 22px; background-image: radial-gradient(circle at 0 10px, var(--bg) 5px, transparent 5.5px); background-size: 10px 20px; background-repeat: repeat-y; }
	.tk-body h2 { font: var(--t-title3); overflow-wrap: anywhere; }
	.tk-what { color: var(--text-muted); font: var(--t-callout); }
	.tk-who { display: flex; align-items: center; gap: 8px; margin-top: 4px; font: var(--t-footnote); }
	.tk-note { margin-top: 4px; padding: 8px 10px; border-radius: 10px; background: var(--surface-2); font: var(--t-footnote); font-style: italic; }
	.tk-foot { margin-top: auto; padding-top: 12px; border-top: 1px dashed var(--line); }
	.tk.stamped .tk-art, .tk.stamped .tk-body > :not(.tk-foot) { opacity: 0.72; }
	.stamp-mark { position: absolute; right: 16px; top: 18px; padding: 4px 10px; border: 2px solid currentColor; border-radius: 6px; color: var(--ok-text, var(--ok)); font: var(--t-footnote); font-weight: 800; letter-spacing: 0.08em; text-transform: uppercase; transform: rotate(-8deg); animation: stamp 420ms var(--ease-spring, var(--ease-out)) both; pointer-events: none; }
	.tk.no .stamp-mark { color: var(--text-muted); }
	@keyframes stamp { 0% { opacity: 0; transform: rotate(-8deg) scale(1.8); } 60% { opacity: 1; transform: rotate(-8deg) scale(0.94); } 100% { transform: rotate(-8deg) scale(1); } }

	/* 5 Ledger: quiet, dense, keyboard first. */
	.ledger { border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); overflow: hidden; }
	.ledger:focus { outline: none; }
	.lg-head, .lg-row { display: grid; grid-template-columns: minmax(0, 2.4fr) minmax(0, 1.2fr) 110px 132px; align-items: center; gap: 12px; padding: 0 16px; }
	.lg-head { min-height: 36px; border-bottom: 1px solid var(--line); background: var(--surface-2); color: var(--text-muted); font: var(--t-caption); font-weight: 600; letter-spacing: 0.04em; text-transform: uppercase; }
	.lg-row { position: relative; min-height: 56px; transition: background var(--dur-fast) var(--ease-out); }
	.lg-row + .lg-row { border-top: 1px solid var(--line); }
	.lg-row:hover { background: var(--surface-2); }
	.lg-row.cur::before { content: ''; position: absolute; left: 0; top: 10px; bottom: 10px; width: 3px; border-radius: 0 3px 3px 0; background: var(--accent); }
	.lg-title { display: flex; align-items: center; gap: 12px; min-width: 0; }
	.lg-pic { flex: none; width: 28px; aspect-ratio: 2 / 3; --poster-ratio: 2 / 3; border-radius: 4px; overflow: hidden; background: var(--surface-3); }
	.lg-title button { display: flex; flex-direction: column; min-width: 0; padding: 4px 0; border: 0; background: none; text-align: left; cursor: pointer; border-radius: 4px; }
	.lg-title strong { font: var(--t-subhead); font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.lg-title span { color: var(--text-muted); font: var(--t-caption); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.lg-who { display: flex; align-items: center; gap: 6px; min-width: 0; font: var(--t-footnote); white-space: nowrap; overflow: hidden; }
	.lg-when { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.lg-act { display: flex; justify-content: flex-end; gap: 6px; }
	.lg-keys { margin: 0; padding: 10px 16px; border-top: 1px solid var(--line); }
	kbd { padding: 1px 6px; border-radius: 4px; background: var(--surface-3); box-shadow: inset 0 -1px 0 var(--line); font: var(--t-caption); font-weight: 600; }

	@media (max-width: 767px) {
		.sp-stage { min-height: 0; padding: 18px; }
		.sp-in { flex-direction: column; align-items: flex-start; gap: 16px; }
		.sp-art { width: 140px; }
		.sp-txt h2 { font: var(--t-title2); }
		.sp-act, .sp-act .answer { width: 100%; }
		.sp-act :global(.ui-btn) { flex: 1 1 auto; }
		.sp-nav { top: 12px; right: 12px; }
		.hint:not(.lg-keys), .lg-keys { display: none; }
		.board { grid-template-columns: repeat(4, 82vw); margin: 0 calc(-1 * var(--gutter, 16px)); padding: 0 var(--gutter, 16px) 8px; scroll-padding: 0 var(--gutter, 16px); }
		.tk-art { width: 96px; }
		.tk-body { padding: 14px 14px 14px 20px; }
		.stamp-mark { right: 10px; top: 10px; }
		.lg-head { display: none; }
		.lg-row { grid-template-columns: minmax(0, 1fr) auto; grid-template-areas: 'title act' 'who act'; padding: 10px 14px; gap: 4px 10px; }
		.lg-title { grid-area: title; }
		.lg-who { grid-area: who; padding-left: 40px; color: var(--text-muted); }
		.lg-when { display: none; }
		.lg-act { grid-area: act; }
	}
	@media (prefers-reduced-motion: reduce) {
		.bubble, .sp-light, .sp-in, .stamp-mark { animation: none; }
		.tk, .bc, .st, .sp-art { transition: none; }
	}
</style>
