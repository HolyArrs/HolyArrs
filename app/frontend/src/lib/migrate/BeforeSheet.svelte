<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import type { ShowList } from './ItemsSheet.svelte';
	import { chooseHint, needsLook, resultWords, tone, type Bucket, type Line, type PersonChoice } from './move.ts';

	let {
		open = $bindable(false),
		groups,
		choose,
		people,
		total = 0,
		trying = false,
		ready = true,
		notice = '',
		onchoose,
		onmove,
		onshow
	}: {
		open?: boolean;
		groups: Bucket[];
		choose: Line[];
		people: Record<string, PersonChoice>;
		total?: number;
		trying?: boolean;
		ready?: boolean;
		notice?: string;
		onchoose: (key: string, c: PersonChoice) => void;
		onmove: () => void;
		onshow: ShowList;
	} = $props();

	const options = $derived<{ value: string; label: string }[]>([
		{ value: 'merge', label: t('migrate.samePerson') },
		{ value: 'skip', label: t('migrate.leaveOut') }
	]);
	const problems = $derived(
		groups.flatMap((g) =>
			(['not_found', 'failed'] as const)
				.filter((r) => (g.counts[r] ?? 0) > 0)
				.map((r) => ({ key: `${g.group}:${r}`, label: g.label, result: r, lines: g.lines.filter((l) => l.result === r) }))
		)
	);
	function lineRow(l: Line, i: number) {
		return { key: l.key + ':' + i, name: serverWords(l.name), hint: l.result === 'not_found' ? undefined : serverWords(l.detail), pill: { word: resultWords(l.result), tone: tone(l.result) }, look: needsLook(l.result) };
	}
</script>

<Sheet bind:open title={t('migrate.beforeYouMove')} description={t('migrate.willMoveTheseWon', { total })} size="md" placement="auto">
	<div data-testid="before-sheet">
		{#if notice}<p class="notice warn" role="status">{notice}</p>{/if}
		{#if problems.length > 0}
			<ul class="rg">
				{#each problems as p (p.key)}
					<li class="rg-row" data-testid="problem-row">
						<span class="rg-txt"><b>{p.lines.length} {p.label.toLowerCase()}</b> · {resultWords(p.result).toLowerCase()}</span>
						<button type="button" class="rg-show" onclick={() => onshow(p.label, p.lines, lineRow)} aria-label={t('migrate.show3', { label: p.label, result: resultWords(p.result).toLowerCase() })}>{t('migrate.show2')}</button>
					</li>
				{/each}
			</ul>
		{/if}
		{#if choose.length > 0}
			<section class="choices" aria-label={t('migrate.needsAChoice')}>
				<h3>{t('migrate.needsAChoice')}</h3>
				<p class="hint">{t('migrate.untilYouChooseThey')}</p>
				{#each choose as l, n (l.key)}
					<div class="crow" data-testid="choice-row">
						<div class="txt"><span class="label">{l.name}</span><span class="hint">{chooseHint(l.choose)}</span></div>
						<SegmentedControl label={t('migrate.choiceFor', { name: l.name })} testid="move-choice-{n}" {options} value={people[l.key] ?? ''} onchange={(v) => onchoose(l.key, v as PersonChoice)} />
					</div>
				{/each}
			</section>
		{/if}
	</div>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (open = false)}>{t('migrate.goBack')}</Button>
		<Button variant="primary" onclick={onmove} disabled={!ready || trying} busy={trying}>{t('migrate.moveAnyway')}</Button>
	{/snippet}
</Sheet>

<style>
	.rg { list-style: none; margin: 0 0 12px; padding: 0 16px; border-radius: 16px; background: var(--surface-2); border: 1px solid var(--line); }
	.rg-row { display: flex; align-items: center; gap: 12px; min-height: 48px; padding: 8px 0; border-top: 1px solid var(--line); }
	.rg-row:first-child { border-top: 0; }
	.rg-txt { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: var(--t-callout); color: var(--text); }
	.rg-txt b { font-weight: 600; }
	.rg-show { flex: none; border: 0; padding: 4px 0; background: none; color: var(--text-muted); font: 500 13px/1.2 var(--font-ui); cursor: pointer; }
	.rg-show:hover { color: var(--text); }
	.rg-show:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 6px; }
	.choices h3 { margin: 0; font: var(--t-headline); }
	.choices > .hint { margin: 2px 0 8px; color: var(--text-muted); font: var(--t-footnote); }
	.crow { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px 12px; padding: 8px 0; border-top: 1px solid var(--line); }
	.crow .txt { display: flex; flex-direction: column; min-width: 0; flex: 1 1 200px; }
	.crow .label { font: var(--t-callout); }
	.crow .hint { color: var(--text-muted); font: var(--t-footnote); }
	.notice { margin: 0 0 8px; }
	@media (max-width: 600px), (pointer: coarse) {
		.rg-show { min-height: 44px; padding: 0 4px; }
	}
</style>
