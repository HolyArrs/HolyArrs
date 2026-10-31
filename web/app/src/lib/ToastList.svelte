<script lang="ts">
	import { onMount } from 'svelte';
	import { dropIn, dropOut } from './ui/transitions.ts';
	import { toasts } from './toasts.svelte';
	import Toast from './ui/Toast.svelte';
	import {
		METADATA_BACK,
		METADATA_DOWN,
		METADATA_DOWN_TITLE,
		clearMetadataBack,
		dismissMetadataDown,
		metadataDownShown,
		metadataOutage,
		watchMetadata
	} from './metadata-health.svelte.ts';
	import { session } from './session.svelte.ts';

	// All notifications slide up at the bottom centre (above the tab bar on
	// phones) with a soft spring and leave downwards, quicker; reduced motion fades only.
	const toastIn = dropIn;
	const toastOut = dropOut;

	// Metadata outage: full card, shrinks to a pill after 6 s, tap to open again.
	const downShown = $derived(metadataDownShown());
	let compact = $state(false);
	let shrinkTimer: ReturnType<typeof setTimeout> | undefined;
	function openCard() {
		compact = false;
		clearTimeout(shrinkTimer);
		shrinkTimer = setTimeout(() => (compact = true), 6000);
	}
	$effect(() => {
		if (downShown) openCard();
		else clearTimeout(shrinkTimer);
	});

	// Recovery: a short green note, gone after 3 s.
	const back = $derived(metadataOutage.back > 0);
	$effect(() => {
		if (!back) return;
		const t = setTimeout(clearMetadataBack, 3000);
		return () => clearTimeout(t);
	});

	// Health checks need a signed-in session (the endpoint answers 401
	// before sign-in, which would read as an outage). Sign-in itself never
	// needs metadata, so nothing is checked or shown until signed in.
	const signedIn = $derived(session.s.state === 'signed_in');
	$effect(() => {
		if (!signedIn) return;
		return watchMetadata();
	});
	onMount(() => () => clearTimeout(shrinkTimer));

	// Toasts live in the top layer (a manual popover), so they stay above an open
	// <dialog> such as the Requests window and Undo can still be clicked. Shown again
	// on every new toast, so they land above whatever dialog opened since.
	let host = $state<HTMLElement>();
	$effect(() => {
		void toasts.list.length;
		void downShown;
		void back;
		const el = host;
		if (!el || typeof el.showPopover !== 'function') return;
		try {
			if (el.matches(':popover-open')) el.hidePopover();
			el.showPopover();
		} catch {
			// popover unsupported: the fixed layer still shows outside dialogs
		}
	});
</script>

<div class="toasts" role="status" aria-live="polite" popover="manual" bind:this={host}>
	{#if signedIn && downShown}
		<div class="slot" in:toastIn out:toastOut data-testid="metadata-down">
			<Toast tone="warn" icon="cloudOff" title={METADATA_DOWN_TITLE} text={METADATA_DOWN} time="now" {compact} onexpand={openCard} ondismiss={dismissMetadataDown} />
		</div>
	{:else if signedIn && back}
		<div class="slot" in:toastIn out:toastOut data-testid="metadata-back">
			<Toast tone="ok" icon="check" title={METADATA_BACK} text="Search and new artwork are back." time="now" ondismiss={clearMetadataBack} />
		</div>
	{/if}
	{#each toasts.list as t (t.id)}
		<div class="slot" in:toastIn out:toastOut>
			<Toast text={t.text} tone={t.tone} action={t.action ? { label: t.action.label, run: () => { t.action?.run(); toasts.dismiss(t.id); } } : undefined} ondismiss={() => toasts.dismiss(t.id)} />
		</div>
	{/each}
</div>

<style>
	.toasts {
		/* Undo the popover defaults; keep the bottom-centre placement. */
		inset: auto;
		margin: 0;
		padding: 0;
		border: 0;
		background: none;
		color: inherit;
		overflow: visible;
		position: fixed;
		z-index: 90;
		left: 50%;
		bottom: calc(var(--s-4, 16px) + env(safe-area-inset-bottom, 0px));
		transform: translateX(-50%);
		display: flex;
		flex-direction: column;
		gap: var(--s-2);
		width: min(400px, calc(100vw - 32px));
		pointer-events: none;
	}
	@media (max-width: 767px) {
		.toasts { bottom: calc(var(--tabbar, 64px) + 12px + env(safe-area-inset-bottom, 0px)); }
	}
	.slot { display: flex; flex-direction: column; }
</style>
