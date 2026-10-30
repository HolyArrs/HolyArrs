<script lang="ts">
	// Five Approvals layouts Luis is comparing (pill bar, ?view=1…5):
	// 1 Inbox, 2 Ticket rows, 3 Ticket wall, 4 Ledger + ticket, 5 By day.
	// Approvals is a quick pass: see who asked for what, answer, move on. So each
	// layout is a list or tickets you scan with the status tabs above, every answer
	// is one click, and the full ask opens in a window that closes cleanly.
	import { flip } from 'svelte/animate';
	import { slide } from 'svelte/transition';
	import Poster from '../Poster.svelte';
	import Avatar from '../ui/Avatar.svelte';
	import Button from '../ui/Button.svelte';
	import IconButton from '../ui/IconButton.svelte';
	import StatusPill from '../ui/StatusPill.svelte';
	import KidMark from './KidMark.svelte';
	import { ago, dateTime } from '../format.ts';
	import { kindWord, type ApprovalsCtx } from './approvals.ts';
	import { artistPhoto } from './details.svelte.ts';
	import { dateGroups } from './inbox.ts';
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
		/** Opens the ask in its window (seasons, folder, rules, note). */
		open: (a: Ask, el?: HTMLElement | null) => void;
	} = $props();

	const waiting = (a: Ask) => ctx.approver && a.state === 'asked' && !ctx.isPending(a.id);
	const art = (a: Ask) => (a.kind === 'artist' ? artistPhoto(a) : a.poster) ?? null;
	const shown = $derived([...ctx.shown].sort((x, y) => Date.parse(y.asked_at) - Date.parse(x.asked_at)));
	const days = $derived(dateGroups(shown));
	/** The answer in words, as the asker would hear it. */
	const verdict = (a: Ask) => (a.state === 'turned_down' ? 'Turned down' : a.state === 'available' ? 'Ready to watch' : 'Approved');

	// 4 Ledger + ticket: one row opens into its ticket at a time.
	let openRow = $state(0);
</script>

