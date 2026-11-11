<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { getContext as moveContext } from 'svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import StatusPill, { type StatusKind } from '#lib/ui/StatusPill.svelte';
	import StepCapsule from './StepCapsule.svelte';
	import { appName, bestStats, countWords, hostOf, latestPerInstance, moveCounts, moveStateWord, type Move, type Snapshot } from './move.ts';
	import { dateTime } from '../core/format.ts';

	const countSteps = moveContext<boolean | undefined>('move-hide-count') !== true;

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
		usedBy: Map<number, number>;
		reading?: string;
		moves: Move[];
		movesError?: string;
		onpick: (id: number, on: boolean) => void;
		onadd: () => void;
		onagain: (s: Snapshot) => void;
		onforget: (s: Snapshot) => void;
		onopen: (m: Move) => void;
		onretrymoves: () => void;
	} = $props();

	let allMoves = $state(false);
	const shownMoves = $derived(allMoves ? moves : moves.slice(0, 5));
	const pills: Record<string, StatusKind> = { ok: 'in_library', info: 'downloading', wait: 'missing', fail: 'failed', idle: 'requested' };

	// One row per copy of an app (kind and address); an older read of the same copy is switched off.
	const latest = $derived(latestPerInstance(snapshots));
	$effect(() => {
		for (const s of snapshots) {
			const use = latest.includes(s);
			if (use !== picked.includes(s.id)) onpick(s.id, use);
		}
	});

	/** The app's name, with its address when another copy of it is listed. */
	function label(s: Snapshot): string {
		const twin = latest.some((x) => x !== s && x.kind === s.kind);
		return twin && s.address ? t('migrate.appAt', { name: appName(s.kind), address: hostOf(s.address) }) : appName(s.kind);
	}

	function hint(s: Snapshot): string {
		const used = usedBy.get(s.id);
		return [used ? t('migrate.movedInMove', { n: used }) : '', bestStats(s.kind, s.counts) || countWords(s.kind, s.counts)].filter(Boolean).join(' · ');
	}
</script>

