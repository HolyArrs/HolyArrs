import { toasts } from '#lib/core/toasts.svelte.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { serverWords } from '#lib/core/i18n/server-words.ts';
import type { Choice } from '#lib/core/types.ts';

/** chosenToast says how a pick by hand went: sent, still sending, or refused
 * with the reason. It never says sent for a Release the Downloader did not get. */
export function chosenToast(ch: Choice, more?: string): void {
	if (ch.not_sent) return void toasts.push(t('mediafiles.releaseSheet.sendFailed'), 'fail', { body: serverWords(ch.not_sent) });
	const body = ch.warnings.length ? (ch.warnings.length === 1 ? serverWords(ch.warnings[0]) : t('mediafiles.title.warnings', { n: ch.warnings.length })) : more;
	toasts.push(t(ch.sending ? 'mediafiles.title.sending' : 'mediafiles.title.sent'), 'ok', body ? { body } : undefined);
}
