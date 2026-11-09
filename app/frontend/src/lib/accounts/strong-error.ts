import { ApiError } from '#lib/core/api.ts';
import { plainMessage } from '#lib/core/settings.ts';
import { session } from '../core/session.svelte.ts';

export function strong(e: unknown): string {
	return e instanceof ApiError && e.status === 403 && session.s.via === 'local_network'
		? 'Sign in with your password for this.'
		: plainMessage(e);
}
