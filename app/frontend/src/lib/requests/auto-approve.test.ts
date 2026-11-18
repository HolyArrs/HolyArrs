import { describe, expect, it } from 'vitest';
import { approveRows, approveChanges, type ApproveUser } from './auto-approve.ts';

const users: ApproveUser[] = [
	{ id: 1, name: 'Luis', role: 'owner', state: 'active', perms: [] },
	{ id: 2, name: 'Ana', role: 'member', state: 'active', perms: ['ask', 'auto_approve'] },
	{ id: 3, name: 'Bo', role: 'admin', state: 'active', perms: ['ask'] },
	{ id: 4, name: 'Cy', role: 'member', state: 'disabled', perms: ['ask'] },
	{ id: 5, name: 'Di', role: 'member', state: 'active', perms: [] }
];

describe('approveRows', () => {
	it('lists the owner first from the setting and others from their own switch', () => {
		const rows = approveRows(users, true, { id: 1, role: 'owner' });
		expect(rows.map((r) => [r.id, r.on, r.locked])).toEqual([
			[1, true, false],
			[2, true, false],
			[3, false, false]
		]);
	});
	it('locks admins and yourself for an admin', () => {
		const rows = approveRows(users, false, { id: 3, role: 'admin' });
		expect(rows.find((r) => r.id === 1)?.on).toBe(false);
		expect(rows.find((r) => r.id === 3)?.locked).toBe(true);
		expect(rows.find((r) => r.id === 2)?.locked).toBe(false);
	});
});

describe('approveChanges', () => {
	it('gives each changed person their new permissions, leaving the owner out', () => {
		const before = approveRows(users, true, { id: 1, role: 'owner' });
		const after = before.map((r) => ({ ...r, on: !r.on }));
		expect(approveChanges(users, before, after)).toEqual([
			{ id: 2, perms: ['ask'] },
			{ id: 3, perms: ['ask', 'auto_approve'] }
		]);
	});
});
