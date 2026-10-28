<script lang="ts">
	// The Approvals page in five Stage views (?stage=1…5): the picked ask
	// large, the list roomier than the old slim strip. All share the same
	// controls: status tabs with counts, one Filter menu, Poster size and a
	// list that hides (remembered per member, like the Library), Review one
	// by one, one Choose all for bulk decisions, and a 6 s Undo on every
	// decision. In every list the name picks the ask and the picture opens
	// the title's page.
	//   1 Wide list     a 360 px column of two-line rows beside the Stage
	//   2 By person     the same column, the rows under each person who asked
	//   3 Poster strip  the asks as a poster strip above the Stage
	//   4 Cards         a two-column card list beside the Stage
	//   5 By type       the column split into Movies, TV shows and Music
	// Below 1024 px every view is a list, and an ask opens full screen.
	import { onMount, tick } from 'svelte';
	import Poster from '../Poster.svelte';
	import Icon from '../Icon.svelte';
	import Avatar from '../ui/Avatar.svelte';
	import Button from '../ui/Button.svelte';
	import IconButton from '../ui/IconButton.svelte';
	import Chip from '../ui/Chip.svelte';
	import Menu, { type MenuItem } from '../ui/Menu.svelte';
	import StatusPill from '../ui/StatusPill.svelte';
	import KidMark from './KidMark.svelte';
	import AskPanel from './AskPanel.svelte';
	import FocusView from './FocusView.svelte';
	import { to } from '../paths.ts';
	import { ago, dateTime } from '../format.ts';
	import { kindWord, personGroups, type ApprovalsCtx } from './approvals.ts';
	import { askSummary, inboxTabs, loadPanel, loadSize, savePanel, saveSize, showNotOut, sizeLabels, watchSeason, type InboxSize } from './inbox.ts';
	import { artistPhoto } from './details.svelte.ts';
	import type { Ask } from '../types.ts';

	let { ctx, look }: { ctx: ApprovalsCtx; look: number } = $props();

	// Poster size and whether the list shows, remembered per member.
	let size = $state<InboxSize>('m');
	let listOpen = $state(true);
	let loadedFor = $state<number | undefined | null>(null);
	$effect(() => {
		const me = ctx.me;
		if (loadedFor === me) return;
		loadedFor = me;
		size = loadSize(me);
		listOpen = loadPanel(me);
	});
	function setSize(z: InboxSize) {
		size = z;
		saveSize(ctx.me, z);
	}
	function setList(open: boolean) {
		listOpen = open;
		savePanel(ctx.me, open);
	}
	const listId = 'approvals-list';

	// The Stage needs room: from 1024 px; narrower screens open an ask full screen.
	let wide = $state(true);
	onMount(() => {
		const q = window.matchMedia('(min-width: 1024px)');
		const sync = () => (wide = q.matches);
		sync();
		q.addEventListener('change', sync);
		return () => q.removeEventListener('change', sync);
	});

	// The picked ask stays picked while the list changes; once it leaves, the next one comes up.
	let pickedId = $state(0);
	let lastAt = $state(0);
	const at = $derived.by(() => {
		const i = ctx.shown.findIndex((a) => a.id === pickedId);
		return i >= 0 ? i : Math.min(lastAt, Math.max(ctx.shown.length - 1, 0));
	});
	const picked = $derived(ctx.shown[at] ?? null);
	$effect(() => {
		for (const x of ctx.shown) ctx.want(x);
	});

	// Full screen (phones and tablets, and Approve with options in one-by-one review).
	let fullOpen = $state(false);
	let fullId = $state(0);
	const fullAsk = $derived(ctx.viewed.find((a) => a.id === fullId) ?? null);
	let opener: HTMLElement | null = null;
	async function openFull(a: Ask, el?: HTMLElement | null) {
		fullId = a.id;
		opener = el ?? null;
		fullOpen = true;
		await tick();
		document.getElementById('stage-full-title')?.focus({ preventScroll: true });
	}
	async function closeFull() {
		fullOpen = false;
		await tick();
		if (opener?.isConnected) opener.focus();
	}
	$effect(() => {
		if (fullOpen && fullAsk && fullAsk.state !== 'asked' && ctx.tab === 'waiting') void closeFull();
	});

	function pick(a: Ask, el?: HTMLElement | null) {
		const i = ctx.shown.findIndex((x) => x.id === a.id);
		pickedId = a.id;
		lastAt = Math.max(i, 0);
		if (!wide) void openFull(a, el);
	}
	function step(by: number) {
		const n = ctx.shown.length;
		if (!n) return;
		const next = ctx.shown[(((at + by) % n) + n) % n];
		pickedId = next.id;
		lastAt = (((at + by) % n) + n) % n;
		void tick().then(() => document.querySelector<HTMLElement>(`[data-row="${next.id}"]`)?.scrollIntoView({ block: 'nearest', inline: 'nearest' }));
	}
	function listKeys(e: KeyboardEvent) {
		const keys = look === 3 ? ['ArrowRight', 'ArrowLeft'] : ['ArrowDown', 'ArrowUp'];
		if (!keys.includes(e.key)) return;
		const rows = [...(e.currentTarget as HTMLElement).querySelectorAll<HTMLElement>('[data-row]')];
		const i = rows.indexOf(document.activeElement as HTMLElement);
		if (i < 0) return;
		e.preventDefault();
		const n = Math.min(Math.max(i + (e.key === keys[0] ? 1 : -1), 0), rows.length - 1);
		rows[n]?.focus();
		const a = ctx.shown.find((x) => x.id === Number(rows[n]?.dataset.row));
		if (a && wide) pick(a);
	}

	// One by one: the Focus review over the same list.
	let oneByOne = $state(false);
	$effect(() => {
		if (oneByOne && (ctx.tab !== 'waiting' || !ctx.shown.length)) oneByOne = false;
	});
	const focusCtx: ApprovalsCtx = $derived(Object.create(ctx, {
		choosable: { get: () => false },
		withOptions: { value: (a: Ask) => void openFull(a, document.activeElement as HTMLElement) },
		review: { value: (a: Ask, el?: HTMLElement) => void openFull(a, el) }
	}));

	// What a row says: what the ask is for (shows and music), else the year.
	const line = (a: Ask) => askSummary(a, ctx.info(a)).line || (a.year ? String(a.year) : '');
	const notOut = (a: Ask) => {
		if (a.kind !== 'show') return false;
		const s = watchSeason(a, ctx.info(a));
		return !!s && !!showNotOut(s, ctx.episodes(a.tmdb_id, s));
	};
	const waiting = (a: Ask) => ctx.approver && a.state === 'asked' && !ctx.isPending(a.id);

	const chosen = $derived(ctx.shown.filter((a) => ctx.isChosen(a.id)));
	const allChosen = $derived(ctx.shown.length > 0 && chosen.length === ctx.shown.length);

	// The filter menu: Show, Type and Asked by in one small menu.
	const typeWords: Record<string, string> = { all: 'Every type', movie: 'Movies', show: 'TV shows', artist: 'Music' };
	const filterOn = $derived((ctx.mineOnly ? 1 : 0) + (ctx.kind !== 'all' ? 1 : 0) + (ctx.person ? 1 : 0));
	const filterItems = $derived<MenuItem[]>([
		{ kind: 'heading', label: 'Show' },
		{ label: "Everyone's asks", checked: !ctx.mineOnly, onselect: () => ctx.setMine(false) },
		{ label: 'Mine', checked: ctx.mineOnly, onselect: () => ctx.setMine(true) },
		{ kind: 'separator' },
		{ kind: 'heading', label: 'Type' },
		...(['all', 'movie', 'show', 'artist'] as const).map((k) => ({ label: typeWords[k], checked: ctx.kind === k, onselect: () => ctx.setKind(k) })),
		...(ctx.people.length > 1
			? ([
					{ kind: 'separator' },
					{ kind: 'heading', label: 'Asked by' },
					{ label: 'Anyone', checked: !ctx.person, onselect: () => ctx.setPerson(0) },
					...ctx.people.map((p) => ({ label: `${p.name} (${p.n})`, checked: ctx.person === p.id, onselect: () => ctx.setPerson(p.id) }))
				] as MenuItem[])
			: [])
	]);
	const sizeItems = $derived<MenuItem[]>([{ kind: 'heading', label: 'Poster size' }, ...(['s', 'm', 'l'] as const).map((z) => ({ label: `${sizeLabels[z]} posters`, checked: size === z, onselect: () => setSize(z) }))]);
	const tabs = $derived(inboxTabs.filter((t) => ctx.approver || t.tab !== 'failed'));
	const people = $derived(personGroups(ctx.shown));
	const types = $derived(
		(['movie', 'show', 'artist'] as const)
			.map((k) => ({ id: k, name: typeWords[k], asks: ctx.shown.filter((a) => (k === 'movie' ? a.kind !== 'show' && a.kind !== 'artist' : a.kind === k)) }))
			.filter((g) => g.asks.length)
	);
	const split = $derived(wide && !oneByOne);
	const emptyWord = $derived(ctx.tab === 'waiting' ? 'Inbox clear' : `Nothing ${inboxTabs.find((t) => t.tab === ctx.tab)?.label.toLowerCase()}`);
