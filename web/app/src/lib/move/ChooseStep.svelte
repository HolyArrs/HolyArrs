<script lang="ts">
	import SettingsCard from '../ui/SettingsCard.svelte';
	import Button from '../ui/Button.svelte';
	import IconButton from '../ui/IconButton.svelte';
	import EmptyState from '../ui/EmptyState.svelte';
	import type { ShowList } from './ItemsSheet.svelte';
	import { appName, folderProblem, groupLabels, OTHER, plural, type PathRule, type PersonChoice, type Plan, type PlanGroup, type PlanItem } from '../move.ts';

	// Step 2: where the old folders go, which groups come over, and who the
	// old owner and admins are here. Every change plans again; the old rows
	// stay (dimmed) while it does.
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

	// Old folders with no Library folder of that path here.
	const kindOf = $derived(new Map((plan?.groups.find((g) => g.name === 'folders')?.items ?? []).map((it) => [it.folder?.old_path ?? '', it.folder?.kind ?? ''])));
	const oldRows = $derived((plan?.old_paths ?? []).filter((p) => !folders.some((f) => f.path === p)));
	const shown = $derived((plan?.groups ?? []).filter((g) => g.items.length > 0 || skip.includes(g.name)));
	const emptyGroups = $derived((plan?.groups ?? []).filter((g) => g.items.length === 0 && !skip.includes(g.name) && g.name !== 'other'));
	const peopleItems = $derived(plan?.groups.find((g) => g.name === 'people')?.items ?? []);
	const asked = $derived(peopleItems.filter((it) => it.person?.was_owner || it.person?.was_admin));
	const members = $derived(peopleItems.filter((it) => !(it.person?.was_owner || it.person?.was_admin)));

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

	function groupHint(g: PlanGroup): string {
		if (skip.includes(g.name)) return 'Left out';
		const n = g.items.length;
		const needs = g.items.filter((it) => it.needs);
		if (needs.length === n && n > 0 && needs.every((it) => it.needs === needs[0].needs)) return `${n} · ${n === 1 ? 'needs' : 'each needs'} ${needs[0].needs}`;
		if (needs.length > 0) return `${n} · ${needs.length} need your input`;
		const from = apps(g.items);
		return from ? `${n} · from ${from}` : `${n}`;
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

	// A path rule with a problem inside a closed Advanced is opened, so its error shows.
	$effect(() => {
		if (!extraErrors.some((e) => e !== '')) return;
		const d = document.querySelector<HTMLDetailsElement>('#move-folders details');
		if (d && !d.open) d.open = true;
	});

	function clipMiddle(p: string, max = 42): string {
		if (p.length <= max) return p;
		const half = Math.floor((max - 1) / 2);
		return p.slice(0, half) + '…' + p.slice(p.length - half);
	}
</script>

{#snippet ruleRows()}
	{#each extra as r, i (i)}
		{@const err = extraErrors[i] ?? ''}
		<div class="srow side rule">
			<div class="txt"><span class="label">Path rule {i + 1}</span></div>
			<div class="ctl">
				<label class="sr-only" for="move-rule-from-{i}">Old app's path</label>
				<input id="move-rule-from-{i}" class="input mono half" value={r.from} placeholder="/movies" aria-invalid={err ? 'true' : undefined} aria-describedby={err ? `move-rule-${i}-err` : undefined} oninput={(e) => onextra(extra.map((x, j) => (j === i ? { ...x, from: e.currentTarget.value } : x)))} onblur={oncommit} onkeydown={keyDone} />
				<span aria-hidden="true">→</span>
				<label class="sr-only" for="move-rule-to-{i}">Path here</label>
				<input id="move-rule-to-{i}" class="input mono half" value={r.to} placeholder="/data/movies" aria-invalid={err ? 'true' : undefined} aria-describedby={err ? `move-rule-${i}-err` : undefined} oninput={(e) => onextra(extra.map((x, j) => (j === i ? { ...x, to: e.currentTarget.value } : x)))} onblur={oncommit} onkeydown={keyDone} />
				<IconButton icon="x" label="Remove path rule {i + 1}" onclick={() => (onextra(extra.filter((_, j) => j !== i)), oncommit())} />
			</div>
			{#if err}<div class="below"><p class="form-error" id="move-rule-{i}-err">{err}</p></div>{/if}
		</div>
	{/each}
{/snippet}

<div class="choose" class:dim={planning && !!plan} aria-busy={planning}>
	<SettingsCard id="move-step" title="Step 2 of 5 · What to move" description="Pick what comes over. Every change updates the list.">
		{#if !plan}
			<p class="hint">Updating…</p>
		{:else if shown.length === 0}
			<EmptyState compact icon="check" title="Nothing new to move" text="Everything in these reads is already here or has no match." />
		{:else}
			{#each shown as g (g.name)}
				<div class="srow" data-testid="group-row">
					<div class="txt">
						<label for="move-group-{g.name}">{g.label}</label>
						<span class="hint">{groupHint(g)}</span>
					</div>
					<div class="ctl">
						<Button size="md" onclick={() => onshow(g.label, g.items, itemRow)} disabled={g.items.length === 0} aria-label="Show {g.label}">Show</Button>
						<label class="switch">
							<input id="move-group-{g.name}" type="checkbox" checked={!skip.includes(g.name)} onchange={(e) => ongroup(g.name, e.currentTarget.checked)} aria-label="Move {g.label}" />
							<span class="sw" aria-hidden="true"></span>
						</label>
					</div>
				</div>
			{/each}
			{#if plan.not_moved.length > 0}
				<div class="srow" data-testid="not-moved-row">
					<div class="txt"><span class="label">Not moved</span><span class="hint">{plural(plan.not_moved.length, 'setting')} with no match here</span></div>
					<div class="ctl">
						<Button onclick={() => onshow('Not moved', plan?.not_moved ?? [], (n, i) => ({ key: n.from + i, name: n.what, hint: `${groupLabels[n.group] ?? 'Other settings'} · ${n.why}` }))}>Show</Button>
						<span class="switch-gap" aria-hidden="true"></span>
					</div>
				</div>
			{/if}
			{#if emptyGroups.length > 0}
				<p class="hint empty">Nothing to move for: {emptyGroups.map((g) => g.label).join(', ')}</p>
			{/if}
		{/if}
	</SettingsCard>

	{#if plan && oldRows.length > 0}
		<SettingsCard id="move-folders" title="Folders" description="Point each old folder to a Library folder here. Titles in a folder you leave unset stay behind." advancedId="move-path-rules">
			{#each oldRows as p, n (p)}
				{@const pick = picks[p] ?? ''}
				{@const err = folderProblem(p, pick, others[p])}
				<div class="srow side" data-testid="folder-row">
					<div class="txt">
						<label for="move-folder-{n}" class="mono" title={p}><span aria-hidden="true">{clipMiddle(p)}</span><span class="sr-only">{p}</span></label>
						<span class="hint">{pick && pick === suggested[p] ? 'Suggested' : pick ? 'Its titles come over here.' : 'Unset: if this path is not on this server, its titles stay behind.'}</span>
					</div>
					<div class="ctl">
						<select id="move-folder-{n}" class="input" value={pick} aria-invalid={err ? 'true' : undefined} aria-describedby={err ? `move-folder-${n}-err` : undefined} onchange={(e) => onfolder(p, e.currentTarget.value)}>
							<option value="">Leave unset</option>
							{#each choicesFor(p) as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
							<option value={OTHER}>Other path…</option>
						</select>
					</div>
					{#if pick === OTHER || err}
						<div class="below">
							{#if pick === OTHER}
								<label class="sr-only" for="move-folder-{n}-other">Path here for {p}</label>
								<input
									id="move-folder-{n}-other"
									class="input mono wide"
									value={others[p] ?? ''}
									placeholder="/data/movies"
									aria-invalid={err ? 'true' : undefined}
									aria-describedby={err ? `move-folder-${n}-err` : undefined}
									oninput={(e) => onother(p, e.currentTarget.value)}
									onblur={oncommit}
									onkeydown={keyDone}
								/>
							{/if}
							{#if err}<p class="form-error" id="move-folder-{n}-err">{err}</p>{/if}
						</div>
					{/if}
				</div>
			{/each}
			{#snippet advanced()}
				<p class="hint">Other path rules: when an old app saw a folder under another path (another container), say both paths.</p>
				{@render ruleRows()}
				<div><Button icon="plus" variant="plain" onclick={() => onextra([...extra, { from: '', to: '' }])}>Add a path rule</Button></div>
			{/snippet}
		</SettingsCard>
	{:else if plan && (extra.length > 0 || plan.not_moved.some((n) => n.group === 'titles'))}
		<!-- Titles that found no place, or rules typed earlier: path rules stay at hand. -->
		<SettingsCard id="move-folders" title="Path rules" description="When an old app saw a folder under another path, say both paths.">
			{@render ruleRows()}
			<div><Button icon="plus" variant="plain" onclick={() => onextra([...extra, { from: '', to: '' }])}>Add a path rule</Button></div>
		</SettingsCard>
	{/if}

	{#if plan && peopleItems.length > 0 && !skip.includes('people')}
		<SettingsCard id="move-people" title="People" description="People come over turned off until you invite them. Their passwords stay behind.">
			{#each asked as it, n (it.key)}
				{@const owner = !!it.person?.was_owner}
				<div class="srow" data-testid="person-row">
					<div class="txt"><span class="label">{it.name}</span><span class="hint">{personHint(it)}</span></div>
					<div class="ctl">
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
					</div>
				</div>
			{/each}
			{#if members.length > 0}
				<div class="srow">
					<div class="txt"><span class="label">{members.length} {asked.length > 0 ? 'more ' : ''}{members.length === 1 ? 'person comes' : 'people come'} over as {members.length === 1 ? 'a member' : 'members'}</span></div>
					<div class="ctl"><Button onclick={() => onshow('People', members, itemRow)}>Show</Button><span class="switch-gap" aria-hidden="true"></span></div>
				</div>
			{/if}
		</SettingsCard>
	{/if}
</div>

<style>
	.choose { display: contents; }
	.dim :global(.ui-settings .body) { opacity: 0.6; transition: opacity var(--dur-fast) var(--ease-out); }
	.empty { padding: 8px 0 4px; }
	/* Rows without a switch keep the same right edge as rows with one. */
	.switch-gap { display: inline-block; width: 40px; }
	.sw-word { color: var(--text-muted); font: var(--t-footnote); white-space: nowrap; }
	select.input { width: 260px; height: 32px; }
	.wide { width: 100%; max-width: 100%; box-sizing: border-box; }
	/* Beats the settings rule for inputs in a row (.settings-content .srow > .ctl > .input). */
	.rule .ctl > .input.half { width: 160px; height: 32px; }
	.below .form-error { margin: 6px 0 0; }
	@media (max-width: 600px) {
		.side { grid-template-columns: minmax(0, 1fr); }
		.side .ctl { justify-self: stretch; justify-content: flex-start; flex-wrap: wrap; }
		.side select.input { width: 100%; }
		.rule .ctl { flex-wrap: nowrap; }
		.rule .ctl > .input.half { flex: 1 1 0; width: auto; min-width: 0; }
	}
	@media (max-width: 767px) and (pointer: coarse) {
		select.input, .rule .ctl > .input.half { height: 44px; }
		.switch-gap { width: 44px; }
	}
</style>
