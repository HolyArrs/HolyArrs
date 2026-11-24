<script lang="ts">
	// Setup was skipped after the account step: a small card (Home, Settings) reopens the wizard at its first unfinished step.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { to } from '#lib/core/paths.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
</script>

{#if session.s.setupSkipped && session.s.role === 'owner'}
	<a class="finish" href={to('/welcome?first=1')} data-testid="finish-setup">
		<span class="ic" aria-hidden="true"><Icon name="sliders" size={18} /></span>
		<span class="tx"><b>{t('welcome.finishSetup')}</b><span class="small muted">{t('welcome.finishSetupAbout')}</span></span>
		<Icon name="chevronRight" size={16} />
	</a>
{/if}

<style>
	.finish { display: flex; align-items: center; gap: 12px; min-height: 44px; padding: 12px 16px; border-radius: 14px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); color: var(--text); text-decoration: none; }
	.finish:hover { background: var(--surface-2); }
	.finish:focus-visible { outline: 2px solid var(--focus, var(--accent)); outline-offset: 2px; }
	.ic { display: grid; place-items: center; width: 36px; height: 36px; border-radius: 10px; background: var(--surface-2); flex: none; }
	.tx { flex: 1; min-width: 0; display: grid; gap: 2px; }
</style>
