<script lang="ts">
	// Connect Jackett or Prowlarr (#96): HolyArrs takes the address and key, tests them, and brings the sites in as
	// Sources in one step. Jackett's sites stay in step on their own;
	// Prowlarr writes its sites in through the old-app API.
	import { onMount } from 'svelte';
	import { api, ApiError, message } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { ago } from '#lib/format.ts';
	import { to } from '#lib/paths.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Field from '#lib/ui/Field.svelte';
	import Button from '#lib/ui/Button.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Icon from '#lib/Icon.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';

	type Kind = 'jackett' | 'prowlarr';
	interface Bridge {
		id: number;
		kind: Kind;
		url: string;
		callback_url?: string;
		last_check?: string;
		last_error?: string;
		sites: number;
		sources: number;
		local?: boolean;
	}

	let { onchange }: { onchange?: () => void } = $props();

	const mayEdit = $derived(session.can('settings'));
	const names: Record<Kind, string> = { jackett: 'Jackett', prowlarr: 'Prowlarr' };
	const kindChoices: { value: Kind; label: string }[] = [
		{ value: 'jackett', label: 'Jackett' },
		{ value: 'prowlarr', label: 'Prowlarr' }
	];

	let bridges = $state<Bridge[] | null>(null);
	let loadError = $state('');
	let open = $state(false);
	let kind = $state<Kind>('jackett');
	let address = $state('');
	let key = $state('');
	let callback = $state('');
	let testing = $state(false);
	let connecting = $state(false);
	// A test or connect answer; a good test shows as a short tag ("Prowlarr v2.0.5 · 12 sites").
	let result = $state<{ ok: boolean; text: string; tag?: string } | null>(null);
	let errors = $state<Record<string, string>>({});
	let keysNeeded = $state<string[]>([]);
	let newKeys = $state(false);
	let typedKeys = $state<Record<string, string>>({ sonarr: '', radarr: '' });
	let checking = $state<number | null>(null);
	// Old-app keys a Prowlarr connect made, shown once (an app that used a replaced one, such as Seerr, needs it).
	let madeKeys = $state<Record<string, string> | null>(null);

	let removing = $state<Bridge | null>(null);
	let removeOpen = $state(false);
	let keepSources = $state(false);
	let removeBusy = $state(false);
	let removeError = $state('');

	async function load() {
		try {
			bridges = (await api.get<{ bridges: Bridge[] }>('/bridges')).bridges;
			loadError = '';
		} catch (e) {
			loadError = message(e);
		}
	}
	onMount(() => {
		void load();
		// How Prowlarr reaches HolyArrs: the address this page was opened at,
		// unless that is this machine's own name, which Prowlarr's container can't reach.
		if (!/^(localhost|127\.|\[::1\])/.test(location.hostname)) callback = location.origin + to('/').replace(/\/$/, '');
	});

	function startConnect() {
		open = true;
		result = null;
		errors = {};
		keysNeeded = [];
	}

	function body() {
		const keys: Record<string, string> = {};
		for (const [side, k] of Object.entries(typedKeys)) if (k.trim()) keys[side] = k.trim();
		return { kind, url: address.trim(), key: key.trim(), callback_url: kind === 'prowlarr' ? callback.trim() : '', new_keys: newKeys, keys };
	}

	function fieldError(e: unknown): boolean {
		const field = e instanceof ApiError && typeof e.body.field === 'string' ? e.body.field : '';
		if (['kind', 'url', 'key', 'callback_url'].includes(field)) {
			errors = { [field]: message(e) };
			return true;
		}
		return false;
	}

	async function test() {
		errors = {};
		result = null;
		testing = true;
		try {
			const r = await api.post<{ text: string; sites?: string[]; keys?: Record<string, string> }>('/bridges/test', body());
			let text = r.text;
			if (r.sites?.length) text += ` ${r.sites.slice(0, 8).join(', ')}${r.sites.length > 8 ? '…' : ''}.`;
			const ver = r.text.match(/\d+(?:\.\d+){1,2}/)?.[0];
			const tag = [`${names[kind]}${ver ? ` v${ver}` : ''}`, r.sites?.length ? `${r.sites.length} ${r.sites.length === 1 ? 'site' : 'sites'}` : ''].filter(Boolean).join(' · ');
			const have = Object.entries(r.keys ?? {}).filter(([, v]) => v === 'have').map(([k]) => k);
			keysNeeded = have;
			result = { ok: true, text, tag };
		} catch (e) {
			if (!fieldError(e)) result = { ok: false, text: message(e) };
		} finally {
			testing = false;
		}
	}

	async function connect() {
		errors = {};
		result = null;
		connecting = true;
		try {
			const b = await api.post<Bridge & { keys?: Record<string, string> }>('/bridges', body());
			madeKeys = b.keys && Object.keys(b.keys).length ? b.keys : null;
			toasts.push(b.kind === 'jackett' ? `Jackett connected: ${b.sources} ${b.sources === 1 ? 'Source' : 'Sources'} added.` : 'Prowlarr connected. It sends its sites in now.', 'ok');
			open = false;
			key = '';
			typedKeys = { sonarr: '', radarr: '' };
			newKeys = false;
			keysNeeded = [];
			await load();
			onchange?.();
		} catch (e) {
			if (e instanceof ApiError && e.status === 409 && e.body.need === 'keys' && Array.isArray(e.body.sides)) {
				keysNeeded = e.body.sides as string[];
				result = { ok: false, text: message(e) };
			} else if (!fieldError(e)) result = { ok: false, text: message(e) };
		} finally {
			connecting = false;
		}
	}

	async function check(b: Bridge) {
		checking = b.id;
		try {
			const r = await api.post<{ bridge: Bridge; result?: { added: number; removed: number; updated: number } }>(`/bridges/${b.id}/check`, {});
			const res = r.result;
			toasts.push(res ? (res.added + res.removed + res.updated ? `In step: ${res.added} added, ${res.removed} removed, ${res.updated} changed.` : 'Already in step.') : 'Prowlarr is sending its sites again.', 'ok');
			onchange?.();
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			checking = null;
			await load();
		}
	}

	function askRemove(b: Bridge) {
		removing = b;
		keepSources = false;
		removeError = '';
		removeOpen = true;
	}

	async function remove(e: SubmitEvent) {
		e.preventDefault();
		if (!removing) return;
		removeBusy = true;
		removeError = '';
		try {
			await api.del(`/bridges/${removing.id}${keepSources ? '?keep_sources=true' : ''}`);
			toasts.push(`${names[removing.kind]} disconnected.`, 'ok');
			removeOpen = false;
			await load();
			onchange?.();
		} catch (err) {
			removeError = message(err);
		} finally {
			removeBusy = false;
		}
	}

	const stateLine = (b: Bridge) =>
		b.kind === 'jackett'
			? `${b.sites} ${b.sites === 1 ? 'site' : 'sites'}, ${b.sources} ${b.sources === 1 ? 'Source' : 'Sources'}`
			: `${b.sources} ${b.sources === 1 ? 'Source' : 'Sources'}`;
