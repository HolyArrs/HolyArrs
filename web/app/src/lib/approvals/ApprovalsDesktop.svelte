<script lang="ts">
	// Approvals on a desktop. One toolbar the Downloads way: the status tabs as a
	// pill group on the left; on the right a capsule with the type filter and,
	// next to it, the view switch (Cards or Stage). Cards are business cards:
	// the cover and everything that matters inside the card, with Approve and
	// Turn down. A card opens a window with that ask on the Stage.
	import { tick } from 'svelte';
	import { flip } from 'svelte/animate';
	import { fade, fly } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import Poster from '../Poster.svelte';
	import Icon, { type IconName } from '../Icon.svelte';
	import Avatar from '../ui/Avatar.svelte';
	import KidMark from './KidMark.svelte';
	import ApprovalsStage from './ApprovalsStage.svelte';
	import { ago, dateTime } from '../format.ts';
	import type { ApprovalsCtx } from './approvals.ts';
	import { artistPhoto, askArtist } from './details.svelte.ts';
	import { to } from '../paths.ts';
	import { askSummary, inboxTabs } from './inbox.ts';
	import { lifecycle } from './lifecycle.ts';
	import type { Ask } from '../types.ts';

	let { ctx }: { ctx: ApprovalsCtx } = $props();

	const waiting = (a: Ask) => ctx.approver && a.state === 'asked' && !ctx.isPending(a.id);
	const tabs = $derived(inboxTabs.filter((t) => ctx.approver || t.tab !== 'failed'));
	const list = $derived([...ctx.shown].sort((x, y) => Date.parse(y.asked_at) - Date.parse(x.asked_at)));
	$effect(() => {
		for (const a of list.slice(0, 16)) ctx.want(a);
	});

	const types: { k: 'all' | 'movie' | 'show' | 'artist'; label: string; icon: IconName }[] = [
		{ k: 'all', label: 'All', icon: 'library' },
		{ k: 'movie', label: 'Movies', icon: 'film' },
		{ k: 'show', label: 'TV shows', icon: 'tv' },
		{ k: 'artist', label: 'Music', icon: 'music' }
	];
	const views: { k: 'cards' | 'stage'; label: string; icon: IconName }[] = [
		{ k: 'cards', label: 'Cards', icon: 'grid' },
		{ k: 'stage', label: 'Stage', icon: 'sidebar' }
	];

	// The Stage needs a wide screen; narrower ones always show cards.
	let wide = $state(true);
	$effect(() => {
		const q = matchMedia('(min-width: 1024px)');
		const sync = () => (wide = q.matches);
		sync();
		q.addEventListener('change', sync);
		return () => q.removeEventListener('change', sync);
	});

	// The view, remembered per member.
	const viewKey = $derived(`holyarrs.approvals-view.${ctx.me ?? 'me'}`);
	let view = $state<'cards' | 'stage'>('cards');
	$effect(() => {
		try {
			view = localStorage.getItem(viewKey) === 'stage' ? 'stage' : 'cards';
		} catch {
			/* storage blocked: cards */
		}
	});
	const shownView = $derived(wide ? view : 'cards');
	function setView(v: 'cards' | 'stage') {
		view = v;
		try {
			localStorage.setItem(viewKey, v);
		} catch {
			/* storage blocked */
		}
	}

	// The raised plate slides to the picked item in each group (as on Downloads).
	function plate(node: HTMLElement, _key: unknown) {
		const place = () => {
			const on = node.querySelector<HTMLElement>('[aria-pressed="true"], [aria-selected="true"]');
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

	// The card's banner: the title's wide art (show or movie backdrop, the artist's wide photo).
	const banner = (a: Ask) => {
		if (a.kind === 'artist') return askArtist(a)?.backgrounds[0] ?? '';
		const d = ctx.details(a);
		return d?.backdrop ? to(d.backdrop) : '';
	};

	// What the ask is for, in words that fit a card.
	const kindOf = (a: Ask) => (a.kind === 'show' ? 'TV show' : a.kind === 'artist' ? 'Music' : 'Movie');
	const wants = (a: Ask) => {
		if (a.kind === 'movie') return 'The movie';
		const l = askSummary(a, ctx.info(a)).line;
		return l.split(' · ').at(-1) || l || (a.kind === 'artist' ? 'Every album' : 'Every season');
	};

	// Approving shows its answer on the card for a beat, then the card leaves (Undo stays in the toast).
	let approving = $state(new Set<number>());
	function approveOne(a: Ask) {
		if (approving.has(a.id)) return;
		approving = new Set(approving).add(a.id);
		setTimeout(() => {
			ctx.approve([a]);
			const next = new Set(approving);
			next.delete(a.id);
			approving = next;
		}, 560);
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
		if (win && !win.open) win.showModal();
	}
	function hide() {
		if (win?.open) win.close();
	}
	let wasWaiting = false;
	$effect(() => {
		const w = !!openAsk && waiting(openAsk);
		if (wasWaiting && !w && win?.open) setTimeout(hide, 650);
		wasWaiting = w;
	});
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
		<div class="tabs" role="tablist" aria-label="Status" use:plate={[ctx.tab, ctx.counts]}>
			<span class="plate" aria-hidden="true"></span>
			{#each tabs as t (t.tab)}
				<button type="button" role="tab" class="tab" class:zero={!ctx.counts[t.tab]} aria-selected={ctx.tab === t.tab} onclick={() => ctx.setTab(t.tab)}>{t.label}<span class="n" class:wait={t.tab === 'waiting' && ctx.counts[t.tab]}>{ctx.counts[t.tab] ?? 0}</span></button>
			{/each}
		</div>
		<div class="right">
			{#if shownView === 'cards' && ctx.tab === 'waiting' && list.filter(waiting).length > 1}
				<button type="button" class="approve-all" in:fade={{ duration: 200 }} onclick={() => ctx.approve(list.filter(waiting))}><Icon name="check" size={16} />Approve all {list.filter(waiting).length}</button>
			{/if}
			<div class="cap">
				<div class="seg" role="group" aria-label="Type" use:plate={ctx.kind}>
					<span class="plate" aria-hidden="true"></span>
					{#each types as t (t.k)}
						<button type="button" class="it" aria-pressed={ctx.kind === t.k} onclick={() => ctx.setKind(t.k)}><span class="ic"><Icon name={t.icon} size={18} /></span><span class="lb">{t.label}</span></button>
					{/each}
				</div>
				{#if wide}
				<span class="vr" aria-hidden="true"></span>
				<div class="seg" role="group" aria-label="View" use:plate={view}>
					<span class="plate" aria-hidden="true"></span>
					{#each views as v (v.k)}
						<button type="button" class="it" aria-pressed={view === v.k} title="{v.label} view" onclick={() => setView(v.k)}><span class="ic"><Icon name={v.icon} size={18} /></span><span class="lb">{v.label}</span></button>
					{/each}
				</div>
				{/if}
			</div>
		</div>
	</div>

	{#if shownView === 'stage'}
		<div in:fade={{ duration: 260 }}><ApprovalsStage {ctx} embedded /></div>
	{:else}
		<ul class="cards" aria-label="Asks" in:fade={{ duration: 260 }}>
			{#each list as a (a.id)}
				{@const w = ctx.who(a)}
				{@const life = lifecycle(a)}
				{@const photo = a.kind === 'artist' ? artistPhoto(a) : null}
				{@const wide = banner(a)}
				<li class="bc" class:answered={!waiting(a)} animate:flip={{ duration: 340, easing: cubicOut }} in:fly={{ y: 10, duration: 320, easing: cubicOut }} out:fade={{ duration: 220 }}>
					<!-- The whole card opens the window; the answers sit above this layer. -->
					<button type="button" class="open" aria-label="Open {a.name}, asked by {w.name}" aria-haspopup="dialog" onclick={(e) => void show(a, e.currentTarget)}></button>
					<div class="ban" aria-hidden="true">
						{#if wide}<span class="ban-img" style:background-image={`url("${wide}")`}></span>{:else}<span class="ban-img soft" style:background-image={photo || a.poster ? `url("${photo ?? a.poster}")` : undefined}></span>{/if}
						<span class="ban-kind">{kindOf(a)}</span>
					</div>
					<span class="cover" aria-hidden="true">{#if photo}<img src={photo} alt="" loading="lazy" decoding="async" />{:else}<Poster src={a.poster} name={a.name} />{/if}</span>
					<div class="body">
						<h3 class="title">{a.name}</h3>
						<span class="wants">{a.year ? `${a.year} · ` : ''}{wants(a)}</span>
						<span class="who"><Avatar name={w.name} src={w.avatar} size={20} /><span><b>{w.name}</b>{#if w.kid}<KidMark label="Kid" size="sm" />{/if} · <time datetime={a.asked_at} title={dateTime(a.asked_at)}>{ago(a.asked_at)}</time></span></span>
					</div>
					<div class="foot">
						{#if waiting(a)}
							<button type="button" class="b no" aria-label="Turn down {a.name}" data-testid="ask-decline" onclick={() => ctx.turnDown([a])}><Icon name="x" size={15} /><span class="bl">Turn down</span></button>
							<button type="button" class="b yes" data-testid="ask-approve" onclick={() => approveOne(a)}><Icon name="check" size={15} />Approve</button>
						{:else if ctx.isPending(a.id)}
							<span class="state">Sending your answer…</span>
						{:else}
							<span class="state tone-{life.tone}"><span class="dot"></span>{ctx.decided(a) || life.label}</span>
						{/if}
					</div>
					<span class="life tone-{approving.has(a.id) ? 'ok' : life.tone}" aria-hidden="true"><span class="fill" style:width="{approving.has(a.id) ? 100 : life.pct}%"></span></span>
					{#if approving.has(a.id)}
						<span class="done" role="status" in:fade={{ duration: 180 }}><span class="done-ic" in:fly={{ y: 6, duration: 320, easing: cubicOut }}><Icon name="check" size={22} /></span>Approved</span>
					{/if}
				</li>
			{:else}
				<li class="calm">
					<span class="calm-ic"><Icon name="check" size={24} /></span>
					<h2>{ctx.tab === 'waiting' ? 'You’re all caught up' : 'Nothing here right now'}</h2>
					<p>{ctx.tab === 'waiting' ? 'New asks land here the moment someone asks.' : 'Pick another tab, or another type.'}</p>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
<dialog class="win" bind:this={win} aria-label={openAsk?.name ?? 'Ask'} oncancel={cancel} onclose={() => { openId = 0; from?.focus({ preventScroll: true }); }} onclick={(e) => e.target === win && hide()}>
	{#if openAsk}
		<button type="button" class="close" aria-label="Close" onclick={hide}><Icon name="x" size={18} /></button>
		<div class="win-in"><ApprovalsStage {ctx} single={openAsk} /></div>
	{/if}
</dialog>

<style>
	.desktop { --ez: cubic-bezier(0.32, 0.72, 0, 1); display: flex; flex-direction: column; gap: 16px; }
	button { font: inherit; color: inherit; }
	button:focus-visible { outline: none; box-shadow: var(--focus-ring); }

	/* The toolbar: tabs left, filter and view right. */
	.toolbar { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 12px; }
	.right { display: flex; align-items: center; gap: 10px; }
	.plate { position: absolute; z-index: 0; left: 0; top: 0; width: var(--iw, 0); height: var(--ih, 0); background: var(--seg-thumb); box-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.12), 0 2px 8px rgb(0 0 0 / 0.08); opacity: var(--io, 0); transform: translate(var(--ix, 0), var(--iy, 0)); transition: transform 320ms var(--ez), width 320ms var(--ez), opacity 160ms var(--ease-out); pointer-events: none; }
	.tabs { position: relative; display: flex; align-items: center; gap: 2px; height: 52px; box-sizing: border-box; padding: 3px; border: 1px solid var(--line); border-radius: 14px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); backdrop-filter: blur(18px) saturate(1.4); -webkit-backdrop-filter: blur(18px) saturate(1.4); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); }
	.tabs .plate { border-radius: 10px; }
	.tab { position: relative; z-index: 1; display: inline-flex; align-items: center; gap: 7px; height: 44px; padding: 0 16px; border: 0; border-radius: 10px; background: none; color: color-mix(in srgb, var(--text) 76%, var(--bg)); font: var(--t-subhead); font-weight: 500; white-space: nowrap; cursor: pointer; transition: color 200ms var(--ease-out); }
	.tab:hover, .tab[aria-selected='true'] { color: var(--text); }
	.tab.zero:not([aria-selected='true']) { opacity: 0.7; }
	.n { opacity: 0.75; font-weight: 600; font-variant-numeric: tabular-nums; }
	.n.wait { color: var(--wait-text, var(--wait)); opacity: 1; }
	.cap { display: flex; align-items: center; height: 52px; box-sizing: border-box; padding: 3px; border: 1px solid var(--line); border-radius: 14px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); backdrop-filter: blur(18px) saturate(1.4); -webkit-backdrop-filter: blur(18px) saturate(1.4); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); }
	.seg { position: relative; display: flex; align-items: stretch; gap: 2px; height: 44px; }
	.seg .plate { border-radius: 10px; }
	.it { position: relative; z-index: 1; display: grid; grid-template-rows: 18px 12px; align-content: center; justify-items: center; gap: 3px; width: 68px; height: 44px; padding: 0; border: 0; border-radius: 10px; background: none; color: var(--text); cursor: pointer; transition: background 160ms var(--ease-out), transform 180ms var(--ez); }
	.ic { display: grid; place-items: center; height: 18px; }
	.lb { font: 500 11px/1 var(--font-ui); color: var(--text-muted); transition: color 160ms var(--ease-out); }
	.it:hover .lb, .it[aria-pressed='true'] .lb { color: var(--text); }
	.it:hover:not([aria-pressed='true']) { background: color-mix(in srgb, var(--text) 7%, transparent); }
	.it:active { transform: scale(0.97); }
	.vr { width: 1px; height: 26px; margin: 0 4px; background: var(--line); }
	.approve-all { display: inline-flex; align-items: center; gap: 6px; height: 52px; padding: 0 18px; border: 1px solid var(--line); border-radius: 14px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); color: var(--text); font: var(--t-subhead); font-weight: 600; cursor: pointer; backdrop-filter: blur(18px) saturate(1.4); -webkit-backdrop-filter: blur(18px) saturate(1.4); transition: transform 160ms var(--ease-out), background 200ms var(--ease-out); }
	.approve-all:hover { background: var(--surface-2); }
	.approve-all:active { transform: scale(0.96); }

	/* Business cards, portrait: the title's banner art on top, the poster overlapping it,
	   the few facts that matter, the answers at the foot. 12px inner padding, 20px corners. */
	.cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(250px, 100%), 1fr)); gap: 16px; margin: 0; padding: 0; list-style: none; }
	.bc { position: relative; isolation: isolate; overflow: hidden; display: flex; flex-direction: column; border: 1px solid var(--line); border-radius: 20px; background: var(--surface-1); transition: transform 300ms var(--ez), border-color 220ms var(--ease-out), box-shadow 300ms var(--ease-out); }
	.bc:hover { transform: translateY(-3px); border-color: var(--line-strong, var(--line)); box-shadow: 0 24px 48px -24px rgb(0 0 0 / 0.6); }
	.open { position: absolute; inset: 0; z-index: 1; padding: 0; border: 0; border-radius: inherit; background: none; cursor: pointer; }
	.open:focus-visible { box-shadow: inset 0 0 0 2px var(--accent); }
	.ban { position: relative; height: 128px; overflow: hidden; background: var(--surface-3); }
	.ban-img { position: absolute; inset: 0; background-size: cover; background-position: center 28%; transform: scale(1.02); transition: transform 900ms cubic-bezier(0.22, 1, 0.36, 1); }
	.ban-img.soft { filter: blur(18px) saturate(1.3); transform: scale(1.3); opacity: 0.8; }
	.bc:hover .ban-img:not(.soft) { transform: scale(1.08); }
	.ban::after { content: ''; position: absolute; inset: 0; background: linear-gradient(to bottom, rgb(0 0 0 / 0.25), transparent 40%, color-mix(in srgb, var(--surface-1) 85%, transparent) 100%); }
	.ban-kind { position: absolute; top: 10px; left: 10px; z-index: 1; padding: 3px 9px; border-radius: 999px; background: rgb(12 12 14 / 0.55); color: #fff; font: 600 11px/1.3 var(--font-ui); letter-spacing: 0.04em; -webkit-backdrop-filter: blur(10px); backdrop-filter: blur(10px); }
	.cover { position: relative; z-index: 0; display: block; width: 72px; aspect-ratio: 2 / 3; --poster-ratio: 2 / 3; margin: -54px 0 0 14px; border-radius: 8px; overflow: hidden; background: var(--surface-3); box-shadow: 0 0 0 3px var(--surface-1), 0 14px 28px -12px rgb(0 0 0 / 0.7); transition: transform 300ms var(--ez); }
	.cover img { width: 100%; height: 100%; object-fit: cover; object-position: center 25%; display: block; }
	.bc:hover .cover { transform: translateY(-2px) rotate(-1.5deg); }
	.body { display: flex; flex-direction: column; gap: 4px; padding: 12px 14px 0; min-width: 0; }
	.title { font: var(--t-title3); letter-spacing: -0.01em; line-height: 1.2; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
	.wants { color: var(--text-muted); font: var(--t-footnote); }
	.who { display: inline-flex; align-items: center; gap: 7px; margin-top: 6px; color: var(--text-muted); font: var(--t-footnote); }
	.who b { color: var(--text); font-weight: 600; }
	.foot { position: relative; z-index: 2; display: grid; grid-template-columns: 1fr 1fr; gap: 8px; margin-top: auto; padding: 14px 14px 16px; }
	.b { display: inline-flex; align-items: center; justify-content: center; gap: 6px; height: 40px; padding: 0 12px; border: 0; border-radius: 12px; font: var(--t-subhead); font-weight: 600; cursor: pointer; transition: transform 140ms var(--ease-out), background 200ms var(--ease-out); }
	.b:active { transform: scale(0.96); }
	.b.no { background: color-mix(in srgb, var(--text) 8%, transparent); color: var(--text); }
	.b.no:hover { background: color-mix(in srgb, var(--text) 13%, transparent); }
	.b.yes { background: var(--text); color: var(--bg); }
	.state { grid-column: 1 / -1; display: inline-flex; align-items: center; gap: 7px; min-height: 40px; color: var(--text-muted); font: var(--t-footnote); }
	.dot { width: 7px; height: 7px; border-radius: 50%; background: currentColor; }
	.state.tone-wait .dot { background: var(--wait); }
	.state.tone-go .dot { background: var(--accent); }
	.state.tone-ok .dot { background: var(--ok); }
	.state.tone-bad .dot { background: var(--fail, #e5484d); }
	.life { position: absolute; left: 0; right: 0; bottom: 0; z-index: 2; height: 3px; overflow: hidden; background: rgb(127 127 127 / 0.1); }
	.fill { display: block; height: 100%; border-radius: 0 3px 3px 0; transition: width 900ms cubic-bezier(0.22, 1, 0.36, 1); }
	.tone-wait .fill { background: var(--wait); }
	.tone-go .fill { background: var(--accent); }
	.tone-ok .fill { background: var(--ok); }
	.tone-off .fill { background: rgb(127 127 127 / 0.45); }
	.tone-bad .fill { background: var(--fail, #e5484d); }
	.answered .ban-img { filter: saturate(0.7); }
	.done { position: absolute; inset: 0; z-index: 2; display: flex; align-items: center; justify-content: center; gap: 10px; background: color-mix(in srgb, var(--surface-1) 78%, transparent); -webkit-backdrop-filter: blur(6px); backdrop-filter: blur(6px); color: var(--text); font: var(--t-headline); }
	.done-ic { display: grid; place-items: center; width: 40px; height: 40px; border-radius: 50%; background: var(--ok); color: #fff; box-shadow: 0 8px 20px -8px color-mix(in srgb, var(--ok) 80%, transparent); }
	.calm { grid-column: 1 / -1; display: grid; justify-items: center; gap: 6px; padding: 64px 20px; border-radius: 18px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); color: var(--text-muted); text-align: center; }
	.calm-ic { display: grid; place-items: center; width: 52px; height: 52px; margin-bottom: 4px; border-radius: 50%; background: var(--surface-2); color: var(--text); }
	.calm h2 { color: var(--text); font: var(--t-title3); }

	/* The window: one ask on the Stage. */
	.win { width: min(1180px, calc(100vw - 48px)); max-width: none; height: min(760px, calc(100dvh - 48px)); max-height: none; margin: auto; padding: 0; border: 0; border-radius: 24px; background: #0b0b0c; overflow: hidden; box-shadow: 0 0 0 1px rgb(255 255 255 / 0.08), 0 50px 120px -30px rgb(0 0 0 / 0.85); }
	.win[open] { animation: win-in 420ms cubic-bezier(0.22, 1, 0.36, 1) both; }
	.win::backdrop { background: rgb(0 0 0 / 0.6); -webkit-backdrop-filter: blur(8px); backdrop-filter: blur(8px); animation: fade-in 300ms var(--ease-out) both; }
	.win-in { height: 100%; }
	.close { position: absolute; top: 18px; right: 18px; z-index: 10; display: grid; place-items: center; width: 38px; height: 38px; border: 0; border-radius: 50%; background: rgb(20 20 24 / 0.55); color: #fff; box-shadow: inset 0 0 0 1px rgb(255 255 255 / 0.18); -webkit-backdrop-filter: blur(14px); backdrop-filter: blur(14px); cursor: pointer; transition: background 200ms var(--ease-out); }
	.close:hover { background: rgb(20 20 24 / 0.8); }
	.win :global(.ctl.top) { right: 68px !important; }
	@keyframes win-in { from { opacity: 0; transform: translateY(16px) scale(0.98); } }
	@keyframes fade-in { from { opacity: 0; } }
	/* Phones: the tabs swipe, the filter fills the width, one card per row, the window fills the screen. */
	@media (max-width: 767px) {
		.desktop { gap: 12px; padding-bottom: calc(var(--tabbar, 64px) + 16px + env(safe-area-inset-bottom, 0px)); }
		.toolbar { flex-direction: column; align-items: stretch; gap: 10px; }
		.tabs { width: 100%; overflow-x: auto; scrollbar-width: none; overscroll-behavior-x: contain; }
		.tabs::-webkit-scrollbar { display: none; }
		.tab { flex: none; padding: 0 14px; font: var(--t-footnote); font-weight: 600; }
		.right { flex-direction: column-reverse; align-items: stretch; gap: 10px; }
		.cap { width: 100%; }
		.cap .seg { flex: 1; }
		.it { flex: 1; width: auto; }
		.approve-all { justify-content: center; height: 48px; }
		.cards { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
		.ban { height: 84px; }
		.cover { width: 56px; margin: -40px 0 0 10px; }
		.body { padding: 10px 10px 0; }
		.title { font: var(--t-headline); }
		.who { font: var(--t-caption); }
		.foot { padding: 12px 10px 14px; gap: 6px; }
		.foot { grid-template-columns: 40px minmax(0, 1fr); }
		.b { height: 40px; padding: 0 8px; font: var(--t-footnote); font-weight: 600; }
		.b.no .bl { display: none; }
		.win { width: 100vw; height: 100dvh; border-radius: 0; }
		.close { top: calc(12px + env(safe-area-inset-top, 0px)); right: 12px; width: 44px; height: 44px; }
		.win :global(.ctl.top) { right: 64px !important; }
	}
	@media (prefers-reduced-motion: reduce) {
		.plate, .bc, .cover, .fill, .it, .win[open], .win::backdrop { transition: none; animation: none; }
	}
</style>
