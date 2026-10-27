<script lang="ts">
	import Avatar from '#lib/ui/Avatar.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import { session } from '#lib/session.svelte.ts';
	import { WORDS as ASKED } from '#lib/askedby.ts';
	import { ago } from '#lib/format.ts';
	import { PICTURE_HELP, TRAILER_HELP, TRAILER_OPTIONS, type AccountModel } from './account.svelte.ts';
	import TwoFactor from './parts/TwoFactor.svelte';
	import Reauth from './parts/Reauth.svelte';
	import Password from './parts/Password.svelte';
	import Linked from './parts/Linked.svelte';
	import Alerts from './parts/Alerts.svelte';
	import Follows from './parts/Follows.svelte';
	import PictureActions from './parts/PictureActions.svelte';

	// 2 · Netflix Account: section names on the left, dense rows on the right,
	// each with a plain link. A link opens its edit right under the row.
	let { m }: { m: AccountModel } = $props();

	type Edit = '' | 'picture' | 'password' | 'twofactor' | 'reauth' | 'linked' | 'trailers' | 'alerts' | 'follows';
	let editing = $state<Edit>('');
	const toggle = (k: Edit) => (editing = editing === k ? '' : k);
	const done = () => (editing = '');
	const alertWord = $derived([m.targets.email, m.targets.discord_id ? 'Discord' : ''].filter(Boolean).join(' · ') || 'Only on your Wishlist');
</script>

