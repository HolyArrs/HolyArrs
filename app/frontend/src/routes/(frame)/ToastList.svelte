<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { decisionPins, feedPins, forgetFeeds } from './bell-feeds.svelte.ts';
	import Toast from './Toast.svelte';
	import { AlertsPanel } from '#lib/alerts/index.ts';
	import { metadataBackTitle, metadataDownTitle, clearMetadataBack, dismissMetadataDown, metadataDownShown, metadataOutage, watchMetadata } from '#lib/metadata/index.ts';
	import { onMount, untrack } from 'svelte';
	import { notifyIn, notifyOut } from '#lib/core/transitions.ts';
	import { alertsPanel, toasts } from '#lib/core/toasts.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';

	// Notifications drop in at the top right under the top bar (top, full width on phones) with a soft spring and leave upwards, quicker; reduced motion fades only.
	const toastIn = notifyIn;
	const toastOut = notifyOut;

	// Screen readers: the stack itself is a polite live region, so each alert is read as it drops in, with no hidden second copy of its text.

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
	// The outage and the recovery are kept in Recent alerts too (the card itself is not a toast).
	$effect(() => {
		if (signedIn && downShown) untrack(() => toasts.record(metadataDownTitle(), 'warn', { body: t('frame.toasts.libraryWorks') }));
	});
	$effect(() => {
		if (signedIn && back) untrack(() => toasts.record(metadataBackTitle(), 'ok', { body: t('frame.toasts.restored') }));
	});

	// Recovery: a short green note, gone after 3 s.
	const back = $derived(metadataOutage.back > 0);
	$effect(() => {
		if (!back) return;
		const t = setTimeout(clearMetadataBack, 3000);
		return () => clearTimeout(t);
	});

	// Health checks need a signed-in session (the endpoint answers 401 before sign-in, which would read as an outage). Sign-in itself never needs metadata, so nothing is checked or shown until signed in.
	const signedIn = $derived(session.s.state === 'signed_in');
	// Recent alerts belong to this sign-in: signing out clears them.
	$effect(() => {
		if (session.s.state === 'signed_out') {
			toasts.forgetAll();
			forgetFeeds();
			alertsPanel.open = false;
		}
	});
	$effect(() => {
		if (!signedIn) return;
		return watchMetadata();
	});
	onMount(() => () => clearTimeout(shrinkTimer));

	// Toasts live in the top layer (a manual popover), so they stay above an open <dialog> such as the Requests window and Undo can still be clicked. Shown again on every new toast, so they land above whatever dialog opened since.
	let host = $state<HTMLElement>();
	let home = $state<HTMLElement>();
	let inWin = $state(false);
	function raise() {
		const el = host;
		if (!el || typeof el.showPopover !== 'function') return;
		try {
			if (el.matches(':popover-open')) el.hidePopover();
			el.showPopover();
		} catch {
			// Popover unsupported: the fixed layer still shows outside dialogs.
		}
	}
	// A modal <dialog> makes everything outside it inert, a top-layer popover included, so while one is open the toasts move inside it (Undo stays clickable, and a click on a toast never reads as a click on the dialog's backdrop). They move back when it closes.
	function place() {
		const el = host;
		if (!el || !home) return;
		const open = [...document.querySelectorAll<HTMLDialogElement>('dialog[open]')].filter((d) => {
			try {
				return d.matches(':modal');
			} catch {
				return true;
			}
		});
		const want: Node = open.at(-1) ?? home.parentNode!;
		if (el.parentNode === want) return;
		if (want === home.parentNode) home.after(el);
		else want.appendChild(el);
		inWin = want !== home.parentNode;
		raise();
	}
	onMount(() => {
		place();
		// A dialog removed while still open (a page left mid close animation) takes the toasts with it and fires no 'open' change, so a detached host is brought home as well.
		const mo = new MutationObserver((recs) => {
			if ((host && !host.isConnected) || recs.some((r) => r.type === 'attributes')) place();
		});
		mo.observe(document.body, { subtree: true, childList: true, attributes: true, attributeFilter: ['open'] });
		return () => {
			mo.disconnect();
			// Back home before Svelte removes it, so it never lingers inside a dialog.
			if (host && home && host.parentNode !== home.parentNode) home.after(host);
		};
	});
	// Preview builds only (VITE_ALERTS_DEMO): a button that plays sample alerts, for checking the look on real pages. Normal builds drop this code.
	const demo = !!import.meta.env.VITE_ALERTS_DEMO;
	let step = 0;
	// Built only into the preview: normal builds see `false ? [...] : []` and drop the samples.
	const samples: (() => void)[] = import.meta.env.VITE_ALERTS_DEMO ? [
		() => toasts.push('Dune: Part Two requested', 'ok', { body: 'Waiting for approval' }),
		() => toasts.push('Severance deleted', 'ok', { body: 'Files kept 7 days', ms: 10000, action: { label: 'Undo', run: () => toasts.push('Severance restored', 'ok') } }),
		() => toasts.push("Couldn't reach qBittorrent", 'fail', { body: 'Check Settings › Downloaders' }),
		() => toasts.push('Searching for The Bear', 'info'),
		() => toasts.push('API key copied', 'ok'),
		() => [1, 2, 3, 4].forEach((n) => toasts.push(`Andor S2E${n} marked failed`, 'info', { body: 'Searching for the next Release' })),
		// Earlier alerts, so Recent alerts shows how times read (now, 5m, 1h, 3h, 2d).
		() => {
			const ago: [string, 'ok' | 'fail' | 'info', string | undefined, number][] = [
				['Andor added', 'ok', 'Searching for a Release', 2 * 86400],
				["Couldn't reach Prowlarr", 'fail', 'Check Settings › Sources', 3 * 3600],
				['Backup made', 'ok', undefined, 3600],
				['Searching for The Bear', 'info', undefined, 5 * 60]
			];
			for (const [title, tone, body, secs] of ago) {
				const id = toasts.push(title, tone, { body });
				const row = toasts.past.find((p) => p.id === id);
				if (row) row.at = Date.now() - secs * 1000;
			}
			toasts.push('Severance deleted', 'ok', { body: 'Files kept 7 days', ms: 10000, action: { label: 'Undo', run: () => toasts.push('Severance restored', 'ok') } });
		},
		() => Array.from({ length: 10 }, (_, i) => i + 1).forEach((n) => toasts.push(`The Bear S3E${n} added`, 'ok', { body: 'Searching for a Release' }))
	] : [];
	function playDemo() {
		if (samples.length) samples[step++ % samples.length]();
	}
	onMount(() => {
		if (demo) (window as unknown as { __holyarrsToasts?: typeof toasts }).__holyarrsToasts = toasts;
	});
	$effect(() => {
		void toasts.list.length;
		void downShown;
		void back;
		place();
		raise();
	});
