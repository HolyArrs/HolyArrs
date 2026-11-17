<script lang="ts">
	// Plugins: the signed plugin list, what each may reach, and install, start, stop, remove. Owner only.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Badge from '#lib/ui/Badge.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { onMount, tick } from 'svelte';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { to } from '#lib/core/paths.ts';
	import '../(frame)/settings-content.css';
	import { abilities, findPlugins, hasUpdate, notices, pluginAbout, pluginName, pluginState, reaches, shownVersion, sortPlugins, stateLabel, type Plugin, type PluginOverview, type PluginSort, type PluginView } from '#lib/plugins/index.ts';
	import { DlFind, dlTop } from '#lib/downloads/index.ts';
	import { LibraryViewSwitch } from '#lib/mediafiles/index.ts';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import { views as savedViews } from '#lib/core/views.svelte.ts';

	const owner = $derived(session.s.role === 'owner');
	let ov = $state<PluginOverview | null>(null);
	let loaded = $state(false);
	let error = $state('');
	let missing = $state(false);
	let busy = $state('');
	let reaching = $state<Plugin | null>(null);
	let reachOpen = $state(false);
	let removing = $state<Plugin | null>(null);
	let removeOpen = $state(false);

	// Search, sort and view: picked like the Library's, and kept on the account the same way.
	let q = $state('');
	let sortBy = $state<PluginSort>(savedViews.get('plugins.sort') === 'new' ? 'new' : 'name');
	let view = $state<PluginView>(savedViews.get('plugins.view') === 'list' ? 'list' : 'grid');
	let pageWidth = $state(1200);
	const phone = $derived(pageWidth < 640);
	function setSort(v: PluginSort) {
		sortBy = v;
		savedViews.set('plugins.sort', v);
	}
	function setView(v: PluginView) {
		view = v;
		savedViews.set('plugins.view', v);
	}
	const sortItems = $derived<MenuItem[]>([
		{ kind: 'heading', label: t('plugins.sortBy') },
		{ label: t('plugins.sortName'), checked: sortBy === 'name', onselect: () => setSort('name') },
		{ label: t('plugins.sortNew'), checked: sortBy === 'new', onselect: () => setSort('new') }
	]);
	const shown = $derived(ov ? findPlugins(sortPlugins(ov.plugins, sortBy), q) : []);

	const notes = $derived(ov ? notices(ov) : []);
	const canInstall = $derived(!!ov && ov.key_set && ov.can_run);
	const can = $derived(reaching ? abilities(reaching) : []);

	async function load() {
		try {
			ov = await api.get<PluginOverview>('/plugins');
			error = '';
		} catch (e) {
			if (e instanceof ApiError && e.status === 404) missing = true;
			else error = message(e);
		} finally {
			loaded = true;
		}
	}
	onMount(() => {
		if (owner) void load();
		else loaded = true;
	});

	// After a dialog action, focus lands on the row's main button, or the heading when the row is gone.
	async function act(p: Plugin, run: () => Promise<unknown>, done: string, fail: string, refocus = false) {
		if (busy) return;
		busy = p.id;
		try {
			await run();
			toasts.push(t(done, { name: pluginName(p.id) }), 'ok');
		} catch (e) {
			toasts.push(t(fail, { name: pluginName(p.id) }), 'fail', { body: message(e) });
		} finally {
			await load();
			busy = '';
			removing = null;
		}
		if (refocus) {
			await tick();
			const row = document.querySelector<HTMLElement>(`[data-testid="plugin-row"][data-id="${CSS.escape(p.id)}"] .acts button:not([disabled])`);
			(row ?? document.getElementById('h-plugins'))?.focus();
		}
	}
	// Start shows for an off plugin only when the owner stopped it (or no reason is given); other reasons need a fix first.
	const startable = (p: Plugin) => !p.disabled_reason || p.disabled_reason === 'stopped by the owner' || p.disabled_reason === 'signed out by the owner';

	const path = (p: Plugin) => `/plugins/${encodeURIComponent(p.id)}`;

	function askInstall(p: Plugin) {
		reaching = p;
		reachOpen = true;
	}
	function install() {
		const p = reaching;
		reachOpen = false;
		if (p) void act(p, () => api.post(`${path(p)}/install`), 'plugins.installed', 'plugins.installFailed', true);
	}
	function askRemove(p: Plugin) {
		removing = p;
		removeOpen = true;
	}
	function remove(keep: boolean) {
		const p = removing;
		removeOpen = false;
		if (p) void act(p, () => api.del(path(p) + (keep ? '?keep=1' : '')), 'plugins.removed', 'plugins.removeFailed', true);
	}
</script>

