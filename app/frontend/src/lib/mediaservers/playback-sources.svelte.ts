import { api } from '#lib/core/api.ts';
import { plainMessage } from '#lib/core/settings.ts';
import type { MediaServerView } from './types.ts';

export const playbackSources = $state<{ servers: Pick<MediaServerView, 'id' | 'name'>[]; loaded: boolean; error: string }>({
	servers: [],
	loaded: false,
	error: ''
});

let request = 0;

export function setPlaybackSources(servers: MediaServerView[]) {
	request++;
	playbackSources.servers = servers.filter((m) => m.kind === 'plex' && m.monitor_playback)
		.map(({ id, name }) => ({ id, name }));
	playbackSources.loaded = true;
	playbackSources.error = '';
}

export function forgetPlaybackSources() {
	setPlaybackSources([]);
	playbackSources.loaded = false;
}

export async function refreshPlaybackSources() {
	const id = ++request;
	try {
		const { media_servers } = await api.get<{ media_servers: MediaServerView[] }>('/media-servers');
		if (id === request) setPlaybackSources(media_servers);
	} catch (err) {
		if (id === request) {
			playbackSources.error = plainMessage(err);
			playbackSources.loaded = true;
		}
	}
}
