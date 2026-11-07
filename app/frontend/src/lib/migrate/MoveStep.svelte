<script lang="ts">
	import { getContext as moveContext } from 'svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Button from '#lib/ui/Button.svelte';
	import StepCapsule from './StepCapsule.svelte';
	import type { ShowList } from './ItemsSheet.svelte';
	import { needsGroups, plural, type Report } from './move.ts';
	import { to } from '../core/paths.ts';

	const countSteps = moveContext<boolean | undefined>('move-hide-count') !== true;

	let {
		dry,
		running = false,
		progress = 0,
		note = '',
		lost = false,
		safeMode = false,
		onshow
	}: {
		dry: Report | null;
		running?: boolean;
		progress?: number;
		note?: string;
		lost?: boolean;
		safeMode?: boolean;
		onshow: ShowList;
	} = $props();

	const lines = $derived(dry?.lines ?? []);
	const made = (l: { result: string }) => l.result === 'create' || l.result === 'created';
	const links = $derived(lines.some((l) => (l.group === 'sources' || l.group === 'downloaders') && made(l)));
	const newPeople = $derived(lines.filter((l) => l.group === 'people' && made(l)).length);
	const needs = $derived(needsGroups(dry?.needs_input ?? []).flatMap((g) => g.items.map((t, i) => ({ key: `${g.label}-${i}`, name: t, hint: g.label }))));
	const detail = $derived(note.replace(/\s+(moved|undone)\.?$/, ''));
</script>

<SettingsCard id="move-step" title={countSteps ? 'Step 3 of 4 · Move' : 'Move'} description={running ? undefined : 'Before you move'}>
	{#if running}
		<div class="srow">
			<div class="cap">
				{#if lost}
					<StepCapsule tone="soft" fill={progress} status="pending" word="Can't reach HolyArrs" detail="trying again" testid="move-progress" />
					<p class="sr-only" role="status">Can't reach HolyArrs. Trying again.</p>
				{:else}
					<StepCapsule tone="go" live progress fill={progress} status="processing" word="Moving" detail={detail || undefined} label="Move progress" testid="move-progress" />
				{/if}
			</div>
		</div>
		<div class="srow">
			<p class="txt hint">You can leave this page. The move keeps going.</p>
		</div>
	{:else}
		{#if safeMode}
			<p class="notice warn" role="alert">HolyArrs started in safe mode. Restart it normally to move or undo. <a href={to('/settings/danger')}>Danger zone</a></p>
		{/if}
		{#if links}
			<div class="srow">
				<p class="txt">Sources and Downloaders arrive paused. They can't reach your home network until you turn on May reach the local network.</p>
			</div>
		{/if}
		{#if needs.length > 0}
			<div class="srow">
				<div class="txt"><span class="label">{plural(needs.length, 'key or password', 'keys and passwords')} to type again afterwards</span></div>
				<div class="ctl"><Button onclick={() => onshow('Keys to add again', needs, (n) => n)} aria-label="Show Keys to add again">Show</Button></div>
			</div>
		{/if}
		{#if newPeople > 0}
			<div class="srow">
				<p class="txt">{plural(newPeople, 'person arrives', 'people arrive')} turned off until you invite them.</p>
			</div>
		{/if}
		<div class="srow">
			<p class="txt hint">Your old apps aren't changed. You can undo this move.</p>
		</div>
	{/if}
</SettingsCard>

<style>
	.cap { grid-column: 1 / -1; min-width: 0; }
	p.txt { margin: 0; grid-column: 1 / -1; font: var(--t-callout); }
	p.txt.hint { color: var(--text-muted); }
	.notice { margin: 4px 0 8px; }
</style>
