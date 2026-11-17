import { beforeEach, describe, expect, it, vi } from 'vitest';

const push = vi.fn();
vi.mock('#lib/core/toasts.svelte.ts', () => ({ toasts: { push: (...a: unknown[]) => push(...a) } }));

const { chosenToast } = await import('./chosen-toast.ts');

describe('chosenToast', () => {
	beforeEach(() => push.mockClear());

	it('never says sent when the Downloader did not get the Release', () => {
		chosenToast({ release_id: 1, slot_id: 2, forced: false, warnings: [], not_sent: 'the Release link leads nowhere' });
		expect(push).toHaveBeenCalledWith("Couldn't send to Downloader", 'fail', { body: 'the Release link leads nowhere' });
	});

	it('says sending while the send has not ended', () => {
		chosenToast({ release_id: 1, slot_id: 2, forced: false, warnings: [], sending: true });
		expect(push).toHaveBeenCalledWith('Sending to Downloader', 'ok', undefined);
	});

	it('says sent once the Downloader got it', () => {
		chosenToast({ release_id: 1, slot_id: 2, forced: false, warnings: [] }, 'for 3 episodes');
		expect(push).toHaveBeenCalledWith('Sent to Downloader', 'ok', { body: 'for 3 episodes' });
	});
});
