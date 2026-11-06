<script lang="ts" module>
	// "Change what moves" stays open for the visit once someone opens it.
	let keepOpen = false;
</script>

<script lang="ts">
	import SettingsCard from '../ui/SettingsCard.svelte';
	import Button from '../ui/Button.svelte';
	import IconButton from '../ui/IconButton.svelte';
	import EmptyState from '../ui/EmptyState.svelte';
	import Icon from '../Icon.svelte';
	import type { ShowList } from './ItemsSheet.svelte';
	import { appName, folderProblem, groupLabels, OTHER, plural, type PathRule, type PersonChoice, type Plan, type PlanGroup, type PlanItem } from '../move.ts';

	// Step 2: a summary of what moves, then only what needs the person; the
	// full list (groups, folders, path rules) sits behind "Change what moves".
	// Every change plans again; the old rows stay (dimmed) while it does.
	let {
		plan,
		planning = false,
		folders,
		picks,
		others,
		extra,
		extraErrors,
		suggested,
		skip,
		people,
		onfolder,
		onother,
		onextra,
		ongroup,
		onperson,
		oncommit,
		onshow
	}: {
		plan: Plan | null;
		planning?: boolean;
		/** Library folders here. */
		folders: { path: string; kind: string; label: string }[];
		/** Old path → picked Library folder path, OTHER, or '' (unset). */
		picks: Record<string, string>;
		/** Old path → the typed path for OTHER. */
		others: Record<string, string>;
		extra: PathRule[];
		extraErrors: string[];
		/** Old path → the suggested Library folder. */
		suggested: Record<string, string>;
		skip: string[];
		people: Record<string, PersonChoice>;
		onfolder: (from: string, pick: string) => void;
		onother: (from: string, to: string) => void;
		onextra: (rules: PathRule[]) => void;
		ongroup: (name: string, on: boolean) => void;
		onperson: (key: string, c: PersonChoice | null) => void;
		/** A typed path is done (blur or Enter): plan again now. */
		oncommit: () => void;
		onshow: ShowList;
	} = $props();

	let open = $state(keepOpen);
	let advOpen = $state(false);
	let peopleOpen = $state(false);
	function toggle() {
		open = !open;
		keepOpen = open;
	}

	// Old folders with no Library folder of that path here.
	const kindOf = $derived(new Map((plan?.groups.find((g) => g.name === 'folders')?.items ?? []).map((it) => [it.folder?.old_path ?? '', it.folder?.kind ?? ''])));
	const oldRows = $derived((plan?.old_paths ?? []).filter((p) => !folders.some((f) => f.path === p)));
	const shown = $derived((plan?.groups ?? []).filter((g) => g.items.length > 0 || skip.includes(g.name)));
	const emptyGroups = $derived((plan?.groups ?? []).filter((g) => g.items.length === 0 && !skip.includes(g.name) && g.name !== 'other'));
	const on = $derived(shown.filter((g) => g.items.length > 0 && !skip.includes(g.name)));
	const peopleItems = $derived(plan?.groups.find((g) => g.name === 'people')?.items ?? []);
	const peopleOn = $derived(peopleItems.length > 0 && !skip.includes('people'));
	const asked = $derived(peopleItems.filter((it) => it.person?.was_owner || it.person?.was_admin));
	const pathRules = $derived(!!plan && oldRows.length === 0 && (extra.length > 0 || plan.not_moved.some((n) => n.group === 'titles')));

	// A folder with nothing picked needs the person; once shown there it stays
	// there (so a pick does not make the row jump away).
	let held = $state<string[]>([]);
	$effect(() => {
		if (planning) return;
		const add = oldRows.filter((p) => !picks[p] && !held.includes(p));
		if (add.length > 0) held = [...held, ...add];
	});
	const needFolders = $derived(oldRows.filter((p) => held.includes(p) || !picks[p]));

	// A problem in a closed list opens it, so its error shows.
	$effect(() => {
		if (oldRows.some((p) => !needFolders.includes(p) && folderProblem(p, picks[p] ?? '', others[p]) !== '')) open = true;
		if (extraErrors.some((e) => e !== '')) {
			open = true;
			advOpen = true;
		}
	});

	// The summary: the biggest things in bold, the rest named after "plus".
	const big = ['titles', 'wishlist', 'people'];
	const counted: Record<string, [string, string]> = { titles: ['title', 'titles'], wishlist: ['request', 'requests'], people: ['person', 'people'] };
	const named: Record<string, string> = { folders: 'folders', pick_rules: 'pick rules', never_add: 'never-add list', outside_lists: 'outside lists', naming: 'naming', subtitles: 'subtitles', sources: 'sources', downloaders: 'downloaders', alerts: 'alerts', other: 'other settings' };
	function andList(xs: string[]): string {
		return xs.length < 2 ? (xs[0] ?? '') : `${xs.slice(0, -1).join(', ')} and ${xs[xs.length - 1]}`;
	}
	function countOf(g: PlanGroup): string {
		const w = counted[g.name];
		// Groups without their own count word read as "Naming: 3", never "3 Naming".
		return w ? plural(g.items.length, w[0], w[1]) : `${g.label}: ${g.items.length}`;
	}
	const lead = $derived.by(() => {
		const b = big.map((n) => on.find((g) => g.name === n)).filter((g): g is PlanGroup => !!g);
		if (b.length > 0 || on.length === 0) return b;
		return [on.reduce((a, g) => (g.items.length > a.items.length ? g : a))];
	});
	const bigLine = $derived(lead.length ? lead.map(countOf).join(', ') : 'Nothing switched on');
	const restLine = $derived.by(() => {
		const rest = on.filter((g) => !lead.includes(g)).map((g) => named[g.name] ?? g.label);
		return rest.length ? `plus ${andList(rest)}` : '';
	});

	// Things to type after the move (keys, passwords, links), people aside.
	const keysLine = $derived.by(() => {
		const parts: string[] = [];
		const needs: string[] = [];
		for (const g of on) {
			if (g.name === 'people') continue;
			const n = g.items.filter((it) => it.needs);
			if (n.length === 0) continue;
			parts.push(plural(n.length, g.label.replace(/s$/, ''), g.label));
			for (const it of n) needs.push(it.needs ?? '');
		}
		if (parts.length === 0) return '';
		const keys = needs.every((x) => /key/.test(x));
		const many = needs.length > 1;
		return `${andList(parts)}: ${keys ? `type ${many ? 'their keys' : 'its key'}` : `fill ${many ? 'them' : 'it'} in`} after the move`;
	});
	const nothingNeeded = $derived(needFolders.length === 0 && !keysLine && !peopleOn);

	function choicesFor(p: string): { value: string; label: string }[] {
		const k = kindOf.get(p);
		const out = folders.filter((f) => !k || !f.kind || f.kind === k).map((f) => ({ value: f.path, label: f.path }));
		const s = suggested[p];
		if (s && !out.some((o) => o.value === s)) out.unshift({ value: s, label: s });
		return out;
	}

	function apps(items: PlanItem[]): string {
		const seen: string[] = [];
		for (const it of items) {
			const a = appName(it.from.split(' ')[0] ?? '');
			if (a && !seen.includes(a)) seen.push(a);
			if (seen.length > 3) break;
		}
		return seen.join(', ');
	}

	/** Where a group comes from, or what it needs (warn), or Left out. */
	function groupNote(g: PlanGroup): { text: string; warn: boolean } {
		if (skip.includes(g.name)) return { text: 'Left out', warn: false };
		const n = g.items.length;
		const needs = g.items.filter((it) => it.needs);
		if (needs.length === n && n > 0 && needs.every((it) => it.needs === needs[0].needs)) return { text: `${n === 1 ? 'needs' : 'each needs'} ${needs[0].needs}`, warn: true };
		if (needs.length > 0) return { text: `${needs.length} need your input`, warn: true };
		const from = apps(g.items);
		return { text: from ? `from ${from}` : '', warn: false };
	}

	function itemRow(it: PlanItem) {
		const hint = [it.why || it.detail, it.needs ? `Needs ${it.needs}` : ''].filter(Boolean).join(' · ');
		return { key: it.key, name: it.name, hint };
	}

	function personHint(it: PlanItem): string {
		const app = appName(it.person?.app ?? it.from.split(' ')[0] ?? '');
		if (it.person?.was_owner) return `Owner in ${app}. Their requests become yours.`;
		return `Admin in ${app}. Off: comes over as a member.`;
	}

	function keyDone(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			e.preventDefault();
			oncommit();
		}
	}

	function clipMiddle(p: string, max = 42): string {
		if (p.length <= max) return p;
		const half = Math.floor((max - 1) / 2);
		return p.slice(0, half) + '…' + p.slice(p.length - half);
	}
