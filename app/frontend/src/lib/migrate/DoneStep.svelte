<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import { getContext as moveContext } from 'svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Button from '#lib/ui/Button.svelte';
	import StepCapsule from './StepCapsule.svelte';
	import ResultGroups from './ResultGroups.svelte';
	import FolderOutcome from './FolderOutcome.svelte';
	import { FinishAlerts } from '#lib/alerts/index.ts';
	import type { ShowList } from './ItemsSheet.svelte';
	import { doneLook, moveCounts, noFolderCount, progressWords, running as isRunning, seasonsShows, type Bucket, type Move } from './move.ts';
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
	const detail = $derived(progressWords(note));
	const plainNext = $derived(!rep || !!rep.undo || (rep.lines ?? []).length === 0);
	const noFolder = $derived(noFolderCount(lines));
	const seasons = $derived(seasonsShows(lines));
	const look = $derived(doneLook(counts.failed ?? 0, seasons.length));
	// The import made Alerts: the Finish your alerts card asks for what each still needs.
	// Rename was chosen: the Rename preview shows each change before any file moves.
	const renameNext = $derived(!rep?.undo && !undone && move.choices?.names === 'rename');
	const madeAlerts = $derived(!rep?.undo && !undone && lines.some((l) => l.group === 'alerts' && l.result === 'created'));
</script>

<SettingsCard id="move-step" title={countSteps ? t('migrate.step4Of4') : t('migrate.done')}>
	<div class="srow">
		<div class="cap" data-testid="done-result">
			{#if busy && lost}
				<StepCapsule tone="soft" fill={progress} status="pending" word={t('migrate.canTReachHolyarrs2')} detail={t('migrate.tryingAgain')} />
				<p class="sr-only" role="status">{t('migrate.canTReachHolyarrs')}</p>
			{:else if busy}
				<StepCapsule tone="go" live progress fill={progress} status="processing" word={move.undo_state ? t('migrate.undoing') : t('migrate.moving')} detail={detail || undefined} label={move.undo_state ? t('migrate.undoProgress') : t('migrate.moveProgress')} testid="move-progress" />
			{:else if undoFailed}
				<StepCapsule tone="bad" fill={1} status="failed" word={t('migrate.undoFailed')} detail={(counts.failed ?? 0) === 1 ? t('migrate.item', { n: counts.failed ?? 0 }) : t('migrate.items', { n: counts.failed ?? 0 })} />
			{:else if undone}
				<StepCapsule tone="ok" fill={1} status="ready" word={t('migrate.undone')} detail={moveCounts(move) || undefined} />
			{:else if move.state === 'failed'}
				<StepCapsule tone="bad" fill={1} status="failed" word={t('migrate.stoppedWithErrors')} detail={moveCounts(move) || undefined} />
			{:else}
				<StepCapsule tone={look.tone} fill={1} status={look.status} word={t('migrate.moved')} detail={look.check ? t('migrate.needsYou') : moveCounts(move) || undefined} />
			{/if}
		</div>
		<span class="hint when">{t('migrate.move2', { id: move.id, date: dateTime(move.created_at) })}</span>
	</div>
</SettingsCard>

{#if rep && !busy}
	{#if plainNext || renameNext || noFolder > 0 || openChoices > 0 || (counts.failed ?? 0) > 0 || seasons.length > 0}
	<SettingsCard id="move-todo" title={t('migrate.needsYou')}>
		{#if plainNext}
			{#each rep.next ?? [] as n (n)}
				<div class="srow"><p class="txt">{serverWords(n)}</p></div>
			{/each}
		{:else}
			<div class="cards">
				{#snippet card(title: string, sub: string, act: string, go: (() => void) | string)}
					<div class="box card1">
						<span class="txt"><b>{title}</b>{#if sub}<span class="sub">{sub}</span>{/if}</span>
						{#if !act}{:else if typeof go === 'string'}<a class="quiet" href={go}>{act}</a>{:else}<button type="button" class="quiet" onclick={go}>{act}</button>{/if}
					</div>
				{/snippet}
				{#if noFolder > 0}
					{@render card(noFolder === 1 ? t('migrate.titleHaveNoFolder', { n: noFolder }) : t('migrate.titlesHaveNoFolder', { n: noFolder }), t('migrate.addTheirFolderThen'), t('migrate.folders'), to('/settings/library-folders'))}
				{/if}
				{#if renameNext}
					{@render card(t('migrate.namesRenameNext'), t('migrate.namesRenameNextHint'), t('migrate.namesRenameCheck'), to('/library/rename'))}
				{/if}
				{#if openChoices > 0}
					{@render card(openChoices === 1 ? t('migrate.chooseForPerson', { n: openChoices }) : t('migrate.chooseForPeople', { n: openChoices }), t('migrate.theyStayBehindUntil'), t('migrate.choose'), onchoose)}
				{/if}
				{#if seasons.length > 0}
					{@render card(seasons.length === 1 ? t('migrate.seasonsDifferOne') : t('migrate.seasonsDifferOther', { n: seasons.length }), t('migrate.seasonsDifferWhy', { list: seasons.map((s) => s.name).join(', ') }), seasons.length === 1 && seasons[0].id ? t('migrate.open') : '', seasons.length === 1 && seasons[0].id ? to(`/library/${seasons[0].id}`) : '')}
					{#if seasons.length > 1}
						{#each seasons.filter((s) => s.id) as s (s.id)}
							<div class="box card1" data-testid="seasons-show"><span class="txt"><b>{s.name}</b></span><a class="quiet" href={to(`/library/${s.id}`)}>{t('migrate.open')}</a></div>
						{/each}
					{/if}
				{/if}
				{#if (counts.failed ?? 0) > 0}
					{@render card((counts.failed ?? 0) === 1 ? t('migrate.itemFailed', { n: counts.failed ?? 0 }) : t('migrate.itemsFailed', { n: counts.failed ?? 0 }), t('migrate.reasonsMatchByHand'), t('migrate.matchByHand'), to('/library/import'))}
				{/if}
			</div>
		{/if}
	</SettingsCard>
	{/if}
	{#if madeAlerts}<FinishAlerts />{/if}
	{#if groups.length > 0}
		<SettingsCard id="move-results" title={t('migrate.results')}>
			{#if !rep.undo}<div class="fo-gap"><FolderOutcome {lines} after /></div>{/if}
			<ResultGroups report={rep} {groups} after {onshow} />
		</SettingsCard>
	{/if}
{/if}

<style>
	.cap { grid-column: 1 / -1; min-width: 0; }
	.fo-gap :global(ul) { margin-bottom: 10px; }
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
