<script lang="ts">
	import Icon, { type IconName } from '../Icon.svelte';
	import TopMenu from './TopMenu.svelte';
	import DlFind from './DlFind.svelte';
	import { tip } from './tip.ts';
	import { KIND_TABS } from './size.ts';

	// The Downloads toolbar: three glass capsules. Actions on the left; Library and Sources
	// on the right. Every button is the same width (icon over label) so highlights sit centred.
	// The page has no top bar: the sidebar button (when the sidebar is folded) joins the actions
	// capsule behind a hairline, and the plain Downloads search fills the gap before the filters.
	type Act = { key: string; icon: IconName; label: string; hint: string; disabled: boolean; run?: () => void; href?: string; more?: { label: string; onselect: () => void }[] };
	let {
		kindOn = $bindable(''),
		srcOn = $bindable(''),
		kindCount,
		srcCount,
		actions,
		watch = false,
		term = $bindable(''),
		shown,
		total,
		side = false,
		sideControls = 'sidebar',
		onside,
		searchKey = 'Ctrl K'
	}: {
		kindOn?: string;
		srcOn?: string;
		kindCount: (k: string) => number;
		srcCount: (k: string) => number;
		actions: Act[];
		watch?: boolean;
		term?: string;
		shown: number;
		total: number;
		side?: boolean;
		sideControls?: string;
		onside?: () => void;
		searchKey?: string;
	} = $props();

	const KIND_ICON: Record<string, IconName> = { movie: 'film', show: 'tv', artist: 'music' };
	const kinds = KIND_TABS.filter((t) => t.key).map((t) => ({ key: t.key as string, label: t.label, icon: KIND_ICON[t.key] }));
	const sources: { key: string; label: string; icon: IconName }[] = [
		{ key: 'usenet', label: 'Usenet', icon: 'usenet' },
		{ key: 'torrent', label: 'Torrent', icon: 'torrent' }
	];
	type F = { key: string; label: string; icon: IconName };
	// The accessible name starts with the visible label (WCAG 2.5.3); a longer hint follows it.
	const nameOf = (a: Act) => (a.hint && a.hint !== a.label ? `${a.label} — ${a.hint}` : a.label);

	const pickKind = (k: string) => (kindOn = kindOn === k ? '' : k);
	const pickSrc = (k: string) => (srcOn = srcOn === k ? '' : k);
	const showSide = $derived(side && !!onside);

	// The sliding highlight: each segment measures its picked item and moves one shared plate to it.
	function slider(node: HTMLElement, _dep: string) {
		const place = () => {
			const on = node.querySelector<HTMLElement>('[aria-pressed="true"]');
			if (!on) {
				node.style.setProperty('--io', '0');
				return;
			}
			node.style.setProperty('--ix', `${on.offsetLeft}px`);
			node.style.setProperty('--iw', `${on.offsetWidth}px`);
			node.style.setProperty('--ih', `${on.offsetHeight}px`);
			node.style.setProperty('--iy', `${on.offsetTop}px`);
			node.style.setProperty('--io', '1');
		};
		place();
		const ro = new ResizeObserver(place);
		ro.observe(node);
		let frame = 0;
		return {
			update: () => {
				cancelAnimationFrame(frame);
				frame = requestAnimationFrame(place);
			},
			destroy: () => {
				cancelAnimationFrame(frame);
				ro.disconnect();
			}
		};
	}
</script>

