<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/Icon.svelte';
	import { ago, dateTime } from '#lib/format.ts';
	import type { AccountModel } from '../account.svelte.ts';
	import { deviceIcon } from '../device.ts';

	// Signed-in devices with Sign out, and Sign out all other devices.
	// look="rows" is a plain list; "tiles" gives each device an icon tile.
	let { m, look = 'rows', all = true }: { m: AccountModel; look?: 'rows' | 'tiles'; all?: boolean } = $props();
</script>

<div class="devices {look}" data-testid="sessions">
	{#if m.rowsError}
		<p class="form-error" role="alert">{m.rowsError}</p>
	{:else if !m.rows}
		<div class="sk" style="height: 64px"></div>
	{:else if m.rows.length === 0}
		<p class="small muted">There are no active sessions to manage.</p>
	{:else}
		<ul>
			{#each m.shownRows ?? [] as s (s.id)}
				<li data-testid="session" data-current={s.current}>
					{#if look === 'tiles'}<span class="tile" class:cur={s.current}><Icon name={deviceIcon(s.device)} size={18} /></span>{/if}
					<span class="grow">
						<span class="dev">{s.device}{#if s.current}<span class="here">This device</span>{/if}</span>
						<span class="xs muted when">Signed in {dateTime(s.created_at)} · last used <time datetime={s.last_used_at}>{ago(s.last_used_at)}</time></span>
					</span>
					{#if !s.current}
						<Button
							variant="plain"
							size="sm"
							type="button"
							disabled={m.rowsBusy}
							onclick={() => m.endSession(s)}
							aria-label="Sign out {s.device}, last used {ago(s.last_used_at)}">Sign out</Button>
					{/if}
				</li>
			{/each}
		</ul>
		{#if m.hiddenDevices > 0}
			<button type="button" class="more" onclick={() => (m.allDevices = true)}>Show all {m.rows.length} devices</button>
		{/if}
		{#if all && m.otherDevices > 0}
			<div class="all">
				<Button type="button" disabled={m.rowsBusy} aria-busy={m.rowsBusy} onclick={m.endOtherSessions}>Sign out all other devices</Button>
			</div>
		{/if}
	{/if}
</div>

<style>
	ul { display: flex; flex-direction: column; }
	li { display: flex; align-items: center; gap: var(--s-3); min-height: 60px; padding: 10px 0; border-bottom: 1px solid var(--line); }
	li:last-child { border-bottom: 0; }
	.tile { width: 36px; height: 36px; flex: none; display: grid; place-items: center; border-radius: 10px; background: var(--surface-2); color: var(--text-muted); }
	.tile.cur { background: color-mix(in srgb, var(--ok) 14%, transparent); color: var(--ok-text); }
	.grow { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
	.dev { font-weight: 600; font-size: var(--text-sm); overflow-wrap: anywhere; display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
	.here { font: var(--t-caption); color: var(--ok-text); background: color-mix(in srgb, var(--ok) 12%, transparent); padding: 2px 7px; border-radius: var(--r-pill); }
	.when { overflow-wrap: anywhere; }
	.all { margin-top: var(--s-3); }
	.more { min-height: 44px; color: var(--accent-text); font: var(--t-callout); font-weight: 600; }
	.more:hover { text-decoration: underline; }
	@media (max-width: 600px) {
		.all :global(.ui-btn) { width: 100%; min-height: 44px; }
	}
</style>
