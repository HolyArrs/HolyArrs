<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Settings home: five group cards in a Bento grid over the blurred wall of this Library's posters. Each card shows what lives inside it and opens on its first page (a zoom on wider screens, a push on phones). My account is in the account menu.
	import Icon from '#lib/ui/Icon.svelte';
	import SettingsTile from '#lib/ui/SettingsTile.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import { entryLabel, searchSettings, settingIndex, type SettingEntry } from './(home)/settings-search.ts';
	import GroupArt from './(home)/GroupArt.svelte';
	import { homeArt } from './(home)/art.svelte.ts';
	import { cancelOpen, push, settingsNav, zoomBack, zoomOpen, clearFlight } from './(home)/zoom.ts';
	import { partsNow, offPartName } from '#lib/mediafiles/index.ts';
	import { settingsNow, groupState, stateWords, type GroupState } from '#lib/alerts/index.ts';
	import { page } from '$app/state';
	import { goto, preloadData } from '$app/navigation';
	import { onMount, tick } from 'svelte';
	import { to } from '#lib/core/paths.ts';
	import { grouped, groups as allGroups, sections as all, visible } from '#lib/core/settings.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { settingsFind } from '#lib/core/settings-find.svelte.ts';

	const sections = $derived(visible(all, session.can).filter((section) => !offPartName(partsNow.answer, section.href)));
	const groups = $derived(grouped(sections).filter((g) => g.group.id !== 'account'));
	const open = $derived(new Set(sections.map((s) => s.href)));
	const sectionOf = (href: string) => all.find((s) => s.href === href);
	const groupLabel = (href: string) => allGroups.find((g) => g.id === sectionOf(href)?.group)?.label ?? '';

	// Pages are results too, so "subtitles" finds the page as well as its fields.
	const index = $derived<SettingEntry[]>([...sections.map((s) => ({ label: s.label, href: s.href, anchor: '', words: s.about })), ...settingIndex]);

	// The top bar's "Search settings" (or the field below on a phone) hands over ?q= too.
	const q0 = page.url.searchParams.get('q');
	if (q0) settingsFind.term = q0;
	const query = $derived(settingsFind.term);
	const results = $derived(searchSettings(query, (h) => open.has(h), groupLabel, index).slice(0, 30));
	// Enter in the top bar field opens the first match.
	$effect(() => {
		settingsFind.first = results[0] ? link(results[0]) : '';
	});
	const answer = $derived(settingsNow.answer);

	const tone = (s: GroupState) => (s === 'attention' ? 'fail' : s === 'unset' ? 'wait' : s === 'ok' ? 'ok' : '');
	const link = (r: SettingEntry) => to(r.href) + (r.anchor ? `#${r.anchor}` : '');

	let home = $state<HTMLElement>();

	// The next page is fetched as soon as the pointer or focus reaches a card, so by the time the zoom lands it is ready to mount under the sheet.
	const loads = new Map<string, Promise<unknown>>();
	function warm(href: string): Promise<unknown> {
		let p = loads.get(href);
		if (!p) {
			p = preloadData(href).catch(() => null);
			loads.set(href, p);
		}
		return p;
	}

	function openGroup(e: MouseEvent, id: string, href: string) {
		if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
		settingsNav.move = 'open';
		settingsNav.group = id;
		const landed = zoomOpen(e.currentTarget as HTMLElement, id);
		if (!landed) return;
		e.preventDefault();
		const ready = warm(href);
		// Load the page only once the sheet fills the screen: it mounts unseen, then the sheet clears.
		void landed.then(async (ok) => {
			if (!ok) return;
			await ready;
			await goto(href);
		});
	}

	// Escape while a card is still growing turns it back.
	function key(e: KeyboardEvent) {
		if (e.key === 'Escape' && cancelOpen()) {
			settingsNav.move = null;
			e.preventDefault();
		}
	}

	onMount(() => {
		const back = settingsNav.move === 'back' ? settingsNav.group : null;
		settingsNav.move = null;
		if (!back) {
			clearFlight();
			return;
		}
		void tick().then(() => {
			const card = home?.querySelector<HTMLElement>(`.sx-card[data-group="${back}"]`);
			if (home && !push(home, true) && card) zoomBack(card, back);
			card?.focus({ preventScroll: true });
		});
	});
</script>

<svelte:head><title>{t('settings.home.title')} · HolyArrs</title></svelte:head>
<svelte:window onkeydown={key} />