{#snippet pic(a: Ask, cls: string)}
	{@const src = art(a)}
	<span class={cls}>{#if a.kind === 'artist' && src}<img {src} alt="" loading="lazy" decoding="async" />{:else}<Poster src={a.kind === 'artist' ? null : src} name={a.name} />{/if}</span>
{/snippet}
{#snippet fill(a: Ask, cls: string)}
	<!-- A picture that always fills its frame: cover-cropped, top-weighted. -->
	{@const src = art(a)}
	{#if src}<span class="{cls} fill" aria-hidden="true" style:background-image={`url("${src}")`}></span>{:else}<span class="{cls} fill blank" aria-hidden="true"><Poster src={null} name={a.name} /></span>{/if}
{/snippet}
{#snippet answer(a: Ask, size: 'sm' | 'md' | 'lg' = 'md')}
	{#if waiting(a)}
		<span class="answer">
			<Button {size} variant="primary" icon="check" type="button" data-testid="ask-approve" onclick={() => ctx.approve([a])}>Approve</Button>
			<Button {size} icon="x" type="button" data-testid="ask-decline" onclick={() => ctx.turnDown([a])}>Turn down</Button>
		</span>
	{:else if ctx.isPending(a.id)}
		<span class="sending">Sending your answer…</span>
	{:else}
		<StatusPill kind={ctx.pill(a).kind} label={ctx.pill(a).label} />
	{/if}
{/snippet}
{#snippet who(a: Ask, size = 20)}
	{@const w = ctx.who(a)}
	<span class="who"><Avatar name={w.name} src={w.avatar} size={size} /><span class="wn">{w.name}</span>{#if w.kid}<KidMark label="Kid" size="sm" />{/if}</span>
{/snippet}
{#snippet stub(a: Ask)}
	{#if !waiting(a) && a.state !== 'asked' && !ctx.isPending(a.id)}
		<span class="stamp" class:no={a.state === 'turned_down'} aria-hidden="true">{verdict(a)}</span>
	{/if}
{/snippet}

{#if look === 1}
	<!-- 1 Inbox: one line per ask, unanswered ones bold with a dot. -->
	<ul class="inbox" data-look="inbox" aria-label="Asks">
		{#each shown as a (a.id)}
			<li class="ib" class:unread={waiting(a)} animate:flip={{ duration: 240 }}>
				<span class="ib-dot" aria-hidden="true"></span>
				{@render pic(a, 'ib-pic')}
				<button class="ib-main" type="button" onclick={(e) => open(a, e.currentTarget)}>
					<span class="ib-l1"><strong>{ctx.who(a).name}</strong> asked for <strong>{a.name}</strong></span>
					<span class="ib-l2">{kindWord(a.kind)} · {line(a)}{#if a.note} — &ldquo;{a.note}&rdquo;{/if}</span>
				</button>
				<time class="ib-when" datetime={a.asked_at} title={dateTime(a.asked_at)}>{ago(a.asked_at)}</time>
				<span class="ib-act">
					{#if waiting(a)}
						<IconButton icon="check" variant="tinted" label="Approve {a.name}" data-testid="ask-approve" onclick={() => ctx.approve([a])} />
						<IconButton icon="x" variant="tinted" label="Turn down {a.name}" data-testid="ask-decline" onclick={() => ctx.turnDown([a])} />
					{:else}{@render answer(a, 'sm')}{/if}
				</span>
			</li>
		{/each}
	</ul>
{:else if look === 2}
	<!-- 2 Ticket rows: a wide ticket per ask; the answer is the tear-off stub. -->
	<ul class="trows" data-look="ticket-rows" aria-label="Asks">
		{#each shown as a (a.id)}
			<li class="tr" class:done={!waiting(a) && a.state !== 'asked'} animate:flip={{ duration: 260 }}>
				<button class="tr-art" type="button" aria-label="Open {a.name}" onclick={(e) => open(a, e.currentTarget)}>{@render fill(a, 'tr-img')}</button>
				<div class="tr-body">
					<span class="eyebrow">{kindWord(a.kind)}{a.year ? ` · ${a.year}` : ''}</span>
					<button class="tr-name" type="button" onclick={(e) => open(a, e.currentTarget)}>{a.name}</button>
					{#if a.kind !== 'movie'}<span class="tr-what">{line(a)}</span>{/if}
					<span class="tr-who">{@render who(a, 20)}<span class="sep">·</span><time datetime={a.asked_at} title={dateTime(a.asked_at)}>asked {ago(a.asked_at)}</time></span>
					{#if a.note}<p class="note">&ldquo;{a.note}&rdquo;</p>{/if}
				</div>
				<div class="tr-stub">
					{#if waiting(a) || ctx.isPending(a.id)}{@render answer(a, 'md')}{:else}{@render stub(a)}{/if}
				</div>
			</li>
		{/each}
	</ul>
{:else if look === 3}
	<!-- 3 Ticket wall: tickets side by side, picture on top, answer at the tear line. -->
	<ul class="twall" data-look="ticket-wall" aria-label="Asks">
		{#each shown as a (a.id)}
			<li class="tw" class:done={!waiting(a) && a.state !== 'asked'} animate:flip={{ duration: 280 }}>
				<button class="tw-art" type="button" aria-label="Open {a.name}" onclick={(e) => open(a, e.currentTarget)}>
					{@render fill(a, 'tw-img')}
					<span class="tw-who">{@render who(a, 22)}</span>
					{@render stub(a)}
				</button>
				<div class="tw-body">
					<span class="eyebrow">{kindWord(a.kind)}{a.year ? ` · ${a.year}` : ''} · asked {ago(a.asked_at)}</span>
					<button class="tw-name" type="button" onclick={(e) => open(a, e.currentTarget)}>{a.name}</button>
					{#if a.kind !== 'movie'}<span class="tr-what">{line(a)}</span>{/if}
					{#if a.note}<p class="note">&ldquo;{a.note}&rdquo;</p>{/if}
				</div>
				<div class="tw-foot">{@render answer(a, 'md')}</div>
			</li>
		{/each}
	</ul>
{:else if look === 4}
	<!-- 4 Ledger + ticket: a quiet table; a row opens into its ticket in place. -->
	<div class="ledger" data-look="ledger-ticket">
		<div class="lg-head" aria-hidden="true"><span>Title</span><span>Asked by</span><span>When</span><span></span></div>
		<ul aria-label="Asks">
			{#each shown as a (a.id)}
				{@const on = openRow === a.id}
				<li class="lg" class:on animate:flip={{ duration: 220 }}>
					<div class="lg-row">
						<button class="lg-title" type="button" aria-expanded={on} onclick={() => (openRow = on ? 0 : a.id)}>
							{@render pic(a, 'lg-pic')}
							<span class="lg-txt"><strong>{a.name}</strong><span>{kindWord(a.kind)} · {line(a)}</span></span>
						</button>
						<span class="lg-who">{@render who(a, 20)}</span>
						<time class="lg-when" datetime={a.asked_at} title={dateTime(a.asked_at)}>{ago(a.asked_at)}</time>
						<span class="lg-act">
							{#if waiting(a)}
								<IconButton icon="check" variant="tinted" size="sm" label="Approve {a.name}" data-testid="ask-approve" onclick={() => ctx.approve([a])} />
								<IconButton icon="x" variant="tinted" size="sm" label="Turn down {a.name}" data-testid="ask-decline" onclick={() => ctx.turnDown([a])} />
							{:else}{@render answer(a, 'sm')}{/if}
						</span>
					</div>
					{#if on}
						<div class="lg-ticket" transition:slide={{ duration: 220 }}>
							{@render fill(a, 'lt-img')}
							<div class="lt-body">
								<span class="eyebrow">{kindWord(a.kind)}{a.year ? ` · ${a.year}` : ''}</span>
								<h3>{a.name}</h3>
								<span class="tr-what">{line(a)}</span>
								<span class="tr-who">{@render who(a, 22)}<span class="sep">·</span>asked {ago(a.asked_at)}</span>
								{#if a.note}<p class="note">&ldquo;{a.note}&rdquo;</p>{/if}
								{#if ctx.decided(a)}<span class="tr-what">{ctx.decided(a)}</span>{/if}
								<div class="lt-act">{@render answer(a, 'md')}<Button size="md" variant="plain" type="button" onclick={(e: MouseEvent) => open(a, e.currentTarget as HTMLElement)}>Seasons, folder and rules</Button></div>
							</div>
						</div>
					{/if}
				</li>
			{/each}
		</ul>
	</div>
{:else if look === 5}
	<!-- 5 By day: what came in today, yesterday, earlier; answer straight from the line. -->
	<div class="days" data-look="by-day">
		{#each days as d (d.id)}
			<section class="day" aria-labelledby="d-{d.id}">
				<h2 id="d-{d.id}">{d.name}<span>{d.asks.filter(waiting).length ? `${d.asks.filter(waiting).length} to answer` : 'All answered'}</span></h2>
				<ul>
					{#each d.asks as a (a.id)}
						<li class="dy" animate:flip={{ duration: 240 }}>
							<button class="dy-art" type="button" aria-label="Open {a.name}" onclick={(e) => open(a, e.currentTarget)}>{@render pic(a, 'dy-pic')}</button>
							<div class="dy-txt">
								<button class="dy-name" type="button" onclick={(e) => open(a, e.currentTarget)}>{a.name}</button>
								<span class="tr-what">{kindWord(a.kind)} · {line(a)}</span>
								<span class="tr-who">{@render who(a, 18)}<span class="sep">·</span><time datetime={a.asked_at} title={dateTime(a.asked_at)}>{ago(a.asked_at)}</time></span>
							</div>
							<div class="dy-act">{@render answer(a, 'md')}</div>
						</li>
					{/each}
				</ul>
			</section>
		{/each}
	</div>
{/if}
{#if ctx.tab === 'waiting' && !shown.some(waiting) && shown.length}
	<p class="clear">You’ve answered everyone. New asks land here the moment someone asks.</p>
{/if}

<style>
	/* Shared pieces: one poster frame, one answer pair, one note style, one stamp. */
	button { font: inherit; color: inherit; }
	button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	ul { margin: 0; padding: 0; list-style: none; }
	.answer { display: inline-flex; flex-wrap: wrap; gap: 8px; }
	.sending { color: var(--text-muted); font: var(--t-footnote); }
	.eyebrow { color: var(--text-muted); font: var(--t-caption); font-weight: 600; letter-spacing: 0.03em; }
	.who { display: inline-flex; align-items: center; gap: 6px; min-width: 0; }
	.wn { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.sep { color: var(--text-faint, var(--text-muted)); }
	.note { margin-top: 6px; padding: 8px 12px; border-radius: 10px; background: var(--surface-2); font: var(--t-footnote); font-style: italic; overflow-wrap: anywhere; }
	.tr-what { color: var(--text-muted); font: var(--t-callout); }
	.tr-who { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; margin-top: 4px; color: var(--text-muted); font: var(--t-footnote); }
	.tr-who .wn { color: var(--text); font-weight: 600; }
	.fill { display: block; background-size: cover; background-position: center 22%; background-color: var(--surface-3); }
	.fill.blank { display: grid; }
	.ib-pic, .lg-pic, .dy-pic { display: block; flex: none; aspect-ratio: 2 / 3; --poster-ratio: 2 / 3; overflow: hidden; background: var(--surface-3); }
	.ib-pic > img, .lg-pic > img, .dy-pic > img { width: 100%; height: 100%; object-fit: cover; object-position: center 25%; display: block; }
	.stamp { position: absolute; padding: 4px 10px; border: 2px solid currentColor; border-radius: 6px; color: var(--ok-text, var(--ok)); background: color-mix(in srgb, var(--bg) 70%, transparent); font: var(--t-caption); font-weight: 800; letter-spacing: 0.1em; text-transform: uppercase; white-space: nowrap; transform: rotate(-7deg); animation: stamp 420ms var(--ease-spring, var(--ease-out)) both; pointer-events: none; }
	.stamp.no { color: var(--text-muted); }
	@keyframes stamp { 0% { opacity: 0; transform: rotate(-7deg) scale(1.8); } 60% { opacity: 1; transform: rotate(-7deg) scale(0.94); } 100% { transform: rotate(-7deg) scale(1); } }
	.clear { margin-top: 20px; padding: 18px 20px; border-radius: var(--r-card); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); color: var(--text-muted); font: var(--t-callout); text-align: center; }

	/* 1 Inbox */
	.inbox { border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); overflow: hidden; }
	.ib { position: relative; display: flex; align-items: center; gap: 12px; min-height: 64px; padding: 8px 12px 8px 10px; transition: background var(--dur-fast) var(--ease-out); }
	.ib + .ib { border-top: 1px solid var(--line); }
	.ib:hover { background: var(--surface-2); }
	.ib-dot { flex: none; width: 8px; height: 8px; border-radius: 50%; background: transparent; }
	.ib.unread .ib-dot { background: var(--accent); }
	.ib-pic { width: 34px; border-radius: 5px; }
	.ib-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; padding: 4px 0; border: 0; background: none; text-align: left; cursor: pointer; border-radius: 6px; }
	.ib-main::after { content: ''; position: absolute; inset: 0; }
	.ib-l1 { font: var(--t-subhead); color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.ib-l1 strong { color: var(--text); font-weight: 500; }
	.ib.unread .ib-l1 strong { font-weight: 700; }
	.ib-l2 { color: var(--text-muted); font: var(--t-footnote); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.ib-when { flex: none; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.ib.unread .ib-when { color: var(--accent-text); font-weight: 600; }
	.ib-act { position: relative; z-index: 1; flex: none; display: inline-flex; gap: 6px; }
	@media (hover: hover) and (min-width: 768px) {
		.ib.unread .ib-act { opacity: 0; transition: opacity var(--dur-fast) var(--ease-out); }
		.ib.unread:hover .ib-act, .ib.unread:focus-within .ib-act { opacity: 1; }
		.ib.unread:hover .ib-when, .ib.unread:focus-within .ib-when { display: none; }
	}

	/* 2 Ticket rows: picture stub | body | perforated tear-off with the answer. */
	.trows { display: flex; flex-direction: column; gap: 12px; }
	.tr { position: relative; display: grid; grid-template-columns: 112px minmax(0, 1fr) 220px; min-height: 168px; border-radius: var(--r-card); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); overflow: hidden; transition: transform var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out); }
	.tr:hover { transform: translateY(-1px); box-shadow: 0 0 0 1px var(--line-strong, var(--line)), var(--shadow-card, none); }
	.tr-art { position: relative; padding: 0; border: 0; background: none; cursor: pointer; }
	.tr-img { position: absolute; inset: 0; }
	.tr-body { display: flex; flex-direction: column; align-items: flex-start; gap: 2px; min-width: 0; padding: 16px 20px; }
	.tr-name, .tw-name, .dy-name { max-width: 100%; padding: 0; border: 0; background: none; text-align: left; font: var(--t-title3); cursor: pointer; overflow-wrap: anywhere; border-radius: 4px; }
	.tr-name:hover, .tw-name:hover, .dy-name:hover { text-decoration: underline; text-decoration-color: color-mix(in srgb, currentColor 35%, transparent); text-underline-offset: 4px; }
	.tr-stub { position: relative; display: flex; align-items: center; justify-content: center; padding: 16px; border-left: 2px dashed var(--line); background: color-mix(in srgb, var(--surface-2) 50%, transparent); }
	.tr-stub::before, .tr-stub::after { content: ''; position: absolute; left: -11px; width: 20px; height: 20px; border-radius: 50%; background: var(--bg); box-shadow: inset 0 0 0 1px var(--line); }
	.tr-stub::before { top: -10px; }
	.tr-stub::after { bottom: -10px; }
	.tr-stub .answer { flex-direction: column; width: 100%; }
	.tr-stub .stamp { position: static; }
	.tr.done .tr-art, .tr.done .tr-body { opacity: 0.75; }

	/* 3 Ticket wall */
	.twall { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(260px, 100%), 1fr)); gap: 18px; }
	.tw { position: relative; display: flex; flex-direction: column; border-radius: var(--r-card); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); overflow: hidden; transition: transform var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out); }
	.tw:hover { transform: translateY(-2px); box-shadow: 0 0 0 1px var(--line-strong, var(--line)), var(--shadow-lift, none); }
	.tw-art { position: relative; display: block; aspect-ratio: 4 / 3; padding: 0; border: 0; background: none; cursor: pointer; overflow: hidden; }
	.tw-img { position: absolute; inset: 0; transition: transform var(--dur) var(--ease-out); }
	.tw-art:hover .tw-img { transform: scale(1.03); }
	.tw-art::after { content: ''; position: absolute; inset: 0; background: linear-gradient(transparent 55%, rgb(0 0 0 / 0.55)); pointer-events: none; }
	.tw-who { position: absolute; left: 12px; bottom: 10px; z-index: 1; color: #fff; font: var(--t-footnote); font-weight: 600; text-shadow: 0 1px 2px rgb(0 0 0 / 0.5); }
	.tw-art .stamp { right: 12px; top: 12px; z-index: 1; }
	.tw-body { display: flex; flex-direction: column; align-items: flex-start; gap: 2px; padding: 14px 16px 12px; }
	.tw-name { font: var(--t-headline); }
	.tw-foot { position: relative; margin-top: auto; padding: 12px 16px 14px; border-top: 2px dashed var(--line); }
	.tw-foot::before, .tw-foot::after { content: ''; position: absolute; top: -11px; width: 20px; height: 20px; border-radius: 50%; background: var(--bg); box-shadow: inset 0 0 0 1px var(--line); }
	.tw-foot::before { left: -10px; }
	.tw-foot::after { right: -10px; }
	.tw-foot .answer { display: grid; grid-template-columns: 1fr 1fr; width: 100%; }
	.tw.done .tw-body { opacity: 0.75; }

	/* 4 Ledger + ticket */
	.ledger { border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); overflow: hidden; }
	.lg-head, .lg-row { display: grid; grid-template-columns: minmax(0, 2.4fr) minmax(0, 1.2fr) 100px 96px; align-items: center; gap: 12px; padding: 0 16px; }
	.lg-head { min-height: 34px; border-bottom: 1px solid var(--line); background: var(--surface-2); color: var(--text-muted); font: var(--t-caption); font-weight: 600; letter-spacing: 0.04em; text-transform: uppercase; }
	.lg + .lg { border-top: 1px solid var(--line); }
	.lg-row { min-height: 56px; transition: background var(--dur-fast) var(--ease-out); }
	.lg-row:hover, .lg.on .lg-row { background: var(--surface-2); }
	.lg-title { display: flex; align-items: center; gap: 12px; min-width: 0; padding: 6px 0; border: 0; background: none; text-align: left; cursor: pointer; border-radius: 6px; }
	.lg-pic { width: 28px; border-radius: 4px; }
	.lg-txt { display: flex; flex-direction: column; min-width: 0; }
	.lg-txt strong { font: var(--t-subhead); font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.lg-txt span { color: var(--text-muted); font: var(--t-caption); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.lg-who { min-width: 0; font: var(--t-footnote); }
	.lg-when { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.lg-act { display: flex; justify-content: flex-end; gap: 6px; }
	.lg-ticket { display: grid; grid-template-columns: 120px minmax(0, 1fr); margin: 0 16px 16px; border-radius: var(--r-card); background: var(--bg); box-shadow: 0 0 0 1px var(--line); overflow: hidden; }
	.lt-img { min-height: 180px; }
	.lt-body { display: flex; flex-direction: column; align-items: flex-start; gap: 2px; padding: 16px 18px; border-left: 2px dashed var(--line); }
	.lt-body h3 { font: var(--t-title3); }
	.lt-act { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 12px; }

	/* 5 By day */
	.days { display: flex; flex-direction: column; gap: 22px; }
	.day h2 { display: flex; align-items: baseline; gap: 10px; margin: 0 4px 8px; font: var(--t-title3); }
	.day h2 span { color: var(--text-muted); font: var(--t-footnote); font-weight: 400; }
	.day ul { border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); overflow: hidden; }
	.dy { display: flex; align-items: center; gap: 14px; padding: 12px 16px; }
	.dy + .dy { border-top: 1px solid var(--line); }
	.dy-art { flex: none; padding: 0; border: 0; background: none; cursor: pointer; border-radius: 6px; overflow: hidden; }
	.dy-pic { width: 52px; }
	.dy-txt { flex: 1; min-width: 0; display: flex; flex-direction: column; align-items: flex-start; gap: 1px; }
	.dy-name { font: var(--t-headline); }
	.dy-act { flex: none; }

	@media (max-width: 767px) {
		.ib { gap: 10px; padding: 10px; }
		.ib-act { display: none; }
		.ib.unread .ib-act { display: inline-flex; }
		.ib.unread .ib-when { display: none; }
		.tr { grid-template-columns: 96px minmax(0, 1fr); min-height: 0; }
		.tr-body { padding: 14px; }
		.tr-stub { grid-column: 1 / -1; border-left: 0; border-top: 2px dashed var(--line); padding: 12px 14px; }
		.tr-stub::before { top: -11px; left: -10px; }
		.tr-stub::after { top: -11px; bottom: auto; left: auto; right: -10px; }
		.tr-stub .answer { display: grid; grid-template-columns: 1fr 1fr; }
		.tr-name { font: var(--t-headline); }
		.twall { grid-template-columns: minmax(0, 1fr); }
		.lg-head { display: none; }
		.lg-row { grid-template-columns: minmax(0, 1fr) auto; grid-template-areas: 'title act' 'who act'; padding: 8px 14px; gap: 2px 10px; }
		.lg-title { grid-area: title; }
		.lg-who { grid-area: who; padding-left: 40px; color: var(--text-muted); }
		.lg-when { display: none; }
		.lg-act { grid-area: act; }
		.lg-ticket { grid-template-columns: 88px minmax(0, 1fr); margin: 0 12px 12px; }
		.lt-act, .lt-act .answer { width: 100%; }
		.dy { flex-wrap: wrap; padding: 12px; }
		.dy-pic { width: 44px; }
		.dy-act { width: 100%; padding-left: 58px; }
		.dy-act .answer { display: grid; grid-template-columns: 1fr 1fr; }
	}
	@media (prefers-reduced-motion: reduce) {
		.stamp { animation: none; }
		.tr, .tw, .tw-img { transition: none; }
	}
</style>
