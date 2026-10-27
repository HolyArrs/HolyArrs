<script lang="ts">
	import Avatar from '#lib/ui/Avatar.svelte';
	import Button from '#lib/ui/Button.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Icon from '#lib/Icon.svelte';
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

	// 4 · Security first: one status card with the checks that matter and a
	// big button for each fix; the rest of the account in compact cards below.
	let { m }: { m: AccountModel } = $props();

	type Act = '' | 'twofactor' | 'reauth' | 'pwchange' | 'linked' | 'alerts';
	let act = $state<Act>('');
	let actEl: HTMLElement | undefined = $state();
	let devicesEl: HTMLElement | undefined = $state();
	const ACT_TITLE: Record<Exclude<Act, ''>, string> = {
		twofactor: 'Two-factor sign-in',
		reauth: 'Confirm local sign-in',
		pwchange: 'Change password',
		linked: 'Linked sign-in',
		alerts: 'Alerts for you'
	};
	async function open(a: Act) {
		act = a;
		await tick();
		actEl?.scrollIntoView({ behavior: 'smooth', block: 'start' });
		actEl?.focus({ preventScroll: true });
	}
	const toDevices = () => devicesEl?.scrollIntoView({ behavior: 'smooth', block: 'start' });

	const localWay = $derived(session.s.authMethod.startsWith('local_'));
	const checks = $derived([
		{ ok: session.s.twoFactor, title: 'Two-factor sign-in', okWord: 'On. A code from your phone is asked after your password.', badWord: 'Off. Anyone with your password can sign in.' },
		{
			ok: session.s.via !== 'local_network',
			title: 'Sign-in',
			okWord: localWay ? 'You signed in with your HolyArrs password.' : `You signed in with ${m.method || 'a linked account'}.`,
			badWord: 'You are on your local network without a password.'
		},
		{
			ok: m.rows !== null && m.otherDevices === 0,
			title: 'Signed-in devices',
			okWord: 'Only this device is signed in.',
			badWord: m.rows ? `${m.rows.length} devices are signed in. Check you know each one.` : 'Loading devices…'
		}
	]);
	const passed = $derived(checks.filter((c) => c.ok).length);
	const R = 34;
	const C = 2 * Math.PI * R;
</script>

