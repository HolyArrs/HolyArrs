<script lang="ts">
	// Approvals the Explore way: poster shelves (Movies, TV shows, Music) for the
	// status tab you are on. Hover a poster for a quick yes or no; click it for a
	// window with that ask on the Stage (trailer banner, Choose what to get, rule).
	import { tick } from 'svelte';
	import Shelf from '../ui/Shelf.svelte';
	import Poster from '../Poster.svelte';
	import Icon, { type IconName } from '../Icon.svelte';
	import Avatar from '../ui/Avatar.svelte';
	import ApprovalsStage from './ApprovalsStage.svelte';
	import { ago } from '../format.ts';
	import type { ApprovalsCtx } from './approvals.ts';
	import { artistPhoto } from './details.svelte.ts';
	import { inboxTabs } from './inbox.ts';
	import { lifecycle } from './lifecycle.ts';
	import type { Ask } from '../types.ts';

	let { ctx }: { ctx: ApprovalsCtx } = $props();

	const waiting = (a: Ask) => ctx.approver && a.state === 'asked' && !ctx.isPending(a.id);
	const tabs = $derived(inboxTabs.filter((t) => ctx.approver || t.tab !== 'failed'));
	const newest = $derived([...ctx.shown].sort((x, y) => Date.parse(y.asked_at) - Date.parse(x.asked_at)));
	const shelves = $derived(
		(
			[
				{ k: 'movie', title: 'Movies', icon: 'film' },
				{ k: 'show', title: 'TV shows', icon: 'tv' },
				{ k: 'artist', title: 'Music', icon: 'music' }
			] as { k: string; title: string; icon: IconName }[]
		)
			.map((s) => ({ ...s, asks: newest.filter((a) => (s.k === 'movie' ? a.kind !== 'show' && a.kind !== 'artist' : a.kind === s.k)) }))
			.filter((s) => s.asks.length)
	);
	const waitingAll = $derived(newest.filter(waiting));

	// The window: one ask on the Stage.
	let win = $state<HTMLDialogElement>();
	let openId = $state(0);
	const openAsk = $derived(ctx.viewed.find((a) => a.id === openId) ?? null);
	let from: HTMLElement | null = null;
	async function show(a: Ask, el: HTMLElement) {
		openId = a.id;
		from = el;
		await tick();
		if (win && !win.open) win.showModal();
	}
	function hide() {
		if (win?.open) win.close();
	}
	// Answered inside the window: let the answer land, then close.
	let wasWaiting = false;
	$effect(() => {
		const w = !!openAsk && waiting(openAsk);
		if (wasWaiting && !w && win?.open) setTimeout(hide, 650);
		wasWaiting = w;
	});
	function cancel(e: Event) {
		// Esc first folds the "What to get" sheet, then closes the window.
		const back = win?.querySelector<HTMLElement>('.frame.choosing .sh-h .x');
		if (back) {
			e.preventDefault();
			back.click();
		}
	}
</script>