{#snippet link(k: Edit, word: string, testid?: string)}
	<button type="button" class="lnk" aria-expanded={editing === k} onclick={() => toggle(k)} data-testid={testid}>{editing === k ? 'Close' : word}</button>
{/snippet}

<div class="nfx">
	<header class="top">
		<h1>Account</h1>
		<p class="sub">{m.name} · {m.role}</p>
	</header>

	{#if session.s.mustTwoFactor}
		<p class="notice warn" role="alert">The owner asks every admin to use two-factor sign-in. Turn it on under Security to use the rest of HolyArrs.</p>
	{/if}

	<section class="sec" aria-labelledby="n-profile">
		<h2 id="n-profile">Profile</h2>
		<div class="rows">
			<div class="rw">
				<div class="who">
					<Avatar name={m.name} src={m.preview || session.s.avatar} size={56} />
					<div>
						<p class="strong">{m.name}</p>
						<p class="muted small">{m.role}</p>
					</div>
				</div>
				{@render link('picture', session.s.avatar ? 'Change picture' : 'Add picture', 'link-picture')}
			</div>
			{#if editing === 'picture'}
				<div class="ed">
					<p class="small muted">{PICTURE_HELP}</p>
					<PictureActions {m} ondone={done} />
				</div>
			{/if}
			<div class="rw">
				<div class="kv">
					<p class="strong">Signed in</p>
					<p class="muted small">{[m.viaShort, m.method].filter(Boolean).join(' · ')}</p>
				</div>
			</div>
		</div>
	</section>

	<section class="sec" aria-labelledby="n-sec">
		<h2 id="n-sec">Security</h2>
		<div class="rows">
			<div class="rw">
				<div class="kv">
					<p class="strong">Password</p>
					<p class="muted small dots" aria-label="Hidden">••••••••</p>
				</div>
				{@render link('password', 'Change password', 'link-password')}
			</div>
			{#if editing === 'password'}
				<div class="ed">
					{#if !session.s.localRecent}
						<p class="small muted">First confirm your HolyArrs password.</p>
						<Reauth {m} intro={false} />
					{:else}
						<Password {m} ondone={done} />
					{/if}
				</div>
			{/if}
			<div class="rw">
				<div class="kv">
					<p class="strong">Two-factor sign-in</p>
					<p class="small" class:ok={session.s.twoFactor} class:muted={!session.s.twoFactor}>{session.s.twoFactor ? 'On' : 'Off'}</p>
				</div>
				{@render link('twofactor', session.s.twoFactor ? 'Change' : 'Turn on', 'link-twofactor')}
			</div>
			{#if editing === 'twofactor'}<div class="ed"><TwoFactor {m} /></div>{/if}
			<div class="rw">
				<div class="kv">
					<p class="strong">Local sign-in</p>
					<p class="muted small">{session.s.localRecent ? 'Confirmed recently' : 'Not confirmed recently'}</p>
				</div>
				{@render link('reauth', 'Confirm')}
			</div>
			{#if editing === 'reauth'}<div class="ed"><Reauth {m} ondone={done} /></div>{/if}
			{#if m.hasLinkWay}
				<div class="rw">
					<div class="kv">
						<p class="strong">Linked sign-in</p>
						<p class="muted small">Single sign-on or a trusted proxy</p>
					</div>
					{@render link('linked', 'Link')}
				</div>
				{#if editing === 'linked'}
					<div class="ed">
						{#if !session.s.localRecent}<Reauth {m} />{:else}<Linked {m} />{/if}
					</div>
				{/if}
			{/if}
		</div>
	</section>

	<section class="sec" aria-labelledby="n-dev">
		<h2 id="n-dev">Devices</h2>
		<div class="rows" data-testid="sessions">
			{#if m.rowsError}
				<p class="form-error" role="alert">{m.rowsError}</p>
			{:else if !m.rows}
				<div class="sk" style="height: 56px"></div>
			{:else}
				{#each m.shownRows ?? [] as s (s.id)}
					<div class="rw" data-testid="session" data-current={s.current}>
						<div class="kv">
							<p class="strong">{s.device}</p>
							<p class="muted small">{s.current ? 'This device' : `Last used ${ago(s.last_used_at)}`}</p>
						</div>
						{#if !s.current}
							<button type="button" class="lnk" disabled={m.rowsBusy} onclick={() => m.endSession(s)} aria-label="Sign out {s.device}">Sign out</button>
						{/if}
					</div>
				{/each}
				{#if m.hiddenDevices > 0}
					<div class="rw">
						<p class="muted small">{m.hiddenDevices} more signed-in devices</p>
						<button type="button" class="lnk" onclick={() => (m.allDevices = true)}>Show all</button>
					</div>
				{/if}
				{#if m.otherDevices > 0}
					<div class="rw">
						<p class="muted small">Sign out every device except this one.</p>
						<button type="button" class="lnk" disabled={m.rowsBusy} onclick={m.endOtherSessions}>Sign out all other devices</button>
					</div>
				{/if}
			{/if}
		</div>
	</section>

	<section class="sec" aria-labelledby="n-play">
		<h2 id="n-play">Playback on this device</h2>
		<div class="rows">
			<div class="rw">
				<div class="kv">
					<p class="strong">Trailers</p>
					<p class="muted small">{m.trailerWord}</p>
				</div>
				{@render link('trailers', 'Change', 'link-trailers')}
			</div>
			{#if editing === 'trailers'}
				<div class="ed">
					<p class="small muted">{TRAILER_HELP}</p>
					<SegmentedControl label="Play trailers" options={TRAILER_OPTIONS} value={m.pref} onchange={m.setPref} />
				</div>
			{/if}
			<div class="rw">
				<div class="kv">
					<p class="strong">Recommendations</p>
					<p class="muted small">Explore learns from the categories you open</p>
				</div>
				<button type="button" class="lnk" onclick={m.resetRecommendations} disabled={m.resetBusy} data-testid="reset-recommendations">Reset</button>
			</div>
		</div>
	</section>

	<section class="sec" aria-labelledby="n-alerts">
		<h2 id="n-alerts">Alerts and following</h2>
		<div class="rows">
			<div class="rw">
				<div class="kv">
					<p class="strong">Alerts for you</p>
					<p class="muted small">{alertWord}</p>
				</div>
				{@render link('alerts', 'Change')}
			</div>
			{#if editing === 'alerts'}<div class="ed"><Alerts {m} ondone={done} /></div>{/if}
			<div class="rw">
				<div class="kv">
					<p class="strong">What you follow</p>
					<p class="muted small">{m.follows.length === 1 ? '1 title' : `${m.follows.length} titles`}</p>
				</div>
				{@render link('follows', 'Manage')}
			</div>
			{#if editing === 'follows'}<div class="ed"><Follows {m} /></div>{/if}
			{#if session.can('ask')}
				<div class="rw">
					<label class="kv chk">
						<input type="checkbox" checked={m.showMyName} onchange={(e) => m.setShowMyName(e.currentTarget.checked)} data-testid="show-my-name" />
						<span>
							<span class="strong">{ASKED.personalLabel}</span>
							<span class="muted small block">{ASKED.personalHelp}</span>
						</span>
					</label>
				</div>
			{/if}
		</div>
	</section>
</div>

<style>
	.nfx { width: min(980px, 100%); margin: 0 auto; padding: var(--s-6) 0 var(--s-10); }
	.top { padding-bottom: var(--s-6); border-bottom: 2px solid var(--text); margin-bottom: var(--s-2); }
	h1 { font: var(--t-title1); font-size: 32px; letter-spacing: var(--t-title1-track); }
	.sub { margin-top: 4px; color: var(--text-muted); font: var(--t-callout); }
	.notice { margin: var(--s-4) 0; }
	.sec { display: grid; grid-template-columns: 240px minmax(0, 1fr); gap: var(--s-6); padding: var(--s-6) 0; border-bottom: 1px solid var(--line-strong); }
	h2 { font: var(--t-headline); color: var(--text-muted); text-transform: uppercase; font-size: 13px; letter-spacing: 0.06em; padding-top: 14px; }
	.rows { display: flex; flex-direction: column; min-width: 0; }
	.rw { display: flex; align-items: center; justify-content: space-between; gap: var(--s-4); min-height: 60px; padding: 10px 0; border-bottom: 1px solid var(--line); }
	.rows > .rw:last-child,
	.rows > .ed:last-child { border-bottom: 0; }
	.kv { min-width: 0; display: flex; flex-direction: column; gap: 2px; }
	.kv p, .strong { overflow-wrap: anywhere; }
	.strong { font-weight: 600; font-size: var(--text-base); }
	.ok { color: var(--ok-text); font-weight: 600; }
	.dots { letter-spacing: 0.1em; }
	.who { display: flex; align-items: center; gap: var(--s-4); min-width: 0; }
	.block { display: block; margin-top: 2px; }
	.chk { flex-direction: row; align-items: flex-start; gap: var(--s-3); cursor: pointer; }
	.chk input { margin-top: 4px; width: 18px; height: 18px; accent-color: var(--accent-fill); flex: none; }
	.lnk { flex: none; color: var(--info-text); font: var(--t-callout); font-weight: 600; padding: 10px 0 10px 8px; min-height: 44px; text-align: right; }
	.lnk:hover { text-decoration: underline; }
	.lnk:disabled { opacity: 0.5; }
	.ed { padding: var(--s-4); margin: 4px 0 var(--s-3); border-radius: var(--r-card); background: var(--surface-2); display: flex; flex-direction: column; gap: var(--s-3); align-items: flex-start; animation: drop var(--dur) var(--ease-out); }
	.ed > :global(*) { max-width: 100%; }
	@keyframes drop { from { opacity: 0; transform: translateY(-4px); } }
	@media (max-width: 860px) {
		.sec { grid-template-columns: 1fr; gap: 0; padding: var(--s-5) 0 var(--s-3); }
		h2 { padding-top: 0; }
	}
	@media (max-width: 767px) {
		.nfx { padding-top: var(--s-2); }
		h1 { font-size: 28px; }
		.top { padding-bottom: var(--s-4); }
		.ed { margin-inline: -4px; padding: var(--s-3); }
		.rw { gap: var(--s-3); }
		.lnk { max-width: 46%; }
	}
	@media (prefers-reduced-motion: reduce) { .ed { animation: none; } }
</style>
