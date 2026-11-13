// What apps such as Profilarr sent to the Sonarr and Radarr sides of the old-app API, as one read-only list.
export type PushedSide = 'sonarr' | 'radarr';
export interface Pushed {
	formats: { side: PushedSide; name: string; updated_at?: string }[];
	profiles: { side: PushedSide; name: string; pick_rules_id: number; notes: string[]; updated_at?: string }[];
}
export interface PushedRow {
	key: string;
	name: string;
	kind: 'format' | 'profile';
	side: PushedSide;
	updated: string;
}

export function pushedRows(p: Pushed | undefined): PushedRow[] {
	if (!p) return [];
	const rows: PushedRow[] = [
		...(p.formats ?? []).map((f): PushedRow => ({ key: `f-${f.side}-${f.name}`, name: f.name, kind: 'format', side: f.side, updated: f.updated_at ?? '' })),
		...(p.profiles ?? []).map((x): PushedRow => ({ key: `p-${x.pick_rules_id}`, name: x.name, kind: 'profile', side: x.side, updated: x.updated_at ?? '' }))
	];
	return rows.sort((a, b) => b.updated.localeCompare(a.updated) || a.name.localeCompare(b.name));
}
