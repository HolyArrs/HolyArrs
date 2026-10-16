<script lang="ts">
	import { to } from './paths.ts';
	// A poster, or the title's initials on a quiet tile when there is no picture.
	let { src, name }: { src?: string | null; name: string } = $props();
	let broken = $state(false);
	const usable = $derived(!!src && !broken && (src.startsWith('https://') || src.startsWith('/')));
	const initials = $derived(
		name
			.split(/\s+/)
			.filter(Boolean)
			.slice(0, 2)
			.map((w) => w[0]?.toUpperCase() ?? '')
			.join('')
	);
</script>

<div class="poster">
	{#if usable}
		<img src={to(src ?? '')} alt="" loading="lazy" decoding="async" referrerpolicy="no-referrer" onerror={() => (broken = true)} />
	{:else}
		<span aria-hidden="true">{initials}</span>
	{/if}
</div>

<style>
	.poster {
		position: relative;
		aspect-ratio: var(--poster-ratio);
		border-radius: var(--r-btn);
		overflow: hidden;
		background:
			linear-gradient(160deg, color-mix(in srgb, var(--ha-steel) 40%, transparent), transparent 60%),
			var(--surface-2);
		border: 1px solid var(--line);
		display: grid;
		place-items: center;
	}
	img {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	span {
		font: var(--weight-semibold) var(--text-xl) / 1 var(--font-display);
		color: var(--text-faint);
		letter-spacing: var(--tracking-display);
	}
</style>
