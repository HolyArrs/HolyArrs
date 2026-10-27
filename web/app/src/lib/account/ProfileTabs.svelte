<script lang="ts">
	import Avatar from '#lib/ui/Avatar.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Poster from '#lib/Poster.svelte';
	import { session } from '#lib/session.svelte.ts';
	import { WORDS as ASKED } from '#lib/askedby.ts';
	import { to } from '#lib/paths.ts';
	import { PICTURE_HELP, TRAILER_HELP, TRAILER_OPTIONS, type AccountModel } from './account.svelte.ts';
	import TwoFactor from './parts/TwoFactor.svelte';
	import Reauth from './parts/Reauth.svelte';
	import Password from './parts/Password.svelte';
	import Linked from './parts/Linked.svelte';
	import Alerts from './parts/Alerts.svelte';
	import Follows from './parts/Follows.svelte';
	import PictureActions from './parts/PictureActions.svelte';
	import Devices from './parts/Devices.svelte';

	// 3 · Profile: a banner made from your own recent asks, your numbers, then
	// tabs. Overview is about what you asked for; the rest is the account.
	let { m }: { m: AccountModel } = $props();

	type Tab = 'overview' | 'settings' | 'security' | 'devices';
	let tab = $state<Tab>('overview');
	const TABS: { id: Tab; label: string }[] = [
		{ id: 'overview', label: 'Overview' },
		{ id: 'settings', label: 'Settings' },
		{ id: 'security', label: 'Security' },
		{ id: 'devices', label: 'Devices' }
	];
	const usable = (p?: string) => !!p && (p.startsWith('https://') || p.startsWith('/'));
	const art = $derived(m.myAsks.filter((a) => usable(a.poster)).slice(0, 8));
	const stats = $derived([
		{ n: m.myAsks.length, label: 'Asks made' },
		{ n: m.myAsks.filter((a) => a.state === 'approved' || a.state === 'available').length, label: 'Approved' },
		{ n: m.myAsks.filter((a) => a.state === 'available').length, label: 'In the Library' }
	]);
	function key(e: KeyboardEvent, i: number) {
		const step = e.key === 'ArrowRight' ? 1 : e.key === 'ArrowLeft' ? -1 : 0;
		if (!step) return;
		e.preventDefault();
		const next = TABS[(i + step + TABS.length) % TABS.length];
		tab = next.id;
		document.getElementById(`pt-${next.id}`)?.focus();
	}
</script>

