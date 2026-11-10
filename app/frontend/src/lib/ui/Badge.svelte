<script lang="ts">
	// A tiny label chip, drawn as an art pill when placed over artwork.
	import ArtPill from './ArtPill.svelte';
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	let { label, on = 'surface', title }: { label: string; on?: 'surface' | 'art'; title?: string } = $props();
</script>

{#if on === 'art'}<ArtPill text={serverWords(label)} label={title} />{:else}<span class="ui-badge {on}" {title}>{serverWords(label)}</span>{/if}

<style>
	.ui-badge { display: inline-flex; align-items: center; height: 20px; padding: 0 6px; border-radius: 5px; border: 1px solid var(--line-strong); color: var(--text-muted); font: var(--t-caption); letter-spacing: .01em; white-space: nowrap; font-variant-numeric: tabular-nums; }
</style>
