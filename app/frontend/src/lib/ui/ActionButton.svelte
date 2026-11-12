<script lang="ts">
	// A glass action button or link, round with a hover label or a pill with text.
	import type { HTMLButtonAttributes } from 'svelte/elements';
	import Icon, { type IconName } from './Icon.svelte';
	import { press } from '../core/motion.ts';

	type Props = Omit<HTMLButtonAttributes, 'children'> & {
		icon: IconName;
		label: string;
		caption?: string;
		href?: string;
		primary?: boolean;
		pill?: boolean;
		pressed?: boolean;
		busy?: boolean;
	};
	let { icon, label, caption, href, primary = false, pill = false, pressed, busy = false, type = 'button', class: cls = '', ...rest }: Props = $props();
	// Icon-only round button unless it is primary or a pill.
	const round = $derived(!primary && !pill);
	const word = $derived(caption ?? label);
	// Round or differing caption hides the visible text from readers, so name it.
	const aria = $derived(round || (caption && caption !== label) ? label : undefined);
</script>

<!-- Shared body for the link and button forms. -->
{#snippet inner()}
	<Icon name={icon} size={round ? 20 : 18} />
	{#if round}<span class="tip" aria-hidden="true">{word}</span>{:else}<span class="cap">{word}</span>{/if}
{/snippet}

{#if href}
	<a class="ui-act {cls}" class:primary class:round {href} aria-label={aria} use:press={round ? 'round' : 'button'}>{@render inner()}</a>
{:else}
	<button
		class="ui-act {cls}"
		class:primary
		class:round
		{type}
		aria-label={aria}
		aria-pressed={pressed}
		aria-busy={busy || undefined}
		use:press={round ? 'round' : 'button'}
		{...rest}
	>
		{@render inner()}
	</button>
{/if}

<style>
	.ui-act {
		position: relative; display: inline-flex; align-items: center; justify-content: center; gap: 8px; height: 44px; min-width: 44px; padding: 0 20px 0 16px;
		border-radius: 999px; border: 1px solid rgb(255 255 255 / 22%); background: rgb(255 255 255 / 14%); color: #fff;
		backdrop-filter: blur(20px) saturate(170%); -webkit-backdrop-filter: blur(20px) saturate(170%);
		font: 600 15px/1 var(--font-ui); letter-spacing: -0.005em; white-space: nowrap; cursor: pointer;
		transition: background var(--dur-fast) var(--ease-out), border-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out);
	}
	.ui-act:hover { background: rgb(255 255 255 / 24%); border-color: rgb(255 255 255 / 34%); }
	.round { width: 44px; padding: 0; }
	.primary { background: #fff; border-color: #fff; color: #111; padding: 0 24px 0 20px; box-shadow: 0 6px 20px rgb(0 0 0 / 22%); backdrop-filter: none; -webkit-backdrop-filter: none; }
	.primary:hover { background: rgb(255 255 255 / 88%); border-color: transparent; }
	.round[aria-pressed='true'] { background: rgb(255 255 255 / 92%); border-color: transparent; color: #111; }
	.round[aria-pressed='true']:hover { background: #fff; }
	.cap { overflow: hidden; text-overflow: ellipsis; }
	.ui-act:focus-visible { outline: none; box-shadow: 0 0 0 2px #111, 0 0 0 4px #fff; }
	.ui-act:disabled { cursor: not-allowed; opacity: var(--disabled-opacity); }
	.ui-act[aria-busy='true'] { animation: ui-act-busy 1.2s var(--ease-in-out) infinite; }
	/* Hover label for the icon-only form. */
	.tip {
		position: absolute; left: 50%; top: calc(100% + 8px); z-index: 5; translate: -50% -4px; padding: 5px 9px; border-radius: 7px;
		background: rgb(20 20 22 / 92%); color: #fff; font: 600 12px/1.2 var(--font-ui); white-space: nowrap; pointer-events: none;
		opacity: 0; transition: opacity var(--dur-fast) var(--ease-out), translate var(--dur-fast) var(--ease-out); box-shadow: 0 6px 18px rgb(0 0 0 / 30%);
	}
	/* Delay so the label shows on rest, not on a pass-over. */
	.round:hover .tip, .round:focus-visible .tip { opacity: 1; translate: -50% 0; transition-delay: 500ms; }
	@keyframes ui-act-busy { 50% { opacity: .6; } }
	@media (prefers-reduced-motion: reduce) { .ui-act[aria-busy='true'] { animation: none; opacity: .75; } .tip { transition: none; } }
	/* Touch has no hover, so the label would stick. */
	@media (hover: none) { .tip { display: none; } }
</style>
