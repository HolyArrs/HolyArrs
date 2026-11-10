<script lang="ts">
	// One translated line with bold, code and link markers. Text is always escaped; links come from `links`.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { fillWords, splitWords } from './words.ts';

	let {
		key,
		vars,
		links = {},
		linkClass,
		codeClass
	}: {
		key: string;
		vars?: Record<string, string | number>;
		/** Marker name to address, e.g. { a: '/settings/danger' }. */
		links?: Record<string, string>;
		linkClass?: string;
		codeClass?: string;
	} = $props();

	const parts = $derived(splitWords(t(key)).map((p) => ({ tag: p.tag, text: fillWords(p.text, vars) })));
</script>

{#each parts as p, i (i)}{#if p.tag === 'b' || p.tag === 'strong'}<b>{p.text}</b>{:else if p.tag === 'em'}<em>{p.text}</em>{:else if p.tag === 'code'}<code class={codeClass}>{p.text}</code>{:else if p.tag === 'mono'}<span class="mono">{p.text}</span>{:else if p.tag && links[p.tag] !== undefined}<a class={linkClass} href={links[p.tag]}>{p.text}</a>{:else}{p.text}{/if}{/each}
