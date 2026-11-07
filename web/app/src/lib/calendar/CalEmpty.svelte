<script lang="ts">
	import EmptyState from '#lib/ui/EmptyState.svelte';

	// What a variant shows when the shown weeks hold nothing: say so, and offer the next weeks.
	let { onlater, text = 'Nothing in these weeks with these filters.', label = 'Show the weeks after' }: { onlater: () => void; text?: string; label?: string } = $props();
</script>

<div class="cal-empty" data-testid="cal-look-empty">
	<EmptyState compact icon="calendar" title="Nothing coming up here" {text} action={{ label, onclick: onlater, icon: 'chevronRight' }} />
</div>

<style>
	.cal-empty { margin-bottom: 24px; }
</style>
