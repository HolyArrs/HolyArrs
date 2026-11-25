import { describe, expect, it } from 'vitest';
import { bridgeCall, cardLink, playerCall } from './plugin-cards.ts';

describe('plugin cards', () => {
	it.each([
		['/library/movies', '/library/movies'],
		['/title/movie/12', '/title/movie/12'],
		['https://evil.example', undefined],
		['//evil.example', undefined],
		['javascript:alert(1)', undefined],
		['/../settings', undefined],
		['/api/v1/plugin-pages/x/call', undefined],
		['/api', undefined],
		['/x" onclick="y', undefined],
		[undefined, undefined]
	])('link %s', (link, want) => {
		expect(cardLink(link)).toBe(want);
	});
});

describe('plugin page bridge', () => {
	const frame = {};
	const msg = (data: unknown, source: unknown = frame, origin = 'null') => ({ data, source, origin });
	it.each([
		['a good call', msg({ id: 1, method: 'GET', path: '/api/v1/titles' }), true],
		['from another window', msg({ id: 1, method: 'GET', path: '/api/v1/titles' }, {}), false],
		['from a real origin', msg({ id: 1, method: 'GET', path: '/api/v1/titles' }, frame, 'https://evil.example'), false],
		['outside the API', msg({ id: 1, method: 'GET', path: '/settings' }), false],
		['climbing', msg({ id: 1, method: 'GET', path: '/api/v1/../auth/me' }), false],
		['odd method', msg({ id: 1, method: 'PATCH', path: '/api/v1/titles' }), false],
		['no id', msg({ method: 'GET', path: '/api/v1/titles' }), false],
		['a stream link', msg({ id: 1, method: 'POST', path: '/api/v1/plugin-hooks/stream', body: { file_id: 1 } }), false],
		['a stream address', msg({ id: 1, method: 'GET', path: '/api/v1/plugin-stream/hms_x' }), false],
		['the player route', msg({ id: 1, method: 'POST', path: '/api/v1/plugin-pages/p/play', body: { file_id: 1 } }), false],
		['not an object', msg('GET /api/v1/titles'), false]
	])('%s', (_, e, ok) => {
		expect(bridgeCall(e, frame) !== null).toBe(ok);
	});
});

describe('plugin page player messages', () => {
	const frame = {};
	const msg = (data: unknown, source: unknown = frame, origin = 'null') => ({ data, source, origin });
	it.each([
		['play a file', msg({ id: 1, player: 'play', file_id: 7 }), { id: 1, op: 'play', file_id: 7 }],
		['seek', msg({ id: 2, player: 'seek', time: 12.5 }), { id: 2, op: 'seek', time: 12.5 }],
		['pause', msg({ id: 3, player: 'pause' }), { id: 3, op: 'pause' }],
		['close', msg({ id: 4, player: 'close' }), { id: 4, op: 'close' }],
		['play with no file', msg({ id: 1, player: 'play' }), null],
		['play with a path', msg({ id: 1, player: 'play', file_id: '/etc/passwd' }), null],
		['seek backwards past zero', msg({ id: 1, player: 'seek', time: -1 }), null],
		['an unknown op', msg({ id: 1, player: 'download', file_id: 1 }), null],
		['from another window', msg({ id: 1, player: 'pause' }, {}), null],
		['from a real origin', msg({ id: 1, player: 'pause' }, frame, 'https://evil.example'), null],
		['no id', msg({ player: 'pause' }), null]
	])('%s', (_, e, want) => {
		expect(playerCall(e, frame)).toEqual(want);
	});
});
