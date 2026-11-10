<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import EmptyState from '#lib/ui/EmptyState.svelte';

	// Shown when the visible weeks hold nothing; offers the next weeks.
	let { onlater, text = t('discover.calEmpty.nothingInWeeks'), label = t('discover.calEmpty.showWeeksAfter') }: { onlater: () => void; text?: string; label?: string } = $props();
</script>

<div class="cal-empty" data-testid="cal-look-empty">
	<EmptyState compact icon="calendar" title={t('discover.calEmpty.nothingComingUpHere')} {text} action={{ label, onclick: onlater, icon: 'chevronRight' }} />
</div>

<style>
	.cal-empty { margin-bottom: 24px; }
</style>
