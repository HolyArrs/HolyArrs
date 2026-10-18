<script lang="ts">
	import { press } from '../motion.ts';

	// A person in a cast and crew shelf: round photo (initials fallback), name, role.
	let { name, role, image, href, size = 96 }: { name: string; role?: string; image?: string | null; href?: string; size?: number } = $props();
	const initials = $derived(name.split(/\s+/).filter(Boolean).slice(0, 2).map((w) => w[0]?.toUpperCase()).join(''));
	let failed = $state(false);
</script>

<svelte:element this={href ? 'a' : 'div'} class="ui-person" href={href} style="--size: {size}px" use:press={'card'}>
	<span class="ph">
		{#if image && !failed}<img src={image} alt="" loading="lazy" decoding="async" onerror={() => (failed = true)} />{:else}<span aria-hidden="true">{initials}</span>{/if}
	</span>
	<span class="n">{name}</span>
	{#if role}<span class="r">{role}</span>{/if}
</svelte:element>

<style>
	.ui-person { display: flex; flex-direction: column; align-items: center; gap: 2px; width: calc(var(--size) + 16px); text-align: center; color: var(--text); border-radius: var(--r-card); }
	.ph { width: var(--size); height: var(--size); margin-bottom: 6px; display: grid; place-items: center; border-radius: 50%; overflow: hidden; background: linear-gradient(160deg, var(--surface-2), var(--surface-3)); color: var(--text-muted); font: var(--t-title3); box-shadow: var(--shadow-card); transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out); }
	img { width: 100%; height: 100%; object-fit: cover; }
	a.ui-person:hover .ph { transform: translateY(var(--lift)); box-shadow: var(--shadow-lift); }
	a.ui-person:focus-visible { outline: none; }
	a.ui-person:focus-visible .ph { box-shadow: var(--focus-ring); }
	.n { font: var(--t-subhead); font-weight: 600; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.r { color: var(--text-muted); font: var(--t-footnote); max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