</script>

<SettingsCard
	id="connect-bridge"
	title="Jackett or Prowlarr"
	description="Its sites become your Sources."
>
	{#if loadError && !bridges}
		<LoadError hasData={false} message={loadError} onretry={load} />
	{:else if bridges}
		{#if bridges.length}
			{#if madeKeys}
				<div class="made" role="status" data-testid="bridge-made-keys">
					<p class="small"><b>New old-app keys, shown only now.</b> Prowlarr has them already. An app that used an old one, such as Seerr, needs the new one:</p>
					{#each Object.entries(madeKeys) as [side, k] (side)}
						<p class="small">{side === 'sonarr' ? 'TV shows (Sonarr side)' : 'Movies (Radarr side)'}: <code class="mono">{k}</code></p>
					{/each}
					<div><Button type="button" variant="plain" onclick={() => (madeKeys = null)}>Done</Button></div>
				</div>
			{/if}
			<ul class="bridges" data-testid="bridges">
				{#each bridges as b (b.id)}
					<li data-testid="bridge">
						<div class="grow">
							<b>{names[b.kind]}</b> <span class="mono xs muted">{b.url}</span>
							<div class="small">{stateLine(b)}{#if b.last_check} · checked {ago(b.last_check)}{/if}</div>
							{#if b.last_error}<div class="small bad-text" data-testid="bridge-error">{b.last_error}</div>{/if}
							{#if b.local}<div class="small muted" data-testid="bridge-local">Can reach your home network</div>{/if}
							{#if b.kind === 'jackett'}<div class="small muted">Removed Sources stay removed until you connect it again</div>{/if}
						</div>
						{#if mayEdit}
							<div class="acts">
								<Button type="button" variant="plain" busy={checking === b.id} onclick={() => check(b)} data-testid="bridge-check">Check now</Button>
								<Button type="button" variant="plain" onclick={() => askRemove(b)} data-testid="bridge-remove">Disconnect…</Button>
							</div>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
		{#if mayEdit && !open}
			<div><Button type="button" icon="plus" onclick={startConnect} data-testid="bridge-start">{bridges.length ? 'Connect another' : 'Connect Jackett or Prowlarr'}</Button></div>
		{/if}
		{#if open}
			<div class="form" data-testid="bridge-form">
				<SegmentedControl options={kindChoices} bind:value={kind} label="App" testid="br-kind" />
				<Field id="br-url" label="Address" hint={kind === 'jackett' ? 'Add its base path, if it has one.' : undefined} error={errors.url}>
					<input id="br-url" class="input mono" type="url" inputmode="url" autocomplete="off" spellcheck="false" placeholder={kind === 'jackett' ? 'http://jackett:9117' : 'http://prowlarr:9696'} bind:value={address} aria-invalid={errors.url ? 'true' : undefined} aria-describedby={errors.url ? 'br-url-error' : kind === 'jackett' ? 'br-url-hint' : undefined} />
				</Field>
				<Field id="br-key" label="Key" hint={kind === 'jackett' ? 'On Jackett’s main page' : 'Prowlarr › Settings › General'} error={errors.key}>
					<input id="br-key" class="input mono" type="password" autocomplete="new-password" spellcheck="false" bind:value={key} aria-invalid={errors.key ? 'true' : undefined} aria-describedby={errors.key ? 'br-key-error' : 'br-key-hint'} />
				</Field>
				{#if kind === 'prowlarr'}
					<Field id="br-callback" label="HolyArrs address" hint="Prowlarr must be able to reach it." error={errors.callback_url}>
						<input id="br-callback" class="input mono" type="url" autocomplete="off" spellcheck="false" bind:value={callback} aria-invalid={errors.callback_url ? 'true' : undefined} aria-describedby={errors.callback_url ? 'br-callback-error' : 'br-callback-hint'} />
					</Field>
					{#if keysNeeded.length}
						<div class="keys" data-testid="bridge-keys">
							<label class="check"><input type="checkbox" bind:checked={newKeys} data-testid="bridge-new-keys" /> <span>Make new old-app keys. An app using the old ones, such as Seerr, must get the new ones too.</span></label>
							{#if !newKeys}
								<p class="hint">Or type the key you have:</p>
								{#each keysNeeded as side (side)}
									<Field id="br-key-{side}" label={side === 'sonarr' ? 'TV shows key (Sonarr side)' : 'Movies key (Radarr side)'}>
										<input id="br-key-{side}" class="input mono" type="password" autocomplete="new-password" spellcheck="false" bind:value={typedKeys[side]} />
									</Field>
								{/each}
							{/if}
						</div>
					{/if}
				{/if}
				{#if result && !result.tag}<p class="small" class:bad-text={!result.ok} class:ok-text={result.ok} role="status" data-testid="bridge-result">{result.text}</p>{/if}
				<div class="acts">
					{#if result?.tag}<span class="tag" role="status" data-testid="bridge-result" title={result.text}><Icon name="check" size={12} stroke={2.5} />{result.tag}</span>{/if}
					<Button type="button" variant="plain" onclick={() => (open = false)}>Cancel</Button>
					<Button type="button" busy={testing} disabled={!address.trim() || !key.trim()} onclick={test} data-testid="bridge-test">Test</Button>
					<Button type="button" variant="primary" busy={connecting} disabled={!address.trim() || !key.trim()} onclick={connect} data-testid="bridge-connect">Connect</Button>
				</div>
			</div>
		{/if}
	{/if}
</SettingsCard>

<Sheet bind:open={removeOpen} title="Disconnect {removing ? names[removing.kind] : ''}?" description={removing?.url} size="sm">
	<form id="remove-bridge" class="form" onsubmit={remove}>
		{#if removing?.kind === 'jackett'}
			<label class="check"><input type="radio" name="keep" checked={!keepSources} onchange={() => (keepSources = false)} /> <span>Remove its {removing.sources} {removing.sources === 1 ? 'Source' : 'Sources'}</span></label>
			<label class="check"><input type="radio" name="keep" checked={keepSources} onchange={() => (keepSources = true)} /> <span>Keep them as ordinary Sources (they no longer follow Jackett)</span></label>
		{:else if removing}
			<p class="small">HolyArrs leaves Prowlarr's apps and removes the {removing.sources} {removing.sources === 1 ? 'Source' : 'Sources'} Prowlarr wrote.</p>
			<label class="check"><input type="checkbox" bind:checked={keepSources} /> <span>Only forget it here: HolyArrs stays in Prowlarr's apps and keeps its Sources</span></label>
		{/if}
		{#if removeError}<p class="form-error" role="alert">{removeError}</p>{/if}
	</form>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (removeOpen = false)}>Cancel</Button>
		<Button variant="destructive" type="submit" form="remove-bridge" busy={removeBusy}>Disconnect</Button>
	{/snippet}
</Sheet>

<style>
	.form { display: flex; flex-direction: column; gap: 16px; }
	.bridges { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 12px; }
	.bridges > li { display: flex; gap: 12px; align-items: flex-start; flex-wrap: wrap; }
	.grow { flex: 1 1 16rem; min-width: 0; overflow-wrap: anywhere; }
	.acts { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; justify-content: flex-end; }
	.tag { margin-right: auto; display: inline-flex; align-items: center; gap: 4px; padding: 4px 8px; border-radius: 999px; font: 500 12px/1.2 var(--font-ui); color: var(--ok-text, #30d158); border: 1px solid color-mix(in srgb, var(--ok-text, #30d158) 40%, transparent); }
	.keys { display: flex; flex-direction: column; gap: 12px; }
	.made { display: flex; flex-direction: column; gap: 4px; padding: 12px; border-radius: 12px; background: var(--surface-2); overflow-wrap: anywhere; }
	.check { display: flex; gap: 8px; align-items: flex-start; font: var(--t-callout); }
	.check input { margin-top: 3px; flex: none; }
</style>
