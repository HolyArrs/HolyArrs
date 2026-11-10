<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { tick } from 'svelte';
	import Button from '#lib/ui/Button.svelte';
	import { ApiError } from '../core/api.ts';
	import { session } from '../core/session.svelte.ts';
	import { pixelTrail } from './pixel-trail.ts';
	import type { Way } from './signin-art.ts';

	type Props = {
		ticket: string;
		pw: string;
		otp: string;
		useBackup: boolean;
		busy: boolean;
		error: string;
		lastWay: Way | '';
		waiting: number;
		failN: number;
		tileWay: boolean;
		failed: (err: unknown) => void;
	};
	let { ticket = $bindable(), pw = $bindable(), otp = $bindable(), useBackup = $bindable(), busy = $bindable(), error = $bindable(), lastWay = $bindable(), waiting, failN, tileWay, failed }: Props = $props();

	// Two-factor: six digit boxes (typing moves on, a pasted or autofilled code fills them all and sends), or one box for a backup code.
	let digits = $state(['', '', '', '', '', '']);
	let digitsEl = $state<HTMLDivElement>();
	const digitInputs = () => [...(digitsEl?.querySelectorAll<HTMLInputElement>('input') ?? [])];
	function clearDigits() {
		digits = ['', '', '', '', '', ''];
		otp = '';
		void tick().then(() => (useBackup ? document.querySelector<HTMLInputElement>('#otp') : digitInputs()[0])?.focus());
	}
	function fillDigits(from: number, text: string): boolean {
		const ds = text.replace(/\D/g, '').slice(0, 6 - from).split('');
		if (!ds.length) return false;
		ds.forEach((d, k) => (digits[from + k] = d));
		otp = digits.join('');
		const ins = digitInputs();
		ds.forEach((d, k) => ins[from + k] && (ins[from + k].value = d));
		ins[Math.min(from + ds.length, 5)]?.focus();
		if (otp.length === 6 && !busy) digitsEl?.closest('form')?.requestSubmit();
		return true;
	}
	function typeDigit(i: number, e: Event) {
		const el = e.currentTarget as HTMLInputElement;
		if (fillDigits(i, el.value)) return;
		digits[i] = '';
		otp = digits.join('');
		el.value = '';
	}
	function digitKey(i: number, e: KeyboardEvent) {
		const ins = digitInputs();
		if (e.key === 'Backspace' && !digits[i] && i > 0) {
			e.preventDefault();
			digits[i - 1] = '';
			otp = digits.join('');
			ins[i - 1].value = '';
			ins[i - 1].focus();
		} else if (e.key === 'ArrowLeft' && i > 0) {
			e.preventDefault();
			ins[i - 1].focus();
		} else if (e.key === 'ArrowRight' && i < 5) {
			e.preventDefault();
			ins[i + 1].focus();
		}
	}
	function pasteDigits(i: number, e: ClipboardEvent) {
		const text = (e.clipboardData?.getData('text') ?? '').replace(/\D/g, '');
		if (!text) return;
		e.preventDefault();
		fillDigits(text.length >= 6 ? 0 : i, text);
	}
	function toggleBackup() {
		useBackup = !useBackup;
		error = '';
		clearDigits();
	}

	async function submitCode(e: SubmitEvent) {
		lastWay = '';
		e.preventDefault();
		error = '';
		// The button stays on; a short code gets a plain note instead of a dead button.
		if (!useBackup && otp.length < 6) {
			error = t('accounts.signin.allDigits');
			digitInputs()[Math.max(0, digits.findIndex((d) => !d))]?.focus();
			return;
		}
		if (useBackup && !otp.trim()) {
			error = t('accounts.signin.enterBackup');
			document.querySelector<HTMLInputElement>('#otp')?.focus();
			return;
		}
		busy = true;
		try {
			await session.loginCode(ticket, otp.trim());
		} catch (err) {
			if (err instanceof ApiError && err.status === 401 && String(err.body.error ?? '').includes('ended')) {
				ticket = '';
				pw = '';
			}
			failed(err);
			clearDigits();
		} finally {
			busy = false;
		}
	}
</script>

<form onsubmit={submitCode} novalidate>
	{#if useBackup}
		<div class="field">
			<label for="otp">{t('accounts.signin.backupCode')}</label>
			<input id="otp" class="input mono otp" inputmode="text" bind:value={otp} autocomplete="off" spellcheck="false" maxlength="11" required />
		</div>
	{:else}
		<div class="digits" role="group" aria-label={t('accounts.signin.sixDigit')} bind:this={digitsEl}>
			{#each digits as d, i (i)}
				{#if i === 3}<span class="dash" aria-hidden="true">–</span>{/if}
				<input
					class="digit"
					value={d}
					inputmode="numeric"
					autocomplete={i === 0 ? 'one-time-code' : 'off'}
					aria-label={t('accounts.signin.digit', { n: i + 1 })}
					onfocus={(e) => e.currentTarget.select()}
					oninput={(e) => typeDigit(i, e)}
					onkeydown={(e) => digitKey(i, e)}
					onpaste={(e) => pasteDigits(i, e)}
				/>
			{/each}
		</div>
	{/if}
	{#if error}
		<p class="form-error" role="alert">{error}{#if waiting > 0}<span class="mono" aria-hidden="true"> ({waiting}s)</span>{/if}</p>
	{/if}
	<div class="submit" use:pixelTrail={{ selector: '.ui-btn', busy: busy && !tileWay, fail: tileWay ? 0 : failN }}><Button variant="primary" size="lg" full type="submit" disabled={waiting > 0} {busy}>{t('accounts.signin.signIn')}</Button></div>
	<button class="link" type="button" onclick={toggleBackup}>{useBackup ? t('accounts.signin.useSix') : t('accounts.signin.useBackup')}</button>
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

	/* Two-factor: six digit boxes, a dash after the third. */
	.digits { display: flex; justify-content: center; gap: 8px; }
	/* The boxes share the row, up to 44 px each, so six always fit (360 px phones too). */
	.digit {
		flex: 1 1 0;
		/* width 0: an empty input's own width must not widen the row (and the card) on 360 px phones. */
		width: 0;
		min-width: 0;
		max-width: 44px;
		height: 54px;
		padding: 0;
		text-align: center;
		border-radius: 12px;
		border: 1px solid var(--line);
		background: var(--surface-2);
		color: var(--text);
		font: 600 22px/1 var(--font-mono);
		caret-color: var(--accent);
	}
	.digit:focus { outline: none; border-color: var(--accent) !important; }
	.dash { align-self: center; color: rgb(255 255 255 / 0.4); }
	:global(.light) .digit { background: rgb(255 255 255 / 0.6); border-color: rgb(255 255 255 / 0.75); color: #111; }
	:global(.light) .dash { color: rgb(0 0 0 / 0.35); }
	.otp { text-align: center; font-size: 22px; letter-spacing: 0.3em; }
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
		.digits { gap: 6px; }
		.digit { max-width: 40px; height: 50px; }
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
