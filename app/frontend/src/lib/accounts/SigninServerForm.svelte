<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { pixelTrail } from './pixel-trail.ts';
	import type { FoundServer } from '../core/session.svelte.ts';

	type Props = {
		kind: 'jellyfin' | 'emby';
		address: string;
		name: string;
		pw: string;
		showPw: boolean;
		foundHere: FoundServer[];
		looking: boolean;
		error: string;
		waiting: number;
		busy: boolean;
		failN: number;
		wayWord: Record<'jellyfin' | 'emby', string>;
		submitServer: (e: SubmitEvent) => void;
		focusStep: () => void;
	};
	let { kind, address = $bindable(), name = $bindable(), pw = $bindable(), showPw = $bindable(), foundHere, looking, error, waiting, busy, failN, wayWord, submitServer, focusStep }: Props = $props();
</script>

<form onsubmit={submitServer} novalidate>
	<div class="field">
		<label for="addr">{t('accounts.signin.address')}</label>
		<input id="addr" class="input mono" bind:value={address} placeholder="http://192.168.1.10:8096" inputmode="url" autocomplete="url" autocapitalize="none" spellcheck="false" maxlength="500" required />
		{#if foundHere.length}
			<div class="accts found" role="group" aria-label={t('accounts.signin.found')} data-testid="setup-found">
				{#each foundHere as f (f.address)}
					<div class="acct">
						<button type="button" class="acct-pick" aria-pressed={address === f.address} onclick={() => { address = f.address; focusStep(); }}>
							<span class="acct-name"><span class="acct-n">{f.name}</span><span class="acct-way mono">{f.address}</span></span>
							<span class="acct-end" aria-hidden="true">{address === f.address ? '✓' : '›'}</span>
						</button>
					</div>
				{/each}
			</div>
		{:else if looking}
			<p class="note looking" role="status">{t('accounts.signin.looking')}</p>
		{/if}
	</div>
	<div class="field">
		<label for="name">{t('accounts.signin.name')}</label>
		<input id="name" class="input" bind:value={name} autocomplete="username" autocapitalize="none" spellcheck="false" maxlength="128" required />
	</div>
	<div class="field">
		<label for="password">{t('accounts.signin.passwordLabel')}</label>
		<div class="pw">
			<input id="password" class="input" type={showPw ? 'text' : 'password'} bind:value={pw} autocomplete="current-password" />
			<IconButton icon={showPw ? 'eyeOff' : 'eye'} label={t('accounts.signin.showPasswordLabel')} aria-pressed={showPw} onclick={() => (showPw = !showPw)} />
		</div>
	</div>
	{#if error}
		<p class="form-error" role="alert">{error}{#if waiting > 0}<span class="mono" aria-hidden="true"> ({waiting}s)</span>{/if}</p>
	{/if}
	<div class="submit" use:pixelTrail={{ selector: '.ui-btn', busy, fail: failN }}>
		<Button variant="primary" size="lg" full type="submit" disabled={waiting > 0} {busy}>{t('accounts.signin.signInWith', { way: wayWord[kind] })}</Button>
	</div>
</form>

<style>
	/* The entrance: each part fades and lifts 6 px, 70 ms after the one before; the logo settles from a hair smaller instead. Opacity and transform only, so it stays on the compositor. */
	@media (prefers-reduced-motion: no-preference) {
		:global(.intro) .field, :global(.intro) .submit {
			animation: part-in var(--dur-slow) var(--ease-out) calc(90ms + var(--i, 1) * 70ms) both;
		}
}
	:global(.gl-wait) .field, :global(.gl-wait) .submit { animation-play-state: paused; }
	:global(.light) .note, :global(.light) .acct-way { color: rgb(0 0 0 / 0.64); }
	:global(.light) .form-error { background: rgb(255 241 240 / 0.88); border-color: rgb(215 0 21 / 0.35); color: #9b0010; }
	:global(.light) .accts { border-color: rgb(0 0 0 / 0.08); }
	:global(.light) .acct + .acct { border-top-color: rgb(0 0 0 / 0.07); }
	/* Rest faint, hover clearly lit: a small step reads as the wrong row when the poster's tint shifts under the glass. */
	:global(.light) .acct-pick { background: rgb(255 255 255 / 0.22); color: #111; }
	@media (hover: hover) { :global(.light) .acct-pick:hover { background: rgb(255 255 255 / 0.8); }
}
	:global(.light) .acct-pick:active { background: rgb(255 255 255 / 0.92); }
	:global(.light) .acct-pick:focus-visible { outline-color: #1d1d1f; }
	:global(.light) .acct-end { color: rgb(0 0 0 / 0.35); }
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
	/* People who signed in here: a list of rows, or one person's step. */
	.accts { display: flex; flex-direction: column; border: 1px solid rgb(255 255 255 / 0.1); border-radius: 16px; overflow: hidden; }
	.acct + .acct { border-top: 1px solid rgb(255 255 255 / 0.08); }
	.acct-pick {
		display: flex;
		align-items: center;
		gap: 14px;
		width: 100%;
		min-height: 64px;
		padding: 12px 18px 12px 14px;
		border: 0;
		background: rgb(255 255 255 / 0.03);
		color: #f5f5f7;
		font: 500 15px/1.2 var(--font-ui);
		text-align: left;
		cursor: pointer;
		transition: background-color var(--dur-fast) var(--ease-out);
	}
	/* One ring, inside the row (the global ring would bleed onto the next row). */
	.acct-pick:focus-visible { outline: 2px solid var(--accent-text); outline-offset: -2px; box-shadow: none; }
	@media (hover: hover) {
		.acct-pick:hover { background: rgb(255 255 255 / 0.12); }
		/* The arrow of the row under the pointer steps forward and darkens or brightens. */
		.acct-pick:hover .acct-end { color: currentColor; transform: translateX(2px); }
}
	.acct-pick:active { background: rgb(255 255 255 / 0.16); }
	.acct-end { transition: transform var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	.acct-name { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
	.acct-n { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.acct-way { font: 400 12px/1.2 var(--font-ui); color: rgb(255 255 255 / 0.55); white-space: nowrap; }
	.acct-end { margin-left: auto; flex: none; color: rgb(255 255 255 / 0.4); font-size: 18px; }
	.note { margin-top: 10px; text-align: center; color: rgb(255 255 255 / 0.55); font: var(--t-footnote); }
	/* First run: servers found on the network sit above the address field. */
	.found { margin-top: 8px; }
	.found .acct-pick { min-height: 56px; }
	.found .acct-pick[aria-pressed='true'] .acct-end { color: var(--accent-text); }
	:global(.light) .found .acct-pick[aria-pressed='true'] .acct-end { color: #1d1d1f; }
	.found .acct-way { overflow: hidden; text-overflow: ellipsis; }
	.looking { margin: 6px 0 0; }

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
		.acct-pick { min-height: 60px; }
		.submit :global(.ui-btn) { min-height: 48px; }
		.submit { margin-top: 8px; }
		.submit :global(.ui-btn) { width: 100%; }
		/* Views that do not end on a 44 px link still get room at the bottom edge. */
}
</style>
