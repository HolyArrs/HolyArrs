<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import SigninFace from './SigninFace.svelte';
	import { pixelTrail } from './pixel-trail.ts';
	import type { RecentPerson, RecentWay } from './recent-people.ts';
	import type { Way } from './signin-art.ts';

	type Props = {
		person: RecentPerson;
		name: string;
		pw: string;
		showPw: boolean;
		error: string;
		waiting: number;
		busy: boolean;
		lastWay: Way | '';
		tileWay: boolean;
		failN: number;
		wayName: (w: RecentWay) => string;
		press: (w: Way) => void;
		submit: (e: SubmitEvent) => void;
	};
	let { person, name, pw = $bindable(), showPw = $bindable(), error, waiting, busy, lastWay, tileWay, failN, wayName, press, submit }: Props = $props();
</script>

<div class="who" data-testid="signin-person">
	<SigninFace p={person} size={84} />
	<span class="who-n">{person.name}</span>
	<!-- The password box below already says it; the list keeps the label. -->
	{#if person.way !== 'password'}<span class="acct-way">{wayName(person.way)}</span>{/if}
</div>
{#if person.way === 'plex' || person.way === 'oidc'}
	{@const w = person.way}
	<div class="submit" use:pixelTrail={{ selector: '.ui-btn', busy: busy && lastWay === w, fail: lastWay === w ? failN : 0 }}>
		<Button variant="primary" size="lg" full type="button" {busy} onclick={() => press(w)}>
			{w === 'plex' ? t('accounts.tiles.continueWith', { name: 'Plex' }) : t('accounts.tiles.continueWith', { name: t('accounts.tiles.sso').toLowerCase() })}
		</Button>
	</div>
	<p class="note">{w === 'plex' ? t('accounts.signin.opensPlex') : t('accounts.signin.opensPage')}</p>
	{#if error}<p class="form-error" role="alert">{error}{#if waiting > 0}<span class="mono" aria-hidden="true"> ({waiting}s)</span>{/if}</p>{/if}
{:else}
	<form onsubmit={submit} novalidate>
		<input type="text" name="username" autocomplete="username" value={name} hidden readonly />
		<div class="field">
			<label for="password">{person.way === 'password' ? t('accounts.signin.passwordLabel') : t('accounts.signin.wayPasswordLabel', { way: wayName(person.way) })}</label>
			<div class="pw">
				<input id="password" class="input" type={showPw ? 'text' : 'password'} bind:value={pw} autocomplete="current-password" required />
				<IconButton icon={showPw ? 'eyeOff' : 'eye'} label={t('accounts.signin.showPasswordLabel')} aria-pressed={showPw} onclick={() => (showPw = !showPw)} />
			</div>
		</div>
		{#if error}
			<p class="form-error" role="alert">{error}{#if waiting > 0}<span class="mono" aria-hidden="true"> ({waiting}s)</span>{/if}</p>
		{/if}
		<div class="submit" use:pixelTrail={{ selector: '.ui-btn', busy: busy && !tileWay, fail: tileWay ? 0 : failN }}>
			<Button variant="primary" size="lg" full type="submit" disabled={waiting > 0} {busy}>
				{person.way === 'password' ? t('accounts.signin.signIn') : t('accounts.signin.signInWith', { way: wayName(person.way) })}
			</Button>
		</div>
	</form>
{/if}

<style>
	/* The entrance: each part fades and lifts 6 px, 70 ms after the one before; the logo settles from a hair smaller instead. Opacity and transform only, so it stays on the compositor. */
	@media (prefers-reduced-motion: no-preference) {
		:global(.intro) .field, :global(.intro) .submit {
			animation: part-in var(--dur-slow) var(--ease-out) calc(90ms + var(--i, 1) * 70ms) both;
		}
}
	:global(.gl-wait) .field, :global(.gl-wait) .submit { animation-play-state: paused; }
	:global(.light) .who-n { color: #111; }
	:global(.light) .note, :global(.light) .acct-way { color: rgb(0 0 0 / 0.64); }
	:global(.light) .form-error { background: rgb(255 241 240 / 0.88); border-color: rgb(215 0 21 / 0.35); color: #9b0010; }
	@media (hover: hover) and (prefers-reduced-motion: no-preference) {
		/* A soft lift on light glass; the dark mode's deep shadow looks like a smudge here. */
		:global(.light) .submit :global(.ui-btn:not(:disabled):hover) { box-shadow: 0 4px 14px rgb(0 0 0 / 0.1), 0 1px 3px rgb(0 0 0 / 0.06); }
}
	.pw { position: relative; display: flex; align-items: center; }
	.pw .input { padding-right: 52px; }
	/* Edge draws its own eye in password fields; ours is the only one. */
	.pw .input::-ms-reveal { display: none; }
	.pw > :global(*:last-child) { position: absolute; right: 4px; }
	.submit { margin-top: 20px; }
	/* Busy keeps the button's full colour (only the spinner says it is working); disabled stays dimmed. */
	.submit :global(.ui-btn[aria-busy='true']:disabled) { opacity: 1; }
	/* Errors: a soft red note centred under the fields, with a small warning mark. */
	.form-error {
		margin-top: 14px;
		padding: 10px 14px;
		border-radius: 12px;
		background: rgb(255 69 58 / 0.12);
		border: 1px solid rgb(255 69 58 / 0.26);
		color: #ffb3ad;
		font: 500 13px/1.45 var(--font-ui);
		text-align: center;
	}
	.form-error::before {
		content: '';
		display: inline-block;
		width: 15px;
		height: 15px;
		margin: 0 7px 0 0;
		vertical-align: -3px;
		background: currentColor;
		-webkit-mask: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24'%3E%3Cpath d='M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm-1 5h2v7h-2zm0 9h2v2h-2z'/%3E%3C/svg%3E") center / contain no-repeat;
		mask: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24'%3E%3Cpath d='M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm-1 5h2v7h-2zm0 9h2v2h-2z'/%3E%3C/svg%3E") center / contain no-repeat;
	}
	.who-n { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.acct-way { font: 400 12px/1.2 var(--font-ui); color: rgb(255 255 255 / 0.55); white-space: nowrap; }
	/* One remembered person: a big face in a soft ring, their name under it. */
	.who { display: flex; flex-direction: column; align-items: center; gap: 12px; margin: 4px 0 24px; }
	:global(.title.sr-only) + .who { margin-top: 4px; }
	.who-n { max-width: 100%; color: #fff; font: 600 20px/1.2 var(--font-ui); letter-spacing: -0.01em; }
	/* The way sits close under the name, as in the list rows. */
	.who .acct-way { margin-top: -8px; }
	.note { margin-top: 10px; text-align: center; color: rgb(255 255 255 / 0.55); font: var(--t-footnote); }

	/* Hover lives on the buttons only: a 1 px lift, a soft glow and the site's pixel trail inside the button (pixel-trail.ts paints a canvas clipped to the button's radius). Focus rings come from :focus-visible. */
	.submit :global(.ui-btn) { position: relative; overflow: hidden; isolation: isolate; transition: transform var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out); }
	.submit :global(.ui-btn > :not(canvas)) { position: relative; z-index: 1; }
	@media (hover: hover) and (prefers-reduced-motion: no-preference) {
		.submit :global(.ui-btn:not(:disabled):hover) {
			transform: translateY(-1px);
			box-shadow: 0 6px 22px rgb(0 0 0 / 0.35), 0 0 18px rgb(255 255 255 / 0.08);
		}
		/* Pressed: settle back down, so a click reads as a press. */
		.submit :global(.ui-btn:not(:disabled):active) { transform: translateY(0) scale(0.98); box-shadow: none; }
}

	@media (prefers-reduced-motion: reduce) {
		/* A static press: no scale, just a dimmer face; busy keeps the Button spinner. */
		.submit :global(.ui-btn:active:not(:disabled)) { transform: none; filter: brightness(0.86); }
}
	/* Phones: the desktop card, floating low over full-bleed art with 24 px of art on every side; it rides above the keyboard (--kb) with a smooth lift. */
	@media (max-width: 600px) {
		.who { gap: 10px; margin-bottom: 16px; }
		.who-n { font-size: 18px; }
		.submit :global(.ui-btn) { min-height: 48px; }
		.submit { margin-top: 8px; }
		.submit :global(.ui-btn) { width: 100%; }
		/* Views that do not end on a 44 px link still get room at the bottom edge. */
		:global(.step) > .note:last-child, :global(.step) > .form-error:last-child { margin-bottom: 12px; }
}
</style>
