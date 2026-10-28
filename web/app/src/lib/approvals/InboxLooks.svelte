<script lang="ts">
	// The Approvals Inbox in five arrangements (?inbox=1…5), all with the same
	// controls: status tabs with counts (like Downloads), one Filter menu
	// (Show mine or everyone's, type, Asked by), Poster size and the side panel
	// (remembered per member, like the Library), Review one by one, and one
	// Choose all for bulk decisions.
	//   1 Rows      two-line rows, the full ask on the right
	//   2 Table     dense table rows like Downloads, the full ask on the right
	//   3 By person rows under each person who asked
	//   4 Posters   big poster rows with the summary and season pills inline, a slim panel
	//   5 Stage     the picked ask large, the list as a slim strip on the right
	// Below 1024 px every arrangement is a list, and an ask opens full screen.
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
	import AskSummary from './AskSummary.svelte';
	import FocusView from './FocusView.svelte';
	import { glide } from '../ui/transitions.ts';
	import { ago, dateTime } from '../format.ts';
	import { kindWord, personGroups, type ApprovalsCtx } from './approvals.ts';
	import { askSummary, inboxTabs, loadPanel, loadSize, movieRelease, savePanel, saveSize, showNotOut, sizeLabels, watchSeason, type InboxSize } from './inbox.ts';
	import type { Ask } from '../types.ts';

	let { ctx, look }: { ctx: ApprovalsCtx; look: number } = $props();

	// Poster size and the side panel, remembered per member.
	let size = $state<InboxSize>('m');
	let panel = $state(true);
	let loadedFor = $state<number | undefined | null>(null);
	$effect(() => {
		const me = ctx.me;
		if (loadedFor === me) return;
		loadedFor = me;
		size = loadSize(me);
		panel = loadPanel(me);
	});
	function setSize(z: InboxSize) {
		size = z;
		saveSize(ctx.me, z);
	}
	function setPanel(open: boolean) {
		panel = open;
		savePanel(ctx.me, open);
	}
	const panelId = 'approvals-panel';

	// The side panel needs room: from 1024 px; narrower screens open an ask full screen.
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
		document.getElementById('inbox-full-title')?.focus({ preventScroll: true });
	}
	async function closeFull() {
		fullOpen = false;
		await tick();
		if (opener?.isConnected) opener.focus();
	}
	// A decision from the full screen closes it once the ask has moved on.
	$effect(() => {
		if (fullOpen && fullAsk && fullAsk.state !== 'asked' && ctx.tab === 'waiting') void closeFull();
	});

	function pick(a: Ask, el?: HTMLElement | null) {
		const i = ctx.shown.findIndex((x) => x.id === a.id);
		pickedId = a.id;
		lastAt = Math.max(i, 0);
		if (!wide) {
			void openFull(a, el);
			return;
		}
		if (!panel) setPanel(true);
	}
	function listKeys(e: KeyboardEvent) {
		if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
		const rows = [...((e.currentTarget as HTMLElement).querySelectorAll<HTMLElement>('[data-row]') ?? [])];
		const i = rows.indexOf(document.activeElement as HTMLElement);
		if (i < 0) return;
		e.preventDefault();
		const n = Math.min(Math.max(i + (e.key === 'ArrowDown' ? 1 : -1), 0), rows.length - 1);
		rows[n]?.focus();
		const id = Number(rows[n]?.dataset.row);
		const a = ctx.shown.find((x) => x.id === id);
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

	// What a row says.
	const line = (a: Ask) => askSummary(a, ctx.info(a), ctx.details(a)?.runtime).line;
	const notOut = (a: Ask) => {
		const info = ctx.info(a);
		if (a.kind === 'movie') return movieRelease(info).notOut;
		const s = watchSeason(a, info);
		return s ? showNotOut(s, ctx.episodes(a.tmdb_id, s)) : '';
	};
	const ruleWord = (a: Ask) => {
		const d = ctx.defaultRule(a);
		return d ? `Default rule (${d})` : 'Default rule';
	};
	const waiting = (a: Ask) => ctx.approver && a.state === 'asked' && !ctx.isPending(a.id);

	// Choose all, once, above every arrangement (the table puts it in its header).
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
	const groups = $derived(personGroups(ctx.shown));
	const split = $derived(wide && !oneByOne);
	const emptyWord = $derived(ctx.tab === 'waiting' ? 'Inbox clear' : `Nothing ${inboxTabs.find((t) => t.tab === ctx.tab)?.label.toLowerCase()}`);
</script>

<svelte:window onkeydown={(e) => fullOpen && e.key === 'Escape' && !document.querySelector('dialog[open]') && void closeFull()} />

{#snippet chooseBox(x: Ask)}
	{#if ctx.choosable}<input class="tick" type="checkbox" aria-label="Choose {x.name}" checked={ctx.isChosen(x.id)} onchange={() => ctx.toggleChosen(x.id)} />{/if}
{/snippet}
{#snippet thumb(x: Ask)}
	<span class="thumb" class:square={x.kind === 'artist'}><Poster src={x.poster} name={x.name} /></span>
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
{#snippet row(x: Ask, person = true)}
	{@const n = notOut(x)}
	<li class="row" class:on={split && picked?.id === x.id} class:chosen={ctx.isChosen(x.id)} class:sending={ctx.isPending(x.id)} data-testid="ask">
		{@render chooseBox(x)}
		<button class="hit" type="button" data-row={x.id} aria-current={split && picked?.id === x.id ? 'true' : undefined} aria-controls={split ? panelId : undefined} onclick={(e) => pick(x, e.currentTarget)}>
			{@render thumb(x)}
			<span class="txt">
				<span class="r1"><span class="tag">{kindWord(x.kind)}</span><span class="name">{x.name}</span><time class="age" datetime={x.asked_at} title={dateTime(x.asked_at)}>{ago(x.asked_at)}</time></span>
				<span class="r2"><span class="sline">{line(x) || ctx.what(x) || (x.year ? String(x.year) : '')}</span>{#if n}<span class="soon">Not out yet</span>{/if}{#if person}<span class="dot" aria-hidden="true">·</span>{@render whoLine(x)}{/if}</span>
			</span>
		</button>
		{@render quick(x)}
	</li>
{/snippet}

<div class="inbox look-{look} size-{size}" data-testid="approvals-inbox" data-look={look} data-size={size} data-panel={wide ? (panel ? 'open' : 'hidden') : undefined}>
	<div class="bar">
		<div class="chips" role="group" aria-label="Show" data-testid="ask-tabs">
			{#each tabs as t (t.tab)}
				<span class="cw" class:zero={ctx.counts[t.tab] === 0}><Chip label={t.label} count={ctx.counts[t.tab]} selected={ctx.tab === t.tab} onclick={() => ctx.setTab(t.tab)} /></span>
			{/each}
		</div>
		<div class="tools">
			<Menu label="Filter asks" variant="button" text={filterOn ? `Filter · ${filterOn}` : 'Filter'} icon="chevronDown" items={filterItems} width={240} align="end" />
			{#if !oneByOne}<Menu label="Poster size" variant="button" text={sizeLabels[size]} icon="chevronDown" items={sizeItems} width={200} align="end" />{/if}
			{#if wide && !oneByOne}
				<Button variant="secondary" size="md" icon="sidebar" aria-expanded={panel} aria-controls={panelId} data-testid="panel-toggle" onclick={() => setPanel(!panel)}>{look === 5 ? (panel ? 'Hide list' : 'Show list') : panel ? 'Hide panel' : 'Show panel'}</Button>
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
				{#if look !== 2 || !wide}
					<label class="all"><input type="checkbox" checked={allChosen} indeterminate={chosen.length > 0 && !allChosen} onchange={() => ctx.chooseAll(ctx.shown)} />{chosen.length ? `${chosen.length} chosen` : 'Choose all'}</label>
				{:else}
					<span class="all">{chosen.length ? `${chosen.length} chosen` : 'Choose asks in the table'}</span>
				{/if}
				{#if chosen.length}
					<Button size="sm" variant="primary" icon="check" type="button" onclick={() => ctx.approve(chosen)}>Approve {chosen.length}</Button>
					<Button size="sm" icon="x" type="button" onclick={() => ctx.turnDown(chosen)}>Turn down {chosen.length}</Button>
				{/if}
				<span class="count">{ctx.shown.length} {ctx.shown.length === 1 ? 'ask' : 'asks'}</span>
			</div>
		{/if}

		<div class="split" class:open={split && panel}>
			<!-- svelte-ignore a11y_no_static_element_interactions -->
			<div class="list-col" onkeydown={listKeys}>
				{#if look === 5 && split}
					{#if picked}
						{#key picked.id}<div class="stage-wrap"><AskPanel {ctx} a={picked} variant="stage" headingId="inbox-stage-title" /></div>{/key}
					{/if}
				{:else if look === 2 && wide}
					<div class="tbl-wrap">
						<table class="tbl" data-testid="ask-table">
							<thead>
								<tr>
									{#if ctx.choosable}<th class="c-tick"><input class="tick" type="checkbox" aria-label="Choose all" checked={allChosen} indeterminate={chosen.length > 0 && !allChosen} onchange={() => ctx.chooseAll(ctx.shown)} /></th>{/if}
									<th class="c-art"><span class="sr-only">Poster</span></th>
									<th class="c-title">Title</th>
									<th class="c-for">Asked for</th>
									<th class="c-rule">Rule</th>
									<th class="c-who">Asked by</th>
									<th class="c-when">When</th>
									<th class="c-act"><span class="sr-only">Decide</span></th>
								</tr>
							</thead>
							<tbody>
								{#each ctx.shown as x (x.id)}
									{@const n = notOut(x)}
									<tr class:on={split && panel && picked?.id === x.id} class:chosen={ctx.isChosen(x.id)} class:sending={ctx.isPending(x.id)} data-testid="ask">
										{#if ctx.choosable}<td class="c-tick">{@render chooseBox(x)}</td>{/if}
										<td class="c-art">{@render thumb(x)}</td>
										<td class="c-title"><button class="cell-hit" type="button" data-row={x.id} aria-current={split && panel && picked?.id === x.id ? 'true' : undefined} aria-controls={panelId} onclick={(e) => pick(x, e.currentTarget)}><span class="tag">{kindWord(x.kind)}</span><span class="name">{x.name}</span></button></td>
										<td class="c-for"><span class="sline">{line(x) || ctx.what(x)}</span>{#if n}<span class="soon">Not out yet</span>{/if}</td>
										<td class="c-rule"><span class="sline">{ruleWord(x)}</span></td>
										<td class="c-who">{@render whoLine(x)}</td>
										<td class="c-when"><time datetime={x.asked_at} title={dateTime(x.asked_at)}>{ago(x.asked_at)}</time></td>
										<td class="c-act">{@render quick(x)}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{:else if look === 3}
					<div class="groups">
						{#each groups as g (g.id)}
							{@const first = g.asks[0]}
							{@const w = ctx.who(first)}
							<section class="group" aria-labelledby="g-{g.id}">
								<header class="gh">
									<Avatar name={w.name} src={w.avatar} size={28} />
									<h2 id="g-{g.id}">{w.name}</h2>
									<KidMark label={w.kid} size="sm" />
									<span class="gn">{g.asks.length} {g.asks.length === 1 ? 'ask' : 'asks'}</span>
									{#if ctx.choosable && g.asks.filter(waiting).length > 1}
										<Button size="sm" variant="secondary" icon="check" type="button" onclick={() => ctx.approve(g.asks.filter(waiting))}>Approve all {g.asks.filter(waiting).length}</Button>
									{/if}
								</header>
								<ul class="rows" aria-label="{w.name}'s asks">
									{#each g.asks as x (x.id)}{@render row(x, false)}{/each}
								</ul>
							</section>
						{/each}
					</div>
				{:else if look === 4}
					<ul class="big" aria-label="Asks">
						{#each ctx.shown as x (x.id)}
							<li animate:glide class="card" class:on={split && panel && picked?.id === x.id} class:chosen={ctx.isChosen(x.id)} class:sending={ctx.isPending(x.id)} data-testid="ask">
								{@render chooseBox(x)}
								<button class="big-art" type="button" data-row={x.id} aria-label="Open {x.name}" aria-current={split && panel && picked?.id === x.id ? 'true' : undefined} aria-controls={split ? panelId : undefined} onclick={(e) => pick(x, e.currentTarget)}>{@render thumb(x)}</button>
								<div class="big-body">
									<div class="r1"><span class="tag">{kindWord(x.kind)}</span><h2 class="bname"><button class="name-btn" type="button" onclick={(e) => pick(x, e.currentTarget)}>{x.name}</button></h2></div>
									<AskSummary {ctx} a={x} pills />
									<p class="meta"><span class="rule">{ruleWord(x)}</span><span class="dot" aria-hidden="true">·</span>{@render whoLine(x)}<span class="dot" aria-hidden="true">·</span><time datetime={x.asked_at} title={dateTime(x.asked_at)}>{ago(x.asked_at)}</time></p>
									{#if waiting(x)}
										<div class="big-acts">
											<Button size="sm" variant="primary" icon="check" type="button" data-testid="ask-approve" onclick={() => ctx.approve([x])}>Approve</Button>
											<Button size="sm" icon="x" type="button" data-testid="ask-decline" onclick={() => ctx.turnDown([x])}>Turn down</Button>
										</div>
									{:else if ctx.isPending(x.id)}
										<span class="sending-word">Sending in a moment…</span>
									{:else if x.state !== 'asked'}
										<div><StatusPill kind={ctx.pill(x).kind} label={ctx.pill(x).label} /></div>
									{/if}
								</div>
							</li>
						{/each}
					</ul>
				{:else}
					<ul class="rows" aria-label="Asks">
						{#each ctx.shown as x (x.id)}{@render row(x)}{/each}
					</ul>
				{/if}
			</div>

			{#if split}
				<aside class="pane" id={panelId} aria-label={look === 5 ? 'Asks' : 'The picked ask'} data-testid="ask-pane" hidden={!panel ? true : undefined} inert={!panel}>
					{#if look === 5}
						<ul class="strip" aria-label="Asks">
							{#each ctx.shown as x (x.id)}
								<li class="sitem" class:on={picked?.id === x.id} class:chosen={ctx.isChosen(x.id)} class:sending={ctx.isPending(x.id)} data-testid="ask">
									{@render chooseBox(x)}
									<button class="hit" type="button" data-row={x.id} aria-current={picked?.id === x.id ? 'true' : undefined} onclick={(e) => pick(x, e.currentTarget)}>
										{@render thumb(x)}
										<span class="txt"><span class="name">{x.name}</span><span class="sline">{ctx.who(x).name} · {ago(x.asked_at)}</span></span>
									</button>
								</li>
							{/each}
						</ul>
					{:else if picked}
						{#key picked.id}<div class="pin"><AskPanel {ctx} a={picked} variant={look === 4 ? 'slim' : 'full'} headingId="inbox-pane-title" /></div>{/key}
					{/if}
				</aside>
			{/if}
		</div>
	{/if}
</div>

{#if fullOpen && fullAsk}
	<div class="full" role="dialog" aria-modal="true" aria-label={fullAsk.name} data-testid="ask-full">
		<div class="full-bar"><button class="back" type="button" onclick={() => void closeFull()}><Icon name="back" size={20} />Asks</button></div>
		{#key fullAsk.id}<AskPanel {ctx} a={fullAsk} headingId="inbox-full-title" />{/key}
	</div>
{/if}

<style>
	.inbox { --thumb: 44px; min-width: 0; }
	.size-s { --thumb: 32px; }
	.size-l { --thumb: 60px; }
	.look-2.size-s { --thumb: 26px; }
	.look-2.size-m { --thumb: 34px; }
	.look-2.size-l { --thumb: 46px; }
	.look-4 { --pw: 320px; }
	.look-4.size-s { --thumb: 64px; }
	.look-4.size-m { --thumb: 88px; }
	.look-4.size-l { --thumb: 120px; }
	.look-5.size-s { --thumb: 36px; --pw: 220px; }
	.look-5.size-m { --thumb: 48px; --pw: 260px; }
	.look-5.size-l { --thumb: 64px; --pw: 300px; }
	.look-1, .look-3 { --pw: 400px; }
	.look-2 { --pw: 380px; }

	.bar { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 16px; margin-bottom: 12px; }
	.chips { display: flex; flex-wrap: wrap; gap: 8px; }
	.cw { display: inline-flex; transition: opacity var(--dur-fast) var(--ease-out); }
	.cw.zero { opacity: 0.5; }
	.cw.zero:hover, .cw.zero:focus-within { opacity: 0.8; }
	.tools { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-left: auto; }

	.lhead { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; min-height: 44px; margin-bottom: 10px; padding: 4px 12px; border-radius: var(--r-pill); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); transition: background var(--dur-fast) var(--ease-out); }
	.lhead.on { background: var(--surface-2); }
	.all { display: inline-flex; align-items: center; gap: 8px; margin-right: auto; font: var(--t-subhead); color: var(--text-muted); }
	.count { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.tick { width: 18px; height: 18px; flex: none; accent-color: var(--accent-fill); }

	.split { display: grid; grid-template-columns: minmax(0, 1fr) 0px; gap: 0; align-items: start; transition: grid-template-columns var(--dur-base) var(--ease-out), gap var(--dur-base) var(--ease-out); }
	.split.open { grid-template-columns: minmax(0, 1fr) var(--pw); gap: 16px; }
	.list-col { min-width: 0; }
	.pane { position: sticky; top: calc(var(--topbar) + 12px); min-width: 0; max-height: calc(100dvh - var(--topbar) - 32px); overflow-y: auto; overscroll-behavior: contain; padding: 16px; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); transition: opacity var(--dur-base) var(--ease-out), translate var(--dur-base) var(--ease-out), visibility 0s linear 0s; }
	:global(html.bar-hidden) .pane { top: 12px; max-height: calc(100dvh - 32px); }
	.pane[hidden] { display: block; visibility: hidden; opacity: 0; translate: 24px 0; padding-inline: 0; overflow: hidden; transition: opacity var(--dur-fast) var(--ease-in), translate var(--dur-fast) var(--ease-in), padding var(--dur-base) var(--ease-out), visibility 0s linear var(--dur-fast); }
	.look-5 .pane { padding: 6px; }
	.pin { animation: pane-in var(--dur-base) var(--ease-out) both; }
	@keyframes pane-in { from { opacity: 0; translate: 0 6px; } }

	/* Rows (1, 3) */
	.rows { margin: 0; padding: 0; list-style: none; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); overflow: hidden; }
	.row { position: relative; display: flex; align-items: center; gap: 10px; padding: 0 12px; transition: background var(--dur-fast) var(--ease-out), opacity var(--dur-fast) var(--ease-out); }
	.row + .row { border-top: 1px solid var(--line); }
	.row:hover { background: var(--surface-2); }
	.row.on { background: var(--accent-soft); }
	.row.on::before { content: ''; position: absolute; left: 0; top: 8px; bottom: 8px; width: 3px; border-radius: 0 3px 3px 0; background: var(--accent); }
	.row.chosen, .card.chosen, .sitem.chosen, tr.chosen { background: color-mix(in srgb, var(--accent) 10%, transparent); }
	.sending { opacity: 0.6; }
	.hit { flex: 1; min-width: 0; display: flex; align-items: center; gap: 12px; padding: 10px 0; border: 0; background: none; color: inherit; text-align: left; cursor: pointer; border-radius: 8px; }
	.hit:focus-visible, .cell-hit:focus-visible, .name-btn:focus-visible, .big-art:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.thumb { display: block; width: var(--thumb); flex: none; border-radius: 6px; overflow: hidden; transition: width var(--dur-base) var(--ease-out); }
	.thumb.square { --poster-ratio: 1; }
	.txt { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 3px; }
	.r1 { display: flex; align-items: center; gap: 8px; min-width: 0; }
	.r2 { display: flex; align-items: center; gap: 6px; min-width: 0; color: var(--text-muted); font: var(--t-footnote); }
	.tag { flex: none; padding: 1px 7px; border-radius: var(--r-sm); background: var(--surface-2); color: var(--text-muted); font: var(--t-caption); font-weight: 600; white-space: nowrap; }
	.row.on .tag { background: var(--surface-1); }
	.name { flex: 1; min-width: 0; font: var(--t-subhead); font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.age { flex: none; color: var(--text-muted); font: var(--t-caption); }
	.sline { min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.soon { flex: none; padding: 0 6px; border-radius: var(--r-sm); background: color-mix(in srgb, var(--wait) 14%, transparent); color: var(--wait-text); font: var(--t-caption); font-weight: 600; white-space: nowrap; }
	.dot { flex: none; }
	.who { display: inline-flex; align-items: center; gap: 5px; min-width: 0; }
	.wn { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.quick { display: inline-flex; gap: 6px; flex: none; }
	.sending-word { flex: none; color: var(--text-muted); font: var(--t-footnote); }
	.size-s .row .r2 { display: none; }
	.size-s .row .txt { flex-direction: row; }

	/* By person (3) */
	.groups { display: flex; flex-direction: column; gap: 16px; }
	.gh { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin: 0 4px 8px; }
	.gh h2 { font: var(--t-headline); }
	.gn { margin-right: auto; color: var(--text-muted); font: var(--t-footnote); }

	/* Table (2) */
	.tbl-wrap { border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); overflow: hidden; }
	.tbl { width: 100%; border-collapse: collapse; table-layout: fixed; font: var(--t-footnote); }
	.tbl th { height: 36px; padding: 0 8px; border-bottom: 1px solid var(--line); color: var(--text-muted); font: var(--t-caption); font-weight: 600; text-align: left; text-transform: none; }
	.tbl td { height: calc(var(--thumb) * 1.5 + 12px); padding: 0 8px; border-top: 1px solid var(--line); vertical-align: middle; overflow: hidden; }
	.tbl tbody tr:first-child td { border-top: 0; }
	.tbl tbody tr { transition: background var(--dur-fast) var(--ease-out), opacity var(--dur-fast) var(--ease-out); }
	.tbl tbody tr:hover { background: var(--surface-2); }
	.tbl tr.on { background: var(--accent-soft); }
	.c-tick { width: 36px; padding-left: 12px !important; }
	.c-art { width: calc(var(--thumb) + 16px); }
	.c-title { width: 26%; }
	.c-for { width: 26%; }
	.c-rule { width: 15%; }
	.c-who { width: 13%; }
	.c-when { width: 9%; color: var(--text-muted); white-space: nowrap; }
	.c-act { width: 84px; text-align: right; }
	.tbl td.c-for { color: var(--text-muted); }
	.tbl td.c-for, .tbl td.c-rule { white-space: nowrap; }
	.tbl td.c-for .soon { margin-left: 6px; }
	.tbl td.c-for .sline, .tbl td.c-rule .sline { display: inline-block; max-width: 100%; vertical-align: middle; }
	.cell-hit { display: flex; align-items: center; gap: 8px; width: 100%; min-height: 36px; padding: 0; border: 0; background: none; color: inherit; text-align: left; cursor: pointer; border-radius: 6px; }
	.split.open .c-rule, .split.open .c-when { display: none; }
	@media (max-width: 1279px) { .c-rule { display: none; } }

	/* Posters (4) */
	.big { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: 10px; }
	.card { position: relative; display: flex; align-items: flex-start; gap: 14px; padding: 12px; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); transition: background var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out), opacity var(--dur-fast) var(--ease-out); }
	.card.on { box-shadow: 0 0 0 2px var(--accent); }
	.card .tick { margin-top: 2px; }
	.big-art { flex: none; padding: 0; border: 0; background: none; cursor: pointer; border-radius: 8px; }
	.big-art .thumb { border-radius: 8px; }
	.big-body { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 8px; }
	.bname { min-width: 0; font: var(--t-headline); }
	.name-btn { max-width: 100%; padding: 0; border: 0; background: none; color: inherit; font: inherit; text-align: left; cursor: pointer; overflow-wrap: anywhere; border-radius: 4px; }
	.name-btn:hover { text-decoration: underline; text-underline-offset: 3px; }
	.meta { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 6px; color: var(--text-muted); font: var(--t-footnote); }
	.rule { white-space: nowrap; }
	.big-acts { display: flex; flex-wrap: wrap; gap: 8px; }

	/* Stage (5) */
	.stage-wrap { animation: pane-in var(--dur-base) var(--ease-out) both; }
	.strip { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: 2px; }
	.sitem { position: relative; display: flex; align-items: center; gap: 8px; padding: 0 8px; border-radius: 10px; transition: background var(--dur-fast) var(--ease-out), opacity var(--dur-fast) var(--ease-out); }
	.sitem:hover { background: var(--surface-2); }
	.sitem.on { background: var(--accent-soft); }
	.sitem .hit { padding: 6px 0; gap: 10px; }
	.sitem .txt { gap: 1px; }
	.sitem .sline { color: var(--text-muted); font: var(--t-caption); }

	.calm { display: flex; flex-direction: column; align-items: center; gap: 6px; padding: 56px 20px; text-align: center; color: var(--text-muted); border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.calm h2 { font: var(--t-headline); color: var(--text); }
	.muted { color: var(--text-muted); }
	.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }

	/* Full screen ask (below 1024 px, and options in one-by-one review). */
	.full { position: fixed; inset: 0; z-index: 60; overflow-y: auto; overscroll-behavior: contain; padding: 0 16px calc(var(--tabbar, 0px) + 24px + env(safe-area-inset-bottom, 0px)); background: var(--bg); animation: slide-in var(--dur) var(--ease-out) both; }
	.full-bar { position: sticky; top: 0; z-index: 2; display: flex; margin: 0 -16px 12px; padding: calc(6px + env(safe-area-inset-top, 0px)) 8px 6px; background: color-mix(in srgb, var(--bg) 82%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); border-bottom: 1px solid var(--line); }
	.back { min-height: 44px; display: inline-flex; align-items: center; gap: 4px; padding: 0 8px; border: 0; background: none; border-radius: 10px; color: var(--accent-text); font: var(--t-headline); cursor: pointer; }
	.back:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (min-width: 768px) { .full :global(.panel) { max-width: 720px; margin: 0 auto; } }
	@keyframes slide-in { from { transform: translateX(24px); opacity: 0; } }

	@media (max-width: 767px) {
		/* The five tabs share one row, the word above its count (as on Downloads). */
		.chips { width: 100%; display: grid; grid-template-columns: repeat(var(--tabs, 5), minmax(0, 1fr)); gap: 4px; }
		.chips .cw, .chips .cw :global(.ui-chip) { width: 100%; min-width: 0; }
		.chips .cw :global(.ui-chip), .chips .cw :global(.main) { height: 100%; }
		.chips .cw :global(.main) { width: 100%; flex-direction: column; justify-content: center; gap: 1px; min-height: 48px; padding: 4px 2px; font-size: 12.5px; line-height: 1.15; text-align: center; white-space: normal; overflow-wrap: anywhere; }
		.tools { width: 100%; margin-left: 0; }
		.tools > :global(*) { flex: 1 1 auto; }
		.lhead { border-radius: var(--r-card); }
		.row { padding: 0 10px; }
		.hit { min-height: 60px; }
		.r2 .dot, .r2 .who { display: none; }
		.card { padding: 10px; gap: 10px; }
		.look-4.size-l { --thumb: 96px; }
		.look-4.size-m { --thumb: 72px; }
	}
	@media (prefers-reduced-motion: reduce) {
		.split, .pane, .pane[hidden], .thumb, .row, .card, .sitem, .cw, .lhead { transition: none; }
		.pin, .stage-wrap, .full { animation: none; }
	}
</style>