<div class="pt">
	<header class="banner">
		<div class="mosaic" aria-hidden="true">
			{#each art as a (a.id)}<img src={to(a.poster ?? '')} alt="" referrerpolicy="no-referrer" />{/each}
		</div>
		<div class="scrim" aria-hidden="true"></div>
		<div class="id">
			<button type="button" class="pic" onclick={() => (tab = 'settings')} aria-label="Change profile picture in Settings">
				<Avatar name={m.name} src={m.preview || session.s.avatar} size={112} />
			</button>
			<div class="names">
				<h1>{m.name}</h1>
				<p class="meta"><span class="badge">{m.role}</span>{#if m.method}<span>Signs in with {m.method}</span>{/if}</p>
			</div>
			<dl class="stats">
				{#each stats as s (s.label)}
					<div>
						<dt>{s.label}</dt>
						<dd>{m.asks === null ? '–' : s.n}</dd>
					</div>
				{/each}
			</dl>
		</div>
	</header>

	<div class="tabs" role="tablist" aria-label="Account">
		{#each TABS as t, i (t.id)}
			<button
				type="button"
				role="tab"
				id="pt-{t.id}"
				aria-selected={tab === t.id}
				aria-controls="pt-panel"
				tabindex={tab === t.id ? 0 : -1}
				class:on={tab === t.id}
				onclick={() => (tab = t.id)}
				onkeydown={(e) => key(e, i)}
			>
				{t.label}
				{#if t.id === 'security' && !session.s.twoFactor}<span class="dot" aria-label="Two-factor sign-in is off"></span>{/if}
				{#if t.id === 'devices' && m.rows}<span class="n">{m.rows.length}</span>{/if}
			</button>
		{/each}
	</div>

	{#if session.s.mustTwoFactor}
		<p class="notice warn" role="alert">The owner asks every admin to use two-factor sign-in. Turn it on under Security to use the rest of HolyArrs.</p>
	{/if}

	<div class="panel" id="pt-panel" role="tabpanel" aria-labelledby="pt-{tab}">
		{#if tab === 'overview'}
			<div class="ov-head">
				<h2>Your recent asks</h2>
				<a class="more" href={to('/wishlist')}>Open your Wishlist</a>
			</div>
			{#if m.asks === null}
				<div class="grid">
					{#each Array(6) as _, i (i)}<div class="sk" style="aspect-ratio: 2 / 3; border-radius: var(--r-card)"></div>{/each}
				</div>
			{:else if m.myAsks.length === 0}
				<div class="empty">
					<p class="strong">Nothing asked for yet</p>
					<p class="small muted">Movies, shows and music you ask for show up here with how far along they are.</p>
				</div>
			{:else}
				<ul class="grid">
					{#each m.myAsks.slice(0, 12) as a (a.id)}
						<li>
							<svelte:element this={a.title_id ? 'a' : 'div'} class="ask" href={a.title_id ? to(`/library/${a.title_id}`) : undefined}>
								<Poster src={a.poster} name={a.name} />
								<span class="an">{a.name}</span>
								<span class="as st-{a.state}">{a.state_label}</span>
							</svelte:element>
						</li>
					{/each}
				</ul>
			{/if}
		{:else if tab === 'settings'}
			<div class="cards">
				<SettingsCard title="Profile picture" description={PICTURE_HELP}>
					<div class="pfp">
						<Avatar name={m.name} src={m.preview || session.s.avatar} size={72} label="Your profile picture" />
						<PictureActions {m} />
					</div>
				</SettingsCard>
				<SettingsCard title="Trailers on this device" description={TRAILER_HELP}>
					<SegmentedControl label="Play trailers" options={TRAILER_OPTIONS} value={m.pref} onchange={m.setPref} />
				</SettingsCard>
				<SettingsCard title="Alerts for you" description="You always see news about your asks on your Wishlist. Add these to also hear by email or in Discord, when the owner has set those up.">
					<Alerts {m} />
				</SettingsCard>
				{#if session.can('ask')}
					<SettingsCard title={ASKED.personalLabel} description={ASKED.personalHelp}>
						<label class="box"><input type="checkbox" checked={m.showMyName} onchange={(e) => m.setShowMyName(e.currentTarget.checked)} data-testid="show-my-name" /> {ASKED.personalLabel}</label>
					</SettingsCard>
				{/if}
				<SettingsCard title="What you follow" description="New episodes and seasons of shows you follow, and new albums of artists you follow, reach you here and where Alerts reach you.">
					<Follows {m} />
				</SettingsCard>
				<SettingsCard title="Recommendations" description="Explore's category strip learns from the categories you open. This stays on this server. Reset it to start fresh.">
					<div><Button variant="plain" onclick={m.resetRecommendations} busy={m.resetBusy} data-testid="reset-recommendations">Reset my recommendations</Button></div>
				</SettingsCard>
			</div>
		{:else if tab === 'security'}
			<div class="cards">
				<SettingsCard title="Signed in">
					<dl class="facts-plain">
						<dt>Name</dt>
						<dd>{m.name}</dd>
						<dt>Role</dt>
						<dd>{m.role}</dd>
						<dt>How</dt>
						<dd>{m.via}</dd>
						{#if m.method}<dt>Sign-in method</dt><dd>{m.method}</dd>{/if}
					</dl>
				</SettingsCard>
				<SettingsCard title="Two-factor sign-in"><TwoFactor {m} /></SettingsCard>
				<SettingsCard title="Confirm local sign-in"><Reauth {m} /></SettingsCard>
				{#if m.hasLinkWay}<SettingsCard title="Linked sign-in"><Linked {m} /></SettingsCard>{/if}
				<SettingsCard title="Password"><Password {m} /></SettingsCard>
			</div>
		{:else}
			<SettingsCard title="Signed-in devices" description="Every browser or phone where you are signed in. Sign out one you do not know, or all but this one.">
				<Devices {m} look="tiles" />
			</SettingsCard>
		{/if}
	</div>
</div>

<style>
	.pt { width: min(1120px, 100%); margin: 0 auto; padding-bottom: var(--s-10); }
	.banner { position: relative; overflow: hidden; border-radius: var(--r-panel); min-height: 260px; display: flex; align-items: flex-end; background: linear-gradient(135deg, color-mix(in srgb, var(--accent) 55%, #000), #111); isolation: isolate; }
	.mosaic { position: absolute; inset: -40px; display: flex; z-index: -2; filter: blur(26px) saturate(1.4); transform: scale(1.1); }
	.mosaic img { flex: 1; min-width: 0; height: 100%; object-fit: cover; }
	.scrim { position: absolute; inset: 0; z-index: -1; background: linear-gradient(to bottom, var(--on-art-scrim-none), var(--on-art-scrim-mid) 45%, var(--on-art-scrim)); }
	.id { width: 100%; display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: end; gap: var(--s-5); padding: var(--s-8) var(--s-8) var(--s-6); color: var(--on-art); }
	.pic { border-radius: 50%; }
	.pic :global(.ui-avatar) { box-shadow: 0 0 0 3px var(--on-art-ring), 0 8px 24px rgb(0 0 0 / 40%); }
	.names { min-width: 0; padding-bottom: 6px; }
	h1 { font: var(--t-large-title); letter-spacing: var(--t-large-title-track); overflow-wrap: anywhere; }
	.meta { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-top: 6px; color: var(--on-art-muted); font: var(--t-callout); }
	.badge { font: var(--t-caption); letter-spacing: 0.04em; text-transform: uppercase; padding: 3px 9px; border-radius: var(--r-pill); background: var(--on-art-fill); color: var(--on-art); -webkit-backdrop-filter: blur(10px); backdrop-filter: blur(10px); }
	.stats { display: flex; gap: 6px; padding-bottom: 6px; }
	.stats > div { display: flex; flex-direction: column-reverse; align-items: center; min-width: 92px; padding: 10px 12px; border-radius: var(--r-card); background: var(--on-art-fill); -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px); }
	.stats dt { font: var(--t-caption); color: var(--on-art-muted); text-transform: uppercase; letter-spacing: 0.04em; white-space: nowrap; }
	.stats dd { font: var(--t-title2); letter-spacing: var(--t-title2-track); }

	.tabs { display: flex; gap: 4px; margin: var(--s-4) 0 var(--s-5); border-bottom: 1px solid var(--line); overflow-x: auto; scrollbar-width: none; }
	.tabs::-webkit-scrollbar { display: none; }
	.tabs button { flex: none; display: inline-flex; align-items: center; gap: 6px; min-height: 46px; padding: 0 14px; color: var(--text-muted); font: var(--t-headline); font-size: 15px; border-bottom: 2px solid transparent; margin-bottom: -1px; transition: color var(--dur-fast) var(--ease-out), border-color var(--dur-fast) var(--ease-out); }
	.tabs button:hover { color: var(--text); }
	.tabs button.on { color: var(--text); border-bottom-color: var(--accent); }
	.dot { width: 7px; height: 7px; border-radius: 50%; background: var(--wait); }
	.n { font: var(--t-caption); padding: 1px 7px; border-radius: var(--r-pill); background: var(--surface-3); color: var(--text-muted); }
	.notice { margin-bottom: var(--s-4); }

	.ov-head { display: flex; align-items: baseline; justify-content: space-between; gap: var(--s-3); margin-bottom: var(--s-4); flex-wrap: wrap; }
	.ov-head h2 { font: var(--t-title3); letter-spacing: var(--t-title3-track); }
	.more { color: var(--accent-text); font: var(--t-callout); font-weight: 600; }
	.more:hover { text-decoration: underline; }
	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: var(--s-5) var(--s-4); }
	.ask { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
	.ask :global(.poster) { border-radius: var(--r-card); overflow: hidden; transition: transform var(--dur) var(--ease-spring), box-shadow var(--dur) var(--ease-out); }
	a.ask:hover :global(.poster) { transform: translateY(var(--lift)); box-shadow: var(--shadow-lift); }
	.an { font: var(--t-subhead); font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.as { font: var(--t-footnote); color: var(--text-muted); }
	.as.st-available { color: var(--ok-text); }
	.as.st-approved { color: var(--info-text); }
	.as.st-turned_down { color: var(--text-faint); }
	.empty { padding: var(--s-8) var(--s-5); text-align: center; border: 1px dashed var(--line-strong); border-radius: var(--r-panel); background: var(--surface-2); }
	.strong { font-weight: 600; }

	.cards { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--s-4); align-items: start; }
	.pfp { display: flex; align-items: center; gap: var(--s-4); flex-wrap: wrap; }
	.facts-plain { display: grid; grid-template-columns: max-content 1fr; gap: var(--s-2) var(--s-5); font-size: var(--text-sm); }
	.facts-plain dt { color: var(--text-muted); }
	.facts-plain dd { overflow-wrap: anywhere; }

	@media (max-width: 1000px) {
		.id { grid-template-columns: auto minmax(0, 1fr); }
		.stats { grid-column: 1 / -1; }
		.stats > div { flex: 1; }
	}
	@media (max-width: 900px) {
		.cards { grid-template-columns: 1fr; }
	}
	@media (max-width: 767px) {
		.pt { padding-top: 0; }
		.banner { margin: 0 calc(-1 * var(--gutter)); border-radius: 0; min-height: 0; }
		.id { grid-template-columns: 1fr; justify-items: center; text-align: center; gap: var(--s-3); padding: var(--s-10) var(--s-4) var(--s-4); }
		.pic :global(.ui-avatar) { --av: 88px !important; }
		.names { padding-bottom: 0; }
		h1 { font: var(--t-title1); letter-spacing: var(--t-title1-track); }
		.meta { justify-content: center; }
		.stats { width: 100%; }
		.stats > div { min-width: 0; padding: 8px 4px; }
		.stats dt { font-size: 10px; letter-spacing: 0.02em; }
		.tabs { position: sticky; top: var(--topbar); z-index: 5; background: var(--bg); margin: 0 calc(-1 * var(--gutter)) var(--s-4); padding: 0 calc(var(--gutter) - 6px); }
		.tabs button { padding: 0 10px; }
		.grid { grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--s-4) var(--s-3); }
	}
	@media (prefers-reduced-motion: reduce) {
		.ask :global(.poster), .tabs button { transition: none; }
	}
</style>
