<script lang="ts">
	// A plugin's recent calls to HolyArrs: what it was allowed, what was refused and when it went over its limits.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { api } from '#lib/core/api.ts';
	import { ago } from '#lib/core/format.ts';

	interface Call {
		at: string;
		method: string;
		path: string;
		outcome: 'allowed' | 'refused' | 'limited';
	}
	let { id }: { id: string } = $props();
	let data = $state<{ calls: Call[]; refused: number; limited: number } | null>(null);
	$effect(() => {
		const want = id;
		void api
			.get<{ calls: Call[]; refused: number; limited: number }>(`/plugins/${encodeURIComponent(want)}/activity`)
			.then((d) => (data = d))
			.catch(() => (data = null));
	});
	const word = (o: Call['outcome']) => (o === 'allowed' ? t('plugins.callAllowed') : o === 'refused' ? t('plugins.callRefused') : t('plugins.callLimited'));
</script>

{#if data?.calls?.length}
	<details class="activity" data-testid="plugin-activity">
		<summary class="small">{t('plugins.recentCalls', { refused: data.refused, limited: data.limited })}</summary>
		<ul class="xs">
			{#each data.calls.slice(0, 30) as c, i (i)}
				<li class:bad={c.outcome !== 'allowed'}><span class="mono">{c.method} {c.path}</span><span class="muted">{word(c.outcome)} · <time datetime={c.at}>{ago(c.at)}</time></span></li>
			{/each}
		</ul>
	</details>
{/if}

<style>
	.activity { margin-top: var(--s-3); }
	ul { list-style: none; margin: 6px 0 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }
	li { display: flex; flex-direction: column; min-width: 0; }
	.mono { font-family: var(--font-mono, monospace); overflow-wrap: anywhere; }
	.bad .mono { color: var(--fail-text); }
	summary { cursor: pointer; min-height: 32px; display: flex; align-items: center; color: var(--text-muted); font-size: 12px; font-weight: 600; }
</style>
