<script lang="ts">
	import { getContext as moveContext } from 'svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Button from '#lib/ui/Button.svelte';
	import StepCapsule from './StepCapsule.svelte';
	import ResultGroups from './ResultGroups.svelte';
	import type { ShowList } from './ItemsSheet.svelte';
	import { moveCounts, plural, running as isRunning, type Bucket, type Move } from './move.ts';
	import { to } from '../core/paths.ts';
	import { dateTime } from '../core/format.ts';

	const countSteps = moveContext<boolean | undefined>('move-hide-count') !== true;

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
		openChoices?: number;
		progress?: number;
		note?: string;
		lost?: boolean;
		onchoose: () => void;
		onshow: ShowList;
	} = $props();

	const rep = $derived(move.report);
	const counts = $derived(rep?.counts ?? {});
	const busy = $derived(isRunning(move));
	const undone = $derived(move.state === 'undone');
	const undoFailed = $derived(move.undo_state === 'failed' && !undone);
	const lines = $derived(rep?.lines ?? []);
	const detail = $derived(note.replace(/\s+(moved|undone)\.?$/, ''));
	const plainNext = $derived(!rep || !!rep.undo || rep.lines.length === 0);
</script>

<SettingsCard id="move-step" title={countSteps ? 'Step 4 of 4 · Done' : 'Done'}>
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
	{#if plainNext || (counts.not_found ?? 0) > 0 || openChoices > 0 || (counts.failed ?? 0) > 0}
	<SettingsCard id="move-todo" title="Needs you">
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
					{@render card(`${plural(counts.not_found ?? 0, 'title')} have no folder here`, 'Add their folder, then move again.', 'Folders', to('/settings/library-folders'))}
				{/if}
				{#if openChoices > 0}
					{@render card(`Choose for ${plural(openChoices, 'person', 'people')}`, 'They stay behind until you do.', 'Choose', onchoose)}
				{/if}
				{#if (counts.failed ?? 0) > 0}
					{@render card(`${plural(counts.failed ?? 0, 'item')} failed`, 'Reasons are in Results.', '', () => {})}
				{/if}
			</div>
		{/if}
	</SettingsCard>
	{/if}
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
	.cards { display: grid; gap: 10px; }
	.box { margin: 0; border-radius: 16px; background: var(--surface-2); border: 1px solid var(--line); }
	.card1 { display: flex; align-items: center; gap: 12px; padding: 14px 16px; }
	.txt { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; font: var(--t-callout); color: var(--text); }
	.sub { color: var(--text-muted); font: var(--t-footnote); }
	.quiet { flex: none; border: 0; padding: 4px 0; background: none; color: var(--text-muted); font: 500 13px/1.2 var(--font-ui); cursor: pointer; text-decoration: none; }
	.quiet:hover { color: var(--text); }
	.quiet:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 6px; }
	@media (pointer: coarse) { .quiet { min-height: 44px; padding: 0 8px; margin: 0 -8px; display: inline-flex; align-items: center; } }
</style>
