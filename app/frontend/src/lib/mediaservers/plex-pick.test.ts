import { describe, expect, it } from 'vitest';
import { firstPlexRow, plexAddresses, type PlexServerRow } from './plex-pick.ts';

const row = (p: Partial<PlexServerRow>): PlexServerRow => ({ id: 'a', name: 'Home', address: '10.0.0.5', port: 32400, local: true, url: 'http://10.0.0.5:32400', ...p });

describe('plexAddresses', () => {
	it.each([
		['home with both: secure first', row({ secure_url: 'https://10-0-0-5.x.plex.direct:32400' }), ['https://10-0-0-5.x.plex.direct:32400', 'http://10.0.0.5:32400']],
		['home, plain only', row({}), ['http://10.0.0.5:32400']],
		['remote: secure only', row({ local: false, url: 'https://r.plex.direct:32400', secure_url: 'https://r.plex.direct:32400' }), ['https://r.plex.direct:32400']],
		['relay: its one address', row({ local: false, relay: true, url: 'https://relay.plex.direct:8443', secure_url: 'https://relay.plex.direct:8443' }), ['https://relay.plex.direct:8443']]
	])('%s', (_, c, want) => expect(plexAddresses(c)).toEqual(want));
});

describe('firstPlexRow', () => {
	it.each([
		['none', [], -1],
		['home before remote', [row({ local: false }), row({})], 1],
		['remote before relay', [row({ local: false, relay: true }), row({ local: false })], 1],
		['relay when alone', [row({ local: false, relay: true })], 0]
	])('%s', (_, list, want) => expect(firstPlexRow(list as PlexServerRow[])).toBe(want));
});
