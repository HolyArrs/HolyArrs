<script lang="ts">
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Poster from '#lib/ui/Poster.svelte';
	import VerbMark from './HistoryVerb.svelte';
	import type { HistoryRow } from './history.ts';
	import { FACT_LABEL, glue, panelView, pathBits } from './history-panel.ts';
	import type { Snippet } from 'svelte';
	import { to } from '#lib/core/paths.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import type { TitleView } from '#lib/core/types.ts';

	// One History row in a sheet (bottom sheet on phones), with the same content and order as the details panel.
	let { row = $bindable(null), titleById, people, me, actions }: { row: HistoryRow | null; titleById: Map<number, TitleView>; people?: Map<number, string>; me?: number; /** Extra buttons for the row. */ actions?: Snippet<[HistoryRow]> } = $props();
	let open = $state(false);
	$effect(() => {
		open = !!row;
	});
	const v = $derived(row ? panelView(row, { titles: titleById, people, me }) : null);
	const titleLink = $derived(v?.actions.find((a) => a.key === 'title'));
	async function copy(path: string) {
		try {
			await navigator.clipboard.writeText(path);
			toasts.push(t('downloads.hes.copied'), 'ok');
		} catch {
			toasts.push(t('downloads.hes.copyFailed'), 'fail', { body: t('updates.goBack.copyFailedBody') });
		}
	}
</script>

<Sheet bind:open title={v?.title ?? t('downloads.hes.event')} placement="auto" size="md" hideTitle onclose={() => (row = null)}>
	{#if row && v}
		<div class="es" data-testid="hv-sheet">
			<div class="head">
				{#if v.art}<span class="art" class:square={v.art.square}><Poster src={v.art.src} name={v.art.name} /></span>{/if}
				<div class="hw">
					<VerbMark verb={v.verb} size="sm" />
					<p class="ttl">{v.title}</p>
					{#if v.meta}<p class="sub">{v.meta}</p>{/if}
				</div>
				<IconButton icon="x" label={t('health.why.close')} variant="tinted" size="sm" tooltip={false} onclick={() => (open = false)} />
			</div>
			{#if v.steps.length}
				<ol class="steps" aria-label={t('downloads.hes.steps')}>
					{#each v.steps as st (st.key)}<li class={st.state}><span class="pip" aria-hidden="true"></span>{st.label}<span class="sr-only">: {st.state === 'done' ? t('downloads.hes.done') : st.state === 'failed' ? t('downloads.hes.failed') : st.state === 'needs' ? t('downloads.hes.waiting') : t('downloads.hes.notYet')}</span></li>{/each}
				</ol>
			{/if}
			{#if v.line}<p class="sum" class:long={v.long}>{serverWords(v.line)}</p>{/if}
			<dl class="pf">
				{#each v.facts as f (f.label)}
					<dt>{FACT_LABEL[f.label]}</dt>
					<dd class:mono={f.mono} class:one={f.label === 'Release'} title={f.label === 'Release' ? f.value : undefined}>
						{#if f.at}<time datetime={f.at}>{f.value}</time>{#if f.ago}<span class="ago">{` · ${f.ago}`}</span>{/if}
						{:else if f.full}<span class="path"><span class="pv" title={f.full}>{#each pathBits(f.value) as b, i (i)}{glue(b)}<wbr>{/each}</span><button type="button" class="cp" aria-label={t('downloads.hes.copyPath')} onclick={() => copy(f.full ?? '')}><Icon name="copy" size={14} /></button></span>
						{:else}{f.value}{/if}
					</dd>
				{/each}
			</dl>
			{#if actions ? v.find || v.actions.length : titleLink}
				<div class="acts">
					{#if actions}{@render actions(row)}
					{:else if titleLink}<Button variant="secondary" size="sm" href={to(titleLink.href)} iconAfter="chevronRight">{titleLink.label}</Button>{/if}
				</div>
			{/if}
		</div>
	{/if}
</Sheet>

<style>
	.es { display: flex; flex-direction: column; min-width: 0; margin: 8px -6px 0; }
	.head { display: flex; gap: 12px; align-items: start; min-width: 0; }
	.head > :global(.ui-ibtn) { flex: none; margin: -4px -4px 0 0; }
	.art { flex: none; width: 56px; --poster-ratio: 2 / 3; }
	.art.square { --poster-ratio: 1; }
	.art :global(.poster) { border-radius: 10px; box-shadow: 0 0 0 1px var(--line); }
	.hw { flex: 1; display: flex; flex-direction: column; align-items: flex-start; min-width: 0; }
	.ttl { margin: 4px 0 0; color: var(--text); font: var(--t-title3); letter-spacing: var(--t-title3-track); display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; overflow: hidden; overflow-wrap: anywhere; }
	.sub { width: 100%; margin: 2px 0 0; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.steps { display: flex; align-items: center; margin: 12px 0 0; padding: 0; list-style: none; font: var(--t-footnote); white-space: nowrap; }
	.steps li { display: inline-flex; align-items: center; gap: 6px; color: var(--text-muted); }
	.steps li + li { flex: 1 1 auto; min-width: 0; }
	.steps li + li::before { content: ''; flex: 1 1 auto; min-width: 10px; height: 1px; margin: 0 4px 0 10px; background: var(--line); }
	.pip { flex: none; width: 8px; height: 8px; border-radius: 50%; box-shadow: inset 0 0 0 1.5px var(--text-muted); }
	.steps .done { color: var(--text); }
	.steps .done .pip { background: var(--ok); box-shadow: none; }
	.steps .failed { color: var(--fail-text); font-weight: 600; }
	.steps .failed .pip { background: var(--fail); box-shadow: none; }
	.steps .needs { color: var(--text); font-weight: 600; }
	.steps .needs .pip { background: var(--accent); box-shadow: none; }
	.sum { margin: 12px 0 0; color: var(--text); font: var(--t-subhead); display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; overflow: hidden; overflow-wrap: anywhere; }
	.sum.long { -webkit-line-clamp: 3; line-clamp: 3; }
	.pf { display: grid; grid-template-columns: 72px minmax(0, 1fr); gap: 8px 12px; margin: 20px 0 0; padding: 0; font: var(--t-footnote); }
	.pf dt { color: var(--text-muted); }
	.pf dd { margin: 0; min-width: 0; color: var(--text); font: var(--t-footnote); font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
	.pf dd.mono { font: var(--t-caption); font-family: var(--font-mono); font-weight: 400; line-height: 1.45; }
	.pf dd.one { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.pf .ago { color: var(--text-muted); }
	.path { display: inline-flex; align-items: flex-start; gap: 4px; min-width: 0; max-width: 100%; }
	.pv { min-width: 0; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; overflow: hidden; overflow-wrap: anywhere; }
	.cp { flex: none; display: grid; place-items: center; width: 24px; height: 24px; margin-top: -4px; padding: 0; border: 0; border-radius: 6px; background: none; color: var(--text-muted); cursor: pointer; }
	.cp:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.acts { margin-top: 16px; padding-top: 16px; border-top: 1px solid var(--line); }
	.acts :global(.da) { flex-direction: column; align-items: stretch; }
	.acts :global(.ui-btn) { width: 100%; min-height: 44px; justify-content: center; border-radius: var(--r-pill); }
	.acts :global(.ask .btns) { flex-direction: column; align-items: stretch; }
	.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
</style>
