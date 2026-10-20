<script lang="ts">
	import { askedByDated, askedByTip, firstName, initial, shortDate, WORDS, type Asker } from '../askedby.ts';

	// Asked by on the title page: "Asked by Ana · 12 Oct" under the title.
	// People who see every asker (they approve asks) can open the full list.
	let { askers, seesAll = false }: { askers: Asker[]; seesAll?: boolean } = $props();
	let open = $state(false);
	const id = $props.id();
	const line = $derived(askedByDated(askers));
	const tip = $derived(askedByTip(askers, seesAll));
</script>

{#if askers.length}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="asked" data-testid="asked-by-line" onkeydown={(e) => e.key === 'Escape' && open && ((open = false), e.stopPropagation())}>
		<!-- A popover button: the detail opens by click, tap or keyboard (no hover-only tooltip). -->
		<button type="button" class="t" aria-expanded={open} aria-controls="{id}-pop" onclick={() => (open = !open)}>{line}</button>
		{#if open}
			<div class="pop" id="{id}-pop">
				<p>{tip}</p>
				{#if seesAll && askers.length > 1}
					<ul aria-label={WORDS.listTitle}>
						{#each askers as a (a.id)}<li><span class="face">{initial(a.name)}</span>{a.me ? WORDS.you : firstName(a.name)}<span class="d">{shortDate(a.asked_at)}</span></li>{/each}
					</ul>
				{/if}
			</div>
		{/if}
	</div>
{/if}

<style>
	.asked { display: flex; flex-direction: column; gap: 6px; }
	.t { all: unset; font: 500 14px/1.35 var(--font-ui); color: rgb(255 255 255 / 86%); text-shadow: 0 1px 2px rgb(0 0 0 / 60%); }
	.t { cursor: pointer; text-decoration: underline dotted rgb(255 255 255 / 50%); text-underline-offset: 3px; }
	.t:focus-visible { box-shadow: var(--focus-ring); border-radius: 4px; }
	.pop { display: flex; flex-direction: column; gap: 6px; padding: 8px 10px; width: max-content; max-width: 100%; border-radius: 12px; color: var(--on-art); background: var(--on-art-scrim-mid); box-shadow: inset 0 0 0 1px var(--on-art-ring); -webkit-backdrop-filter: blur(14px); backdrop-filter: blur(14px); }
	.pop p { margin: 0; font: var(--t-caption); }
	ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 6px; }
	li { display: flex; align-items: center; gap: 8px; font: var(--t-caption); }
	.d { margin-left: auto; padding-left: 12px; opacity: 0.7; }
	.face { width: 18px; height: 18px; display: grid; place-items: center; border-radius: 50%; font: 700 10px/1 var(--font-ui); background: var(--accent); color: var(--on-art); }
</style>
