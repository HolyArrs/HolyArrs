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
		/** Go back to Try it with the same reads, to choose. */
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

<SettingsCard id="move-step" title="Step 5 of 5 · Done">
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
	<SettingsCard id="move-todo" title="To do now">
		{#if plainNext}
			{#each rep.next as n (n)}
				<div class="srow"><p class="txt">{n}</p></div>
			{/each}
		{:else}
			{#if (counts.not_found ?? 0) > 0}
				<div class="srow"><p class="txt">Some folders were not found here. Point them to a Library folder and move again; nothing is added twice.</p></div>
			{/if}
			{#if needs.length > 0}
				<div class="srow">
					<div class="txt"><span class="label">Type {plural(needs.length, 'key or password', 'keys and passwords')}</span><span class="hint">Sources, Downloaders, Alerts and subtitle sites wait for them.</span></div>
					<div class="ctl"><Button onclick={() => onshow('Keys to type again', needs, (n) => n)} aria-label="Show Keys to type again">Show</Button></div>
				</div>
			{/if}
			{#if links}
				<div class="srow">
					<div class="txt"><span class="label">Check Sources and Downloaders, then resume them</span><span class="hint">Turn on May reach the local network only for one on your home network.</span></div>
					<div class="ctl"><Button href={to('/settings/sources')}>Open Sources</Button></div>
				</div>
			{/if}
			{#if lists}
				<div class="srow">
					<div class="txt"><span class="label">Turn on the Outside lists</span><span class="hint">They came over paused.</span></div>
					<div class="ctl"><Button href={to('/settings/outside-lists')}>Open Outside lists</Button></div>
				</div>
			{/if}
			{#if newPeople > 0}
				<div class="srow">
					<div class="txt"><span class="label">Invite {plural(newPeople, 'person', 'people')}</span><span class="hint">They came over turned off.</span></div>
					<div class="ctl"><Button href={to('/settings/users')}>Open Users and roles</Button></div>
				</div>
			{/if}
			{#if openChoices > 0}
				<div class="srow">
					<div class="txt"><span class="label">Choose for {plural(openChoices, 'person', 'people')}</span><span class="hint">Until you choose, they stay behind.</span></div>
					<div class="ctl"><Button onclick={onchoose}>Choose</Button></div>
				</div>
			{/if}
			{#if (counts.failed ?? 0) > 0}
				<div class="srow"><p class="txt">{plural(counts.failed ?? 0, 'item')} failed. The reason is on its line below.</p></div>
			{/if}
			{#if rep.not_moved.length > 0}
				<div class="srow">
					<div class="txt"><span class="label">Look over {plural(rep.not_moved.length, 'setting')} that weren't moved</span></div>
					<div class="ctl">
						<Button onclick={() => onshow('Not moved', rep?.not_moved ?? [], (n, i) => ({ key: n.from + i, name: n.what, hint: n.why }))} aria-label="Show Not moved">Show</Button>
					</div>
				</div>
			{/if}
			<div class="srow"><p class="txt hint">Keep the old apps until HolyArrs runs the way you want.</p></div>
		{/if}
	</SettingsCard>
	{#if groups.length > 0}
		<SettingsCard id="move-results" title="Results by group">
			<ResultGroups report={rep} {groups} after {onshow} />
		</SettingsCard>
	{/if}
{/if}

<style>
	.cap { grid-column: 1 / -1; min-width: 0; }
	.when { grid-column: 1 / -1; padding-top: 6px; }
	p.txt { margin: 0; grid-column: 1 / -1; font: var(--t-callout); }
	p.txt.hint { color: var(--text-muted); }
</style>
