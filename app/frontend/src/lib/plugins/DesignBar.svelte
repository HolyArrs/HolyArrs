<script lang="ts">
	// Lab-only "Design preview" bar: Scenarios dresses the list as one plugin state at a time.
	// It shows only in the dev server (import.meta.env.DEV), never in a release build, so its words are not translated.
	import { scenarioNames, type Scenario } from './scenarios.svelte.ts';
	let { scenario = $bindable('') }: { scenario?: Scenario } = $props();
</script>

<div class="dbar" role="toolbar" aria-label="Design preview" data-testid="plugins-design-bar">
	<span class="lbl">Design preview</span>
	<label class="grp scen"><span class="sr-only">Scenarios</span>
		<select bind:value={scenario} data-testid="plugins-scenario" aria-label="Scenarios">
			{#each scenarioNames as [v, n] (v)}<option value={v}>{v ? n : 'Scenarios: live data'}</option>{/each}
		</select>
	</label>
</div>

<style>
	.dbar { position: fixed; z-index: 50; left: 50%; bottom: calc(16px + env(safe-area-inset-bottom)); transform: translateX(-50%); display: flex; flex-wrap: wrap; justify-content: center; align-items: center; gap: 6px; max-width: calc(100vw - 32px); padding: 6px 8px; border-radius: 999px; border: 1px solid var(--line-strong); background: color-mix(in srgb, var(--surface-1) 85%, transparent); -webkit-backdrop-filter: blur(18px); backdrop-filter: blur(18px); box-shadow: 0 10px 30px -10px rgb(0 0 0 / 0.5); font-size: 12px; }
	.lbl { padding-inline: 6px; color: var(--text-muted); font-weight: 600; white-space: nowrap; }
	.grp { display: flex; flex-wrap: wrap; justify-content: center; gap: 2px; padding: 2px; border-radius: 999px; background: var(--surface-2); }
	.scen select { height: 28px; padding: 0 10px; border: 0; border-radius: 999px; background: none; color: var(--text); font: inherit; font-weight: 600; cursor: pointer; }
	@media (max-width: 640px) { .dbar { bottom: calc(76px + env(safe-area-inset-bottom)); } .lbl { width: 100%; text-align: center; } .grp { border-radius: 14px; } }
</style>
