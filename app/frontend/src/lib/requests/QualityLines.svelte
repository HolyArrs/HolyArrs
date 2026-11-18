<script lang="ts">
	// What people can ask for, one kind's list on Request rules. Each line is one box: the owner's own name and the name people see side by
	// side, then what it asks for, whether it is shown and its place. The owner's own lines also pick their quality and can be removed.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { LINES_MAX, addLine, moveLine, ownLine, ownerName, removeLine } from './quality-lines.ts';
	import { qualityName } from './quality-row.ts';
	import type { QualityLine } from './types.ts';

	let {
		lines = $bindable(),
		kind,
		rules,
		hint
	}: { lines: QualityLine[]; kind: 'movie' | 'show'; /** The Pick rules a line of this kind can use. */ rules: { id: number; name: string }[]; /** What a line asks for, in words. */ hint: (l: QualityLine) => string } = $props();
</script>

<ul class="q-list">
	{#each lines as l, i (l.id)}
		{@const id = `q-${kind}-${l.id}`}
		<li class="q-box" aria-label={ownerName(l) || t('settings.requests.choiceNew')}>
			<div class="q-names" class:own={ownLine(l.id)}>
				<label class="q-f" for="{id}-label"
					><span>{t('settings.requests.choiceLabel')}</span>
					<input id="{id}-label" class="input" maxlength="30" placeholder={qualityName(l.id, l.name.trim()) || t('settings.requests.choiceNew')} bind:value={lines[i].label} data-testid="q-label-{kind}-{l.id}" /></label
				>
				<label class="q-f" for={id}
					><span>{t('settings.requests.choiceSeen')}</span>
					<input {id} class="input" maxlength="30" required={ownLine(l.id)} placeholder={ownLine(l.id) ? t('settings.alerts.name') : qualityName(l.id)} aria-describedby="{id}-hint" bind:value={lines[i].name} /></label
				>
				{#if ownLine(l.id)}
					<label class="q-f" for="{id}-rules"
						><span>{t('requests.window.quality')}</span>
						<select id="{id}-rules" class="input" bind:value={lines[i].pick_rules_id} data-testid="q-rules-{kind}-{l.id}">
							{#each rules as r (r.id)}
								<option value={r.id}>{r.name}</option>
							{/each}
						</select></label
					>
				{/if}
			</div>
			<div class="q-foot">
				<span class="hint" id="{id}-hint">{hint(l)}</span>
				<label class="switch q-sw"><input type="checkbox" checked={!l.hidden} onchange={(e) => (lines[i].hidden = !e.currentTarget.checked)} data-testid="q-shown-{kind}-{l.id}" /><span class="sw" aria-hidden="true"></span><span>{t('settings.requests.choiceShown')}</span></label>
				<span class="q-acts">
					<IconButton icon="chevronUp" label={t('downloads.act.up')} disabled={i === 0} onclick={() => (lines = moveLine(lines, i, -1))} data-testid="q-up-{kind}-{l.id}" />
					<IconButton icon="chevronDown" label={t('downloads.act.down')} disabled={i === lines.length - 1} onclick={() => (lines = moveLine(lines, i, 1))} data-testid="q-down-{kind}-{l.id}" />
					{#if ownLine(l.id)}<IconButton icon="trash" label={t('downloads.act.remove')} onclick={() => (lines = removeLine(lines, i))} data-testid="q-remove-{kind}-{l.id}" />{/if}
				</span>
			</div>
		</li>
	{/each}
</ul>
<div class="q-add">
	<Button size="sm" icon="plus" disabled={lines.length >= LINES_MAX || !rules.length} onclick={() => (lines = addLine(lines, rules[0]?.id ?? 0))} data-testid="q-add-{kind}">{t('settings.requests.choiceAdd')}</Button>
</div>

<style>
	.q-list { display: grid; gap: var(--s-2); margin: 0; padding: 0; list-style: none; }
	.q-box { display: grid; gap: var(--s-2); padding: var(--s-4) var(--s-4) var(--s-2); border-radius: 16px; background: var(--surface-2); }
	.q-names { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--s-2) var(--s-3); }
	.q-names.own { grid-template-columns: repeat(3, minmax(0, 1fr)); }
	.q-f { display: grid; gap: var(--s-1); min-width: 0; }
	.q-f span { font: var(--t-footnote); font-weight: 600; color: var(--text-muted); }
	.q-box .input { min-height: 44px; width: 100%; background: var(--surface-1); }
	/* The hint takes what is left of the row; the switch and the order buttons keep together at the end and wrap under it when narrow. */
	.q-foot { display: flex; flex-wrap: wrap; align-items: center; gap: 0 var(--s-3); }
	.q-foot .hint { flex: 1 1 200px; min-width: 0; margin: 0; overflow-wrap: anywhere; }
	.q-sw { display: inline-flex; align-items: center; gap: var(--s-2); min-height: 44px; margin-inline-start: auto; font: var(--t-callout); }
	.q-acts { display: inline-flex; align-items: center; gap: 0; min-width: 108px; }
	.q-acts :global(button:disabled) { opacity: 0.3; cursor: default; }
	.q-add { justify-self: start; }
	/* On phones each field takes the full width, so a 30-letter name is read whole instead of cut at the box edge. */
	@media (max-width: 640px) {
		.q-names, .q-names.own { grid-template-columns: minmax(0, 1fr); gap: var(--s-2); }
		.q-foot { gap: var(--s-1) var(--s-3); }
		.q-sw { margin-inline-start: 0; }
		.q-acts { margin-inline-start: auto; justify-content: flex-end; }
	}
</style>