<script lang="ts">
	import Icon from '#lib/ui/Icon.svelte';
	import { api } from '#lib/core/api.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { to } from '#lib/core/paths.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { daysLeftWords, soonestExpiry, type HeldFile } from './holding.ts';

	// Under a title's files, a quiet line says when deleted or replaced copies of it wait in Recently deleted, with a way to restore them. Nothing shows when there are none.
	let { titleId }: { titleId: number } = $props();

	let files = $state<HeldFile[]>([]);

	async function load(id: number) {
		try {
			const r = await api.get<{ files: HeldFile[] }>(`/holding?title_id=${id}`);
			if (id === titleId) files = r.files ?? [];
		} catch {
			// Only a hint: a failed read shows nothing.
		}
	}

	$effect(() => {
		const id = titleId;
		void load(id);
		let wait: ReturnType<typeof setTimeout> | undefined;
		const off = live.on('event', (e) => {
			const ev = e as { Kind?: string; TitleID?: number };
			if (ev.TitleID !== id || !/^(file_replaced|file_held|file_back|holding_cleaned)$/.test(ev.Kind ?? '')) return;
			clearTimeout(wait);
			wait = setTimeout(() => void load(id), 400);
		});
		return () => {
			off();
			clearTimeout(wait);
		};
	});
</script>

{#if files.length}
	<p class="held" data-testid="title-held-copies">
		<Icon name="trash" size={15} />
		<span class="what">
			{t(files.length === 1 ? 'mediafiles.held.one' : 'mediafiles.held.many', { n: files.length })}
			<span class="left">· {daysLeftWords(soonestExpiry(files))}</span>
		</span>
		<a class="go" href={to(`/library/deleted?title=${titleId}`)} data-testid="title-held-restore">{t('mediafiles.held.review')}</a>
	</p>
{/if}

<style>
	.held { display: flex; align-items: center; flex-wrap: wrap; gap: 6px 10px; margin: 12px 0 0; padding: 10px 14px; border: 1px solid var(--line); border-radius: 12px; color: var(--text-muted); font: var(--t-footnote); }
	.what { flex: 1; min-width: 0; }
	.left { font-variant-numeric: tabular-nums; }
	.go { color: var(--accent-text); font-weight: 600; text-decoration: none; min-height: 24px; display: inline-flex; align-items: center; }
	.go:hover { text-decoration: underline; text-underline-offset: 3px; }
	@media (pointer: coarse) { .go { min-height: 44px; margin-block: -10px; } }
</style>
