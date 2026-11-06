<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, message } from '#lib/api.ts';
	import { session } from '#lib/session.svelte.ts';
	import { theme } from '#lib/theme.svelte.ts';
	import SigninBackdrop from '#lib/SigninBackdrop.svelte';
	import SigninPortals from '#lib/SigninPortals.svelte';

	const MIN_LENGTH = $derived(session.s.minPassword);
	const linkPart = $derived(page.params.token ?? '');

	let invite = $state<{ role: string; max_rating?: string } | null>(null);
	let gone = $state('');
	let name = $state('');
	let pw = $state('');
	let pwAgain = $state('');
	let error = $state('');
	let busy = $state(false);

	onMount(async () => {
		void session.loadMinPassword();
		try {
			invite = await api.get<{ role: string; max_rating?: string }>(`/auth/invites/${encodeURIComponent(linkPart)}`);
		} catch (e) {
			gone = message(e);
		}
	});

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		if (!name.trim()) return void (error = 'Pick a name.');
		if (pw.length < MIN_LENGTH) return void (error = `The password needs at least ${MIN_LENGTH} characters.`);
		if (pw !== pwAgain) return void (error = 'The two passwords are not the same.');
		busy = true;
		try {
			await session.acceptInvite(linkPart, name.trim(), pw);
			await goto(to('/'), { replace: true });
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}
	let showPw = $state(false);
</script>

<svelte:head><title>Join HolyArrs</title></svelte:head>

