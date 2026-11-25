<script lang="ts" module>
	import { api } from '#lib/core/api.ts';
	import { LOCALES, lang, setLang, type Locale } from '#lib/core/i18n/i18n.svelte.ts';
	import { pickBeforeSignIn, saveLanguage } from '#lib/accounts/index.ts';
	import { listInstalledPublic, packList } from '#lib/core/i18n/language-packs.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';

	/** The language step's outcome: a language whose pack couldn't be installed (named on Welcome), or null. */
	export const setupLang = $state<{ failed: Locale | null; main: Locale | null }>({ failed: null, main: null });
	/** Each picked language's pack: downloading, there, or failed. Kept for the visit, so Back shows the same marks. */
	export const packState = $state<Record<string, 'installing' | 'ready' | 'failed'>>({ en: 'ready' });

	// Downloads under way, so a second ask (tick, then star) waits for the same one.
	const running = new Map<string, Promise<boolean>>();
	// Removals under way: a re-tick waits for one to finish, then installs again.
	const removing = new Map<string, Promise<void>>();
	// Languages wanted again since their removal began: the removal skips its DELETE.
	const wanted = new Set<string>();

	/** Installs one pack through the first-run route now (no-op when it is already there). */
	export function installPack(code: Locale): Promise<boolean> {
		const gone = removing.get(code);
		if (gone) {
			wanted.add(code);
			return gone.then(() => installPack(code));
		}
		if (code === 'en' || packState[code] === 'ready' || packList.installed?.has(code)) {
			packState[code] = 'ready';
			return Promise.resolve(true);
		}
		const already = running.get(code);
		if (already) return already;
		packState[code] = 'installing';
		const job = api.post(`/auth/setup/language/${code}`).then(
			() => {
				packState[code] = 'ready';
				void listInstalledPublic();
				return true;
			},
			() => {
				packState[code] = 'failed';
				return false;
			}
		);
		running.set(code, job);
		void job.finally(() => running.delete(code));
		return job;
	}

	/** Unticked during setup: the pack comes off the server again (English and the default stay). */
	export async function removePack(code: Locale): Promise<void> {
		if (code === 'en' || removing.has(code)) return;
		wanted.delete(code);
		const job = (async () => {
			await running.get(code);
			// Ticked again meanwhile: keep the pack.
			if (wanted.has(code)) return;
			try {
				await api.del(`/auth/setup/language/${code}`);
			} catch {
				// Already gone, or setup closed: nothing to undo here.
			}
			delete packState[code];
			void listInstalledPublic();
		})();
		removing.set(code, job);
		await job.finally(() => {
			removing.delete(code);
			wanted.delete(code);
		});
	}

	export const languageName = (c: Locale) => LOCALES.find((l) => l.code === c)?.name ?? c;
</script>

<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { browserLocale } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import { onMount, untrack } from 'svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { pixelTrail } from '#lib/accounts/index.ts';

	let { ondone }: { ondone: () => void } = $props();

	// The browser's language leads as the default; English is always there.
	// Back keeps the starred default and every ticked pack (failed ones stay ticked so they can be retried).
	let main = $state<Locale>(setupLang.main ?? (lang.choice !== 'auto' ? lang.value : browserLocale()));
	let picked = $state<Set<Locale>>(new Set(['en', untrack(() => main), ...(Object.keys(packState) as Locale[])]));
	// Any ticked language other than the default that failed: named in a visible, spoken line.
	const otherFailed = $derived(LOCALES.find((l) => l.code !== main && picked.has(l.code) && packState[l.code] === 'failed'));

	// Shows a language as soon as its pack is there: the page changes in place, no reload.
	async function show(c: Locale) {
		if (!(await installPack(c)) || setupLang.main !== c) return;
		// A signed-in owner keeps the pick on their account, or their saved language would switch the page straight back.
		if (session.s.state === 'signed_in') await saveLanguage(c).catch(() => setLang(c));
		else await setLang(c);
	}
	onMount(async () => {
		setupLang.main = untrack(() => main);
		await listInstalledPublic();
		// After a reload: what is already on the server shows ticked.
		const have = packList.installed;
		if (have) picked = new Set([...picked, ...LOCALES.map((l) => l.code).filter((c) => have.has(c))]);
		void show(untrack(() => main));
	});

	function toggle(c: Locale) {
		// English is built in: always there, never unticked.
		if (c === 'en') return;
		const next = new Set(picked);
		// A failed pack stays ticked: a click tries again (the default also switches the page).
		if (next.has(c) && packState[c] === 'failed') {
			void (c === main ? show(c) : installPack(c));
			return;
		}
		if (next.has(c) && c !== main) {
			next.delete(c);
			void removePack(c);
		} else {
			next.add(c);
			void (c === main ? show(c) : installPack(c));
		}
		picked = next;
	}
	function makeMain(c: Locale) {
		main = c;
		setupLang.main = c;
		picked = new Set([...picked, c]);
		void show(c);
	}
	function go() {
		const m = main;
		setupLang.main = m;
		setupLang.failed = packState[m] === 'failed' ? m : null;
		const shown = packState[m] === 'ready' ? m : 'en';
		if (session.s.state === 'signed_in') void saveLanguage(shown).catch(() => {});
		else {
			pickBeforeSignIn();
			void setLang(shown);
		}
		// Still downloading: switch when it lands, wherever setup is by then.
		if (packState[m] === 'installing') void show(m);
		ondone();
	}
