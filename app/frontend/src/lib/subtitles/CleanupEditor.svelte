<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { cleanupAll, cleanupKeys, cleanupSteps, langName, nextCleanupLanguage, noCleanup, renameCleanup, type CleanupOptions, type CleanupStep, type Language } from './api.ts';

	// rows is the cleanup setting itself: one row of steps per language, '*' for every other language.
	let { rows = $bindable(), langs, id }: { rows: Record<string, CleanupOptions>; langs: Language[]; id: string } = $props();

	const keys = $derived(cleanupKeys(rows, langs));
	const name = (k: string) => (k === cleanupAll ? t('subtitles.cleanup.all') : langName(k, langs) || k);
	const stepLabel = (s: CleanupStep): string =>
		({
			remove_ads: t('subtitles.cleanup.removeAds'),
			remove_hi: t('subtitles.cleanup.removeHi'),
			remove_tags: t('subtitles.cleanup.removeTags'),
			remove_emoji: t('subtitles.cleanup.removeEmoji'),
			ocr_fixes: t('subtitles.cleanup.ocrFixes'),
			common_fixes: t('subtitles.cleanup.commonFixes'),
			fix_uppercase: t('subtitles.cleanup.fixUppercase'),
			fix_overlaps: t('subtitles.cleanup.fixOverlaps')
		})[s];

	function add() {
		const k = nextCleanupLanguage(rows, langs);
		if (k) rows[k] = noCleanup();
	}
</script>

<ul class="rows" data-testid="cleanup-editor">
	{#each keys as k, i (k)}
		<li class="row" data-testid="cleanup-row">
			<div class="head">
				<div class="field">
					<label for="{id}-lang-{i}">{t('subtitles.cleanup.language')}</label>
					<select id="{id}-lang-{i}" class="input" value={k} onchange={(e) => renameCleanup(rows, k, e.currentTarget.value)}>
						<option value={cleanupAll} disabled={k !== cleanupAll && cleanupAll in rows}>{t('subtitles.cleanup.all')}</option>
						{#each langs as l (l.code)}<option value={l.code} disabled={l.code !== k && l.code in rows}>{l.name}</option>{/each}
					</select>
				</div>
				<IconButton icon="x" variant="tinted" tooltip={false} onclick={() => delete rows[k]} label={t('subtitles.cleanup.takeOff', { name: name(k) })} />
			</div>
			<div class="steps">
				{#each cleanupSteps as s (s)}
					<label class="checkline">
						<input type="checkbox" bind:checked={rows[k][s]} data-testid="cleanup-{s}" />
						<span>{stepLabel(s)}</span>
					</label>
				{/each}
			</div>
		</li>
	{:else}
		<li class="none">{t('subtitles.cleanup.empty')}</li>
	{/each}
</ul>
{#if nextCleanupLanguage(rows, langs)}
	<div class="add"><Button icon="plus" onclick={add} data-testid="cleanup-add">{t('subtitles.cleanup.add')}</Button></div>
{/if}

<style>
	/* Empty list: a full row like the other empty lines in Settings. */
	.none { display: flex; align-items: center; min-height: 52px; color: var(--text-muted); font: 400 14px/1.35 var(--font-ui); }
	.rows {
		display: flex;
		flex-direction: column;
		gap: var(--s-2);
		margin-top: var(--s-3);
		padding: 0;
		list-style: none;
	}
	.row {
		padding: 12px;
		border-radius: var(--r-card);
		background: var(--surface-2);
	}
	.head {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto;
		gap: var(--s-3);
		align-items: end;
	}
	/* Label above its picker, as in the language rows above. */
	.head .field {
		display: flex !important;
		flex-direction: column;
		gap: 6px;
		min-width: 0;
		margin-top: 0;
		padding: 0 !important;
		border-top: 0 !important;
	}
	.steps {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0 var(--s-3);
		margin-top: var(--s-2);
	}
	.steps .checkline {
		margin: 0;
		min-height: 40px;
	}
	.add {
		margin-top: var(--s-3);
	}
	@media (max-width: 600px) {
		.row {
			padding: var(--s-3);
		}
		.steps {
			grid-template-columns: 1fr;
		}
		.steps .checkline {
			min-height: 44px;
		}
		.head :global(.input) {
			min-height: 44px;
		}
		.add :global(.ui-btn) {
			width: 100%;
		}
	}
</style>
