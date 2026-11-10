import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { api, message } from '../core/api.ts';
import { toasts } from '../core/toasts.svelte.ts';

// What each Downloader says about itself.
export type ClientStatus = { paused: boolean; speed: number; up_speed: number; speed_limit?: number; up_limit?: number; session_down: number; session_up: number; all_time_down: number; all_time_up: number; day_down: number; month_down: number; free_space: number; connection?: 'connected' | 'firewalled' | 'disconnected'; slow_mode?: boolean; time_left?: number };

export type Client = { id: number; name: string; protocol?: string; status?: ClientStatus; error?: string };

/** Polls the Downloaders' own status every 5 seconds. Managers only: watchers would get a 403 each time. */
export class Downloaders {
	clients = $state<Client[]>([]);
	loaded = $state(false);
	// Slow mode is a torrent Downloader's second, slower set of speed limits.
	slowBusy = $state(0);

	reporting = $derived(this.clients.filter((c) => c.status && !c.error));
	freeAll = $derived(this.reporting.map((c) => c.status?.free_space ?? 0).filter((n) => n > 0));
	health = $derived(!this.loaded ? t('downloads.clients.notReported') : !this.clients.length ? t('downloads.clients.notConnected') : this.reporting.length === this.clients.length ? t('downloads.clients.connected') : this.reporting.length ? t('downloads.clients.someConnected', { n: this.reporting.length, total: this.clients.length }) : t('downloads.clients.notConnected'));

	constructor(watch: () => boolean) {
		$effect(() => {
			if (watch()) return;
			void this.read();
			const t = setInterval(() => {
				if (document.visibilityState === 'visible') void this.read();
			}, 5000);
			const back = () => {
				if (document.visibilityState === 'visible') void this.read();
			};
			document.addEventListener('visibilitychange', back);
			return () => {
				clearInterval(t);
				document.removeEventListener('visibilitychange', back);
			};
		});
	}

	sumOf = (k: keyof ClientStatus) => this.reporting.reduce((n, c) => n + (Number(c.status?.[k]) || 0), 0);

	async read() {
		try {
			this.clients = (await api.get<{ clients: Client[] }>('/downloads/clients')).clients ?? [];
		} catch {
			this.clients = [];
		}
		this.loaded = true;
	}

	async setSlow(c: Client, on: boolean) {
		this.slowBusy = c.id;
		try {
			await api.post(`/downloaders/${c.id}/slow-mode`, { on });
			await this.read();
		} catch (e) {
			toasts.push(t('downloads.clients.slowFailed'), 'fail', { body: message(e) });
		} finally {
			this.slowBusy = 0;
		}
	}
}
