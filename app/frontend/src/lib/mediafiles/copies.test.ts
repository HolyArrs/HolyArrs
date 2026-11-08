import { describe, expect, it } from 'vitest';
import { copyKey, copiesQuery, toggleCopy, type CopySelection } from './copies.ts';

describe('Library copy selection', () => {
	it('keeps exact file and Version selections across pages', () => {
		const first: CopySelection = { file_id: 10, version_id: 20 };
		const second: CopySelection = { file_id: 11, version_id: 20 };
		let selected: CopySelection[] = [];
		selected = toggleCopy(selected, first);
		selected = toggleCopy(selected, second);
		expect(selected.map(copyKey)).toEqual(['10:20', '11:20']);
		selected = toggleCopy(selected, first);
		expect(selected).toEqual([second]);
	});

	it('builds bounded encoded list queries', () => {
		expect(copiesQuery({ q: 'Alien & Aliens', codec: 'hevc', sort: 'codec', limit: 50, offset: 100 })).toBe(
			'/library/copies?q=Alien+%26+Aliens&codec=hevc&sort=codec&limit=50&offset=100'
		);
	});
});
