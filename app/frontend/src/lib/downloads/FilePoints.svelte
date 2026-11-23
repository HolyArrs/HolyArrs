<script lang="ts">
	// How the Pick rules score a file in the Library: total against good enough, the traits that fit and why it is short.
	import { api } from '#lib/core/api.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { stepLabel, traitLabel } from './pick-rules.ts';
	import { filePointsWhy, type FilePointsView } from './file-points.ts';

	let { titleId, slotId }: { titleId: number; slotId: number } = $props();
	let fp = $state<FilePointsView | null>(null);

	$effect(() => {
		const url = `/titles/${titleId}/points?slot=${slotId}`;
		fp = null;
		void api.get<FilePointsView>(url).then((r) => (fp = r), () => (fp = null));
	});

	const total = $derived(fp ? (fp.good_enough_points != null ? t('downloads.filePoints.ofNeed', { n: fp.points, need: fp.good_enough_points }) : t('mediafiles.release.points', { n: fp.points })) : '');
	const why = $derived(fp && !fp.good_enough ? filePointsWhy(fp, stepLabel) : []);
	const name = (key: string) => fp?.names[key] ?? traitLabel(key);
</script>

{#if fp}
	<div class="fp" data-testid="file-points">
		<p class="fp-head">
			<b>{total}</b>
			<span class="fp-verdict" class:ok={fp.good_enough} data-testid="file-points-verdict">{fp.good_enough ? t('downloads.filePoints.goodEnough') : t('downloads.filePoints.belowGoodEnough')}</span>
		</p>
		{#each why as w (w)}<p class="fp-why">{w}</p>{/each}
		{#if fp.traits.length}
			<p class="fp-traits">{#each fp.traits as tr (tr.trait)}<span class="fp-trait">{name(tr.trait)} <b>{tr.points > 0 ? `+${tr.points}` : `−${Math.abs(tr.points)}`}</b></span>{/each}</p>
		{:else}
			<p class="fp-why">{t('downloads.filePoints.noTraits')}</p>
		{/if}
	</div>
{/if}

<style>
	.fp { display: grid; gap: 6px; min-width: 0; }
	.fp p { margin: 0; }
	.fp-head { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; font: 600 13px/1.3 var(--font-ui); }
	.fp-verdict { display: inline-flex; align-items: center; height: 20px; padding: 0 8px; border-radius: 99px; background: var(--accent-soft); color: var(--accent-text); font: 700 11px/1 var(--font-ui); }
	.fp-verdict.ok { background: color-mix(in srgb, var(--ok) 16%, transparent); color: var(--ok-text); }
	.fp-why { color: var(--text-muted); font: 500 12px/1.4 var(--font-ui); overflow-wrap: anywhere; }
	.fp-traits { display: flex; flex-wrap: wrap; gap: 5px; }
	.fp-trait { display: inline-flex; align-items: center; gap: 4px; max-width: 100%; height: 22px; padding: 0 8px; border-radius: 5px; border: 1px solid color-mix(in srgb, var(--text) 28%, transparent); color: var(--text-muted); font: 600 11px/1 var(--font-ui); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.fp-trait b { color: var(--text); font-variant-numeric: tabular-nums; }
</style>
