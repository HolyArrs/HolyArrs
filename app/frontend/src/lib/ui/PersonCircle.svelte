<script lang="ts">
	// Round person photo with a name and role, with a wiki photo as backup.
	import ArtPlaceholder from './ArtPlaceholder.svelte';
	import { loadWikiFacts } from './wiki-facts.ts';
	import { tmdbSized } from './library-art.ts';
	import { press } from '../core/motion.ts';
	import { api } from '../core/api.ts';
	import { pictureOf } from '../core/pictures.ts';

	let { name, role, image, href, size = 96, needsArt = false }: { name: string; role?: string; image?: string | null; href?: string; size?: number; needsArt?: boolean } = $props();

	let failed = $state(false);
	let loaded = $state(false);
	let backup = $state('');
	let backupFailed = $state(false);
	let near = $state(false);

	const label = $derived(role ? `${name}, ${role}` : name);
	const personId = $derived(href?.match(/\/people\/(\d+)/)?.[1] ?? '');
	const shown = $derived(image && !failed ? image : backup && !backupFailed ? backup : '');
	const tint = $derived(pictureOf(image)?.color || null);
	const gone = $derived(needsArt && !shown);

	// Only ask for the backup photo once the circle is near the screen.
	$effect(() => {
		if ((image && !failed) || !personId || backup || !near) return;
		const id = personId;
		void loadWikiFacts(api.get, 'person', id, name).then((f) => {
			if (id === personId && f.image) backup = f.image;
		});
	});

	// Flips once, then stops watching.
	function nearView(node: HTMLElement) {
		const io = new IntersectionObserver((seen) => {
			if (seen.some((e) => e.isIntersecting)) {
				near = true;
				io.disconnect();
			}
		}, { rootMargin: '200px' });
		io.observe(node);
		return { destroy: () => io.disconnect() };
	}
</script>

<svelte:element this={href ? 'a' : 'div'} class="ui-person" class:gone href={href} aria-label={href ? label : undefined} style="--size: {size}px" use:press={'card'}>
	<span class="ph" class:sk={!!shown && !loaded} style:background={tint} use:nearView>
		{#if !shown || !loaded}<ArtPlaceholder {name} shape="circle" loading={!!shown && !loaded} color={tint} />{/if}
		{#if shown}{#key shown}<img src={tmdbSized(shown, 'profile', size)} class:in={loaded} alt="" loading="lazy" decoding="async" onload={() => (loaded = true)} onerror={() => (shown === backup ? (backupFailed = true) : (failed = true), (loaded = false))} />{/key}{/if}
	</span>
	<span class="n" aria-hidden={href ? 'true' : undefined}>{name}</span>
	{#if role}<span class="r" aria-hidden={href ? 'true' : undefined}>{role}</span>{/if}
</svelte:element>

<style>
	/* Hides the tile and its list item when art is required and missing. */
	.ui-person.gone, :global(li:has(> .ui-person.gone)) { display: none; }
	.ui-person { display: flex; flex-direction: column; align-items: center; gap: 2px; width: calc(var(--size) + 16px); text-align: center; color: var(--text); border-radius: var(--r-card); }
	.ph { position: relative; width: var(--size); height: var(--size); margin-bottom: 6px; display: grid; place-items: center; border-radius: 50%; overflow: hidden; background: linear-gradient(160deg, var(--surface-2), var(--surface-3)); color: var(--text-muted); font: var(--t-title3); box-shadow: var(--shadow-card); transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out); }
	img { width: 100%; height: 100%; object-fit: cover; opacity: 0; filter: saturate(.85) brightness(.94); transition: opacity var(--dur) var(--ease-out), filter var(--dur) var(--ease-out), transform var(--dur) var(--ease-out); }
	a.ui-person:hover img, a.ui-person:focus-visible img { filter: none; transform: scale(1.03); }
	@media (prefers-reduced-motion: reduce) { a.ui-person:hover img { transform: none; } }
	img.in { opacity: 1; }
	.ph.sk { border-radius: 50%; animation: sk-sweep 1.6s ease-in-out infinite; }
	a.ui-person { cursor: pointer; }
	a.ui-person:hover .ph { transform: translateY(var(--lift)); box-shadow: var(--shadow-lift), 0 0 0 2px color-mix(in srgb, var(--text) 28%, transparent); }
	a.ui-person:focus-visible { outline: none; }
	a.ui-person:focus-visible .ph { box-shadow: var(--focus-ring); }
	.n { font: var(--t-subhead); font-weight: 600; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.r { color: var(--text-muted); font: var(--t-footnote); max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
