<script lang="ts" module>
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import type { IconName } from '#lib/ui/Icon.svelte';
	export type Tab = { label: string; icon: IconName; href?: string; current?: boolean; badge?: number; dot?: boolean; onclick?: () => void };
</script>

<script lang="ts">
	import Icon from '#lib/ui/Icon.svelte';
	import { press } from '#lib/core/motion.ts';

	// Phone bottom tab bar: translucent and safe-area aware. Items are links, or buttons (with onclick) that open a sheet such as More.
	let { items, label }: { items: Tab[]; label?: string } = $props();
</script>

<nav class="ui-tabbar tabbar" aria-label={label ?? t('frame.tabs.main')}>
	{#each items as tab (tab.label)}
		{#if tab.href}
			<a class="tab" class:active={tab.current} href={tab.href} aria-current={tab.current ? 'page' : undefined} use:press={'round'}>
				<span class="ic"><Icon name={tab.icon} size={22} />{#if tab.badge}<b class="count" aria-hidden="true">{tab.badge}</b>{/if}</span><span>{tab.label}</span>
			</a>
		{:else}
			<button class="tab" class:active={tab.current} type="button" aria-haspopup="dialog" aria-current={tab.current ? 'page' : undefined} onclick={tab.onclick} use:press={'round'}>
				<span class="ic"><Icon name={tab.icon} size={22} />{#if tab.dot}<i class="dot" aria-hidden="true"></i>{/if}</span><span>{tab.label}</span>{#if tab.dot}<span class="sr-only">{t('frame.tabs.attention')}</span>{/if}
			</button>
		{/if}
	{/each}
</nav>

<style>
	.ui-tabbar {
		position: fixed; z-index: 35; inset: auto 0 0; display: none; padding: 4px 6px env(safe-area-inset-bottom);
		background: var(--sidebar-bg); border-top: 1px solid var(--line);
		backdrop-filter: saturate(180%) blur(20px); -webkit-backdrop-filter: saturate(180%) blur(20px);
	}
	@media (max-width: 767px) { .ui-tabbar { display: flex; } }
	.tab { flex: 1; min-width: 0; min-height: var(--tabbar, 56px); display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 3px; border-radius: 10px; color: var(--text); font: 500 11px/1.1 var(--font-ui); }
	.tab span:last-child { max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	/* The bar is see-through, so the page behind can darken it: the active label keeps the text colour (4.5:1 on any page) and only its icon is red (3:1 is enough for an icon). */
	.tab.active { color: var(--text); font-weight: 600; }
	.tab.active :global(.icon) { color: var(--accent-text); }
	.tab:not(.active) :global(.icon) { color: var(--text-muted); }
	.tab:focus-visible { outline: none; box-shadow: inset 0 0 0 2px var(--accent); }
	.ic { position: relative; display: grid; place-items: center; }
	.count { position: absolute; top: -4px; left: 14px; min-width: 16px; height: 16px; padding: 0 4px; display: grid; place-items: center; border-radius: 999px; background: var(--accent-fill); color: var(--text-on-red); font: 600 10px/1 var(--font-ui); }
	.dot { position: absolute; top: -1px; right: -3px; width: 7px; height: 7px; border-radius: 50%; background: var(--accent); box-shadow: 0 0 0 2px var(--bg); }
</style>
