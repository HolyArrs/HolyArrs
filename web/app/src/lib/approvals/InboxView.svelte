<script lang="ts">
	// 4 Inbox: a compact list on the left, the chosen ask in full on the right
	// with its options (Library folder, Pick rules), a note to the asker and the
	// decision. On a phone the list opens the ask full screen.
	import Poster from '../Poster.svelte';
	import Icon from '../Icon.svelte';
	import Avatar from '../ui/Avatar.svelte';
	import KidMark from './KidMark.svelte';
	import AskDetail from './AskDetail.svelte';
	import { ago } from '../format.ts';
	import { kindWord, type ApprovalsCtx } from './approvals.ts';
	import { askTabs } from '../wishlist.ts';
	import { tick } from 'svelte';

	let { ctx }: { ctx: ApprovalsCtx } = $props();
	let pickedId = $state(0);
	let lastAt = $state(0);
	const at = $derived.by(() => {
		const i = ctx.shown.findIndex((a) => a.id === pickedId);
		return i >= 0 ? i : Math.min(lastAt, Math.max(ctx.shown.length - 1, 0));
	});
	const a = $derived(ctx.shown[at] ?? null);
	let phoneOpen = $state(false);
	let opener: HTMLElement | null = null;

	async function pick(i: number, el: HTMLElement) {
		pickedId = ctx.shown[i]?.id ?? 0;
		lastAt = i;
		opener = el;
		if (window.matchMedia('(max-width: 767px)').matches) {
			phoneOpen = true;
			await tick();
			document.getElementById('inbox-title')?.focus({ preventScroll: true });
		}
	}
	async function back() {
		phoneOpen = false;
		await tick();
		opener?.focus();
	}
	function listKeys(e: KeyboardEvent) {
		if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
		e.preventDefault();
		const rows = [...((e.currentTarget as HTMLElement).closest('ul')?.querySelectorAll<HTMLButtonElement>('.row-btn') ?? [])];
		const i = rows.indexOf(document.activeElement as HTMLButtonElement);
		const n = Math.min(Math.max((i < 0 ? at : i) + (e.key === 'ArrowDown' ? 1 : -1), 0), rows.length - 1);
		rows[n]?.focus();
		pickedId = ctx.shown[n]?.id ?? 0;
		lastAt = n;
	}
</script>

<svelte:window onkeydown={(e) => phoneOpen && e.key === 'Escape' && !document.querySelector('dialog[open]') && void back()} />

