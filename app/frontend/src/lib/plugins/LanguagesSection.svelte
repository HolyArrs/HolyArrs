<!-- The Language packs card's sheet: one row per language with its own Install or Remove. English is built in; any
     number of packs can be installed or removed at any time, and installed ones show in the language picker. -->
<script lang="ts">
	import { t, type Locale } from '#lib/core/i18n/i18n.svelte.ts';
	import { listLanguages, installLanguage, removeLanguage, packs, type LanguageRow } from '#lib/core/i18n/language-packs.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { message } from '#lib/core/api.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import { onMount } from 'svelte';
	import { englishName, languageAction, ownName, sortLanguageRows } from './languages.ts';
	import { tick } from 'svelte';

	let rows = $state<LanguageRow[]>([]);
	let loaded = $state(false);
	let removing = $state<Locale | null>(null);
	const owner = $derived(session.s.role === 'owner');
	// The order is set when the sheet opens, so a card doesn't jump away from the button just pressed.
	let order = $state<Locale[]>([]);
	const byCode = $derived(new Map(rows.map((r) => [r.code, r])));
	const shown = $derived(order.map((c) => byCode.get(c)).filter((r): r is LanguageRow => !!r));
	// The row's button is swapped (Install <-> Remove) when its state changes; put focus back on the new one.
	// Busy buttons stay focusable (aria-disabled), so focus never falls to the page meanwhile.
	async function refocus(code: Locale) {
		await tick();
		languageAction(document, code)?.focus();
	}

	async function load() {
		try {
			rows = (await listLanguages()).languages;
		} catch {
			rows = [];
		}
		if (!order.length) order = sortLanguageRows(rows).map((r) => r.code);
		loaded = true;
	}
	async function add(code: Locale) {
		if (packs.busy === code || rows.find((r) => r.code === code)?.state === 'installing') return;
		if (!(await installLanguage(code))) toasts.push(t('accounts.language.failed'), 'fail');
		await load();
		await refocus(code);
	}
	async function drop(code: Locale) {
		if (removing === code) return;
		removing = code;
		try {
			await removeLanguage(code);
		} catch (err) {
			toasts.push(t('plugins.languageRemoveFailed'), 'fail', { body: message(err) });
		} finally {
			removing = null;
		}
		await load();
		await refocus(code);
	}
	onMount(load);
</script>

<div class="langs" data-testid="plugins-languages">
	{#if loaded && shown.length}
		<ul class="ltiles">
			{#each shown as r (r.code)}
				{@const name = ownName(r.code)}
				{@const wait = packs.busy === r.code || r.state === 'installing'}
				<li class="ltile" data-testid="language-row" data-code={r.code}>
					<span class="nm">
						<span class="own" lang={r.code} dir="auto">{name}</span>
						<span class="en muted">{englishName(r.code)}{#if r.size}{' · '}{t('plugins.languageSize', { n: Math.max(1, Math.round(r.size / 1024)) })}{/if}</span>
					</span>
					<span class="act">
						{#if r.state === 'built_in'}
							<span class="st muted">{t('plugins.languageBuiltIn')}</span>
						{:else if r.state === 'installed'}
							<span class="st ok" title={t('plugins.languageInstalled')}><Icon name="check" size={14} /><span class="sr-only">{t('plugins.languageInstalled')}</span></span>
							{#if owner}<button type="button" class="get ic plain" aria-label={t('plugins.removeName', { name })} title={t('plugins.remove')} aria-disabled={removing === r.code} onclick={() => drop(r.code)} data-testid="language-remove">{#if removing === r.code}<span class="spin" aria-hidden="true"></span>{:else}<Icon name="x" size={14} />{/if}</button>{/if}
						{:else}
							{#if r.state === 'failed'}<span class="st bad" title={t('plugins.languageFailed')}><Icon name="info" size={14} /><span class="sr-only">{t('plugins.languageFailed')}</span></span>{/if}
							<button type="button" class="get ic" aria-label={r.state === 'failed' ? t('accounts.language.retry') : t('plugins.installName', { name })} title={t(r.state === 'failed' ? 'accounts.language.retry' : 'plugins.install')} aria-disabled={wait} onclick={() => add(r.code)} data-testid="language-install">{#if wait}<span class="spin" aria-hidden="true"></span>{:else}<Icon name={r.state === 'failed' ? 'refresh' : 'plus'} size={15} />{/if}</button>
						{/if}
					</span>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<style>
	.langs { container-type: inline-size; }
	/* Compact rows so all languages fit one desktop window: 4 across from about 1000px, 3, 2, then 1 on a phone. */
	.ltiles { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(auto-fill, minmax(min(200px, 100%), 1fr)); gap: 6px; }
	.ltile { display: flex; align-items: center; gap: var(--s-2); min-width: 0; min-height: 40px; padding: 3px 4px 3px 12px; border-radius: 10px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.nm { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
	/* Names are never cut short: a long one wraps inside its cell. */
	.own { font-size: 14px; line-height: 1.2; font-weight: 600; letter-spacing: normal; color: var(--text); overflow-wrap: anywhere; }
	.en { font-size: 11px; line-height: 1.25; overflow-wrap: anywhere; }
	.act { flex: none; display: flex; align-items: center; gap: 4px; }
	.st { display: inline-flex; align-items: center; gap: 4px; font: var(--t-footnote); white-space: nowrap; }
	.st.ok { color: var(--ok, var(--text)); }
	.st.bad { color: var(--fail); }
	.get { display: inline-flex; align-items: center; justify-content: center; gap: 6px; min-height: 44px; min-width: 44px; padding: 0 var(--s-3); border: 0; border-radius: 999px; background: var(--surface-2); color: var(--text); font: var(--t-footnote); font-weight: 600; cursor: pointer; white-space: nowrap; }
	.get:hover:not([aria-disabled='true']) { filter: brightness(1.08); }
	.get[aria-disabled='true'] { cursor: default; opacity: 0.6; }
	.get:focus-visible { outline: 2px solid var(--focus, var(--accent)); outline-offset: 2px; }
	.get.plain { background: none; box-shadow: 0 0 0 1px var(--line); }
	/* Icon buttons: 32px with a mouse, 44px to tap. */
	.get.ic { min-width: 32px; min-height: 32px; width: 32px; height: 32px; padding: 0; }
	@media (pointer: coarse), (max-width: 600px) { .get.ic { width: 44px; height: 44px; min-width: 44px; min-height: 44px; } }
	.spin { width: 14px; height: 14px; border-radius: 50%; border: 2px solid currentColor; border-right-color: transparent; animation: lspin 0.8s linear infinite; }
	@keyframes lspin { to { transform: rotate(1turn); } }
	@media (prefers-reduced-motion: reduce) { .spin { animation-duration: 2s; } }
</style>
