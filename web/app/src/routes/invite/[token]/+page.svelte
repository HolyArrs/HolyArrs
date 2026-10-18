<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, message } from '#lib/api.ts';
	import { session } from '#lib/session.svelte.ts';
	import { theme } from '#lib/theme.svelte.ts';

	const MIN_LENGTH = $derived(session.s.minPassword);
	const linkPart = $derived(page.params.token ?? '');
	const mark = $derived(theme.value === 'light' ? '/brand/mark-color.svg' : '/brand/mark-color-on-dark.svg');

	let invite = $state<{ role: string } | null>(null);
	let gone = $state('');
	let name = $state('');
	let pw = $state('');
	let pwAgain = $state('');
	let error = $state('');
	let busy = $state(false);

	onMount(async () => {
		void session.loadMinPassword();
		try {
			invite = await api.get<{ role: string }>(`/auth/invites/${encodeURIComponent(linkPart)}`);
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

<main class="wrap">
	<div class="panel rise">
		<div class="brand">
			<img src={to(mark)} alt="" width="44" height="44" />
			<span>HolyArrs</span>
		</div>
		<h1 class="title">Join HolyArrs</h1>
		{#if gone}
			<p class="notice bad" role="alert" style="margin-top: var(--s-4)">{gone}</p>
			<p class="small muted" style="margin-top: var(--s-4)"><a class="ul" href={to(`/login`)}>Go to the sign-in page</a></p>
		{:else if !invite}
			<div class="sk" style="height: 220px; margin-top: var(--s-4)"></div>
		{:else}
			<p class="lead">You were invited as {invite.role === 'admin' ? 'an Admin' : 'a Member'}. Pick the name and password you will sign in with.</p>
			<form onsubmit={submit} novalidate>
				<div class="field">
					<label for="name">Name</label>
					<input id="name" class="input" bind:value={name} autocomplete="username" maxlength="64" required />
				</div>
				<div class="field">
					<label for="password">Password</label>
					<div class="pw">
						<input id="password" class="input" type={showPw ? 'text' : 'password'} bind:value={pw} autocomplete="new-password" required />
						<Button variant="plain" size="sm" type="button" aria-label={showPw ? 'Hide password' : 'Show password'} aria-pressed={showPw} onclick={() => (showPw = !showPw)}>{showPw ? 'Hide' : 'Show'}</Button>
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
</main>

<style>
	.wrap {
		min-height: 100dvh;
		display: grid;
		place-items: center;
		padding: clamp(16px, 5vw, 48px);
		background: radial-gradient(120% 80% at 50% 0%, var(--accent-soft), transparent 60%), var(--bg);
	}
	.panel {
		width: min(420px, 100%);
		padding: clamp(24px, 5vw, 40px);
		border-radius: var(--r-modal);
		background: var(--surface-1);
		box-shadow: var(--shadow-modal), 0 0 0 1px var(--line);
	}
	.brand {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 10px;
		margin-bottom: 20px;
		font: var(--t-headline);
		color: var(--text-muted);
	}
	.title { font: var(--t-title1); letter-spacing: var(--t-title1-track); text-align: center; }
	.lead { margin: 8px 0 24px; color: var(--text-muted); font: var(--t-callout); text-align: center; }
	.panel :global(.field + .field) { margin-top: 14px; }
	.pw { position: relative; display: flex; align-items: center; }
	.pw .input { padding-right: 72px; }
	.pw > :global(*:last-child) { position: absolute; right: 4px; }
	.submit { margin-top: 24px; }
	@media (max-width: 480px) {
		.wrap { place-items: start center; padding: 24px 16px; }
		.panel { box-shadow: none; background: transparent; padding: 8px 0; }
	}
</style>
