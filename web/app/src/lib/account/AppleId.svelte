<script lang="ts">
	import Avatar from '#lib/ui/Avatar.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Icon, { type IconName } from '#lib/Icon.svelte';
	import { session } from '#lib/session.svelte.ts';
	import { WORDS as ASKED } from '#lib/askedby.ts';
	import { ago, dateTime } from '#lib/format.ts';
	import type { SessionRow } from '#lib/sessions.ts';
	import { PICTURE_HELP, TRAILER_HELP, TRAILER_OPTIONS, type AccountModel } from './account.svelte.ts';
	import { deviceIcon } from './device.ts';
	import TwoFactor from './parts/TwoFactor.svelte';
	import Reauth from './parts/Reauth.svelte';
	import Password from './parts/Password.svelte';
	import Linked from './parts/Linked.svelte';
	import Alerts from './parts/Alerts.svelte';
	import Follows from './parts/Follows.svelte';
	import PictureActions from './parts/PictureActions.svelte';

	// 1 · Apple ID: a big profile header, then calm grouped lists. Every row
	// opens a sheet for its edit, so the page itself stays a quiet overview.
	let { m }: { m: AccountModel } = $props();

	type Key = 'picture' | 'pwchange' | 'twofactor' | 'reauth' | 'linked' | 'trailers' | 'recs' | 'alerts' | 'follows' | 'device';
	let open = $state(false);
	let key = $state<Key>('picture');
	let device = $state<SessionRow | null>(null);
	const TITLES: Record<Key, string> = {
		picture: 'Profile picture',
		pwchange: 'Password',
		twofactor: 'Two-factor sign-in',
		reauth: 'Confirm local sign-in',
		linked: 'Linked sign-in',
		trailers: 'Trailers on this device',
		recs: 'Recommendations',
		alerts: 'Alerts for you',
		follows: 'What you follow',
		device: 'Device'
	};
	function show(k: Key, d: SessionRow | null = null) {
		key = k;
		device = d;
		open = true;
	}
	const close = () => (open = false);
	async function signOut(s: SessionRow) {
		await m.endSession(s);
		close();
	}
	const alertWord = $derived(m.targets.email ? m.targets.email : m.targets.discord_id ? 'Discord' : 'Wishlist only');
</script>

