<script lang="ts">
	// One plugin's details: logo, name, developers, actions, what it does, how it works, what it can reach, what's new and facts.
	// One dialog (P6b): a header band with logo, name, developers and buttons, then detail chips, About and Can access; it fits 1366 x 768 without scrolling.
	import type { Snippet } from 'svelte';
	import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
	import { bytes, date } from '#lib/core/format.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Badge from '#lib/ui/Badge.svelte';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import ScopeDetails from './ScopeDetails.svelte';
	import PluginPagesLinks from './PluginPagesLinks.svelte';
	import PluginActivity from './PluginActivity.svelte';
	import { abilities, categoryLabel, categoryOf, developers, pluginAbout, pluginState, offReason, safeLink, noLockNotice, shownName, shownVersion, freshUpdate, shortCount, shortDate, pluginIcon, devPicture, devLink, isProject, brokenPics as broken } from './view.ts';
	import Avatar from '#lib/ui/Avatar.svelte';
	import ClampText from './ClampText.svelte';
	import type { Plugin, PluginOverview } from './types.ts';
	import { tint } from './tint.ts';

	let { p, badge = '', noLockTag = false, noLockReason, actions, extra, onclose, wideExtra = false, can: canGiven }: { p: Plugin; badge?: string; noLockTag?: boolean; noLockReason?: PluginOverview['file_lock_reason']; actions: Snippet; extra?: Snippet; onclose: () => void; /** The extra part runs the whole width (Language Packs' list). */ wideExtra?: boolean; /** What it can reach, when not a plugin's own access (Language Packs). */ can?: string[] } = $props();

	// Where the plugin file lock is explained.
	const NO_LOCK_DOCS = 'https://docs.holyarrs.com/run/platforms/#plugin-file-lock';
	const name = $derived(shownName(p));
	const devs = $derived(developers(p));
	const can = $derived(canGiven ?? abilities(p));
	const repo = $derived(safeLink(p.repo));
	const shortRepo = $derived(repo ? repo.replace(/^https:\/\/(www\.)?(github\.com\/)?/, '') : '');
	// The Source chip shows only the repo's own name (owner/name in its tooltip).
	const repoName = $derived(shortRepo.replace(/\/$/, '').split('/').pop() ?? '');
	const onGithub = $derived(/^https:\/\/(www\.)?github\.com\//.test(repo));
	// The dialog's About: the long description and the steps as one short block.
	const aboutParas = $derived([...(p.long_description || pluginAbout(p)).split(/\n+/).filter(Boolean), ...(p.how_it_works?.length ? [p.how_it_works.join(' ')] : [])]);
	// The version links to its GitHub release: <id>-v<version> in the shared plugins repo, v<version> in a plugin's own repo.
	const releaseLink = $derived.by(() => {
		if (!repo || !/^https:\/\/github\.com\/[^/]+\/[^/]+\/?$/.test(repo)) return '';
		const base = repo.replace(/\/$/, '');
		const tag = /\/holyarrs-plugins$/i.test(base) ? `${p.id}-v${p.version}` : `v${p.version}`;
		return `${base}/releases/tag/${encodeURIComponent(tag)}`;
	});
	const email = $derived(p.support_email ?? '');
	const support = $derived(safeLink(p.support_url));
	// Contact copies the email (no mailto: many people have no mail app set up); with no email it opens the support page.
	let copied = $state(false);
	let copiedTimer: ReturnType<typeof setTimeout> | undefined;
	async function copyEmail() {
		try {
			await navigator.clipboard.writeText(email);
			copied = true;
			clearTimeout(copiedTimer);
			copiedTimer = setTimeout(() => (copied = false), 1600);
		} catch {
			toasts.push(email, 'ok');
		}
	}
	const updatedOn = $derived(p.updated ? date(p.updated + 'T12:00:00Z') : '');
	const updatedShort = $derived(p.updated ? shortDate(p.updated, appLocale()) : '');
</script>

{#snippet logo(size: number)}
	{@const src = pluginIcon(p, size)}
	{#if src && !broken.has(src)}<img class="pic" {src} alt="" width={size} height={size} style:--pic={size + 'px'} onerror={() => broken.add(src)} />{:else}<span class="pic none" aria-hidden="true" style:--pic={size + 'px'}>{name.slice(0, 1)}</span>{/if}
{/snippet}

{#snippet devLinks()}
	{#if devs.length > 1}
		<span class="muted devlbl">{t('plugins.factDevelopers')}</span>
		<ul class="devlist">
			{#each devs as d (d)}<li><a class="dev" href={devLink(d)} target="_blank" rel="noopener noreferrer"><Avatar name={d} src={devPicture(d)} bare={isProject(d)} size={20} />{d}</a></li>{/each}
		</ul>
	{:else if devs.length}
		<a class="dev" href={devLink(devs[0])} target="_blank" rel="noopener noreferrer"><Avatar name={devs[0]} src={devPicture(devs[0])} bare={isProject(devs[0])} size={20} />{devs[0]}</a>
	{:else}
		{#if (p.author || 'HolyArrs').toLowerCase() === 'holyarrs'}<a class="dev" href={devLink('HolyArrs')} target="_blank" rel="noopener noreferrer"><Avatar name="HolyArrs" src={devPicture('HolyArrs')} bare size={20} />HolyArrs</a>{:else}<span class="dev"><Avatar name={p.author ?? ''} src={devPicture(p.author ?? '')} bare={isProject(p.author ?? '')} size={20} />{p.author}</span>{/if}
	{/if}
{/snippet}

{#snippet factRows()}
	<dt>{t('plugins.factVersion')}</dt><dd class="mono ver" title={shownVersion(p)}>{#if releaseLink}<a href={releaseLink} target="_blank" rel="noopener noreferrer" class="ext">{shownVersion(p)}</a>{:else}{shownVersion(p)}{/if}</dd>
	{#if p.stats?.downloads != null}<dt>{t('plugins.factDownloads')}</dt><dd class="dl" data-testid="plugin-downloads" title={t('plugins.downloads', { n: shortCount(p.stats.downloads, appLocale()) })}><span class="dfull">{t('plugins.downloads', { n: shortCount(p.stats.downloads, appLocale()) })}</span><span class="dnum" aria-hidden="true"><Icon name="download" size={12} />{shortCount(p.stats.downloads, appLocale())}</span></dd>{/if}
	{#if p.updated}<dt>{t('plugins.factUpdated')}</dt><dd class:fresh={freshUpdate(p)} title={updatedOn} data-testid="plugin-updated">{updatedShort}</dd>{/if}
	{#if p.size}<dt>{t('plugins.factSize')}</dt><dd>{bytes(p.size)}</dd>{/if}
	{#if repo}<dt>{t('plugins.factSource')}</dt><dd class="repo"><a href={repo} target="_blank" rel="noopener noreferrer" class="ext src" title={shortRepo} aria-label={`${t('plugins.factSource')}: ${shortRepo}`}>{#if onGithub}<svg class="gh" viewBox="0 0 16 16" width="14" height="14" aria-hidden="true"><path fill="currentColor" d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"/></svg>{/if}<span class="rn">{repoName}</span>{#if p.stats?.stars != null}<span class="stars" data-testid="plugin-stars" aria-label={t('plugins.stars', { n: p.stats.stars })}><Icon name="star" size={12} />{shortCount(p.stats.stars, appLocale())}</span>{/if}<svg class="out" viewBox="0 0 12 12" width="10" height="10" aria-hidden="true"><path d="M4 2h6v6M10 2 3 9" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg></a></dd>{/if}
	{#if email}<dt>{t('plugins.contact')}</dt><dd class="ct" data-testid="plugin-contact"><button type="button" class="src" title={email} aria-label={`${t('plugins.contact')}: ${email}`} onclick={copyEmail}><Icon name={copied ? 'check' : 'mail'} size={14} /><span class="rn">{copied ? t('plugins.copied') : t('plugins.contact')}</span></button></dd>
	{:else if support}<dt>{t('plugins.contact')}</dt><dd class="ct" data-testid="plugin-contact"><a class="src" href={support} target="_blank" rel="noopener noreferrer"><Icon name="mail" size={14} /><span class="rn">{t('plugins.contact')}</span></a></dd>{/if}
{/snippet}

<div class="info modal dense pv11 s3 s4" class:wide-extra={wideExtra} use:tint={pluginIcon(p)} data-testid="plugin-detail" data-id={p.id}>
		<div class="wash" aria-hidden="true"></div>
	<header class="dhead">
		{@render logo(64)}
		<div class="who">
			<!-- svelte-ignore a11y_autofocus -->
			<h2 class="dname" dir="auto" tabindex="-1" autofocus>{name}</h2>
			<div class="devs small" data-testid="plugin-devs">{@render devLinks()}</div>
			<div class="xs muted meta">
				<span class="ccat">{categoryLabel(categoryOf(p))}</span>
				<span class="signed"><Icon name="shield" size={13} />{t('plugins.signed')}</span>
				{#if pluginState(p) === 'off' && p.disabled_reason}<span class="schip" data-testid="plugin-hint"><Icon name="pause" size={12} /><span>{offReason(p.disabled_reason)}</span></span>{:else if badge}<Badge label={badge} />{/if}
			</div>
		</div>
	</header>
	<span class="shut"><IconButton icon="x" label={t('ui.sheet.close')} variant="tinted" size="sm" tooltip={false} onclick={onclose} data-testid="plugin-detail-close" /></span>
	<div class="acts pills" data-testid="plugin-acts">
		{@render actions()}
		{#if noLockTag}<details class="xs nolock" data-testid="plugin-nolock"><summary><Icon name="info" size={13} />{t('plugins.noLockTag')}</summary><p class="small">{noLockNotice(noLockReason)} {t('plugins.noLockRisk')}</p><a class="small" href={NO_LOCK_DOCS} target="_blank" rel="noopener noreferrer" data-testid="plugin-nolock-docs">{t('plugins.noLockLearn')}<Icon name="arrowUpRight" size={13} /></a></details>{/if}
	</div>
	<section class="cell a-about"><h3>{t('plugins.secAbout')}</h3><ClampText lines={4}>{#each aboutParas as para, i (i)}<p class="small body" dir="auto">{para}</p>{/each}</ClampText></section>
	<section class="cell a-reach" data-testid="plugin-reach"><h3>{t('plugins.secAccess')}</h3><ul class="can small fit-grid" style:--fit-col="230px">{#each can as c (c)}<li class:wide={c.length > 80}><Icon name="check" size={15} /><span dir="auto">{c}</span></li>{/each}</ul><ScopeDetails {p} />{#if p.installed}<PluginPagesLinks id={p.id} /><PluginActivity id={p.id} />{/if}</section>
	<section class="cell a-facts"><h3>{t('plugins.secDetails')}</h3><dl class="facts xs chipfacts" data-testid="plugin-chips">{@render factRows()}</dl></section>
	{#if extra}<div class="a-extra">{@render extra()}</div>{/if}
</div>

<style>
	.info { display: grid; gap: var(--s-4); min-width: 0; }
	.dhead { display: flex; gap: var(--s-4); align-items: center; min-width: 0; }
	.shut { align-self: flex-start; margin-inline-start: auto; flex: none; }
	.who { min-width: 0; display: grid; gap: var(--s-1); }
	.dname:focus { outline: none; }
	.dname { margin: 0; font: var(--t-headline); font-size: 22px; font-weight: 650; letter-spacing: normal; overflow-wrap: anywhere; }
	.pic { flex: none; width: var(--pic); height: var(--pic); border-radius: calc(var(--pic) * 0.23); object-fit: contain; box-shadow: 0 0 0 1px var(--line); }
	.pic.none { display: grid; place-items: center; background: var(--surface-2); font-weight: var(--weight-semibold); font-size: calc(var(--pic) * 0.4); color: var(--text-muted); }
	.devs { display: flex; flex-wrap: wrap; align-items: center; gap: var(--s-1) var(--s-2); }
	.dev { display: inline-flex; align-items: center; gap: 6px; color: var(--text); text-decoration: none; }
	.dev:hover { text-decoration: underline; }
	.dev img { width: 20px; height: 20px; border-radius: 50%; background: var(--surface-2); }
	.meta { display: flex; flex-wrap: wrap; align-items: center; column-gap: var(--s-2); row-gap: var(--s-1); }
	.signed { display: inline-flex; align-items: center; gap: var(--s-1); }
	.acts { display: flex; flex-wrap: wrap; gap: var(--s-2); }
	/* The tag opens to say why there is no lock here, what that means, and where to read more. */
	.nolock { margin: 0; color: var(--wait); }
	.nolock summary { display: inline-flex; align-items: center; gap: var(--s-1); cursor: pointer; min-height: 32px; }
	.nolock p { margin: 4px 0; color: var(--text); }
	.nolock a { display: inline-flex; align-items: center; gap: 4px; color: var(--accent); }
	.body { margin: 0; overflow-wrap: anywhere; }
	.can { list-style: none; margin: 0; padding: 0; display: grid; gap: var(--s-2); }
	.can li { display: flex; gap: var(--s-2); align-items: flex-start; min-width: 0; }
	.can li > span { min-width: 0; overflow-wrap: anywhere; }
	.can li :global(svg) { flex: none; margin-top: 2px; color: var(--ok, currentColor); }
	.facts { display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: var(--s-2) var(--s-4); margin: 0; }
	.facts dt { color: var(--text-muted); }
	.facts dd { margin: 0; overflow-wrap: anywhere; }
	.ext { color: var(--text); }
	/* The dialog: one grid, the parts placed by name. It fits a 1366 x 768 screen; phones scroll. */
	.dense { display: grid; gap: var(--s-3) var(--s-4); align-items: start; grid-template-columns: repeat(3, minmax(0, 1fr)) auto; }
	.dense .dhead { grid-area: hd; } .dense .shut { grid-area: x; justify-self: end; align-self: start; } .dense .acts { grid-area: ac; align-items: center; }
	.a-extra { grid-column: 1 / -1; min-width: 0; }
	/* A long line (a list of hosts) spans the card instead of growing tall; it is never cut short. */
	.can li.wide, .can li:only-child { grid-column: 1 / -1; }
	.a-about { grid-area: ab; } .a-reach { grid-area: rc; } .a-facts { grid-area: fc; }
	.cell { display: grid; gap: var(--s-2); min-width: 0; align-content: start; }
	.cell h3 { margin: 0; font-size: 12px; font-weight: 600; color: var(--text-muted); letter-spacing: normal; }
	.dense .dname { font-size: 22px; }
	.dense .facts { gap: 6px var(--s-3); }
	.devlbl { flex-basis: 100%; font-size: 12px; }
	.devlist { list-style: none; margin: 0; padding: 0; display: flex; flex-wrap: wrap; gap: 6px; }
	.devlist .dev { padding: 2px 10px 2px 2px; border-radius: 999px; background: var(--surface-2); box-shadow: 0 0 0 1px var(--line); }
	.ccat { display: inline-flex; align-items: center; height: 20px; padding: 0 8px; border-radius: 999px; background: color-mix(in srgb, var(--accent) 16%, var(--surface-2)); color: var(--text); font-weight: 600; }
	/* The buttons are pills, like the card's Install; every action button has the same size, padding, type and radius. */
	.pills :global(.ui-btn) { box-sizing: border-box; height: 32px; min-height: 32px; min-width: 0; padding: 0 14px; gap: 5px; font-size: 13px; font-weight: 600; line-height: 1; }
	.pills :global(.ui-btn svg) { width: 14px; height: 14px; }
	.pills :global(.ui-btn) { border-radius: 999px; }
	/* Detail chips: always one row. Narrower dialogs get smaller chips; the repo name, then downloads, shorten with an ellipsis; Contact turns to its icon on a phone. */
	.a-facts { container-type: inline-size; }
	.dense .chipfacts { display: flex; flex-wrap: nowrap; gap: 6px; overflow: hidden; padding: 1px; --cfs: 12px; --cpx: 10px; }
	.chipfacts dd { flex: none; min-width: 0; font-size: var(--cfs); white-space: nowrap; }
	.chipfacts dd.ver, .chipfacts dd.dl { display: block; line-height: 24px; height: 24px; min-height: 0; padding-block: 0; overflow: hidden; text-overflow: ellipsis; }
	.chipfacts dd.ver { flex: 0 1 auto; min-width: 4ch; max-width: 14ch; }
	.chipfacts dd.dl { flex: 0 0 auto; }
	.chipfacts dd.repo { flex: 0 100 auto; min-width: 5.5em; display: flex; }
	.chipfacts dd.repo .src { min-width: 0; overflow: hidden; }
	.chipfacts dd.repo .stars, .chipfacts dd.repo .gh { flex: none; }
	.chipfacts dd.repo .rn { min-width: 3ch; }
	.chipfacts .dnum { display: none; align-items: center; gap: 3px; }
	@container (max-width: 720px) { .dense .chipfacts { gap: 4px; --cfs: 11px; --cpx: 7px; } .chipfacts .src { gap: 4px; } }
	@container (max-width: 480px) { .chipfacts { --cfs: 10.5px; --cpx: 6px; } .chipfacts dd.ct .rn { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); } .chipfacts .stars { margin-left: 2px; } .chipfacts .out, .chipfacts .gh { display: none; } .chipfacts .dfull { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); } .chipfacts .dnum { display: inline-flex; } .chipfacts dd.dl { display: inline-flex; } }
	.chipfacts dt { display: none; }
	.chipfacts dd.fresh { background: color-mix(in srgb, var(--ok) 18%, var(--surface-2)); box-shadow: 0 0 0 1px color-mix(in srgb, var(--ok) 40%, var(--line)); }
	.stars { display: inline-flex; align-items: center; gap: 3px; margin-left: 6px; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.chipfacts dd { display: inline-flex; align-items: center; min-height: 24px; padding: 2px var(--cpx); border-radius: 999px; background: var(--surface-2); box-shadow: 0 0 0 1px var(--line); }
	/* P6b square hero: a narrower dialog; the band holds logo, name, developer, category, Signed and the buttons under the name; then detail chips, About, Can access. */
	.pv11 { position: relative; grid-template-columns: minmax(0, 1fr); grid-template-areas: 'hd' 'ac' 'fc' 'ab' 'rc'; row-gap: var(--s-4); }
	.pv11::before { content: ''; grid-row: 1 / 3; grid-column: 1 / -1; align-self: stretch; margin: -4px; border-radius: 14px; background: var(--surface-2); }
	.pv11 .dhead { grid-row: 1; z-index: 1; padding: var(--s-4) 48px 0 var(--s-4); align-items: flex-start; }
	.pv11 .acts { grid-row: 2; z-index: 1; padding: 0 var(--s-4) var(--s-4); margin-top: calc(-1 * var(--s-2)); }
	.pv11 .shut { position: absolute; top: 10px; inset-inline-end: 10px; z-index: 2; }
	.pv11 .devlbl, .pv11 .a-facts h3 { display: none; }
	.pv11 .devs .dev { padding: 2px 10px 2px 2px; border-radius: 999px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.pv11 .devlist .dev { background: var(--surface-1); }
	.pv11 .a-about .body { font-size: 14px; line-height: 1.5; }
	.pv11 .can { grid-template-columns: repeat(2, minmax(0, 1fr)); column-gap: var(--s-4); }
	/* Dialog actions: every one is a pill like Install; the quiet ones sit on a raised surface so they read as buttons on the band. */
	.pills :global(.ui-btn:not(.primary)) { background: var(--surface-1); box-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.10); }
	.pills :global(.ui-btn:not(.primary):hover:not(:disabled)) { background: var(--surface-2); }
	/* Why a plugin is off: a small chip, not loose text. */
	.schip { align-self: center; display: inline-flex; align-items: center; gap: 6px; min-height: 28px; padding: 0 12px 0 10px; border-radius: 999px; background: color-mix(in srgb, var(--wait) 14%, var(--surface-1)); box-shadow: 0 0 0 1px color-mix(in srgb, var(--wait) 32%, var(--line)); color: var(--text); font-size: 12px; font-weight: 600; }
	.meta .schip { min-height: 22px; padding: 0 8px 0 7px; font-size: 11px; }
	.schip > span { display: inline-block; }
	.schip > span::first-letter { text-transform: uppercase; }
	.schip :global(svg) { flex: none; color: var(--wait); }
	/* Source chip: the one link to the plugin's repo, so it reads as a button. */
	.src { font: inherit; border: 0; display: inline-flex; align-items: center; gap: 6px; min-height: 24px; max-width: 100%; padding: 0 var(--cpx, 10px); border-radius: 999px; background: var(--surface-1); font-size: var(--cfs, inherit); white-space: nowrap; box-shadow: 0 0 0 1px var(--line-strong); color: var(--text); font-weight: 600; text-decoration: none; cursor: pointer; transition: background-color 150ms ease, box-shadow 150ms ease; }
	.src:hover { background: var(--surface-2); box-shadow: 0 0 0 1px var(--text-muted, var(--line-strong)); }
	.src:focus-visible { outline: 2px solid var(--accent, currentColor); outline-offset: 2px; }
	.src .rn { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.gh, .out { flex: none; }
	.out { opacity: 0.6; }
	.src:hover .out { opacity: 1; }
	.chipfacts dd:has(> .src) { padding: 0; background: none; box-shadow: none; }

	/* Surfaces S1-S3 on the P6b dialog. --tint is the icon's main colour; .wash is the header band behind the logo, name and buttons. */
	.wash { display: none; }
	.pv11.s3::before { display: none; }
	.pv11 .wash { display: block; position: relative; z-index: 0; grid-row: 1 / 3; grid-column: 1 / -1; align-self: stretch; overflow: hidden; pointer-events: none; }
	/* S1 soft material: the icon itself, blurred and enlarged, under a frosted layer, as behind an Apple TV app page. */
	/* S2 tinted glass: the icon's colour washes the whole top of the dialog and fades into the page. */
	/* S3 bezel tray: a tinted plate set in the grey tray; About and Can access sit in their own plates. */
	.s3 .wash { margin: -4px; border-radius: 24px; background: radial-gradient(120% 120% at 0% 0%, color-mix(in srgb, var(--tint, var(--surface-2)) 22%, var(--surface-1)), var(--surface-1) 70%); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.07), 0 0 0 1px var(--line), 0 14px 32px -20px rgb(0 0 0 / 0.5); }
	.s3.pv11 .a-about, .s3.pv11 .a-reach, .s3.pv11 .a-extra :global(.approve) { padding: 14px 16px; border-radius: 20px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.s3 .chipfacts dd { background: var(--surface-1); }
	/* S4: S3's raised panels, with S2's wash of the icon colour across the header. */
	.s4 .wash { margin: -4px; border-radius: 24px; background: linear-gradient(180deg, color-mix(in srgb, var(--tint, var(--surface-2)) 30%, var(--surface-1)) 0%, color-mix(in srgb, var(--tint, var(--surface-2)) 10%, var(--surface-1)) 75%, var(--surface-1) 100%); }
	.s3 .dhead .pic { border-radius: 22%; box-shadow: 0 0 0 1px rgb(0 0 0 / 0.08), 0 12px 24px -10px rgb(0 0 0 / 0.5); }
	.s3 .cell h3 { font-size: 12px; font-weight: 600; letter-spacing: 0.02em; color: var(--text-muted); }
	/* The dialog's parts come in one after another the first time it opens. */
	@media (prefers-reduced-motion: no-preference) {
		.s3 > :not(.wash):not(.shut) { animation: part-in 300ms cubic-bezier(0.2, 0, 0, 1) both; }
		.s3 > .acts { animation-delay: 60ms; }
		.s3 > .a-facts { animation-delay: 110ms; }
		.s3 > .a-about { animation-delay: 160ms; }
		.s3 > .a-reach { animation-delay: 210ms; }
	}
	@keyframes part-in { from { opacity: 0; transform: translateY(8px); filter: blur(4px); } }
	/* Wide window (fit-to-screen): About beside Can access, approvals across the bottom, so nothing scrolls on desktop. */
	@media (min-width: 1100px) {
		.dense.pv11.pv11 { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); grid-template-areas: 'hd hd' 'ac ac' 'fc fc' 'ab rc' 'ex rc'; }
		/* The two columns end on one line: About (and Your choices under it, when a plugin waits for approval) is as tall as Can access. */
		.pv11 .a-about, .pv11 .a-reach { align-self: stretch; }
		/* Language Packs: its languages run across the whole window, under two cards of one height. */
		.dense.pv11.pv11.wide-extra { grid-template-areas: 'hd hd' 'ac ac' 'fc fc' 'ab rc' 'ex ex'; }
		.pv11 .a-extra { grid-area: ex; display: grid; align-self: stretch; }
		.pv11 .can { grid-template-columns: repeat(auto-fill, minmax(230px, 1fr)); }
	}
	@media (max-width: 860px) {
		.dense.pv11.pv11 { grid-template-columns: minmax(0, 1fr); grid-template-areas: 'hd' 'ac' 'fc' 'ab' 'rc'; }
		.pv11 .can { grid-template-columns: minmax(0, 1fr); }
	}
</style>
