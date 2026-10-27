<script lang="ts">
	import Avatar from '#lib/ui/Avatar.svelte';
	import Button from '#lib/ui/Button.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Icon, { type IconName } from '#lib/Icon.svelte';
	import { session } from '#lib/session.svelte.ts';
	import { WORDS as ASKED } from '#lib/askedby.ts';
	import { tick } from 'svelte';
	import { PICTURE_HELP, TRAILER_HELP, TRAILER_OPTIONS, type AccountModel } from './account.svelte.ts';
	import TwoFactor from './parts/TwoFactor.svelte';
	import Reauth from './parts/Reauth.svelte';
	import Password from './parts/Password.svelte';
	import Linked from './parts/Linked.svelte';
	import Alerts from './parts/Alerts.svelte';
	import Follows from './parts/Follows.svelte';
	import PictureActions from './parts/PictureActions.svelte';
	import Devices from './parts/Devices.svelte';

	// 5 · Side tabs: a list of sections on the left and one panel at a time.
	// On phones the list is its own screen and a section opens with Back.
	let { m }: { m: AccountModel } = $props();

	type Sec = 'profile' | 'signin' | 'twofactor' | 'devices' | 'playback' | 'alerts';
	let sel = $state<Sec | null>(null);
	let narrow = $state(false);
	let panelHead: HTMLElement | undefined = $state();
	$effect(() => {
		const mq = matchMedia('(max-width: 767px)');
		const set = () => (narrow = mq.matches);
		set();
		mq.addEventListener('change', set);
		return () => mq.removeEventListener('change', set);
	});
	const shown = $derived<Sec | null>(sel ?? (narrow ? null : 'profile'));
	const SECS = $derived<{ id: Sec; label: string; icon: IconName; value: string }[]>([
		{ id: 'profile', label: 'Profile', icon: 'user', value: m.role },
		{ id: 'signin', label: 'Sign-in and security', icon: 'key', value: session.s.localRecent ? 'Confirmed' : '' },
		{ id: 'twofactor', label: 'Two-factor sign-in', icon: 'shield', value: session.s.twoFactor ? 'On' : 'Off' },
		{ id: 'devices', label: 'Devices', icon: 'tv', value: m.rows ? String(m.rows.length) : '' },
		{ id: 'playback', label: 'Playback', icon: 'play', value: m.trailerWord },
		{ id: 'alerts', label: 'Alerts and following', icon: 'bell', value: '' }
	]);
	const current = $derived(SECS.find((s) => s.id === shown));
	async function pick(id: Sec) {
		sel = id;
		if (narrow) {
			await tick();
			window.scrollTo({ top: 0 });
			panelHead?.focus();
		}
	}
	function key(e: KeyboardEvent, i: number) {
		const step = e.key === 'ArrowDown' ? 1 : e.key === 'ArrowUp' ? -1 : 0;
		if (!step || narrow) return;
		e.preventDefault();
		const next = SECS[(i + step + SECS.length) % SECS.length];
		sel = next.id;
		document.getElementById(`st-${next.id}`)?.focus();
	}
</script>

