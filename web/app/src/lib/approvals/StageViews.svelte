<script lang="ts">
	import { prefetchParts } from './details.svelte.ts';
	// The Approvals page: a grouped list beside the picked ask, large. Status
	// tabs with counts, one Filter menu, Poster size, a list that hides
	// (remembered per member, like the Library), Group by (Type, Person or
	// Date, remembered per member), Review one by one, Choose all for bulk
	// decisions, and a 6 s Undo on every decision. The whole row only picks
	// the ask (its name button stretches over it); only the big panel leads
	// to the title's page. Below 1024 px an ask opens full screen.
	import { onMount, tick } from 'svelte';
	import Poster from '../Poster.svelte';
	import Icon from '../Icon.svelte';
	import Avatar from '../ui/Avatar.svelte';
	import Button from '../ui/Button.svelte';
	import IconButton from '../ui/IconButton.svelte';
	import Chip from '../ui/Chip.svelte';
	import Shelf from '../ui/Shelf.svelte';
	import VariantBar from '../ui/VariantBar.svelte';
	import LookViews from './LookViews.svelte';
	import Menu, { type MenuItem } from '../ui/Menu.svelte';
	import StatusPill from '../ui/StatusPill.svelte';
	import KidMark from './KidMark.svelte';
	import AskPanel from './AskPanel.svelte';
	import FocusView from './FocusView.svelte';
	import { ago, dateTime } from '../format.ts';
	import { kindWord, personGroups, type ApprovalsCtx } from './approvals.ts';
	import { type GroupBy } from './inbox.ts';
	import { askSummary, dateGroups, groupByWords, inboxTabs, loadGroupBy, loadPanel, loadSize, loadView, saveGroupBy, savePanel, saveSize, saveView, type InboxView, showNotOut, sizeLabels, watchSeason, type InboxSize } from './inbox.ts';
	import { artistPhoto } from './details.svelte.ts';
	import type { Ask } from '../types.ts';

	let { ctx }: { ctx: ApprovalsCtx } = $props();

	// Poster size and whether the list shows, remembered per member.
	let size = $state<InboxSize>('m');
	let listOpen = $state(true);
	let groupBy = $state<GroupBy>('type');
	// The poster wall (posters like Explore, asker on each) or the list beside the Stage.
	let view = $state<InboxView>('rows');
	// The five layouts on the pill bar (?view=1…5); 0 is the page as it is.
	let look = $state(0);
	const lookOptions = [
		{ value: 0, label: 'Now' },
		{ value: 1, label: 'Queue' },
		{ value: 2, label: 'Mod queue' },
		{ value: 3, label: 'Table + sheet' },
		{ value: 4, label: 'Overview' },
		{ value: 5, label: 'Ask cards' },
		{ value: 6, label: 'Poster sheet' },
		{ value: 7, label: 'Pick and act' },
		{ value: 8, label: 'People' },
		{ value: 9, label: 'Review window' },
		{ value: 10, label: 'Tracker' },
		{ value: 11, label: 'Desk' },
		{ value: 12, label: 'Screening Room' },
		{ value: 13, label: 'Up Next' },
		{ value: 14, label: 'Split stacks' },
		{ value: 15, label: 'Banner + Desk' },
		{ value: 16, label: 'Banner + shelves' },
		{ value: 17, label: 'Triage' },
		{ value: 18, label: 'By person' },
		{ value: 19, label: 'Desk + panel' }
	];
	function setView(v: InboxView) {
		view = v;
		saveView(ctx.me, v);
	}
	// On the wall the tick boxes and bulk bar wait for Select, like Photos.
	let selecting = $state(false);
	function setGroup(g: GroupBy) {
		groupBy = g;
		saveGroupBy(ctx.me, g);
	}
	let loadedFor = $state<number | undefined | null>(null);
	$effect(() => {
		const me = ctx.me;
		if (loadedFor === me) return;
		loadedFor = me;
		size = loadSize(me);
		listOpen = loadPanel(me);
		groupBy = loadGroupBy(me);
		view = loadView(me);
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
	// The picked ask's lists and the next one's are fetched ahead, so its seasons and albums open at once.
	$effect(() => prefetchParts(picked, ctx.shown[at + 1]));
	$effect(() => {
		for (const x of ctx.shown) ctx.want(x);
	});

	// Full screen (phones and tablets, and Approve with options in one-by-one review).
	let fullOpen = $state(false);
	let fullId = $state(0);
	const fullAsk = $derived(ctx.viewed.find((a) => a.id === fullId) ?? null);
	let opener: HTMLElement | null = null;
	// The ask opens in a window over the page (the browser's top layer, so no bar
	// covers its close button). Esc, the close button or a click outside closes it.
	let win: HTMLDialogElement | undefined = $state();
	async function openFull(a: Ask, el?: HTMLElement | null) {
		fullId = a.id;
		opener = el ?? null;
		fullOpen = true;
		await tick();
		if (win && !win.open) win.showModal();
		document.getElementById('stage-full-title')?.focus({ preventScroll: true });
	}
	async function closeFull() {
		if (win?.open) win.close();
		else closed();
	}
	function closed() {
		fullOpen = false;
		if (opener?.isConnected) opener.focus({ preventScroll: true });
	}
	$effect(() => {
		if (fullOpen && fullAsk && fullAsk.state !== 'asked' && ctx.tab === 'waiting') void closeFull();
	});

	function pick(a: Ask, el?: HTMLElement | null) {
		const i = ctx.shown.findIndex((x) => x.id === a.id);
		pickedId = a.id;
		lastAt = Math.max(i, 0);
		if (!wide || wall || look > 0) void openFull(a, el);
	}
	function step(by: number) {
		const n = ordered.length;
		if (!n) return;
		const cur = Math.max(ordered.findIndex((a) => a.id === picked?.id), 0);
		const next = ordered[(((cur + by) % n) + n) % n];
		pickedId = next.id;
		lastAt = Math.max(ctx.shown.indexOf(next), 0);
		void tick().then(() => document.querySelector<HTMLElement>(`[data-row="${next.id}"]`)?.scrollIntoView({ block: 'nearest', inline: 'nearest' }));
	}
	function listKeys(e: KeyboardEvent) {
		const keys = ['ArrowDown', 'ArrowUp'];
		if (!keys.includes(e.key)) return;
		if (e.key === 'Enter' || e.key === ' ') return;
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
	const line = (a: Ask) => askSummary(a, ctx.info(a)).line || (a.kind === 'artist' ? 'Every album' : a.year ? String(a.year) : kindWord(a.kind));
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
	// How the page lays asks out: wide rows (as Seerr's request list), the poster wall, or the list beside the Stage.
	const viewWords: Record<InboxView, string> = { rows: 'Rows', wall: 'Posters', list: 'List and panel' };
	const viewPick = $derived<MenuItem[]>([{ kind: 'heading', label: 'Layout' }, ...(['rows', 'wall', 'list'] as const).map((v) => ({ label: viewWords[v], checked: view === v, onselect: () => setView(v) }))]);
	// Rows sort by when asked or when last changed, newest first unless flipped.
	type SortBy = 'asked' | 'changed';
	let sortBy = $state<SortBy>('asked');
	let oldestFirst = $state(false);
	const changedAt = (a: Ask) => a.decided_at ?? a.asked_at;
	const sorted = $derived.by(() => {
		const at = (a: Ask) => Date.parse(sortBy === 'changed' ? changedAt(a) : a.asked_at) || 0;
		return [...ctx.shown].sort((x, y) => (oldestFirst ? at(x) - at(y) : at(y) - at(x)));
	});
	const statusItems = $derived<MenuItem[]>(tabs.map((t) => ({ label: `${t.label} (${ctx.counts[t.tab]})`, checked: ctx.tab === t.tab, onselect: () => ctx.setTab(t.tab) })));
	const sortItems = $derived<MenuItem[]>([
		{ label: 'Newest asks', checked: sortBy === 'asked', onselect: () => (sortBy = 'asked') },
		{ label: 'Last changed', checked: sortBy === 'changed', onselect: () => (sortBy = 'changed') }
	]);
	// The wall keeps one View menu: shelves by, filter and the list switch.
	const viewItems = $derived<MenuItem[]>([
		{ kind: 'heading', label: 'Shelves by' },
		...(['type', 'person', 'date'] as const).map((g) => ({ label: groupByWords[g], checked: groupBy === g, onselect: () => setGroup(g) })),
		{ kind: 'separator' },
		...filterItems,
		{ kind: 'separator' },
		...viewPick
	]);
	const people = $derived(personGroups(ctx.shown));
	const types = $derived(
		(['movie', 'show', 'artist'] as const)
			.map((k) => ({ id: k, name: typeWords[k], asks: ctx.shown.filter((a) => (k === 'movie' ? a.kind !== 'show' && a.kind !== 'artist' : a.kind === k)) }))
			.filter((g) => g.asks.length)
	);
	const groups = $derived<{ id: string | number; name: string; asks: Ask[] }[]>(groupBy === 'person' ? people : groupBy === 'date' ? dateGroups(ctx.shown) : types);
	// The picked ask and the stepper follow the list as it is grouped on screen.
	const ordered = $derived(groups.flatMap((g) => g.asks));
	const wall = $derived(view === 'wall' && !oneByOne && !look);
	const rowsView = $derived(view === 'rows' && !oneByOne && !look);
	const split = $derived(wide && !oneByOne && !wall && !rowsView && !look);
	const emptyWord = $derived(ctx.tab === 'waiting' ? 'Inbox clear' : `Nothing ${inboxTabs.find((t) => t.tab === ctx.tab)?.label.toLowerCase()}`);
</script>


{#snippet chooseBox(x: Ask)}
	{#if ctx.choosable}<input class="tick" type="checkbox" aria-label="Choose {x.name}" checked={ctx.isChosen(x.id)} onchange={() => ctx.toggleChosen(x.id)} />{/if}
{/snippet}
{#snippet thumb(x: Ask, cls = 'thumb')}
	{@const music = x.kind === 'artist'}
	{@const src = music ? artistPhoto(x) : x.poster}
	<span class={cls} data-testid="ask-picture">{#if music && src}<img src={src} alt="" loading="lazy" decoding="async" />{:else}<Poster src={music ? null : src} name={x.name} />{/if}</span>
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

<div class="stage size-{size}" class:collapsed={split && !listOpen} data-testid="approvals-stage" data-size={size} data-list={split ? (listOpen ? 'open' : 'hidden') : undefined}>
	<div class="sbar">
		{#if rowsView}
			<Menu label="Status" variant="button" text={`${tabs.find((t) => t.tab === ctx.tab)?.label ?? 'Waiting'} · ${ctx.counts[ctx.tab]}`} icon="chevronDown" items={statusItems} width={220} />
		{:else if ![2, 4, 8, 12, 14, 16].includes(look)}
		<div class="chips" role="group" aria-label="Show" data-testid="ask-tabs">
			{#each tabs as t (t.tab)}
				<span class="cw" class:zero={ctx.counts[t.tab] === 0}><Chip label={t.label} count={ctx.counts[t.tab]} selected={ctx.tab === t.tab} onclick={() => ctx.setTab(t.tab)} /></span>
			{/each}
		</div>
		{/if}
		<div class="tools">
			{#if look > 0}
				<Menu label="Filter asks" variant="button" text={filterOn ? `Filter · ${filterOn}` : 'Filter'} icon="chevronDown" items={filterItems} width={240} align="end" />
			{:else if rowsView}
				<Menu label="Sort" variant="button" text={sortBy === 'changed' ? 'Last changed' : 'Newest asks'} icon="chevronDown" items={sortItems} width={200} align="end" />
				<IconButton icon="chevronUpDown" label={oldestFirst ? 'Oldest first' : 'Newest first'} onclick={() => (oldestFirst = !oldestFirst)} />
				<Menu label="Filter asks" variant="button" text={filterOn ? `Filter · ${filterOn}` : 'Filter'} icon="chevronDown" items={filterItems} width={240} align="end" />
				<Menu label="Layout" variant="button" text="Rows" icon="chevronDown" items={viewPick} width={200} align="end" />
			{:else if wall}
				{#if ctx.choosable && ctx.shown.length > 1}
					<Button variant={selecting ? 'primary' : 'secondary'} size="md" data-testid="select" onclick={() => { selecting = !selecting; if (!selecting) for (const a of chosen) ctx.toggleChosen(a.id); }}>{selecting ? 'Done' : 'Select'}</Button>
				{/if}
				<Menu label="View" variant="button" text={filterOn ? `View · ${filterOn}` : 'View'} icon="chevronDown" items={viewItems} width={240} align="end" />
			{:else}
				<Menu label="Filter asks" variant="button" text={filterOn ? `Filter · ${filterOn}` : 'Filter'} icon="chevronDown" items={filterItems} width={240} align="end" />
				{#if !oneByOne}<Menu label="Poster size" variant="button" text={sizeLabels[size]} icon="chevronDown" items={sizeItems} width={200} align="end" />{/if}
				{#if !oneByOne}<Menu label="Layout" variant="button" text="List" icon="chevronDown" items={viewPick} width={200} align="end" />{/if}
				{#if split}
					<Button variant="secondary" size="md" icon="sidebar" aria-expanded={listOpen} aria-controls={listId} data-testid="list-toggle" onclick={() => setList(!listOpen)}>{listOpen ? 'Hide list' : 'Show list'}</Button>
				{/if}
				{#if ctx.approver && ctx.tab === 'waiting' && ctx.shown.length}
					<Button variant={oneByOne ? 'primary' : 'secondary'} size="md" icon={oneByOne ? 'back' : 'eye'} data-testid="one-by-one" onclick={() => (oneByOne = !oneByOne)}>{oneByOne ? 'Back to the list' : 'Review one by one'}</Button>
				{/if}
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
		{#if ctx.choosable && !look && (!wall || selecting)}
			<div class="lhead" class:on={chosen.length > 0} data-testid="ask-bulk">
				<label class="all"><input class="tick" type="checkbox" checked={allChosen} indeterminate={chosen.length > 0 && !allChosen} onchange={() => ctx.chooseAll(ctx.shown)} />{chosen.length ? `${chosen.length} chosen` : 'Choose all'}</label>
				{#if chosen.length}
					<Button size="sm" variant="primary" icon="check" type="button" onclick={() => ctx.approve(chosen)}>Approve {chosen.length}</Button>
					<Button size="sm" icon="x" type="button" onclick={() => ctx.turnDown(chosen)}>Turn down {chosen.length}</Button>
				{/if}
				{#if split && ctx.shown.length > 1}
					<span class="stepper">
						<IconButton icon="chevronLeft" size="sm" label="The ask before" onclick={() => step(-1)} />
						<span class="count">{Math.max(ordered.findIndex((a) => a.id === picked?.id), 0) + 1} of {ctx.shown.length}</span>
						<IconButton icon="chevronRight" size="sm" label="The next ask" onclick={() => step(1)} />
					</span>
				{:else}
					<span class="count">{ctx.shown.length} {ctx.shown.length === 1 ? 'ask' : 'asks'}</span>
				{/if}
			</div>
		{/if}

		{#if look > 0}
			<LookViews {ctx} {look} {line} open={pick} />
		{:else if rowsView}
			<ul class="wrows" aria-label="Asks" data-testid="ask-rows">
				{#each sorted as x (x.id)}
					{@const p = ctx.who(x)}
					<li class="wr" class:chosen={ctx.isChosen(x.id)} class:sending={ctx.isPending(x.id)} data-testid="ask">
						{#if x.poster}<div class="wr-bg" aria-hidden="true" style:background-image={`url("${x.poster}")`}></div>{/if}
						{@render chooseBox(x)}
						<button class="wr-art" type="button" aria-label="Open {x.name}" onclick={(e) => pick(x, e.currentTarget)}>{@render thumb(x, 'cover')}</button>
						<div class="wr-main">
							<span class="wr-k">{kindWord(x.kind)}{x.year ? ` · ${x.year}` : ''}</span>
							<button class="wr-name" type="button" data-row={x.id} data-testid="ask-name" onclick={(e) => pick(x, e.currentTarget)}>{x.name}</button>
							{#if x.kind !== 'movie'}<span class="wr-line">{line(x)}</span>{/if}
							{#if notOut(x)}<span class="soon">Not out yet</span>{/if}
						</div>
						<dl class="wr-meta">
							<dt>Status</dt><dd><StatusPill kind={ctx.pill(x).kind} label={ctx.pill(x).label} /></dd>
							<dt>Asked</dt><dd><span class="who"><Avatar name={p.name} src={p.avatar} size={20} /><span class="wn">{p.name}</span>{#if p.kid}<KidMark label="Kid" size="sm" />{/if}</span> · <time datetime={x.asked_at} title={dateTime(x.asked_at)}>{ago(x.asked_at)}</time></dd>
							{#if ctx.decided(x)}<dt>Changed</dt><dd>{ctx.decided(x)}{#if x.decided_at} · <time datetime={x.decided_at} title={dateTime(x.decided_at)}>{ago(x.decided_at)}</time>{/if}</dd>{/if}
						</dl>
						<div class="wr-act">
							{#if waiting(x)}
								<Button size="md" variant="primary" icon="check" type="button" data-testid="ask-approve" onclick={() => ctx.approve([x])}>Approve</Button>
								<Button size="md" icon="x" type="button" data-testid="ask-decline" onclick={() => ctx.turnDown([x])}>Turn down</Button>
							{:else if ctx.isPending(x.id)}
								<span class="sending-word">Sending…</span>
							{:else}
								<Button size="md" type="button" onclick={(e: MouseEvent) => pick(x, e.currentTarget as HTMLElement)}>Details</Button>
							{/if}
						</div>
					</li>
				{/each}
			</ul>
		{:else if wall}
			{@const hero = ctx.tab === 'waiting' ? (ordered.find(waiting) ?? null) : null}
			{#if hero}
				{@const hw = ctx.who(hero)}
				<section class="hero" aria-label="Newest ask" data-testid="ask-hero">
					{#if hero.poster}<div class="hero-bg" aria-hidden="true" style:background-image={`url("${hero.poster}")`}></div>{/if}
					<button class="hero-art" type="button" aria-label="Open {hero.name}" onclick={(e) => pick(hero, e.currentTarget)}>{@render thumb(hero, 'cover')}</button>
					<div class="hero-txt">
						<span class="hero-k">{kindWord(hero.kind)} · {line(hero)}</span>
						<h2 class="hero-name">{hero.name}</h2>
						<span class="who hero-who"><Avatar name={hw.name} src={hw.avatar} size={24} /><span>{hw.name} asked <time datetime={hero.asked_at} title={dateTime(hero.asked_at)}>{ago(hero.asked_at)}</time></span>{#if hw.kid}<KidMark label="Kid" size="sm" />{/if}</span>
						{#if hero.note}<p class="hero-note">&ldquo;{hero.note}&rdquo;</p>{/if}
						<div class="hero-act">
							<Button variant="primary" size="lg" icon="check" type="button" data-testid="hero-approve" onclick={() => ctx.approve([hero])}>Approve</Button>
							<Button size="lg" icon="x" type="button" data-testid="hero-decline" onclick={() => ctx.turnDown([hero])}>Turn down</Button>
							<Button variant="plain" size="lg" type="button" onclick={(e: MouseEvent) => pick(hero, e.currentTarget as HTMLElement)}>Details</Button>
						</div>
					</div>
				</section>
			{/if}
			<div class="shelves" data-testid="ask-wall">
				{#each groups as g (g.id)}
					{@const w = groupBy === 'person' ? ctx.who(g.asks[0]) : null}
					<Shelf title={`${w ? w.name : g.name} · ${g.asks.length}`} item="var(--ask-item)" items={g.asks} key={(x) => x.id} lines={2}>
						{#snippet card(x: Ask)}
							{@const p = ctx.who(x)}
							<div class="tile" class:chosen={ctx.isChosen(x.id)} class:sending={ctx.isPending(x.id)} data-testid="ask">
								<div class="art">
									{@render thumb(x, 'cover')}
									{#if groupBy !== 'person'}<span class="asker" title="Asked by {p.name}"><Avatar name={p.name} src={p.avatar} size={28} /></span>{/if}
									{#if selecting}<span class="tick-at">{@render chooseBox(x)}</span>{/if}
									{#if x.state !== 'asked' || ctx.isPending(x.id)}<span class="pill-at">{@render quick(x)}</span>{/if}
								</div>
								<button class="name" type="button" data-row={x.id} data-testid="ask-name" onclick={(e) => (selecting && waiting(x) ? ctx.toggleChosen(x.id) : pick(x, e.currentTarget))}>{x.name}</button>
								<span class="sub">{#if groupBy !== 'person'}{p.name} · {/if}{notOut(x) ? 'Not out yet' : line(x)}</span>
							</div>
						{/snippet}
					</Shelf>
				{/each}
			</div>
		{:else}
		<div class="split">
			<!-- svelte-ignore a11y_no_static_element_interactions -->
			<div class="list" id={listId} onkeydown={listKeys} hidden={split && !listOpen ? true : undefined} inert={split && !listOpen}>
				<div class="groupby" role="group" aria-label="Group by" data-testid="group-by">
					<span class="gb-l">Group by</span>
					{#each ['type', 'person', 'date'] as const as g (g)}
						<button type="button" class="gb" aria-pressed={groupBy === g} onclick={() => setGroup(g)}>{groupByWords[g]}</button>
					{/each}
				</div>
				<div class="groups">
					{#each groups as g (g.id)}
						{@const w = groupBy === 'person' ? ctx.who(g.asks[0]) : null}
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
								{#each g.asks as x (x.id)}{@render row(x, groupBy !== 'person', groupBy !== 'type')}{/each}
							</ul>
						</section>
					{/each}
				</div>
			</div>

			{#if split && picked}
				<section class="main" aria-label="The picked ask" data-testid="ask-stage">
					{#key picked.id}<div class="stage-in"><AskPanel {ctx} a={picked} variant="stage" headingId="stage-title" /></div>{/key}
				</section>
			{/if}
		</div>
		{/if}
	{/if}
</div>

<VariantBar options={lookOptions} bind:value={look} storageKey="holyarrs.approvals-look" label="Approvals layout" />

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
<dialog class="win" bind:this={win} aria-label={fullAsk?.name ?? 'Ask'} data-testid="ask-full" onclose={closed} onclick={(e) => e.target === win && void closeFull()}>
	{#if fullOpen && fullAsk}
		<div class="win-in">
			<div class="win-bar">
				<span class="win-t">{fullAsk.name}</span>
				<button class="win-x" type="button" aria-label="Close" onclick={() => void closeFull()}><Icon name="x" size={18} /></button>
			</div>
			<div class="win-body">{#key fullAsk.id}<AskPanel {ctx} a={fullAsk} headingId="stage-full-title" />{/key}</div>
		</div>
	{/if}
</dialog>

<style>
	/* One size per element: --thumb for list pictures, --lw for the list column. */
	.stage { --thumb: 48px; --lw: 360px; min-width: 0; }
	.size-s { --thumb: 36px; --lw: 320px; }
	.size-l { --thumb: 64px; --lw: 400px; }

	.sbar { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 16px; margin-bottom: 12px; }
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
	.list { min-width: 0; }
	.split > .list { position: sticky; top: calc(var(--topbar) + 12px); max-height: calc(100dvh - var(--topbar) - 32px); overflow-y: auto; overscroll-behavior: contain; scrollbar-gutter: stable; border-radius: var(--r-panel); transition: opacity var(--dur) var(--ease-out), translate var(--dur) var(--ease-out); }
	:global(html.bar-hidden) .split > .list { top: 12px; max-height: calc(100dvh - 32px); }
	.list[hidden] { display: block; visibility: hidden; opacity: 0; translate: -24px 0; overflow: hidden; }
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
	.row.chosen { background: color-mix(in srgb, var(--accent) 10%, transparent); }
	.sending { opacity: 0.6; }
	.thumb { display: block; flex: none; width: var(--thumb); border-radius: 6px; overflow: hidden; transition: width var(--dur) var(--ease-out), transform var(--dur-fast) var(--ease-out); }
	.round { aspect-ratio: 1; border-radius: 50%; background: var(--surface-3); --poster-ratio: 1; }
	.round img { width: 100%; height: 100%; object-fit: cover; display: block; }
	/* The whole row is one target: the name button stretches over it; the tick and quick buttons sit above. */
	.name::after { content: ''; position: absolute; inset: 0; border-radius: inherit; }
	.row { position: relative; cursor: pointer; }
	.tick, .quick { position: relative; z-index: 1; }
	.name:focus-visible { outline: none; }
	.row:has(.name:focus-visible) { outline: none; box-shadow: inset 0 0 0 2px var(--accent), var(--focus-ring); }
	.row:has(.name:active) { background: var(--surface-3); }
	.row:has(.name:active) .thumb { transform: scale(0.98); }
	.txt { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
	.r1 { display: flex; align-items: center; gap: 8px; min-width: 0; }
	.r2, .r3 { display: flex; align-items: center; gap: 6px; min-width: 0; color: var(--text-muted); font: var(--t-footnote); }
	.r3 { font: var(--t-caption); }
	.tag { flex: none; align-self: flex-start; padding: 1px 7px; border-radius: var(--r-sm); background: var(--surface-2); color: var(--text-muted); font: var(--t-caption); font-weight: 600; white-space: nowrap; }
	.r1 .tag { align-self: center; }
	.row.on .tag { background: var(--surface-1); }
	.name { min-width: 0; max-width: 100%; padding: 0; border: 0; background: none; color: inherit; font: var(--t-subhead); font-weight: 600; text-align: left; cursor: pointer; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; border-radius: 4px; }
	.sline { min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.soon { flex: none; padding: 0 6px; border-radius: var(--r-sm); background: color-mix(in srgb, var(--wait) 14%, transparent); color: var(--wait-text); font: var(--t-caption); font-weight: 600; white-space: nowrap; }
	.dot { flex: none; }
	.who { display: inline-flex; align-items: center; gap: 5px; min-width: 0; }
	.wn { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.quick { display: inline-flex; gap: 6px; flex: none; }
	.sending-word { flex: none; color: var(--text-muted); font: var(--t-footnote); }
	/* Every list picture is one 2:3 frame; an artist photo fills it. Rows keep three one-line lines. */
	.thumb { aspect-ratio: 2 / 3; --poster-ratio: 2 / 3; background: var(--surface-3); }
	.thumb > img { width: 100%; height: 100%; object-fit: cover; object-position: center 25%; display: block; }
	.r2, .r3 { flex-wrap: nowrap; white-space: nowrap; min-height: 1.4em; }
	.r3 time { flex: none; white-space: nowrap; }

	/* Groups (2 By person, 5 By type) */
	.groups { display: flex; flex-direction: column; gap: 14px; }
	.gh { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; min-height: 32px; margin: 0 4px 6px; }
	.gh h2 { font: var(--t-headline); }
	.gn { margin-right: auto; padding: 0 7px; border-radius: var(--r-pill); background: var(--surface-2); color: var(--text-muted); font: var(--t-caption); font-weight: 600; font-variant-numeric: tabular-nums; }

	/* Group by */
	.groupby { display: flex; align-items: center; gap: 4px; margin: 0 4px 10px; }
	.gb-l { margin-right: 4px; color: var(--text-muted); font: var(--t-footnote); }
	.gb { min-height: 32px; padding: 0 12px; border: 0; border-radius: var(--r-pill); background: var(--surface-2); color: var(--text); font: var(--t-footnote); font-weight: 600; cursor: pointer; transition: background var(--dur-fast) var(--ease-out); }
	.gb:hover { background: var(--surface-3); }
	.gb[aria-pressed='true'] { background: var(--accent-fill); color: var(--on-accent, #fff); }
	.gb:focus-visible { outline: none; box-shadow: var(--focus-ring); }

	/* The poster wall: Explore's shelves and card width, a hero for the newest ask. */
	.stage { --ask-item: 168px; }
	.shelves { display: flex; flex-direction: column; gap: 28px; }
	.tile { position: relative; display: flex; flex-direction: column; gap: 2px; min-width: 0; transition: opacity var(--dur-fast) var(--ease-out); }
	.tile .art { position: relative; margin-bottom: 6px; border-radius: 10px; box-shadow: 0 0 0 1px var(--line); transition: transform var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out); }
	.tile:has(.name:hover) .art { transform: scale(1.03); }
	.tile:has(.name:active) .art { transform: scale(0.98); }
	.tile.chosen .art { box-shadow: 0 0 0 3px var(--accent); }
	.cover { display: block; aspect-ratio: 2 / 3; --poster-ratio: 2 / 3; border-radius: 10px; overflow: hidden; background: var(--surface-3); }
	.cover > img { width: 100%; height: 100%; object-fit: cover; object-position: center 25%; display: block; }
	.asker { position: absolute; right: 6px; bottom: 6px; display: inline-flex; border-radius: 50%; box-shadow: 0 0 0 2px var(--bg); }
	.tick-at { position: absolute; left: 8px; top: 8px; z-index: 1; pointer-events: none; }
	.pill-at { position: absolute; left: 6px; top: 6px; }
	.tile .name { min-height: 22px; }
	.sub { color: var(--text-muted); font: var(--t-caption); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.tile:has(.name:focus-visible) .art { box-shadow: 0 0 0 2px var(--accent), var(--focus-ring); }

	.hero { position: relative; isolation: isolate; overflow: hidden; display: flex; gap: 24px; align-items: flex-end; margin-bottom: 28px; padding: 24px; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.hero-bg { position: absolute; inset: -40px; z-index: -1; background-size: cover; background-position: center 30%; filter: blur(40px) saturate(1.3); opacity: 0.45; }
	.hero-art { flex: none; width: 180px; padding: 0; border: 0; background: none; border-radius: 12px; overflow: hidden; cursor: pointer; box-shadow: 0 12px 32px rgb(0 0 0 / 0.35); }
	.hero-art:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.hero-txt { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
	.hero-k { color: var(--text-muted); font: var(--t-subhead); }
	.hero-name { font: var(--t-title1); overflow-wrap: anywhere; }
	.hero-who { font: var(--t-callout); color: var(--text); }
	.hero-note { font: var(--t-callout); color: var(--text); }
	.hero-act { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 10px; }

	/* Rows: one wide card per ask, as Seerr's request list; poster blurred behind, facts in the middle, answers on the right. */
	.wrows { display: flex; flex-direction: column; gap: 10px; margin: 0; padding: 0; list-style: none; }
	.wr { position: relative; isolation: isolate; overflow: hidden; display: flex; align-items: center; gap: 16px; padding: 12px 16px; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); transition: opacity var(--dur-fast) var(--ease-out); }
	.wr.chosen { box-shadow: 0 0 0 2px var(--accent); }
	.wr-bg { position: absolute; inset: 0; z-index: -1; background-size: cover; background-position: center 30%; filter: blur(24px) saturate(1.3); opacity: 0.28; mask-image: linear-gradient(90deg, #000 0%, transparent 75%); pointer-events: none; }
	.wr-art { flex: none; width: 72px; padding: 0; border: 0; background: none; border-radius: 8px; overflow: hidden; cursor: pointer; }
	.wr-art:focus-visible, .wr-name:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.wr-main { flex: 1 1 200px; min-width: 0; display: flex; flex-direction: column; gap: 2px; align-items: flex-start; }
	.wr-k { color: var(--text-muted); font: var(--t-footnote); }
	.wr-name { max-width: 100%; padding: 0; border: 0; background: none; color: var(--text); font: var(--t-headline); text-align: left; cursor: pointer; overflow-wrap: anywhere; border-radius: 4px; }
	.wr-name:hover { text-decoration: underline; text-underline-offset: 3px; }
	.wr-line { color: var(--text-muted); font: var(--t-callout); }
	.wr-meta { flex: 1 1 260px; min-width: 0; display: grid; grid-template-columns: auto minmax(0, 1fr); align-items: center; gap: 6px 12px; margin: 0; font: var(--t-footnote); }
	.wr-meta dt { color: var(--text-muted); }
	.wr-meta dd { margin: 0; min-width: 0; display: flex; align-items: center; gap: 4px; flex-wrap: wrap; }
	.wr-act { flex: none; display: flex; flex-direction: column; gap: 8px; min-width: 140px; }

	.calm { display: flex; flex-direction: column; align-items: center; gap: 6px; padding: 56px 20px; text-align: center; color: var(--text-muted); border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.calm h2 { font: var(--t-headline); color: var(--text); }
	.muted { color: var(--text-muted); }

	/* The ask window: a large sheet over a dimmed page; full screen on phones. */
	.win { width: min(960px, calc(100vw - 48px)); max-width: none; height: min(880px, calc(100dvh - 48px)); max-height: none; margin: auto; padding: 0; border: 0; border-radius: var(--r-modal); background: var(--bg); color: var(--text); box-shadow: 0 0 0 1px var(--line), var(--shadow-modal); overflow: hidden; }
	.win[open] { animation: win-in var(--dur) var(--ease-out) both; }
	.win::backdrop { background: rgb(0 0 0 / 0.55); -webkit-backdrop-filter: blur(6px); backdrop-filter: blur(6px); animation: fade-in var(--dur) var(--ease-out) both; }
	.win-in { display: flex; flex-direction: column; height: 100%; }
	.win-bar { flex: none; display: flex; align-items: center; gap: 12px; min-height: 52px; padding: 0 10px 0 20px; border-bottom: 1px solid var(--line); background: color-mix(in srgb, var(--bg) 85%, transparent); }
	.win-t { flex: 1; min-width: 0; font: var(--t-headline); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.win-x { display: grid; place-items: center; width: 36px; height: 36px; border: 0; border-radius: 50%; background: var(--surface-2); color: var(--text); cursor: pointer; transition: background var(--dur-fast) var(--ease-out); }
	.win-x:hover { background: var(--surface-3); }
	.win-x:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.win-body { flex: 1; min-height: 0; overflow-y: auto; overscroll-behavior: contain; padding: 20px 24px 32px; }
	@keyframes win-in { from { opacity: 0; transform: translateY(12px) scale(0.985); } }
	@keyframes fade-in { from { opacity: 0; } }
	@media (max-width: 767px) {
		.win { width: 100vw; height: 100dvh; border-radius: 0; }
		.win-bar { padding-top: env(safe-area-inset-top, 0px); min-height: calc(52px + env(safe-area-inset-top, 0px)); }
		.win-x { width: 44px; height: 44px; }
		.win-body { padding: 16px 16px calc(24px + env(safe-area-inset-bottom, 0px)); }
	}

	@media (max-width: 1023px) {
		.split { grid-template-columns: minmax(0, 1fr); }
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
		.stage { --ask-item: 38vw; }
		.wr { flex-wrap: wrap; padding: 12px; gap: 12px; }
		.wr-art { width: 64px; }
		.wr-meta { order: 3; width: 100%; grid-template-columns: auto minmax(0, 1fr); }
		.wr-act { order: 4; width: 100%; }
		.wr-act > :global(*) { flex: 1 1 auto; }
		.hero { flex-direction: column; align-items: flex-start; gap: 14px; padding: 16px; }
		.hero-art { width: 120px; }
		.hero-name { font: var(--t-title2); }
		.hero-act { width: 100%; }
		.hero-act > :global(*) { flex: 1 1 auto; }
		.name { min-height: 32px; }
	}
	@media (prefers-reduced-motion: reduce) {
		.split, .list, .thumb, .row, .cw, .lhead { transition: none; }
		.stage-in, .win[open], .win::backdrop { animation: none; }
	}
</style>
