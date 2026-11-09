// How a request reads on its card: words, tone, icon and the steps of a download.
import { type IconName } from '#lib/ui/Icon.svelte';
import { titleStatus } from '#lib/ui/title-status.ts';
import { askedBy as askedByRaw, alsoDeclined as alsoDeclinedRaw } from './ask-words.ts';
import { liveFacts } from './live.ts';
import { askArtist, askDetails } from './details.svelte.ts';
import { profile } from '#lib/accounts/index.ts';
import { seasonWords } from '#lib/shows/index.ts';
import { albumWords } from '#lib/music/index.ts';
import { decided as decidedRaw } from '#lib/core/plain.ts';
import { to } from '#lib/core/paths.ts';
import { ago } from '#lib/core/format.ts';
import type { TitleLive } from './live.ts';
import { session } from '#lib/core/session.svelte.ts';
import { looksLikeId } from '#lib/core/plain.ts';
import type { Ask } from '#lib/core/types.ts';

export function createAskCards(ctx: { approver: boolean; titleLive: Map<number, TitleLive>; asks: Ask[]; load: (quiet?: boolean) => Promise<void>; loadLive: () => Promise<void> }) {
	const wideOf = (a: Ask): string => {
		if (a.kind === 'artist') return askArtist(a)?.backgrounds[0] ?? '';
		const d = askDetails(a);
		return d?.backdrop ? to(d.backdrop) : '';
	};
	const cardTone = (a: Ask) => (whyFailed(a) ? 'bad' : a.state === 'asked' ? 'wait' : a.state === 'approved' ? 'go' : a.state === 'turned_down' ? 'soft' : 'ok');
	const cardIcon = (a: Ask): IconName => (whyFailed(a) ? 'alert' : a.state === 'asked' ? 'clock' : a.state === 'turned_down' ? 'x' : a.state === 'available' ? 'library' : 'check');
	const kinds = [
		{ k: 'all', label: 'Everything' },
		{ k: 'movie', label: 'Movies' },
		{ k: 'show', label: 'TV shows' },
		{ k: 'artist', label: 'Music' }
	] as const;

	// One status vocabulary (title-status.ts): approvers get the staff words.
	const askPill = (a: Ask) => titleStatus({ ask: a.state, titleKind: a.kind === 'artist' ? 'artist' : a.kind === 'show' ? 'show' : 'movie' }, ctx.approver ? 'staff' : 'member');

	// Id-like account names read as a person, never a raw id.
	const plain = (t: string) => t.replace(/[\w.-]+/g, (w) => (/\d/.test(w) && /\p{L}/u.test(w) && looksLikeId(w) ? 'someone in your household' : w));
	const askedBy = (a: Ask) => plain(askedByRaw(a));
	const decided = (a: Ask) => plain(decidedRaw(a, session.s.id != null ? { id: session.s.id, displayName: profile.displayName } : undefined) ?? '');
	const alsoDeclined = (a: Ask) => plain(alsoDeclinedRaw(a, session.s.id != null ? { id: session.s.id, displayName: profile.displayName } : undefined));

	function year(a: Ask) {
		return a.year ? ` (${a.year})` : '';
	}

	/** What a show or artist ask is for; '' for movies. */
	function what(a: Ask) {
		if (a.kind === 'artist') return a.album_words ?? albumWords(a.albums);
		return a.kind === 'show' ? (a.season_words ?? seasonWords(a.seasons)) : '';
	}

	/** Why an approved ask is stuck (the server's reason when it has one); '' when nothing failed. */
	function whyFailed(a: Ask): string {
		if (a.state !== 'approved') return '';
		if (a.problem) return a.problem;
		if (!a.title_id) return 'HolyArrs couldn’t add it to the Library.';
		const f = liveFacts(ctx.titleLive.get(a.title_id));
		return f.phase === 'failed' ? `The download failed.${f.reason ? ` ${f.reason}` : ''}` : '';
	}

	/** Says, in plain words, where the person's own approved ask is now: no percent, no download names. */
	function stepLine(a: Ask): string {
		if (a.state !== 'approved' || !a.step) return '';
		switch (a.step.kind) {
			case 'searching':
				return 'Searching for it now';
			case 'downloading':
				return 'Downloading now';
			case 'importing':
				return 'Adding it to the Library';
			case 'waiting_on_admin':
				return 'Waiting on an admin';
			case 'not_out':
				return 'Not out yet';
			case 'waiting':
				return a.step.searched_at ? `Nothing found yet · last looked ${ago(a.step.searched_at)}` : 'Not looked for yet';
		}
		return '';
	}
	// While one of the person's own asks is on its way, read the list again now and then: a search starting or an import finishing reaches them only this way. Without "Run the Library" there are no download events, so the downloads read here would freeze; the poll reads them again too while any are on screen.
	const noDlEvents = $derived(!session.can('manage_library'));
	const inFlight = $derived(
		ctx.asks.some((a) => a.state === 'approved' && ['searching', 'downloading', 'importing', 'waiting_on_admin'].includes(a.step?.kind ?? '')) || (noDlEvents && [...ctx.titleLive.values()].some((t) => t.download && t.download.phase !== 'failed'))
	);
	$effect(() => {
		if (!inFlight) return;
		const t = setInterval(() => {
			if (document.visibilityState !== 'visible') return;
			void ctx.load(true);
			if (noDlEvents) void ctx.loadLive();
		}, 15_000);
		return () => clearInterval(t);
	});

	/** What happens after approval. */
	function approvedLine(a: Ask) {
		if (a.kind === 'show') return `HolyArrs keeps ${what(a)} up to date and looks for the episodes.`;
		if (a.kind === 'artist') return `HolyArrs keeps ${what(a)} up to date and looks for the albums.`;
		return 'HolyArrs is looking for it.';
	}

	// Who asked: picture and age limit.

	return {
		decided,
		plain,
		what,
		year,
		askedBy,
		alsoDeclined,
		askPill,
		approvedLine,
		get kinds() { return kinds; },
		wideOf,
		cardTone,
		cardIcon,
		whyFailed,
		stepLine,
	};
}
