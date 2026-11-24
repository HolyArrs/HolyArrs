<!-- Settings > Media servers, adding Plex: pick one of the owner's own Plex servers and which of its addresses to use. The address is saved with the owner's Plex sign-in, so no token is typed. -->
<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { api, ApiError, message } from '#lib/core/api.ts';
	import Button from '#lib/ui/Button.svelte';
	import Field from '#lib/ui/Field.svelte';
	import { firstPlexRow, plexAddresses, type PlexServerRow } from './plex-pick.ts';

	let { url = $bindable(''), name = $bindable(''), usePlex = $bindable(false) }: { url?: string; name?: string; usePlex?: boolean } = $props();

	let rows = $state<PlexServerRow[]>([]);
	let pick = $state('manual');
	let finding = $state(false);
	let note = $state('');
	const row = $derived(pick === 'manual' ? undefined : rows[Number(pick)]);
	const addresses = $derived(row ? plexAddresses(row) : []);

	const where = (c: PlexServerRow) => (c.relay ? t('settings.mediaServers.plexRelay') : c.local ? t('welcome.server.local') : t('welcome.server.remote'));

	async function find() {
		finding = true;
		note = '';
		try {
			rows = (await api.get<{ servers: PlexServerRow[] }>('/auth/plex/servers')).servers ?? [];
			note = t('welcome.server.found', { count: rows.length });
			const i = firstPlexRow(rows);
			choose(i < 0 ? 'manual' : String(i));
		} catch (err) {
			note = err instanceof ApiError && err.status === 409 ? t('settings.mediaServers.plexNotLinked') : message(err);
		} finally {
			finding = false;
		}
	}
	function choose(v: string) {
		pick = v;
		const c = rows[Number(v)];
		usePlex = v !== 'manual' && !!c;
		if (!usePlex || !c) return;
		url = plexAddresses(c)[0] ?? '';
		if (!name.trim()) name = c.name;
	}
</script>

<div class="pick" data-testid="plex-pick">
	<div class="find">
		<Button variant="secondary" size="sm" icon="search" type="button" disabled={finding} aria-busy={finding} onclick={find} data-testid="plex-pick-find">{t('welcome.server.refresh')}</Button>
		{#if note}<span class="hint" role="status">{note}</span>{/if}
	</div>
	{#if rows.length}
		<Field id="ms-plex-pick" label={t('settings.mediaServers.plexPick')}>
			<select id="ms-plex-pick" class="input" value={pick} onchange={(e) => choose((e.currentTarget as HTMLSelectElement).value)} data-testid="plex-pick-server">
				{#each rows as c, i (c.id + c.address + c.port)}<option value={String(i)}>{c.name} · {c.address} · {where(c)}</option>{/each}
				<option value="manual">{t('welcome.server.manual')}</option>
			</select>
		</Field>
		{#if addresses.length > 1}
			<Field id="ms-plex-addr" label={t('settings.mediaServers.plexAddress')}>
				<select id="ms-plex-addr" class="input mono" bind:value={url} data-testid="plex-pick-address">
					{#each addresses as a (a)}<option value={a}>{a}</option>{/each}
				</select>
			</Field>
		{/if}
		{#if usePlex}<p class="hint">{t('settings.mediaServers.plexUsesSignIn')}</p>{/if}
	{/if}
</div>

<style>
	.pick {
		margin-top: var(--s-4);
	}
	.find {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		flex-wrap: wrap;
	}
	.pick select {
		max-width: 100%;
		text-overflow: ellipsis;
	}
</style>
