<!-- The app's language: a compact menu with search on desktop, a bottom sheet on phones. Automatic follows the browser. -->
<script lang="ts">
	import { lang, t, browserLocale, type LocaleChoice } from '#lib/core/i18n/i18n.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { message } from '#lib/core/api.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import Popover from '#lib/ui/Popover.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import { press } from '#lib/core/motion.ts';
	import { saveLanguage } from './profile.svelte.ts';
	import { languageGroups, languageName, matchLanguages } from './language-list.ts';

	let {
		id = 'language',
		variant = 'field',
		align = 'start',
		onchoose
	}: {
		id?: string;
		/** signin: a quiet corner button (globe only on phones). */
		variant?: 'field' | 'signin';
		align?: 'start' | 'end';
		/** Replaces saving to the account, for the sign-in page. */
		onchoose?: (c: LocaleChoice) => void;
	} = $props();

	const uid = $props.id();
	const listId = `${uid}-list`;
	const { top, more } = languageGroups();

	let wide = $state(typeof window !== 'undefined' && window.matchMedia('(min-width: 768px)').matches);
	$effect(() => {
		const mq = window.matchMedia('(min-width: 768px)');
		const on = () => (wide = mq.matches);
		mq.addEventListener('change', on);
		return () => mq.removeEventListener('change', on);
	});

	let open = $state(false);
	let sheetOpen = $state(false);
	let q = $state('');
	let active = $state(0);
	// Arrow keys show Menu's ring; typing shows a soft fill on what Enter picks.
	let keyed = $state(false);
	let busy = $state(false);
	let listEl = $state<HTMLElement>();
	let sheetBtn = $state<HTMLButtonElement>();

	const current = $derived(lang.choice === 'auto' ? t('accounts.language.auto') : languageName(lang.choice));
	const browser = $derived(languageName(browserLocale()));
	const name = $derived(`${t('accounts.language.label')}: ${current}`);

	const autoShown = $derived(!q.trim() || t('accounts.language.auto').toLowerCase().includes(q.trim().toLowerCase()));
	const found = $derived(matchLanguages(q, lang.value));
	const foundTop = $derived(found.filter((l) => top.includes(l)));
	const foundMore = $derived(found.filter((l) => more.includes(l)));
	const flat = $derived<LocaleChoice[]>([...(autoShown ? (['auto'] as const) : []), ...found.map((l) => l.code)]);
	const count = $derived(t(flat.length === 1 ? 'accounts.language.count.one' : 'accounts.language.count.other', { n: flat.length }));

	// Each open starts with an empty search on the current choice.
	function reset() {
		q = '';
		keyed = false;
		active = Math.max(0, flat.indexOf(lang.choice));
	}
	$effect(() => {
		void q;
		if (q.trim()) {
			active = 0;
			keyed = false;
		}
	});
	$effect(() => {
		if (!listEl || active < 0) return;
		listEl.querySelector(`[data-i="${active}"]`)?.scrollIntoView({ block: 'nearest' });
	});

	async function choose(c: LocaleChoice, close: () => void) {
		close();
		if (c === lang.choice) return;
		if (onchoose) return onchoose(c);
		busy = true;
		try {
			await saveLanguage(c);
		} catch (err) {
			toasts.push(t('accounts.language.saveFailed'), 'fail', { body: message(err) });
		} finally {
			busy = false;
		}
	}

	function keys(e: KeyboardEvent, close: () => void) {
		const n = flat.length;
		if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
			e.preventDefault();
			if (!n) return;
			active = keyed || q.trim() ? (active + (e.key === 'ArrowDown' ? 1 : -1) + n) % n : active;
			keyed = true;
		} else if (e.key === 'Enter') {
			e.preventDefault();
			if (flat[active]) void choose(flat[active], close);
		} else if (e.key === 'Tab') {
			// The panel sits at the page's top level, so Tab goes back to the button.
			e.preventDefault();
			close();
		} else if (e.key === 'Escape' && !wide && q) {
			// In the sheet, the first Escape clears the search.
			e.preventDefault();
			e.stopPropagation();
			q = '';
		}
	}

	function openSheet() {
		reset();
		sheetOpen = true;
		// Touch opens on the list so the keyboard stays down.
		setTimeout(() => listEl?.focus({ preventScroll: true }), 30);
	}
	const closeSheet = () => {
		sheetOpen = false;
		sheetBtn?.focus({ preventScroll: true });
	};
