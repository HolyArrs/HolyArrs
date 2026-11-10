<script lang="ts">
	// Season dropdown, or a plain label when there is one season.
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import { seasonName } from './labels.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	let {
		seasons,
		value,
		fresh,
		onpick
	}: {
		seasons: { number: number; name?: string }[];
		value: number;
		fresh?: number;
		onpick: (n: number) => void;
	} = $props();

	// Provider names can be missing, so fall back to a generated label.
	const nameOf = (s: { number: number; name?: string }) => s.name || seasonName(s.number);
	const cur = $derived(seasons.find((s) => s.number === value));
	const items = $derived<MenuItem[]>(seasons.map((s) => ({ label: nameOf(s), checked: s.number === value, hint: s.number === fresh ? t('shows.picker.new') : undefined, onselect: () => onpick(s.number) })));
</script>

<span class="picker" data-testid="season-picker">
	{#if seasons.length > 1}
		<Menu label={t('shows.picker.pick')} variant="button" icon="tv" text={cur ? nameOf(cur) : t('shows.picker.season')} align="end" {items} />
	{:else if cur}
		<span class="one">{nameOf(cur)}</span>
	{/if}
	{#if fresh !== undefined && fresh === value}<span class="new" data-testid="season-new">{t('shows.picker.new')}</span>{/if}
</span>

<style>
	.picker { display: inline-flex; align-items: center; gap: 8px; }
	.one { font: var(--t-callout); font-weight: 600; }
	.new { display: inline-flex; align-items: center; height: 22px; padding: 0 8px; border-radius: 999px; background: var(--accent-fill); color: var(--text-on-red); font: 700 12px/1 var(--font-ui); letter-spacing: .02em; }
</style>
