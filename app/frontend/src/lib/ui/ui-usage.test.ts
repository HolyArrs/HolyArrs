import { describe, expect, it } from 'vitest';
import { buildGraph, isTest, usersOf } from '../../structure-graph.ts';

const graph = buildGraph();
const parts = graph.files.filter((f) => f.startsWith('lib/ui/') && !isTest(f) && !f.endsWith('.css'));

const single = parts.filter((f) => {
	const users = usersOf(f, graph);
	users.delete('devui');
	return users.size < 2;
});

describe('lib/ui', () => {
	it('holds only parts used by two or more modules', () => {
		expect(single).toEqual([]);
	});
});
