<script lang="ts">
	import Field from '#lib/ui/Field.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { onMount } from 'svelte';
	import { api, message, ApiError } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { ago, date } from '#lib/format.ts';
	import Icon from '#lib/Icon.svelte';
	import type { ApiKey } from '#lib/types.ts';
	import { session } from '#lib/session.svelte.ts';

	let skipLocal = $state(false);
	let loaded = $state(false);
	let error = $state('');
	let savingSkip = $state(false);
	let savingRequire = $state(false);

	let keys = $state<ApiKey[]>([]);
	let keysNote = $state('');
	let label = $state('');
	let making = $state(false);
	let fresh = $state<{ key: string; label: string } | null>(null);

	function strongNote(e: unknown): string {
		return e instanceof ApiError && e.status === 403 ? 'Sign in with your password to manage API keys.' : message(e);
	}

	interface Way {
		on: boolean;
		url?: string;
		server_id?: string;
		join: 'nobody' | 'list' | 'everyone';
		allow: string;
	}
	type WayName = 'plex' | 'jellyfin' | 'emby';
	interface SignInSettings {
		skip_login_local_network: boolean;
		require_two_factor_admins: boolean;
		min_password_length: number;
		plex: Way;
		jellyfin: Way;
		emby: Way;
	}
	const blank = (): Way => ({ on: false, url: '', server_id: '', join: 'nobody', allow: '' });
	let ways = $state<Record<WayName, Way>>({ plex: blank(), jellyfin: blank(), emby: blank() });
	let requireAdmins = $state(false);
	let minLength = $state(4);
	let savingMinLength = $state(false);
	let savingWay = $state<WayName | ''>('');
	const isOwner = $derived(session.s.role === 'owner');
	const wayWord: Record<WayName, string> = { plex: 'Plex', jellyfin: 'Jellyfin', emby: 'Emby' };

	function took(r: SignInSettings) {
		skipLocal = r.skip_login_local_network;
		requireAdmins = r.require_two_factor_admins;
		minLength = r.min_password_length ?? 4;
		ways = { plex: { ...blank(), ...r.plex }, jellyfin: { ...blank(), ...r.jellyfin }, emby: { ...blank(), ...r.emby } };
	}

	async function saveWay(w: WayName, e: SubmitEvent) {
		e.preventDefault();
		savingWay = w;
		try {
			took(await api.put<SignInSettings>('/auth/settings', { [w]: ways[w] }));
			toasts.push(`${wayWord[w]} sign-in saved.`, 'ok');
		} catch (err) {
			toasts.push(strongNote(err), 'fail');
		} finally {
			savingWay = '';
		}
	}

	async function setRequire(v: boolean) {
		if (savingRequire) return;
		const before = requireAdmins;
		requireAdmins = v;
		savingRequire = true;
		try {
			took(await api.put<SignInSettings>('/auth/settings', { require_two_factor_admins: v }));
			toasts.push(v ? 'Admins must use two-factor sign-in.' : 'Two-factor sign-in is up to each admin.', 'ok');
		} catch (err) {
			requireAdmins = before;
			toasts.push(strongNote(err), 'fail');
		} finally {
			savingRequire = false;
		}
	}

	async function saveMin(e: SubmitEvent) {
		e.preventDefault();
		savingMinLength = true;
		try {
			took(await api.put<SignInSettings>('/auth/settings', { min_password_length: Number(minLength) }));
			session.s.minPassword = minLength;
			toasts.push(`New passwords need at least ${minLength} characters.`, 'ok');
		} catch (err) {
			toasts.push(strongNote(err), 'fail');
		} finally {
			savingMinLength = false;
		}
	}

	onMount(async () => {
		const [s, k] = await Promise.allSettled([api.get<SignInSettings>('/auth/settings'), api.get<{ keys: ApiKey[] }>('/auth/keys')]);
		if (s.status === 'fulfilled') took(s.value);
		else error = message(s.reason);
		if (k.status === 'fulfilled') keys = k.value.keys;
		else keysNote = strongNote(k.reason);
		loaded = true;
	});

	async function setSkip(v: boolean) {
		if (savingSkip) return;
		const before = skipLocal;
		skipLocal = v;
		savingSkip = true;
		try {
			const r = await api.put<{ skip_login_local_network: boolean }>('/auth/settings', { skip_login_local_network: v });
			skipLocal = r.skip_login_local_network;
			toasts.push(skipLocal ? 'No sign-in needed on your local network.' : 'Sign-in needed everywhere.', 'ok');
		} catch (e) {
			skipLocal = before;
			toasts.push(strongNote(e), 'fail');
		} finally {
			savingSkip = false;
		}
	}

	async function makeKey(e: SubmitEvent) {
		e.preventDefault();
		if (!label.trim()) return;
		making = true;
		try {
			const r = await api.post<{ id: number; label: string; key: string }>('/auth/keys', { label: label.trim() });
			fresh = { key: r.key, label: r.label };
			label = '';
			keys = (await api.get<{ keys: ApiKey[] }>('/auth/keys')).keys;
		} catch (err) {
			toasts.push(strongNote(err), 'fail');
		} finally {
			making = false;
		}
	}

	async function revoke(k: ApiKey) {
		if (!confirm(`Revoke the API key "${k.label}"? Anything using it stops working.`)) return;
		try {
			await api.del(`/auth/keys/${k.id}`);
			keys = keys.filter((x) => x.id !== k.id);
			toasts.push('API key revoked.', 'ok');
		} catch (e) {
			toasts.push(strongNote(e), 'fail');
		}
	}

	async function copy() {
		if (!fresh) return;
		try {
			await navigator.clipboard.writeText(fresh.key);
			toasts.push('Copied.', 'ok');
		} catch {
			toasts.push('Copy did not work here; select the key and copy it by hand.', 'fail');
		}
	}
