<script lang="ts">
	import SettingsCard from '../ui/SettingsCard.svelte';
	import Button from '../ui/Button.svelte';
	import Sheet from '../ui/Sheet.svelte';
	import SegmentedControl from '../ui/SegmentedControl.svelte';
	import StepCapsule from '../ui/StepCapsule.svelte';
	import ResultGroups from './ResultGroups.svelte';
	import type { ShowList } from './ItemsSheet.svelte';
	import { chooseHint, moveTotal, plural, stayTotal, type Bucket, type Line, type PersonChoice, type Report } from '../move.ts';

	// Step 3: a test run of the move. Nothing changes. People the server
	// cannot match on its own get Same person or Leave out here; a choice runs
	// the test again in place.
	let {
		dry,
		groups,
		choose,
		trying = false,
		stale = false,
		failed: runFailed = false,
		people,
		openChoices = 0,
		notice = '',
		onchoose,
		onretry,
		onshow
	}: {
		dry: Report | null;
		groups: Bucket[];
		choose: Line[];
		trying?: boolean;
		stale?: boolean;
		/** The last test run failed. */
		failed?: boolean;
		people: Record<string, PersonChoice>;
		/** People still waiting for a choice (they stay behind). */
		openChoices?: number;
		notice?: string;
		onchoose: (key: string, c: PersonChoice) => void;
		/** Run the test again after it failed. */
		onretry: () => void;
		onshow: ShowList;
	} = $props();

	let allChoices = $state(false);
	const counts = $derived(dry?.counts ?? {});
	const failed = $derived(counts.failed ?? 0);
	const total = $derived(moveTotal(counts));
	const behind = $derived(Math.max(0, stayTotal(counts) - openChoices));
	const options: { value: string; label: string }[] = [
		{ value: 'merge', label: 'Same person' },
		{ value: 'skip', label: 'Leave out' }
	];
</script>

{#snippet choiceRow(l: Line, n: number)}
	<div class="srow side" data-testid="choice-row">
		<div class="txt"><span class="label">{l.name}</span><span class="hint">{chooseHint(l.choose)}</span></div>
		<div class="ctl">
			<SegmentedControl label="Choice for {l.name}" testid="move-choice-{n}" {options} value={people[l.key] ?? ''} onchange={(v) => onchoose(l.key, v as PersonChoice)} />
		</div>
	</div>
{/snippet}

<SettingsCard id="move-step" title="Step 3 of 5 · Try it" description="A test run. Nothing changes yet.">
	{#if notice}<p class="notice warn" role="status">{notice}</p>{/if}
	<div class="srow">
		<div class="cap" data-testid="try-result" role="status">
			{#if runFailed && !trying}
				<StepCapsule tone="bad" fill={1} status="failed" word="The test run failed" />
			{:else if stale && !trying}
				<StepCapsule tone="wait" fill={1} status="pending" word="Changed since the test" detail="it runs again" />
			{:else if trying && !dry}
				<StepCapsule tone="go" live fill={0.4} status="processing" word="Trying the move…" />
			{:else if trying}
				<StepCapsule tone="go" live fill={0.6} status="processing" word="Updating…" />
			{:else if dry && failed > 0}
				<StepCapsule tone="bad" fill={1} status="failed" word="{plural(failed, 'item')} won't work" />
			{:else if dry}
				<StepCapsule tone="ok" fill={1} status="ready" word="Ready" detail={plural(total, 'item')} />
			{/if}
		</div>
		{#if runFailed && !trying}
			<div class="retry"><Button onclick={onretry}>Try again</Button></div>
		{/if}
	</div>
	{#if dry}
		<div class="srow tally" data-testid="try-tally">
			<p class="txt">
				<span>{plural(total, 'moves over', 'move over')}</span>{#if openChoices > 0}{' · '}<span>{plural(openChoices, 'needs you', 'need you')}</span>{/if}{#if behind > 0}{' · '}<span>{plural(behind, 'stays behind', 'stay behind')}</span>{/if}
			</p>
		</div>
	{/if}
</SettingsCard>

{#if dry && choose.length > 0}
	<SettingsCard id="move-choices" title="Needs a choice" description="Until you choose, they stay behind.">
		{#each choose.slice(0, 10) as l, n (l.key)}
			{@render choiceRow(l, n)}
		{/each}
		{#if choose.length > 10}
			<div class="srow">
				<div class="txt"></div>
				<div class="ctl"><Button onclick={() => (allChoices = true)}>Show all {choose.length}</Button></div>
			</div>
		{/if}
	</SettingsCard>
	<Sheet bind:open={allChoices} title="Needs a choice" size="lg" placement="auto">
		{#each choose as l, n (l.key)}
			{@render choiceRow(l, n + 1000)}
		{/each}
	</Sheet>
{/if}

{#if dry}
	<SettingsCard id="move-groups" title="By group">
		<ResultGroups report={dry} {groups} {onshow} />
	</SettingsCard>
{/if}

<style>
	.cap { grid-column: 1 / -1; min-width: 0; }
	.retry { grid-column: 1 / -1; justify-self: start; padding-top: 8px; }
	.tally .txt { margin: 0; display: block; font: var(--t-callout); color: var(--text); }
	.tally span { white-space: nowrap; }
	.notice { margin: 4px 0 8px; }
	@media (max-width: 600px) {
		.side { grid-template-columns: minmax(0, 1fr); }
		.side .ctl { justify-self: start; }
	}
</style>
