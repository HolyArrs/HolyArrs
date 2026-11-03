<script lang="ts">
	import SettingsCard from '../ui/SettingsCard.svelte';
	import Button from '../ui/Button.svelte';
	import IconButton from '../ui/IconButton.svelte';
	import EmptyState from '../ui/EmptyState.svelte';
	import LoadError from '../ui/LoadError.svelte';
	import StatusPill, { type StatusKind } from '../ui/StatusPill.svelte';
	import StepCapsule from '../ui/StepCapsule.svelte';
	import { dateTime } from '../format.ts';
	import { appLabel, appName, countWords, moveCounts, moveStateWord, type AppKind, type Move, type Snapshot } from '../move.ts';

	// Step 1: the reads kept so far, each switched on or off for this move,
	// and the earlier moves below.
	let {
		snapshots,
		picked,
		usedBy,
		reading = '',
		moves,
		movesError = '',
		onpick,
		onadd,
		onagain,
		onforget,
		onopen,
		onretrymoves
	}: {
		snapshots: Snapshot[];
		picked: number[];
		/** Read id → the done move that used it. */
		usedBy: Map<number, number>;
		reading?: string;
		moves: Move[];
		movesError?: string;
		onpick: (id: number, on: boolean) => void;
		onadd: () => void;
		onagain: (kind: AppKind) => void;
		onforget: (s: Snapshot) => void;
		onopen: (m: Move) => void;
		onretrymoves: () => void;
	} = $props();

	let allMoves = $state(false);
	const shownMoves = $derived(allMoves ? moves : moves.slice(0, 5));
	const pills: Record<string, StatusKind> = { ok: 'in_library', info: 'downloading', wait: 'missing', fail: 'failed', idle: 'requested' };

	function hint(s: Snapshot): string {
		const used = usedBy.get(s.id);
		return [used ? `Moved in Move ${used}` : '', `Read ${dateTime(s.created_at)}`, countWords(s.kind, s.counts)].filter(Boolean).join(' · ');
	}
</script>

<SettingsCard id="move-step" title="Step 1 of 5 · Read your apps" description="HolyArrs only looks. Nothing changes in those apps.">
	{#if snapshots.length === 0 && !reading}
		<EmptyState
			icon="upload"
			title="Bring your old apps over"
			text="Read Radarr, Sonarr and the others. Nothing changes in them, and nothing moves until you say so."
			action={{ label: 'Add an app', icon: 'plus', onclick: onadd }}
		/>
	{:else}
		{#if reading}
			<div class="srow" data-testid="read-pending">
				<div class="txt"><span class="label">{appLabel(reading)}</span><span class="hint one">Reading now. You can keep going.</span></div>
				<div class="ctl"><span class="cap"><StepCapsule tone="go" live fill={0.4} word="Reading…" /></span></div>
			</div>
		{/if}
		{#each snapshots as s (s.id)}
			{@const h = hint(s)}
			<div class="srow" data-testid="read-row">
				<div class="txt">
					<span class="label">{appLabel(s.kind)}</span>
					<span class="hint one" title={h}>{h}</span>
				</div>
				<div class="ctl">
					<label class="switch">
						<input id="move-read-{s.id}" type="checkbox" checked={picked.includes(s.id)} onchange={(e) => onpick(s.id, e.currentTarget.checked)} aria-label="Use the {appName(s.kind)} read from {dateTime(s.created_at)}" />
						<span class="sw" aria-hidden="true"></span>
					</label>
					<IconButton icon="refresh" label="Read {appName(s.kind)} again" disabled={!!reading} onclick={() => onagain(s.kind as AppKind)} />
					<IconButton icon="trash" label="Forget the {appName(s.kind)} read from {dateTime(s.created_at)}" onclick={() => onforget(s)} />
				</div>
			</div>
		{/each}
		<div class="srow add">
			<div class="txt"><span class="hint">Read another app, or a backup file.</span></div>
			<div class="ctl"><Button icon="plus" onclick={onadd} disabled={!!reading}>Add an app</Button></div>
		</div>
	{/if}
</SettingsCard>

{#if moves.length > 0 || movesError}
	<SettingsCard id="move-earlier" title="Earlier moves">
		{#if movesError}<LoadError hasData message={movesError} onretry={onretrymoves} />{/if}
		{#each shownMoves as m (m.id)}
			{@const st = moveStateWord(m)}
			<div class="srow earlier" data-testid="earlier-move">
				<div class="txt">
					<span class="label">Move {m.id} · {dateTime(m.created_at)}</span>
					<span class="hint one">{moveCounts(m) || 'Nothing counted yet'}</span>
				</div>
				<div class="ctl">
					<span class="pill"><StatusPill kind={pills[st.tone]} label={st.word} /></span>
					<Button onclick={() => onopen(m)} aria-label="Open move {m.id}">Open</Button>
				</div>
			</div>
		{/each}
		{#if moves.length > 5 && !allMoves}
			<div class="srow">
				<div class="txt"></div>
				<div class="ctl"><Button onclick={() => (allMoves = true)}>Show all {moves.length}</Button></div>
			</div>
		{/if}
	</SettingsCard>
{/if}

<style>
	.one { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	/* One left edge for every pill, whatever its word. */
	.pill { display: flex; justify-content: flex-start; width: 168px; min-width: 0; }
	.cap { display: block; width: 168px; }
	@media (max-width: 600px) {
		.cap { width: 120px; }
		/* Phones: the pill and Open go under the move's name, so nothing is cut. */
		.srow.earlier { grid-template-columns: minmax(0, 1fr); }
		.earlier .ctl { justify-self: start; }
	}
</style>