<div class="sx-home" bind:this={home}>
	<div class="sx-head">
		<h1 class="sr-only">{t('settings.home.title')}</h1>
		<div class="find">
			<Icon name="search" size={17} />
			<label class="sr-only" for="settings-find">{t('settings.home.search')}</label>
			<input id="settings-find" class="input" type="search" placeholder={t('settings.home.search')} autocomplete="off" enterkeyhint="search" bind:value={settingsFind.term} aria-controls={query.trim() ? 'settings-results' : undefined} />
		</div>
	</div>

	{#if query.trim()}
		{#if results.length}
			<ul class="results" id="settings-results" aria-label={t('settings.home.matching')} data-testid="setting-results">
				{#each results as r (r.href + '#' + r.anchor)}
					{@const s = sectionOf(r.href)}
					<li>
						<a class="row-link" href={link(r)} data-testid="setting-result" onclick={() => (settingsFind.term = '')}>
							<SettingsTile icon={s?.icon ?? 'settings'} tint={s?.tint ?? 'gray'} />
							<span class="setting-copy">
								<strong>{entryLabel(r)}</strong>
								<span>{r.anchor ? `${groupLabel(r.href)} › ${s?.label ?? ''}${r.advanced ? ' › ' + t('settings.home.advanced') : ''}` : t('settings.home.page', { group: groupLabel(r.href) })}</span>
							</span>
							<Icon name="chevronRight" size={16} />
						</a>
					</li>
				{/each}
			</ul>
		{:else}
			<EmptyState compact icon="search" title={t('settings.home.noMatch')} text={t('settings.home.noMatchText')} action={{ label: t('settings.home.clear'), onclick: () => (settingsFind.term = '') }} />
		{/if}
	{:else}
		<!-- Problems show in the alerts panel and as the group's chip on its card, not as a banner here. -->
		<div class="bento" data-count={groups.length}>
			{#each groups as { group, items } (group.id)}
				{@const gs = answer ? groupState(answer, items.map((i) => i.href)) : null}
				{@const href = to(items[0].href)}
				<a class="sx-card" {href} data-group={group.id} data-testid="settings-group" aria-describedby="line-{group.id}" onclick={(e) => openGroup(e, group.id, href)} onpointerenter={() => warm(href)} onpointerdown={() => warm(href)} onfocus={() => warm(href)}>
					<span class="sx-face">
						<span class="lift" aria-hidden="true"></span>
						<span class="words">
							<h2 class="nm">{group.label}</h2>
							<span class="pp" id="line-{group.id}">{group.line}</span>
						</span>
						{#if gs && (gs.state === 'attention' || gs.state === 'unset') && stateWords(gs)}<span class="chip {tone(gs.state)}" data-testid="group-status">{stateWords(gs)}</span>{/if}
						<span class="art-hold"><GroupArt group={group.id} posters={homeArt.posters} people={homeArt.people} /></span>
					</span>
				</a>
			{/each}
		</div>
	{/if}
</div>

<style>
	/* Desktop: a compact landing, at most 640px tall and 1180px wide, centred. On short windows it shrinks to fit (main's 24px top and 64px bottom padding taken out), so it never scrolls. */
	/* Desktop: the grid sits in the middle of the window, across and down, and grows with it (no scroll). */
	.sx-home { display: flex; flex-direction: column; justify-content: center; width: 100%; max-width: min(1480px, calc((100dvh - var(--topbar) - 88px) * 1.9)); margin-inline: auto; margin-bottom: -40px; height: calc(100dvh - var(--topbar) - 48px); min-height: 420px; }
	.lede { margin: -14px 0 22px; color: var(--text-muted); font: var(--t-body); }
	/* The top bar holds "Search settings" on tablets and desktops; phones have no top bar, so the field lives here. */
	.find { position: relative; display: none; align-items: center; margin: -8px 0 18px; }
	.find > :global(.icon) { position: absolute; left: 14px; color: var(--text-muted); pointer-events: none; }
	.find .input { width: 100%; min-height: 44px; padding-left: 40px; border-radius: var(--r-pill); background: color-mix(in srgb, var(--surface-2) 80%, transparent); border-color: transparent; font: var(--t-body); }
	@media (max-width: 767px) { .find { display: flex; } }

	.fix { display: inline-flex; align-items: center; gap: 2px; min-height: 44px; padding: 0 6px 0 12px; color: var(--accent-text); font: var(--t-callout); font-weight: 600; }

	/* Bento: Library tall on the left, four around it. */
	.bento { flex: 0 1 auto; height: min(820px, 100%); display: grid; grid-template-columns: 1.35fr 1fr 1fr; grid-template-rows: minmax(150px, 1fr) minmax(150px, 1fr); gap: 14px; min-height: 0; }
	.sx-card { position: relative; display: block; min-width: 0; border-radius: var(--r-panel); color: var(--text); text-decoration: none; -webkit-tap-highlight-color: transparent; }
	.sx-card[data-group='library'] { grid-row: span 2; }
	.sx-card:focus-visible { outline: none; }
	.sx-card:focus-visible .sx-face { box-shadow: var(--focus-ring), 0 1px 2px rgb(0 0 0 / 0.12); }
	/* The link box never moves (so the pointer never slips off its edge and the hover never flickers); the face inside it lifts. */
	.sx-face {
		position: absolute; inset: 0; display: block; overflow: hidden; padding: 24px; border-radius: inherit;
		/* Status-plate glass: see-through, frosted, with a lit top edge. */
		background: color-mix(in srgb, var(--surface-1) 52%, transparent);
		-webkit-backdrop-filter: blur(28px) saturate(1.6); backdrop-filter: blur(28px) saturate(1.6);
		box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.18), 0 0 0 1px color-mix(in srgb, var(--text) 12%, transparent), 0 20px 50px -30px rgb(0 0 0 / 0.5);
		transition: transform var(--dur-slow) var(--ease-out);
		will-change: transform;
	}
	/* The deeper shadow is its own layer that fades in: no shadow repaint on hover. */
	.lift { position: absolute; inset: 0; border-radius: inherit; box-shadow: 0 2px 4px rgb(0 0 0 / 0.1), 0 14px 32px rgb(0 0 0 / 0.2); opacity: 0; transition: opacity var(--dur-slow) var(--ease-out); pointer-events: none; }
	.words { position: relative; z-index: 2; display: block; }
	.nm { font: 700 22px/1.2 var(--font-display, var(--font-ui)); letter-spacing: -0.018em; }
	.pp { display: block; margin-top: 4px; color: var(--text-muted); font: var(--t-subhead); }
	.sx-card[data-group='library'] .nm { font-size: 30px; letter-spacing: -0.022em; }
	.chip { position: absolute; z-index: 2; top: 20px; right: 20px; }
	.art-hold :global(.sx-art) { transition: transform 700ms var(--ease-out); }
	@media (hover: hover) {
		.sx-card:hover .sx-face { transform: translateY(-3px); }
		.sx-card:hover .lift { opacity: 1; }
		.sx-card:hover .art-hold :global(.sx-art) { transform: scale(1.025); }
	}
	.sx-card:active .sx-face { transform: scale(0.985); transition-duration: var(--dur-instant); }

	/* Tablet: Library across the top, two by two under it. */
	@media (min-width: 768px) and (max-width: 1100px) {
		.sx-home { height: auto; }
		.bento { grid-template-columns: 1fr 1fr; grid-template-rows: 260px repeat(2, 220px); min-height: 0; flex: none; height: auto; }
		.sx-card[data-group='library'] { grid-row: auto; grid-column: span 2; }
	}
	/* Phone: two columns, Library wide on top. */
	@media (max-width: 767px) {
		.sx-home { min-height: 0; height: auto; }
		.lede { margin: -10px 0 16px; }
		.bento { grid-template-columns: 1fr 1fr; grid-template-rows: 210px; grid-auto-rows: 168px; gap: 12px; min-height: 0; flex: none; height: auto; }
		.sx-card[data-group='library'] { grid-row: auto; grid-column: span 2; }
		.sx-face { padding: 16px; }
		.nm { font-size: 19px; }
		.sx-card[data-group='library'] .nm { font-size: 24px; }
		.pp { font-size: 13px; }
		.chip { top: 14px; right: 14px; }
	}
	/* Four groups (a part turned off or not allowed): the last card takes the row, so no cell sits empty. */
	.bento[data-count='4'] .sx-card:last-child { grid-column: span 2; }

	@media (prefers-reduced-motion: reduce) {
		.sx-face, .lift, .art-hold :global(.sx-art) { transition: none; }
		.sx-card:hover .sx-face, .sx-card:active .sx-face { transform: none; }
		.sx-card:hover .art-hold :global(.sx-art) { transform: none; }
	}

	/* Search results. */
	.results { margin: 0; border-radius: var(--r-panel); background: color-mix(in srgb, var(--surface-1) 90%, transparent); box-shadow: 0 0 0 1px var(--line); padding: 6px; }
	.results li + li { border-top: 1px solid var(--line); }
	.row-link { display: flex; align-items: center; gap: 12px; min-height: 56px; padding: 8px 12px; border-radius: var(--r-btn); transition: background var(--dur-fast) var(--ease-out); }
	.row-link:hover { background: var(--surface-2); }
	.setting-copy { display: block; min-width: 0; flex: 1; }
	.setting-copy strong, .setting-copy span { display: block; }
	.setting-copy strong { font: var(--t-headline); font-size: 15px; }
	.setting-copy span { margin-top: 2px; color: var(--text-muted); font: var(--t-subhead); font-weight: 400; }
	.row-link > :global(.icon:last-child) { flex: none; color: var(--text-muted); }

	/* The zoom's moving pieces live on the page body (zoom.ts). */
	:global(.sx-ghost), :global(.sx-flyart) { position: fixed; z-index: 25; pointer-events: none; transform-origin: 0 0; will-change: transform, opacity; }
	:global(.sx-ghost) { border-radius: 0; background: linear-gradient(color-mix(in srgb, var(--surface-1) 80%, transparent), color-mix(in srgb, var(--surface-1) 80%, transparent)), var(--bg); backface-visibility: hidden; contain: strict; box-shadow: 0 0 0 1px var(--line), 0 12px 28px rgb(0 0 0 / 0.18); }
	:global(.sx-flyart) { overflow: hidden; border-radius: var(--r-panel); }
</style>
