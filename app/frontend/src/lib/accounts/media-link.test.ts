import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { accountChoice } from './media-link.ts';

const mine = readFileSync(new URL('./MediaServerLink.svelte', import.meta.url), 'utf8');
const t = (key: string, vars?: Record<string, string | number>) => key + (vars ? ` ${vars.name}` : '');

describe('My account: Link my Jellyfin or Emby account', () => {
	it('puts the server buttons under the words on a phone', () => {
		expect(mine).toMatch(/@media \(max-width: 600px\)[^}]*\.srow[^}]*grid-template-columns: minmax\(0, 1fr\)/);
	});
	it('says Change on a server the person already linked', () => {
		expect(mine).toMatch(/linkOf\(s\.kind\) \? t\('accounts\.account\.change'\) : t\('accounts\.security\.link'\)/);
	});
});

describe('accountChoice', () => {
	it('offers a free account and the one this person already has', () => {
		expect(accountChoice({ id: 'a', name: 'Mum' }, 4, t)).toEqual({ label: 'Mum', taken: false });
		expect(accountChoice({ id: 'a', name: 'Mum', taken_by: 4, taken_name: 'ana' }, 4, t)).toEqual({ label: 'Mum', taken: false });
	});
	it('marks an account linked to someone else as taken, with their name', () => {
		expect(accountChoice({ id: 'k', name: 'Kid', taken_by: 9, taken_name: 'leo' }, 4, t)).toEqual({ label: 'Kid · settings.users.accountLinked leo', taken: true });
	});
});
