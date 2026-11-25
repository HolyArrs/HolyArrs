<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
</script>

<!-- The move page while it loads: the step rail and two cards. -->
<div class="sk-rail" aria-hidden="true">{#each [1, 2, 3, 4] as n (n)}<span class="sk"></span>{/each}</div>
<SkSettings cards={2} rows={4} />

<style>
	.sk-rail { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px; }
	.sk-rail .sk { display: block; height: 40px; border-radius: 12px; }
	@media (pointer: coarse) {
		.sk-rail .sk { height: 44px; }
	}
	@media (max-width: 600px) {
		.sk-rail { grid-template-columns: minmax(0, 1fr) 44px 44px 44px; }
		.sk-rail .sk { height: 44px; }
	}
</style>
