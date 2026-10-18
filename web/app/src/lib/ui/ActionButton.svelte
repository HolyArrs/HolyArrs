<script lang="ts">
	import type { HTMLButtonAttributes } from 'svelte/elements';
	import Icon, { type IconName } from '../Icon.svelte';
	import { press } from '../motion.ts';

	// A round action with its word under it, for title hero action rows
	// (Apple TV style). `primary` fills it white on artwork; `pressed`
	// shows an on/off action such as Keep up to date.
	type Props = Omit<HTMLButtonAttributes, 'children'> & {
		icon: IconName;
		label: string;
		/** Shorter visible word; `label` stays the accessible name. */
		caption?: string;
		href?: string;
		primary?: boolean;
		pressed?: boolean;
		busy?: boolean;
	};
	let { icon, label, caption, href, primary = false, pressed, busy = false, type = 'button', class: cls = '', ...rest }: Props = $props();
</script>

{#if href}
	<a class="ui-act {cls}" class:primary {href} aria-label={caption && caption !== label ? label : undefined} use:press={'round'}>
		<span class="dot" aria-hidden="true"><Icon name={icon} size={22} /></span>
		<span class="cap">{caption ?? label}</span>
	</a>
{:else}
	<button
		class="ui-act {cls}"
		class:primary
		{type}
		aria-label={caption && caption !== label ? label : undefined}
		aria-pressed={pressed}
		aria-busy={busy || undefined}
		use:press={'round'}
		{...rest}
	>
		<span class="dot" aria-hidden="true"><Icon name={icon} size={22} /></span>
		<span class="cap">{caption ?? label}</span>
	</button>
{/if}

<style>
	.ui-act { display: inline-flex; flex-direction: column; align-items: center; gap: 6px; min-width: 64px; max-width: 84px; padding: 2px; color: var(--on-art); border-radius: 14px; background: none; border: 0; cursor: pointer; }
	.dot { width: 52px; height: 52px; display: grid; place-items: center; border-radius: 50%; background: var(--on-art-fill); box-shadow: inset 0 0 0 1px var(--on-art-ring); backdrop-filter: blur(18px) saturate(160%); -webkit-backdrop-filter: blur(18px) saturate(160%); transition: background var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	.ui-act:hover .dot { background: var(--on-art-fill-hover); }
	.primary .dot, .ui-act[aria-pressed='true'] .dot { background: var(--on-art); color: var(--on-art-ink); box-shadow: none; }
	.primary:hover .dot, .ui-act[aria-pressed='true']:hover .dot { background: var(--on-art-fill-strong); }
	.cap { font: var(--t-footnote); font-size: 12px; font-weight: 600; line-height: 1.2; text-align: center; color: var(--on-art-soft); text-wrap: balance; }
	.ui-act:focus-visible { outline: none; }
	.ui-act:focus-visible .dot { box-shadow: 0 0 0 2px var(--on-art-ink), 0 0 0 4px var(--on-art); }
	.ui-act:disabled { cursor: not-allowed; opacity: .5; }
	.ui-act[aria-busy='true'] .dot { animation: ui-act-busy 1.2s var(--ease-in-out) infinite; }
	@keyframes ui-act-busy { 50% { opacity: .55; } }
	@media (prefers-reduced-motion: reduce) { .ui-act[aria-busy='true'] .dot { animation: none; opacity: .7; } }
	@media (max-width: 400px) { .ui-act { min-width: 56px; } .dot { width: 48px; height: 48px; } }
</style>
