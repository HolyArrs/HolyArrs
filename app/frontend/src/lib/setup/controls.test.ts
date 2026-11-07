import { describe, expect, it } from 'vitest';
import { createClient } from '../core/api.ts';
import { applyParts, isConfirm, linkTarget } from './controls.ts';
import type { PartName, PartsAnswer } from '../mediafiles/parts.ts';

function answer(on: Record<PartName, boolean>): PartsAnswer {
	return {
		parts: (Object.keys(on) as PartName[]).map((p) => ({ part: p, name: p, on: on[p], state: on[p] ? 'running' : 'off' })),
		screens: {},
		safe_mode: false
	};
}

describe('setup controls', () => {
	it('a link key points to its app, anything else to nothing', () => {
		expect(linkTarget('link:downloader:3')).toEqual({ kind: 'downloader', id: '3' });
		expect(linkTarget('link:subtitle_site:podnapisi')).toEqual({ kind: 'subtitle_site', id: 'podnapisi' });
		expect(linkTarget('stage:search')).toBeNull();
		expect(linkTarget('link:other:1')).toBeNull();
		expect(isConfirm({ confirm: 'x', text: 't' })).toBe(true);
		expect(isConfirm({ parts: [] })).toBe(false);
	});

	it('the user saves parts / they switch in order and turn-offs are confirmed at once', async () => {
		const on: Record<PartName, boolean> = { movies: true, shows: true, music: true, subtitles: true, wishlist: true };
		const calls: string[] = [];
		const c = createClient(async (url: string, init?: RequestInit) => {
			const path = url.replace('/api/v1', '');
			const body = init?.body ? JSON.parse(String(init.body)) : {};
			calls.push(`${init?.method ?? 'GET'} ${path}${body.confirm ? ' confirmed' : ''}`);
			const m = /^\/parts\/(\w+)\/(on|off)$/.exec(path);
			if (m && m[2] === 'off' && !body.confirm) return new Response(JSON.stringify({ confirm: 'id-' + m[1], text: 'sure?' }), { status: 202 });
			if (m) on[m[1] as PartName] = m[2] === 'on';
			return new Response(JSON.stringify(answer(on)), { status: 200 });
		});
		const got = await applyParts({ movies: true, shows: false, music: false, subtitles: false, wishlist: true }, c);
		expect(calls).toEqual([
			'GET /parts',
			'POST /parts/subtitles/off',
			'POST /parts/subtitles/off confirmed',
			'POST /parts/music/off',
			'POST /parts/music/off confirmed',
			'POST /parts/shows/off',
			'POST /parts/shows/off confirmed'
		]);
		expect(got.parts.filter((p) => p.on).map((p) => p.part)).toEqual(['movies', 'wishlist']);
	});
});
