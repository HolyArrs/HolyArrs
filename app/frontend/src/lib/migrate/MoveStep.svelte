<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { getContext as moveContext } from 'svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Button from '#lib/ui/Button.svelte';
	import StepCapsule from './StepCapsule.svelte';
	import type { ShowList } from './ItemsSheet.svelte';
	import { needsGroups, progressWords, type Report } from './move.ts';
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
	const detail = $derived(progressWords(note));
</script>

<SettingsCard id="move-step" title={countSteps ? t('migrate.step3Of4') : t('migrate.move')} description={running ? undefined : t('migrate.beforeYouMove')}>
	{#if running}
		<div class="srow">
			<div class="cap">
				{#if lost}
					<StepCapsule tone="soft" fill={progress} status="pending" word={t('migrate.canTReachHolyarrs2')} detail={t('migrate.tryingAgain')} testid="move-progress" />
					<p class="sr-only" role="status">{t('migrate.canTReachHolyarrs')}</p>
				{:else}
					<StepCapsule tone="go" live progress fill={progress} status="processing" word={t('migrate.moving')} detail={detail || undefined} label={t('migrate.moveProgress')} testid="move-progress" />
				{/if}
			</div>
		</div>
		<div class="srow">
			<p class="txt hint">{t('migrate.youCanLeaveThis')}</p>
		</div>
	{:else}
		{#if safeMode}
			<p class="notice warn" role="alert"><Words key="migrate.holyarrsStartedInSafe2" links={{ a: to('/settings/danger') }} /></p>
		{/if}
		{#if links}
			<div class="srow">
				<p class="txt">{t('migrate.sourcesAndDownloadersArrive')}</p>
			</div>
		{/if}
		{#if needs.length > 0}
			<div class="srow">
				<div class="txt"><span class="label">{needs.length === 1 ? t('migrate.keyOrPasswordTo', { n: needs.length }) : t('migrate.keysAndPasswordsTo', { n: needs.length })}</span></div>
				<div class="ctl"><Button onclick={() => onshow(t('migrate.keysToAddAgain'), needs, (n) => n)} aria-label={t('migrate.showKeysToAdd')}>{t('migrate.show2')}</Button></div>
			</div>
		{/if}
		{#if newPeople > 0}
			<div class="srow">
				<p class="txt">{newPeople === 1 ? t('migrate.personArrivesTurnedOff', { n: newPeople }) : t('migrate.peopleArriveTurnedOff', { n: newPeople })}</p>
			</div>
		{/if}
		<div class="srow">
			<p class="txt hint">{t('migrate.yourOldAppsAren')}</p>
		</div>
	{/if}
</SettingsCard>

<style>
	.cap { grid-column: 1 / -1; min-width: 0; }
	p.txt { margin: 0; grid-column: 1 / -1; font: var(--t-callout); }
	p.txt.hint { color: var(--text-muted); }
	.notice { margin: 4px 0 8px; }
</style>
