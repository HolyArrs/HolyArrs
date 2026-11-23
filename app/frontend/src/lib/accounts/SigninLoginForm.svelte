<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { pixelTrail } from './pixel-trail.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import AvatarCrop from './AvatarCrop.svelte';
	import { tick, untrack } from 'svelte';

	type Props = {
		mode: 'setup' | 'code' | 'login';
		via: 'choose' | 'password' | 'jellyfin' | 'emby';
		name: string;
		// First run only: the owner's display name, filled from the name until edited.
		display?: string;
		// First run only: the picture picked for the owner, uploaded once the account exists.
		avatar?: File | null;
		// Back from the wizard: the account already exists; its saved picture shows, empty passwords keep the current one, and removing the picture sets avatarGone.
		edit?: boolean;
		// Setup's sign-in (the owner exists, this browser is not signed in): the account step's look, Username and Password side by side.
		setupLook?: boolean;
		currentPic?: string | null;
		avatarGone?: boolean;
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
	let { mode, via, name = $bindable(), display = $bindable(''), avatar = $bindable(null), edit = false, setupLook = false, currentPic = null, avatarGone = $bindable(false), pw = $bindable(), pwAgain = $bindable(), showPw = $bindable(), error, waiting, busy, tileWay, failN, minLength, wayWord, submit, pick }: Props = $props();
	// A kept display name that differs from the username (after a reload) counts as edited.
	let displayEdited = $state(untrack(() => display !== '' && display !== name.trim()));
	// The picture: initials until one is picked and cropped (JPEG, PNG or WebP, up to 50 MB; the crop shrinks it to about 512 px).
	// Starts from what will be saved, so a remount (Back, then on) shows the picked or removed picture.
	let preview = $state(untrack(() => (avatar ? URL.createObjectURL(avatar) : avatarGone ? '' : (currentPic ?? ''))));
	let avatarBtn = $state<HTMLButtonElement>();
	// After the crop closes, focus goes back to the picture button (not the top of the page).
	const refocus = () => void tick().then(() => avatarBtn?.focus({ preventScroll: true }));
	let picker = $state<HTMLInputElement>();
	let picError = $state('');
	// The picked file while the crop shows.
	let cropping = $state<File | null>(null);
	// The circle's size, the same while cropping (180 px on the desktop card, 150 px otherwise).
	const picSize = typeof matchMedia === 'function' && matchMedia('(min-width: 1100px)').matches ? 164 : 150;
	const initials = $derived(
		(display.trim() || name.trim())
			.split(/\s+/)
			.filter(Boolean)
			.slice(0, 2)
			.map((w) => [...w][0]?.toUpperCase() ?? '')
			.join('')
	);
	function pickPic(e: Event) {
		const f = (e.currentTarget as HTMLInputElement).files?.[0];
		(e.currentTarget as HTMLInputElement).value = '';
		usePic(f);
	}
	// A picture pasted from the clipboard (Ctrl+V / Cmd+V) on the account step goes into the same crop; pasted text in the fields is left alone.
	function pastePic(e: ClipboardEvent) {
		if (!picker) return;
		const f = [...(e.clipboardData?.files ?? [])].find((x) => x.type.startsWith('image/'));
		if (!f) return;
		e.preventDefault();
		usePic(f);
	}
	function usePic(f: File | undefined) {
		picError = '';
		if (!f) return;
		if (!/^image\/(jpeg|png|webp)$/.test(f.type)) {
			picError = t('welcome.account.picType');
			return;
		}
		if (f.size > 50 * 1024 * 1024) {
			picError = t('welcome.account.picTooBig');
			return;
		}
		cropping = f;
	}
	function removePic() {
		if (preview && preview !== currentPic) URL.revokeObjectURL(preview);
		preview = '';
		avatar = null;
		if (currentPic) avatarGone = true;
	}
	function cropped(b: Blob, url: string) {
		if (preview && preview !== currentPic) URL.revokeObjectURL(preview);
		avatarGone = false;
		avatar = new File([b], b.type === 'image/webp' ? 'avatar.webp' : 'avatar.jpg', { type: b.type });
		preview = url;
		cropping = null;
		refocus();
	}
	$effect(() => {
		if (mode === 'setup' && !displayEdited) display = name.trim();
	});
</script>

<svelte:window onpaste={pastePic} />

<form onsubmit={submit} novalidate class:acct={mode === 'setup'} class:acct-in={setupLook}>
	{#if mode === 'setup'}
		<div class="acct-grid">
			<div class="pic">
				<!-- A picked photo is cropped right here: the circle shows it, the controls take the hint's fixed space, so nothing else moves. -->
				{#if cropping}
					<AvatarCrop file={cropping} size={picSize} oncancel={() => { cropping = null; refocus(); }} onfail={() => { cropping = null; picError = t('welcome.account.picType'); refocus(); }} ondone={cropped} />
				{:else}
					<button bind:this={avatarBtn} type="button" class="avatar" aria-label={t('welcome.account.picture')} onclick={() => picker?.click()} data-testid="setup-avatar">
						{#if preview}<img src={preview} alt="" />{:else}<span class="pic-empty">{#if initials}<span class="pic-ini" aria-hidden="true">{initials}</span>{:else}<Icon name="user" size={40} />{/if}</span>{/if}<span class="pic-hover" aria-hidden="true"><Icon name="image" size={28} /></span>
					</button>
					<div class="under" style="display: flex; flex-direction: column; align-items: center; gap: 2px;">{#if picError}<span class="hint pic-error" role="alert">{picError}</span>{:else if preview}<span class="pic-acts"><button type="button" class="pic-act" onclick={() => picker?.click()}>{t('welcome.account.changePhoto')}</button><span aria-hidden="true">·</span><button type="button" class="pic-act" onclick={removePic}>{t('welcome.account.removePhoto')}</button></span>{:else}<span class="hint">{t('welcome.account.pictureHint')}</span><span class="hint pic-paste">{t('welcome.account.pasteHint')}</span>{/if}</div>
				{/if}
				<input bind:this={picker} type="file" accept="image/jpeg,image/png,image/webp" hidden onchange={pickPic} />
			</div>
			<div class="fields">
				<div class="field">
					<label for="name">{t('welcome.account.username')}</label>
					<input id="name" class="input" bind:value={name} oninput={(e) => (name = e.currentTarget.value = e.currentTarget.value.replace(/[^A-Za-z0-9_.]/g, '').slice(0, 30))} autocomplete="username" autocapitalize="none" spellcheck="false" maxlength="30" placeholder={t('welcome.account.usernameHint')} required />
				</div>
				<div class="field">
					<label for="display">{t('welcome.account.display')}</label>
					<input id="display" class="input" bind:value={display} oninput={() => (displayEdited = true)} autocomplete="nickname" maxlength="64" placeholder={t('welcome.account.displayHint')} />
				</div>
				<div class="field">
					<label for="password">{t('accounts.signin.passwordLabel')}</label>
					<div class="pw">
						<input id="password" class="input" type={showPw ? 'text' : 'password'} bind:value={pw} autocomplete="new-password" placeholder={edit ? t('welcome.account.keepPasswordHint') : t('accounts.signin.atLeast', { n: minLength })} required={!edit} />
						<IconButton tabindex={-1} icon={showPw ? 'eyeOff' : 'eye'} label={t('accounts.signin.showPasswordLabel')} aria-pressed={showPw} onclick={() => (showPw = !showPw)} />
					</div>
				</div>
				<div class="field">
					<label for="pwAgain">{t('accounts.signin.again')}</label>
					<input id="pwAgain" class="input" type={showPw ? 'text' : 'password'} bind:value={pwAgain} autocomplete="new-password" placeholder={edit ? t('welcome.account.keepPasswordHint') : t('welcome.account.againHint')} required={!edit} />
				</div>
			</div>
		</div>
	{:else}
	<div class:fields={setupLook}>
	<div class="field">
		<label for="name">{t('welcome.account.username')}</label>
		<input id="name" class="input" bind:value={name} autocomplete="username" autocapitalize="none" spellcheck="false" maxlength={via === 'password' ? 64 : 128} required />
	</div>
	<div class="field">
		<label for="password">{t('accounts.signin.passwordLabel')}</label>
		<div class="pw">
			<input id="password" class="input" type={showPw ? 'text' : 'password'} bind:value={pw} autocomplete="current-password" required />
			<IconButton tabindex={-1} icon={showPw ? 'eyeOff' : 'eye'} label={t('accounts.signin.showPasswordLabel')} aria-pressed={showPw} onclick={() => (showPw = !showPw)} />
		</div>
	</div>
	</div>
	{/if}
	{#if error}
		<p class="form-error" role="alert">{error}{#if waiting > 0}<span class="mono" aria-hidden="true"> ({waiting}s)</span>{/if}</p>
	{/if}
	<div class="submit" use:pixelTrail={{ selector: '.ui-btn', busy: busy && !tileWay, fail: tileWay ? 0 : failN }}>
		<Button variant="primary" size="lg" full type="submit" disabled={waiting > 0} {busy}>
			{mode === 'setup' ? (edit ? t('welcome.account.saveContinue') : t('welcome.account.createContinue')) : via === 'jellyfin' || via === 'emby' ? t('accounts.signin.signInWith', { way: wayWord[via] }) : setupLook ? t('welcome.account.signInContinue') : t('accounts.signin.signIn')}
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
	/* The account step: picture on the left, the four fields in a 2 x 2 grid on the right; stacked on phones. */
	.acct-grid { position: relative; display: grid; grid-template-columns: 170px minmax(0, 1fr); gap: 32px; align-items: center; }
	/* Fixed space under the circle: the hint, or the crop's zoom and buttons. */
	.pic :global(.under) { height: 70px; }
	.pic .under { display: flex; align-items: flex-start; justify-content: center; }
	/* Four boxes of one size: two equal columns, the rows spaced by the grid gap only (no stacked-field margin). */
	.fields { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: var(--gap-field, 16px) 24px; }
	.fields > :global(.field) { margin: 0 !important; min-width: 0; }
	.fields :global(.input) { width: 100%; box-sizing: border-box; }
	/* Hints in the boxes: smaller and dimmer than typed text. */
	.fields :global(.input::placeholder) { font-size: 14px; color: rgb(255 255 255 / 0.62); opacity: 1; }
	.pic { display: flex; flex-direction: column; align-items: center; gap: 10px; text-align: center; }
	.pic .hint { white-space: normal; max-width: 100%; text-wrap: balance; }
	.pic-error { color: #ffb3ad; }
	/* Desktop setup card (the login page's 1100 px step): a larger account step that fills the card. */
	@media (min-width: 1100px) {
		.acct-grid { grid-template-columns: 200px minmax(0, 1fr); gap: 40px; }
		.fields { gap: 36px 24px; }
		.acct.acct .fields :global(.input) { height: 56px; min-height: 56px; font-size: 17px; }
			.acct .avatar { width: 164px; height: 164px; font-size: 52px; }
		/* The circle alone is centred on the two field rows; the hint or the crop controls hang under it in their own space, which the button clears. */
		.pic { position: relative; }
		/* The circle and its caption are centred together on the field rows. */
		.pic > .under, .pic :global(.crop .under) { position: static; margin-top: 10px; height: 64px; min-height: 64px; display: flex; flex-direction: column; justify-content: flex-start; }
		.acct .pic { padding-top: 28px; }
		.pic :global(.crop) { position: relative; }
		/* The button sits under the fields only, at the row spacing; the circle column keeps its own space below for the caption and crop controls. */
		.acct .submit { margin-top: 36px; margin-inline-start: 240px; }
			/* The error lines up with the fields and the button, not the whole card. */
		.acct .form-error { width: min(100%, 900px); margin-inline: auto; box-sizing: border-box; }
		/* Setup's sign-in: the account step's field size and spacing. */
		.acct-in .fields { gap: 36px 24px; }
		.acct-in.acct-in .fields :global(.input) { height: 56px; min-height: 56px; font-size: 17px; }
			.acct-in .submit { margin-top: 36px; }
		}
	@media (max-width: 600px) {
		.acct-grid { grid-template-columns: 1fr; gap: var(--gap-field, 16px); }
		.acct-in .fields { grid-template-columns: 1fr; }
		.fields { grid-template-columns: 1fr; }
		.pic { padding-top: 0; }
	}
	.avatar { width: 150px; height: 150px; padding: 0; border-radius: 50%; overflow: hidden; display: grid; place-items: center; border: 1px solid rgb(255 255 255 / 0.16); background: rgb(255 255 255 / 0.08); color: #fff; font: 600 46px/1 var(--font-ui); cursor: pointer; }
	.avatar:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.avatar img { width: 100%; height: 100%; object-fit: cover; }
	:global(.light) .avatar { color: #1d1d1f; border-color: rgb(0 0 0 / 0.12); background: rgb(0 0 0 / 0.05); }
	.pw { position: relative; display: flex; align-items: center; }
	.pw .input { padding-inline-end: 52px; }
	/* Edge draws its own eye in password fields; ours is the only one. */
	.pw .input::-ms-reveal { display: none; }
	.pw > :global(*:last-child) { position: absolute; inset-inline-end: 4px; }
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
		margin: 0; margin-inline-end: 7px;
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
		.submit :global(.ui-btn) { width: 100%; }
		/* Views that do not end on a 44 px link still get room at the bottom edge. */
		.link { min-height: 40px; margin-bottom: 0; margin-top: 0; }
}
	/* Touch screens: 44 px tap targets for the small controls (after the phone block, so it wins). */
	@media (pointer: coarse) {
		.link { min-height: 44px; }
		.pic-act { min-height: 44px; padding: 4px 8px; }
}
	.pic-empty { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; }
	.avatar { position: relative; }
	.under .hint { display: block; margin: 0; line-height: 1.3; }
	.pic-paste { font-size: 12px; margin-top: 0; }
	/* Pointing at (or tabbing to) the circle darkens it and shows a picture icon: the circle itself is the button. */
	.avatar { cursor: pointer; overflow: hidden; }
	.pic-hover { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center; background: rgb(0 0 0 / 0.5); color: #fff; opacity: 0; transition: opacity 0.15s ease; border-radius: 50%; }
	.avatar:hover .pic-hover, .avatar:focus-visible .pic-hover { opacity: 1; }
	.pic-acts { display: inline-flex; align-items: center; gap: 6px; font: 500 13px/1.3 var(--font-ui); opacity: 0.85; }
	.pic-act { background: none; border: 0; padding: 4px 2px; color: inherit; font: inherit; cursor: pointer; text-decoration: underline; text-underline-offset: 3px; }
	@media (hover: hover) { .pic-act:hover { opacity: 1; color: #fff; } }
	/* Light glass: the picture hints, Change · Remove and the placeholders in dark grey that keeps 4.5:1; hover darkens instead of going white. */
	:global(.light) .pic .hint, :global(.light) .pic-acts { color: #3a3a3c; opacity: 1; }
	:global(.light) .pic .pic-error { color: #b3261e; }
	:global(.light) .pic :global(.crop) { color: #1d1d1f; }
	:global(.light) .pic :global(.crop .mini:not(.go)) { border-color: rgb(0 0 0 / 0.2); }
	:global(.light) .pic :global(.crop .mini.go) { background: var(--accent-fill); }
	@media (hover: hover) { :global(.light) .pic-act:hover { color: #000; } }
	:global(.light) .fields :global(.input::placeholder) { color: #5c5c60; opacity: 1; }
</style>
