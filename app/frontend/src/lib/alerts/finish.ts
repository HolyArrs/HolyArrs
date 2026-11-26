// What an Alert still needs before it can send, and the form body that changes it. The Alerts page and the import's Finish your alerts card share these.
import type { AlertView } from '#lib/core/types.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

/** The one thing an Alert still waits for (none for an Alert that can send). */
export type Missing = '' | 'bot' | 'botAndChat' | 'chat' | 'topic' | 'app' | 'server' | 'serverAndApp' | 'userAndApp' | 'user' | 'link' | 'address';

/** The fields a missing part is typed into, in the order they show. */
export const missingFields: Record<Exclude<Missing, ''>, ('bot_token' | 'chat_id' | 'topic' | 'app_token' | 'server_url' | 'user_key' | 'url')[]> = {
	bot: ['bot_token'],
	botAndChat: ['bot_token', 'chat_id'],
	chat: ['chat_id'],
	topic: ['topic'],
	app: ['app_token'],
	server: ['server_url'],
	serverAndApp: ['server_url', 'app_token'],
	userAndApp: ['user_key', 'app_token'],
	user: ['user_key'],
	link: ['url'],
	address: ['url']
};

/** What a's kind needs that it does not have yet (email never counts: a mail server may need no password). */
export function alertMissing(a: AlertView): Missing {
	switch (a.kind) {
		case 'telegram':
			return a.bot_token ? (a.chat_id ? '' : 'chat') : a.chat_id ? 'bot' : 'botAndChat';
		case 'ntfy':
			return a.topic ? '' : 'topic';
		case 'gotify':
			return a.server_url ? (a.app_token ? '' : 'app') : a.app_token ? 'server' : 'serverAndApp';
		case 'pushover':
			return a.app_token ? (a.user_key ? '' : 'user') : a.user_key ? 'app' : 'userAndApp';
		case 'discord':
			return a.url ? '' : 'link';
		case 'webhook':
			return a.url ? '' : 'address';
	}
	return '';
}

/** The change form for a, as the Alerts page fills it: an empty secret keeps the stored one. */
export function alertForm(a: AlertView) {
	return {
		name: a.name,
		kind: a.kind,
		enabled: a.enabled,
		moments: [...a.moments],
		url: a.url ?? '',
		smtp_host: a.smtp_host ?? '',
		smtp_port: a.smtp_port ?? 587,
		smtp_security: a.smtp_security ?? ('starttls' as const),
		smtp_from: a.smtp_from ?? '',
		smtp_user: a.smtp_user ?? '',
		smtp_pass: '',
		staff_to: a.staff_to ?? '',
		secret: '',
		webhook_auth: a.webhook_auth ?? ('sign' as const),
		bot_token: '',
		chat_id: a.chat_id ?? '',
		server_url: a.server_url ?? '',
		topic: a.topic ?? '',
		user_key: a.user_key ?? '',
		app_token: '',
		clear_app_token: false,
		allow_local_network: a.allow_local_network ?? false
	};
}

/** What is missing, as words that follow "Still needs". */
export function missingWords(m: Exclude<Missing, ''>): string {
	const words: Record<Exclude<Missing, ''>, () => string> = {
		bot: () => t('settings.alerts.needBot'),
		botAndChat: () => t('settings.alerts.needBotAndChat'),
		chat: () => t('settings.alerts.needChat'),
		topic: () => t('settings.alerts.needTopic'),
		app: () => t('settings.alerts.needApp'),
		server: () => t('settings.alerts.needServer'),
		serverAndApp: () => t('settings.alerts.needServerAndApp'),
		userAndApp: () => t('settings.alerts.needUserAndApp'),
		user: () => t('settings.alerts.needUser'),
		link: () => t('settings.alerts.needLink'),
		address: () => t('settings.alerts.needAddress')
	};
	return words[m]();
}
