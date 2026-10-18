<script lang="ts">
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
</script>

<svelte:head><title>Join HolyArrs</title></svelte:head>

<div class="wrap">
	<div class="panel rise">
		<div class="brand">
			<img src={to(mark)} alt="" width="44" height="44" />
			<span>HolyArrs</span>
		</div>
		<h1 class="h2">Join HolyArrs</h1>
		{#if gone}
			<p class="notice bad" role="alert" style="margin-top: var(--s-4)">{gone}</p>
			<p class="small muted" style="margin-top: var(--s-4)"><a class="ul" href={to(`/login`)}>Go to the sign-in page</a></p>
		{:else if !invite}
			<div class="sk" style="height: 220px; margin-top: var(--s-4)"></div>
		{:else}
			<p class="muted small lead">You were invited as {invite.role === 'admin' ? 'an Admin' : 'a Member'}. Pick the name and password you will sign in with.</p>
			<form onsubmit={submit} novalidate>
				<div class="field">
					<label for="name">Name</label>
					<input id="name" class="input" bind:value={name} autocomplete="username" maxlength="64" required />
				</div>
				<div class="field">
					<label for="password">Password</label>
					<input id="password" class="input" type="password" bind:value={pw} autocomplete="new-password" required />
					<span class="hint">At least {MIN_LENGTH} characters.</span>
				</div>
				<div class="field">
					<label for="pwAgain">Password again</label>
					<input id="pwAgain" class="input" type="password" bind:value={pwAgain} autocomplete="new-password" required />
				</div>
				{#if error}<p class="form-error" role="alert">{error}</p>{/if}
				<button class="btn btn-red btn-block submit" type="submit" disabled={busy} aria-busy={busy}>Join and sign in</button>
			</form>
		{/if}
	</div>
</div>

<style>
	.wrap {
		min-height: 100dvh;
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		align-items: center;
		gap: var(--s-6);
		width: min(560px, 100%);
		margin: 0 auto;
		padding: clamp(var(--s-6), 6vw, var(--s-10));
		background: var(--bg);
	}
	.panel {
		width: 100%;
		background: transparent;
		border: 0;
		border-radius: 0;
		padding: var(--s-8);
		box-shadow: none;
	}
	.brand {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		margin-bottom: var(--s-6);
		font: var(--weight-bold) 22px/1 var(--font-display);
		letter-spacing: var(--tracking-display);
	}
	.lead {
		margin: var(--s-2) 0 var(--s-6);
	}
	.submit {
		margin-top: var(--s-6);
	}
	@media (max-width: 800px) {
		.wrap {
			grid-template-columns: minmax(0, 1fr);
			width: min(500px, 100%);
		}
	}
	@media (max-width: 480px) {
		.wrap {
			align-items: start;
			padding: var(--s-4);
		}
		.panel {
			padding: var(--s-6) var(--s-4);
		}
	}
</style>
