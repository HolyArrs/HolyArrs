import { describe, expect, it } from 'vitest';
import { curl, filterOps, groups, operations, type Spec } from './api-docs.ts';

const spec: Spec = {
	paths: {
		'/api/v1/titles': {
			get: { summary: 'List titles', tags: ['Library'], 'x-holyarrs-scope': 'read', 'x-holyarrs-permission': 'signed_in' },
			post: { summary: 'Add a movie', tags: ['Library'], 'x-holyarrs-scope': 'write' },
			parameters: {} as never
		},
		'/api/v1/backups': { get: { summary: 'List backups', tags: ['Backups'], 'x-holyarrs-scope': 'admin' } }
	}
};

describe('API docs page', () => {
	it('user opens the page / operations are listed by tag, path and method', () => {
		const ops = operations(spec);
		expect(ops.map((o) => `${o.method} ${o.path}`)).toEqual(['GET /api/v1/backups', 'GET /api/v1/titles', 'POST /api/v1/titles']);
		expect(ops[1].scope).toBe('read');
		expect(groups(ops).map((g) => g.tag)).toEqual(['Backups', 'Library']);
	});
	it('user types words in the search / only matching operations stay', () => {
		expect(filterOps(operations(spec), 'add').map((o) => o.summary)).toEqual(['Add a movie']);
		expect(filterOps(operations(spec), ' ').length).toBe(3);
	});
	it('user copies an example call / the key is in a header, not the address', () => {
		expect(curl('http://h/api/v1', 'GET', '/titles')).toBe('curl -H "X-Api-Key: $HOLYARRS_KEY" "http://h/api/v1/titles"');
		expect(curl('http://h/api/v1', 'POST', '/x', '{"a":1}')).toBe(
			`curl -X POST -H "X-Api-Key: $HOLYARRS_KEY" -H "Content-Type: application/json" -d '{"a":1}' "http://h/api/v1/x"`
		);
	});
});
