<!-- The Library's count of found music files that fill no track, with the artists that hold most of them; each opens that artist's page, where they are matched. -->
<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import { api } from '#lib/core/api.ts';
	import { to } from '#lib/core/paths.ts';

	type Artist = { title_id: number; name: string; files: number };
	let total = $state(0);
	let artists = $state<Artist[]>([]);
	const SHOWN = 5;

	$effect(() => {
		void (async () => {
			try {
				const r = await api.get<{ total: number; artists: Artist[] }>('/library/unmatched-music');
				total = r.total ?? 0;
				artists = r.artists ?? [];
			} catch {
				total = 0;
			}
		})();
	});
</script>

{#if total > 0}
	<p class="um-note" data-testid="unmatched-music">
		<Icon name="alert" size={16} />
		<span>
			<b>{t(total === 1 ? 'library.unmatchedMusic.one' : 'library.unmatchedMusic.other', { n: total })}</b>
			{#each artists.slice(0, SHOWN) as a (a.title_id)}<a href={to(`/library/${a.title_id}`)} dir="auto">{a.name} ({a.files})</a>{/each}
			{#if artists.length > SHOWN}<small>{t('library.unmatchedMusic.more', { n: artists.length - SHOWN })}</small>{/if}
		</span>
	</p>
{/if}

<style>
	.um-note { display: flex; align-items: flex-start; gap: 8px; margin: 0; padding: 10px 12px; border-radius: var(--r-btn); background: var(--surface-2); color: var(--text); font: var(--t-callout); }
	.um-note span { display: flex; flex-wrap: wrap; align-items: baseline; gap: 4px 12px; min-width: 0; }
	.um-note b { font-weight: 600; }
	.um-note a { color: var(--accent); text-decoration: none; overflow-wrap: anywhere; }
	.um-note a:hover { text-decoration: underline; }
	.um-note small { color: var(--text-muted); }
	.um-note :global(svg) { flex: none; margin-top: 2px; color: var(--text-muted); }
</style>