{#snippet row(k: Key, icon: IconName, tone: string, title: string, value: string, testid?: string)}
	<li>
		<button type="button" class="r" onclick={() => show(k)} data-testid={testid}>
			<span class="tile" style:--tone={tone}><Icon name={icon} size={17} /></span>
			<span class="t">{title}</span>
			<span class="v">{value}</span>
			<Icon name="chevronRight" size={16} />
		</button>
	</li>
{/snippet}

<div class="aid">
	<header class="hero">
		<button type="button" class="pic" onclick={() => show('picture')} aria-label="Edit profile picture">
			<Avatar name={m.name} src={m.preview || session.s.avatar} size={112} />
			<span class="edit">Edit</span>
		</button>
		<h1>{m.name}</h1>
		<p class="meta">
			<span class="badge">{m.role}</span>
			{#if m.method}<span>Signs in with {m.method}</span>{/if}
		</p>
	</header>

	{#if session.s.mustTwoFactor}
		<p class="notice warn" role="alert">The owner asks every admin to use two-factor sign-in. Turn it on below to use the rest of HolyArrs.</p>
	{/if}

	<section class="group" aria-labelledby="g-sec">
		<h2 id="g-sec">Sign-in and security</h2>
		<ul class="list">
			{@render row('pwchange', 'key', 'var(--text-faint)', 'Password', '', 'row-password')}
			{@render row('twofactor', 'shield', 'var(--ok)', 'Two-factor sign-in', session.s.twoFactor ? 'On' : 'Off', 'row-twofactor')}
			{@render row('reauth', 'check', 'var(--info)', 'Confirm local sign-in', session.s.localRecent ? 'Confirmed' : 'Not recently')}
			{#if m.hasLinkWay}{@render row('linked', 'people', 'var(--mark-dl)', 'Linked sign-in', '')}{/if}
			<li>
				<div class="r static">
					<span class="tile" style:--tone="var(--text-faint)"><Icon name="user" size={17} /></span>
					<span class="t">Signed in</span>
					<span class="v">{m.viaShort}</span>
				</div>
			</li>
		</ul>
		<p class="foot">You are signed in {m.via}.</p>
	</section>

	<section class="group" aria-labelledby="g-dev">
		<h2 id="g-dev">This device</h2>
		<ul class="list">
			{@render row('trailers', 'play', 'var(--accent)', 'Trailers', m.trailerWord, 'row-trailers')}
			{@render row('recs', 'sparkle', 'var(--mark-dl)', 'Recommendations', '')}
		</ul>
	</section>

	<section class="group" aria-labelledby="g-alerts">
		<h2 id="g-alerts">Alerts and following</h2>
		<ul class="list">
			{@render row('alerts', 'bell', 'var(--wait)', 'Alerts for you', alertWord)}
			{@render row('follows', 'bellOn', 'var(--info)', 'What you follow', String(m.follows.length))}
			{#if session.can('ask')}
				<li>
					<label class="r static switchrow">
						<span class="tile" style:--tone="var(--ok)"><Icon name="people" size={17} /></span>
						<span class="t">{ASKED.personalLabel}</span>
						<input
							type="checkbox"
							role="switch"
							class="switch"
							checked={m.showMyName}
							onchange={(e) => m.setShowMyName(e.currentTarget.checked)}
							data-testid="show-my-name"
						/>
					</label>
				</li>
			{/if}
		</ul>
		{#if session.can('ask')}<p class="foot">{ASKED.personalHelp}</p>{/if}
	</section>

	<section class="group" aria-labelledby="g-devs">
		<h2 id="g-devs">Devices{#if m.rows}<span class="count">{m.rows.length}</span>{/if}</h2>
		<ul class="list" data-testid="sessions">
			{#if m.rowsError}
				<li><p class="form-error pad" role="alert">{m.rowsError}</p></li>
			{:else if !m.rows}
				<li><div class="sk pad" style="height: 52px"></div></li>
			{:else}
				{#each m.shownRows ?? [] as s (s.id)}
					<li data-testid="session" data-current={s.current}>
						<button type="button" class="r" onclick={() => show('device', s)}>
							<span class="tile" style:--tone={s.current ? 'var(--ok)' : 'var(--text-faint)'}><Icon name={deviceIcon(s.device)} size={17} /></span>
							<span class="t two">
								<span>{s.device}</span>
								<span class="sub">{s.current ? 'This device' : `Last used ${ago(s.last_used_at)}`}</span>
							</span>
							<Icon name="chevronRight" size={16} />
						</button>
					</li>
				{/each}
				{#if m.hiddenDevices > 0}
					<li>
						<button type="button" class="r more" onclick={() => (m.allDevices = true)}><span class="t">Show all {m.rows.length} devices</span></button>
					</li>
				{/if}
				{#if m.otherDevices > 0}
					<li>
						<button type="button" class="r danger" onclick={m.endOtherSessions} disabled={m.rowsBusy} aria-busy={m.rowsBusy}>
							<span class="t">Sign out all other devices</span>
						</button>
					</li>
				{/if}
			{/if}
		</ul>
		<p class="foot">Every browser or phone where you are signed in. Sign out one you do not know.</p>
	</section>
</div>

<Sheet bind:open title={key === 'device' && device ? device.device : TITLES[key]} size="sm">
	{#if key === 'picture'}
		<div class="sheet-pic">
			<Avatar name={m.name} src={m.preview || session.s.avatar} size={128} label="Your profile picture" />
			<PictureActions {m} ondone={close} />
			<p class="small muted">{PICTURE_HELP}</p>
		</div>
	{:else if key === 'pwchange'}
		{#if !session.s.localRecent}
			<p class="small muted step">First confirm your HolyArrs password, then pick a new one.</p>
			<Reauth {m} intro={false} />
		{:else}
			<Password {m} ondone={close} />
		{/if}
	{:else if key === 'twofactor'}
		<TwoFactor {m} />
	{:else if key === 'reauth'}
		<Reauth {m} ondone={close} />
	{:else if key === 'linked'}
		{#if !session.s.localRecent}
			<Reauth {m} />
		{:else}
			<Linked {m} />
		{/if}
	{:else if key === 'trailers'}
		<p class="small muted step">{TRAILER_HELP}</p>
		<SegmentedControl label="Play trailers" options={TRAILER_OPTIONS} value={m.pref} onchange={m.setPref} />
	{:else if key === 'recs'}
		<p class="small muted step">Explore's category strip learns from the categories you open. This stays on this server. Reset it to start fresh.</p>
		<Button variant="secondary" onclick={m.resetRecommendations} busy={m.resetBusy} data-testid="reset-recommendations">Reset my recommendations</Button>
	{:else if key === 'alerts'}
		<p class="small muted step">You always see news about your asks on your Wishlist. Add these to also hear by email or in Discord, when the owner has set those up.</p>
		<Alerts {m} ondone={close} />
	{:else if key === 'follows'}
		<p class="small muted step">New episodes and seasons of shows you follow, and new albums of artists you follow, reach you here and where Alerts reach you.</p>
		<Follows {m} />
	{:else if key === 'device' && device}
		<dl class="facts-plain">
			<dt>Signed in</dt>
			<dd>{dateTime(device.created_at)}</dd>
			<dt>Last used</dt>
			<dd>{ago(device.last_used_at)}</dd>
			<dt>Signed in until</dt>
			<dd>{dateTime(device.expires_at)}</dd>
		</dl>
		{#if device.current}
			<p class="small muted">This is the device you are using now.</p>
		{:else}
			<Button variant="destructive" full onclick={() => device && signOut(device)} disabled={m.rowsBusy} busy={m.rowsBusy}>Sign out this device</Button>
		{/if}
	{/if}
</Sheet>

<style>
	.aid { width: min(680px, 100%); margin: 0 auto; padding: var(--s-6) 0 var(--s-10); display: flex; flex-direction: column; gap: var(--s-8); }
	.hero { display: flex; flex-direction: column; align-items: center; text-align: center; gap: var(--s-2); }
	.pic { position: relative; display: grid; justify-items: center; gap: 8px; border-radius: var(--r-pill); }
	.pic :global(.ui-avatar) { box-shadow: 0 0 0 4px var(--bg), 0 0 0 5px var(--line), var(--shadow-card); transition: transform var(--dur) var(--ease-spring); }
	.pic:hover :global(.ui-avatar) { transform: scale(1.03); }
	.edit { font: var(--t-footnote); font-weight: 600; color: var(--accent-text); }
	h1 { margin-top: var(--s-2); font: var(--t-large-title); letter-spacing: var(--t-large-title-track); overflow-wrap: anywhere; }
	.meta { display: flex; flex-wrap: wrap; justify-content: center; align-items: center; gap: 8px; color: var(--text-muted); font: var(--t-callout); }
	.badge { font: var(--t-caption); letter-spacing: 0.02em; text-transform: uppercase; padding: 3px 9px; border-radius: var(--r-pill); background: var(--accent-soft); color: var(--accent-text); }

	.group h2 { display: flex; align-items: center; gap: 8px; margin: 0 0 8px 16px; font: var(--t-footnote); font-weight: 600; text-transform: uppercase; letter-spacing: 0.04em; color: var(--text-muted); }
	.count { font: var(--t-caption); padding: 1px 7px; border-radius: var(--r-pill); background: var(--surface-3); color: var(--text-muted); }
	.list { border-radius: var(--r-card); background: color-mix(in srgb, var(--surface-2) 75%, var(--surface-1)); overflow: hidden; }
	.list li + li { position: relative; }
	.list li + li::before { content: ''; position: absolute; top: 0; left: 60px; right: 0; height: 1px; background: var(--line); }
	.r { width: 100%; min-height: 52px; display: flex; align-items: center; gap: 12px; padding: 8px 14px 8px 14px; text-align: left; color: var(--text-faint); transition: background var(--dur-fast) var(--ease-out); }
	button.r:hover { background: color-mix(in srgb, var(--text) 4%, transparent); }
	button.r:active { background: color-mix(in srgb, var(--text) 8%, transparent); }
	.tile { width: 32px; height: 32px; flex: none; display: grid; place-items: center; border-radius: 8px; background: var(--tone); color: #fff; }
	.t { flex: 1; min-width: 0; color: var(--text); font: var(--t-body); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.t.two { display: flex; flex-direction: column; white-space: normal; }
	.t.two > span:first-child { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.sub { font: var(--t-footnote); color: var(--text-muted); }
	.v { flex: 0 1 auto; min-width: 0; max-width: 45%; color: var(--text-muted); font: var(--t-callout); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.r.danger { justify-content: center; }
	.r.more .t { color: var(--accent-text); padding-left: 44px; }
	.r.danger .t { color: var(--fail-text); text-align: center; font-weight: 500; }
	.switchrow { cursor: pointer; }
	.switchrow .t { white-space: normal; }
	.switch { appearance: none; flex: none; width: 51px; height: 31px; border-radius: var(--r-pill); background: var(--surface-3); position: relative; cursor: pointer; transition: background var(--dur-fast) var(--ease-out); }
	.switch::after { content: ''; position: absolute; top: 2px; left: 2px; width: 27px; height: 27px; border-radius: 50%; background: #fff; box-shadow: 0 2px 4px rgb(0 0 0 / 20%); transition: transform var(--dur) var(--ease-spring); }
	.switch:checked { background: var(--ok); }
	.switch:checked::after { transform: translateX(20px); }
	.foot { margin: 8px 16px 0; font: var(--t-footnote); color: var(--text-muted); }
	.pad { margin: 12px 14px; }

	.sheet-pic { display: flex; flex-direction: column; align-items: center; gap: var(--s-4); text-align: center; }
	.step { margin-bottom: var(--s-4); }
	.facts-plain { display: grid; grid-template-columns: max-content 1fr; gap: var(--s-2) var(--s-5); margin-bottom: var(--s-5); font-size: var(--text-sm); }
	.facts-plain dt { color: var(--text-muted); }
	@media (max-width: 767px) {
		.aid { padding-top: var(--s-2); gap: var(--s-6); }
		.pic :global(.ui-avatar) { --av: 96px !important; }
		h1 { font: var(--t-title1); letter-spacing: var(--t-title1-track); }
		.group h2 { margin-left: 4px; }
		.foot { margin-inline: 4px; }
	}
	@media (prefers-reduced-motion: reduce) {
		.switch, .switch::after, .pic :global(.ui-avatar) { transition: none; }
	}
</style>