</script>

{#snippet folderRow(p: string, n: number, id: string, testid: string, note: boolean)}
	{@const pick = picks[p] ?? ''}
	{@const err = folderProblem(p, pick, others[p])}
	<li class="mrow wrap" data-testid={testid}>
		<div class="txt">
			<label for={id} class="mono path one" title={p}><span aria-hidden="true">{clipMiddle(p)}</span><span class="sr-only">{p}</span></label>
			{#if note && pick && pick === suggested[p]}<span class="sub">Suggested</span>{/if}
		</div>
		<span class="arrow" aria-hidden="true">→</span>
		<select {id} class="input pick" value={pick} aria-invalid={err ? 'true' : undefined} aria-describedby={err ? `${id}-err` : undefined} onchange={(e) => onfolder(p, e.currentTarget.value)}>
			<option value="">Leave unset</option>
			{#each choicesFor(p) as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
			<option value={OTHER}>Other path…</option>
		</select>
		{#if pick === OTHER || err}
			<div class="below">
				{#if pick === OTHER}
					<label class="sr-only" for="{id}-other">Path here for {p}</label>
					<input id="{id}-other" class="input mono wide" value={others[p] ?? ''} placeholder="/data/movies" aria-invalid={err ? 'true' : undefined} aria-describedby={err ? `${id}-err` : undefined} oninput={(e) => onother(p, e.currentTarget.value)} onblur={oncommit} onkeydown={keyDone} />
				{/if}
				{#if err}<p class="form-error" id="{id}-err">{err}</p>{/if}
			</div>
		{/if}
	</li>
{/snippet}

{#snippet ruleRows()}
	{#each extra as r, i (i)}
		{@const err = extraErrors[i] ?? ''}
		<li class="mrow wrap rule">
			<span class="sr-only">Path rule {i + 1}</span>
			<label class="sr-only" for="move-rule-from-{i}">Old app's path</label>
			<input id="move-rule-from-{i}" class="input mono half" value={r.from} placeholder="/movies" aria-invalid={err ? 'true' : undefined} aria-describedby={err ? `move-rule-${i}-err` : undefined} oninput={(e) => onextra(extra.map((x, j) => (j === i ? { ...x, from: e.currentTarget.value } : x)))} onblur={oncommit} onkeydown={keyDone} />
			<span class="arrow" aria-hidden="true">→</span>
			<label class="sr-only" for="move-rule-to-{i}">Path here</label>
			<input id="move-rule-to-{i}" class="input mono half" value={r.to} placeholder="/data/movies" aria-invalid={err ? 'true' : undefined} aria-describedby={err ? `move-rule-${i}-err` : undefined} oninput={(e) => onextra(extra.map((x, j) => (j === i ? { ...x, to: e.currentTarget.value } : x)))} onblur={oncommit} onkeydown={keyDone} />
			<IconButton icon="x" label="Remove path rule {i + 1}" onclick={() => (onextra(extra.filter((_, j) => j !== i)), oncommit())} />
			{#if err}<div class="below"><p class="form-error" id="move-rule-{i}-err">{err}</p></div>{/if}
		</li>
	{/each}
	<li class="mrow"><Button icon="plus" variant="plain" onclick={() => onextra([...extra, { from: '', to: '' }])}>Add a path rule</Button></li>
{/snippet}

<div class="choose" class:dim={planning && !!plan} aria-busy={planning}>
	<SettingsCard id="move-step" title="Step 2 of 5 · What to move">
		{#if !plan}
			<p class="hint">Updating…</p>
		{:else}
			{#if shown.length === 0}
				<EmptyState compact icon="check" title="Nothing new to move" text="Everything in these reads is already here or has no match." />
			{:else}
				<div class="box sum" data-testid="move-summary">
					<b class="big">{bigLine}</b>
					{#if restLine}<span class="sub">{restLine}</span>{/if}
				</div>
			{/if}

			{#if shown.length === 0 && needFolders.length === 0}
				<!-- Nothing to move: no "Needs you" either. -->
			{:else if nothingNeeded}
				<h3 class="head">Needs you</h3>
				<p class="sub none">Nothing needs you. Everything moves as shown.</p>
			{:else}
				<h3 class="head">Needs you</h3>
				{#if needFolders.length}<p class="sub">Titles in a folder you leave unset stay behind.</p>{/if}
				<ul class="box list" data-testid="needs-you">
					{#each needFolders as p, n (p)}
						{@render folderRow(p, n, `move-need-folder-${n}`, 'need-folder-row', false)}
					{/each}
					{#if keysLine}
						<li class="mrow"><span class="txt warn">{keysLine}</span></li>
					{/if}
					{#if peopleOn}
						<li class="mrow">
							<span class="txt">{peopleItems.length === 1 ? '1 person comes' : `${peopleItems.length} people come`} over turned off until you invite them</span>
							{#if asked.length > 3}
								<button type="button" class="quiet" aria-expanded={peopleOpen} onclick={() => (peopleOpen = !peopleOpen)}>Review people</button>
							{:else}
								<button type="button" class="quiet" onclick={() => onshow('People', peopleItems, itemRow)}>Show</button>
							{/if}
						</li>
						{#if asked.length <= 3 || peopleOpen}
							{#each asked as it, n (it.key)}
								{@const owner = !!it.person?.was_owner}
								<li class="mrow" data-testid="person-row">
									<div class="txt"><span class="name one">{it.name}</span><span class="sub one" title={personHint(it)}>{personHint(it)}</span></div>
									<span class="sw-word" aria-hidden="true">{owner ? 'This is me' : 'Keep as admin'}</span>
									<label class="switch">
										<input
											id="move-person-{n}"
											type="checkbox"
											checked={people[it.key] === (owner ? 'owner' : 'admin')}
											onchange={(e) => onperson(it.key, e.currentTarget.checked ? (owner ? 'owner' : 'admin') : null)}
											aria-label="{owner ? 'This is me' : 'Keep as admin'}: {it.name}"
										/>
										<span class="sw" aria-hidden="true"></span>
									</label>
								</li>
							{/each}
							{#if asked.length > 3}
								<li class="mrow"><span class="txt"></span><button type="button" class="quiet" onclick={() => onshow('People', peopleItems, itemRow)}>Show all</button></li>
							{/if}
						{/if}
					{/if}
				</ul>
			{/if}

			{#if shown.length > 0 || oldRows.length > 0 || pathRules}
				<button type="button" class="quiet more" aria-expanded={open} aria-controls="move-all" onclick={toggle}>
					<Icon name={open ? 'chevronDown' : 'chevronRight'} size={15} /><span>Change what moves</span>
				</button>
			{/if}
			{#if open}
				<div id="move-all" class="all">
					{#if shown.length > 0}
						<ul class="box list">
							{#each shown as g (g.name)}
								{@const note = groupNote(g)}
								<li class="mrow" data-testid="group-row">
									<span class="txt one" title="{g.label} {g.items.length}{note.text ? ` · ${note.text}` : ''}">
										<label class="name" for="move-group-{g.name}">{g.label}</label>
										<span class="count">{g.items.length}</span>{#if note.text}<span class="sub"> · </span><span class="sub" class:warn={note.warn}>{note.text}</span>{/if}
									</span>
									<button type="button" class="quiet" onclick={() => onshow(g.label, g.items, itemRow)} disabled={g.items.length === 0} aria-label="Show {g.label}">Show</button>
									<label class="switch">
										<input id="move-group-{g.name}" type="checkbox" checked={!skip.includes(g.name)} onchange={(e) => ongroup(g.name, e.currentTarget.checked)} aria-label="Move {g.label}" />
										<span class="sw" aria-hidden="true"></span>
									</label>
								</li>
							{/each}
						</ul>
					{/if}

					{#if oldRows.length > 0}
						<h3 class="head" id="move-folders">Folders</h3>
						<ul class="box list">
							{#each oldRows.filter((x) => !needFolders.includes(x)) as p, n (p)}
								{@render folderRow(p, n, `move-folder-${n}`, 'folder-row', true)}
							{/each}
						</ul>
						<details class="adv" bind:open={advOpen}>
							<summary><Icon name="chevronRight" size={15} /><span>Advanced</span></summary>
							<p class="sub">When an old app saw a folder under another path, say both paths.</p>
							<ul class="box list">{@render ruleRows()}</ul>
						</details>
					{:else if pathRules}
						<h3 class="head" id="move-folders">Path rules</h3>
						<p class="sub">When an old app saw a folder under another path, say both paths.</p>
						<ul class="box list">{@render ruleRows()}</ul>
					{/if}

					{#if plan.not_moved.length > 0}
						<ul class="box list tail">
							<li class="mrow" data-testid="not-moved-row">
								<span class="txt one"><span class="name">Not moved</span> <span class="sub">{plural(plan.not_moved.length, 'setting')} with no match here</span></span>
								<button type="button" class="quiet" onclick={() => onshow('Not moved', plan?.not_moved ?? [], (x, i) => ({ key: x.from + i, name: x.what, hint: `${groupLabels[x.group] ?? 'Other settings'} · ${x.why}` }))}>Show</button>
							</li>
						</ul>
					{/if}
					{#if emptyGroups.length > 0}
						<p class="sub none">Nothing to move for: {emptyGroups.map((g) => g.label).join(', ')}</p>
					{/if}
				</div>
			{/if}
		{/if}
	</SettingsCard>
</div>

<style>
	.choose { display: contents; }
	.dim :global(.ui-settings .body) { opacity: 0.6; transition: opacity var(--dur-fast) var(--ease-out); }

	/* One surface box per list, rows split by --line, like the read rows. */
	.box { margin: 0; border-radius: 16px; background: var(--surface-2); border: 1px solid var(--line); }
	.list { list-style: none; padding: 0 16px; }
	.sum { display: flex; flex-direction: column; gap: 2px; padding: 14px 16px; }
	.big { font: 600 17px/1.3 var(--font-ui); color: var(--text); }
	.head { margin: 18px 0 8px; font: var(--t-subhead); font-weight: 600; color: var(--text); }
	.sub { color: var(--text-muted); font: var(--t-footnote); font-weight: 400; }
	.warn, .sub.warn { color: var(--wait-text); }
	.none { margin: 0; }

	.mrow { display: flex; align-items: center; gap: 10px; min-height: 48px; padding: 6px 0; min-width: 0; }
	.mrow + .mrow { border-top: 1px solid var(--line); }
	.wrap { flex-wrap: wrap; }
	.txt { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; font: var(--t-callout); color: var(--text); }
	span.txt.one { display: block; }
	.one { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.name { font: 600 14px/1.3 var(--font-ui); color: var(--text); }
	.count { color: var(--text-muted); margin-left: 6px; }
	.path { font-size: 13px; color: var(--text); }
	.arrow { flex: none; color: var(--text-muted); }
	.sw-word { color: var(--text-muted); font: var(--t-footnote); white-space: nowrap; }
	.switch { flex: none; }

	select.pick { flex: none; width: 240px; min-height: 36px; height: 36px; }
	.half { flex: 1 1 0; min-width: 0; min-height: 36px; height: 36px; }
	.wide { width: 100%; max-width: 100%; box-sizing: border-box; min-height: 36px; }
	.below { flex-basis: 100%; padding-bottom: 6px; }
	.below .form-error { margin: 6px 0 0; }

	.quiet { flex: none; border: 0; padding: 4px 0; background: none; color: var(--text-muted); font: 500 13px/1.2 var(--font-ui); cursor: pointer; }
	.quiet:hover:not(:disabled) { color: var(--text); }
	.quiet:disabled { opacity: 0.5; cursor: default; }
	.quiet:focus-visible, .adv summary:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 6px; }
	.more { display: inline-flex; align-items: center; gap: 4px; margin-top: 14px; }
	.all { margin-top: 10px; }
	.tail { margin-top: 10px; }
	.all > .none { margin-top: 10px; }

	.adv { margin-top: 10px; }
	.adv summary { display: inline-flex; align-items: center; gap: 4px; list-style: none; cursor: pointer; color: var(--text-muted); font: 500 13px/1.2 var(--font-ui); padding: 4px 0; }
	.adv summary::-webkit-details-marker { display: none; }
	.adv summary :global(svg) { transition: transform var(--dur-fast) var(--ease-out); }
	.adv[open] summary :global(svg) { transform: rotate(90deg); }
	.adv > .sub { margin: 6px 0 8px; }

	/* Phones: the folder pick goes under its path; 44px to tap. */
	@media (max-width: 600px) {
		.wrap .txt { flex-basis: 100%; }
		.wrap .txt + .arrow { display: none; }
		select.pick { flex: 1 1 100%; width: auto; min-width: 0; }
	}
	@media (max-width: 600px), (pointer: coarse) {
		.quiet, .adv summary { min-height: 44px; padding: 0 8px; margin: 0 -8px; }
		.more { margin-left: -8px; }
		select.pick, .half, .wide { min-height: 44px; height: 44px; }
	}
</style>
