import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { SRC, buildGraph, isTest, usersOf } from '../../structure-graph.ts';


const PENDING_FILE = join(SRC, 'lib/ui/ui-pending.json');
const pending: string[] = existsSync(PENDING_FILE) ? JSON.parse(readFileSync(PENDING_FILE, 'utf8')) : [];

const graph = buildGraph();
const parts = graph.files.filter((f) => f.startsWith('lib/ui/') && !isTest(f) && !f.endsWith('.css'));

const single = parts.filter((f) => {
	const users = usersOf(f, graph);
	users.delete('devui');
	return users.size < 2;
});

if (process.env.UPDATE_PENDING) writeFileSync(PENDING_FILE, JSON.stringify([...single].sort(), null, '\t') + '\n');

describe('lib/ui', () => {
	it('holds only parts used by two or more modules', () => {
		expect(single.filter((f) => !pending.includes(f))).toEqual([]);
	});

	it('lists no part that qualifies now', () => {
		expect(pending.filter((f) => !single.includes(f))).toEqual([]);
	});
});
