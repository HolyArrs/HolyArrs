<!-- My streaming services: the country, the services picked there, and whether requests on them skip the download. -->
<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { defaultRegion, serviceRegion } from '#lib/ui/region.ts';
	import Field from '#lib/ui/Field.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { countryOptions, toggleService, MAX_MY_SERVICES } from './streaming-settings.ts';

	type Service = { id: number; name: string; logo?: string };
	let { region = $bindable(''), services = $bindable([]), skip = $bindable(false) }: { region?: string; services?: number[]; skip?: boolean } = $props();

	let list = $state<Service[]>([]);
	let error = $state('');
	let loading = $state(false);
	let shown = $state('');
	const countries = $derived(countryOptions(region));
	const picked = $derived(new Set(services));
	// The picked services first, in their order, so a long list still shows what is on.
	const ordered = $derived([...list.filter((s) => picked.has(s.id)), ...list.filter((s) => !picked.has(s.id))]);

	$effect(() => {
		const r = region;
		if (/^[A-Z]{2}$/.test(r) && r !== shown) void load(r);
	});

	async function load(r: string) {
		shown = r;
		loading = true;
		error = '';
		try {
			const got = (await api.get<{ services: Service[] }>(`/watch-services?region=${r}`)).services ?? [];
			if (shown === r) list = got;
		} catch (e) {
			if (shown === r) error = message(e);
		} finally {
			if (shown === r) loading = false;
		}
	}

	function pickCountry(code: string) {
		if (code !== region) services = [];
		region = code;
	}
</script>

<Field id="streaming-country" label={t('settings.requests.streamingCountry')}>
	<select id="streaming-country" class="input" value={region} onchange={(e) => pickCountry(e.currentTarget.value)} data-testid="streaming-country">
		<option value="">{t('settings.requests.streamingCountryNone')}</option>
		{#each countries as c (c.code)}
			<option value={c.code}>{c.name}</option>
		{/each}
	</select>
</Field>
{#if !region}
	<div><Button variant="plain" type="button" onclick={() => pickCountry(serviceRegion(defaultRegion()))}>{t('settings.requests.streamingUseMine')}</Button></div>
{:else if error}
	<p class="form-error" role="alert">{error}</p>
{:else if loading && !list.length}
	<p class="hint">{t('settings.requests.streamingLoading')}</p>
{:else if list.length}
	<fieldset class="services" data-testid="streaming-services">
		<legend class="hint">{t('settings.requests.streamingPicked', { n: services.length, max: MAX_MY_SERVICES })}</legend>
		<ul>
			{#each ordered as s (s.id)}
				<li>
					<label class="svc">
						<input type="checkbox" checked={picked.has(s.id)} disabled={!picked.has(s.id) && services.length >= MAX_MY_SERVICES} onchange={(e) => (services = toggleService(services, s.id, e.currentTarget.checked))} />
						{#if s.logo}<img src={s.logo} alt="" width="24" height="24" loading="lazy" />{/if}
						<span>{s.name}</span>
					</label>
				</li>
			{/each}
		</ul>
	</fieldset>
{:else}
	<p class="hint">{t('settings.requests.streamingNone')}</p>
{/if}
<label class="checkline">
	<input type="checkbox" bind:checked={skip} disabled={!region || !services.length} data-testid="skip-on-services" />
	<span><b>{t('settings.requests.streamingSkip')}</b><br /><span class="hint">{t('settings.requests.streamingSkipHint')}</span></span>
</label>

<style>
	.services { border: 0; margin: 0; padding: 0; min-width: 0; }
	ul { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 200px), 1fr)); gap: 0 var(--s-3); max-height: 320px; overflow-y: auto; }
	.svc { display: flex; align-items: center; gap: var(--s-2); min-height: 44px; min-width: 0; font: var(--t-callout); }
	.svc span { overflow-wrap: anywhere; }
	img { border-radius: 6px; flex: none; }
</style>