{#snippet act(a: Act)}
	<span class="aw" data-testid="tb-{a.key}">
		<span class="tw2" use:tip={a.hint !== a.label ? a.hint : ''}>
			{#if a.href}
				<a class="it act stk" href={a.href} aria-label={nameOf(a)} data-testid={a.key === 'turned' ? 'tb-turned-down' : undefined}><span class="ic"><Icon name={a.icon} size={20} /></span><span class="lb">{a.label}</span></a>
			{:else}
				<button type="button" class="it act stk" disabled={a.disabled} aria-label={nameOf(a)} onclick={() => a.run?.()} data-testid={a.key === 'remove' ? 'tb-remove' : `tb-${a.key}-btn`}><span class="ic"><Icon name={a.icon} size={20} /></span><span class="lb">{a.label}</span></button>
			{/if}
		</span>
		{#if a.more}<TopMenu label="More {a.label.toLowerCase()} actions" items={a.more} />{/if}
	</span>
{/snippet}

{#snippet seg(label: string, list: F[], current: string, count: (k: string) => number, pick: (k: string) => void, tid: string, gtid: string)}
	<div class="seg" role="group" aria-label={label} data-testid={gtid} use:slider={current}>
		<i class="plate" aria-hidden="true"></i>
		{#each list as f (f.key)}
			{@const n = count(f.key)}
			{@const on = current === f.key}
			<button type="button" class="it fl stk" class:on class:zero={n === 0} aria-pressed={on} onclick={() => pick(f.key)} data-testid={tid} data-key={f.key} data-count={n}>
				<span class="ic"><Icon name={f.icon} size={20} /></span><span class="lb">{f.label}</span>
			</button>
		{/each}
	</div>
{/snippet}

{#snippet acts()}
	{#if !watch}<div class="grp acts" role="toolbar" aria-label="Download actions" data-testid="dl-toolbar">{#each actions as a (a.key)}{@render act(a)}{/each}</div>{/if}
{/snippet}
{#snippet library()}{@render seg('Library', kinds, kindOn, kindCount, pickKind, 'bar-kind', 'dl-type-filter')}{/snippet}
{#snippet srcs()}{@render seg('Sources', sources, srcOn, srcCount, pickSrc, 'bar-source', 'dl-source-filter')}{/snippet}
{#snippet sideBtn()}
	<span class="tw2" use:tip={'Show sidebar'}><button type="button" class="it side" aria-label="Show sidebar" aria-expanded={false} aria-controls={sideControls} onclick={() => onside?.()} data-testid="dl-side-btn"><span class="ic"><Icon name="sidebar" size={20} /></span></button></span>
{/snippet}

<div class="dlbar" data-testid="dl-bar">
	<div class="row">
		{#if showSide || !watch}<div class="cap" class:sidecap={watch}>{#if showSide}{@render sideBtn()}{#if !watch}<span class="vr ph" aria-hidden="true"></span>{/if}{/if}{@render acts()}</div>{/if}
		<DlFind bind:term {shown} {total} {searchKey} />
		<div class="cap">{@render library()}</div><div class="cap">{@render srcs()}</div>
	</div>
</div>

<style>
	.dlbar { --ez: cubic-bezier(0.32, 0.72, 0, 1); flex: 1 1 0; min-width: 0; display: grid; gap: 4px; }
	.row { display: flex; align-items: center; gap: 8px; min-width: 0; padding: 4px 0; overflow-x: auto; overflow-y: hidden; overscroll-behavior-x: contain; scrollbar-width: none; }
	.row::-webkit-scrollbar { display: none; }
	.grp, .seg { display: flex; flex: none; align-items: center; gap: 2px; padding: 0; margin: 0; border: 0; background: none; }
	.aw, .tw2 { display: inline-flex; align-items: center; flex: none; }

	/* The glass capsule: 52px tall, 3px inset, so every 44px button sits centred. */
	.cap { display: flex; flex: none; align-items: center; height: 52px; box-sizing: border-box; padding: 3px; border: 1px solid var(--line); border-radius: 14px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); backdrop-filter: blur(18px) saturate(1.4); -webkit-backdrop-filter: blur(18px) saturate(1.4); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); }
	.cap > .grp, .cap > .seg { height: 44px; min-height: 0; align-items: stretch; }
	.dlbar .seg > .it, .dlbar .grp > .aw { margin: 0; }

	.it { position: relative; z-index: 1; display: inline-flex; align-items: center; flex: none; gap: 6px; padding: 0 10px; border: 0; border-radius: var(--r-btn); background: none; color: var(--text); font: var(--t-footnote); white-space: nowrap; text-decoration: none; cursor: pointer; transition: background var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out), transform 180ms var(--ez); }
	.it .ic { position: relative; display: grid; place-items: center; color: inherit; transition: transform 180ms var(--ez); }
	.it .lb { display: inline-flex; align-items: center; gap: 4px; }
	.dlbar .it.stk { display: grid; grid-template-rows: 18px 12px; align-content: center; justify-items: center; gap: 3px; width: 68px; min-width: 0; height: 44px; padding: 0; border-radius: 10px; }
	.dlbar .it.stk .ic { height: 18px; }
	.dlbar .it.stk .lb { font: 500 11px/1 var(--font-ui); letter-spacing: 0; color: var(--text-muted); transition: color 160ms var(--ease-out); }
	.it:hover:not(:disabled) .lb { color: var(--text); }
	.dlbar .it:hover:not(:disabled) { background: color-mix(in srgb, var(--text) 7%, transparent); }
	.dlbar .it:active:not(:disabled) { transform: scale(0.97); }
	.it:disabled { cursor: not-allowed; }
	/* Not usable until a download is picked: keep the shape crisp, dim only the ink. */
	.dlbar .it:disabled .ic, .dlbar .it:disabled .lb { color: color-mix(in srgb, var(--text) 38%, transparent); }
	.it:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.it.zero:not(.on) { opacity: 0.72; }
	.dlbar .it.on, .dlbar .it.on .lb { color: var(--text); font-weight: 500; }
	.dlbar .it.on .ic { color: var(--text); }

	/* The picked filter gets the status tabs' raised plate, which slides between items. */
	.seg { position: relative; }
	.plate { position: absolute; z-index: 0; left: 0; top: 0; width: var(--iw, 0); height: var(--ih, 0); border-radius: 10px; background: var(--seg-thumb); box-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.12), 0 2px 8px rgb(0 0 0 / 0.08); opacity: var(--io, 0); transform: translate(var(--ix, 0), var(--iy, 0)); transition: transform 320ms var(--ez), width 320ms var(--ez), opacity 160ms var(--ease-out); pointer-events: none; }
	.dlbar .seg .it.on:hover { background: none; }
	.dlbar .aw { position: relative; }
	.dlbar .aw :global(.caret) { position: absolute; top: 3px; right: 3px; z-index: 2; width: 16px; height: 16px; min-width: 0; margin: 0; padding: 0; border-radius: 5px; opacity: 0.55; }
	.dlbar .aw :global(.caret:hover) { opacity: 1; background: color-mix(in srgb, var(--text) 10%, transparent); }
	.dlbar .aw :global(.caret svg) { width: 11px; height: 11px; }

	/* The sidebar button, folded into the actions capsule behind a hairline. */
	.dlbar .it.side { justify-content: center; width: 44px; height: 44px; padding: 0; border-radius: 10px; }
	.vr { flex: none; align-self: center; width: 1px; height: 26px; margin: 0 4px; background: var(--line); }

	/* Wide screens: nothing needs to scroll. */
	@media (min-width: 1100px) {
		.row { overflow: visible; }
	}
	/* Tablets and phones: the row scrolls inside itself (no page scroll); its edge fades to show there is more. */
	@media (max-width: 1099px) {
		.row > :global(.dlfind) { min-width: 200px; }
	}
	/* Phones: no sidebar (the tab bar); the page's own magnifier opens the search. */
	@media (max-width: 767px) {
		.row { -webkit-mask-image: linear-gradient(90deg, #000 calc(100% - 20px), transparent); mask-image: linear-gradient(90deg, #000 calc(100% - 20px), transparent); }
		.dlbar .it.stk { width: 58px; }
		.row > :global(.dlfind), .sidecap, .dlbar .it.side, .vr.ph { display: none; }
	}
	@media (prefers-reduced-motion: reduce) {
		.it, .it .ic, .plate, .dlbar .it.stk .lb { transition: none; }
		.dlbar .it:active:not(:disabled) { transform: none; }
	}
</style>