<div class="st" class:detail={narrow && shown}>
	{#if session.s.mustTwoFactor}
		<p class="notice warn" role="alert">The owner asks every admin to use two-factor sign-in. Turn it on under Two-factor sign-in to use the rest of HolyArrs.</p>
	{/if}

	<div class="split">
		{#if !narrow || !shown}
			<nav class="side" aria-label="Account sections">
				<div class="me">
					<Avatar name={m.name} src={m.preview || session.s.avatar} size={narrow ? 72 : 48} />
					<div class="me-t">
						<h1>{m.name}</h1>
						<p class="small muted">{m.role}</p>
					</div>
				</div>
				<ul role={narrow ? undefined : 'tablist'} aria-orientation={narrow ? undefined : 'vertical'}>
					{#each SECS as s, i (s.id)}
						<li role={narrow ? undefined : 'presentation'}>
							<button
								type="button"
								id="st-{s.id}"
								role={narrow ? undefined : 'tab'}
								aria-selected={narrow ? undefined : shown === s.id}
								aria-controls={narrow ? undefined : 'st-panel'}
								tabindex={narrow || shown === s.id ? 0 : -1}
								class:on={!narrow && shown === s.id}
								onclick={() => pick(s.id)}
								onkeydown={(e) => key(e, i)}
							>
								<Icon name={s.icon} size={18} />
								<span class="l">{s.label}</span>
								{#if s.id === 'twofactor' && !session.s.twoFactor}<span class="warn-dot" aria-hidden="true"></span>{/if}
								{#if narrow}
									<span class="v">{s.value}</span>
									<Icon name="chevronRight" size={16} />
								{/if}
							</button>
						</li>
					{/each}
				</ul>
			</nav>
		{/if}

		{#if shown}
			<section class="panel" id="st-panel" role={narrow ? undefined : 'tabpanel'} aria-labelledby="st-h">
				{#if narrow}
					<button type="button" class="back" onclick={() => (sel = null)}><Icon name="chevronLeft" size={18} /> Account</button>
				{/if}
				<h2 id="st-h" bind:this={panelHead} tabindex="-1">{current?.label}</h2>

				{#if shown === 'profile'}
					<div class="blk">
						<div class="pfp">
							<Avatar name={m.name} src={m.preview || session.s.avatar} size={88} label="Your profile picture" />
							<div class="pfp-r">
								<PictureActions {m} />
								<p class="xs muted">{PICTURE_HELP}</p>
							</div>
						</div>
					</div>
					<div class="blk">
						<h3>Signed in</h3>
						<dl class="kv">
							<dt>Name</dt>
							<dd>{m.name}</dd>
							<dt>Role</dt>
							<dd>{m.role}</dd>
							<dt>How</dt>
							<dd>{m.via}</dd>
							{#if m.method}<dt>Sign-in method</dt><dd>{m.method}</dd>{/if}
						</dl>
					</div>
				{:else if shown === 'signin'}
					<div class="blk"><h3>Confirm local sign-in</h3><Reauth {m} /></div>
					{#if m.hasLinkWay}<div class="blk"><h3>Linked sign-in</h3><Linked {m} /></div>{/if}
					<div class="blk"><h3>Password</h3><Password {m} /></div>
				{:else if shown === 'twofactor'}
					<div class="blk"><TwoFactor {m} /></div>
				{:else if shown === 'devices'}
					<div class="blk">
						<p class="small muted lead">Every browser or phone where you are signed in. Sign out one you do not know, or all but this one.</p>
						<Devices {m} look="tiles" />
					</div>
				{:else if shown === 'playback'}
					<div class="blk">
						<h3>Trailers on this device</h3>
						<p class="small muted lead">{TRAILER_HELP}</p>
						<SegmentedControl label="Play trailers" options={TRAILER_OPTIONS} value={m.pref} onchange={m.setPref} />
					</div>
					<div class="blk">
						<h3>Recommendations</h3>
						<p class="small muted lead">Explore's category strip learns from the categories you open. This stays on this server. Reset it to start fresh.</p>
						<Button variant="secondary" onclick={m.resetRecommendations} busy={m.resetBusy} data-testid="reset-recommendations">Reset my recommendations</Button>
					</div>
				{:else if shown === 'alerts'}
					<div class="blk">
						<h3>Alerts for you</h3>
						<p class="small muted lead">You always see news about your asks on your Wishlist. Add these to also hear by email or in Discord, when the owner has set those up.</p>
						<Alerts {m} />
					</div>
					<div class="blk"><h3>What you follow</h3><Follows {m} /></div>
					{#if session.can('ask')}
						<div class="blk">
							<h3>{ASKED.personalLabel}</h3>
							<p class="small muted lead">{ASKED.personalHelp}</p>
							<label class="box"><input type="checkbox" checked={m.showMyName} onchange={(e) => m.setShowMyName(e.currentTarget.checked)} data-testid="show-my-name" /> {ASKED.personalLabel}</label>
						</div>
					{/if}
				{/if}
			</section>
		{/if}
	</div>
</div>

<style>
	.st { width: min(1080px, 100%); margin: 0 auto; padding: var(--s-6) 0 var(--s-10); }
	.notice { margin-bottom: var(--s-4); }
	.split { display: grid; grid-template-columns: 260px minmax(0, 1fr); gap: var(--s-6); align-items: start; }
	.side { position: sticky; top: calc(var(--topbar) + 16px); display: flex; flex-direction: column; gap: var(--s-4); }
	.me { display: flex; align-items: center; gap: var(--s-3); padding: 0 8px; }
	.me-t { min-width: 0; }
	h1 { font: var(--t-headline); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.side ul { display: flex; flex-direction: column; gap: 2px; }
	.side button { width: 100%; display: flex; align-items: center; gap: 10px; min-height: 40px; padding: 0 10px; border-radius: var(--r-btn); color: var(--text-muted); font: var(--t-callout); font-weight: 500; text-align: left; transition: background var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	.side button:hover { background: var(--surface-2); color: var(--text); }
	.side button.on { background: var(--surface-3); color: var(--text); font-weight: 600; }
	.side button.on :global(.icon) { color: var(--accent-text); }
	.l { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.warn-dot { width: 8px; height: 8px; border-radius: 50%; background: var(--wait); flex: none; }
	.v { color: var(--text-muted); font: var(--t-callout); max-width: 40%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

	.panel { min-width: 0; padding: var(--s-6); border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); animation: fade var(--dur) var(--ease-out); }
	@keyframes fade { from { opacity: 0; transform: translateX(6px); } }
	.panel > h2 { font: var(--t-title2); letter-spacing: var(--t-title2-track); margin-bottom: var(--s-2); outline: none; }
	.blk { padding: var(--s-5) 0; border-bottom: 1px solid var(--line); display: flex; flex-direction: column; align-items: flex-start; }
	.blk:last-child { border-bottom: 0; padding-bottom: 0; }
	.blk > :global(*) { max-width: 100%; }
	.blk h3 { font: var(--t-headline); margin-bottom: var(--s-3); }
	.lead { margin-bottom: var(--s-3); }
	.pfp { display: flex; align-items: center; gap: var(--s-5); flex-wrap: wrap; }
	.pfp-r { display: flex; flex-direction: column; gap: var(--s-2); min-width: 0; flex: 1 1 240px; }
	.kv { display: grid; grid-template-columns: max-content 1fr; gap: var(--s-2) var(--s-5); font-size: var(--text-sm); }
	.kv dt { color: var(--text-muted); }
	.kv dd { overflow-wrap: anywhere; }

	@media (max-width: 767px) {
		.st { padding-top: var(--s-2); }
		.split { grid-template-columns: 1fr; gap: 0; }
		.side { position: static; gap: var(--s-5); }
		.me { flex-direction: column; text-align: center; gap: var(--s-2); padding: var(--s-2) 0; }
		h1 { font: var(--t-title2); letter-spacing: var(--t-title2-track); white-space: normal; }
		.side ul { gap: 0; border-radius: var(--r-card); overflow: hidden; background: color-mix(in srgb, var(--surface-2) 75%, var(--surface-1)); }
		.side li + li { border-top: 1px solid var(--line); }
		.side button { min-height: 52px; padding: 0 14px; border-radius: 0; color: var(--text); font: var(--t-body); }
		.side button :global(.icon:first-child) { color: var(--accent-text); }
		.side button > :global(.icon:last-child) { color: var(--text-faint); }
		.panel { padding: 0; background: none; box-shadow: none; animation-name: slide; }
		@keyframes slide { from { opacity: 0; transform: translateX(16px); } }
		.back { display: inline-flex; align-items: center; gap: 2px; min-height: 44px; margin-left: -6px; color: var(--accent-text); font: var(--t-body); font-weight: 500; }
		.panel > h2 { font: var(--t-title1); letter-spacing: var(--t-title1-track); }
		.blk { padding: var(--s-4); margin-top: var(--s-3); border: 0; border-radius: var(--r-card); background: color-mix(in srgb, var(--surface-2) 75%, var(--surface-1)); }
		.blk:last-child { padding-bottom: var(--s-4); }
		.pfp { flex-direction: column; text-align: center; width: 100%; }
		.pfp-r { align-items: center; flex-basis: auto; }
	}
	@media (prefers-reduced-motion: reduce) { .panel { animation: none; } .side button { transition: none; } }
</style>
