<script lang="ts">
	import { roundArt, stateWord, subline, suggestHref, type SuggestAnswer } from '../live-search.ts';
	import { appPicture } from '../music/music-pages.ts';
	import Icon, { type IconName } from '../Icon.svelte';
	// No art yet: the row shows its kind's glyph, the same stand-in the posters use.
	const glyph: Record<string, IconName> = { movie: 'film', show: 'tv', person: 'user', artist: 'user', album: 'music', song: 'music' };

	// The live search dropdown (music plan Phase 4): a glass panel under the
	// field. Word completions first, then rich rows with 44 px art (round for
	// artists and people), a "Type · detail" subline and the state chip.
	// `active` indexes completions then rows; the field owns the keys.
	let { answer, active, id, onpick, onword }: { answer: SuggestAnswer; active: number; id: string; onpick: (href: string) => void; onword: (w: string) => void } = $props();
	const words = $derived(answer.completions);
</script>

<div class="glass" {id} role="listbox" aria-label="Suggestions" data-testid="search-suggest">
	{#each words as w, i (w)}
		<div id="{id}-{i}" role="option" tabindex="-1" aria-selected={active === i} class="word" class:on={active === i} data-testid="suggest-word" onpointerdown={(e) => e.preventDefault()} onclick={() => onword(w)} onkeydown={(e) => e.key === 'Enter' && onword(w)}>{w}</div>
	{/each}
	{#if words.length && answer.items.length}<div class="rule" role="presentation"></div>{/if}
	{#each answer.items as r, j (r.kind + r.name + (r.sub ?? '') + j)}
		{@const i = words.length + j}
		<a id="{id}-{i}" role="option" aria-selected={active === i} class="row" class:on={active === i} href={suggestHref(r)} data-testid="suggest-row" data-kind={r.kind} tabindex="-1" onpointerdown={(e) => e.preventDefault()} onclick={(e) => { e.preventDefault(); onpick(suggestHref(r)); }}>
			<span class="art" class:round={roundArt(r.kind)}>{#if r.art}<img src={appPicture(r.art)} alt="" loading="lazy" decoding="async" />{:else}<span class="ph" aria-hidden="true"><Icon name={glyph[r.kind] ?? 'music'} size={18} /></span>{/if}</span>
			<span class="text"><span class="name">{r.name}</span><span class="sub">{subline(r)}</span></span>
			{#if r.state}<span class="chip {r.state}">{stateWord[r.state]}</span>{/if}
		</a>
	{/each}
</div>

<style>
	.glass { position: absolute; z-index: 40; top: calc(100% + 8px); left: 0; right: 0; max-height: min(70vh, 640px); overflow-y: auto; padding: 6px; border-radius: 12px; background: color-mix(in srgb, var(--surface-2) 94%, transparent); -webkit-backdrop-filter: blur(60px) saturate(2.2); backdrop-filter: blur(60px) saturate(2.2); box-shadow: inset 0 0 0 1px rgb(255 255 255 / 0.12), 0 8px 40px rgb(0 0 0 / 0.45); text-align: left; }
	/* On a phone the field sits in a sticky strip where the blur does not
	   reach the page, so the results below showed through; use the solid fill. */
	@media (max-width: 767px) { .glass { background: var(--surface-2); } }
	.word { display: flex; align-items: center; min-height: 32px; padding: 0 8px; border-radius: 6px; font: 600 14px/1.2 var(--font-ui); color: var(--text); cursor: pointer; }
	.rule { height: 1px; margin: 6px 8px; background: color-mix(in srgb, var(--text) 12%, transparent); }
	.row { display: flex; align-items: center; gap: 12px; min-height: 56px; padding: 6px 8px; border-radius: 6px; color: var(--text); text-decoration: none; }
	@media (hover: hover) { .word:not(.on):hover, .row:not(.on):hover { background: color-mix(in srgb, var(--text) 7%, transparent); } }
	.on { background: var(--accent-fill); color: var(--text-on-red, #fff); }
	.on .sub { color: inherit; opacity: 1; }
	.art { flex: none; width: 44px; height: 44px; border-radius: 4px; overflow: hidden; background: var(--surface-3); }
	.art.round { border-radius: 999px; }
	.art img { width: 100%; height: 100%; object-fit: cover; }
	.ph { width: 100%; height: 100%; display: grid; place-items: center; color: var(--text-muted); }
	.text { display: grid; min-width: 0; flex: 1; }
	.name { font: 600 14px/1.3 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.sub { font-size: 13px; color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.chip { flex: none; padding: 2px 8px; border-radius: 999px; font: 600 11px/1.5 var(--font-ui); background: color-mix(in srgb, var(--text) 12%, transparent); color: var(--text); }
	.chip.have { background: color-mix(in srgb, var(--ok, #3fb950) 24%, transparent); }
	.chip.asked { background: color-mix(in srgb, var(--accent) 26%, transparent); }
	.on .chip { background: rgb(255 255 255 / 0.22); color: inherit; }
</style>
