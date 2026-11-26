// Where the app opens: the page each person picked in My account (Home or Explore). Home stays the opening page while something waits there for them, and a link to any other page always opens that page.

import { session } from '#lib/core/session.svelte.ts';
import { views } from '#lib/core/views.svelte.ts';
import { appPath } from '#lib/core/paths.ts';
import { offPartName, partsNow } from '#lib/mediafiles/index.ts';
import { showJoinedWelcome } from '../(home)/joined-welcome.ts';

export interface OpeningInput {
	opensOn: string;
	role: string;
	/** The owner is still in first-run setup. */
	setupOpen: boolean;
	/** The owner skipped part of setup: the Finish setup card waits on Home. */
	setupSkipped: boolean;
	/** Two-factor sign-in must be turned on first: the frame sends them to My account. */
	mustTwoFactor: boolean;
	/** The saved "welcome.seen". */
	seen: unknown;
	/** Explore is turned off for this person, as the side menu decides. */
	exploreOff: boolean;
}

export function openingPath(i: OpeningInput): '/' | '/explore' {
	if (i.opensOn !== 'explore' || i.setupOpen || i.mustTwoFactor || i.exploreOff) return '/';
	if (i.role === 'owner' && i.setupSkipped) return '/';
	return showJoinedWelcome(i) ? '/' : '/explore';
}

/** Where the app opens for whoever is signed in. */
export const openingHere = () =>
	openingPath({
		opensOn: session.s.opensOn,
		role: session.s.role,
		setupOpen: session.s.setupOpen,
		setupSkipped: session.s.setupSkipped,
		mustTwoFactor: session.s.mustTwoFactor,
		seen: views.get('welcome.seen'),
		exploreOff: !!offPartName(partsNow.answer, '/explore')
	});

/** The address opens the app itself: the bare root, with no query or hash that points somewhere on Home. */
export const openedAtRoot = (url: Pick<URL, 'pathname' | 'search' | 'hash'>): boolean => appPath(url.pathname) === '/' && !url.search && !url.hash;

/** Where sign-in goes on to: the page asked for, or the opening page when none was. */
export const openingAfterSignIn = (next: string): string => (next === '/' ? openingHere() : next);
