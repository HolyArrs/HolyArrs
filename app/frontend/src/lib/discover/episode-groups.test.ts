import { describe, expect, it } from 'vitest';
import { orderLabel, orderMenuItems, orderBody, splitOutside, type EpisodeGroup } from './episode-groups.ts';

const orders: [string, string][] = [['aired', 'As aired'], ['daily', 'By date'], ['absolute', 'Absolute']];
const words = { numbering: 'Numbering', fromTmdb: 'Orders from TMDB for this show', unavailable: 'TMDB order (unavailable)', counts: (g: EpisodeGroup) => `${g.seasons} seasons · ${g.episodes} episodes` };
const disney: EpisodeGroup = { id: 'a'.repeat(24), name: 'Disney+ order', type: 4, seasons: 19, episodes: 410 };

describe('orderMenuItems', () => {
	it('hides the TMDB section when the show has no groups', () => {
		const items = orderMenuItems({ orders, order: 'aired', chosen: null, groups: [], words, pick: () => {} });
		expect(items.map((i) => ('label' in i ? i.label : i.kind))).toEqual(['As aired', 'By date', 'Absolute']);
		expect(items[0]).toMatchObject({ checked: true });
	});
	it('lists each group with its counts, checked when chosen, and unchecks the numbering', () => {
		const picked: unknown[] = [];
		const items = orderMenuItems({ orders, order: 'aired', chosen: disney.id, groups: [disney], words, pick: (b) => picked.push(b) });
		expect(items.map((i) => ('label' in i ? i.label : i.kind))).toEqual(['Numbering', 'As aired', 'By date', 'Absolute', 'separator', 'Orders from TMDB for this show', 'Disney+ order']);
		expect(items[1]).toMatchObject({ checked: false });
		expect(items[6]).toMatchObject({ checked: true, sub: '19 seasons · 410 episodes' });
		(items[2] as { onselect: () => void }).onselect();
		(items[6] as { onselect: () => void }).onselect();
		expect(picked).toEqual([{ episode_group_id: '', episode_order: 'daily' }, { episode_group_id: disney.id }]);
	});
});

describe('a chosen order TMDB cannot read', () => {
	it('stays checked in the menu under a fallback label', () => {
		const items = orderMenuItems({ orders, order: 'aired', chosen: disney.id, groups: [], words, pick: () => {} });
		expect(items.map((i) => ('label' in i ? i.label : i.kind))).toEqual(['As aired', 'By date', 'Absolute', 'separator', 'TMDB order (unavailable)']);
		expect(items[0]).toMatchObject({ checked: false });
		expect(items[4]).toMatchObject({ checked: true });
	});
	it('names the trigger by the fallback, not the numbering', () => {
		expect(orderLabel({ id: disney.id, name: '' }, 'As aired', 'TMDB order (unavailable)')).toBe('TMDB order (unavailable)');
		expect(orderLabel({ id: disney.id, name: 'Disney+ order' }, 'As aired', 'x')).toBe('Disney+ order');
		expect(orderLabel(null, 'As aired', 'x')).toBe('As aired');
	});
});

describe('orderBody', () => {
	it('saves the group id when a group is picked', () => {
		expect(orderBody({ group: disney.id }, null)).toEqual({ episode_group_id: disney.id });
	});
	it('clears the group when a numbering is picked on a show that has one', () => {
		expect(orderBody({ order: 'absolute' }, disney.id)).toEqual({ episode_group_id: '', episode_order: 'absolute' });
	});
	it('sends only the numbering when no group is set', () => {
		expect(orderBody({ order: 'absolute' }, null)).toEqual({ episode_order: 'absolute' });
	});
});

describe('splitOutside', () => {
	it('moves episodes the order leaves out into their own list, keeping order', () => {
		const eps: { id: number; outside_order?: boolean }[] = [{ id: 1 }, { id: 2, outside_order: true }, { id: 3 }, { id: 4, outside_order: true }];
		expect(splitOutside(eps)).toEqual({ inside: [{ id: 1 }, { id: 3 }], outside: [{ id: 2, outside_order: true }, { id: 4, outside_order: true }] });
	});
	it('keeps every episode inside when none is marked', () => {
		expect(splitOutside<{ id: number; outside_order?: boolean }>([{ id: 1 }]).outside).toEqual([]);
	});
});
