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
		footer,
		dotPanel
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
		/** The list a row's red dot opens. */
		dotPanel?: Snippet<[() => void]>;
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
			// Measure against the list: the row's own offsetTop is relative to its positioned <li>, which is always 0.
			const top = row.getBoundingClientRect().top - list.getBoundingClientRect().top;
			pill = { top, height: row.offsetHeight, on: true, still: first };
		});
		return () => cancelAnimationFrame(raf);
	});
	const key = (s: NavSection, i: NavItem) => `${s.id}:${i.href}:${i.label}`;
</script>

<aside {id} class="ui-sidebar" class:collapsed class:overlay aria-label={t('frame.sidebar.label')}>
	<div class="top">
		<a class="brand" href={brandHref} aria-label={t('frame.sidebar.home')} onclick={onpick}>
			<!-- Eleven+: full wordmark when open, the symbol alone when collapsed. -->
			<img class="mark" src={markSrc} alt="" width="28" height="28" /><img class="word" src={markSrc.replace(/mark-color(-on-dark)?\.svg/, (_m, d) => (d ? 'wordmark-on-dark.svg' : 'wordmark.svg'))} alt="Eleven+" width="133" height="32" />
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
								<SidebarItem href={link(item.href)} label={item.label} icon={item.icon} current={currentHref === item.href} {collapsed} badge={item.badge} badgeLabel={item.badgeLabel} ring={item.ring} dot={item.dot} {dotPanel} badgeTestid={item.href === '/downloads' ? 'nav-decide' : undefined} onclick={onpick} />
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
	.brand { display: flex; align-items: center; gap: 10px; min-width: 0; height: 48px; border-radius: 8px; font: var(--t-headline); font-size: 17px; letter-spacing: -0.02em; white-space: nowrap; overflow: hidden; }
	.brand img { flex: none; }
	.brand:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.word { transition: opacity var(--dur-fast) var(--ease-out) var(--label-delay); }
	.collapsed .word { opacity: 0; }
	.collapsed .top { flex-direction: column; align-items: center; padding: 0; gap: 4px; }
	.collapsed .brand { width: 40px; justify-content: center; }
	.collapsed .tog { display: none; }
	.collapsed .brand .word { display: none; }
	.brand .mark { display: none; }
	.collapsed .brand .mark { display: block; }
	.brand .word { height: 25px; width: auto; padding-left: 4px; }
	.scroll { flex: 1; min-height: 0; overflow-y: auto; overflow-x: hidden; margin: 0 -4px; padding: 0 4px; scrollbar-width: none; } /* Eleven+: no visible scrollbar */
	.scroll::-webkit-scrollbar { display: none; }
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
	li { position: relative; }
	.foot { margin-top: 8px; padding-top: 8px; border-top: 1px solid var(--line); }
	@media (max-width: 767px) { .ui-sidebar { display: none; } }

	/* ===== Eleven+ refined sidebar (open) ===== */
	.ui-sidebar { padding: 14px 12px 12px; }
	.top { margin-bottom: 10px; }
	.brand .word { height: 32px; }
	.sec { height: auto; padding: 16px 12px 6px; color: var(--text-faint); font: 600 12.5px/1.2 var(--font-ui); letter-spacing: 0; }
	.section + .section { margin-top: 4px; }
	.pill { border-radius: 10px; background: var(--accent-soft); }
	/* Thin accent marker on the edge of the selected row */
	.pill::before { content: ''; position: absolute; inset-inline-start: -12px; top: 9px; bottom: 9px; width: 3px; border-radius: 0 3px 3px 0; background: var(--accent); transition: opacity var(--dur-fast) var(--ease-out); }
	.foot { border-top: 0; padding-top: 4px; }

	/* ===== Eleven+ attached rail (collapsed) ===== */
	.collapsed .top { margin-bottom: 12px; }
	.collapsed .brand { width: 48px; height: 44px; }
	.collapsed .brand .mark { width: 30px; height: 30px; translate: 16% 0; } /* centre the II, not the II+ */
	.collapsed .scroll { margin: 0; padding: 0; scrollbar-width: none; }
	.collapsed .pill { border-radius: 14px; }
	.collapsed .pill::before { opacity: 0; }
	.collapsed .sec { height: 14px; padding: 0; }
	.collapsed .div { left: 14px; right: 14px; top: 6px; }
	.collapsed .foot { display: flex; justify-content: center; }
</style>
