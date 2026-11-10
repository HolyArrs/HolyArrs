import { ApiError } from '#lib/core/api.ts';
import { plainMessage } from '#lib/core/settings.ts';
import { session } from '../core/session.svelte.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export function strong(e: unknown): string {
	return e instanceof ApiError && e.status === 403 && session.s.via === 'local_network'
		? t('accounts.strong.passwordLabel')
		: plainMessage(e);
}
