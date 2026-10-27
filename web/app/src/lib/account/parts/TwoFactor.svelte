<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import { session } from '#lib/session.svelte.ts';
	import type { AccountModel } from '../account.svelte.ts';

	// Two-factor sign-in: backup codes when fresh, the off / new-codes form
	// while on, the setup steps, or the button that starts setup.
	let { m, intro = true }: { m: AccountModel; intro?: boolean } = $props();
</script>

<div class="tf">
	{#if intro}
		<p class="small muted lead">After your password, HolyArrs asks for a six-digit code from an authenticator app on your phone.</p>
	{/if}

	{#if m.codes.length}
		<div class="fresh" role="status">
			<p class="small"><b>Backup codes.</b> Each works once if you lose your phone. Keep them somewhere safe; they are shown only now.</p>
			<ul class="codes">
				{#each m.codes as c (c)}<li>{c}</li>{/each}
			</ul>
			<div class="row">
				<Button icon="copy" type="button" onclick={m.copyCodes}>Copy</Button>
				<Button variant="plain" type="button" onclick={() => (m.codes = [])}>Done</Button>
			</div>
		</div>
	{/if}

	{#if session.s.twoFactor}
		<p class="chip ok">On</p>
		<form class="tf-form" onsubmit={m.withCode}>
			<fieldset class="field">
				<legend class="label">With a code from your app or a backup code</legend>
				<label class="checkline"><input type="radio" bind:group={m.offMode} value="codes" /> Make new backup codes</label>
				<label class="checkline"><input type="radio" bind:group={m.offMode} value="off" /> Turn two-factor sign-in off</label>
			</fieldset>
			<div class="field">
				<label for="off-code">Code</label>
				<input id="off-code" class="input code" bind:value={m.offCode} autocomplete="one-time-code" inputmode="text" maxlength="11" />
			</div>
			<Button variant={m.offMode === 'off' ? 'destructive' : 'secondary'} type="submit" disabled={m.tfBusy || !m.offCode.trim()} busy={m.tfBusy}>
				{m.offMode === 'off' ? 'Turn off' : 'Make new codes'}
			</Button>
		</form>
	{:else if m.setup}
		<ol class="steps small">
			<li>
				Open your authenticator app and add an account. On this phone you can <a class="ul" href={m.setup.uri}>open it in the app</a>, or type this key:
				<code class="key">{m.setup.secret.replace(/(.{4})/g, '$1 ').trim()}</code>
			</li>
			<li>Type the six-digit code the app shows.</li>
		</ol>
		<form class="row on-form" onsubmit={m.enable}>
			<label class="sr-only" for="tf-code">Six-digit code</label>
			<input id="tf-code" class="input code six" bind:value={m.code} autocomplete="one-time-code" inputmode="numeric" maxlength="6" placeholder="123456" />
			<Button variant="primary" type="submit" disabled={m.tfBusy || m.code.trim().length !== 6} aria-busy={m.tfBusy}>Turn on</Button>
			<Button variant="plain" type="button" onclick={() => (m.setup = null)}>Cancel</Button>
		</form>
	{:else}
		<Button icon="shield" variant="primary" type="button" onclick={m.startTwoFactor} disabled={m.tfBusy} aria-busy={m.tfBusy}>Set up two-factor sign-in</Button>
	{/if}
	{#if m.tfError}<p class="form-error" role="alert">{m.tfError}</p>{/if}
</div>

<style>
	.tf { display: flex; flex-direction: column; align-items: flex-start; gap: var(--s-3); min-width: 0; }
	.tf > :global(*) { max-width: 100%; }
	.lead { margin: 0; }
	.fresh { align-self: stretch; padding: var(--s-4); border: 1px solid var(--line); background: var(--surface-2); border-radius: var(--r-btn); }
	.codes { display: grid; grid-template-columns: repeat(auto-fill, minmax(9rem, 1fr)); gap: var(--s-1) var(--s-4); margin: var(--s-3) 0; font-family: var(--font-mono); }
	.steps { margin: 0 0 0 var(--s-5); display: grid; gap: var(--s-2); list-style: decimal; }
	.key { display: block; margin-top: var(--s-2); padding: 10px var(--s-3); background: var(--surface-2); border-radius: var(--r-sm); overflow-wrap: anywhere; font-family: var(--font-mono); color: var(--text); }
	.code { font-family: var(--font-mono); letter-spacing: 0.08em; }
	.six { max-width: 160px; }
	.tf-form { width: min(100%, 420px); }
	.tf-form :global(button) { margin-top: var(--s-4); }
	fieldset { border: 0; padding: 0; margin: 0; }
	.on-form { flex-wrap: wrap; }
	@media (max-width: 600px) {
		.six { flex: 1 1 100%; max-width: none; }
		.on-form > :global(.ui-btn),
		.fresh :global(.ui-btn) { min-height: 44px; }
	}
</style>
