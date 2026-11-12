import { api } from '../core/api.ts';
import type { AskLimit } from './types.ts';

/** Which kinds can take a 4K request right now (4K quality set up); none when it cannot be read. */
export async function fourKReady(): Promise<{ movie: boolean; show: boolean }> {
	try {
		const l = await api.get<AskLimit>('/asks/limit');
		return { movie: !!l.four_k?.movie, show: !!l.four_k?.show };
	} catch {
		return { movie: false, show: false };
	}
}