</script>

{#if demo && signedIn}<button type="button" class="alerts-demo" onclick={playDemo}>Try alerts</button>{/if}
<span class="toast-home" hidden bind:this={home}></span>
<div class="toasts" class:in-win={inWin} role="status" aria-live="polite" popover="manual" bind:this={host}>
	{#if signedIn && downShown}
		<div class="slot" in:toastIn out:toastOut data-testid="metadata-down">
			<Toast tone="warn" icon="cloudOff" title={metadataDownTitle()} text={t('frame.toasts.libraryWorks')} time="now" {compact} onexpand={openCard} ondismiss={dismissMetadataDown} />
		</div>
	{:else if signedIn && back}
		<div class="slot" in:toastIn out:toastOut data-testid="metadata-back">
			<Toast tone="ok" icon="check" title={metadataBackTitle()} text={t('frame.toasts.restored')} time="now" ondismiss={clearMetadataBack} />
		</div>
	{/if}
	<!-- While Recent alerts is open the pills step aside: every alert is in the list. -->
	{#each alertsPanel.open ? [] : toasts.list as t (t.id)}
		<div class="slot" in:toastIn out:toastOut>
			<Toast title={t.body ? t.text : undefined} text={t.body ?? t.text} tone={t.tone} action={t.action ? { label: t.action.label, run: () => toasts.runPast(t.id) } : undefined} ondismiss={() => toasts.dismiss(t.id)} onhold={() => toasts.pause(t.id)} onlet={() => toasts.resume(t.id)} />
		</div>
	{/each}
	{#if toasts.counts.waiting > 0 && !alertsPanel.open}
		<button type="button" class="more" data-alerts-open in:toastIn out:toastOut onclick={() => (alertsPanel.open = true)}>{t('frame.toasts.more', { n: toasts.counts.waiting })}</button>
	{/if}
</div>
{#if signedIn}<AlertsPanel before={feedPins.update ? [feedPins.update] : []} after={decisionPins()} />{/if}

<style>
	.alerts-demo { position: fixed; z-index: 89; inset-inline-end: 20px; bottom: calc(20px + env(safe-area-inset-bottom, 0px)); padding: 8px 14px; border-radius: 999px; font: var(--t-subhead); font-weight: 600; color: #f5f5f7; background: rgb(30 30 34 / 80%); -webkit-backdrop-filter: blur(20px); backdrop-filter: blur(20px); box-shadow: 0 6px 20px rgb(0 0 0 / 35%), inset 0 0 0 0.5px rgb(255 255 255 / 12%); }
	@media (max-width: 767px) { .alerts-demo { bottom: calc(var(--tabbar, 60px) + 12px + env(safe-area-inset-bottom, 0px)); } }
	.toasts {
		/* Undo the popover defaults: top right, just under the top bar. */
		inset: auto;
		margin: 0;
		padding: 0;
		border: 0;
		background: none;
		color: inherit;
		overflow: visible;
		position: fixed;
		z-index: 90;
		top: calc(var(--topbar, 56px) + 10px + env(safe-area-inset-top, 0px));
		inset-inline-end: max(20px, env(safe-area-inset-right, 0px));
		display: flex;
		flex-direction: column;
		gap: 10px;
		width: min(380px, calc(100vw - 32px));
		pointer-events: none;
	}
	/* Inside an open window the top right holds its close button, so the toasts sit at the bottom centre of the screen instead. */
	.toasts.in-win { top: auto; right: auto; left: 50%; transform: translateX(-50%); bottom: calc(16px + env(safe-area-inset-bottom, 0px)); }
	@media (max-width: 767px) {
		.toasts { left: 12px; right: 12px; width: auto; top: calc(var(--topbar, 56px) + 8px + env(safe-area-inset-top, 0px)); }
		/* A full-screen window on a phone keeps its answer buttons at the bottom; drop in under its top controls. */
		.toasts.in-win { left: 12px; right: 12px; transform: none; bottom: auto; top: calc(72px + env(safe-area-inset-top, 0px)); }
	}
	.slot { display: flex; flex-direction: column; }
	/* "+N more": how many wait behind the two on screen; opens Recent alerts. */
	.more { pointer-events: auto; align-self: center; padding: 6px 14px; border-radius: 999px; font: var(--t-footnote); font-weight: 600; color: #f5f5f7; background: rgb(28 28 32 / 86%); -webkit-backdrop-filter: blur(30px) saturate(180%); backdrop-filter: blur(30px) saturate(180%); box-shadow: 0 8px 24px rgb(0 0 0 / 32%), inset 0 0 0 0.5px rgb(255 255 255 / 12%); }
	.more:hover { background: rgb(44 44 50 / 86%); }
	.more:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.toasts.in-win .slot { align-items: center; }
</style>
