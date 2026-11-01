<script lang="ts">
	// The next steps a History row offers: swap a bad release for another, or
	// try a failed one again. Preview only: they say they are coming soon.
	import Button from '../ui/Button.svelte';
	import { toasts } from '../toasts.svelte.ts';
	import { canBlock, canRetry } from './dirs.ts';
	import type { HistoryRow } from '../history.ts';

	let { row, size = 'md' }: { row: HistoryRow; size?: 'sm' | 'md' } = $props();
	const soon = (what: string) => toasts.push(`${what}: coming soon.`, 'info');
</script>

{#if canBlock(row)}<Button variant="secondary" {size} icon="declined" data-testid="history-block" onclick={() => soon('Bad release, find another')}>Bad release, find another</Button>{/if}
{#if canRetry(row)}<Button variant="secondary" {size} icon="refresh" data-testid="history-retry" onclick={() => soon('Retry')}>Retry</Button>{/if}
