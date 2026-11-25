<script lang="ts">
	// Settings > Import: the Downloaders a read just brought. One that answers is resumed;
	// one that does not says so in one muted line; one whose key the old app hides asks for it once.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import TestTag from '#lib/ui/TestTag.svelte';
	import { BroughtKey, checkBrought, kindWords } from '#lib/downloads/index.ts';
	import { to } from '#lib/core/paths.ts';
	import { appName, type AppKind } from './move.ts';
	import type { Brought } from './bring.ts';

	let { items }: { items: Brought[] } = $props();
	let tests = $state<Record<number, { ok: boolean; text: string }>>({});
	const seen = new Set<number>();

	$effect(() => {
		for (const b of items) {
			if (seen.has(b.id)) continue;
			seen.add(b.id);
			void checkBrought(b.id).then((r) => (tests = { ...tests, [b.id]: r }));
		}
	});
</script>

<ul class="brought" aria-label={t('settings.downloaders.title')} data-testid="brought-list">
	{#each items as b (b.id)}
		{@const test = tests[b.id]}
		<li>
			<div class="top">
				<div class="text"><b>{b.name}</b><span>{t('welcome.foundIn', { app: appName(b.app as AppKind) })}</span></div>
				{#if test}<TestTag ok={test.ok} text={test.text} app={kindWords[b.kind as keyof typeof kindWords] ?? b.kind} />{/if}
			</div>
			{#if test && !test.ok && b.needs}
				<BroughtKey id={b.id} kind={b.kind} app={appName(b.app as AppKind)} needs={b.needs} onsaved={(r) => (tests = { ...tests, [b.id]: r })} />
			{:else if test && !test.ok}
				<p class="line">{t('welcome.cantReachYet')} <a href={to('/settings/downloaders')}>{t('welcome.edit2')}</a></p>
			{/if}
		</li>
	{/each}
</ul>

<style>
	.brought { list-style: none; margin: 16px 0 0; padding: 0; display: grid; gap: 8px; }
	li { padding: 12px 14px; border-radius: 14px; background: var(--surface-2); border: 1px solid var(--line); min-width: 0; }
	.top { display: flex; align-items: center; gap: 12px; min-width: 0; }
	.text { flex: 1; min-width: 0; }
	.text b { display: block; font: 600 15px/1.3 var(--font-ui); color: var(--text); }
	.text span { display: block; margin-top: 2px; font-size: 12.5px; color: var(--text-muted); overflow-wrap: anywhere; }
	.line { margin: 8px 0 0; color: var(--text-muted); font: var(--t-footnote); }
</style>
