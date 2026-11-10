<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import IconButton from '#lib/ui/IconButton.svelte';
	import SidebarItem from './SidebarItem.svelte';
	import type { NavItem, NavSection } from './nav.ts';
	import type { Snippet } from 'svelte';

	// The sidebar: brand, sections and account footer. One selected fill slides between rows.
	let {
		id = 'sidebar',
		sections,
		collapsed,
		overlay = false,
		currentHref,
		link,
		brandHref,
		markSrc,
		ontoggle,
		onpick,
		toggle = $bindable(),
		footer
	}: {
		id?: string;
		sections: NavSection[];
		/** Icons only (72px). */
		collapsed: boolean;
		/** Shown over the content (tablet). */
		overlay?: boolean;
		currentHref: string | null;
		/** Turns an app path into a link (base path). */
		link: (href: string) => string;
		brandHref: string;
		markSrc: string;
		ontoggle: () => void;
		onpick?: () => void;
		toggle?: HTMLElement;
		footer?: Snippet;
	} = $props();

	let list = $state<HTMLElement>();
	let pill = $state<{ top: number; height: number; on: boolean; still: boolean }>({ top: 0, height: 0, on: false, still: true });

	// Move the selected fill to the current row after each change.
	$effect(() => {
		void currentHref;
		void collapsed;
		void sections;
		if (!list) return;
		const raf = requestAnimationFrame(() => {
			const row = list?.querySelector<HTMLElement>('[aria-current="page"]');
			if (!row || !list) return void (pill = { ...pill, on: false });
			const first = !pill.on;
			pill = { top: row.offsetTop, height: row.offsetHeight, on: true, still: first };
		});
		return () => cancelAnimationFrame(raf);
	});
	const key = (s: NavSection, i: NavItem) => `${s.id}:${i.href}:${i.label}`;
</script>

<aside {id} class="ui-sidebar" class:collapsed class:overlay aria-label={t('frame.sidebar.label')}>
	<div class="top">
		<a class="brand" href={brandHref} aria-label={t('frame.sidebar.home')} onclick={onpick}>
			<img src={markSrc} alt="" width="28" height="28" /><span class="word">HolyArrs</span>
		</a>
		<span class="tog">
			<IconButton bind:node={toggle} icon="sidebar" label={collapsed ? t('frame.sidebar.show') : t('frame.sidebar.hide')} aria-expanded={!collapsed} aria-controls={id} tipSide={collapsed ? 'right' : 'below'} onclick={ontoggle} />
		</span>
	</div>
	<nav aria-label={t('frame.tabs.main')} class="scroll">
		<div class="list" bind:this={list}>
			<span class="pill" class:on={pill.on} class:still={pill.still} style="transform: translateY({pill.top}px); height: {pill.height}px" aria-hidden="true"></span>
			{#each sections as s (s.id)}
				<div class="section">
					{#if s.label}<h2 class="sec"><span class="t">{s.label}</span><span class="div" aria-hidden="true"></span></h2>{/if}
					<ul>
						{#each s.items as item (key(s, item))}
							<li>
								<SidebarItem href={link(item.href)} label={item.label} icon={item.icon} current={currentHref === item.href} {collapsed} badge={item.badge} badgeLabel={item.badgeLabel} ring={item.ring} dot={item.dot} badgeTestid={item.href === '/downloads' ? 'nav-decide' : undefined} onclick={onpick} />
							</li>
						{/each}
					</ul>
				</div>
			{/each}
		</div>
	</nav>
	{#if footer}<div class="foot">{@render footer()}</div>{/if}
</aside>

<style>
	.ui-sidebar {
		--w: var(--sidebar-expanded);
		position: fixed; z-index: 40; inset-block: 0; inset-inline: 0 auto; width: var(--w); display: flex; flex-direction: column;
		padding: 10px 10px 10px; background: var(--sidebar-bg); border-inline-end: 1px solid var(--line);
		backdrop-filter: saturate(180%) blur(20px); -webkit-backdrop-filter: saturate(180%) blur(20px);
		/* Expand: width grows first, labels fade in after. */
		transition: width var(--dur) var(--ease-out) 0ms, box-shadow var(--dur) var(--ease-out);
		--label-delay: var(--dur-fast);
	}
	.ui-sidebar.collapsed {
		--w: var(--sidebar-collapsed);
		/* Collapse: labels fade out first, then width shrinks. */
		transition: width var(--dur) var(--ease-out) var(--dur-fast), box-shadow var(--dur) var(--ease-out);
		--label-delay: 0ms;
	}
	@supports not (backdrop-filter: blur(1px)) { .ui-sidebar { background: var(--sidebar-solid); } }
	.overlay { box-shadow: var(--shadow-modal); background: var(--sidebar-solid); }
	.top { display: flex; align-items: center; justify-content: space-between; gap: 6px; min-height: 44px; padding: 0 2px 0 8px; margin-bottom: 6px; }
	.brand { display: flex; align-items: center; gap: 10px; min-width: 0; height: 40px; border-radius: 8px; font: var(--t-headline); font-size: 17px; letter-spacing: -0.02em; white-space: nowrap; overflow: hidden; }
	.brand img { flex: none; }
	.brand:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.word { transition: opacity var(--dur-fast) var(--ease-out) var(--label-delay); }
	.collapsed .word { opacity: 0; }
	.collapsed .top { flex-direction: column; align-items: center; padding: 0; gap: 4px; }
	.collapsed .brand { width: 40px; justify-content: center; }
	.collapsed .tog { display: none; }
	.collapsed .brand .word { display: none; }
	.scroll { flex: 1; min-height: 0; overflow-y: auto; overflow-x: hidden; margin: 0 -4px; padding: 0 4px; scrollbar-width: thin; }
	.list { position: relative; }
	.pill { position: absolute; z-index: 0; left: 0; right: 0; top: 0; border-radius: 9px; background: var(--surface-3); opacity: 0; transition: transform var(--dur) var(--ease-spring), height var(--dur) var(--ease-spring), opacity var(--dur-fast) var(--ease-out); }
	.pill.on { opacity: 1; }
	.pill.still { transition: opacity var(--dur-fast) var(--ease-out); }
	.section + .section { margin-top: 14px; }
	.sec { height: 26px; padding: 6px 12px 0; color: var(--text-muted); font: 600 12px/1.3 var(--font-ui); white-space: nowrap; overflow: hidden; position: relative; }
	.sec .t { transition: opacity var(--dur-fast) var(--ease-out) var(--label-delay); }
	.collapsed .sec .t { opacity: 0; }
	.div { position: absolute; left: 14px; right: 14px; top: 13px; height: 1px; background: var(--line-strong); opacity: 0; transition: opacity var(--dur-fast) var(--ease-out); }
	.collapsed .div { opacity: 1; }
	.collapsed .section + .section { margin-top: 6px; }
	ul { display: flex; flex-direction: column; gap: 2px; }
	.foot { margin-top: 8px; padding-top: 8px; border-top: 1px solid var(--line); }
	@media (max-width: 767px) { .ui-sidebar { display: none; } }
</style>
