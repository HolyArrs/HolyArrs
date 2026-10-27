<script lang="ts">
	import Menu, { type MenuItem } from '../ui/Menu.svelte';
	import { to } from '../paths.ts';
	import type { DownloadRow } from '../downloaders.ts';

	// The ⋯ menu every Downloads layout shares: Details, then Mark failed and
	// Remove when HolyArrs can still act on the download, and the way to Needs
	// a decision when the download waits for one.
	let {
		r,
		name,
		busy,
		ondetails,
		onfail,
		onremove,
		details = true,
		on = 'surface'
	}: {
		r: DownloadRow;
		name: string;
		busy: number;
		ondetails: (r: DownloadRow) => void;
		onfail: (r: DownloadRow) => void;
		onremove: (r: DownloadRow) => void;
		/** Include Details in the menu (when the layout has no Details button of its own). */
		details?: boolean;
		on?: 'surface' | 'art';
	} = $props();

	const items = $derived.by((): MenuItem[] => {
		const out: MenuItem[] = [];
		if (details) out.push({ label: 'Details', icon: 'info', onselect: () => ondetails(r) });
		if (r.phase === 'needs_a_decision') out.push({ label: 'Open Needs a decision', icon: 'arrow', href: to('/downloads/decide') });
		if (r.can_act) {
			if (out.length) out.push({ kind: 'separator' });
			out.push({ label: 'Mark failed', icon: 'alert', disabled: busy === r.id, onselect: () => onfail(r) });
			out.push({ label: 'Remove', icon: 'trash', destructive: true, disabled: busy === r.id, onselect: () => onremove(r) });
		}
		return out;
	});
</script>

{#if items.length}
	<span class="dl-more {on}" data-testid="dl-more"><Menu label="More for {name}" {items} /></span>
{/if}

<style>
	.dl-more { display: inline-flex; flex: none; }
	.dl-more.art :global(button) { color: var(--on-art); background: var(--on-art-fill); -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px); border-radius: 50%; }
	.dl-more.art :global(button:hover) { background: var(--on-art-fill-hover); }
</style>
