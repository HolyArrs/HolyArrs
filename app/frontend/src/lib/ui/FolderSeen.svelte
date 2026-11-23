<script lang="ts">
	// What HolyArrs sees in a setup folder: how many title folders are inside (movie, show or artist), or that it cannot see it.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { api } from '#lib/core/api.ts';
	import Icon from '#lib/ui/Icon.svelte';

	let { path, kind = '', id = '' }: { path: string; kind?: string; id?: string } = $props();

	let seen = $state<{ path: string; n: number } | { path: string; bad: true } | null>(null);
	let seq = 0;
	$effect(() => {
		const p = path.trim();
		if (!p.startsWith('/')) {
			seen = null;
			return;
		}
		const n = ++seq;
		const timer = setTimeout(async () => {
			try {
				const r = await api.post<{ folders: unknown[] }>('/migrate/folders', { path: p, names: [] });
				if (n === seq) seen = { path: p, n: r.folders.length };
			} catch {
				if (n === seq) seen = { path: p, bad: true };
			}
		}, 350);
		return () => clearTimeout(timer);
	});
	const now = $derived(seen && seen.path === path.trim() ? seen : null);
	const words = (n: number) => t(kind === 'movie' ? 'welcome.folders.movieFolders' : kind === 'show' ? 'welcome.folders.showFolders' : kind === 'music' ? 'welcome.folders.artistFolders' : 'welcome.folders.inside', { n });
</script>

{#if now && 'bad' in now}<p class="seen bad" role="status" data-testid="{id}-missing">{t('migrate.folderMissing')}</p>
{:else if now}<p class="seen" role="status" data-testid="{id}-seen"><Icon name="check" size={14} />{words(now.n)}</p>{/if}

<style>
	.seen { margin: 0; display: flex; align-items: center; gap: 6px; font: var(--t-footnote); color: var(--ok-text); }
	.seen.bad { color: var(--bad-text, #ff6b6b); }
</style>