</script>

<svelte:head><title>Sign-in · HolyArrs</title></svelte:head>

<PageHeader title="Sign-in" subtitle="Who can open HolyArrs, and keys for scripts and other apps." />

{#if !loaded}
	<div class="sk" style="height: 240px"></div>
{:else}
	<SettingsCard title="Local network">
		{#if error}
			<p class="notice bad" role="alert" style="margin-top: var(--s-3)">{error}</p>
		{:else}
			<label class="switch" style="margin-top: var(--s-3)">
				<input type="checkbox" checked={skipLocal} disabled={savingSkip} aria-busy={savingSkip} onchange={(e) => setSkip(e.currentTarget.checked)} />
				<span class="sw" aria-hidden="true"></span>
				<span>Skip sign-in on my local network</span>
			</label>
			<p class="hint">
				Devices at home (private addresses) open HolyArrs as the owner, without a password. With family or friends on the same
				network, leave this off: they would get the owner's rights too.
			</p>
			<p class="hint">Changing keys and passwords still needs a real sign-in.</p>
		{/if}
	</SettingsCard>

	<SettingsCard title="Two-factor sign-in">
		<label class="switch" style="margin-top: var(--s-3)">
			<input type="checkbox" checked={requireAdmins} disabled={!isOwner || savingRequire} aria-busy={savingRequire} onchange={(e) => setRequire(e.currentTarget.checked)} />
			<span class="sw" aria-hidden="true"></span>
			<span>Admins must use two-factor sign-in</span>
		</label>
		<p class="hint">
			An admin without it can open only My account until they turn it on. Anyone can turn it on for themselves in My account.{#if !isOwner}
				Only the owner can change this.{/if}
		</p>
	</SettingsCard>

	<SettingsCard title="Passwords">
		<form class="passwords" onsubmit={saveMin}>
			<fieldset disabled={!isOwner}>
				<Field id="min-pw" label="Shortest password allowed">
					<input id="min-pw" class="input" style="max-width: 120px" type="number" min="4" max="64" step="1" required bind:value={minLength} aria-describedby="min-pw-hint min-pw-warn" />
					<span id="min-pw-hint" class="hint">4 to 64 characters. It counts for new passwords: first setup, invites, password changes and reset-owner. Passwords people already have keep working.</span>
				</Field>
				<p id="min-pw-warn" class="notice warn">
					Short passwords are easy to guess. HolyArrs slows down repeated wrong sign-ins, which helps, but it does not make a short
					password safe. If HolyArrs can be reached from outside your home, use 12 or more.
				</p>
				{#if isOwner}
					<Button variant="primary" type="submit" disabled={savingMinLength} aria-busy={savingMinLength}>Save</Button>
				{:else}
					<p class="hint">Only the owner can change this.</p>
				{/if}
			</fieldset>
		</form>
	</SettingsCard>

	{#each ['plex', 'jellyfin', 'emby'] as const as w (w)}
		<SettingsCard title={`Sign in with ${wayWord[w]}`}>
			<p class="small muted" style="margin: var(--s-2) 0 var(--s-4)">
				{#if w === 'plex'}
					People sign in with their Plex account on Plex's own page; HolyArrs never sees their Plex password.
				{:else}
					People sign in with the name and password they use on your {wayWord[w]} server; HolyArrs checks them with the server and keeps neither.
				{/if}
			</p>
			<form class="way" onsubmit={(e) => saveWay(w, e)}>
				<fieldset disabled={!isOwner}>
					<label class="switch">
						<input type="checkbox" bind:checked={ways[w].on} />
						<span class="sw" aria-hidden="true"></span>
						<span>Show the {wayWord[w]} button on the sign-in page</span>
					</label>
					{#if w === 'plex'}
						<Field id="{w}-server" label="Server id people must share (optional)" hint="Your Plex server's machine identifier. When set, only people your server is shared with may join.">
							<input id="{w}-server" class="input mono" bind:value={ways[w].server_id} maxlength="200" spellcheck="false" />
						</Field>
					{:else}
						<Field id="{w}-url" label="{wayWord[w]} server address">
							<input id="{w}-url" class="input mono" bind:value={ways[w].url} placeholder="http://192.168.1.10:8096" maxlength="500" spellcheck="false" />
						</Field>
					{/if}
					<Field id="{w}-join" label="Who may join the first time" hint="New people join as Members who may ask for titles. Change them on the Users page.">
						<select id="{w}-join" class="input" bind:value={ways[w].join}>
							<option value="nobody">Nobody new; only people already linked</option>
							<option value="list">Only the names or emails below</option>
							<option value="everyone">Everyone {w === 'plex' ? 'with a Plex account (who shares the server, when set)' : `with an account on the ${wayWord[w]} server`}</option>
						</select>
					</Field>
					{#if ways[w].join === 'list'}
						<Field id="{w}-allow" label="Names or emails, one per line">
							<textarea id="{w}-allow" class="input" rows="3" bind:value={ways[w].allow} maxlength="4000"></textarea>
						</Field>
					{/if}
					{#if isOwner}
						<Button variant="primary" type="submit" disabled={savingWay === w} aria-busy={savingWay === w}>Save</Button>
					{/if}
				</fieldset>
			</form>
		</SettingsCard>
	{/each}

	<SettingsCard title="Trusted proxies">
		<p class="small muted" style="margin-top: var(--s-2)">
			If HolyArrs sits behind a reverse proxy, list the proxy addresses in <code>HOLYARRS_TRUSTED_PROXIES</code> and extra host
			names in <code>HOLYARRS_ALLOWED_HOSTS</code> when you start it. Editing them here comes in a later stage.
		</p>
	</SettingsCard>

	<SettingsCard title="API keys">
		<p class="small muted" style="margin: var(--s-2) 0 var(--s-4)">For scripts and other apps. Send the key in the <code>X-Api-Key</code> header, never in an address.</p>

		{#if keysNote}
			<p class="notice warn">{keysNote}</p>
		{:else}
			{#if fresh}
				<div class="fresh" role="status">
					<p class="small"><b>{fresh.label}</b>: copy this key now. It is shown only once.</p>
					<div class="row">
						<code class="key grow">{fresh.key}</code>
						<Button icon="copy" type="button" onclick={copy}>Copy</Button>
						<Button variant="plain" type="button" onclick={() => (fresh = null)}>Done</Button>
					</div>
				</div>
			{/if}

			<form class="row" onsubmit={makeKey}>
				<label class="sr-only" for="key-label">Key name</label>
				<input id="key-label" class="input grow" style="max-width: 320px" bind:value={label} placeholder="What uses this key?" maxlength="64" />
				<Button icon="key" variant="primary" type="submit" disabled={making || !label.trim()} aria-busy={making}>Make a key</Button>
			</form>

			{#if keys.length === 0}
				<p class="small muted" style="margin-top: var(--s-4)">No API keys yet.</p>
			{:else}
				<ul class="rows" style="margin-top: var(--s-3)">
					{#each keys as k (k.id)}
						<li>
							<div class="grow">
								<div class="name">{k.label} <span class="mono xs muted">{k.hint}</span></div>
								<div class="xs muted">Made {date(k.created_at)} · {k.last_used_at ? `last used ${ago(k.last_used_at)}` : 'never used'}</div>
							</div>
							<Button variant="destructive" type="button" onclick={() => revoke(k)}>Revoke</Button>
						</li>
					{/each}
				</ul>
			{/if}
		{/if}
	</SettingsCard>
{/if}

<style>
	.passwords fieldset {
		border: 0;
		padding: 0;
		margin: var(--s-3) 0 0;
		min-width: 0;
		display: grid;
		gap: var(--s-3);
	}
	.way fieldset {
		border: 0;
		padding: 0;
		margin: 0;
		display: grid;
		gap: var(--s-3);
		max-width: 520px;
	}
	.way :global(button),
	.passwords :global(button) {
		justify-self: start;
	}
	textarea.input {
		height: auto;
		padding-top: 10px;
	}
	code {
		font-family: var(--font-mono);
		color: var(--text);
	}
	.name {
		font-weight: var(--weight-semibold);
	}
	.fresh {
		padding: var(--s-4);
		border: 1px solid var(--ha-red);
		background: var(--ha-red-soft);
		border-radius: var(--r-btn);
		margin-bottom: var(--s-4);
	}
	.fresh .row {
		margin-top: var(--s-2);
	}
	.key {
		padding: 10px var(--s-3);
		background: var(--surface-2);
		border-radius: var(--r-sm);
		overflow-wrap: anywhere;
		font-size: var(--text-sm);
	}
</style>
