<script lang="ts">
	import { onMount } from 'svelte';
	import { reducedMotion } from '../motion.ts';

	// In-page anchors under a title hero. A link only shows once its section
	// is on the page; following one opens its disclosure on a phone.
	let { items, label = 'On this page' }: { items: { id: string; label: string }[]; label?: string } = $props();

	let present = $state<Set<string>>(new Set());
	const shown = $derived(items.filter((i) => present.has(i.id)));

	onMount(() => {
		let frame = 0;
		const check = () => {
			cancelAnimationFrame(frame);
			frame = requestAnimationFrame(() => {
				const next = new Set(items.filter((i) => document.getElementById(i.id)).map((i) => i.id));
				if (next.size !== present.size || [...next].some((id) => !present.has(id))) present = next;
			});
		};
		check();
		const mo = new MutationObserver(check);
		mo.observe(document.body, { childList: true, subtree: true });
		return () => {
			mo.disconnect();
			cancelAnimationFrame(frame);
		};
	});

	function go(e: MouseEvent, id: string) {
		const el = document.getElementById(id);
		if (!el) return;
		e.preventDefault();
		const fold = el.closest('details');
		if (fold && !fold.open) fold.open = true;
		el.scrollIntoView({ behavior: reducedMotion() ? 'auto' : 'smooth', block: 'start' });
		history.replaceState(history.state, '', `#${id}`);
	}
</script>

{#if shown.length > 1}
	<nav class="ui-secnav" aria-label={label}>
		<ul>
			{#each shown as s (s.id)}
				<li><a href="#{s.id}" onclick={(e) => go(e, s.id)}>{s.label}</a></li>
			{/each}
		</ul>
	</nav>
{/if}

<style>
	.ui-secnav { min-width: 0; margin-top: calc(clamp(28px, 4vw, 44px) * -0.5); }
	ul { display: flex; gap: 8px; margin: 0; padding: 2px 0; list-style: none; overflow-x: auto; scrollbar-width: none; }
	ul::-webkit-scrollbar { display: none; }
	a { display: inline-flex; align-items: center; min-height: 44px; padding: 0 16px; border-radius: var(--r-pill); background: var(--surface-2); color: var(--text); font: var(--t-subhead); font-size: 14px; white-space: nowrap; transition: background var(--dur-fast) var(--ease-out); }
	a:hover { background: var(--surface-3); }
	a:focus-visible { outline: none; box-shadow: var(--focus-ring); }
</style>
