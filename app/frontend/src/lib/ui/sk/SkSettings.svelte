<script lang="ts">
	// Loading stand-in for settings cards: a heading, then label and field rows.
	let { cards = 1, rows = 3 }: { cards?: number; rows?: number } = $props();
</script>

{#each Array.from({ length: cards }, (_, i) => i) as c (c)}
	<div class="sk-set" aria-hidden="true">
		<span class="sk sk-line h"></span>
		{#each Array.from({ length: rows }, (_, i) => i) as r (r)}
			<span class="row"><span class="sk sk-line l"></span><span class="sk f"></span></span>
		{/each}
	</div>
{/each}

<style>
	.sk-set { display: flex; flex-direction: column; gap: 16px; padding: var(--s-5, 20px); margin-bottom: var(--s-4, 16px); border: 1px solid var(--line); border-radius: var(--r-panel); }
	.h { width: min(220px, 60%); height: 17px; }
	.row { display: flex; flex-direction: column; gap: 8px; }
	.l { width: min(160px, 45%); height: 11px; }
	/* Field cap width matches the real inputs. */
	.f { width: min(520px, 100%); height: 40px; border-radius: var(--r-btn); }
</style>
