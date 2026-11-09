<script lang="ts">
	let { tone = 'info', size = 30 }: { tone?: 'ok' | 'fail' | 'info' | 'warn'; size?: number } = $props();
</script>

<svg class="ui-alert-sym {tone}" width={size} height={size} viewBox="0 0 24 24" aria-hidden="true">
	{#if tone === 'fail'}
		<path class="fill" d="M10.3 2.9 1.4 18.4a2 2 0 0 0 1.7 3h17.8a2 2 0 0 0 1.7-3L13.7 2.9a2 2 0 0 0-3.4 0z" />
		<path class="mark" d="M12 8.6v5M12 17.3h.01" />
	{:else}
		<circle class="fill" cx="12" cy="12" r="11" />
		{#if tone === 'ok'}<path class="mark" d="M7 12.4l3.3 3.3L17 9.1" />
		{:else if tone === 'warn'}<path class="mark" d="M12 6.6v6.6M12 16.9h.01" />
		{:else}<path class="mark" d="M12 11v6M12 7.3h.01" />{/if}
	{/if}
</svg>

<style>
	.ui-alert-sym { --sym: var(--info-on-art, #64b5ff); flex: none; display: block; }
	.ok { --sym: var(--ok-on-art, #30d158); }
	.fail { --sym: var(--fail-on-art, #ff6961); }
	.warn { --sym: var(--wait-on-art, #ffb340); }
	.fill { fill: var(--sym); }
	.mark { fill: none; stroke: #1c1c20; stroke-width: 2.4; stroke-linecap: round; stroke-linejoin: round; }
</style>
