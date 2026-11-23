<script lang="ts">
	// Finish your alerts: after an import, each Alert that came over off and still waits for a token gets its field here, a test and a way to turn it on.
	import { onMount } from 'svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Field from '#lib/ui/Field.svelte';
	import type { AlertView } from '#lib/core/types.ts';
	import { alertForm, alertMissing, missingFields, missingWords } from './finish.ts';

	type Typed = Record<string, string>;
	let waiting = $state<AlertView[]>([]);
	let typed = $state<Record<number, Typed>>({});
	let busy = $state(0);
	let result = $state<Record<number, { ok: boolean; text: string }>>({});

	async function load() {
		try {
			const all = (await api.get<{ alerts: AlertView[] }>('/alerts')).alerts ?? [];
			// One that was turned on here stays in the list, so its "is on" line shows.
			waiting = all.filter((a) => (!a.enabled && alertMissing(a)) || waiting.some((w) => w.id === a.id));
		} catch {
			waiting = [];
		}
	}
	onMount(load);

	const secret = new Set(['bot_token', 'app_token']);
	const label: Record<string, () => string> = {
		bot_token: () => t('settings.alerts.botTokenLabel'),
		chat_id: () => t('settings.alerts.chatId'),
		topic: () => t('settings.alerts.topic'),
		app_token: () => t('settings.alerts.appTokenLabel'),
		server_url: () => t('settings.alerts.serverAddress'),
		user_key: () => t('settings.alerts.userKey'),
		url: () => t('settings.alerts.discordLink')
	};

	function fieldsOf(a: AlertView) {
		const m = alertMissing(a);
		return m ? missingFields[m] : [];
	}

	// ntfy also shows its server (kept from the import, or empty when the old one was refused), so a topic never goes to ntfy.sh unseen.
	function shownOf(a: AlertView) {
		const f = fieldsOf(a);
		return a.kind === 'ntfy' && f.length && !f.includes('server_url') ? (['server_url', ...f] as typeof f) : f;
	}

	function valueOf(a: AlertView, f: string) {
		return typed[a.id]?.[f] ?? (f === 'server_url' ? (a.server_url ?? '') : '');
	}

	// Every field the Alert waits for is typed.
	function ready(a: AlertView) {
		return fieldsOf(a).every((f) => valueOf(a, f).trim() !== '');
	}

	async function save(a: AlertView, on: boolean) {
		const body = { ...alertForm(a), ...(typed[a.id] ?? {}), enabled: on };
		await api.put(`/alerts/${a.id}`, { ...body, smtp_port: Number(body.smtp_port) });
	}

	async function act(a: AlertView, on: boolean) {
		if (!ready(a)) {
			result = { ...result, [a.id]: { ok: false, text: t('settings.alerts.typeFirst') } };
			return;
		}
		busy = a.id;
		try {
			await save(a, on || a.enabled);
			if (on) {
				result = { ...result, [a.id]: { ok: true, text: t('settings.alerts.turnedOn', { name: a.name }) } };
			} else {
				await api.post(`/alerts/${a.id}/test`);
				result = { ...result, [a.id]: { ok: true, text: t('settings.alerts.testSent', { name: a.name }) } };
			}
			typed = { ...typed, [a.id]: {} };
			await load();
		} catch (e) {
			result = { ...result, [a.id]: { ok: false, text: message(e) } };
		} finally {
			busy = 0;
		}
	}

	function set(a: AlertView, f: string, v: string) {
		typed = { ...typed, [a.id]: { ...(typed[a.id] ?? {}), [f]: v } };
	}
</script>

{#if waiting.length}
	<SettingsCard id="finish-alerts" title={t('settings.alerts.finishTitle')} description={t('settings.alerts.finishLead')}>
		<ul class="fa" data-testid="finish-alerts">
			{#each waiting as a (a.id)}
				{@const m = alertMissing(a)}
				<li class="fa-row" data-testid="finish-alert">
					<p class="fa-head"><b>{a.name}</b>{#if m}<span class="fa-sub">{t('settings.alerts.stillNeeds', { what: missingWords(m) })}</span>{/if}</p>
					{#each shownOf(a) as f (f)}
						<Field id="fa-{a.id}-{f}" label={f === 'url' && a.kind === 'webhook' ? t('settings.alerts.hookLink') : label[f]()}>
							<input id="fa-{a.id}-{f}" class="input mono" type={secret.has(f) ? 'password' : 'text'} autocomplete={secret.has(f) ? 'new-password' : 'off'} spellcheck="false"
								placeholder={f === 'server_url' && a.kind === 'ntfy' ? 'https://ntfy.sh' : undefined} value={valueOf(a, f)} oninput={(e) => set(a, f, e.currentTarget.value)} />
						</Field>
					{/each}
					{#if result[a.id]}<p class="small" class:bad={!result[a.id].ok} role="status" data-testid="finish-result">{result[a.id].text}</p>{/if}
					{#if m || !a.enabled}
						<div class="row fa-acts">
							<Button type="button" disabled={busy === a.id} aria-busy={busy === a.id} onclick={() => act(a, false)}>{t('settings.alerts.sendTest')}</Button>
							<Button variant="primary" type="button" disabled={busy === a.id} onclick={() => act(a, true)}>{t('settings.alerts.turnOn')}</Button>
						</div>
					{/if}
				</li>
			{/each}
		</ul>
	</SettingsCard>
{/if}

<style>
	.fa { list-style: none; margin: 0; padding: 0; display: grid; gap: 10px; }
	.fa-row { padding: 14px 16px; border-radius: 16px; background: var(--surface-2); border: 1px solid var(--line); }
	.fa-head { margin: 0; display: flex; flex-wrap: wrap; gap: 4px 10px; font: var(--t-callout); color: var(--text); }
	.fa-sub { color: var(--text-muted); font: var(--t-footnote); align-self: center; }
	.fa-acts { margin-top: var(--s-3); flex-wrap: wrap; }
	.bad { color: var(--fail-text); }
</style>
