<script lang="ts">
	// Plugins: a directory of signed plugins with tabs, search and sort; each opens its details (what it does, what it can reach) with install, start, stop, remove. Owner only.
	// Three views (List, Cards, Large), kept in this browser. The dev server adds a Design preview bar to try the surfaces and ten list looks.
	import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { onMount, tick, untrack } from 'svelte';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { to } from '#lib/core/paths.ts';
	import '../(frame)/settings-content.css';
	import { date } from '#lib/core/format.ts';
	import { PluginInfo, PluginSettings, AddSheet, DevActions, NoLockCard, NoNetLockCard, ApproveAccess, wheelSideways, fitChips, tagRoom, arrowKeys, tint, LanguagesSection, languageCard, LANGUAGE_PACKS_VERSION, LANGUAGE_PACKS_ICON } from '#lib/plugins/index.ts';
	import { listLanguages, packList } from '#lib/core/i18n/language-packs.svelte.ts';
	import { abilities, newAbilities, noLockNotice, categories, categoryLabel, categoryOf, developers, findPlugins, hasUpdate, inTab, notices, pluginAbout, pluginState, pluginBadge as badge, showsNoLockCard, showsNoNetLockCard, showsNoLockTag, shownName, shownVersion, sortPlugins, stateLabel, foldersMissing, isNew, freshUpdate, trendingIds, shortCount, shortDate as shortDay, numDate as numDay, pluginIcon, devPicture, isProject, brokenPics as broken, settingsPage, type Plugin, type PluginOverview, type PluginSort, type PluginTab } from '#lib/plugins/index.ts';
	import Avatar from '#lib/ui/Avatar.svelte';
	import { DlFind } from '#lib/downloads/index.ts';
	import { barTools, barLead, pluginScope } from '#lib/core/search-field.svelte.ts';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import { views as savedViews } from '#lib/core/views.svelte.ts';

	const owner = $derived(session.s.role === 'owner');
	let ov = $state<PluginOverview | null>(null);
	let loaded = $state(false);
	let error = $state('');
	let missing = $state(false);
	let busy = $state('');
	// The plugin being installed or updated from this page (busy also covers Stop, Start and Remove).
	let installing = $state('');
	let openId = $state('');
	let panelOpen = $state(false);
	let modalOpen = $state(false);
	// The view: List, Cards or Large, kept in this browser. An old Compact pick opens as Cards.
	type View = 'list' | 'cards' | 'large';
	function readView(): View {
		try {
			const v = localStorage.getItem('plugins.view');
			return v === 'list' || v === 'large' ? v : 'cards';
		} catch {
			return 'cards';
		}
	}
	let view = $state<View>(readView());
	function setView(v: View) {
		view = v;
		try {
			localStorage.setItem('plugins.view', v);
		} catch {
			/* private window: the pick lasts this visit */
		}
	}
	// The List view uses the table look (V1).
	const lvv = 1 as number;
	// A plugin that finished installing from its card shows Running for a moment.
	let justRan = $state('');
	// An update that asks for more than the installed version: the plugins and what they add, before updating.
	let askAccess = $state<{ list: Plugin[]; adds: string[] } | null>(null);
	let removing = $state<Plugin | null>(null);
	// The plugin whose settings form shows in its dialog, in place of its details.
	let editing = $state('');
	let removeOpen = $state(false);
	// The file lock question shows until answered, and again after Change.
	let lockBusy = $state(false);
	let askLock = $state(false);

	// Search and sort are kept on the account like the Library's; the tab is per visit.
	// The search text lives in the app's top search box (Ctrl K); on phones, where that bar is hidden, the page's own box edits the same text.
	const q = $derived(pluginScope.term);
	let tab = $state<PluginTab>('all');
	let langOpen = $state(false);
	// Language Packs' own logo (tinted like a plugin's) and its version as a plugin.
	const LANG_ICON = to('/plugins/language-packs.svg');
	const LANG_VERSION = LANGUAGE_PACKS_VERSION;
	let sortBy = $state<PluginSort>((['new', 'trending'] as const).find((x) => x === savedViews.get('plugins.sort')) ?? 'name');
	let pageWidth = $state(1200);
	const phone = $derived(pageWidth < 640);
	// The viewport width: the top bar (and its search box) shows from 768px.
	let vw = $state(1200);
	function setSort(v: PluginSort) {
		sortBy = v;
		savedViews.set('plugins.sort', v);
	}
	// Show narrows the list on top of the category chips; per visit, like the tab.
	let showOnly = $state<'all' | 'installed' | 'updates'>('all');
	const lang = $derived(languageCard(packList.installed, q, tab, showOnly, t('plugins.tabLanguagePacks')));
	// Language Packs opens the same window as a plugin: its own logo, About, what it can reach, then the languages.
	const langPlugin = $derived({ id: 'language-packs', version: LANGUAGE_PACKS_VERSION, installed: !!lang?.installed, running: false, enabled: true, name: t('plugins.tabLanguagePacks'), author: 'HolyArrs', category: t('plugins.languages'), description: t('plugins.languagesIntro'), icon: LANGUAGE_PACKS_ICON, stats: ov?.language_packs, access: { hosts: ['plugins.holyarrs.com'], folders: [], actions: [] } } as Plugin);
	const trending = $derived(trendingIds(ov?.plugins ?? []));
	const updates = $derived((ov?.plugins ?? []).filter((p) => p.installed && hasUpdate(p) && !p.needs_app));
	const filterItems = $derived<MenuItem[]>([
		{ kind: 'heading', label: t('plugins.viewLabel') },
		{ label: t('plugins.viewList'), checked: view === 'list', onselect: () => setView('list') },
		{ label: t('plugins.viewCards'), checked: view === 'cards', onselect: () => setView('cards') },
		{ label: t('plugins.viewLarge'), checked: view === 'large', onselect: () => setView('large') },
		{ kind: 'separator' },
		{ kind: 'heading', label: t('plugins.sortBy') },
		{ label: t('plugins.sortName'), checked: sortBy === 'name', onselect: () => setSort('name') },
		{ label: t('plugins.sortNew'), checked: sortBy === 'new', onselect: () => setSort('new') },
		...(trending.size ? [{ label: t('plugins.sortTrending'), checked: sortBy === 'trending', onselect: () => setSort('trending') }] : []),
		{ kind: 'separator' },
		{ kind: 'heading', label: t('plugins.show') },
		{ label: t('plugins.showAll'), checked: showOnly === 'all', onselect: () => (showOnly = 'all') },
		{ label: t('plugins.showInstalled'), checked: showOnly === 'installed', onselect: () => (showOnly = 'installed') },
		{ label: updates.length ? t('plugins.showUpdatesN', { n: updates.length }) : t('plugins.showUpdates'), checked: showOnly === 'updates', onselect: () => (showOnly = 'updates') }
	]);
	let addOpen = $state(false);
	const all = $derived(ov?.plugins ?? []);
	const cats = $derived(categories(all));
	const anyInstalled = $derived(all.some((p) => p.installed));
	const shown = $derived(findPlugins(sortPlugins(inTab(all, tab), sortBy), q).filter((p) => showOnly === 'all' || (showOnly === 'installed' ? p.installed : hasUpdate(p))));

	// Your plugins first (on the plain All view), then the rest.
	const plain = $derived(tab === 'all' && showOnly === 'all' && !q.trim());
	const sections = $derived(
		plain && anyInstalled
			? [
					{ key: 'mine', title: t('plugins.sectionMine'), list: shown.filter((p) => p.installed) },
					{ key: 'more', title: t('plugins.sectionMore'), list: shown.filter((p) => !p.installed) }
				].filter((x) => x.list.length)
			: [{ key: 'all', title: '', list: shown }]
	);
	const notes = $derived(ov ? notices(ov) : []);
	const canInstall = $derived(!!ov && ov.key_set && ov.can_run);
	const sel = $derived(all.find((p) => p.id === openId) ?? null);
	// List look V7 groups the rows under their category.
	const groups = $derived(view === 'list' && lvv === 7 ? cats.map((c) => ({ c, list: shown.filter((p) => categoryOf(p) === c) })).filter((g) => g.list.length) : []);
	const noLockCard = $derived(!!ov && showsNoLockCard(ov));
	const noNetCard = $derived(!!ov && showsNoNetLockCard(ov));
	const noLockTag = $derived(!!ov && showsNoLockTag(ov));
	const noLockWhy = $derived(ov ? `${noLockNotice(ov.file_lock_reason)} ${t('plugins.noLockRisk')}` : ''); // the tag explains itself
	const asking = $derived(noLockCard && (askLock || !ov?.no_file_lock_asked));

	let linked = false;
	async function load() {
		try {
			ov = await api.get<PluginOverview>('/plugins');
			error = '';
			// A link to /plugins#<id> opens that plugin's dialog once.
			if (!linked) {
				linked = true;
				const id = decodeURIComponent(location.hash.slice(1));
				const p = id && ov.plugins.find((x) => x.id === id);
				if (p) show(p);
			}
		} catch (e) {
			if (e instanceof ApiError && e.status === 404) missing = true;
			else error = message(e);
		} finally {
			loaded = true;
		}
	}
	// A /plugins#<id> link followed while on the page opens that plugin too.
	function onHash() {
		const id = decodeURIComponent(location.hash.slice(1));
		const p = id && ov?.plugins.find((x) => x.id === id);
		if (p) {
			if (!(modalOpen && openId === p.id)) show(p);
		}
		// Back from a followed /plugins#<id> link drops the hash, and a link to an unknown plugin opens nothing: close the dialog too.
		else if (modalOpen) void close();
	}
	onMount(() => {
		pluginScope.on = true;
		pluginScope.term = '';
		if (owner) void (load(), listLanguages().catch(() => {}));
		else loaded = true;
		return () => {
			barTools.snippet = null;
			barLead.snippet = null;
			pluginScope.on = false;
			pluginScope.term = '';
		};
	});
	// From 768px the tools sit beside the top bar's search box; below that (bar hidden) beside the page's own box.
	$effect(() => {
		barTools.snippet = vw >= 768 ? pageTools : null;
		barLead.snippet = vw >= 768 ? addBtn : null;
	});

	function show(p: Plugin) {
		openId = p.id;
		editing = '';
		panelOpen = true;
		modalOpen = true;
		history.replaceState(history.state, '', `#${encodeURIComponent(p.id)}`);
	}
	function clearHash() {
		if (location.hash) history.replaceState(history.state, '', location.pathname + location.search);
	}
	async function focusTile(id: string) {
		await tick();
		document.querySelector<HTMLElement>(`[data-testid="plugin-row"][data-id="${CSS.escape(id)}"] .open`)?.focus();
	}
	// Closing goes back to the tile that opened it.
	async function close() {
		const id = openId;
		panelOpen = modalOpen = false;
		clearHash();
		await tick();
		document.querySelector<HTMLElement>(`[data-testid="plugin-row"][data-id="${CSS.escape(id)}"] .open`)?.focus();
	}

	async function act(p: Plugin, run: () => Promise<unknown>, done: string, fail: string) {
		if (busy) return;
		busy = p.id;
		const name = shownName(p);
		try {
			await run();
			toasts.push(t(done, { name }), 'ok');
		} catch (e) {
			toasts.push(t(fail, { name }), 'fail', { body: message(e) });
		} finally {
			await load();
			busy = '';
			removing = null;
		}
		// After a removal from the panel, focus goes back to the tile (or the list).
		if (!panelOpen) {
			await tick();
			const tile = document.querySelector<HTMLElement>(`[data-testid="plugin-row"][data-id="${CSS.escape(p.id)}"] .open`);
			(tile ?? document.getElementById('h-plugins'))?.focus();
		}
	}
	// Start shows for an off plugin only when the owner stopped it (or no reason is given); other reasons need a fix first.
	// A plugin turned off for the missing file lock can start again once the owner allowed it to run without.
	const startable = (p: Plugin) =>
		!p.disabled_reason || p.disabled_reason === 'stopped by the owner' || p.disabled_reason === 'signed out by the owner' || (!!ov?.allow_no_file_lock && p.disabled_reason.includes('(Landlock)')) || (!!ov?.allow_no_net_lock && p.disabled_reason.includes('(seccomp user-notify)'));

	const path = (p: Plugin) => `/plugins/${encodeURIComponent(p.id)}`;

	// Install or update with the steps on the button: the list is read again while the engine works.
	// An update that asks for more than the installed version asks first.
	function install(p: Plugin) {
		const adds = newAbilities(p);
		if (adds.length) askAccess = { list: [p], adds };
		else void doInstall(p);
	}
	async function doInstall(p: Plugin): Promise<boolean> {
		if (busy) return false;
		busy = installing = p.id;
		const name = shownName(p);
		const poll = setInterval(() => void load(), 500);
		let ok = false;
		try {
			// A developer plugin is not on the signed list; it installs from the developer folder.
			await api.post(`${path(p)}/${p.dev ? 'dev-install' : 'install'}`);
			ok = true;
			toasts.push(t(p.installed ? 'plugins.updated' : 'plugins.installed', { name }), 'ok');
		} catch (e) {
			toasts.push(t('plugins.installFailed', { name }), 'fail', { body: message(e) });
		} finally {
			clearInterval(poll);
			await load();
			busy = installing = '';
		}
		if (ok) {
			justRan = p.id;
			setTimeout(() => justRan === p.id && (justRan = ''), 2500);
		}
		return ok;
	}
	function updateAll() {
		const adds = [...new Set(updates.flatMap((p) => newAbilities(p)))];
		if (adds.length) askAccess = { list: [...updates], adds };
		else void runUpdates([...updates]);
	}
	async function runUpdates(list: Plugin[]) {
		for (const p of list) await doInstall(p);
	}
	function confirmAccess() {
		const a = askAccess;
		askAccess = null;
		if (a) void (a.list.length === 1 ? doInstall(a.list[0]) : runUpdates(a.list));
	}
	/** The card's install step in words; Running for a moment after it finished. */
	function stepLabel(p: Plugin): string {
		if (p.install_step) return t(`plugins.step.${p.install_step}`);
		if (installing === p.id) return t('plugins.step.downloading');
		return justRan === p.id ? t('plugins.step.running') : '';
	}
	function askRemove(p: Plugin) {
		removing = p;
		removeOpen = true;
	}
	// Remove always deletes the plugin with its settings; the panel then offers Install again.
	function remove() {
		const p = removing;
		removeOpen = false;
		if (p) void act(p, () => api.del(path(p)), 'plugins.removed', 'plugins.removeFailed');
	}

