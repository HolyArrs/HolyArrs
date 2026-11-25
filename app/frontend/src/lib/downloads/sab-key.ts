// SABnzbd's API key read from its General settings page (Settings > Downloaders: Fetch key). The username and password, when
// SABnzbd asks for them, are used for this one fetch and never kept.
import { api } from '#lib/core/api.ts';

export type SabKeyResult = { key: string; note: 'keyFetched' | 'keyNeedsLogin' | 'keyWrongLogin' | 'keyNotFound'; needLogin: boolean };

export async function fetchSabKey(url: string, allowLocal: boolean, user: string | null, pass: string): Promise<SabKeyResult> {
	try {
		const r = await api.post<{ key?: string; need_login?: boolean; problem?: string }>('/migrate/key-lookup', { kind: 'sabnzbd', url: url.trim(), allow_local_network: allowLocal, username: user ?? '', password: pass });
		if (r.key) return { key: r.key, note: 'keyFetched', needLogin: false };
		if (r.need_login) return { key: '', note: 'keyNeedsLogin', needLogin: true };
		return { key: '', note: r.problem === 'wrong_login' || user ? 'keyWrongLogin' : 'keyNotFound', needLogin: user !== null };
	} catch {
		return { key: '', note: 'keyNotFound', needLogin: user !== null };
	}
}