<div class="inbox" class:phone-open={phoneOpen}>
	<div class="list-pane">
		{#if ctx.shown.length === 0}
			<div class="calm" data-testid="calm">
				<Icon name="inbox" size={28} />
				<h2>{ctx.tab === 'waiting' ? 'Inbox clear' : `Nothing ${askTabs.find((t) => t.tab === ctx.tab)?.label.toLowerCase()}`}</h2>
				<p class="muted">{ctx.tab === 'waiting' ? 'Nothing waits for an answer.' : 'Pick another tab above.'}</p>
			</div>
		{:else}
			<div class="list-head">
				{#if ctx.choosable}
					<label class="all"><input type="checkbox" checked={ctx.shown.every((x) => ctx.isChosen(x.id))} onchange={() => ctx.chooseAll(ctx.shown)} />All</label>
				{/if}
				<span class="muted">{ctx.shown.length} {ctx.shown.length === 1 ? 'ask' : 'asks'}</span>
			</div>
			<ul class="list" aria-label="Asks">
				{#each ctx.shown as x, i (x.id)}
					{@const who = ctx.who(x)}
					<li class="item" class:on={a?.id === x.id} class:chosen={ctx.isChosen(x.id)} class:sending={ctx.isPending(x.id)} data-testid="ask">
						{#if ctx.choosable}<input class="tick" type="checkbox" aria-label="Choose {x.name}" checked={ctx.isChosen(x.id)} onchange={() => ctx.toggleChosen(x.id)} />{/if}
						<button class="row-btn" type="button" aria-current={a?.id === x.id} onkeydown={listKeys} onclick={(e) => void pick(i, e.currentTarget)}>
							<span class="thumb"><Poster src={x.poster} name={x.name} /></span>
							<span class="txt">
								<span class="r1"><span class="name">{x.name}</span><time class="age" datetime={x.asked_at}>{ago(x.asked_at)}</time></span>
								<span class="r2"><Avatar name={who.name} src={who.avatar} size={16} /><span class="who">{who.name}</span>{#if who.kid}<KidMark label="Kid" size="sm" />{/if}</span>
								<span class="r3">{kindWord(x.kind)}{ctx.what(x) ? ` · ${ctx.what(x)}` : x.year ? ` · ${x.year}` : ''}</span>
							</span>
							<span class="chev" aria-hidden="true"><Icon name="back" size={16} /></span>
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	</div>

	<div class="detail-pane" role={phoneOpen ? 'dialog' : undefined} aria-modal={phoneOpen ? 'true' : undefined} aria-label={a ? a.name : undefined}>
		{#if a}
			<div class="phone-bar"><button class="back" type="button" onclick={() => void back()}><Icon name="back" size={20} />Asks</button></div>
			{#key a.id}<AskDetail {ctx} {a} form headingId="inbox-title" />{/key}
		{:else}
			<div class="empty-detail">
				<Icon name="check" size={32} />
				<p>{ctx.tab === 'waiting' ? 'All answered. Pick an ask on the left when one comes in.' : 'Nothing to show.'}</p>
			</div>
		{/if}
	</div>
</div>

<style>
	.inbox { display: grid; grid-template-columns: minmax(280px, 380px) minmax(0, 1fr); gap: 0; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); overflow: hidden; min-height: 560px; }
	.list-pane { border-right: 1px solid var(--line); min-width: 0; display: flex; flex-direction: column; }
	.list-head { display: flex; align-items: center; gap: 12px; min-height: 44px; padding: 6px 14px; border-bottom: 1px solid var(--line); font: var(--t-footnote); }
	.all { display: inline-flex; align-items: center; gap: 6px; font: var(--t-subhead); }
	.all input, .tick { width: 18px; height: 18px; accent-color: var(--accent-fill); flex: none; }
	.list { overflow-y: auto; max-height: calc(100dvh - var(--topbar) - 240px); min-height: 0; }
	.item { position: relative; display: flex; align-items: center; gap: 0; border-bottom: 1px solid var(--line); transition: background var(--dur-fast); }
	.item .tick { margin-left: 14px; }
	.item:hover { background: var(--surface-2); }
	.item.on { background: var(--accent-soft); }
	.item.on::before { content: ''; position: absolute; left: 0; top: 8px; bottom: 8px; width: 3px; border-radius: 0 3px 3px 0; background: var(--accent); }
	.item.chosen { background: color-mix(in srgb, var(--accent) 10%, transparent); }
	.item.sending { opacity: 0.6; }
	.row-btn { flex: 1; min-width: 0; display: flex; align-items: center; gap: 12px; padding: 10px 14px; text-align: left; }
	.thumb { width: 40px; flex: none; border-radius: 6px; overflow: hidden; }
	.txt { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
	.r1 { display: flex; align-items: baseline; gap: 8px; }
	.name { flex: 1; min-width: 0; font: var(--t-subhead); font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.age { flex: none; color: var(--text-muted); font: var(--t-caption); }
	.r2 { display: flex; align-items: center; gap: 6px; min-width: 0; font: var(--t-footnote); color: var(--text-muted); }
	.who { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.r3 { color: var(--text-faint); font: var(--t-caption); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.chev { display: none; transform: scaleX(-1); color: var(--text-faint); }
	.detail-pane { min-width: 0; padding: 18px; overflow-y: auto; max-height: calc(100dvh - var(--topbar) - 196px); }
	.phone-bar { display: none; }
	.calm { display: flex; flex-direction: column; align-items: center; gap: 6px; padding: 56px 20px; text-align: center; color: var(--text-muted); }
	.calm h2 { font: var(--t-headline); color: var(--text); }
	.empty-detail { height: 100%; min-height: 400px; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 10px; color: var(--text-muted); text-align: center; border-radius: var(--r-card); background: repeating-linear-gradient(135deg, transparent 0 14px, color-mix(in srgb, var(--line) 50%, transparent) 14px 15px); }
	@media (max-width: 1023px) {
		.inbox { grid-template-columns: minmax(240px, 320px) minmax(0, 1fr); }
	}
	@media (max-width: 767px) {
		.inbox { grid-template-columns: minmax(0, 1fr); min-height: 0; border-radius: var(--r-card); }
		.list-pane { border-right: 0; }
		.list { max-height: none; }
		.item.on { background: transparent; }
		.item.on::before { display: none; }
		.row-btn { padding: 12px 14px; min-height: 64px; }
		.thumb { width: 44px; }
		.chev { display: grid; }
		.detail-pane { display: none; }
		.phone-open .detail-pane { display: block; position: fixed; inset: 0; z-index: 60; max-height: none; padding: 0 16px calc(var(--tabbar) + 24px + env(safe-area-inset-bottom, 0px)); background: var(--bg); overscroll-behavior: contain; animation: slide-in 260ms var(--ease-out) both; }
		.phone-bar { display: flex; position: sticky; top: 0; z-index: 2; margin: 0 -16px 12px; padding: calc(6px + env(safe-area-inset-top, 0px)) 8px 6px; background: color-mix(in srgb, var(--bg) 82%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); border-bottom: 1px solid var(--line); }
		.back { min-height: 44px; display: inline-flex; align-items: center; gap: 4px; padding: 0 8px; border-radius: 10px; color: var(--accent-text); font: var(--t-headline); }
	}
	@keyframes slide-in { from { transform: translateX(24px); opacity: 0; } }
	@media (prefers-reduced-motion: reduce) { .phone-open .detail-pane { animation: none; } }
</style>