<svelte:head><title>{t('plugins.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('plugins.title')} />

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
		{#if ov.plugins.length > 0}
			<div class="ptools" class:phone role="toolbar" aria-label={t('plugins.toolsLabel')}>
				<DlFind bind:term={q} shown={shown.length} total={ov.plugins.length} searchKey={phone ? '' : dlTop.searchKey} placeholder={t('plugins.search')} testid="plugins-find" />
				<span class="cap" data-testid="plugins-sort"><Menu label={t('plugins.sortBy')} variant="icon" icon="sliders" items={sortItems} width={200} align="end" /></span>
				<span class="cap t-view"><LibraryViewSwitch value={view} options={[{ value: 'list', label: t('plugins.viewList'), icon: 'menu' }, { value: 'grid', label: t('plugins.viewCards'), icon: 'grid' }]} onchange={setView} compact /></span>
			</div>
		{/if}
		<div class="plist" id="h-plugins" tabindex="-1" role="region" aria-label={t('plugins.listLabel')}>
			{#if ov.plugins.length === 0}
				<EmptyState icon="grid" title={t('plugins.none')} text={t('plugins.noneText')} />
			{:else if shown.length === 0}
				<EmptyState icon="search" title={t('plugins.noMatch')} text={t('plugins.noMatchText', { q: q.trim() })} action={{ label: t('plugins.clearSearch'), onclick: () => (q = '') }} />
			{:else}
				<ul class="grid" class:list={view === 'list'} data-testid="plugins-{view}">
					{#each shown as p (p.id)}
						{@const s = pluginState(p)}
						{@const name = pluginName(p.id)}
						{@const about = pluginAbout(p)}
						{@const reach = reaches(p)}
						<li class="pcard" data-testid="plugin-row" data-id={p.id}>
							<div class="top">
								{#if p.icon}<img class="pic" src={p.icon} alt="" width="44" height="44" />{:else}<span class="pic none" aria-hidden="true">{name.slice(0, 1)}</span>{/if}
								<div class="who">
									<h2 class="pname">{name}</h2>
									<div class="xs muted meta">
										<span>{t('plugins.byAuthor', { author: p.author || 'HolyArrs' })}</span>
										<span class="mono">{shownVersion(p)}</span>
										<span class="signed"><Icon name="shield" size={13} />{t('plugins.signed')}</span>
									</div>
								</div>
							</div>
							{#if about}<p class="about small">{about}</p>{/if}
							<div class="access xs" data-testid="plugin-access">
								<span class="muted">{t('plugins.cardCan')}</span>
								{#if reach.length}
									<ul>{#each reach as r (r)}<li>{r}</li>{/each}</ul>
								{:else}
									<p>{t('plugins.cardNothing')}</p>
								{/if}
							</div>
							<div class="state">
								<Badge label={hasUpdate(p) ? t('plugins.stateUpdate') : s === 'off' ? t('plugins.stateOff') : stateLabel(p)} />
								{#if s === 'off' && p.disabled_reason}<span class="xs muted" data-testid="plugin-hint">{p.disabled_reason}</span>{/if}
							</div>
							<div class="acts">
								{#if s === 'not_installed'}
									<Button variant="primary" aria-label={t('plugins.installName', { name })} disabled={!canInstall || !!busy} busy={busy === p.id} onclick={() => askInstall(p)}>{t('plugins.install')}</Button>
								{:else}
									{#if hasUpdate(p)}
										<Button variant="primary" aria-label={t('plugins.updateName', { name })} disabled={!canInstall || !!busy} busy={busy === p.id && !removing} onclick={() => askInstall(p)}>{t('plugins.update')}</Button>
									{/if}
									{#if p.id === 'tailscale'}<Button href={to('/settings/remote-access')} aria-label={t('plugins.openName', { name })}>{t('plugins.open')}</Button>{/if}
									{#if s === 'running'}
										<Button aria-label={t('plugins.stopName', { name })} disabled={!!busy} busy={busy === p.id} onclick={() => act(p, () => api.post(`${path(p)}/stop`), 'plugins.stopped', 'plugins.stopFailed')}>{t('plugins.stop')}</Button>
									{:else if s === 'stopped' || (s === 'off' && startable(p))}
										<Button aria-label={t('plugins.startName', { name })} disabled={!ov.can_run || !!busy} busy={busy === p.id} onclick={() => act(p, () => api.post(`${path(p)}/start`), 'plugins.started', 'plugins.startFailed')}>{t('plugins.start')}</Button>
									{/if}
									<Button variant="plain" aria-label={t('plugins.removeName', { name })} disabled={!!busy} busy={busy === p.id && removing?.id === p.id} onclick={() => askRemove(p)}>{t('plugins.remove')}</Button>
								{/if}
							</div>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	</div>
{/if}

<Sheet bind:open={reachOpen} size="sm" title={reaching ? t('plugins.reachTitle', { name: pluginName(reaching.id) }) : t('plugins.install')}>
	{#if reaching}
		<div class="rhead">
			{#if reaching.icon}<img class="pic" src={reaching.icon} alt="" width="44" height="44" />{:else}<span class="pic none" aria-hidden="true">{pluginName(reaching.id).slice(0, 1)}</span>{/if}
			<div class="who">
				<div class="pname">{pluginName(reaching.id)}</div>
				<div class="xs muted">{t('plugins.byAuthor', { author: reaching.author || 'HolyArrs' })} · <span class="mono">{reaching.version}</span></div>
			</div>
		</div>
		{#if pluginAbout(reaching)}<p class="small about">{pluginAbout(reaching)}</p>{/if}
	{/if}
	<div class="reach" data-testid="plugin-reach">
		<p class="small label">{t('plugins.canTitle')}</p>
		<ul>{#each can as c (c)}<li class="small"><Icon name="check" size={15} /><span>{c}</span></li>{/each}</ul>
	</div>
	<div class="confirm-actions">
		<Button variant="plain" onclick={() => (reachOpen = false)}>{t('plugins.cancel')}</Button>
		<Button variant="primary" disabled={!canInstall} onclick={install}>{t('plugins.install')}</Button>
	</div>
</Sheet>

<Sheet bind:open={removeOpen} size="sm" title={removing ? t('plugins.removeTitle', { name: pluginName(removing.id) }) : t('plugins.remove')} description={t('plugins.removeAbout')}>
	<div class="confirm-actions">
		<Button variant="plain" onclick={() => (removeOpen = false)}>{t('plugins.cancel')}</Button>
		<Button onclick={() => remove(true)}>{t('plugins.removeKeep')}</Button>
		<Button variant="destructive" onclick={() => remove(false)}>{t('plugins.removeAll')}</Button>
	</div>
</Sheet>

<style>
	/* The toolbar row: the same search well and glass capsules as the Library's toolbar. */
	.ptools { grid-column: 1 / -1; display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
	.ptools :global(.dlfind) { flex: 1 1 260px; min-width: 0; }
	.ptools.phone :global(.dlfind) { flex-basis: 100%; }
	.cap { display: flex; flex: none; align-items: center; gap: 2px; height: 52px; box-sizing: border-box; padding: 3px; border: 1px solid var(--line); border-radius: 14px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); -webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); }
	.cap :global(.ui-ibtn), .cap :global(.trigger) { width: 44px; height: 44px; border-radius: 10px; }
	.t-view { --vs-bg: transparent; }
	.t-view :global(.vs) { padding: 0; gap: 2px; }
	.t-view :global(.vs button[role='radio']) { min-height: 44px; min-width: 44px; }
	.t-view :global(.vs .seg) { border-radius: 10px; }
	/* List view: one column of compact rows; icon and name, then about, state and buttons line up across rows. */
	.grid.list { grid-template-columns: 1fr; gap: var(--s-2); }
	.list .pcard { display: grid; grid-template-columns: minmax(200px, 1.1fr) minmax(0, 1.6fr) minmax(120px, 0.5fr) auto; grid-template-areas: 'top about state acts'; align-items: center; column-gap: var(--s-4); row-gap: var(--s-2); padding: var(--s-3) var(--s-4); }
	.list .top { grid-area: top; }
	.list .about { grid-area: about; }
	.list .state { grid-area: state; }
	.list .acts { grid-area: acts; margin-top: 0; justify-content: flex-end; }
	.list .access { display: none; }
	@container (max-width: 760px) { .list .pcard { grid-template-columns: minmax(0, 1fr); grid-template-areas: 'top' 'about' 'state' 'acts'; } .list .acts { justify-content: flex-start; } }
	.notice { margin-bottom: var(--s-4); }
	.plist { grid-column: 1 / -1; outline: none; container-type: inline-size; }
	.grid { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 300px), 1fr)); gap: var(--s-3); }
	.pcard { display: flex; flex-direction: column; gap: var(--s-3); min-width: 0; padding: var(--s-4); border-radius: 12px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.top, .rhead { display: flex; gap: var(--s-3); align-items: center; min-width: 0; }
	.pic { flex: none; width: 44px; height: 44px; border-radius: 10px; object-fit: contain; }
	.pic.none { display: grid; place-items: center; background: var(--surface-2); box-shadow: 0 0 0 1px var(--line); font-weight: var(--weight-semibold); font-size: 18px; color: var(--text-muted); }
	.who { min-width: 0; }
	.pname { margin: 0; font: var(--t-headline); font-size: 15px; font-weight: 600; letter-spacing: normal; overflow-wrap: anywhere; }
	.meta { display: flex; flex-wrap: wrap; column-gap: var(--s-2); row-gap: var(--s-1); }
	.signed { display: inline-flex; align-items: center; gap: var(--s-1); }
	.about { margin: 0; color: var(--text-muted); overflow-wrap: anywhere; }
	.access, .reach { display: grid; gap: var(--s-2); }
	.access ul, .reach ul { list-style: none; margin: 0; padding: 0; display: grid; gap: var(--s-1); }
	.access p { margin: 0; }
	.state { display: flex; flex-wrap: wrap; align-items: center; gap: var(--s-2); }
	.acts { display: flex; flex-wrap: wrap; gap: var(--s-2); margin-top: auto; }
	.rhead { margin-bottom: var(--s-3); }
	.reach { margin-top: var(--s-3); }
	.reach .label { margin: 0; font-weight: var(--weight-semibold); }
	.reach li { display: flex; gap: var(--s-2); align-items: flex-start; }
	.reach li :global(svg) { flex: none; margin-top: 2px; }
	.confirm-actions { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: var(--s-2); margin-top: var(--s-4); }
</style>
