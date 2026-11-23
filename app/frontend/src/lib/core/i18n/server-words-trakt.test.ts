import { afterEach, describe, expect, it } from 'vitest';
import en from './messages/en/server-lines.json';
import de from './messages/de/server-lines.json';
import { lang } from './i18n.svelte.ts';
import { serverWords } from './server-words.ts';

const start = { value: lang.value, words: lang.words };
afterEach(() => {
	lang.value = start.value;
	lang.words = start.words;
});

describe('Trakt sign-in without a client secret', () => {
	it('shows the refusal in the picked language', () => {
		lang.value = 'de';
		lang.words = { ...(en as Record<string, string>), ...(de as Record<string, string>) };
		expect(serverWords("Add your Trakt app's client secret in Settings first; Trakt needs it to keep you signed in.")).toBe(
			(de as Record<string, string>)['server.metadata.add-your-trakt-app-s-client']
		);
	});
});