<!-- The same frame as the sign-in page: the wallpaper, the edge frame and the glass card (dark ink on light glass in light). -->
<main class="wrap" data-theme="dark">
	<SigninBackdrop />
	<div class="panel" class:light={theme.value === 'light'} data-theme={theme.value} data-signin-panel>
		<div class="brand">
			<!-- Both marks load up front, so switching the appearance never shows a blank logo. -->
			<img class="mark-dark" src={to('/brand/mark-color-on-dark.svg')} alt="" width="36" height="36" />
			<img class="mark-light" src={to('/brand/mark-color.svg')} alt="" width="36" height="36" />
			<span>HolyArrs</span>
		</div>
		<h1 class="title">Join HolyArrs</h1>
		{#if gone}
			<p class="form-error" role="alert">This invite is used up or has ended. Ask for a new one.</p>
			<p class="back"><a href={to(`/login`)}>Go to sign in</a></p>
		{:else if !invite}
			<div class="sk"></div>
		{:else}
			<p class="lead">You were invited as {invite.role === 'admin' ? 'an Admin' : invite.max_rating ? `a Kid (titles rated up to ${invite.max_rating})` : 'a Member'}. Pick the name and password you will sign in with.</p>
			<form onsubmit={submit} novalidate>
				<div class="field">
					<label for="name">Name</label>
					<input id="name" class="input" bind:value={name} autocomplete="username" maxlength="64" required />
				</div>
				<div class="field">
					<label for="password">Password</label>
					<div class="pw">
						<input id="password" class="input" type={showPw ? 'text' : 'password'} bind:value={pw} autocomplete="new-password" required />
						<IconButton icon={showPw ? 'eyeOff' : 'eye'} label={showPw ? 'Hide password' : 'Show password'} aria-pressed={showPw} onclick={() => (showPw = !showPw)} />
					</div>
					<span class="hint">At least {MIN_LENGTH} characters.</span>
				</div>
				<div class="field">
					<label for="pwAgain">Password again</label>
					<input id="pwAgain" class="input" type={showPw ? 'text' : 'password'} bind:value={pwAgain} autocomplete="new-password" required />
				</div>
				{#if error}<p class="form-error" role="alert">{error}</p>{/if}
				<div class="submit"><Button variant="primary" size="lg" full type="submit" {busy}>Join and sign in</Button></div>
			</form>
		{/if}
	</div>
	<SigninPortals />
</main>

<style>
	/* The sign-in page's frame (routes/login): one full-bleed wallpaper, an inset edge frame, and the
	   card on hairline glass. The page stays dark in light mode too; the card turns to light glass. */
	.wrap {
		position: relative;
		min-height: 100dvh;
		display: grid;
		justify-items: center;
		align-items: start;
		--fr: 14px;
		padding: calc(var(--fr) + max(clamp(16px, 5vh, 48px), (100dvh - 600px) / 2)) calc(clamp(16px, 5vw, 96px) + var(--fr)) calc(clamp(16px, 5vh, 48px) + var(--fr));
		background: #050507;
		color: var(--text);
		isolation: isolate;
	}
	:global(html.sa-seeded) .wrap { background: transparent; }
	.panel {
		position: relative;
		z-index: 1;
		--z: 1;
		zoom: var(--z);
		width: min(calc(clamp(360px, 24vw, 440px) / var(--z)), calc(100% / var(--z)));
		padding: 34px 34px 32px;
		border-radius: 28px;
		background: rgb(14 14 18 / 0.46);
		-webkit-backdrop-filter: blur(40px) saturate(1.7) brightness(0.72);
		backdrop-filter: blur(40px) saturate(1.7) brightness(0.72);
		border: 1px solid rgb(255 255 255 / 0.07);
		box-shadow: 0 0 0 1px rgb(0 0 0 / 0.35), 0 30px 90px rgb(0 0 0 / 0.55), 0 6px 18px rgb(0 0 0 / 0.35);
	}
	.panel.light {
		background: rgb(255 253 250 / 0.66);
		-webkit-backdrop-filter: blur(64px) saturate(1.5) brightness(1.5);
		backdrop-filter: blur(64px) saturate(1.5) brightness(1.5);
		border-color: rgb(255 255 255 / 0.55);
		box-shadow: 0 0 0 1px rgb(255 255 255 / 0.25), 0 24px 70px rgb(0 0 0 / 0.2), 0 4px 14px rgb(0 0 0 / 0.08);
		--focus-ring: 0 0 0 2px rgb(255 255 255 / 0.9), 0 0 0 4px #1d1d1f;
	}
	@supports not ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px))) {
		.panel { background: rgb(18 18 22 / 0.92); }
		.panel.light { background: rgb(248 246 242 / 0.94); }
	}
	.mark-light, .light .mark-dark { display: none; }
	.light .mark-light { display: block; }
	.brand { display: flex; align-items: center; justify-content: center; gap: 14px; margin-bottom: 28px; font: 650 24px/1 var(--font-ui); letter-spacing: -0.015em; color: #fff; }
	.title { font: var(--t-title1); letter-spacing: var(--t-title1-track); text-align: center; color: #fff; text-wrap: balance; }
	.lead { margin: 8px 0 20px; color: rgb(255 255 255 / 0.68); font: var(--t-callout); text-align: center; text-wrap: pretty; }
	.light .brand, .light .title { color: #111; }
	.light .lead, .light .back a { color: rgb(0 0 0 / 0.64); }
	.panel :global(.field + .field) { margin-top: 12px; }
	.panel :global(.field) { text-align: center; }
	.panel :global(.field .input) { text-align: left; }
	.light :global(.input) { background: rgb(255 255 255 / 0.6); border-color: rgb(255 255 255 / 0.75); color: #111; }
	.light :global(.input:hover) { border-color: rgb(0 0 0 / 0.16); }
	.light :global(.input:focus-visible) { border-color: var(--accent); box-shadow: 0 0 0 3px rgb(229 53 43 / 0.22); }
	.pw { position: relative; display: flex; align-items: center; }
	.pw .input { padding-right: 52px; }
	.pw .input::-ms-reveal { display: none; }
	.pw > :global(*:last-child) { position: absolute; right: 4px; }
	.submit { margin-top: 20px; }
	.submit :global(.ui-btn[aria-busy='true']:disabled) { opacity: 1; }
	.sk { height: 300px; margin-top: 20px; border-radius: var(--r-card); background: rgb(255 255 255 / 0.06); }
	.light .sk { background: rgb(0 0 0 / 0.05); }
	.form-error { margin-top: 14px; padding: 10px 14px; border-radius: 12px; background: rgb(255 69 58 / 0.12); border: 1px solid rgb(255 69 58 / 0.26); color: #ffb3ad; font: 500 13px/1.45 var(--font-ui); text-align: center; }
	.light .form-error { background: rgb(255 241 240 / 0.88); border-color: rgb(215 0 21 / 0.35); color: #9b0010; }
	.back { margin: 16px 0 0; text-align: center; font: var(--t-subhead); }
	.back a { color: rgb(255 255 255 / 0.78); text-decoration: underline; text-underline-offset: 3px; }
	@media (min-width: 601px) { .wrap { --fr: 18px; } }
	@media (min-width: 1280px) { .wrap { --fr: 22px; } }
	@media (min-width: 1440px) { .wrap { --fr: 24px; } }
	@media (min-width: 1920px) { .wrap { --fr: 32px; } }
	@media (min-width: 1600px) and (min-height: 760px) { .panel { --z: 1.08; } }
	@media (min-width: 2200px) and (min-height: 1000px) { .panel { --z: 1.2; } }
	/* Phones: a floating card at the bottom, as on the sign-in page. */
	@media (max-width: 600px) {
		.wrap { --fr: 0px; place-items: end center; padding: max(24px, env(safe-area-inset-top)) max(24px, env(safe-area-inset-right)) max(24px, calc(env(safe-area-inset-bottom) + 12px)) max(24px, env(safe-area-inset-left)); }
		.panel { width: 100%; max-width: 440px; padding: 14px 16px 16px; border-radius: 24px; }
		.brand { gap: 12px; margin-bottom: 16px; font-size: 18px; }
		.brand img { width: 26px; height: 26px; }
		.title { font-size: 20px; line-height: 1.15; }
		.lead { margin: 4px 0 10px; }
		.panel :global(.field) { gap: 2px; }
		.panel :global(.field + .field) { margin-top: 4px; }
		.panel :global(.input), .submit :global(.ui-btn) { min-height: 48px; }
		.submit { margin-top: 8px; }
	}
</style>
