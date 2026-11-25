import { afterEach, describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { alertForm, alertMissing } from './finish.ts';
import { lang } from '#lib/core/i18n/i18n.svelte.ts';
import { serverWords } from '#lib/core/i18n/server-words.ts';
import en from '#lib/core/i18n/messages/en/server-lines.json';
import type { AlertView } from '#lib/core/types.ts';

const base: AlertView = { id: 1, name: 'Phone', kind: 'telegram', enabled: false, moments: ['grab'] };
const page = readFileSync(new URL('../../routes/settings/alerts/+page.svelte', import.meta.url), 'utf8');
const done = readFileSync(new URL('../migrate/DoneStep.svelte', import.meta.url), 'utf8');
const start = { value: lang.value, words: lang.words };
afterEach(() => {
	lang.value = start.value;
	lang.words = start.words;
});

describe('alertMissing', () => {
	it('names what an imported Alert still waits for', () => {
		const cases: [Partial<AlertView>, string][] = [
			[{ kind: 'telegram' }, 'botAndChat'],
			[{ kind: 'telegram', chat_id: '@news' }, 'bot'],
			[{ kind: 'telegram', chat_id: '@news', bot_token: '••••' }, ''],
			[{ kind: 'ntfy', server_url: 'https://ntfy.example.com' }, 'topic'],
			[{ kind: 'ntfy', topic: 'tv' }, ''],
			[{ kind: 'gotify', server_url: 'https://gotify.example.com' }, 'app'],
			[{ kind: 'gotify' }, 'serverAndApp'],
			[{ kind: 'pushover' }, 'userAndApp'],
			[{ kind: 'pushover', user_key: 'u' }, 'app'],
			[{ kind: 'discord' }, 'link'],
			[{ kind: 'webhook', url: '••••' }, ''],
			[{ kind: 'email' }, '']
		];
		for (const [a, want] of cases) expect(alertMissing({ ...base, ...a } as AlertView), JSON.stringify(a)).toBe(want);
	});

	it('changes an Alert with every secret left empty, so stored ones stay', () => {
		const f = alertForm({ ...base, kind: 'gotify', server_url: 'https://gotify.example.com', app_token: '••••' });
		expect(f.app_token).toBe('');
		expect(f.bot_token).toBe('');
		expect(f.server_url).toBe('https://gotify.example.com');
		expect(f.moments).toEqual(['grab']);
	});
});

describe('the Alerts page and the import', () => {
	it('asks for a token instead of "Stored" when none is kept, and tests only once one is', () => {
		expect(page).toContain("editing?.bot_token ? t('settings.alerts.stored') : t('settings.alerts.pasteBot')");
		expect(page).toContain("t('settings.alerts.typeFirst')");
		expect(page).toContain('data-testid="alert-needs"');
	});

	it('shows Finish your Alerts after an import that made Alerts', () => {
		expect(done).toContain('<FinishAlerts />');
	});

	it('shows the settings an Alert left behind in the picked language, line by line', async () => {
		const de = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('../core/i18n/messages/de/server-lines.json', { eager: true })).map((f) => f.default));
		lang.value = 'de';
		lang.words = { ...(en as Record<string, string>), ...de };
		expect(serverWords('HolyArrs has no place for: the Telegram topic; sending silently')).toBe('HolyArrs hat keinen Platz für: das Telegram-Thema; stilles Senden');
		expect(serverWords('HolyArrs has no place for: the user name bob (HolyArrs signs in to ntfy with an access token)')).toBe(
			'HolyArrs hat keinen Platz für: den Benutzernamen bob (HolyArrs meldet sich bei ntfy mit einem Zugriffstoken an)'
		);
	});
});