{#snippet reads()}
	{#if snapshots.length === 0 && !reading && !countSteps}
		<button type="button" class="add-row first" onclick={onadd}><Icon name="plus" size={16} />{t('migrate.addAnApp')}</button>
	{:else if snapshots.length === 0 && !reading}
		<EmptyState
			icon="upload"
			title={t('migrate.bringYourOldApps')}
			text={t('migrate.readSonarrRadarrAnd')}
			action={{ label: t('migrate.addAnApp'), icon: 'plus', onclick: onadd }}
		/>
	{:else}
		<ul class="reads">
			{#if reading}
				<li class="read" data-testid="read-pending">
					<div class="top">
						<div class="what"><b>{appName(reading)}</b><span class="sub">{t('migrate.readingNowYouCan')}</span></div>
						<span class="cap"><StepCapsule tone="go" live fill={0.4} word={t('migrate.reading')} /></span>
					</div>
				</li>
			{/if}
			{#each latest as s (s.id)}
				{@const h = hint(s)}
				<li class="read" data-testid="read-row">
					<div class="top">
						<div class="what" title={t('migrate.read2', { date: dateTime(s.created_at) })}><b>{label(s)}</b><span class="sub one">{h || t('migrate.nothingFound')}</span></div>
						<span class="ok-tag" title={t('migrate.read2', { date: dateTime(s.created_at) })}><Icon name="check" size={12} stroke={2.5} />{s.app_version ? `v${s.app_version.split('.').slice(0, 3).join('.')}` : t('migrate.read')}</span>
					</div>
					<div class="acts">
						<button type="button" class="quiet" disabled={!!reading} onclick={() => onagain(s)} aria-label={t('migrate.edit', { name: label(s) })}>{t('migrate.edit2')}</button>
						<button type="button" class="quiet danger" onclick={() => onforget(s)} aria-label={t('migrate.forgetTheReadFrom', { name: label(s), date: dateTime(s.created_at) })}>{t('migrate.remove')}</button>
					</div>
				</li>
			{/each}
		</ul>
		<button type="button" class="add-row" onclick={onadd} disabled={!!reading}><Icon name="plus" size={16} />{t('migrate.addAnApp')}</button>
	{/if}
{/snippet}

{#if countSteps}
	<SettingsCard id="move-step" title={t('migrate.step1Of4')}>{@render reads()}</SettingsCard>
{:else}
	<div id="move-step" tabindex="-1" aria-label={t('migrate.yourApps')}>{@render reads()}</div>
{/if}

{#if moves.length > 0 || movesError}
	<SettingsCard id="move-earlier" title={t('migrate.earlierMoves')}>
		{#if movesError}<LoadError hasData message={movesError} onretry={onretrymoves} />{/if}
		{#each shownMoves as m (m.id)}
			{@const st = moveStateWord(m)}
			<div class="srow earlier" data-testid="earlier-move">
				<div class="txt">
					<span class="label">{t('migrate.move2', { id: m.id, date: dateTime(m.created_at) })}</span>
					<span class="hint one">{moveCounts(m) || t('migrate.nothingCountedYet')}</span>
				</div>
				<div class="ctl">
					<span class="pill"><StatusPill kind={pills[st.tone]} label={st.word} /></span>
					<Button onclick={() => onopen(m)} aria-label={t('migrate.openMove', { id: m.id })}>{t('migrate.open')}</Button>
				</div>
			</div>
		{/each}
		{#if moves.length > 5 && !allMoves}
			<div class="srow">
				<div class="txt"></div>
				<div class="ctl"><Button onclick={() => (allMoves = true)}>{t('migrate.showAll', { n: moves.length })}</Button></div>
			</div>
		{/if}
	</SettingsCard>
{/if}

<style>
	.reads { list-style: none; margin: 0; padding: 0; display: grid; gap: 10px; }
	.read { padding: 14px 16px 10px; border-radius: 16px; background: var(--surface-2); border: 1px solid var(--line); }
	.ok-tag { flex: none; display: inline-flex; align-items: center; gap: 4px; padding: 4px 8px; border-radius: 999px; font: 500 12px/1.2 var(--font-mono); color: var(--ok-text, #30d158); border: 1px solid color-mix(in srgb, var(--ok-text, #30d158) 40%, transparent); }
	.top { display: flex; align-items: center; gap: 12px; }
	.what { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
	.what b { font: 600 15px/1.3 var(--font-ui); color: var(--text); }
	.sub { color: var(--text-muted); font: var(--t-footnote); }
	.acts { display: flex; gap: 16px; padding-top: 6px; }
	.quiet { border: 0; padding: 4px 0; background: none; color: var(--text-muted); font: 500 13px/1.2 var(--font-ui); cursor: pointer; }
	.quiet:hover:not(:disabled) { color: var(--text); }
	.quiet.danger:hover { color: var(--fail-text, #ff6961); }
	.quiet:disabled { opacity: 0.5; cursor: default; }
	@media (max-width: 600px), (pointer: coarse) {
		.acts { gap: 8px; padding-top: 0; margin-bottom: -8px; }
		.quiet { min-height: 44px; padding: 0 8px; margin-left: -8px; }
	}
	.quiet:focus-visible, .add-row:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 6px; }
	.add-row { width: 100%; margin-top: 10px; display: flex; align-items: center; gap: 10px; min-height: 48px; padding: 0 16px; border: 1px dashed color-mix(in srgb, var(--text) 18%, transparent); border-radius: 16px; background: none; color: var(--text-muted); font: 500 15px/1 var(--font-ui); cursor: pointer; }
	.add-row.first { margin-top: 0; }
	#move-step:focus { outline: none; }
	.add-row:hover:not(:disabled) { color: var(--text); border-color: color-mix(in srgb, var(--text) 32%, transparent); }
	.one { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.pill { display: flex; justify-content: flex-start; width: 168px; min-width: 0; }
	.cap { display: block; width: 168px; }
	@media (max-width: 600px) {
		.cap { width: 120px; }
		.srow.earlier { grid-template-columns: minmax(0, 1fr); }
		.earlier .ctl { justify-self: start; }
	}
</style>
