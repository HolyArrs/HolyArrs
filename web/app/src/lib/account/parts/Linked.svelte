<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import { session } from '#lib/session.svelte.ts';
	import type { AccountModel } from '../account.svelte.ts';

	// Linked sign-in: single sign-on or a trusted proxy, after a recent local sign-in.
	let {
		m,
		intro = true,
		confirmWord = 'Confirm your local sign-in first, then link another sign-in method.'
	}: { m: AccountModel; intro?: boolean; confirmWord?: string } = $props();
</script>

{#if intro}
	<p class="small muted lead">
		Link another sign-in only while you are already signed in. HolyArrs matches the provider's stable account id and never links accounts by email address.
	</p>
{/if}
{#if !session.s.localRecent}
	<p class="notice warn small">{confirmWord}</p>
{:else}
	<div class="row">
		{#if session.s.ways.includes('oidc')}
			<Button type="button" onclick={m.startOIDCLink} disabled={m.linkBusy} aria-busy={m.linkBusy}>Link single sign-on</Button>
		{/if}
		{#if session.s.ways.includes('forward_auth')}
			<Button type="button" onclick={m.linkForwardAuth} disabled={m.linkBusy} aria-busy={m.linkBusy}>Link trusted proxy sign-in</Button>
		{/if}
	</div>
{/if}
{#if m.linkError}<p class="form-error" role="alert">{m.linkError}</p>{/if}

<style>
	.lead { margin: 0 0 var(--s-3); }
</style>