<div class="shelves" data-testid="approvals-shelves">
	<header class="head">
		<div class="tabs" role="tablist" aria-label="Show asks that are">
			{#each tabs as t (t.tab)}
				<button type="button" role="tab" class="tab" class:wait={t.tab === 'waiting'} aria-selected={ctx.tab === t.tab} onclick={() => ctx.setTab(t.tab)}>{t.label}{#if ctx.counts[t.tab]}<span class="n">{ctx.counts[t.tab]}</span>{/if}</button>
			{/each}
		</div>
		{#if ctx.tab === 'waiting' && waitingAll.length > 1}
			<button type="button" class="all" onclick={() => ctx.approve(waitingAll)}><Icon name="check" size={16} />Approve all {waitingAll.length}</button>
		{/if}
	</header>

	{#each shelves as s (s.k)}
		<Shelf title={s.title} subtitle={`${s.asks.length} ${s.asks.length === 1 ? 'ask' : 'asks'}`} item="var(--ask-item)" items={s.asks} key={(a) => a.id} lines={2}>
			{#snippet card(a: Ask)}
				{@const w = ctx.who(a)}
				{@const life = lifecycle(a)}
				<div class="sc-card" class:answered={!waiting(a)}>
					<div class="sc-art">
						<button type="button" class="sc-hit" aria-label="Open {a.name}, asked by {w.name}" aria-haspopup="dialog" onclick={(e) => void show(a, e.currentTarget)}>
							<span class="sc-pic">{#if a.kind === 'artist' && artistPhoto(a)}<img src={artistPhoto(a)} alt="" loading="lazy" decoding="async" />{:else}<Poster src={a.poster} name={a.name} />{/if}</span>
						</button>
						<span class="sc-av" title="Asked by {w.name}"><Avatar name={w.name} src={w.avatar} size={26} /></span>
						{#if waiting(a)}
							<span class="sc-quick">
								<button type="button" class="q no" aria-label="Turn down {a.name}" data-testid="ask-decline" onclick={() => ctx.turnDown([a])}><Icon name="x" size={18} /></button>
								<button type="button" class="q yes" aria-label="Approve {a.name}" data-testid="ask-approve" onclick={() => ctx.approve([a])}><Icon name="check" size={18} /></button>
							</span>
						{:else}
							<span class="sc-tag tone-{life.tone}">{life.stage === 'getting' ? 'On its way' : life.label}</span>
						{/if}
						<span class="sc-life tone-{life.tone}"><span class="sc-fill" style:width="{life.pct}%"></span></span>
					</div>
					<span class="sc-name">{a.name}</span>
					<span class="sc-sub">{w.name} · {ago(a.asked_at)}</span>
				</div>
			{/snippet}
		</Shelf>
	{:else}
		<div class="calm"><Icon name="check" size={26} /><h2>{ctx.tab === 'waiting' ? 'You’re all caught up' : 'Nothing here right now'}</h2><p>{ctx.tab === 'waiting' ? 'New asks land here the moment someone asks.' : 'Pick another tab above.'}</p></div>
	{/each}
</div>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
<dialog class="win" bind:this={win} aria-label={openAsk?.name ?? 'Ask'} oncancel={cancel} onclose={() => { openId = 0; from?.focus({ preventScroll: true }); }} onclick={(e) => e.target === win && hide()}>
	{#if openAsk}
		<button type="button" class="close" aria-label="Close" onclick={hide}><Icon name="x" size={18} /></button>
		<div class="win-in"><ApprovalsStage {ctx} single={openAsk} /></div>
	{/if}
</dialog>

<style>
	.shelves { --ask-item: 168px; display: flex; flex-direction: column; gap: 28px; }
	button { font: inherit; color: inherit; }
	button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 12px; }
	.tabs { position: relative; display: inline-flex; gap: 4px; }
	.tab { display: inline-flex; align-items: baseline; gap: 6px; min-height: 38px; padding: 0 12px; border: 0; border-radius: 10px; background: none; color: var(--text-muted); font: 600 15px/1 var(--font-ui); cursor: pointer; transition: color 220ms var(--ease-out), background 220ms var(--ease-out); }
	.tab:hover { color: var(--text); }
	.tab[aria-selected='true'] { color: var(--text); background: var(--surface-2); }
	.n { color: var(--text-muted); font: 600 12px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.tab.wait .n { color: var(--wait-text, var(--wait)); }
	.all { display: inline-flex; align-items: center; gap: 6px; min-height: 38px; padding: 0 16px; border: 0; border-radius: var(--r-pill); background: var(--text); color: var(--bg); font: var(--t-subhead); font-weight: 600; cursor: pointer; transition: transform 160ms var(--ease-out); }
	.all:active { transform: scale(0.96); }

	/* A poster card: the poster opens the window; a quick yes or no rises on hover. */
	.sc-card { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
	.sc-art { position: relative; margin-bottom: 8px; border-radius: 10px; overflow: hidden; box-shadow: 0 0 0 1px var(--line); transition: transform 300ms cubic-bezier(0.22, 1, 0.36, 1), box-shadow 300ms var(--ease-out); }
	.sc-card:hover .sc-art, .sc-card:focus-within .sc-art { transform: translateY(-4px); box-shadow: 0 0 0 1px var(--line), 0 18px 34px -14px rgb(0 0 0 / 0.6); }
	.sc-hit { display: block; width: 100%; padding: 0; border: 0; background: none; cursor: pointer; }
	.sc-pic { display: block; aspect-ratio: 2 / 3; --poster-ratio: 2 / 3; background: var(--surface-3); }
	.sc-pic img { width: 100%; height: 100%; object-fit: cover; object-position: center 25%; display: block; }
	.sc-av { position: absolute; right: 8px; top: 8px; display: inline-flex; border-radius: 50%; box-shadow: 0 0 0 2px rgb(0 0 0 / 0.35); pointer-events: none; }
	.sc-quick { position: absolute; left: 8px; right: 8px; bottom: 12px; display: grid; grid-template-columns: 1fr 1fr; gap: 6px; opacity: 0; transform: translateY(8px); transition: opacity 220ms var(--ease-out), transform 260ms cubic-bezier(0.22, 1, 0.36, 1); }
	.sc-card:hover .sc-quick, .sc-card:focus-within .sc-quick { opacity: 1; transform: none; }
	@media (hover: none) { .sc-quick { opacity: 1; transform: none; } }
	.q { display: grid; place-items: center; height: 36px; border: 0; border-radius: var(--r-pill); cursor: pointer; -webkit-backdrop-filter: blur(14px); backdrop-filter: blur(14px); transition: transform 140ms var(--ease-out), background 200ms var(--ease-out); }
	.q:active { transform: scale(0.92); }
	.q.no { background: rgb(20 20 24 / 0.55); color: #fff; box-shadow: inset 0 0 0 1px rgb(255 255 255 / 0.18); }
	.q.no:hover { background: rgb(20 20 24 / 0.75); }
	.q.yes { background: #fff; color: #0b0b0c; }
	.sc-tag { position: absolute; left: 8px; bottom: 12px; padding: 3px 9px; border-radius: var(--r-pill); background: rgb(12 12 14 / 0.65); color: #fff; font: 600 11px/1.3 var(--font-ui); -webkit-backdrop-filter: blur(10px); backdrop-filter: blur(10px); }
	.sc-life { position: absolute; left: 0; right: 0; bottom: 0; height: 3px; background: rgb(0 0 0 / 0.35); }
	.sc-fill { display: block; height: 100%; transition: width 900ms cubic-bezier(0.22, 1, 0.36, 1); }
	.tone-wait .sc-fill { background: var(--wait); }
	.tone-go .sc-fill { background: var(--accent); }
	.tone-ok .sc-fill { background: var(--ok); }
	.tone-off .sc-fill { background: rgb(255 255 255 / 0.4); }
	.tone-bad .sc-fill { background: var(--fail, #e5484d); }
	.sc-name { font: var(--t-subhead); font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.sc-sub { color: var(--text-muted); font: var(--t-caption); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.answered .sc-pic { filter: saturate(0.85); }

	.calm { display: grid; place-content: center; justify-items: center; gap: 6px; min-height: 320px; border-radius: 20px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); color: var(--text-muted); text-align: center; }
	.calm h2 { color: var(--text); font: var(--t-title3); }

	/* The window: the Stage for one ask. */
	.win { width: min(1180px, calc(100vw - 48px)); max-width: none; height: min(760px, calc(100dvh - 48px)); max-height: none; margin: auto; padding: 0; border: 0; border-radius: 24px; background: #0b0b0c; overflow: hidden; box-shadow: 0 0 0 1px rgb(255 255 255 / 0.08), 0 50px 120px -30px rgb(0 0 0 / 0.85); }
	.win[open] { animation: win-in 420ms cubic-bezier(0.22, 1, 0.36, 1) both; }
	.win::backdrop { background: rgb(0 0 0 / 0.6); -webkit-backdrop-filter: blur(8px); backdrop-filter: blur(8px); animation: fade-in 300ms var(--ease-out) both; }
	.win-in { height: 100%; }
	.close { position: absolute; top: 18px; right: 18px; z-index: 10; display: grid; place-items: center; width: 38px; height: 38px; border: 0; border-radius: 50%; background: rgb(20 20 24 / 0.55); color: #fff; box-shadow: inset 0 0 0 1px rgb(255 255 255 / 0.18); -webkit-backdrop-filter: blur(14px); backdrop-filter: blur(14px); cursor: pointer; transition: background 200ms var(--ease-out); }
	.close:hover { background: rgb(20 20 24 / 0.8); }
	/* The trailer's own controls sit top right too; keep them clear of the close button. */
	.win :global(.ctl.top) { right: 68px !important; }
	@keyframes win-in { from { opacity: 0; transform: translateY(16px) scale(0.98); } }
	@keyframes fade-in { from { opacity: 0; } }
	@media (prefers-reduced-motion: reduce) {
		.sc-art, .sc-quick, .sc-fill, .win[open], .win::backdrop { transition: none; animation: none; }
	}
</style>
