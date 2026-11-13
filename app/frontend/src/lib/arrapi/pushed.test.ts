import { describe, expect, it } from 'vitest';
import { pushedRows } from './pushed.ts';

describe('pushedRows', () => {
	it('lists formats and profiles together, newest change first', () => {
		const rows = pushedRows({
			formats: [{ side: 'sonarr', name: 'x265', updated_at: '2026-11-12T10:00:00Z' }],
			profiles: [{ side: 'radarr', name: 'HD', pick_rules_id: 4, notes: [], updated_at: '2026-11-12T11:00:00Z' }]
		});
		expect(rows.map((r) => [r.name, r.kind, r.side])).toEqual([
			['HD', 'profile', 'radarr'],
			['x265', 'format', 'sonarr']
		]);
	});
	it('is empty when nothing was sent or the server is older', () => {
		expect(pushedRows(undefined)).toEqual([]);
		expect(pushedRows({ formats: [], profiles: [] })).toEqual([]);
	});
});