</script>

<svelte:window onkeydown={(e) => fullOpen && e.key === 'Escape' && !document.querySelector('dialog[open]') && void closeFull()} />

{#snippet chooseBox(x: Ask)}
	{#if ctx.choosable}<input class="tick" type="checkbox" aria-label="Choose {x.name}" checked={ctx.isChosen(x.id)} onchange={() => ctx.toggleChosen(x.id)} />{/if}
{/snippet}
{#snippet thumb(x: Ask, cls = 'thumb')}
	{@const href = ctx.href(x)}
	{@const music = x.kind === 'artist'}
	{@const src = music ? artistPhoto(x) : x.poster}
	{#if href}
		<a class="{cls} pic" class:round={music} href={to(href)} aria-label="Open {x.name}" data-testid="ask-picture" tabindex="-1">{#if music && src}<img src={src} alt="" loading="lazy" decoding="async" />{:else}<Poster src={music ? null : src} name={x.name} />{/if}</a>
	{:else}
		<span class={cls} class:round={music}>{#if music && src}<img src={src} alt="" loading="lazy" decoding="async" />{:else}<Poster src={music ? null : src} name={x.name} />{/if}</span>
	{/if}
{/snippet}
{#snippet quick(x: Ask)}
	{#if waiting(x)}
		<span class="quick">
			<IconButton icon="check" variant="tinted" size="sm" label="Approve {x.name}" data-testid="ask-approve" onclick={() => ctx.approve([x])} />
			<IconButton icon="x" variant="tinted" size="sm" label="Turn down {x.name}" data-testid="ask-decline" onclick={() => ctx.turnDown([x])} />
		</span>
	{:else if ctx.isPending(x.id)}
		<span class="sending-word">Sending…</span>
	{:else if x.state !== 'asked'}
		<StatusPill kind={ctx.pill(x).kind} label={ctx.pill(x).label} />
	{/if}
{/snippet}
{#snippet whoLine(x: Ask)}
	{@const w = ctx.who(x)}
	<span class="who"><Avatar name={w.name} src={w.avatar} size={16} /><span class="wn">{w.name}</span>{#if w.kid}<KidMark label="Kid" size="sm" />{/if}</span>
{/snippet}
{#snippet nameBtn(x: Ask)}
	<button class="name" type="button" data-row={x.id} data-testid="ask-name" aria-current={split && picked?.id === x.id ? 'true' : undefined} aria-controls={split ? 'stage-title' : undefined} onclick={(e) => pick(x, e.currentTarget)}>{x.name}</button>
{/snippet}
{#snippet row(x: Ask, person = true, kindTag = true)}
	<li class="row" class:on={split && picked?.id === x.id} class:chosen={ctx.isChosen(x.id)} class:sending={ctx.isPending(x.id)} data-testid="ask">
		{@render chooseBox(x)}
		{@render thumb(x)}
		<div class="txt">
			<div class="r1">{#if kindTag}<span class="tag">{kindWord(x.kind)}</span>{/if}{@render nameBtn(x)}</div>
			<div class="r2"><span class="sline">{line(x)}</span>{#if notOut(x)}<span class="soon">Not out yet</span>{/if}</div>
			<div class="r3">{#if person}{@render whoLine(x)}<span class="dot" aria-hidden="true">·</span>{/if}<time datetime={x.asked_at} title={dateTime(x.asked_at)}>{ago(x.asked_at)}</time></div>
		</div>
		{@render quick(x)}
	</li>
{/snippet}
{#snippet card(x: Ask)}
	<li class="card" class:on={split && picked?.id === x.id} class:chosen={ctx.isChosen(x.id)} class:sending={ctx.isPending(x.id)} data-testid="ask">
		<div class="card-art">{@render thumb(x, 'cthumb')}{#if ctx.choosable}<span class="card-tick">{@render chooseBox(x)}</span>{/if}</div>
		<div class="card-body">
			<span class="tag">{kindWord(x.kind)}</span>
			{@render nameBtn(x)}
			<span class="sline">{line(x)}</span>
			<span class="r3">{@render whoLine(x)}<span class="dot" aria-hidden="true">·</span><time datetime={x.asked_at} title={dateTime(x.asked_at)}>{ago(x.asked_at)}</time></span>
			<span class="card-q">{@render quick(x)}</span>
		</div>
	</li>
{/snippet}

<div class="stage look-{look} size-{size}" class:collapsed={split && !listOpen} data-testid="approvals-stage" data-look={look} data-size={size} data-list={split ? (listOpen ? 'open' : 'hidden') : undefined}>
	<div class="bar">
		<div class="chips" role="group" aria-label="Show" data-testid="ask-tabs">
			{#each tabs as t (t.tab)}
				<span class="cw" class:zero={ctx.counts[t.tab] === 0}><Chip label={t.label} count={ctx.counts[t.tab]} selected={ctx.tab === t.tab} onclick={() => ctx.setTab(t.tab)} /></span>
			{/each}
		</div>
		<div class="tools">
			<Menu label="Filter asks" variant="button" text={filterOn ? `Filter · ${filterOn}` : 'Filter'} icon="chevronDown" items={filterItems} width={240} align="end" />
			{#if !oneByOne}<Menu label="Poster size" variant="button" text={sizeLabels[size]} icon="chevronDown" items={sizeItems} width={200} align="end" />{/if}
			{#if split}
				<Button variant="secondary" size="md" icon="sidebar" aria-expanded={listOpen} aria-controls={listId} data-testid="list-toggle" onclick={() => setList(!listOpen)}>{listOpen ? 'Hide list' : 'Show list'}</Button>
			{/if}
			{#if ctx.approver && ctx.tab === 'waiting' && ctx.shown.length}
				<Button variant={oneByOne ? 'primary' : 'secondary'} size="md" icon={oneByOne ? 'back' : 'eye'} data-testid="one-by-one" onclick={() => (oneByOne = !oneByOne)}>{oneByOne ? 'Back to the list' : 'Review one by one'}</Button>
			{/if}
		</div>
	</div>

	{#if oneByOne}
		<FocusView ctx={focusCtx} />
	{:else if ctx.shown.length === 0}
		<div class="calm" data-testid="calm">
			<Icon name="inbox" size={28} />
			<h2>{emptyWord}</h2>
			<p class="muted">{ctx.tab === 'waiting' ? 'Nothing waits for an answer.' : 'Pick another tab above.'}</p>
		</div>
	{:else}
		{#if ctx.choosable}
			<div class="lhead" class:on={chosen.length > 0} data-testid="ask-bulk">
				<label class="all"><input class="tick" type="checkbox" checked={allChosen} indeterminate={chosen.length > 0 && !allChosen} onchange={() => ctx.chooseAll(ctx.shown)} />{chosen.length ? `${chosen.length} chosen` : 'Choose all'}</label>
				{#if chosen.length}
					<Button size="sm" variant="primary" icon="check" type="button" onclick={() => ctx.approve(chosen)}>Approve {chosen.length}</Button>
					<Button size="sm" icon="x" type="button" onclick={() => ctx.turnDown(chosen)}>Turn down {chosen.length}</Button>
				{/if}
				{#if split && ctx.shown.length > 1}
					<span class="stepper">
						<IconButton icon="chevronLeft" size="sm" label="The ask before" onclick={() => step(-1)} />
						<span class="count">{at + 1} of {ctx.shown.length}</span>
						<IconButton icon="chevronRight" size="sm" label="The next ask" onclick={() => step(1)} />
					</span>
				{:else}
					<span class="count">{ctx.shown.length} {ctx.shown.length === 1 ? 'ask' : 'asks'}</span>
				{/if}
			</div>
		{/if}

		<div class="split" class:strip-top={look === 3 && split}>
			<!-- svelte-ignore a11y_no_static_element_interactions -->
			<div class="list" id={listId} onkeydown={listKeys} hidden={split && !listOpen ? true : undefined} inert={split && !listOpen}>
				{#if look === 3 && split}
					<ul class="strip" aria-label="Asks">
						{#each ctx.shown as x (x.id)}
							<li class="sitem" class:on={picked?.id === x.id} class:chosen={ctx.isChosen(x.id)} class:sending={ctx.isPending(x.id)} data-testid="ask">
								<div class="s-art">{@render thumb(x, 'sthumb')}{#if ctx.choosable}<span class="card-tick">{@render chooseBox(x)}</span>{/if}</div>
								{@render nameBtn(x)}
								<span class="s-who">{ctx.who(x).name} · {ago(x.asked_at)}</span>
							</li>
						{/each}
					</ul>
				{:else if look === 4}
					<ul class="cards" aria-label="Asks">
						{#each ctx.shown as x (x.id)}{@render card(x)}{/each}
					</ul>
				{:else if look === 2 || look === 5}
					<div class="groups">
						{#each look === 2 ? people : types as g (g.id)}
							{@const w = look === 2 ? ctx.who(g.asks[0]) : null}
							{@const open = g.asks.filter(waiting)}
							<section class="group" aria-labelledby="g-{g.id}">
								<header class="gh">
									{#if w}<Avatar name={w.name} src={w.avatar} size={24} />{/if}
									<h2 id="g-{g.id}">{w ? w.name : g.name}</h2>
									{#if w}<KidMark label={w.kid} size="sm" />{/if}
									<span class="gn">{g.asks.length}</span>
									{#if ctx.choosable && open.length > 1}
										<Button size="sm" variant="plain" icon="check" type="button" onclick={() => ctx.approve(open)}>Approve all {open.length}</Button>
									{/if}
								</header>
								<ul class="rows" aria-label="{w ? `${w.name}'s` : g.name} asks">
									{#each g.asks as x (x.id)}{@render row(x, look !== 2, look !== 5)}{/each}
								</ul>
							</section>
						{/each}
					</div>
				{:else}
					<ul class="rows" aria-label="Asks">
						{#each ctx.shown as x (x.id)}{@render row(x)}{/each}
					</ul>
				{/if}
			</div>

			{#if split && picked}
				<section class="main" aria-label="The picked ask" data-testid="ask-stage">
					{#key picked.id}<div class="stage-in"><AskPanel {ctx} a={picked} variant="stage" headingId="stage-title" /></div>{/key}
				</section>
			{/if}
		</div>
	{/if}
</div>

{#if fullOpen && fullAsk}
	<div class="full" role="dialog" aria-modal="true" aria-label={fullAsk.name} data-testid="ask-full">
		<div class="full-bar"><button class="back" type="button" onclick={() => void closeFull()}><Icon name="back" size={20} />Asks</button></div>
		{#key fullAsk.id}<AskPanel {ctx} a={fullAsk} headingId="stage-full-title" />{/key}
	</div>
{/if}

<style>
	/* One size per element: --thumb for list pictures, --lw for the list column. */
	.stage { --thumb: 48px; --lw: 360px; min-width: 0; }
	.size-s { --thumb: 36px; --lw: 320px; }
	.size-l { --thumb: 64px; --lw: 400px; }
	.look-3.size-s { --thumb: 72px; }
	.look-3.size-m { --thumb: 96px; }
	.look-3.size-l { --thumb: 120px; }
	.look-4 { --lw: 520px; }
	.look-4.size-s { --thumb: 56px; --lw: 480px; }
	.look-4.size-l { --thumb: 88px; --lw: 580px; }
	.look-4.size-m { --thumb: 72px; }

	.bar { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 16px; margin-bottom: 12px; }
	.chips { display: flex; flex-wrap: wrap; gap: 8px; }
	.cw { display: inline-flex; transition: opacity var(--dur-fast) var(--ease-out); }
	.cw.zero { opacity: 0.5; }
	.cw.zero:hover, .cw.zero:focus-within { opacity: 0.8; }
	.tools { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-left: auto; }

	.lhead { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; min-height: 44px; margin-bottom: 12px; padding: 4px 8px 4px 14px; border-radius: var(--r-pill); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); transition: background var(--dur-fast) var(--ease-out); }
	.lhead.on { background: var(--surface-2); }
	.all { display: inline-flex; align-items: center; gap: 8px; margin-right: auto; font: var(--t-subhead); color: var(--text-muted); }
	.stepper { display: inline-flex; align-items: center; gap: 2px; }
	.count { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.tick { width: 18px; height: 18px; flex: none; accent-color: var(--accent-fill); }

	/* The list and the Stage: the list column hides like the Library's side panel. */
	.split { display: grid; grid-template-columns: var(--lw) minmax(0, 1fr); gap: 16px; align-items: start; transition: grid-template-columns var(--dur) var(--ease-out), gap var(--dur) var(--ease-out); }
	.collapsed .split { grid-template-columns: 0px minmax(0, 1fr); gap: 0; }
	.split.strip-top { grid-template-columns: minmax(0, 1fr); gap: 12px; }
	.list { min-width: 0; }
	.split:not(.strip-top) > .list { position: sticky; top: calc(var(--topbar) + 12px); max-height: calc(100dvh - var(--topbar) - 32px); overflow-y: auto; overscroll-behavior: contain; scrollbar-gutter: stable; border-radius: var(--r-panel); transition: opacity var(--dur) var(--ease-out), translate var(--dur) var(--ease-out); }
	:global(html.bar-hidden) .split:not(.strip-top) > .list { top: 12px; max-height: calc(100dvh - 32px); }
	.list[hidden] { display: block; visibility: hidden; opacity: 0; translate: -24px 0; overflow: hidden; }
	.strip-top > .list[hidden] { display: none; }
	.main { min-width: 0; }
	.stage-in { animation: stage-in var(--dur) var(--ease-out) both; }
	@keyframes stage-in { from { opacity: 0; translate: 0 6px; } }

	/* Rows (1, 2, 5) */
	.rows { margin: 0; padding: 0; list-style: none; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: inset 0 0 0 1px var(--line); overflow: hidden; }
	.row { position: relative; display: flex; align-items: center; gap: 12px; padding: 10px 12px; transition: background var(--dur-fast) var(--ease-out), opacity var(--dur-fast) var(--ease-out); }
	.row + .row { border-top: 1px solid var(--line); }
	.row:hover { background: var(--surface-2); }
	.row.on { background: var(--accent-soft); }
	.row.on::before { content: ''; position: absolute; left: 0; top: 10px; bottom: 10px; width: 3px; border-radius: 0 3px 3px 0; background: var(--accent); }
	.row.chosen, .card.chosen, .sitem.chosen { background: color-mix(in srgb, var(--accent) 10%, transparent); }
	.sending { opacity: 0.6; }
	.thumb, .cthumb, .sthumb { display: block; flex: none; width: var(--thumb); border-radius: 6px; overflow: hidden; transition: width var(--dur) var(--ease-out), transform var(--dur-fast) var(--ease-out); }
	.round { aspect-ratio: 1; border-radius: 50%; background: var(--surface-3); --poster-ratio: 1; }
	.round img { width: 100%; height: 100%; object-fit: cover; display: block; }
	a.pic:hover { transform: scale(1.04); }
	a.pic:focus-visible, .name:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.txt { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
	.r1 { display: flex; align-items: center; gap: 8px; min-width: 0; }
	.r2, .r3 { display: flex; align-items: center; gap: 6px; min-width: 0; color: var(--text-muted); font: var(--t-footnote); }
	.r3 { font: var(--t-caption); }
	.tag { flex: none; align-self: flex-start; padding: 1px 7px; border-radius: var(--r-sm); background: var(--surface-2); color: var(--text-muted); font: var(--t-caption); font-weight: 600; white-space: nowrap; }
	.r1 .tag { align-self: center; }
	.row.on .tag { background: var(--surface-1); }
	.name { min-width: 0; max-width: 100%; padding: 0; border: 0; background: none; color: inherit; font: var(--t-subhead); font-weight: 600; text-align: left; cursor: pointer; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; border-radius: 4px; }
	.name:hover { text-decoration: underline; text-underline-offset: 3px; }
	.sline { min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.soon { flex: none; padding: 0 6px; border-radius: var(--r-sm); background: color-mix(in srgb, var(--wait) 14%, transparent); color: var(--wait-text); font: var(--t-caption); font-weight: 600; white-space: nowrap; }
	.dot { flex: none; }
	.who { display: inline-flex; align-items: center; gap: 5px; min-width: 0; }
	.wn { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.quick { display: inline-flex; gap: 6px; flex: none; }
	.sending-word { flex: none; color: var(--text-muted); font: var(--t-footnote); }
	.size-s .r3 { display: none; }

	/* Groups (2 By person, 5 By type) */
	.groups { display: flex; flex-direction: column; gap: 14px; }
	.gh { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; min-height: 32px; margin: 0 4px 6px; }
	.gh h2 { font: var(--t-headline); }
	.gn { margin-right: auto; padding: 0 7px; border-radius: var(--r-pill); background: var(--surface-2); color: var(--text-muted); font: var(--t-caption); font-weight: 600; font-variant-numeric: tabular-nums; }

	/* Poster strip (3) */
	.strip { margin: 0; padding: 4px 4px 10px; list-style: none; display: flex; gap: 12px; overflow-x: auto; overscroll-behavior-x: contain; scroll-snap-type: x proximity; scrollbar-width: thin; }
	.sitem { flex: none; width: calc(var(--thumb) + 16px); display: flex; flex-direction: column; gap: 4px; padding: 8px; border-radius: var(--r-card); scroll-snap-align: start; transition: background var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out); }
	.sitem:hover { background: var(--surface-2); }
	.sitem.on { background: var(--accent-soft); box-shadow: inset 0 0 0 2px var(--accent); }
	.s-art, .card-art { position: relative; }
	.sthumb { width: 100%; }
	.sitem .round { width: 100%; }
	.card-tick { position: absolute; top: 6px; left: 6px; display: inline-flex; padding: 3px; border-radius: 6px; background: color-mix(in srgb, var(--surface-1) 86%, transparent); }
	.sitem .name { font: var(--t-footnote); font-weight: 600; }
	.s-who { color: var(--text-muted); font: var(--t-caption); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

	/* Cards (4) */
	.cards { margin: 0; padding: 0; list-style: none; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
	.card { display: flex; gap: 12px; min-width: 0; padding: 10px; border-radius: var(--r-card); background: var(--surface-1); box-shadow: inset 0 0 0 1px var(--line); transition: background var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out); }
	.card:hover { background: var(--surface-2); }
	.card.on { box-shadow: inset 0 0 0 2px var(--accent); }
	.card-body { flex: 1; min-width: 0; display: flex; flex-direction: column; align-items: flex-start; gap: 3px; }
	.card-body .sline { max-width: 100%; color: var(--text-muted); font: var(--t-footnote); }
	.card-body .r3 { max-width: 100%; }
	.card-q { margin-top: auto; padding-top: 4px; }

	.calm { display: flex; flex-direction: column; align-items: center; gap: 6px; padding: 56px 20px; text-align: center; color: var(--text-muted); border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.calm h2 { font: var(--t-headline); color: var(--text); }
	.muted { color: var(--text-muted); }

	/* Full screen ask (below 1024 px, and options in one-by-one review). */
	.full { position: fixed; inset: 0; z-index: 60; overflow-y: auto; overscroll-behavior: contain; padding: 0 16px calc(var(--tabbar, 0px) + 24px + env(safe-area-inset-bottom, 0px)); background: var(--bg); animation: slide-in var(--dur) var(--ease-out) both; }
	.full-bar { position: sticky; top: 0; z-index: 2; display: flex; margin: 0 -16px 12px; padding: calc(6px + env(safe-area-inset-top, 0px)) 8px 6px; background: color-mix(in srgb, var(--bg) 82%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); border-bottom: 1px solid var(--line); }
	.back { min-height: 44px; display: inline-flex; align-items: center; gap: 4px; padding: 0 8px; border: 0; background: none; border-radius: 10px; color: var(--accent-text); font: var(--t-headline); cursor: pointer; }
	.back:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (min-width: 768px) { .full :global(.panel) { max-width: 720px; margin: 0 auto; } }
	@keyframes slide-in { from { transform: translateX(24px); opacity: 0; } }

	@media (max-width: 1023px) {
		.split { grid-template-columns: minmax(0, 1fr); }
		.cards { grid-template-columns: repeat(auto-fill, minmax(min(100%, 300px), 1fr)); }
	}
	@media (max-width: 767px) {
		/* The five tabs share one row, the word above its count (as on Downloads). */
		.chips { width: 100%; display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 4px; }
		.chips .cw, .chips .cw :global(.ui-chip) { width: 100%; min-width: 0; }
		.chips .cw :global(.ui-chip), .chips .cw :global(.main) { height: 100%; }
		.chips .cw :global(.main) { width: 100%; flex-direction: column; justify-content: center; gap: 1px; min-height: 48px; padding: 4px 2px; font-size: 12.5px; line-height: 1.15; text-align: center; white-space: normal; overflow-wrap: anywhere; }
		.tools { width: 100%; margin-left: 0; }
		.tools > :global(*) { flex: 1 1 auto; }
		.lhead { border-radius: var(--r-card); }
		.row { padding: 10px; gap: 10px; }
		.name { min-height: 32px; }
	}
	@media (prefers-reduced-motion: reduce) {
		.split, .list, .thumb, .cthumb, .sthumb, .row, .card, .sitem, .cw, .lhead { transition: none; }
		.stage-in, .full { animation: none; }
		a.pic:hover { transform: none; }
	}
</style>