</script>

{#snippet row(c: LocaleChoice, label: string, i: number, close: () => void)}
	<div
		id="{uid}-o{i}"
		data-i={i}
		class="item"
		class:soft={active === i && !keyed && !!q.trim()}
		class:ring={active === i && keyed}
		role="option"
		tabindex="-1"
		aria-selected={lang.choice === c}
		lang={c === 'auto' ? undefined : c}
		onpointerdown={(e) => e.preventDefault()}
		onclick={() => choose(c, close)}
		onkeydown={(e) => e.key === 'Enter' && choose(c, close)}
	>
		<span class="ic">{#if lang.choice === c}<Icon name="check" size={17} />{/if}</span><span class="grow">{label}</span>{#if c === 'auto'}<kbd class="hint">{browser}</kbd>{/if}
	</div>
{/snippet}

{#snippet body(close: () => void)}
	<div class="lp" class:sheet={!wide}>
		<div class="lp-find">
			<Icon name="search" size={16} />
			<input
				class="lp-search"
				type="text"
				role="combobox"
				autocomplete="off"
				spellcheck="false"
				aria-expanded="true"
				aria-controls={listId}
				aria-autocomplete="list"
				aria-activedescendant={flat.length ? `${uid}-o${active}` : undefined}
				aria-label={t('frame.layout.search')}
				placeholder={t('frame.layout.search')}
				bind:value={q}
				onkeydown={(e) => keys(e, close)}
				data-testid="language-search"
			/>
		</div>
		<div class="list" id={listId} role="listbox" tabindex="-1" aria-label={t('accounts.language.label')} bind:this={listEl} onkeydown={(e) => keys(e, close)}>
			{#if autoShown}{@render row('auto', t('accounts.language.auto'), 0, close)}{/if}
			{#if foundTop.length}
				<div role="group" aria-labelledby="{uid}-gt">
					<div class="heading" id="{uid}-gt" role="presentation">{t('accounts.language.top')}</div>
					{#each foundTop as l, k (l.code)}{@render row(l.code, l.name, (autoShown ? 1 : 0) + k, close)}{/each}
				</div>
			{/if}
			{#if foundMore.length}
				<div role="group" aria-labelledby="{uid}-gm">
					<div class="heading" id="{uid}-gm" role="presentation">{t('accounts.language.more')}</div>
					{#each foundMore as l, k (l.code)}{@render row(l.code, l.name, (autoShown ? 1 : 0) + foundTop.length + k, close)}{/each}
				</div>
			{/if}
			{#if !flat.length}<p class="none">{t('downloads.empty.noMatches', { q: q.trim() })}</p>{/if}
		</div>
		<span class="sr" aria-live="polite">{q.trim() ? count : ''}</span>
	</div>
{/snippet}

<span class="ui-langpick {variant}" {id} tabindex="-1" data-testid="language-picker">
	{#if wide}
		<Popover
			label={name}
			variant="button"
			{align}
			width={220}
			portal
			haspopup="listbox"
			panelRole="none"
			panelClass="lp-panel"
			bind:open
			ontriggerkey={(e) => {
				if ((e.key === 'ArrowDown' || e.key === 'ArrowUp') && !open) {
					e.preventDefault();
					open = true;
				}
			}}
			onopen={(p) => {
				reset();
				p.querySelector<HTMLInputElement>('.lp-search')?.focus({ preventScroll: true });
			}}
		>
			{#snippet trigger()}
				<Icon name="globe" size={variant === 'signin' ? 16 : 17} /><span class="cur">{current}</span>{#if variant === 'field'}<Icon name="chevronDown" size={14} />{/if}
			{/snippet}
			{#snippet children(close)}{@render body(close)}{/snippet}
		</Popover>
	{:else}
		<button
			bind:this={sheetBtn}
			class="trigger {variant === 'signin' ? 'icon' : 'button'}"
			type="button"
			aria-label={name}
			aria-haspopup="dialog"
			aria-expanded={sheetOpen}
			disabled={busy}
			onclick={openSheet}
			use:press={variant === 'signin' ? 'round' : 'button'}
		>
			<Icon name="globe" size={variant === 'signin' ? 19 : 17} />{#if variant === 'field'}<span class="cur">{current}</span><Icon name="chevronDown" size={14} />{/if}
		</button>
		<Sheet bind:open={sheetOpen} title={t('accounts.language.label')} size="sm" onclose={() => sheetBtn?.focus({ preventScroll: true })}>
			{@render body(closeSheet)}
		</Sheet>
	{/if}
</span>

<style>
	.ui-langpick { display: inline-flex; max-width: 100%; outline: none; }
	.trigger { display: inline-flex; align-items: center; gap: 6px; color: var(--text); }
	.trigger:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.trigger.button { min-height: 36px; padding: 0 12px 0 14px; border-radius: var(--r-btn); background: var(--surface-2); font: var(--t-headline); font-size: 15px; white-space: nowrap; }
	.trigger.icon { width: 44px; height: 44px; justify-content: center; border-radius: 50%; color: var(--text-muted); }
	.cur { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

	/* Menu's look: 36px one-line rows, tick column, small grey group names. */
	.lp { display: flex; flex-direction: column; }
	/* Sticky over Popover's 6px padding; Chrome counts sticky from inside the padding. */
	.lp-find { position: sticky; top: -6px; z-index: 1; margin: -6px -6px 4px; padding: 6px 6px 0; background: inherit; display: flex; align-items: center; }
	:global(.lp-panel) .lp-find { background: var(--surface-1); }
	:global(html[data-tint] .lp-panel) .lp-find { background: color-mix(in srgb, var(--tint-bg, #1d1d22) 86%, #fff); }
	.lp-find :global(svg) { position: absolute; inset-inline-start: 16px; color: var(--text-muted); pointer-events: none; }
	input.lp-search { width: 100%; height: 36px; min-height: 36px; padding: 0 10px; padding-inline-start: 38px; border: 0; border-radius: 8px; background: color-mix(in srgb, var(--text) 6%, transparent); color: var(--text); font: var(--t-callout); font-size: 15px; box-shadow: none; }
	input.lp-search::placeholder { color: var(--text-faint, var(--text-muted)); }
	input.lp-search:focus-visible { outline: none; box-shadow: inset 0 0 0 2px var(--accent); }
	.list { display: flex; flex-direction: column; outline: none; }
	.item { display: flex; align-items: center; gap: 10px; width: 100%; min-height: 36px; padding-inline: 8px 10px; border-radius: 8px; color: var(--text); font: var(--t-callout); font-size: 15px; text-align: start; cursor: pointer; }
	.item:hover, .item.soft { background: var(--surface-3); }
	.item.ring { background: var(--surface-3); box-shadow: inset 0 0 0 2px var(--accent); }
	.ic { width: 20px; display: grid; place-items: center; color: var(--text-muted); flex: none; }
	.grow { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.hint { flex: 0 1 auto; min-width: 0; overflow: hidden; white-space: nowrap; text-overflow: clip; color: var(--text-muted); font: var(--t-footnote); position: relative; top: 2px; }
	.heading { padding-block: 8px 4px; padding-inline: 38px 10px; color: var(--text-muted); font: var(--t-footnote); font-weight: 600; }
	.none { margin: 0; padding: 10px 10px 8px; padding-inline-start: 38px; color: var(--text-muted); font: var(--t-callout); font-size: 14px; overflow-wrap: anywhere; }
	.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
	/* Phone sheet: touch-size rows. */
	.sheet .lp-find { position: sticky; top: 0; margin: 0 0 6px; padding: 0; }
	.sheet .item { min-height: 44px; }
	@media (pointer: coarse) { .item { min-height: 44px; } .trigger.button { min-height: 44px; } }
</style>
