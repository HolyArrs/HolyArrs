// The words the users screen shows for each permission.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { Perm } from '../core/session.svelte.ts';

/** One line per permission, in the current language. Call it inside a $derived so it follows language changes. */
export function permWords(): Record<Perm, string> {
	return {
		ask: t('settings.users.permAsk'),
		auto_approve: t('settings.users.permAuto'),
		auto_approve_four_k: t('settings.users.permAuto4k'),
		approve: t('settings.users.permApprove'),
		manage_library: t('settings.users.permLibrary'),
		settings: t('settings.users.permSettings'),
		youtube: t('settings.users.permYouTube'),
		save_files: t('settings.users.permSaveFiles')
	};
}
