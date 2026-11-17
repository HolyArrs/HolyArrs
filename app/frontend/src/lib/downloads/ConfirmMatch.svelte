<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Sheet for downloads added outside HolyArrs that could not be matched to one title for sure (never other apps' Usenet categories). Picking a choice adds the title if missing and links the download. Reads only from HolyArrs, no outside lookups.
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { bannerWords, candidateLine, type Match, type Matches } from './matches.ts';
	import { api, message } from '../core/api.ts';
	import { toasts } from '../core/toasts.svelte.ts';

	let { refresh = 0, onchange }: { refresh?: number; onchange?: () => void } = $props();

	let data = $state<Matches | null>(null);
	let open = $state(false);
	let busy = $state(0);
	let running = $state(false);
	let later: ReturnType<typeof setTimeout> | undefined;
	$effect(() => {
		if (!open) clearTimeout(later);
		return () => clearTimeout(later);
	});
	const list = $derived(data?.matches ?? []);
	const words = $derived(bannerWords(list.length));

	async function read() {
		try {
			data = await api.get<Matches>('/downloads/matches');
		} catch {
			// A failed read leaves the banner hidden.
		}
	}
	$effect(() => {
		void refresh;
		void read();
	});

	async function pick(m: Match, body: Record<string, unknown>) {
		busy = m.id;
		try {
			await api.post(`/downloads/matches/${m.id}/pick`, body);
			await read();
			onchange?.();
			if (!list.length) open = false;
		} catch (e) {
			toasts.push(t('downloads.cm.confirmFailed'), 'fail', { body: message(e) });
		} finally {
			busy = 0;
		}
	}
	async function matchNow() {
		running = true;
		try {
			await api.post('/downloads/matches/run');
			toasts.push(t('downloads.cm.matching'), 'info', { body: t('downloads.cm.sure') });
			clearTimeout(later);
			later = setTimeout(() => void read(), 4000);
		} catch (e) {
			toasts.push(t('downloads.cm.startFailed'), 'fail', { body: message(e) });
		} finally {
			running = false;
		}
	}
</script>

{#if words}
	<div class="mb" data-testid="dl-match-banner">
		<span class="mt">{words}</span>
		<button type="button" class="mo" onclick={() => (open = true)} data-testid="dl-match-open">{t('downloads.cm.match')}</button>
	</div>
{/if}

<Sheet bind:open title={t('downloads.cm.title')} description={t('downloads.cm.desc')} size="md">
	<ul class="ml" data-testid="dl-match-list">
		{#each list as m (m.id)}
			<li class="mi">
				<p class="mn" title={m.name}>{m.name}</p>
				{#if m.candidates.length}
					<div class="cs">
						{#each m.candidates as c (c.id)}
							<button type="button" class="cand" disabled={busy === m.id} onclick={() => pick(m, { candidate: c.id })} data-testid="dl-match-pick">
								<span class="cp">{#if c.poster}<img src={c.poster} alt="" loading="lazy" />{/if}</span>
								<span class="cn">{c.name}</span>
								<span class="cy">{candidateLine(c)}</span>
							</button>
						{/each}
					</div>
				{:else}
					<p class="none">{t('downloads.cm.none')}</p>
				{/if}
				<div class="ma">
					<button type="button" class="lk" disabled={busy === m.id} onclick={() => pick(m, { not_media: true })}>{t('downloads.cm.notMedia')}</button>
					<button type="button" class="lk" disabled={busy === m.id} onclick={() => pick(m, { skip: true })}>{t('downloads.cm.skip')}</button>
				</div>
			</li>
		{:else}
			<li class="none">{t('downloads.cm.every')}</li>
		{/each}
	</ul>
	{#snippet footer()}
		<Button variant="plain" onclick={matchNow} busy={running} disabled={running}>{t('downloads.cm.matchNow')}</Button>
		<Button onclick={() => (open = false)}>{t('downloads.cm.done')}</Button>
	{/snippet}
</Sheet>

<style>
	.mb { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin: 0 0 10px; padding: 8px 8px 8px 14px; border: 1px solid var(--line); border-radius: 12px; background: var(--surface-2); }
	.mt { min-width: 0; color: var(--text); font: var(--t-subhead, var(--t-callout)); font-weight: 600; }
	.mo { flex: none; min-height: 32px; padding: 0 14px; border: 0; border-radius: 999px; background: var(--accent); color: var(--surface-1); font: var(--t-subhead, var(--t-callout)); font-weight: 600; cursor: pointer; }
	.mo:focus-visible, .cand:focus-visible, .lk:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
	.ml { display: flex; flex-direction: column; gap: 18px; margin: 0; padding: 0; list-style: none; }
	.mi { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
	.mn { margin: 0; color: var(--text); font: var(--t-footnote); font-family: var(--font-mono); overflow-wrap: anywhere; }
	.cs { display: grid; grid-template-columns: repeat(auto-fill, minmax(96px, 1fr)); gap: 10px; }
	.cand { display: flex; flex-direction: column; gap: 4px; min-width: 0; padding: 6px; border: 1px solid var(--line); border-radius: 10px; background: var(--surface-1); color: var(--text); text-align: left; cursor: pointer; }
	.cand:hover { background: var(--surface-2); }
	.cand:disabled, .lk:disabled { opacity: 0.6; cursor: default; }
	.cp { display: block; aspect-ratio: 2 / 3; overflow: hidden; border-radius: 6px; background: var(--surface-3); }
	.cp img { display: block; width: 100%; height: 100%; object-fit: cover; }
	.cn { font: var(--t-footnote); font-weight: 600; overflow-wrap: anywhere; }
	.cy { color: var(--text-muted); font: var(--t-caption); }
	.ma { display: flex; gap: 14px; }
	.lk { min-height: 32px; padding: 0; border: 0; background: none; color: var(--accent-text, var(--accent)); font: var(--t-subhead, var(--t-callout)); cursor: pointer; }
	.none { margin: 0; color: var(--text-muted); font: var(--t-subhead, var(--t-callout)); }
</style>
