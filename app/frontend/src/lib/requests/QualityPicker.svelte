<script lang="ts">
	// The qualities one person may pick on the request sheet, per kind, in the person editor (Settings > Users): one equal box per line the
	// owner shows (and any hidden one the person is limited to), on or off,
	// named as the owner named it. A kind is listed when there is a choice to make or one to clear.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import { editorLines, lineTicked, onlyTicked, tickLine } from './person-qualities.ts';
	import { ownerName } from './quality-lines.ts';
	import type { PersonQualities, QualityLine } from './types.ts';

	let { lines, value = $bindable() }: { lines: { movie: QualityLine[]; show: QualityLine[] }; value: PersonQualities } = $props();

	const kinds = $derived(
		(
			[
				{ kind: 'movie', label: t('settings.users.movieReq'), lines: editorLines(lines.movie, value.movie) },
				{ kind: 'show', label: t('settings.users.showReq'), lines: editorLines(lines.show, value.show) }
			] as const
		).filter((k) => k.lines.length > 0)
	);
</script>

{#if kinds.length}
	<fieldset class="pq" aria-describedby="person-q-hint" data-testid="person-qualities">
		<legend class="label">{t('settings.users.qualities')}</legend>
		{#each kinds as k (k.kind)}
			<div class="pq-kind" role="group" aria-labelledby="pq-{k.kind}">
				<span class="pq-name" id="pq-{k.kind}">{k.label}</span>
				<div class="pq-grid">
					{#each k.lines as l (l.id)}
						{@const on = lineTicked(value[k.kind], l.id)}
						<button
							type="button"
							class="pq-box"
							aria-pressed={on}
							aria-disabled={onlyTicked(value[k.kind], l.id)}
							onclick={() => {
								if (!onlyTicked(value[k.kind], l.id)) value = { ...value, [k.kind]: tickLine(k.lines, value[k.kind], l.id, !on) };
							}}
							data-testid="pq-{k.kind}-{l.id}"><span class="pq-t">{ownerName(l)}</span><span class="pq-dot" aria-hidden="true">{#if on}<Icon name="check" size={14} />{/if}</span></button
						>
					{/each}
				</div>
			</div>
		{/each}
		<span id="person-q-hint" class="hint">{t('settings.users.qualitiesHint')}</span>
	</fieldset>
{/if}

<style>
	.pq { display: grid; gap: var(--s-3); border: 0; padding: 0; margin: 0 0 var(--s-3); min-width: 0; }
	.pq > legend { padding: 0; margin-bottom: var(--s-2); }
	.pq-kind { display: grid; gap: var(--s-2); }
	.pq-name { font: var(--t-footnote); font-weight: 600; color: var(--text-muted); }
	.pq-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 132px), 1fr)); gap: var(--s-2); }
	/* Off and on share one shape: a hairline box with an empty ring, or a dark ring around the box with a filled tick. */
	.pq-box { display: flex; align-items: center; justify-content: space-between; gap: var(--s-2); min-width: 0; min-height: 44px; padding: var(--s-2) var(--s-3); border: 0; border-radius: 12px; background: var(--surface-1); box-shadow: inset 0 0 0 1px var(--line-strong); color: var(--text-muted); font: var(--t-subhead); font-weight: 600; text-align: start; cursor: pointer; transition: box-shadow 160ms var(--ease-out), color 160ms var(--ease-out), background 160ms var(--ease-out), transform 120ms var(--ease-out); }
	@media (hover: hover) and (pointer: fine) {
		.pq-box:hover:not([aria-disabled='true']) { background: var(--surface-2); color: var(--text); }
	}
	.pq-box:active:not([aria-disabled='true']) { transform: scale(0.98); }
	.pq-box[aria-pressed='true'] { color: var(--text); box-shadow: inset 0 0 0 1.5px var(--text); }
	.pq-box[aria-disabled='true'] { cursor: default; }
	.pq-box:focus-visible { outline: none; box-shadow: inset 0 0 0 1.5px var(--text), var(--focus-ring); }
	.pq-t { min-width: 0; overflow-wrap: anywhere; line-height: 1.25; }
	.pq-dot { display: inline-grid; place-items: center; flex: none; width: 20px; height: 20px; border-radius: 50%; box-shadow: inset 0 0 0 1.5px var(--line-strong); transition: background 160ms var(--ease-out); }
	.pq-box[aria-pressed='true'] .pq-dot { background: var(--text); color: var(--bg); box-shadow: none; }
	.pq .hint { margin: 0; }
	@media (prefers-reduced-motion: reduce) {
		.pq-box, .pq-dot { transition: none; }
		.pq-box:active:not([aria-disabled='true']) { transform: none; }
	}
</style>