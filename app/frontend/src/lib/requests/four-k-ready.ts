import { api } from '../core/api.ts';
import type { AskLimit, QualityChoice } from './types.ts';

/** Which kinds can take a 4K request right now (4K quality set up); none when it cannot be read. */
export async function fourKReady(): Promise<{ movie: boolean; show: boolean }> {
	try {
		const l = await api.get<AskLimit>('/asks/limit');
		return { movie: !!l.four_k?.movie, show: !!l.four_k?.show };
	} catch {
		return { movie: false, show: false };
	}
}

/** The lines of what people can ask for that this person may use, per kind, in order; none when they cannot be read. */
export async function askQualities(): Promise<{ movie: QualityChoice[]; show: QualityChoice[] }> {
	try {
		const l = await api.get<AskLimit>('/asks/limit');
		return { movie: l.qualities?.movie ?? [], show: l.qualities?.show ?? [] };
	} catch {
		return { movie: [], show: [] };
	}
}