<div class="sf">
	<header class="head">
		<h1>My account</h1>
		<p class="muted">Sign-in safety first, then your profile and this device.</p>
	</header>

	{#if session.s.mustTwoFactor}
		<p class="notice warn" role="alert">The owner asks every admin to use two-factor sign-in. Turn it on below to use the rest of HolyArrs.</p>
	{/if}

	<section class="status" class:good={passed === checks.length} aria-labelledby="sf-status">
		<div class="score">
			<svg viewBox="0 0 80 80" width="88" height="88" aria-hidden="true">
				<circle cx="40" cy="40" r={R} class="track" />
				<circle cx="40" cy="40" r={R} class="fill" stroke-dasharray={C} stroke-dashoffset={C * (1 - passed / checks.length)} />
			</svg>
			<span class="num"><b>{passed}</b>/{checks.length}</span>
		</div>
		<div class="status-text">
			<h2 id="sf-status">{passed === checks.length ? 'Your account is well protected' : session.s.twoFactor ? 'Almost there' : 'Make your account safer'}</h2>
			<p class="muted small">{passed} of {checks.length} checks pass for {m.name}.</p>
		</div>
		<ul class="checks">
			<li class:ok={checks[0].ok}>
				<span class="mark"><Icon name={checks[0].ok ? 'check' : 'alert'} size={16} /></span>
				<span class="ct"><b>{checks[0].title}</b><span>{checks[0].ok ? checks[0].okWord : checks[0].badWord}</span></span>
				{#if checks[0].ok}
					<Button size="md" variant="secondary" onclick={() => open('twofactor')}>Manage</Button>
				{:else}
					<Button size="lg" variant="primary" icon="shield" onclick={() => open('twofactor')} data-testid="turn-on-twofactor">Turn on</Button>
				{/if}
			</li>
			<li class:ok={checks[1].ok}>
				<span class="mark"><Icon name={checks[1].ok ? 'check' : 'alert'} size={16} /></span>
				<span class="ct"><b>{checks[1].title}</b><span>{checks[1].ok ? checks[1].okWord : checks[1].badWord}</span></span>
				<Button size="md" variant="secondary" onclick={() => open(session.s.localRecent ? 'pwchange' : 'reauth')}>{session.s.localRecent ? 'Change password' : 'Confirm'}</Button>
			</li>
			<li class:ok={checks[2].ok}>
				<span class="mark"><Icon name={checks[2].ok ? 'check' : 'alert'} size={16} /></span>
				<span class="ct"><b>{checks[2].title}</b><span>{checks[2].ok ? checks[2].okWord : checks[2].badWord}</span></span>
				<Button size="md" variant="secondary" onclick={toDevices}>Review</Button>
			</li>
		</ul>
	</section>

	{#if act}
		<section class="act" bind:this={actEl} tabindex="-1" aria-labelledby="sf-act">
			<header>
				<h2 id="sf-act">{ACT_TITLE[act]}</h2>
				<Button variant="plain" size="sm" onclick={() => (act = '')}>Close</Button>
			</header>
			{#if act === 'twofactor'}
				<TwoFactor {m} />
			{:else if act === 'reauth'}
				<Reauth {m} ondone={() => (act = '')} />
			{:else if act === 'pwchange'}
				{#if !session.s.localRecent}<Reauth {m} intro={false} />{:else}<Password {m} ondone={() => (act = '')} />{/if}
			{:else if act === 'linked'}
				{#if !session.s.localRecent}<Reauth {m} />{:else}<Linked {m} />{/if}
			{:else if act === 'alerts'}
				<Alerts {m} ondone={() => (act = '')} />
			{/if}
		</section>
	{/if}

	<div class="cards">
		<section class="card profile" aria-labelledby="sf-p">
			<div class="pf">
				<Avatar name={m.name} src={m.preview || session.s.avatar} size={64} label="Your profile picture" />
				<div class="pf-t">
					<h3 id="sf-p">{m.name}</h3>
					<p class="small muted">{[m.role, m.method].filter(Boolean).join(' · ')}</p>
				</div>
			</div>
			<PictureActions {m} size="sm" />
			<p class="xs muted">{PICTURE_HELP}</p>
		</section>

		<section class="card" aria-labelledby="sf-si">
			<h3 id="sf-si"><Icon name="key" size={16} /> Sign-in</h3>
			<dl class="kv">
				<dt>How</dt>
				<dd>{m.viaShort}</dd>
				<dt>Local sign-in</dt>
				<dd>{session.s.localRecent ? 'Confirmed recently' : 'Not confirmed recently'}</dd>
			</dl>
			<div class="acts">
				<Button size="sm" onclick={() => open('reauth')}>Confirm local sign-in</Button>
				<Button size="sm" variant="plain" onclick={() => open('pwchange')}>Change password</Button>
				{#if m.hasLinkWay}<Button size="sm" variant="plain" onclick={() => open('linked')}>Link sign-in</Button>{/if}
			</div>
		</section>

		<section class="card" aria-labelledby="sf-tr">
			<h3 id="sf-tr"><Icon name="play" size={16} /> Trailers on this device</h3>
			<p class="xs muted">{TRAILER_HELP}</p>
			<SegmentedControl label="Play trailers" size="sm" options={TRAILER_OPTIONS} value={m.pref} onchange={m.setPref} />
		</section>

		<section class="card wide" bind:this={devicesEl} aria-labelledby="sf-d">
			<h3 id="sf-d"><Icon name="tv" size={16} /> Signed-in devices{#if m.rows}<span class="n">{m.rows.length}</span>{/if}</h3>
			<Devices {m} look="tiles" />
		</section>

		<section class="card" aria-labelledby="sf-al">
			<h3 id="sf-al"><Icon name="bell" size={16} /> Alerts for you</h3>
			<p class="small">{m.targets.email || 'No email'} · {m.targets.discord_id ? 'Discord on' : 'No Discord'}</p>
			<p class="xs muted">News about your asks is always on your Wishlist.</p>
			<div class="acts"><Button size="sm" onclick={() => open('alerts')}>Edit Alerts</Button></div>
		</section>

		<section class="card" aria-labelledby="sf-fo">
			<h3 id="sf-fo"><Icon name="bellOn" size={16} /> What you follow</h3>
			<Follows {m} />
		</section>

		<section class="card" aria-labelledby="sf-more">
			<h3 id="sf-more"><Icon name="sparkle" size={16} /> {session.can('ask') ? 'Recommendations and privacy' : 'Recommendations'}</h3>
			<p class="xs muted">Explore's category strip learns from the categories you open. This stays on this server.</p>
			<div class="acts"><Button size="sm" variant="plain" onclick={m.resetRecommendations} busy={m.resetBusy} data-testid="reset-recommendations">Reset my recommendations</Button></div>
			{#if session.can('ask')}
				<label class="box small"><input type="checkbox" checked={m.showMyName} onchange={(e) => m.setShowMyName(e.currentTarget.checked)} data-testid="show-my-name" /> {ASKED.personalLabel}</label>
				<p class="xs muted">{ASKED.personalHelp}</p>
			{/if}
		</section>
	</div>
</div>

<style>
	.sf { width: min(1080px, 100%); margin: 0 auto; padding: var(--s-6) 0 var(--s-10); display: flex; flex-direction: column; gap: var(--s-5); }
	h1 { font: var(--t-large-title); letter-spacing: var(--t-large-title-track); }
	.head p { margin-top: 4px; font: var(--t-callout); }

	.status { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: var(--s-4) var(--s-5); align-items: center; padding: var(--s-6); border-radius: var(--r-panel); background: linear-gradient(135deg, color-mix(in srgb, var(--wait) 12%, var(--surface-1)), var(--surface-1) 60%); box-shadow: 0 0 0 1px color-mix(in srgb, var(--wait) 30%, var(--line)); --tone: var(--wait); }
	.status.good { background: linear-gradient(135deg, color-mix(in srgb, var(--ok) 12%, var(--surface-1)), var(--surface-1) 60%); box-shadow: 0 0 0 1px color-mix(in srgb, var(--ok) 30%, var(--line)); --tone: var(--ok); }
	.score { position: relative; width: 88px; height: 88px; }
	.score svg { transform: rotate(-90deg); }
	.track { fill: none; stroke: var(--surface-3); stroke-width: 8; }
	.fill { fill: none; stroke: var(--tone); stroke-width: 8; stroke-linecap: round; transition: stroke-dashoffset var(--dur-slow) var(--ease-out); }
	.num { position: absolute; inset: 0; display: flex; flex-wrap: wrap; align-content: center; align-items: baseline; justify-content: center; gap: 1px; font: var(--t-callout); color: var(--text-muted); }
	.num b { font: var(--t-title2); color: var(--text); }
	.status-text h2 { font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	.status-text p { margin-top: 4px; }
	.checks { grid-column: 1 / -1; display: flex; flex-direction: column; border-top: 1px solid var(--line); }
	.checks li { display: flex; align-items: center; gap: var(--s-4); padding: var(--s-4) 0; border-bottom: 1px solid var(--line); }
	.checks li:last-child { border-bottom: 0; padding-bottom: 0; }
	.mark { width: 32px; height: 32px; flex: none; display: grid; place-items: center; border-radius: 50%; background: color-mix(in srgb, var(--wait) 16%, transparent); color: var(--wait-text); }
	.checks li.ok .mark { background: color-mix(in srgb, var(--ok) 16%, transparent); color: var(--ok-text); }
	.ct { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; font: var(--t-callout); }
	.ct span { color: var(--text-muted); }
	.checks :global(.ui-btn) { flex: none; }

	.act { padding: var(--s-5) var(--s-6) var(--s-6); border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 2px var(--accent), var(--shadow-card); scroll-margin-top: calc(var(--topbar) + 16px); outline: none; animation: rise var(--dur) var(--ease-out); }
	.act header { display: flex; align-items: center; justify-content: space-between; gap: var(--s-3); margin-bottom: var(--s-3); }
	.act h2 { font: var(--t-title3); letter-spacing: var(--t-title3-track); }
	@keyframes rise { from { opacity: 0; transform: translateY(var(--rise)); } }

	.cards { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--s-4); align-items: start; }
	.card { min-width: 0; display: flex; flex-direction: column; gap: var(--s-3); padding: var(--s-5); border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); scroll-margin-top: calc(var(--topbar) + 16px); }
	.card.wide { grid-column: span 2; }
	.card h3 { display: flex; align-items: center; gap: 8px; font: var(--t-headline); }
	.card h3 :global(.icon) { color: var(--text-muted); }
	.n { font: var(--t-caption); padding: 1px 7px; border-radius: var(--r-pill); background: var(--surface-3); color: var(--text-muted); }
	.pf { display: flex; align-items: center; gap: var(--s-4); }
	.pf-t { min-width: 0; }
	.pf-t h3 { font: var(--t-title3); overflow-wrap: anywhere; }
	.kv { display: grid; grid-template-columns: max-content 1fr; gap: 4px var(--s-4); font-size: var(--text-sm); }
	.kv dt { color: var(--text-muted); }
	.kv dd { overflow-wrap: anywhere; }
	.acts { display: flex; flex-wrap: wrap; gap: 6px; }

	@media (max-width: 1000px) {
		.cards { grid-template-columns: repeat(2, minmax(0, 1fr)); }
	}
	@media (max-width: 767px) {
		.sf { padding-top: var(--s-2); gap: var(--s-4); }
		h1 { font: var(--t-title1); letter-spacing: var(--t-title1-track); }
		.status { padding: var(--s-5) var(--s-4) var(--s-4); gap: var(--s-3) var(--s-4); }
		.score, .score svg { width: 68px; height: 68px; }
		.status-text h2 { font: var(--t-title3); }
		.checks li { flex-wrap: wrap; gap: var(--s-3); }
		.ct { flex: 1 1 calc(100% - 48px); }
		.checks :global(.ui-btn) { flex: 1 1 100%; min-height: 48px; }
		.act { padding: var(--s-4); }
		.cards { grid-template-columns: 1fr; }
		.card.wide { grid-column: auto; }
		.card { padding: var(--s-4); }
	}
	@media (prefers-reduced-motion: reduce) { .act { animation: none; } .fill { transition: none; } }
</style>
