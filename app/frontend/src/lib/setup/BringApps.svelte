<script lang="ts">
	// Bring my apps on the first-run choice screen: the app cards (step A), what is still missing (step B), and the two big cards that start it.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import ChooseArt from './ChooseArt.svelte';
	import { tick } from 'svelte';
	import type { createWelcomeBring } from './welcome-bring.svelte.ts';
	import type { ToolRow } from './welcome-tools.svelte.ts';

	let { view, bring, busy, appName, onpick, rows = [] }: { appName: (k: string) => string; view: 'apps' | 'rows' | 'tiles' | 'choose'; bring: ReturnType<typeof createWelcomeBring>; busy: boolean; onpick?: (k: 'apps-a' | 'fresh') => void; rows?: ToolRow[] } = $props();
	const host = (u: string) => u.replace(/^https?:\/\//, '').replace(/\/$/, '');
	// Keyboard focus follows Manage: onto the list's title, and back onto the Manage button.
	let wasManaging: string | undefined;
	$effect(() => {
		const m = bring.managing;
		const was = wasManaging;
		wasManaging = m;
		if (m === was) return;
		const card = was === 'overseerr' ? 'seerr' : was;
		// Back on the Manage button, or on the card itself when its last copy was removed.
		void tick().then(() => document.querySelector<HTMLElement>(m ? '.manage-title' : `[data-testid="welcome-import-${card}-manage"], [data-testid="welcome-import-${card}"]`)?.focus());
	});
	const many = (k: string) => (bring.reads[k]?.length ?? 0) > 1;
	// Edit and Remove act on the one copy; with several they open the copy list, so every set works for any count.
	function edit(k: string) {
		if (many(k)) bring.managing = k;
		else bring.editApp(k);
	}
	function remove(k: string) {
		if (many(k)) bring.managing = k;
		else bring.removeApp(k);
	}
	function manage(k: string) {
		bring.managing = k;
	}
	function another(k: string) {
		bring.openImport(k);
	}
	// Remove takes its own row away: focus moves to the next row's Edit, or the list title.
	async function removeCopy(k: string, id: number, i: number) {
		await bring.removeRead(k, id);
		await tick();
		const edits = document.querySelectorAll<HTMLElement>('[data-testid="welcome-manage-edit"]');
		(edits[Math.min(i, edits.length - 1)] ?? document.querySelector<HTMLElement>('.manage-title'))?.focus();
	}
	const noRemove = (k: string) => !many(k) && !bring.canRemove(k);
</script>


{#snippet rowActs(card: string, k: string, name: string, w: string)}
	<!-- The same two pill buttons for one copy or many: Manage opens the list of copies (Edit and Remove each there), Add another reads one more. -->
	<span class="acts pills">
		<button class="pb" type="button" disabled={busy} onclick={() => manage(k)} aria-label={t('welcome.choose.manageApp', { name })} data-testid="welcome-import-{card}-manage">{t('welcome.choose.manage')}</button>
		<button class="pb" type="button" disabled={busy} onclick={() => another(k)} aria-label={t('welcome.choose.addAnotherApp', { name })} data-testid="welcome-import-{card}-another">{t('welcome.addAnother')}</button>
	</span>
{/snippet}



{#if view === 'apps' && bring.managing}
	{@const k = bring.managing}
	<div class="manage" data-testid="welcome-manage">
		<h3 class="manage-title" tabindex="-1">{t('welcome.choose.yourCopies', { name: appName(k) })}</h3>
		<ul class="copies">
			{#each bring.reads[k] ?? [] as r, i (r.id)}
				<li class="copy" data-testid="welcome-manage-row">
					<span class="copy-words"><span class="copy-addr">{host(r.address)}{#if r.from}<span class="from-tag" data-testid="welcome-manage-from">{t('downloads.hl.fromApp', { app: r.from })}</span>{/if}</span><span class="app-count">{bring.countLine(k, r.counts)}</span></span>
					<span class="pills"><button class="pb" type="button" disabled={busy} onclick={() => bring.editApp(k, r.id)} aria-label={t('welcome.edit', { name: host(r.address) })} data-testid="welcome-manage-edit">{t('welcome.edit2')}</button>
					{#if bring.canRemove(k, r.id)}<button class="pb" type="button" disabled={busy} onclick={() => removeCopy(k, r.id, i)} aria-label={t('welcome.choose.removeApp', { name: host(r.address) })} data-testid="welcome-manage-remove">{t('migrate.remove')}</button>{/if}</span>
				</li>
			{/each}
		</ul>
		<span class="manage-foot">
			<!-- The corner Back closes this list; no second Back here. -->
			<span class="pills"><button class="pb" type="button" disabled={busy} onclick={() => bring.openImport(k)} data-testid="welcome-manage-another">{t('welcome.addAnother')}</button></span>
		</span>
	</div>
{:else if view === 'apps'}
	<div class="apps" role="group" aria-label={t('welcome.choose.apps')} data-testid="welcome-import">
		{#each bring.mainApps as a (a.kind)}
			{@const did = bring.doneApps.includes(a.kind) || (a.kind === 'seerr' && bring.doneApps.includes('overseerr'))}
			{@const k = bring.doneApps.includes(a.kind) ? a.kind : 'overseerr'}
			{@const w = did ? bring.doneWords[a.kind] || (a.kind === 'seerr' ? bring.doneWords.overseerr : '') : ''}
			<!-- A Radarr or Sonarr Seerr keeps that HolyArrs could not reach: amber, and it opens prefilled. Once another copy is read, the row keeps its tick and says so after the count. -->
			{@const look = !!bring.checks[a.kind]}
			<div class="app" class:on={did} class:look={look && !did}>
				<button class="app-hit" type="button" disabled={busy} onclick={() => bring.openImport(a.kind)} data-testid="welcome-import-{a.kind}" aria-label={did ? `${a.name}, ${t('welcome.choose.imported')}${look ? `, ${t('migrate.needsALook')}` : ''}` : look ? `${a.name}, ${t('migrate.needsALook')}` : a.name}></button>
				<span class="app-tick" class:todo={!did && !look} aria-hidden="true">{#if did}<Icon name="check" size={14} stroke={2.5} />{:else if look}<Icon name="alert" size={14} stroke={2.5} />{/if}</span>
				<span class="app-logos">{#each a.logos as l (l)}<img class="app-logo" src={bring.appLogo(l)} alt="" width="48" height="48" />{/each}</span>
				<span class="app-name">{a.name}{#if did && bring.fromOf[a.kind]}<span class="from-tag" data-testid="welcome-import-{a.kind}-from">{t('downloads.hl.fromApp', { app: bring.fromOf[a.kind] })}</span>{/if}{#if w}<span class="app-count" data-testid="welcome-import-{a.kind}-count">{w}{#if look} · <button class="look-btn" type="button" disabled={busy} onclick={() => bring.openImport(a.kind)} data-testid="welcome-import-{a.kind}-look">{t('migrate.needsALook')}</button>{/if}</span>{:else if look}<span class="app-count" data-testid="welcome-import-{a.kind}-look">{t('migrate.needsALook')}</span>{:else if a.kind === 'seerr' && !did}<span class="app-hint" data-testid="welcome-import-seerr-hint">{t('welcome.choose.seerrFirst')}</span>{/if}</span>
				{#if did}{@render rowActs(a.kind, k, a.name, w)}{/if}
			</div>
		{/each}
	</div>
{:else if view === 'rows' || view === 'tiles'}
	<!-- Sources and Downloads: one row per app in the same look; a ticked row says where an import brought it from. -->
	<div class="apps" class:tiles={view === 'tiles'} role="group" data-testid="tool-rows">
		{#each rows as r (r.key)}
			<div class="app" class:on={r.done}>
				<button class="app-hit" type="button" disabled={busy} onclick={r.open} data-testid="tool-{r.key}" aria-label={r.done ? `${r.name}, ${t('welcome.choose.imported')}` : r.name}></button>
				<span class="app-tick" class:todo={!r.done} aria-hidden="true">{#if r.done}<Icon name="check" size={14} stroke={2.5} />{/if}</span>
				<span class="app-logos">{#if r.logo}<img class="app-logo" src={r.logo} alt="" width="48" height="48" />{:else}<span class="app-logo icon"><Icon name={r.icon ?? 'search'} size={26} /></span>{/if}</span>
				<span class="app-name">{r.name}{#each r.from as f (f)}<span class="from-tag" data-testid="tool-{r.key}-from">{t('downloads.hl.fromApp', { app: f })}</span>{/each}{#if r.done && r.line}<span class="app-count">{r.line}</span>{/if}</span>
				{#if r.done && r.manage && r.another}
					<span class="acts pills">
						<button class="pb" type="button" disabled={busy} onclick={r.manage} data-testid="tool-{r.key}-manage">{t('welcome.choose.manage')}</button>
						<button class="pb" type="button" disabled={busy} onclick={r.another} data-testid="tool-{r.key}-another">{t('welcome.addAnother')}</button>
					</span>
				{/if}
			</div>
		{/each}
	</div>
{:else}
	{#each [{ k: 'apps-a', id: 'welcome-bring' }, { k: 'fresh', id: 'welcome-fresh' }] as const as c (c.k)}
		<button class="pick" type="button" disabled={busy} onclick={() => onpick?.(c.k)} data-testid={c.id}>
			<span class="pick-words">
				<span class="pick-title">{t(c.k === 'apps-a' ? 'welcome.choose.apps' : 'welcome.choose.fresh')}</span>
				<span class="pick-desc">{t(c.k === 'apps-a' ? 'welcome.choose.appsDesc' : 'welcome.choose.freshDesc')}</span>
			</span>
			<span class="pick-art"><ChooseArt kind={c.k === 'apps-a' ? 'apps' : 'fresh'} /></span>
		</button>
	{/each}
{/if}

<style>
	/* Choose: Import from my apps, one wide row above the parts. */
	/* Import: the apps as small cards, nine across on desktop, in the Link step's card look. */
	.apps { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px; }
	.app { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 6px; min-height: 72px; padding: 8px 4px; border-radius: var(--g-r, 16px); border: 1px solid var(--g-line, var(--line)); background: var(--g-bg, transparent); color: var(--ink, var(--text)); font: 600 13px/1.2 var(--font-ui); cursor: pointer; transition: background-color var(--dur-fast) var(--ease-out), border-color var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out); }
	@media (hover: hover) { .app:hover:not(:disabled) { background: var(--g-bg-hover, transparent); border-color: var(--g-line-hover, var(--line-strong)); } }
	.app:active:not(:disabled) { transform: scale(0.98); }
	.app:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.app:has(> .app-hit:disabled) { opacity: 0.5; }
	.app-logo { width: 32px; height: 32px; }
	.app-name { overflow-wrap: anywhere; text-align: center; }
	/* A done card's actions: quiet text links in one centred row, like the media server cards (Edit · Unlink). */
	.acts { position: relative; z-index: 2; display: flex; flex-wrap: wrap; justify-content: center; align-items: center; gap: 2px 4px; margin-top: 4px; }
	.acts button, .manage button { font: 500 13px/1 var(--font-ui); color: var(--ink, var(--text)); cursor: pointer; }
	.acts button:focus-visible, .manage button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	/* Manage: one row per copy; the list scrolls inside itself, the page never does. */
	/* Right under the lead like the other sub-screens: no top band from the card's spacing. */
	.manage { flex: 1 1 auto; display: flex; flex-direction: column; gap: 10px; min-height: 0; max-height: 52vh; margin-top: 0 !important; }
	.manage-title:focus { outline: none; }
	.manage-title { margin: 0; font: 600 18px/1.3 var(--font-ui); color: var(--ink, var(--text)); }
	.copies { flex: 0 1 auto; min-height: 0; overflow-y: auto; display: grid; align-content: start; gap: 8px; margin: 0; padding: 0; list-style: none; }
	.copy { display: flex; align-items: center; gap: 6px; padding: 10px 12px; border-radius: 14px; border: 1px solid var(--g-line, var(--line)); background: var(--g-bg, transparent); }
	.copy-words { flex: 1 1 auto; min-width: 0; display: grid; gap: 2px; }
	.copy-addr { font: 600 15px/1.3 var(--font-ui); color: var(--ink, var(--text)); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.copy .app-count { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	@media (max-width: 520px) { .copy { flex-wrap: wrap; } .copy-words { flex-basis: 100%; } }
	.from-tag { display: inline-flex; align-items: center; margin-inline-start: 8px; padding: 1px 8px; border-radius: 999px; vertical-align: middle; font: 600 12px/1.4 var(--font-ui); color: rgb(var(--ink-rgb, 255 255 255) / 0.85); background: rgb(var(--ink-rgb, 255 255 255) / 0.12); }
	.app-count { overflow-wrap: anywhere; display: block; margin-top: 2px; font: 400 13px/1.3 var(--font-ui); color: rgb(var(--ink-rgb, 255 255 255) / 0.72); }
	@media (max-width: 1099px) { :global(.card.bring) .app-name { text-align: start; } }
	@media (max-width: 520px) {
		.app { min-height: 56px; gap: 4px; font-size: 12px; padding: 6px 2px; }
		.app-logo { width: 26px; height: 26px; }
	}
	@media (min-width: 1100px) {
		.apps { grid-template-columns: repeat(9, minmax(0, 1fr)); gap: 10px; }
		.app { min-height: clamp(84px, 11vh, 120px); font-size: 15px; gap: 10px; }
		.app-logo { width: 40px; height: 40px; }
	}
	.app-logo { width: 40px; height: 40px; object-fit: contain; display: block; }
	/* Bring my apps: a card is a box with one hit button over it, so Seerr's "Overseerr?" can sit inside as its own button. */
	:global(.card.bring) .app { position: relative; cursor: default; }
	@media (hover: hover) { :global(.card.bring) .app:has(.app-hit:hover:not(:disabled)) { background: var(--g-bg-hover, transparent); border-color: var(--g-line-hover, var(--line-strong)); } }
	:global(.card.bring) .app:has(.app-hit:focus-visible) { box-shadow: var(--focus-ring); }
	.app-hit { position: absolute; inset: 0; z-index: 1; border: 0; border-radius: inherit; background: none; cursor: pointer; }
	.app-hit:focus-visible { outline: none; }
	.app-hit:disabled { cursor: default; }
	.app-logos { display: flex; gap: 8px; }
	.app.on { border-color: rgb(48 209 88 / 0.5); }
	.app.look { border-color: rgb(255 159 10 / 0.55); }
	.app.look .app-tick { background: rgb(255 159 10); color: #1c1c1e; }
	.app.look .app-count, .look-btn { color: rgb(255 179 64); font: 500 15px/1.3 var(--font-ui); }
	.look-btn { position: relative; z-index: 2; padding: 0; border: 0; background: none; cursor: pointer; text-decoration: underline; text-underline-offset: 3px; }
	.look-btn:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 4px; }
	:global(.light) .app.look .app-count, :global(.light) .look-btn { color: rgb(180 83 9); }
	.app-hint { display: block; margin-top: 2px; font: 400 13px/1.3 var(--font-ui); color: rgb(var(--ink-rgb, 255 255 255) / 0.72); overflow-wrap: anywhere; }
	/* One size for the line under the name in every state; the state shows in its colour only. */
	.app-count, .app-hint, .app.look .app-count { font: 400 15px/1.3 var(--font-ui); }
	.app.on .app-count { color: rgb(120 220 140); }
	:global(.light) .app.on .app-count { color: rgb(21 128 61); }
	.app-tick.todo { background: none; border: 1.5px solid rgb(var(--ink-rgb, 255 255 255) / 0.3); box-sizing: border-box; }
	.app-tick { flex: none; display: inline-grid; place-items: center; width: 22px; height: 22px; border-radius: 50%; background: rgb(48 209 88); color: #fff; }
	.app > .app-tick { position: absolute; top: 12px; inset-inline-end: 12px; }
	.app-logo.icon { display: inline-grid; place-items: center; color: var(--ink, var(--text)); }
	@media (max-width: 1099px) {
		:global(.card.bring) .apps { grid-template-columns: 1fr; gap: 8px; }
		:global(.card.bring) .app { flex-direction: row; justify-content: flex-start; min-height: 60px; padding: 8px 14px; gap: 12px; font-size: 16px; }
		:global(.card.bring) .app-logo { width: 32px; height: 32px; }
	}
	/* Choose: two big cards side by side, words on top, art filling the rest. */
	:global(.sheet > .card.choose) > .pick { margin: 0 !important; }
	.pick { position: relative; display: flex; flex-direction: column; min-height: 320px; padding: 0; overflow: hidden; border-radius: 28px; border: 1px solid var(--g-line, var(--line)); background: var(--g-bg, var(--surface-2)); color: var(--ink, var(--text)); text-align: start; cursor: pointer; transition: border-color var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out); }
	.pick-words { position: relative; z-index: 1; display: grid; gap: 6px; padding: 26px 28px 8px; }
	.pick-title { font: 700 clamp(24px, 3vh, 32px) / 1.15 var(--font-display, var(--font-ui)); letter-spacing: -0.01em; overflow-wrap: anywhere; }
	.pick-desc { font: 400 16px/1.35 var(--font-ui); color: rgb(var(--ink-rgb, 255 255 255) / 0.72); text-wrap: pretty; }
	.pick-art { position: relative; flex: 1 1 auto; min-height: 0; display: block; -webkit-mask-image: linear-gradient(180deg, transparent, #000 18%); mask-image: linear-gradient(180deg, transparent, #000 18%); }
	@media (hover: hover) { .pick:hover:not(:disabled) { border-color: var(--g-line-hover, var(--line-strong)); background: var(--g-bg-hover, var(--surface-2)); box-shadow: 0 0 0 1px var(--g-line-hover, var(--line-strong)); } }
	.pick:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.pick:disabled { opacity: 0.5; cursor: default; }
	:global(.welcome.over-art[data-theme='light']) .pick-desc { color: var(--ink-2); }
	@media (min-width: 1100px) {
		/* Bring my apps: the nine apps fill the card, three by three. */
		:global(.sheet.glass > .card.bring) > .apps { flex: 1 1 auto; min-height: 0; margin: 0 !important; grid-template-columns: repeat(5, minmax(0, 1fr)); grid-auto-rows: 1fr; gap: 16px; }
		:global(.card.bring) .app { min-height: 0; gap: 18px; font-size: 20px; border-radius: 22px; }
		:global(.card.bring) .app-logo { width: 64px; height: 64px; }
		.pick { min-height: 0; }
	}
	@media (max-width: 640px) {
		.pick { min-height: 210px; border-radius: 22px; }
		.pick-words { padding: 18px 20px 4px; }
		.pick-title { font-size: 22px; }
		.pick-desc { font-size: 15px; }
	}

	/* Rows: one full-width row per app at every size; tick, logo, name and count, actions at the end. They fill the card. */
	:global(.card.bring) .apps { grid-template-columns: 1fr; gap: 8px; }
	:global(.card.bring) .app { flex-direction: row; flex-wrap: wrap; justify-content: flex-start; min-height: 60px; padding: 8px 14px; gap: 6px 12px; font-size: 16px; }
	:global(.card.bring) .app-name { flex: 1 1 auto; min-width: 0; text-align: start; }
	:global(.card.bring) .app > .app-tick { position: static; order: -1; }
	.acts { margin: 0; margin-inline-start: auto; flex-wrap: nowrap; gap: 6px; }
	@media (min-width: 1100px) {
		:global(.sheet.glass > .card.bring) > .apps { grid-template-columns: 1fr; gap: 10px; }
		:global(.card.bring) .app { flex-wrap: nowrap; padding: 0 28px; gap: 24px; font-size: 20px; }
		:global(.card.bring) .app-logo { width: 56px; height: 56px; }
	}
	.acts button { min-height: 40px; }
	.acts button:disabled, .pb:disabled { opacity: 0.5; cursor: not-allowed; }
	@media (hover: hover) { .pb:hover:not(:disabled) { background: rgb(var(--ink-rgb, 255 255 255) / 0.12); } }
	.pills { display: flex; gap: 6px; }
	/* Sources and Downloads rows: the Parts row height, never stretched to fill the card (two apps stay two short rows). */
	:global(.card.bring) > .apps[data-testid='tool-rows']:not(.tiles), :global(.sheet.glass > .card.bring) > .apps[data-testid='tool-rows']:not(.tiles) { flex: 0 0 auto; grid-auto-rows: 82px; align-content: start; }
	/* Manage and Add another: one fixed look on every step (Parts, Sources, Downloads), whatever size the row's own text is. */
	.pills .pb, :global(.card.bring) .pills .pb { min-height: 40px; height: 40px; padding: 0 16px; border-radius: 999px; border: 1px solid var(--g-line-hover, var(--line-strong)); background: none; color: var(--ink, var(--text)); font: 600 14px/1 var(--font-ui); letter-spacing: 0; white-space: nowrap; }
	/* Tiles (Sources): the apps side by side like the media servers, logo and name centred, tick in the corner. */
	:global(.card.bring) .apps.tiles, :global(.sheet.glass > .card.bring) > .apps.tiles { grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); grid-auto-rows: auto; align-content: start; gap: 16px; }
	:global(.card.bring) .apps.tiles .app { flex-direction: column; flex-wrap: nowrap; align-items: center; justify-content: center; min-height: 200px; padding: 20px 16px; gap: 10px; text-align: center; }
	:global(.card.bring) .apps.tiles .app > .app-tick { position: absolute; top: 14px; inset-inline-end: 14px; order: 0; }
	:global(.card.bring) .apps.tiles .app-name { flex: none; display: flex; flex-direction: column; align-items: center; gap: 6px; text-align: center; }
	:global(.card.bring) .apps.tiles .app-logo { width: 48px; height: 48px; }
	.tiles .acts { margin: 0; }
	/* Jackett's logo is black on clear: it sits on a white disc, like Bazarr's, so it shows on dark glass. */
	.app-logo[src$='jackett.png'] { background: #fff; border-radius: 50%; padding: 7px; box-sizing: border-box; }
	.manage-foot { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; }
</style>