</script>

<svelte:window bind:innerWidth={vw} onhashchange={onHash} />
<svelte:head><title>{t('plugins.title')} · HolyArrs</title></svelte:head>

<h1 class="sr-only">{t('plugins.title')}</h1>

{#snippet logo(p: Plugin, size: number)}
	{@const src = pluginIcon(p, size)}{#if src && !broken.has(src)}<img class="pic" {src} alt="" width={size} height={size} style:--pic={size + 'px'} onerror={() => broken.add(src)} />{:else}<span class="pic none" aria-hidden="true" style:--pic={size + 'px'}>{shownName(p).slice(0, 1)}</span>{/if}
{/snippet}

{#snippet card(p: Plugin)}
	{@const name = shownName(p)}
	{@const dev = developers(p)[0]}
	{@const cv = 9}
	<li class="tile card c{cv} s3 s4" use:tint={pluginIcon(p)} class:picked={panelOpen && openId === p.id} data-testid="plugin-row" data-id={p.id}>
		<button type="button" class="open" aria-label={t('plugins.openDetails', { name })} onclick={() => show(p)}></button>
		<span class="lg">{@render logo(p, cv === 9 ? 64 : 36)}</span>
		<h2 class="pname" dir="auto" use:tagRoom>{name}<span class="tin">{@render tags(p)}</span></h2>
		<span class="cat">{categoryLabel(categoryOf(p))}</span>
		<p class="one small" dir="auto"><span class="tdown" aria-hidden="true">{@render tags(p, true)}</span><span class="otext">{pluginAbout(p)}</span></p>
		<span class="by xs"><Avatar name={dev || p.author || 'HolyArrs'} src={devPicture(dev || p.author || 'HolyArrs')} bare={isProject(dev || p.author || 'HolyArrs')} size={16} /><span class="bn" dir="auto">{p.author || 'HolyArrs'}</span></span>
		{#if hasUpdate(p) && p.installed}<span class="udot" role="img" aria-label={t('plugins.stateUpdate')} title={t('plugins.stateUpdate')} data-testid="plugin-update-dot"></span>{/if}
		<div class="ft" data-testid="plugin-card-foot">{@render footChips(p)}<span class="st">{@render cardAct(p, name)}</span></div>
		{#if p.install_error && !p.install_step}<p class="xs ierr" data-testid="plugin-install-error"><Icon name="alert" size={13} /><span>{p.install_error}</span></p>{/if}
		{#if p.installed && p.needs_app && hasUpdate(p)}<p class="xs need full" data-testid="plugin-update-needs-app">{t('plugins.needsApp', { version: p.needs_app })}</p>{/if}
		{#if noLockTag}<p class="xs nolock" data-testid="plugin-nolock" title={noLockWhy}><Icon name="info" size={13} />{t('plugins.noLockTag')}</p>{/if}
	</li>
{/snippet}

{#snippet langTile(kind: 'card' | 'big' | 'row')}
	<!-- Language Packs: drawn like a plugin's tile (logo, by-line, category, version chip, footer) in every view; it opens its own sheet. -->
	{#if lang}
		{@const name = t('plugins.tabLanguagePacks')}{@const about = t('plugins.languagePacksAbout', { n: lang.n, i: lang.installed })}
		{@const size = kind === 'big' ? 72 : kind === 'card' ? 64 : lvv === 2 ? 52 : 32}
		<li class={kind === 'row' ? `lrow v${lvv}` : `tile card s3 s4 ${kind === 'big' ? 'big' : 'c9'}`} use:tint={LANG_ICON} class:picked={langOpen} data-testid="language-packs-card">
			<button type="button" class="open" aria-label={t('plugins.openDetails', { name })} onclick={() => (langOpen = true)}></button>
			{#if kind === 'big'}<span class="banner" aria-hidden="true"></span>{/if}
			<span class="lg"><img class="pic" src={LANG_ICON} alt="" width={size} height={size} style:--pic={size + 'px'} /></span>
			{#if kind === 'big'}
				<div class="bhead"><h2 class="pname" dir="auto">{name}</h2><span class="by xs"><Avatar name="HolyArrs" src={devPicture('HolyArrs')} bare size={16} /><span class="bn">HolyArrs</span></span><span class="cat">{t('plugins.languages')}</span></div>
				<p class="full small" dir="auto">{about}</p>
				<div class="bfoot" data-testid="language-packs-foot"><span class="chips" use:fitChips><span class="chip xs mono vchip">{LANG_VERSION}</span>{@render dlChip(langPlugin)}</span><span class="st">{@render langAct(name)}</span></div>
			{:else if kind === 'card'}
				<h2 class="pname" dir="auto">{name}</h2>
				<span class="cat">{t('plugins.languages')}</span>
				<p class="one small" dir="auto"><span class="otext">{about}</span></p>
				<span class="by xs"><Avatar name="HolyArrs" src={devPicture('HolyArrs')} bare size={16} /><span class="bn">HolyArrs</span></span>
				<div class="ft" data-testid="language-packs-foot"><span class="chips" use:fitChips><span class="chip xs mono vchip">{LANG_VERSION}</span>{@render dlChip(langPlugin)}</span><span class="st">{@render langAct(name)}</span></div>
			{:else}
				<h2 class="pname" dir="auto">{name}</h2>
				<p class="one small" dir="auto">{about}</p>
				<span class="cat">{t('plugins.languages')}</span>
				<span class="by xs"><Avatar name="HolyArrs" src={devPicture('HolyArrs')} bare size={16} /><span class="bn">HolyArrs</span></span>
				<span class="ver xs"><span class="mono">{LANG_VERSION}</span>{@render dls(langPlugin)}</span>
				<span class="stt xs"></span>
				<span class="st">{@render langAct(name)}</span>
			{/if}
		</li>
	{/if}
{/snippet}

{#snippet langAct(name: string)}
	<button type="button" class="get" aria-label={lang?.installed ? undefined : t('plugins.installName', { name })} onclick={() => (langOpen = true)} data-testid="language-packs-open"><Icon name={lang?.installed ? 'translate' : 'plus'} size={14} />{t(lang?.installed ? 'plugins.languages' : 'plugins.install')}</button>
{/snippet}

{#snippet tags(p: Plugin, down = false)}
	{#if isNew(p)}<span class="tag new" data-testid={down ? 'plugin-new-down' : 'plugin-new'}>{t('plugins.tagNew')}</span>{/if}
	{#if trending.has(p.id)}<span class="tag hot" data-testid={down ? 'plugin-trending-down' : 'plugin-trending'}>{t('plugins.tagTrending')}</span>{/if}
{/snippet}

{#snippet dls(p: Plugin)}
	{#if p.stats?.downloads != null}<span class="dls" data-testid="plugin-downloads">{t('plugins.downloads', { n: shortCount(p.stats.downloads, appLocale()) })}</span>{/if}
{/snippet}

{#snippet dlChip(p: Plugin)}
	{@const dl = p.stats?.downloads != null ? shortCount(p.stats.downloads, appLocale()) : ''}
	{#if dl}<span class="chip xs dlc" data-drop="1" title={t('plugins.downloads', { n: dl })} data-testid="plugin-downloads"><span class="sr-only">{t('plugins.downloads', { n: dl })}</span><span class="dnum" aria-hidden="true"><Icon name="download" size={11} />{dl}</span></span>{/if}
{/snippet}

{#snippet footChips(p: Plugin)}
	<!-- One line of chips: the version always shows whole; downloads drop first, then the date goes numeric, then chips tighten. -->
	<span class="chips" use:fitChips>
		<span class="chip xs mono vchip" title={shownVersion(p)}>{shownVersion(p)}</span>
		{@render dlChip(p)}
		{#if p.updated}<span class="chip xs dchip" class:fresh={freshUpdate(p)} title="{t('plugins.factUpdated')} {date(p.updated + 'T12:00:00Z')}" data-testid="plugin-card-updated"><span class="dlong">{shortDay(p.updated, appLocale())}</span><span class="dshort">{numDay(p.updated, appLocale())}</span></span>{/if}
	</span>
{/snippet}

{#snippet cardAct(p: Plugin, name: string)}
	{@const step = stepLabel(p)}
	{@const s = pluginState(p)}
	{#if step}<span class="get step" role="status" data-testid="plugin-step">{#if p.install_step || installing === p.id}<span class="spin" aria-hidden="true"></span>{:else}<Icon name="owned" size={14} />{/if}{step}</span>
	{:else if p.needs_app && !p.installed}<span class="xs muted need nchip" title={t('plugins.needsApp', { version: p.needs_app })} data-testid="plugin-card-needs-app"><span class="sr-only">{t('plugins.needsApp', { version: p.needs_app })}</span><span aria-hidden="true">{t('plugins.needsAppShort', { version: p.needs_app })}</span></span>
	{:else if p.install_error && !p.installed}<button type="button" class="get" aria-label={t('plugins.retryName', { name })} disabled={!canInstall || !!busy} onclick={() => install(p)} data-testid="plugin-card-retry"><Icon name="refresh" size={14} />{t('plugins.retry')}</button>
	{:else if !p.installed}<button type="button" class="get" aria-label={t('plugins.installName', { name })} disabled={!canInstall || !!busy || (!!p.dev && (!ov?.dev_mode || !!p.dev_error))} onclick={() => install(p)} data-testid="plugin-card-install"><Icon name="plus" size={14} />{t('plugins.install')}</button>
	{:else if hasUpdate(p) && !p.needs_app}<button type="button" class="get" aria-label={t('plugins.updateName', { name })} disabled={!canInstall || !!busy} onclick={() => install(p)} data-testid="plugin-card-update"><Icon name="download" size={14} />{t('plugins.update')}</button>
	{:else if s === 'running'}<button type="button" class="get" aria-label={t('plugins.stopName', { name })} disabled={!!busy} onclick={() => act(p, () => api.post(`${path(p)}/stop`), 'plugins.stopped', 'plugins.stopFailed')} data-testid="plugin-card-stop">{#if busy === p.id}<span class="spin" aria-hidden="true"></span>{:else}<Icon name="pause" size={14} />{/if}{t('plugins.stop')}</button>
	{:else}<button type="button" class="get" aria-label={t('plugins.startName', { name })} title={foldersMissing(p) ? t('plugins.chooseFolders') : s === 'off' && !startable(p) ? stateLabel(p) : undefined} disabled={!ov?.can_run || !!busy || foldersMissing(p) || (s === 'off' && !startable(p))} onclick={() => act(p, () => api.post(`${path(p)}/start`), 'plugins.started', 'plugins.startFailed')} data-testid="plugin-card-start">{#if busy === p.id}<span class="spin" aria-hidden="true"></span>{:else}<Icon name="play" size={14} />{/if}{t('plugins.start')}</button>{/if}
{/snippet}

{#snippet big(p: Plugin)}
	{@const name = shownName(p)}
	{@const dev = developers(p)[0]}
	{@const can = abilities(p)}
	{@const canShown = can.length > 3 ? can.slice(0, 2) : can}
	{@const canMore = can.length - canShown.length}
	<li class="tile card big s3 s4" use:tint={pluginIcon(p)} class:picked={panelOpen && openId === p.id} data-testid="plugin-row" data-id={p.id}>
		<button type="button" class="open" aria-label={t('plugins.openDetails', { name })} onclick={() => show(p)}></button>
		<span class="banner" aria-hidden="true"></span>
		<span class="lg">{@render logo(p, 72)}</span>
		<div class="bhead">
			<h2 class="pname" dir="auto">{name}{@render tags(p)}</h2>
			<span class="by xs"><Avatar name={dev || p.author || 'HolyArrs'} src={devPicture(dev || p.author || 'HolyArrs')} bare={isProject(dev || p.author || 'HolyArrs')} size={16} /><span class="bn" dir="auto">{p.author || 'HolyArrs'}</span></span>
			<span class="cat">{categoryLabel(categoryOf(p))}</span>
		</div>
		<p class="full small" dir="auto">{pluginAbout(p)}</p>
		<div class="bcan" data-testid="plugin-card-access"><span class="xs muted">{t('plugins.secAccess')}</span><ul class="can small">{#each canShown as c (c)}<li><Icon name="check" size={14} /><span dir="auto">{c}</span></li>{/each}{#if canMore}<li class="more muted">{t('plugins.accessMore', { n: canMore })}</li>{/if}</ul></div>
		<div class="bfoot" data-testid="plugin-card-foot">
			{@render footChips(p)}
			{#if hasUpdate(p) && p.installed}<span class="udot inl" role="img" aria-label={t('plugins.stateUpdate')} title={t('plugins.stateUpdate')} data-testid="plugin-update-dot"></span>{/if}
			<span class="st">{@render cardAct(p, name)}</span>
		</div>
		{#if noLockTag}<p class="xs nolock" data-testid="plugin-nolock" title={noLockWhy}><Icon name="info" size={13} />{t('plugins.noLockTag')}</p>{/if}
	</li>
{/snippet}

{#snippet row(p: Plugin)}
	{@const name = shownName(p)}
	{@const b = badge(p)}
	{@const dev = developers(p)[0]}
	<li class="lrow v{lvv}" use:tint={pluginIcon(p)} class:picked={panelOpen && openId === p.id} data-testid="plugin-row" data-id={p.id}>
		<button type="button" class="open" aria-label={t('plugins.openDetails', { name })} onclick={() => show(p)}></button>
		<span class="lg">{#if lvv === 9 && dev}<Avatar name={dev} src={devPicture(dev)} bare={isProject(dev)} size={40} />{:else}{@render logo(p, lvv === 2 ? 52 : 32)}{/if}</span>
		<h2 class="pname" dir="auto">{name}{@render tags(p)}</h2>
		<p class="one small" dir="auto">{pluginAbout(p)}</p>
		<span class="cat">{categoryLabel(categoryOf(p))}</span>
		<span class="by xs">{#if lvv !== 9}<Avatar name={dev || p.author || 'HolyArrs'} src={devPicture(dev || p.author || 'HolyArrs')} bare={isProject(dev || p.author || 'HolyArrs')} size={16} />{/if}<span class="bn" dir="auto">{p.author || 'HolyArrs'}</span></span>
		<span class="ver xs"><span class="mono">{shownVersion(p)}</span>{@render dls(p)}{#if hasUpdate(p) && p.installed}<span class="udot inl" role="img" aria-label={t('plugins.stateUpdate')} title={t('plugins.stateUpdate')} data-testid="plugin-update-dot"></span>{/if}</span>
		<span class="stt xs">{p.installed ? badge(p) : stateLabel(p)}</span>
		<span class="st">{@render cardAct(p, name)}</span>
		{#if noLockTag}<p class="xs nolock" data-testid="plugin-nolock" title={noLockWhy}><Icon name="info" size={13} />{t('plugins.noLockTag')}</p>{/if}
	</li>
{/snippet}

{#snippet info(p: Plugin)}
	{#if editing === p.id}<PluginSettings {p} onback={() => (editing = '')} />{:else}
	<PluginInfo {p} badge={badge(p)} {noLockTag} noLockReason={ov?.file_lock_reason} onclose={close}>
		{#snippet actions()}{@render acts(p)}{/snippet}
		{#snippet extra()}<ApproveAccess {p} reload={load} />{/snippet}
	</PluginInfo>{/if}
{/snippet}

{#snippet settingsBtn(sel: Plugin)}
	{@const name = shownName(sel)}
	{#if settingsPage(sel)}<Button href={to(settingsPage(sel))} icon="sliders" aria-label={t('plugins.settingsName', { name })} data-testid="plugin-settings">{t('plugins.settings')}</Button>
	{:else if sel.settings?.length}<Button icon="sliders" aria-label={t('plugins.settingsName', { name })} onclick={() => (editing = sel.id)} data-testid="plugin-settings">{t('plugins.settings')}</Button>{/if}
{/snippet}

{#snippet acts(sel: Plugin)}
	{@const s = pluginState(sel)}
	{@const name = shownName(sel)}
	{#if sel.dev}
		<DevActions {sel} {ov} {busy} {removing} {act} {askRemove} part="dev">{#snippet settings()}{#if sel.installed}{@render settingsBtn(sel)}{/if}{/snippet}</DevActions>
	{:else if s === 'not_installed'}
		{#if sel.needs_app}<span class="xs muted need" data-testid="plugin-needs-app">{t('plugins.needsApp', { version: sel.needs_app })}</span>
		{:else}<Button variant="primary" aria-label={t('plugins.installName', { name })} disabled={!canInstall || !!busy || !!sel.install_step} busy={busy === sel.id || !!sel.install_step} onclick={() => install(sel)} data-testid="plugin-main-install">{stepLabel(sel) || (sel.install_error ? t('plugins.retry') : t('plugins.install'))}</Button>{/if}
		<span class="signed xs" title={t('plugins.signedHint')} data-testid="plugin-signed"><Icon name="shield" size={13} />{t('plugins.signed')}</span>
	{:else}
		{#if hasUpdate(sel) && !sel.needs_app}
			<Button variant="primary" aria-label={t('plugins.updateName', { name })} disabled={!canInstall || !!busy || !!sel.install_step} busy={(busy === sel.id && !removing) || !!sel.install_step} onclick={() => install(sel)} data-testid="plugin-main-update">{stepLabel(sel) || t('plugins.update')}</Button>
		{:else if s === 'running'}
			<Button variant="primary" aria-label={t('plugins.stopName', { name })} disabled={!!busy} busy={busy === sel.id && !removing} onclick={() => act(sel, () => api.post(`${path(sel)}/stop`), 'plugins.stopped', 'plugins.stopFailed')} data-testid="plugin-main-stop">{t('plugins.stop')}</Button>
		{:else}
			<Button variant="primary" aria-label={t('plugins.startName', { name })} title={foldersMissing(sel) ? t('plugins.chooseFolders') : s === 'off' && !startable(sel) ? stateLabel(sel) : undefined} disabled={!ov?.can_run || !!busy || foldersMissing(sel) || (s === 'off' && !startable(sel))} busy={busy === sel.id && !removing} onclick={() => act(sel, () => api.post(`${path(sel)}/start`), 'plugins.started', 'plugins.startFailed')} data-testid="plugin-main-start">{t('plugins.start')}</Button>
		{/if}
		{@render settingsBtn(sel)}
		{#if hasUpdate(sel) && !sel.needs_app && s === 'running'}
			<Button aria-label={t('plugins.stopName', { name })} disabled={!!busy} busy={busy === sel.id && !removing} onclick={() => act(sel, () => api.post(`${path(sel)}/stop`), 'plugins.stopped', 'plugins.stopFailed')} data-testid="plugin-stop">{t('plugins.stop')}</Button>
		{:else if hasUpdate(sel) && !sel.needs_app}
			<Button aria-label={t('plugins.startName', { name })} title={foldersMissing(sel) ? t('plugins.chooseFolders') : s === 'off' && !startable(sel) ? stateLabel(sel) : undefined} disabled={!ov?.can_run || !!busy || foldersMissing(sel) || (s === 'off' && !startable(sel))} busy={busy === sel.id && !removing} onclick={() => act(sel, () => api.post(`${path(sel)}/start`), 'plugins.started', 'plugins.startFailed')} data-testid="plugin-start">{t('plugins.start')}</Button>
		{/if}
		<Button aria-label={t('plugins.removeName', { name })} disabled={!!busy} busy={busy === sel.id && removing?.id === sel.id} onclick={() => askRemove(sel)}>{t('plugins.remove')}</Button>
		{#if sel.needs_app && hasUpdate(sel)}<span class="xs muted need" data-testid="plugin-needs-app">{t('plugins.needsApp', { version: sel.needs_app })}</span>{/if}
	{/if}
	<DevActions {sel} {ov} {busy} {removing} {act} {askRemove} part="test" />
	{#if sel.install_error && !sel.install_step && sel.installed}<p class="xs ierr dlg" data-testid="plugin-install-error"><Icon name="alert" size={13} /><span>{sel.install_error}</span></p>{/if}
{/snippet}

{#if !owner}
	<EmptyState icon="shield" title={t('plugins.title')} text={t('plugins.ownerOnly')} />
{:else if !loaded}
	<SkSettings rows={2} />
{:else if missing}
	<EmptyState icon="shield" title={t('plugins.title')} text={t('plugins.missing')} />
{:else if !ov}
	<LoadError hasData={false} message={error} onretry={load} />
{:else}
	{#if error}<LoadError hasData={true} message={error} onretry={load} />{/if}
	{#each notes as n (n)}
		<p class="notice warn" role="status" data-testid="plugins-notice"><Icon name="info" size={18} /><span>{n}</span></p>
	{/each}

	<div class="settings-content" bind:clientWidth={pageWidth}>
		{#if ov?.dev_mode}<p class="notice warn devon" role="status" data-testid="plugins-dev-banner"><Icon name="info" size={18} /><span dir="auto">{t('plugins.devBanner', { dir: ov.dev_dir ?? '' })}</span><Button size="sm" onclick={() => (addOpen = true)} data-testid="plugins-dev-banner-open">{t('plugins.devTitle')}</Button></p>{/if}
		{#if noLockCard}<NoLockCard {ov} reload={load} />{/if}
		{#if noNetCard}<NoNetLockCard {ov} reload={load} />{/if}
		{#if all.length > 0 || lang}
				<div class="ptools" class:phone role="toolbar" aria-label={t('plugins.toolsLabel')}>
					{#if vw < 768}{@render addBtn()}{/if}
					<span class="pfind"><DlFind bind:term={pluginScope.term} shown={shown.length} total={all.length} searchKey="" placeholder={t('plugins.search')} testid="plugins-find" /></span>
					{#if vw < 768}{@render pageTools()}{/if}
				</div>
				<div class="tabs" role="group" aria-label={t('plugins.tabsLabel')} data-testid="plugins-tabs" use:wheelSideways>
					<Chip label={t('plugins.tabAll')} selected={tab === 'all'} onclick={() => (tab = 'all')} />
					{#each cats as c (c)}<Chip label={categoryLabel(c)} selected={tab === `cat:${c}`} onclick={() => (tab = `cat:${c}`)} />{/each}
				</div>
			{/if}
			{#if !anyInstalled}<p class="small muted intro" data-testid="plugins-intro">{t('plugins.intro')}</p>{/if}
			<div class="plist" id="h-plugins" tabindex="-1" role="region" aria-label={t('plugins.listLabel')}>
				{#if all.length === 0 && !lang}
					<EmptyState icon="grid" title={t('plugins.none')} text={t('plugins.noneText')} />
				{:else if shown.length === 0 && !lang}
					<EmptyState icon="search" title={t('plugins.noMatch')} text={q.trim() ? t('plugins.noMatchText', { q: q.trim() }) : t('plugins.noneInTab')} action={{ label: t('plugins.resetFilters'), onclick: () => ((pluginScope.term = ''), (tab = 'all'), (showOnly = 'all')) }} />
				{:else if view === 'list'}
					{#if lvv === 7}
						<div class="groups" data-testid="plugins-list">
							{#each groups as g (g.c)}
								<section class="grp">
									<h2 class="grph">{categoryLabel(g.c)}</h2>
									<ul class="lrows v7">{#each g.list as p (p.id)}{@render row(p)}{/each}</ul>
								</section>
							{/each}
						</div>
					{:else}
						<ul class="lrows v{lvv}" data-testid="plugins-list" use:arrowKeys>
							{#if lvv === 1}<li class="lhead xs muted" aria-hidden="true"><span></span><span>{t('plugins.sortName')}</span><span>{t('plugins.factCategory')}</span><span>{t('plugins.factDevelopers')}</span><span>{t('plugins.factVersion')}</span><span></span></li>{/if}
							{@render langTile('row')}{#each shown as p (p.id)}{@render row(p)}{/each}
						</ul>
					{/if}
				{:else}
					{#each sections.length && shown.length ? sections : [{ key: 'l', title: lang?.installed ? t('plugins.sectionMine') : '', list: [] }] as sec, i (sec.key)}
						{#if sec.title}<h2 class="sech" data-testid="plugins-section">{sec.title}</h2>{/if}
						{#if view === 'large'}
							<ul class="tiles cards large" data-testid="plugins-grid" use:arrowKeys>
								{#if i === 0}{@render langTile('big')}{/if}{#each sec.list as p (p.id)}{@render big(p)}{/each}
							</ul>
						{:else}
							<ul class="tiles cards" style:--cw="270px" data-testid="plugins-grid" use:arrowKeys>
								{#if i === 0}{@render langTile('card')}{/if}{#each sec.list as p (p.id)}{@render card(p)}{/each}
							</ul>
						{/if}
					{/each}
				{/if}
			</div>
	</div>
{/if}

<Sheet bind:open={langOpen} size="lg" placement={phone ? 'full' : 'center'} title={t('plugins.tabLanguagePacks')} hideTitle onclose={() => void tick().then(() => document.querySelector<HTMLElement>('[data-testid="language-packs-card"] .open')?.focus())}>
	{#if langOpen}<PluginInfo p={langPlugin} wideExtra can={[t('plugins.languagesReach')]} onclose={() => (langOpen = false)}>{#snippet actions()}{/snippet}{#snippet extra()}<LanguagesSection />{/snippet}</PluginInfo>{/if}
</Sheet>

<Sheet bind:open={modalOpen} size="lg" placement={phone ? 'full' : 'center'} title={sel ? shownName(sel) : t('plugins.title')} hideTitle onclose={() => ((panelOpen = false), clearHash(), void focusTile(openId))}>
	{#if sel}{@render info(sel)}{/if}
</Sheet>

<Sheet open={!!askAccess} size="sm" title={t('plugins.moreAccessTitle')} description={askAccess ? (askAccess.list.length === 1 ? t('plugins.moreAccessOne', { name: shownName(askAccess.list[0]) }) : t('plugins.moreAccessMany', { n: askAccess.list.length })) : ''} onclose={() => (askAccess = null)}>
	{#if askAccess}<ul class="adds small" data-testid="plugin-more-access">{#each askAccess.adds as a (a)}<li><Icon name="plus" size={14} /><span>{a}</span></li>{/each}</ul>{/if}
	<div class="confirm-actions">
		<Button variant="plain" onclick={() => (askAccess = null)}>{t('plugins.cancel')}</Button>
		<Button variant="primary" onclick={confirmAccess} data-testid="plugin-more-access-confirm">{t('plugins.update')}</Button>
	</div>
</Sheet>

<Sheet bind:open={removeOpen} size="sm" title={removing ? t('plugins.removeTitle', { name: shownName(removing) }) : t('plugins.remove')} description={t('plugins.removeBody')}>
	<div class="confirm-actions">
		<Button variant="plain" onclick={() => (removeOpen = false)}>{t('plugins.cancel')}</Button>
		<Button variant="destructive" onclick={remove} data-testid="plugin-remove-confirm">{t('plugins.remove')}</Button>
	</div>
</Sheet>

{#snippet addBtn()}
	<span class="cap" data-testid="plugins-add"><IconButton icon="plus" label={t('plugins.addAria')} onclick={() => (addOpen = true)} /></span>
{/snippet}

{#snippet pageTools()}
	<span class="cap" data-testid="plugins-filter"><Menu label={t('plugins.filter')} variant="icon" icon="sliders" items={filterItems} width={220} align="end" /></span>
	{#if updates.length}<span class="cap" data-testid="plugins-update-all">{#if phone}<IconButton icon="download" label={t('plugins.updateN', { n: updates.length })} disabled={!canInstall || !!busy} onclick={updateAll} />{:else}<Button icon="download" disabled={!canInstall || !!busy} onclick={updateAll}>{t('plugins.updateN', { n: updates.length })}</Button>{/if}</span>{/if}
{/snippet}

<AddSheet bind:open={addOpen} {ov} reload={load} />

<style>
	/* The toolbar row: the same search well and glass capsule as the Library's toolbar. */
	.ptools { position: relative; z-index: 2; grid-column: 1 / -1; display: flex; flex-wrap: nowrap; align-items: center; gap: 8px; }
	.ptools :global(.dlfind) { flex: 1 1 260px; min-width: 0; }
	.ptools.phone :global(.dlfind) { flex: 1 1 0; }
	/* From 768px the app's top search box searches plugins, so the page's own box steps out. */
	.pfind { display: contents; }
	@media (min-width: 768px) { .ptools { display: none; } }
	.cap { display: flex; flex: none; align-items: center; gap: 2px; height: 52px; box-sizing: border-box; padding: 3px; border: 1px solid var(--line); border-radius: 14px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); -webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); }
	.cap :global(.ui-ibtn), .cap :global(.trigger) { width: 44px; height: 44px; border-radius: 10px; }
	/* In the top bar every tool matches the search box: 44px pill, its background, ring and shadow; icon-only tools are 44px circles. */
	:global(:is(.btools, .blead)) .cap { height: 44px; padding: 0; border: 1px solid transparent; border-radius: 999px; background: color-mix(in srgb, var(--surface-2) 70%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); box-shadow: 0 8px 24px rgb(0 0 0 / 0.18), 0 0 0 0.5px color-mix(in srgb, var(--text) 14%, transparent); }
	:global(:is(.btools, .blead)) .cap :global(.ui-ibtn), :global(:is(.btools, .blead)) .cap :global(.trigger) { width: 42px; height: 42px; border-radius: 999px; }
	:global(:is(.btools, .blead)) .cap :global(.ui-btn) { height: 42px; padding-inline: 16px; border: 0; border-radius: 999px; background: none; box-shadow: none; }
	/* On phones the tools match the page's own search box (52px, 14px corners); icon-only tools are square. */
	.ptools .cap :global(.ui-btn) { height: 44px; border-radius: 10px; }
	.views :global(.ui-ibtn[aria-pressed='true']) { background: var(--surface-2); color: var(--text); box-shadow: 0 0 0 1px var(--line-strong); }
	/* Tabs: one row of chips that scrolls sideways on a phone instead of wrapping into a wall. */
	/* The chips scroll sideways; a soft fade at the right edge says there are more, and the last chip is never cut by a hard edge. */
	.tabs { grid-column: 1 / -1; min-width: 0; display: flex; gap: var(--s-2); overflow-x: auto; scrollbar-width: none; padding-block: 2px; padding-inline-end: 32px; margin-block: calc(var(--s-1) * -1); -webkit-mask-image: linear-gradient(to right, #000 calc(100% - 40px), transparent); mask-image: linear-gradient(to right, #000 calc(100% - 40px), transparent); }
	.tabs :global(> *) { flex: none; }
	.tabs::-webkit-scrollbar { display: none; }
	.tabs :global(> *) { flex: none; }
	.notice { margin-bottom: var(--s-4); }
	.plist { grid-column: 1 / -1; outline: none; container-type: inline-size; }
	/* Tiles: three across on a wide page, two on a tablet, one on a phone. */
	.tiles { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--s-3); }
	@container (max-width: 900px) { .tiles { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
	@container (max-width: 560px) { .tiles { grid-template-columns: minmax(0, 1fr); gap: var(--s-2); } }
	.tile { position: relative; display: flex; align-items: flex-start; gap: var(--s-3); min-width: 0; padding: var(--s-4); border-radius: 12px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); transition: box-shadow var(--dur-fast) var(--ease-out), background var(--dur-fast) var(--ease-out); }
	.tile:has(.open:focus-visible) { box-shadow: 0 0 0 2px var(--focus, var(--accent)); }
	/* The whole tile opens the panel; the + sits above it. */
	.open { position: absolute; inset: 0; z-index: 0; border: 0; padding: 0; background: none; border-radius: inherit; cursor: pointer; outline: none; }
	.pic { flex: none; width: var(--pic); height: var(--pic); border-radius: calc(var(--pic) * 0.23); object-fit: contain; pointer-events: none; }
	.pic.none { display: grid; place-items: center; background: var(--surface-2); box-shadow: 0 0 0 1px var(--line); font-weight: var(--weight-semibold); font-size: calc(var(--pic) * 0.4); color: var(--text-muted); }
	.who { min-width: 0; flex: 1; pointer-events: none; }
	.line { display: flex; align-items: center; gap: var(--s-2); min-width: 0; }
	.pname { margin: 0; font: var(--t-headline); font-size: 15px; font-weight: 600; letter-spacing: normal; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.one { margin: 2px 0 0; color: var(--text-muted); display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
	.by { margin: var(--s-1) 0 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

	.nolock { display: flex; align-items: center; gap: var(--s-1); margin: var(--s-1) 0 0; color: var(--wait); }
	.tile.picked { box-shadow: 0 0 0 2px var(--line-strong); }
	.grow { grid-column: 1 / -1; list-style: none; }

	/* Large: two wide tiles per row (one on a phone), a tinted banner from the icon's colour and the full description. */
	.tiles.cards.large { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--s-4); }
	@container (max-width: 640px) { .tiles.cards.large { grid-template-columns: minmax(0, 1fr); } }
	@media (max-width: 640px) { .tiles.cards.large { grid-template-columns: minmax(0, 1fr); } }
	.card.big { display: flex; flex-direction: column; align-items: stretch; gap: var(--s-3); padding: 0 var(--s-4) var(--s-4); }
	.card.big .banner { display: block; height: 72px; margin: 0 calc(-1 * var(--s-4)); background: linear-gradient(135deg, color-mix(in srgb, var(--tint, var(--accent)) 42%, var(--surface-1)), color-mix(in srgb, var(--tint, var(--accent)) 14%, var(--surface-1))); }
	.card.big .lg { margin-top: -44px; width: max-content; border-radius: 18px; box-shadow: 0 0 0 4px var(--surface-1); }
	/* Every Large tile is one fixed template, so the footer sits at the same height on every tile: name 2 lines, words 4 lines, Can access 3 lines. */
	.bhead { display: grid; grid-template-columns: minmax(0, max-content) auto; justify-content: start; align-items: center; gap: 4px var(--s-2); min-width: 0; }
	.card.big .bhead > * { grid-area: auto; }
	.card.big .bhead .pname { grid-column: 1 / -1; font-size: 18px; line-height: 24px; height: 48px; white-space: normal; overflow-wrap: anywhere; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; overflow: hidden; }
	.card.big .by { display: inline-flex; align-items: center; gap: 6px; color: var(--text-muted); min-width: 0; }
	.card.big .full { margin: 0; color: var(--text-muted); display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 4; line-clamp: 4; overflow: hidden; line-height: 20px; height: 80px; overflow-wrap: anywhere; }
	.bcan { display: grid; gap: 4px; }
	.bcan .can { list-style: none; margin: 0; padding: 0; display: grid; grid-auto-rows: 20px; gap: 4px; height: 68px; overflow: hidden; }
	.bcan .can li { display: flex; align-items: center; gap: 6px; min-width: 0; line-height: 20px; }
	.bcan .can li > span { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.bcan .can li.more { padding-inline-start: 20px; }
	.bcan .can :global(svg) { flex: none; color: var(--ok, var(--accent)); }
	.bfoot { display: flex; flex-wrap: nowrap; align-items: center; gap: var(--s-2); min-width: 0; }
	/* Footer chips (Cards and Large): one line that never wraps; the date keeps its year, the version then downloads shorten first. */
	.chips { flex: 1 1 auto; min-width: 0; display: flex; flex-wrap: nowrap; gap: 6px; height: 22px; overflow: hidden; }
	.chips .chip { flex: none; display: inline-block; height: 22px; line-height: 22px; padding: 0 8px; border-radius: 999px; background: var(--surface-2); color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.tiles.cards:not(.large) .chips { gap: 4px; }
	.tiles.cards:not(.large) .chips .chip, .tiles.cards:not(.large) .st .nchip { padding: 0 6px; font-size: 11px; }
	.chips .vchip, .chips .dlc, .chips .dchip { flex: none; text-overflow: clip; }
	.chips .dshort, .chips:global(.sdate) .dlong { display: none; }
	.chips:global(.sdate) .dshort { display: inline; }
	.tiles.cards .chips:global(.tight) { gap: 2px; }
	.tiles.cards .chips:global(.tight) .chip:not(.x) { padding: 0 4px; font-size: 10.5px; letter-spacing: -0.01em; }
	.tiles.cards .chips:global(.tighter) .chip:not(.x):not(.y) { padding: 0 3px; font-size: 9.5px; letter-spacing: -0.02em; }
	.chips :global(.chip[hidden]) { display: none; }
	.chips .dnum { display: inline-flex; align-items: center; gap: 3px; }
	.chips .chip .dls { color: inherit; }
	.ft .st, .bfoot .st { flex: none; max-width: 60%; margin-inline-start: auto; }
	/* A "needs" note gives way to the version and date first; it keeps at least 30% and ends in an ellipsis. */
	.card .ft .st:has(.need), .card .bfoot .st:has(.need) { flex: 0 100 auto; min-width: 30%; }
	.st .nchip { display: inline-block; height: 22px; line-height: 22px; padding: 0 8px; border-radius: 999px; background: var(--surface-2); }
	.st .need { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.get { min-width: 0; max-width: 100%; overflow: hidden; text-overflow: ellipsis; }
	.card.big.s3 { padding: 0 var(--s-4) var(--s-4); }

	/* Cards: narrow tiles, as many per row as fit. */
	/* Room around the grid so a lifted or scaled card never reaches the chips or gets clipped. */
	.tiles.cards { padding: 12px 8px 16px; margin-inline: -8px; }
	.tiles.cards { grid-template-columns: repeat(auto-fill, minmax(min(var(--cw), 100%), 1fr)); gap: var(--s-3); align-items: stretch; }
	.tiles.cards > .card { min-width: 0; align-self: stretch; margin: 0; transform: none; }
	@container (max-width: 560px) { .tiles.cards { grid-template-columns: repeat(auto-fill, minmax(min(var(--cw), 100%), 1fr)); } }
	.card { display: grid; align-items: center; align-content: start; gap: 4px var(--s-2); padding: var(--s-3); overflow: hidden; grid-template-columns: auto minmax(0, 1fr) auto; }
	.card > :not(.open) { position: relative; pointer-events: none; min-width: 0; }
	.card .st { pointer-events: auto; z-index: 1; justify-self: end; display: inline-flex; }
	.card .lg { display: inline-flex; position: relative; width: max-content; }
	/* Cards are one fixed template too: name 2 lines, words 2 lines, one line for the developer, then the footer. */
	.card .pname { white-space: normal; overflow-wrap: anywhere; line-height: 20px; height: 40px; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; }
	.card .one { margin: 0; -webkit-line-clamp: 2; line-clamp: 2; line-height: 18px; height: 36px; overflow-wrap: anywhere; }
	.one .tdown { display: none; }
	:global(.card.tagdown) .tin { display: none; }
	:global(.card.longname) .pname { font-size: 13px; }
	:global(.card.longername) .pname { font-size: 12px; }
	:global(.card.tagdown) .one { display: block; }
	:global(.card.tagdown) .one .tdown { display: flex; height: 18px; align-items: center; }
	:global(.card.tagdown) .one .tdown .tag:first-child { margin-left: 0; }
	:global(.card.tagdown) .one .otext { display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 1; line-clamp: 1; overflow: hidden; }
	.card .cat { max-width: 100%; overflow: hidden; text-overflow: ellipsis; display: block; line-height: 20px; }
	.cat { justify-self: start; display: inline-flex; align-items: center; height: 20px; padding: 0 8px; border-radius: 999px; background: color-mix(in srgb, var(--accent) 16%, var(--surface-2)); color: var(--text); font-size: 11px; font-weight: 600; line-height: 1; white-space: nowrap; }
	.card .by { display: inline-flex; align-items: center; gap: 6px; margin: 0; color: var(--text-muted); }
	.bn { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.ver { color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.get { display: inline-flex; align-items: center; justify-content: center; gap: 5px; height: 32px; padding: 0 14px; border: 0; border-radius: 999px; background: var(--surface-2); box-shadow: 0 0 0 1px var(--line); color: var(--text); font: inherit; font-size: 13px; font-weight: 600; cursor: pointer; }
	.get:hover { background: var(--surface-3, var(--line)); }
	.card .nolock { grid-column: 1 / -1; }
	/* C9 big logo: a tall logo column, the words beside it. */
	.c9 { grid-template-columns: auto minmax(0, 1fr); column-gap: var(--s-3); grid-template-areas: 'lg ct' 'lg nm' 'lg ds' 'lg by' 'ft ft'; } .c9 .lg { align-self: start; } .c9 .st { justify-self: end; } .c9 .ver { justify-self: start; align-self: center; } .card.c9 .ft { grid-area: ft; display: flex; align-items: center; justify-content: space-between; gap: 8px; min-width: 0; } .card .ft .ver { flex: none; white-space: nowrap; } .card .ft .st { flex: none; }
	.card .lg { grid-area: lg; } .card .pname { grid-area: nm; } .card .cat { grid-area: ct; } .card .one { grid-area: ds; } .card .by { grid-area: by; } .card .ver { grid-area: vr; justify-self: end; } .card .st { grid-area: st; }
	.sech { margin: var(--s-4) 0 var(--s-1); font-size: 15px; font-weight: 600; letter-spacing: normal; }
	.sech:first-child { margin-top: 0; }
	.intro { margin: 0 0 var(--s-2); }
	.card > .udot.udot { position: absolute; top: 12px; right: 12px; width: 9px; height: 9px; border-radius: 50%; background: var(--accent); box-shadow: 0 0 0 2px var(--surface-1); pointer-events: none; }
	.card .ierr { grid-column: 1 / -1; display: flex; align-items: flex-start; gap: 6px; margin: 0; color: var(--fail-text); }
	.card .ierr :global(svg), .ierr.dlg :global(svg) { flex: none; margin-top: 1px; }
	.ierr.dlg { flex-basis: 100%; display: flex; align-items: flex-start; gap: 6px; margin: 0; color: var(--fail-text); }
	.card .need.full { grid-column: 1 / -1; margin: 0; }
	.get.step { cursor: default; color: var(--text-muted); }
	.get:disabled { opacity: 0.55; cursor: default; }
	.spin { width: 12px; height: 12px; border-radius: 50%; border: 2px solid currentColor; border-right-color: transparent; animation: pspin 0.8s linear infinite; }
	@keyframes pspin { to { transform: rotate(360deg); } }
	@media (prefers-reduced-motion: reduce) { .spin { animation: none; } }
	.need { color: var(--text-muted); }
	.signed { display: inline-flex; align-items: center; gap: 4px; height: 24px; padding: 0 10px; border-radius: 999px; background: color-mix(in srgb, var(--ok, var(--accent)) 14%, var(--surface-2)); font-weight: 600; white-space: nowrap; }
	.adds { list-style: none; margin: var(--s-2) 0 0; padding: 0; display: grid; gap: 6px; }
	.adds li { display: flex; align-items: flex-start; gap: 6px; }
	.adds :global(svg) { flex: none; margin-top: 2px; color: var(--wait, var(--accent)); }
	.confirm-actions { display: flex; justify-content: flex-end; flex-wrap: nowrap; gap: var(--s-2); margin-top: var(--s-4); }
	/* The plugin dialog: wide enough for every part side by side at 1366 x 768, no scrolling. */
	:global(.ui-sheet.lg:has(.info.modal)) { --w: 1160px; max-height: calc(100dvh - 24px); }
	/* P6b: a squarer dialog. */
	/* The plugin window: narrow where it stacks, wide (About beside Can access) from 1100px so it fits one screen. */
	:global(.ui-sheet.lg:has(.info.pv11)) { --w: 760px; }
	@media (min-width: 1100px) { :global(.ui-sheet.lg:has(.info.pv11)), :global(.ui-sheet.lg:has(.ps)) { --w: 1180px; } }
	/* Settings with many fields get a wider dialog, so the two-column form fits without scrolling. */
	:global(.ui-sheet.lg:has(.ps .prow:nth-child(4))) { --w: 1240px; }
	:global(.ui-sheet.lg:has(.info.s3)) { border-radius: 32px; background: var(--surface-2); }
	@media (max-width: 600px) { :global(.ui-sheet.lg:has(.info.modal)) { border-radius: 0; } }

	/* Surfaces S1-S3 on the cards: one material each.
	   --tint is the icon's main colour (set from the icon); without it every look falls back to the plain surface. */
	.card.s3 { --ease-tv: cubic-bezier(0.2, 0, 0, 1); border-radius: 20px; padding: 14px; }
	/* S1 soft material: a lit top edge, a hairline and a wide soft shadow, as on a tvOS shelf. */
	/* S2 tinted glass: a faint wash of the icon's colour from the top corner, a tinted hairline. */
	/* S3 bezel tray: an outer tray with the card set inside it; inner corners follow the outer ones. */
	.card.s3 { border-radius: 22px; padding: 16px; background: var(--surface-1); box-shadow: inset 0 0 0 5px var(--surface-2), inset 0 0 0 6px var(--line), inset 0 7px 0 -1px rgb(255 255 255 / 0.05), 0 0 0 1px var(--line), 0 14px 32px -20px rgb(0 0 0 / 0.45); }
	/* S4: the S3 tray with a faint wash of the icon colour across the card top. */
	.card.s4 { background: linear-gradient(180deg, color-mix(in srgb, var(--tint, var(--surface-1)) 12%, var(--surface-1)) 0%, var(--surface-1) 45%); }
	/* Hover and keyboard focus (cards and list rows): a ring and soft glow in the icon's colour. Nothing moves, so neighbours stay put. */
	.card, .lrow { transition: box-shadow 200ms ease, background-color 200ms ease; }
	.card:is(:hover, :has(.open:focus-visible)), .lrow:is(:hover, :has(.open:focus-visible)) { z-index: 2; box-shadow: 0 0 0 1.5px color-mix(in srgb, var(--tint, var(--accent)) 70%, transparent), 0 0 28px -4px color-mix(in srgb, var(--tint, var(--accent)) 45%, transparent); }
	.card:has(.open:focus-visible), .lrow:has(.open:focus-visible) { outline: 2px solid var(--focus, var(--accent)); outline-offset: 3px; }

	/* List: one plugin per row. V1-V10 place the same parts differently (lab); release builds use V1. */
	.lrows { list-style: none; margin: 0; padding: 4px; display: grid; gap: 6px; }
	.lrow { position: relative; display: grid; align-items: center; gap: 2px var(--s-3); min-width: 0; padding: 10px 14px; border-radius: 12px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); grid-template-columns: auto minmax(0, 1fr) auto; grid-template-areas: 'lg nm st' 'lg ds st'; }
	.lrow > :not(.open) { position: relative; pointer-events: none; min-width: 0; }
	.lrow .st { pointer-events: auto; z-index: 1; justify-self: end; display: inline-flex; }
	.lrow .lg { grid-area: lg; display: inline-flex; } .lrow .pname { grid-area: nm; } .lrow .one { grid-area: ds; margin: 0; -webkit-line-clamp: 1; line-clamp: 1; } .lrow .cat { grid-area: ct; } .lrow .by { grid-area: by; display: inline-flex; align-items: center; gap: 6px; margin: 0; color: var(--text-muted); } .lrow .ver { grid-area: vr; } .lrow .stt { grid-area: tt; color: var(--text-muted); white-space: nowrap; } .lrow .st { grid-area: st; }
	.lrow .cat, .lrow .by, .lrow .ver, .lrow .stt { display: none; }
	.lrow .nolock { grid-column: 1 / -1; }
	.lhead { display: grid; gap: var(--s-3); padding: 0 14px; grid-template-columns: 32px minmax(0, 2fr) minmax(0, 1fr) minmax(0, 1fr) minmax(150px, 1fr) 96px; }
	/* V1 table: columns for name, category, developer, version and state. */
	.lrow.v1 { grid-template-columns: 32px minmax(0, 2fr) minmax(0, 1fr) minmax(0, 1fr) minmax(150px, 1fr) 96px; grid-template-areas: 'lg nm ct by vr st'; padding-block: 8px; }
	.v1 .one { display: none; } .v1 .cat, .v1 .by, .v1 .ver { display: inline-flex; } .v1 .cat { justify-self: start; }
	/* V2 big icon: a large logo, the name and two lines of words. */
	.lrow.v2 { padding: 14px 16px; } .v2 .one { -webkit-line-clamp: 2; line-clamp: 2; } .v2 .pname { font-size: 16px; }
	/* V3 description under the name (the default shape). */
	/* V4 two lines with chips: name and words, then category, version and developer as chips. */
	.lrow.v4 { grid-template-areas: 'lg nm st' 'lg ds st' 'lg ch st'; }
	.v4 .cat, .v4 .ver, .v4 .by { display: inline-flex; }
	.v4 .cat { grid-area: auto; grid-row: 3; grid-column: 2; justify-self: start; } .v4 .ver { grid-row: 3; grid-column: 2; justify-self: start; margin-inline-start: 90px; } .v4 .by { grid-row: 3; grid-column: 2; justify-self: start; margin-inline-start: 160px; }
	/* V5 zebra rows: no boxes, every other row shaded. */
	.lrows.v5 { gap: 0; } .lrow.v5 { border-radius: 8px; box-shadow: none; background: none; padding-block: 8px; } .lrow.v5:nth-child(even) { background: var(--surface-1); }
	/* V6 actions on the right: version and a full Install button at the end of the row. */
	.lrow.v6 { grid-template-columns: auto minmax(0, 1fr) auto auto; grid-template-areas: 'lg nm vr st' 'lg ds vr st'; } .v6 .ver { display: inline-flex; color: var(--text-muted); }
	/* V7 grouped by category: a heading over each group. */
	.groups { display: grid; gap: var(--s-4); } .grp { display: grid; gap: 6px; } .grph { margin: 0; padding-inline: 4px; font-size: 13px; font-weight: 600; color: var(--text-muted); letter-spacing: normal; }
	/* V8 minimal: words only, one line each. */
	.lrow.v8 { grid-template-columns: minmax(0, 14em) minmax(0, 1fr) auto; grid-template-areas: 'nm ds st'; box-shadow: none; background: none; border-bottom: 1px solid var(--line); border-radius: 0; padding-block: 8px; } .lrows.v8 { gap: 0; } .v8 .lg { display: none; }
	/* V9 developer avatar: the developer's picture leads the row, their name under the plugin's. */
	.lrow.v9 { grid-template-areas: 'lg nm st' 'lg by st'; } .v9 .one { display: none; } .v9 .by { display: inline-flex; }
	/* V10 state column: name, words, then the state in words. */
	.lrow.v10 { grid-template-columns: auto minmax(0, 1fr) 120px auto; grid-template-areas: 'lg nm tt st' 'lg ds tt st'; } .v10 .stt { display: inline; }
	@container (max-width: 560px) {
		.lhead { display: none; }
		.lrow.v1, .lrow.v6, .lrow.v10 { grid-template-columns: auto minmax(0, 1fr) auto; grid-template-areas: 'lg nm st' 'lg ds st'; }
		.v1 .one { display: -webkit-box; } .v1 .cat, .v1 .by, .v1 .ver, .v6 .ver, .v10 .stt { display: none; }
		.lrow.v8 { grid-template-columns: minmax(0, 1fr) auto; grid-template-areas: 'nm st' 'ds st'; }
	}
	.card.s3 :global(.pic) { box-shadow: 0 0 0 1px rgb(0 0 0 / 0.08), 0 4px 10px -4px rgb(0 0 0 / 0.35); }
	.card.s3 .get { background: var(--surface-1); box-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.12); transition: background-color 150ms ease, scale 150ms ease; }
	.card.s3 .get:active { scale: 0.96; }
	.tag { display: inline-flex; align-items: center; height: 18px; margin-left: 6px; padding: 0 7px; border-radius: 999px; font-size: 11px; font-weight: 600; vertical-align: 2px; white-space: nowrap; }
	.tag.new { background: color-mix(in srgb, var(--accent) 16%, var(--surface-2)); color: var(--text); }
	.tag.hot { background: color-mix(in srgb, var(--wait) 18%, var(--surface-2)); color: var(--text); }
	.dls { color: var(--text-muted); white-space: nowrap; }
	.ver .dls::before { content: '·'; margin: 0 5px; }
	.chip.fresh { background: color-mix(in srgb, var(--ok) 18%, var(--surface-2)); box-shadow: 0 0 0 1px color-mix(in srgb, var(--ok) 40%, var(--line)); }
	.lrow .get { background: var(--surface-1); box-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.12); white-space: nowrap; }
	.get { white-space: nowrap; }
	.udot.inl { display: inline-block; width: 7px; height: 7px; margin-left: 6px; border-radius: 50%; background: var(--accent); vertical-align: middle; }
</style>
