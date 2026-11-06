<script lang="ts">
	import SettingsCard from '../ui/SettingsCard.svelte';
	import Button from '../ui/Button.svelte';
	import StepCapsule from '../ui/StepCapsule.svelte';
	import ResultGroups from './ResultGroups.svelte';
	import type { ShowList } from './ItemsSheet.svelte';
	import { to } from '../paths.ts';
	import { dateTime } from '../format.ts';
	import { moveCounts, needsGroups, plural, running as isRunning, type Bucket, type Move } from '../move.ts';

	// Step 5: what the move did, what to do now, and the results per group.
	// Also what "Open" shows for an earlier move.
	let {
		move,
		groups,
		openChoices = 0,
		progress = 0,
		note = '',
		lost = false,
		onchoose,
		onshow
	}: {
		move: Move;
		groups: Bucket[];
		/** People who still need Same person or Leave out. */
		openChoices?: number;
		progress?: number;
		note?: string;
		lost?: boolean;
		/** Go back to Choose with the same reads, to choose. */
		onchoose: () => void;
		onshow: ShowList;
	} = $props();

	const rep = $derived(move.report);
	const counts = $derived(rep?.counts ?? {});
	const busy = $derived(isRunning(move));
	const undone = $derived(move.state === 'undone');
	const undoFailed = $derived(move.undo_state === 'failed' && !undone);
	const lines = $derived(rep?.lines ?? []);
	const made = (l: { result: string }) => l.result === 'create' || l.result === 'created';
	const links = $derived(lines.some((l) => (l.group === 'sources' || l.group === 'downloaders') && made(l)));
	const lists = $derived(lines.some((l) => l.group === 'outside_lists' && made(l)));
	const newPeople = $derived(lines.filter((l) => l.group === 'people' && made(l)).length);
	const needs = $derived(needsGroups(rep?.needs_input ?? []).flatMap((g) => g.items.map((t, i) => ({ key: `${g.label}-${i}`, name: t, hint: g.label }))));
	const detail = $derived(note.replace(/\s+(moved|undone)\.?$/, ''));
	// An undo report, or a move that never started, says its own few lines.
	const plainNext = $derived(!rep || !!rep.undo || rep.lines.length === 0);
</script>

<SettingsCard id="move-step" title="Step 4 of 4 · Done">
	<div class="srow">
		<div class="cap" data-testid="done-result">
			{#if busy && lost}
				<StepCapsule tone="soft" fill={progress} status="pending" word="Can't reach HolyArrs" detail="trying again" />
				<p class="sr-only" role="status">Can't reach HolyArrs. Trying again.</p>
			{:else if busy}
				<StepCapsule tone="go" live progress fill={progress} status="processing" word={move.undo_state ? 'Undoing' : 'Moving'} detail={detail || undefined} label={move.undo_state ? 'Undo progress' : 'Move progress'} testid="move-progress" />
			{:else if undoFailed}
				<StepCapsule tone="bad" fill={1} status="failed" word="Undo failed" detail={plural(counts.failed ?? 0, 'item')} />
			{:else if undone}
				<StepCapsule tone="ok" fill={1} status="ready" word="Undone" detail={moveCounts(move) || undefined} />
			{:else if move.state === 'failed'}
				<StepCapsule tone="bad" fill={1} status="failed" word="Stopped with errors" detail={moveCounts(move) || undefined} />
			{:else}
				<StepCapsule tone={(counts.failed ?? 0) > 0 ? 'bad' : 'ok'} fill={1} status={(counts.failed ?? 0) > 0 ? 'failed' : 'ready'} word="Moved" detail={moveCounts(move) || undefined} />
			{/if}
		</div>
		<span class="hint when">Move {move.id} · {dateTime(move.created_at)}</span>
	</div>
</SettingsCard>

{#if rep && !busy}
	<SettingsCard id="move-todo" title="Next">
		{#if plainNext}
			{#each rep.next as n (n)}
				<div class="srow"><p class="txt">{n}</p></div>
			{/each}
		{:else}
			<div class="cards">
				{#snippet card(title: string, sub: string, act: string, go: (() => void) | string)}
					<div class="box card1">
						<span class="txt"><b>{title}</b>{#if sub}<span class="sub">{sub}</span>{/if}</span>
						{#if !act}{:else if typeof go === 'string'}<a class="quiet" href={go}>{act}</a>{:else}<button type="button" class="quiet" onclick={go}>{act}</button>{/if}
					</div>
				{/snippet}
				{#if (counts.not_found ?? 0) > 0}
					{@render card(`${plural(counts.not_found ?? 0, 'title')} have no folder here`, 'Add their Library folder, then move again.', 'Folders', to('/settings/library-folders'))}
				{/if}
				{#if needs.length > 0}
					{@render card(`Add ${plural(needs.length, 'key', 'keys')}`, 'Sources, Downloaders and Alerts wait for them.', 'Show', () => onshow('Keys to add', needs, (n) => n))}
				{/if}
				{#if links}
					{@render card('Resume Sources and Downloaders', 'They arrived paused.', 'Open', to('/settings/sources'))}
				{/if}
				{#if lists}
					{@render card('Resume Outside lists', 'They arrived paused.', 'Open', to('/settings/outside-lists'))}
				{/if}
				{#if newPeople > 0}
					{@render card(`Invite ${plural(newPeople, 'person', 'people')}`, '', 'Open', to('/settings/users'))}
				{/if}
				{#if openChoices > 0}
					{@render card(`Choose for ${plural(openChoices, 'person', 'people')}`, 'They stay behind until you do.', 'Choose', onchoose)}
				{/if}
				{#if (counts.failed ?? 0) > 0}
					{@render card(`${plural(counts.failed ?? 0, 'item')} failed`, 'Reasons are in Results.', '', () => {})}
				{/if}
			</div>
			<p class="foot">Keep the old apps until you're happy.</p>
		{/if}
	</SettingsCard>
	{#if groups.length > 0}
		<SettingsCard id="move-results" title="Results">
			<ResultGroups report={rep} {groups} after {onshow} />
		</SettingsCard>
	{/if}
{/if}

<style>
	.cap { grid-column: 1 / -1; min-width: 0; }
	.when { grid-column: 1 / -1; padding-top: 6px; }
	p.txt { margin: 0; grid-column: 1 / -1; font: var(--t-callout); }
	p.txt.hint { color: var(--text-muted); }
	.cards { display: grid; gap: 10px; }
	.box { margin: 0; border-radius: 16px; background: var(--surface-2); border: 1px solid var(--line); }
	.card1 { display: flex; align-items: center; gap: 12px; padding: 14px 16px; }
	.txt { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; font: var(--t-callout); color: var(--text); }
	.sub { color: var(--text-muted); font: var(--t-footnote); }
	.quiet { flex: none; border: 0; padding: 4px 0; background: none; color: var(--text-muted); font: 500 13px/1.2 var(--font-ui); cursor: pointer; text-decoration: none; }
	.quiet:hover { color: var(--text); }
	.quiet:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 6px; }
	.foot { margin: 12px 2px 0; color: var(--text-muted); font: var(--t-footnote); }
	@media (pointer: coarse) { .quiet { min-height: 44px; padding: 0 8px; margin: 0 -8px; display: inline-flex; align-items: center; } }
</style>
