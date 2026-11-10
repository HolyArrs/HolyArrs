<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { pixelTrail } from './pixel-trail.ts';

	type Props = {
		mode: 'setup' | 'code' | 'login';
		via: 'choose' | 'password' | 'jellyfin' | 'emby';
		name: string;
		pw: string;
		pwAgain: string;
		showPw: boolean;
		error: string;
		waiting: number;
		busy: boolean;
		tileWay: boolean;
		failN: number;
		minLength: number;
		wayWord: Record<'jellyfin' | 'emby', string>;
		submit: (e: SubmitEvent) => void;
		pick: (w: 'choose' | 'password' | 'jellyfin' | 'emby') => void;
	};
	let { mode, via, name = $bindable(), pw = $bindable(), pwAgain = $bindable(), showPw = $bindable(), error, waiting, busy, tileWay, failN, minLength, wayWord, submit, pick }: Props = $props();
</script>

<form onsubmit={submit} novalidate>
	<div class="field">
		<label for="name">{t('accounts.signin.name')}</label>
		<input id="name" class="input" bind:value={name} autocomplete="username" autocapitalize="none" spellcheck="false" maxlength={via === 'password' ? 64 : 128} required />
	</div>
	<div class="field">
		<label for="password">{t('accounts.signin.passwordLabel')}</label>
		<div class="pw">
			{#if mode === 'setup'}
				<input id="password" class="input" type={showPw ? 'text' : 'password'} bind:value={pw} autocomplete="new-password" required />
			{:else}
				<input id="password" class="input" type={showPw ? 'text' : 'password'} bind:value={pw} autocomplete="current-password" required />
			{/if}
			<IconButton icon={showPw ? 'eyeOff' : 'eye'} label={t('accounts.signin.showPasswordLabel')} aria-pressed={showPw} onclick={() => (showPw = !showPw)} />
		</div>
		{#if mode === 'setup'}<span class="hint">{t('accounts.signin.atLeast', { n: minLength })}</span>{/if}
	</div>
	{#if mode === 'setup'}
		<div class="field">
			<label for="pwAgain">{t('accounts.signin.again')}</label>
			<input id="pwAgain" class="input" type={showPw ? 'text' : 'password'} bind:value={pwAgain} autocomplete="new-password" required />
		</div>
	{/if}
	{#if error}
		<p class="form-error" role="alert">{error}{#if waiting > 0}<span class="mono" aria-hidden="true"> ({waiting}s)</span>{/if}</p>
	{/if}
	<div class="submit" use:pixelTrail={{ selector: '.ui-btn', busy: busy && !tileWay, fail: tileWay ? 0 : failN }}>
		<Button variant="primary" size="lg" full type="submit" disabled={waiting > 0} {busy}>
			{mode === 'setup' ? t('accounts.signin.create') : via === 'jellyfin' || via === 'emby' ? t('accounts.signin.signInWith', { way: wayWord[via] }) : t('accounts.signin.signIn')}
		</Button>
	</div>
	{#if mode === 'login' && (via === 'jellyfin' || via === 'emby')}
		<button class="link" type="button" onclick={() => pick('choose')}>{t('accounts.signin.other')}</button>
	{/if}
</form>

<style>
	/* The entrance: each part fades and lifts 6 px, 70 ms after the one before; the logo settles from a hair smaller instead. Opacity and transform only, so it stays on the compositor. */
	@media (prefers-reduced-motion: no-preference) {
		:global(.intro) .field, :global(.intro) .submit, :global(.intro .step) .link {
			animation: part-in var(--dur-slow) var(--ease-out) calc(90ms + var(--i, 1) * 70ms) both;
		}
}
	:global(.gl-wait) .field, :global(.gl-wait) .submit, :global(.gl-wait .step) .link { animation-play-state: paused; }
	:global(.light) .link {
		color: #1d1d1f;
		font-weight: 600;
		text-decoration: underline;
		text-underline-offset: 3px;
		text-decoration-color: rgb(0 0 0 / 0.25);
	}
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

	.link {
		display: block;
		margin: 12px auto 0;
		padding: 0 10px;
		min-height: 36px;
		border: 0;
		border-radius: var(--r-sm);
		background: none;
		color: var(--accent-text);
		font: 500 14px/1 var(--font-ui);
		cursor: pointer;
	}
	.link:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	/* Text links, the help summary and Copy: a faint wash on hover, a firmer one on press. */
	@media (hover: hover) {
		.link:hover { background: rgb(255 255 255 / 0.08); }
		:global(.light) .link:hover { background: rgb(0 0 0 / 0.05); }
}
	.link:active { background: rgb(255 255 255 / 0.14); }
	:global(.light) .link:active { background: rgb(0 0 0 / 0.09); }

	@media (prefers-reduced-motion: reduce) {
		/* A static press: no scale, just a dimmer face; busy keeps the Button spinner. */
		.submit :global(.ui-btn:active:not(:disabled)) { transform: none; filter: brightness(0.86); }
}
	/* Phones: the desktop card, floating low over full-bleed art with 24 px of art on every side; it rides above the keyboard (--kb) with a smooth lift. */
	@media (max-width: 600px) {
		.submit :global(.ui-btn) { min-height: 48px; }
		.submit { margin-top: 8px; }
		.submit :global(.ui-btn) { width: 100%; }
		/* Views that do not end on a 44 px link still get room at the bottom edge. */
		.link { min-height: 40px; margin-bottom: 0; margin-top: 0; }
}
	/* Touch screens: 44 px tap targets for the small controls (after the phone block, so it wins). */
	@media (pointer: coarse) {
		.link { min-height: 44px; }
}
</style>