</script>

<h1 class="title pl-title" tabindex="-1">{t('welcome.lang.title')}</h1>
<p class="lead pl-lead">{t('welcome.lang.lead')}</p>
<ul class="langs" aria-label={t('welcome.lang.title')}>
	{#each LOCALES as l (l.code)}
		{@const on = picked.has(l.code)}
		{@const isMain = main === l.code}
		{@const st = on ? packState[l.code] : undefined}
		<li class:on class:builtin={l.code === 'en'} class:main={isMain} class:failed={st === 'failed'}>
			<button type="button" class="pick" aria-pressed={on} aria-disabled={l.code === 'en' || isMain || undefined} aria-busy={st === 'installing'} title={st === 'installing' ? t('welcome.lang.installing') : st === 'failed' ? t('welcome.lang.failedRetry') : l.code === 'en' ? t('welcome.lang.builtIn') : l.name} data-testid="setup-lang-{l.code}" onclick={() => toggle(l.code)}>
				<span class="box" aria-hidden="true">{#if st === 'installing'}<span class="spin"></span>{:else if st === 'failed'}<Icon name="alert" size={12} />{:else if on}<Icon name="check" size={14} />{/if}</span>
				<span class="nm" lang={l.code}>{l.name}</span>{#if st === 'installing' || st === 'failed'}<span class="sr-only">{st === 'installing' ? t('welcome.lang.installing') : t('welcome.lang.failedRetry')}</span>{:else if l.code === 'en'}<span class="sr-only">{t('welcome.lang.builtIn')}</span>{/if}
			</button>
			{#if on}
				<button type="button" class="star" aria-pressed={isMain} title={isMain ? t('welcome.lang.isDefault') : t('welcome.lang.makeDefault')} aria-label={`${t('welcome.lang.makeDefault')}: ${l.name}`} onclick={() => makeMain(l.code)}>
					<Icon name="star" size={14} />
				</button>
			{/if}
		</li>
	{/each}
</ul>
<p class="small hint" role="status">{packState[main] === 'installing' ? t('welcome.lang.installingName', { name: languageName(main) }) : packState[main] === 'failed' ? t('welcome.lang.later', { name: languageName(main) }) : t('welcome.lang.hint', { name: languageName(main) })}{#if otherFailed}{' '}{t('welcome.lang.someFailed', { name: otherFailed.name })}{/if}</p>
<div class="submit" use:pixelTrail={{ selector: '.ui-btn' }}>
	<Button variant="primary" size="lg" full type="button" data-testid="setup-lang-go" onclick={go}>{t('welcome.lang.go')}</Button>
</div>

<style>
	/* Same look as the other setup steps: centred heading, readable ink on both glass themes. */
	.pl-title, .pl-lead, .hint { text-align: center; }
	/* The shared title/lead look (the login page's rules are scoped there and don't reach this child). */
	.pl-title { font: var(--t-title1); letter-spacing: var(--t-title1-track); text-wrap: balance; color: #fff; }
	.pl-title:focus { outline: none; }
	.pl-lead { margin: var(--s-2) 0 var(--gap); font: var(--t-callout); text-wrap: pretty; }
	.langs { color: #fff; }
	.pl-lead { color: rgb(255 255 255 / 0.68); }
	.hint { color: rgb(255 255 255 / 0.8); }
	/* Continue: the label sits above the pixel trail, with the same lift as the other steps. */
	.submit :global(.ui-btn) { position: relative; overflow: hidden; isolation: isolate; }
	.submit :global(.ui-btn > :not(canvas)) { position: relative; z-index: 1; }
	@media (hover: hover) {
		.star:hover { opacity: 0.9; }
		.langs li:hover { background: rgb(255 255 255 / 0.08); }
		:global(.panel.light) .langs li:hover { background: rgb(0 0 0 / 0.05); }
	}
	/* English is built in: shown as locked. */
	li.builtin .box { opacity: 0.6; }
	@media (min-width: 1100px) {
		:global(.panel.setup) .pl-title { font-size: clamp(32px, 3.6vh, 40px); line-height: 1.15; }
		:global(.panel.setup) .pl-lead { font-size: 17px; }
		/* Long lists scroll inside themselves, so Continue always stays at the foot. */
		:global(.panel.setup) .langs { flex: 0 1 auto; min-height: 0; overflow-y: auto; overscroll-behavior: contain; width: min(100%, 900px); margin-inline: auto; box-sizing: border-box; padding: 2px; }
	}
	:global(.panel.light) .pl-title { color: #111; }
	:global(.panel.light) .pl-lead { color: rgb(0 0 0 / 0.74); }
	:global(.panel.light) .langs { color: #1d1d1f; }
	:global(.panel.light) .hint { color: rgb(0 0 0 / 0.62); }
	:global(.panel.light) .langs li.on { background: rgb(255 255 255 / 0.55); }
	.langs .box { border-color: currentColor; opacity: 1; }
	.spin { width: 10px; height: 10px; border-radius: 50%; border: 2px solid currentColor; border-top-color: transparent; animation: pl-spin 0.8s linear infinite; }
	li.on .box:has(.spin) { background: transparent; border-color: var(--accent); color: var(--accent); }
	li.failed .box { background: transparent; border-color: #ff6961; color: #ff6961; }
	:global(.panel.light) li.failed .box { border-color: #d70015; color: #d70015; }
	@keyframes pl-spin { to { transform: rotate(360deg); } }
	@media (prefers-reduced-motion: reduce) { .spin { animation: none; border-top-color: currentColor; opacity: 0.6; } }
	/* All 44 at once with no scrolling: four columns on desktop, three on phones (names wrap to two lines there). */
	.langs { list-style: none; margin: 4px 0 0; padding: 0; display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 4px 8px; text-align: start; }
	.langs li { position: relative; display: flex; align-items: center; min-width: 0; border-radius: var(--r-sm); }
	.langs li.on { background: rgb(255 255 255 / 0.12); }
	.pick[aria-disabled="true"] { cursor: default; }
	.pick { flex: 1; min-width: 0; display: flex; align-items: center; gap: 8px; min-height: 30px; padding: 0 8px; border: 0; background: none; color: inherit; font: inherit; font-size: 13px; text-align: start; cursor: pointer; border-radius: inherit; }
	.pick:focus-visible, .star:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
	.box { flex: none; display: grid; place-items: center; width: 16px; height: 16px; border-radius: 4px; border: 1.5px solid; }
	li.on .box { background: var(--accent); border-color: var(--accent); color: var(--on-accent, #fff); }
	.nm { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.star { position: relative; flex: none; display: grid; place-items: center; width: 26px; height: 26px; margin-inline-end: 2px; border: 0; border-radius: 50%; background: none; color: inherit; opacity: 0.55; cursor: pointer; }
	li.main .star { color: var(--accent); opacity: 1; }
	li.main .star :global(svg) { fill: currentColor; }
	/* A 44px tap area on touch without changing the look (rows stay 30px so all 44 fit). */
	@media (pointer: coarse) { .star::before { content: ''; position: absolute; inset-block: -2px; inset-inline: -9px; } }
	.hint { margin-top: 8px; }
	@media (max-width: 600px) {
		.langs { grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 2px 4px; }
		.pick { min-height: 30px; padding: 0 4px; gap: 6px; font-size: 12px; }
		.pl-title { font-size: 20px; line-height: 1.15; }
		.pl-lead { margin: var(--s-1) 0 var(--gap); }
		.star { width: 18px; height: 22px; margin-inline-end: 0; }
		.nm { white-space: normal; text-overflow: clip; overflow-wrap: normal; hyphens: auto; line-height: 1.15; font-size: 12px; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; }
	}
</style>
