<script lang="ts">
	import { press } from '../motion.ts';
	import ArtPlaceholder from './ArtPlaceholder.svelte';

	// A person in a cast and crew shelf: round photo (initials fallback), name, role.
	let { name, role, image, href, size = 96 }: { name: string; role?: string; image?: string | null; href?: string; size?: number } = $props();
	let failed = $state(false);
	let loaded = $state(false);
	// A link reads as one name: "Harold Perrineau, Boyd Stevens".
	const label = $derived(role ? `${name}, ${role}` : name);
</script>

<svelte:element this={href ? 'a' : 'div'} class="ui-person" href={href} aria-label={href ? label : undefined} style="--size: {size}px" use:press={'card'}>
	<span class="ph" class:sk={!!image && !failed && !loaded}>
		{#if !image || failed || !loaded}<ArtPlaceholder {name} shape="circle" loading={!!image && !failed && !loaded} />{/if}
		{#if image && !failed}<img src={image} class:in={loaded} alt="" loading="lazy" decoding="async" onload={() => (loaded = true)} onerror={() => (failed = true)} />{/if}
	</span>
	<span class="n" aria-hidden={href ? 'true' : undefined}>{name}</span>
	{#if role}<span class="r" aria-hidden={href ? 'true' : undefined}>{role}</span>{/if}
</svelte:element>

<style>
	.ui-person { display: flex; flex-direction: column; align-items: center; gap: 2px; width: calc(var(--size) + 16px); text-align: center; color: var(--text); border-radius: var(--r-card); }
	.ph { position: relative; width: var(--size); height: var(--size); margin-bottom: 6px; display: grid; place-items: center; border-radius: 50%; overflow: hidden; background: linear-gradient(160deg, var(--surface-2), var(--surface-3)); color: var(--text-muted); font: var(--t-title3); box-shadow: var(--shadow-card); transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out); }
	img { width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity 240ms ease-out; }
	img.in { opacity: 1; }
	.ph.sk { border-radius: 50%; animation: sk-sweep 1.6s ease-in-out infinite; }
	a.ui-person { cursor: pointer; }
	a.ui-person:hover .ph { transform: translateY(var(--lift)); box-shadow: var(--shadow-lift), 0 0 0 2px color-mix(in srgb, var(--text) 28%, transparent); }
	a.ui-person:focus-visible { outline: none; }
	a.ui-person:focus-visible .ph { box-shadow: var(--focus-ring); }
	.n { font: var(--t-subhead); font-weight: 600; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.r { color: var(--text-muted); font: var(--t-footnote); max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
