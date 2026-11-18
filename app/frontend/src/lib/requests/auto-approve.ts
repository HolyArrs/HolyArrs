import type { Perm } from '#lib/core/session.svelte.ts';

export type ApproveUser = { id: number; name: string; role: string; state: string; perms: Perm[] };
export type ApproveRow = { id: number; name: string; owner: boolean; on: boolean; locked: boolean };

/** The people on the "Approve requests automatically" list: the owner first (their switch is a Request setting), then everyone active who may ask. */
export function approveRows(users: ApproveUser[], ownerOn: boolean, me: { id?: number; role: string }): ApproveRow[] {
	const rows: ApproveRow[] = [];
	for (const u of users) {
		if (u.state !== 'active') continue;
		if (u.role === 'owner') {
			rows.unshift({ id: u.id, name: u.name, owner: true, on: ownerOn, locked: false });
			continue;
		}
		if (u.role !== 'admin' && !u.perms.includes('ask')) continue;
		// Only the owner changes an admin, and nobody changes their own permissions.
		const locked = u.id === me.id || (me.role !== 'owner' && u.role !== 'member');
		rows.push({ id: u.id, name: u.name, owner: false, on: u.perms.includes('auto_approve'), locked });
	}
	return rows;
}

/** The permission edits that save the switches people changed; the owner's switch is saved with the settings. */
export function approveChanges(users: ApproveUser[], before: ApproveRow[], after: ApproveRow[]): { id: number; perms: Perm[] }[] {
	const out: { id: number; perms: Perm[] }[] = [];
	for (const r of after) {
		const was = before.find((b) => b.id === r.id);
		const u = users.find((x) => x.id === r.id);
		if (r.owner || !was || !u || was.on === r.on) continue;
		const rest = u.perms.filter((p) => p !== 'auto_approve');
		out.push({ id: r.id, perms: r.on ? [...rest, 'auto_approve'] : rest });
	}
	return out;
}
