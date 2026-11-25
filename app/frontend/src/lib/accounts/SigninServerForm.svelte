<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { pixelTrail } from './pixel-trail.ts';
	import type { FoundServer } from '../core/session.svelte.ts';
	import { untrack, type Snippet } from 'svelte';

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
		onrefresh?: () => void;
		// The main button's words (default: Sign in with <server>).
		submitLabel?: string;
		// Username and Password side by side (setup's server screen, where the card has no room to stack them).
		pair?: boolean;
		// Shown in the main button's place once connected (setup's server screen).
		done?: Snippet;
	};
	let { kind, address = $bindable(), name = $bindable(), pw = $bindable(), showPw = $bindable(), foundHere, looking, error, waiting, busy, failN, wayWord, submitServer, focusStep, onrefresh, submitLabel, pair = false, done }: Props = $props();
	// Type it myself: the dropdown becomes an editable address box until the next search.
	let typing = $state(false);
	// The http:// / https:// tag at the start of the box: picked on its own (http at home, https outside), one click flips it.
	let scheme = $state<'http' | 'https'>(untrack(() => (address.startsWith('https:') ? 'https' : 'http')));
	let schemePicked = false;
	// A saved address (setup's server screen) opens filled.
	let rest = $state(untrack(() => address.replace(/^https?:\/\//, '').replace(/:\d+\/?$/, '').replace(/\/+$/, '')));
	// Port: its own small box beside the address; 8096 unless the server or the person says otherwise.
	let port = $state(untrack(() => /:(\d+)\/?$/.exec(address)?.[1] ?? '8096'));
	const portOf = (a: string) => /:(\d+)$/.exec(a.replace(/\/+$/, ''))?.[1] ?? '';
	const hostOf = (a: string) => a.replace(/^https?:\/\//, '').replace(/:\d+$/, '').replace(/\/+$/, '');
	const shownPort = $derived(typing || !foundHere.length ? port : portOf(address));
	const homeHost = (h: string) => !h.includes('.') || /^(10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.|127\.|169\.254\.)/.test(h) || /\.(local|lan|home|internal|home\.arpa)$/i.test(h);
	const shownScheme = $derived(typing || !foundHere.length ? scheme : address.startsWith('https:') ? 'https' : 'http');
	function setAddress() {
		const v = rest.trim();
		const p = port.trim();
		address = v ? `${scheme}://${v}${p ? `:${p}` : ''}` : '';
	}
	function typed() {
		const m = /^(https?):\/\/(.*)$/i.exec(rest.trim());
		if (m) {
			scheme = m[1].toLowerCase() as 'http' | 'https';
			schemePicked = true;
			rest = m[2];
		}
		// host:port typed in the address: the port moves to its own box.
		const hp = /^([^/:]+):(\d{1,5})$/.exec(rest.trim());
		if (hp) {
			rest = hp[1];
			port = hp[2];
		}
		if (!m && !schemePicked) scheme = homeHost(rest.trim().split(/[/:]/)[0]) ? 'http' : 'https';
		setAddress();
	}
	function flipScheme() {
		if (typing || !foundHere.length) {
			scheme = scheme === 'http' ? 'https' : 'http';
			schemePicked = true;
			setAddress();
		} else {
			// A found server switched by hand: it becomes a typed address, since the list only holds what the server announced.
			scheme = shownScheme === 'http' ? 'https' : 'http';
			schemePicked = true;
			rest = hostOf(address);
			port = portOf(address) || port;
			typing = true;
			setAddress();
		}
	}
	// Changing the port of a found server turns it into a typed address.
	function portTyped() {
		if (!typing && foundHere.length) {
			scheme = address.startsWith('https:') ? 'https' : 'http';
			schemePicked = true;
			rest = hostOf(address);
			typing = true;
		}
		setAddress();
	}
	function pickServer(v: string) {
		if (v) return void (address = v);
		typing = true;
		address = '';
		rest = '';
		schemePicked = false;
		queueMicrotask(() => document.getElementById('addr')?.focus());
	}
	// New search results fill the address with the first server found; typing over it is kept until the next search.
	let filledFrom: FoundServer[] | null = null;
	$effect(() => {
		if (foundHere.length && foundHere !== filledFrom) {
			filledFrom = foundHere;
			address = foundHere[0].address;
		}
	});
</script>

<form onsubmit={submitServer} novalidate>
	<div class="field">
		<div class="addr-labels"><label for="setup-scheme-btn" class="proto-label">{t('welcome.server.protocol')}</label><label for="addr">{t('welcome.server.ipOrHostPlain')}</label><label for="port" class="port-label">{t('welcome.server.port')}</label></div>
		<div class="addr-line">
			<button id="setup-scheme-btn" type="button" class="input scheme" style="padding: 0; text-align: center; justify-content: center;" class:secure={shownScheme === 'https'} onclick={flipScheme} disabled={busy} aria-label={t('welcome.server.schemeFlip', { scheme: shownScheme })} data-testid="setup-scheme">{shownScheme}://</button>
		<!-- One field: a dropdown of found servers (Type it myself on top) beside the refresh button; Type it myself turns it into an address box. -->
		<div class="addr-row" class:is-select={foundHere.length && !typing}>
			{#if foundHere.length && !typing}
				<select id="addr" class="input" value={address} onchange={(e) => pickServer(e.currentTarget.value)} disabled={busy} data-testid="setup-found">
					<option value="">{t('welcome.server.manual')}</option>
					{#each foundHere as f (f.address)}<option value={f.address}>{hostOf(f.address)}</option>{/each}
				</select>
			{:else}
				<input id="addr" class="input" bind:value={rest} oninput={typed} placeholder={looking ? t('accounts.signin.looking') : t('welcome.server.findPlaceholder')} inputmode="url" autocomplete="url" autocapitalize="none" spellcheck="false" maxlength="500" required />
			{/if}
			{#if onrefresh}<span class="in-field">{#if foundHere.length && !typing}<span class="chev" aria-hidden="true"><Icon name="chevronDown" size={16} /></span><span class="sep" aria-hidden="true"></span>{/if}<IconButton icon="refresh" variant="tinted" label={t('welcome.server.refresh')} onclick={() => { typing = false; onrefresh(); }} disabled={looking || busy} aria-busy={looking} data-testid="setup-find-refresh" /></span>{/if}
		</div>
			<span class="port-wrap"><input id="port" class="input port" value={shownPort} oninput={(e) => { port = e.currentTarget.value.replace(/\D/g, '').slice(0, 5); if (Number(port) > 65535) port = '65535'; e.currentTarget.value = port; portTyped(); }} inputmode="numeric" autocomplete="off" disabled={busy} data-testid="setup-port" /></span>
		</div>
	</div>
	<div class:pair>
	<div class="field">
		<label for="name">{t('welcome.account.username')}</label>
		<input id="name" class="input" bind:value={name} autocomplete="username" autocapitalize="none" spellcheck="false" maxlength="128" required />
	</div>
	<div class="field">
		<label for="password">{t('accounts.signin.passwordLabel')}</label>
		<div class="pw">
			<input id="password" class="input" type={showPw ? 'text' : 'password'} bind:value={pw} autocomplete="current-password" />
			<IconButton tabindex={-1} icon={showPw ? 'eyeOff' : 'eye'} label={t('accounts.signin.showPasswordLabel')} aria-pressed={showPw} onclick={() => (showPw = !showPw)} />
		</div>
	</div>
	</div>
	{#if error}
		<p class="form-error" role="alert">{error}{#if waiting > 0}<span class="mono" aria-hidden="true"> ({waiting}s)</span>{/if}</p>
	{/if}
	{#if done}<div class="submit">{@render done()}</div>{:else}
	<div class="submit" use:pixelTrail={{ selector: '.ui-btn', busy, fail: failN }}>
		<Button variant="primary" size="lg" full type="submit" disabled={waiting > 0} {busy}>{submitLabel ?? t('accounts.signin.signInWith', { way: wayWord[kind] })}</Button>
	</div>
	{/if}
</form>

<style>
	/* The entrance: each part fades and lifts 6 px, 70 ms after the one before; the logo settles from a hair smaller instead. Opacity and transform only, so it stays on the compositor. */
	@media (prefers-reduced-motion: no-preference) {
		:global(.intro) .field, :global(.intro) .submit {
			animation: part-in var(--dur-slow) var(--ease-out) calc(90ms + var(--i, 1) * 70ms) both;
		}
}
	:global(.gl-wait) .field, :global(.gl-wait) .submit { animation-play-state: paused; }
	:global(.light) .form-error { background: rgb(255 241 240 / 0.88); border-color: rgb(215 0 21 / 0.35); color: #9b0010; }
	/* Rest faint, hover clearly lit: a small step reads as the wrong row when the poster's tint shifts under the glass. */
	@media (hover: hover) and (prefers-reduced-motion: no-preference) {
		/* A soft lift on light glass; the dark mode's deep shadow looks like a smudge here. */
		:global(.light) .submit :global(.ui-btn:not(:disabled):hover) { box-shadow: 0 4px 14px rgb(0 0 0 / 0.1), 0 1px 3px rgb(0 0 0 / 0.06); }
}
	.pw { position: relative; display: flex; align-items: center; }
	.pw .input { padding-right: 52px; }
	/* Edge draws its own eye in password fields; ours is the only one. */
	.pw .input::-ms-reveal { display: none; }
	.pw > :global(*:last-child) { position: absolute; right: 4px; }
	/* A flex box, not a line box: an inline button there lands on a fractional line position and the gap above it reads 1 px short on phones. */
	.submit { display: flex; flex-direction: column; margin-top: var(--gap, 20px); }
	/* Busy keeps the button's full colour (only the spinner says it is working); disabled stays dimmed. */
	.submit :global(.ui-btn[aria-busy='true']:disabled) { opacity: 1; }
	/* Errors: a soft red note centred under the fields, with a small warning mark. */
	.form-error {
		margin-top: var(--gap, 20px);
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
	/* One ring, inside the row (the global ring would bleed onto the next row). */
	@media (hover: hover) {
		/* The arrow of the row under the pointer steps forward and darkens or brightens. */
}
	/* First run: servers found on the network sit above the address field. */
	/* The refresh button sits inside the field, so this box is as wide as Name and Password. */
	.addr-row { position: relative; min-width: 0; }
	/* Its own spacing and centring, so it looks the same inside the setup wizard card as on the sign-in page. */
	.field { text-align: center; }
	.field .input { text-align: left; }
	.field + .field, :not(.pair) > .field + div:not(.pair), .field + .pair { margin-top: var(--gap-field, 28px); }
	.pair { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 14px; }
	.pair > .field + .field { margin-top: 0; }
	.addr-labels > label { display: block; }
	/* Address and Port share one row, as wide as the fields below. */
	/* Main label centred over the whole row (like Name and Password); Port sits above its own box. */
	.addr-labels { position: relative; }
	.addr-labels .proto-label { position: absolute; inset-inline-start: 0; top: 0; width: 80px; text-align: center; }
	.addr-labels .port-label { position: absolute; inset-inline-end: 0; top: 0; width: 80px; text-align: center; }
	.addr-line { display: grid; grid-template-columns: 80px minmax(0, 1fr) 80px; gap: 14px; }
	/* Port's name sits small inside its box, so the row has one centred label like Name and Password. */
	.port-wrap { position: relative; display: block; }
	.addr-line .port-wrap .input.port { text-align: center; }
	.addr-line .input.scheme { padding: 0; text-align: center; justify-content: center; font-weight: 400; }
	.addr-line .port { width: 100%; box-sizing: border-box; text-align: center; padding: 0 8px; }
	.addr-row .input { width: 100%; box-sizing: border-box; padding-inline-end: 52px; }
	/* The http:// / https:// tag: its own box left of the address, as tall as the fields. */
	.addr-line .scheme { width: 80px; min-width: 0; padding: 0; font-weight: 400; cursor: pointer; text-align: center; }
	.scheme.secure { border-color: rgb(52 199 89 / 0.5); }
	@media (hover: hover) { .scheme:hover:not(:disabled) { background: rgb(255 255 255 / 0.14); } }
	/* Dropdown: the browser's arrow is hidden; ours sits joined to the refresh button as one control (arrow | refresh). Clicks on the arrow fall through to the dropdown. */
	.addr-row.is-select .input { appearance: none; -webkit-appearance: none; padding-inline-end: 92px; cursor: pointer; }
	.in-field { position: absolute; inset-inline-end: 6px; top: 50%; transform: translateY(-50%); display: flex; align-items: center; gap: 2px; pointer-events: none; }
	.in-field :global(button) { pointer-events: auto; }
	.chev { display: flex; width: 28px; justify-content: center; opacity: 0.75; pointer-events: none; }
	.sep { width: 1px; height: 20px; background: currentColor; opacity: 0.2; margin-inline: 2px; pointer-events: none; }

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
		.submit :global(.ui-btn) { min-height: 48px; }
		.submit :global(.ui-btn) { width: 100%; }
		/* Views that do not end on a 44 px link still get room at the bottom edge. */
}
</style>
