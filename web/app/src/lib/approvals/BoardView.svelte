<script lang="ts">
	import { glide } from '../ui/transitions.ts';
	// 5 Board: every ask on one board, a column per state. Drag a waiting card
	// to Approved or Turned down, or use its ✓ / ✕. On a phone a segmented
	// switch shows one column at a time.
	import Poster from '../Poster.svelte';
	import Icon from '../Icon.svelte';
	import Avatar from '../ui/Avatar.svelte';
	import KidMark from './KidMark.svelte';
	import { to } from '../paths.ts';
	import { ago } from '../format.ts';
	import { boardColumn, boardColumns, kindWord, type ApprovalsCtx, type BoardColumn } from './approvals.ts';
	import type { Ask } from '../types.ts';

	let { ctx }: { ctx: ApprovalsCtx } = $props();
	const cols = $derived(
		boardColumns.map((c) => ({ ...c, asks: ctx.viewed.filter((a) => boardColumn(a, ctx.downloading) === c.col) }))
	);
	let phoneCol = $state<BoardColumn>('waiting');
	let dragId = $state(0);
	let over = $state<BoardColumn | ''>('');
	const dragged = $derived(ctx.viewed.find((a) => a.id === dragId) ?? null);
	const accepts = (c: BoardColumn) => !!dragged && dragged.state === 'asked' && ctx.approver && (c === 'looking' || c === 'turned_down');

	function drop(c: BoardColumn) {
		const a = dragged;
		dragId = 0;
		over = '';
		if (!a || !accepts(c)) return;
		if (c === 'looking') ctx.approve([a]);
		else ctx.turnDown([a]);
	}
	function start(e: DragEvent, a: Ask) {
		dragId = a.id;
		e.dataTransfer?.setData('text/plain', String(a.id));
		if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
	}
	const waiting = $derived(cols[0].asks.filter((a) => !ctx.isPending(a.id)));
	let strip: HTMLElement | undefined = $state();
	function stripKeys(e: KeyboardEvent, i: number) {
		const step = e.key === 'ArrowRight' ? 1 : e.key === 'ArrowLeft' ? -1 : 0;
		if (!step) return;
		e.preventDefault();
		const n = (i + step + boardColumns.length) % boardColumns.length;
		phoneCol = boardColumns[n].col;
		strip?.querySelectorAll<HTMLButtonElement>('button')[n]?.focus();
	}
</script>

