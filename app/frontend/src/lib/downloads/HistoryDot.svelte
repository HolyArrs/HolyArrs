<script lang="ts">
	import type { Verb } from './history-kinds.ts';

	// The Downloads row's state mark: a coloured dot and its word.
	let { verb, detail = '' }: { verb: Verb; detail?: string } = $props();
</script>

<span class="st {verb.tone}" data-tone={verb.tone}><i aria-hidden="true"></i><span class="sw">{verb.word}{#if detail}<span class="sd">{` · ${detail}`}</span>{/if}</span></span>

<style>
	.st { display: inline-flex; align-items: center; gap: 7px; max-width: 100%; min-width: 0; color: var(--text); font-weight: 500; }
	.sw { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.sd { color: var(--text-muted); font-weight: 400; }
	i { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--info); }
	.fail { color: var(--fail-text); }
	.fail i { background: var(--fail); }
	.ok i { background: var(--ok); }
	.wait i { background: var(--wait); }
	.idle { color: var(--text-muted); }
	.idle i { background: var(--text-faint); }
</style>
