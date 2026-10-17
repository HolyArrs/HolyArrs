<script lang="ts">
	import ReleaseList from '#lib/ReleaseList.svelte';
</script>

<svelte:head><title>Turned down · HolyArrs</title></svelte:head>

<div class="release-workspace">
	<p class="eyebrow">Library management</p>
	<ReleaseList list="turned-down" />
</div>

<style>
	.eyebrow { margin: 0 0 var(--s-2); font-size: 11px; font-weight: var(--weight-semibold); letter-spacing: .09em; text-transform: uppercase; color: var(--text-muted); }
	.release-workspace :global(.page-head) { margin-bottom: var(--s-4); }
	.release-workspace :global(.switch-list) { padding: 3px; border: 1px solid var(--line); border-radius: var(--r-btn); background: var(--surface-2); }
	.release-workspace :global(.switch-list a[aria-current='page']) { background: var(--surface-1); box-shadow: var(--shadow-control); }
	.release-workspace :global(.list-head) { min-height: 56px; margin: 0; padding: var(--s-2) var(--s-4); border: 1px solid var(--line); border-bottom: 0; border-radius: var(--r-card) var(--r-card) 0 0; background: var(--surface-2); }
	.release-workspace :global(.rels) { padding: 0; border-radius: 0 0 var(--r-card) var(--r-card); box-shadow: none; }
	.release-workspace :global(.rel) { padding: var(--s-3) var(--s-4); border: 0; border-radius: 0; box-shadow: none; }
	.release-workspace :global(.rel + .rel) { margin-top: 0; border-top: 1px solid var(--line); }
	@media (max-width: 600px) {
		.release-workspace :global(.switch-list), .release-workspace :global(.switch-list .btn) { width: 100%; }
		.release-workspace :global(.switch-list .btn), .release-workspace :global(.rel .btn) { min-height: 44px; }
	}
</style>