<div class="seg" role="radiogroup" aria-label="Column" bind:this={strip}>
	{#each cols as c, i (c.col)}
		<button type="button" role="radio" aria-checked={phoneCol === c.col} tabindex={phoneCol === c.col ? 0 : -1} class="dot-{c.col}" onclick={() => (phoneCol = c.col)} onkeydown={(e) => stripKeys(e, i)}>
			<i aria-hidden="true"></i>{c.short}<b>{c.asks.length}</b>
		</button>
	{/each}
</div>

{#if ctx.approver && waiting.length > 1}
	<div class="board-tools">
		<span class="muted">{waiting.length} waiting</span>
		<button class="tool yes" type="button" onclick={() => ctx.approve(waiting)}><Icon name="check" size={16} />Approve all waiting</button>
	</div>
{/if}

<div class="board" class:dragging={!!dragged}>
	{#each cols as c (c.col)}
		<section class="col col-{c.col}" class:phone-on={phoneCol === c.col} class:target={accepts(c.col)} class:over={over === c.col && accepts(c.col)} aria-label="{c.label}, {c.asks.length}"
			ondragover={(e) => { if (accepts(c.col)) { e.preventDefault(); over = c.col; } }}
			ondragleave={() => over === c.col && (over = '')}
			ondrop={(e) => (e.preventDefault(), drop(c.col))}>
			<header class="col-head">
				<i class="dot" aria-hidden="true"></i>
				<h2>{c.label}</h2>
				<span class="n">{c.asks.length}</span>
				{#if c.col === 'waiting' && ctx.choosable && c.asks.length}
					<button class="mini-all" type="button" onclick={() => ctx.chooseAll(c.asks)}>{c.asks.every((x) => ctx.isChosen(x.id)) ? 'Clear' : 'Choose all'}</button>
				{/if}
			</header>
			{#if accepts(c.col)}<p class="drop-hint">{c.col === 'looking' ? 'Drop to approve' : 'Drop to turn down'}</p>{/if}
			<ul class="cards">
				{#each c.asks as a (a.id)}
					{@const who = ctx.who(a)}
					{@const canAct = ctx.approver && a.state === 'asked' && !ctx.isPending(a.id)}
					<li animate:glide class="bcard" class:chosen={ctx.isChosen(a.id)} class:sending={ctx.isPending(a.id)} class:lifted={dragId === a.id} draggable={canAct ? 'true' : undefined} ondragstart={(e) => start(e, a)} ondragend={() => ((dragId = 0), (over = ''))} data-testid="ask">
						<div class="c-top">
							{#if c.col === 'waiting' && ctx.choosable}<input class="tick" type="checkbox" aria-label="Choose {a.name}" checked={ctx.isChosen(a.id)} onchange={() => ctx.toggleChosen(a.id)} />{/if}
							<span class="thumb"><Poster src={a.poster} name={a.name} /></span>
							<div class="c-txt">
								<p class="name">{#if ctx.href(a)}<a href={to(ctx.href(a))}>{a.name}</a>{:else}{a.name}{/if}</p>
								<p class="meta">{kindWord(a.kind)}{a.year ? ` · ${a.year}` : ''}</p>
								{#if ctx.what(a)}<p class="part">{ctx.what(a)}</p>{/if}
							</div>
						</div>
						<div class="c-foot">
							<Avatar name={who.name} src={who.avatar} size={18} />
							<span class="who">{who.name}</span>
							{#if who.kid}<KidMark label="Kid" size="sm" />{/if}
							<span class="age">{ago(a.decided_at && a.state !== 'asked' ? a.decided_at : a.asked_at)}</span>
						</div>
						{#if a.note && a.state === 'turned_down'}<p class="note">“{a.note}”</p>{/if}
						<div class="c-acts">
							{#if canAct}
								<button class="b yes" type="button" data-testid="ask-approve" aria-label="Approve {a.name}" onclick={() => ctx.approve([a])}><Icon name="check" size={16} />Approve</button>
								<button class="b no" type="button" data-testid="ask-decline" aria-label="Turn down {a.name}" onclick={() => ctx.turnDown([a])}><Icon name="x" size={16} /></button>
								<button class="b no" type="button" aria-label="Approve {a.name} with options" title="Approve with options" onclick={() => ctx.withOptions(a)}><Icon name="sparkle" size={16} /></button>
							{:else if ctx.isPending(a.id)}
								<span class="sending-line">Sending in a moment…</span>
							{/if}
							{#if canAct}
								<button class="b no" type="button" aria-label="Review {a.name}" title="Review" onclick={(e) => ctx.review(a, e.currentTarget)}><Icon name="eye" size={16} /></button>
							{:else}
								<button class="b review" type="button" aria-label="Review {a.name}" onclick={(e) => ctx.review(a, e.currentTarget)}>Review</button>
							{/if}
						</div>
					</li>
				{:else}
					<li class="none">{c.col === 'waiting' ? 'Nothing waiting. All answered.' : 'Nothing here.'}</li>
				{/each}
			</ul>
		</section>
	{/each}
</div>

<style>
	.board { display: grid; grid-template-columns: repeat(5, minmax(192px, 1fr)); gap: 10px; align-items: start; overflow-x: auto; padding-bottom: 8px; }
	.col { display: flex; flex-direction: column; gap: 8px; min-width: 0; padding: 10px; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); transition: box-shadow var(--dur-fast) var(--ease-out), background var(--dur-fast) var(--ease-out); }
	.col-waiting { --c: var(--mark-ask); }
	.col-looking { --c: var(--accent); }
	.col-downloading { --c: var(--mark-dl); }
	.col-done { --c: var(--ok); }
	.col-turned_down { --c: var(--text-faint); }
	.dragging .col:not(.target) { opacity: 0.55; }
	.col.target { box-shadow: 0 0 0 2px color-mix(in srgb, var(--c) 55%, transparent); }
	.col.over { background: color-mix(in srgb, var(--c) 12%, var(--surface-1)); box-shadow: 0 0 0 2px var(--c); }
	.col-head { display: flex; align-items: center; gap: 8px; padding: 2px 4px 4px; }
	.dot { width: 8px; height: 8px; border-radius: 50%; background: var(--c); flex: none; }
	.col-head h2 { font: var(--t-subhead); font-weight: 700; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.n { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.mini-all { margin-left: auto; min-height: 28px; padding: 0 8px; border-radius: 8px; color: var(--accent-text); font: var(--t-footnote); font-weight: 600; white-space: nowrap; }
	.mini-all:hover { background: var(--accent-soft); }
	.drop-hint { padding: 10px; border-radius: var(--r-card); border: 1.5px dashed var(--c); color: var(--text-muted); font: var(--t-footnote); text-align: center; }
	.cards { display: flex; flex-direction: column; gap: 8px; }
	.bcard { display: flex; flex-direction: column; gap: 8px; padding: 10px; border-radius: var(--r-card); background: var(--bg); box-shadow: 0 0 0 1px var(--line), 0 1px 2px rgb(0 0 0 / 6%); border-left: 3px solid var(--c); transition: transform var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out); }
	.bcard[draggable='true'] { cursor: grab; }
	.bcard:hover { box-shadow: 0 0 0 1px var(--line-strong), var(--shadow-card); }
	.bcard.lifted { opacity: 0.5; transform: rotate(-1.5deg) scale(0.98); }
	.bcard.chosen { box-shadow: 0 0 0 2px var(--accent); }
	.bcard.sending { opacity: 0.6; }
	.c-top { display: flex; gap: 10px; align-items: flex-start; }
	.tick { width: 18px; height: 18px; flex: none; accent-color: var(--accent-fill); }
	.thumb { width: 38px; flex: none; border-radius: 5px; overflow: hidden; }
	.c-txt { min-width: 0; flex: 1; }
	.name { font: var(--t-subhead); font-weight: 600; overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
	.name a:hover { text-decoration: underline; text-underline-offset: 3px; }
	.meta { color: var(--text-muted); font: var(--t-caption); }
	.part { margin-top: 4px; display: inline-block; max-width: 100%; padding: 1px 7px; border-radius: var(--r-pill); background: var(--surface-2); font: var(--t-caption); font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.c-foot { display: flex; align-items: center; gap: 6px; min-width: 0; color: var(--text-muted); font: var(--t-caption); }
	.who { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.age { margin-left: auto; white-space: nowrap; }
	.note { font: var(--t-caption); color: var(--text-muted); overflow-wrap: anywhere; }
	.c-acts { display: flex; gap: 6px; }
	.c-acts:empty { display: none; }
	.b { min-height: 32px; display: inline-flex; align-items: center; justify-content: center; gap: 4px; padding: 0 10px; border-radius: 8px; background: var(--surface-2); font: var(--t-footnote); font-weight: 600; }
	.b:hover { background: var(--surface-3); }
	.b.yes { flex: 1; min-width: 0; background: var(--ok-fill); color: #fff; }
	.b.yes:hover { background: var(--ok-fill-hover); }
	.b.no { width: 32px; padding: 0; }
	.b.review { margin-left: auto; background: transparent; color: var(--accent-text); }
	.b.review:hover { background: var(--accent-soft); }
	.sending-line { flex: 1; font: var(--t-caption); color: var(--text-muted); align-self: center; }
	.none { padding: 18px 8px; border-radius: var(--r-card); border: 1.5px dashed var(--line-strong); color: var(--text-faint); font: var(--t-footnote); text-align: center; }
	.col-waiting .none { color: var(--ok-text); border-color: color-mix(in srgb, var(--ok) 45%, transparent); background: color-mix(in srgb, var(--ok) 7%, transparent); }
	.board-tools { display: flex; align-items: center; justify-content: flex-end; gap: 12px; margin-bottom: 10px; font: var(--t-footnote); }
	.tool { min-height: 36px; display: inline-flex; align-items: center; gap: 6px; padding: 0 14px; border-radius: var(--r-pill); font: var(--t-subhead); font-weight: 600; }
	.tool.yes { background: var(--ok-fill); color: #fff; }
	.seg { display: none; }
	@media (pointer: coarse) { .b { min-height: 40px; } .b.no { width: 40px; } }
	@media (max-width: 767px) {
		.seg { display: flex; gap: 4px; overflow-x: auto; scrollbar-width: none; margin: 0 calc(-1 * var(--gutter)) 12px; padding: 0 var(--gutter); }
		.seg::-webkit-scrollbar { display: none; }
		.seg button { flex: none; display: inline-flex; align-items: center; gap: 6px; min-height: 40px; padding: 0 12px; border-radius: var(--r-pill); background: var(--surface-2); color: var(--text-muted); font: var(--t-subhead); font-weight: 600; white-space: nowrap; }
		.seg button[aria-checked='true'] { background: var(--text); color: var(--bg); }
		.seg i { width: 7px; height: 7px; border-radius: 50%; background: var(--c); }
		.seg b { font-weight: 500; opacity: 0.7; }
		.dot-waiting { --c: var(--mark-ask); }
		.dot-looking { --c: var(--accent); }
		.dot-downloading { --c: var(--mark-dl); }
		.dot-done { --c: var(--ok); }
		.dot-turned_down { --c: var(--text-faint); }
		.board { display: block; overflow: visible; }
		.col { display: none; padding: 0; background: none; box-shadow: none; }
		.col.phone-on { display: flex; }
		.col-head { display: none; }
		.bcard { padding: 12px; }
		.thumb { width: 48px; }
		.name { font: var(--t-headline); }
		.b { min-height: 44px; }
		.b.no { width: 44px; }
		.board-tools { justify-content: space-between; }
	}
</style>
