<script lang="ts">
	// "What comes over" in setup: a grid of small tiles, the same before and after the check, so the panel never grows.
	// Each tile's value shows a quiet tick first, a spinner while counting, then the real amount.
	import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import { bringsItems, hasCount, type CountItem } from './brings-counts.ts';

	// apps: Seerr's own Radarr and Sonarr once read, each a tag (green read, amber needs a look).
	let { lines, kind, name, counts, counting, busy, apps = [] }: { lines: string[]; kind: string; name: string; counts: Record<string, number> | null; counting: boolean; busy: boolean; apps?: { label: string; ok: boolean }[] } = $props();

	const rows = $derived(lines.map((l) => ({ line: l, label: t(l, { name }), items: bringsItems(l, kind, counts, appLocale()), wait: (busy || counting) && hasCount(l) })));
</script>

{#snippet value(r: { line: string; items: CountItem[]; wait: boolean })}
	{#if r.wait || (busy && r.line === 'migrate.brings.itsArrs')}<span class="spin" aria-hidden="true"></span>
	{:else if r.line === 'migrate.brings.itsArrs' && apps.length}{#each apps as a (a.label)}<span class="chip" class:warn={!a.ok} data-testid="brings-app">{a.label}</span>{/each}
	{:else if r.items.length}{#each r.items as i, k (i.key)}{#if k}<span class="dot">·</span>{/if}<span class="v" class:zero={i.zero}><b>{i.num}</b>{#if i.after}{' '}{i.after}{/if}</span>{/each}
	{:else}<Icon name="check" size={14} />{/if}
{/snippet}

<div class="tiles" data-testid="brings-counts">
	{#each rows as r (r.line)}<div class="tile"><span class="name">{r.label}</span><span class="val">{@render value(r)}</span></div>{/each}
</div>

<style>
	.tiles { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 6px; text-align: start; font: 400 14px/1.35 var(--font-ui); }
	.tile { display: flex; flex-direction: column; gap: 2px; padding: 7px 10px; min-height: 48px; box-sizing: border-box; border: 1px solid var(--g-line, rgb(255 255 255 / 0.12)); border-radius: 10px; background: var(--g-bg, rgb(255 255 255 / 0.05)); min-width: 0; }
	.name { font-size: 12px; opacity: 0.7; overflow: hidden; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow-wrap: anywhere; }
	.val { min-height: 20px; display: flex; flex-wrap: wrap; align-items: center; gap: 4px; }
	.val :global(svg) { color: var(--ok-text); flex: none; }
	b { font-weight: 700; font-variant-numeric: tabular-nums; }
	.zero { opacity: 0.45; }
	.dot { opacity: 0.4; }
	.chip { display: inline-flex; align-items: center; max-width: 100%; padding: 1px 8px; border-radius: 999px; font-size: 12px; font-weight: 600; overflow-wrap: anywhere; color: var(--ok-text); background: color-mix(in srgb, var(--ok-text) 16%, transparent); }
	.chip.warn { color: var(--warn-text, #f5a524); background: color-mix(in srgb, var(--warn-text, #f5a524) 16%, transparent); }
	.spin { flex: none; width: 14px; height: 14px; border-radius: 50%; border: 2px solid rgb(var(--ink-rgb, 255 255 255) / 0.2); border-top-color: var(--ok-text); animation: bp-spin 0.8s linear infinite; }
	@keyframes bp-spin { to { transform: rotate(360deg); } }
	@media (prefers-reduced-motion: reduce) { .spin { animation: none; border-color: rgb(52 199 89 / 0.5); } }
	@media (max-width: 600px) { .tiles { grid-template-columns: 1fr; } }
</style>
