import { AddArtistDialog, AskArtistDialog, AskShowDialog, QuickAsk } from '#lib/requests/index.ts';
import type { ArtistSheets } from '#lib/music/index.ts';

/** The requests dialogs that music, show and title parts show, passed in because those modules sit below requests. */
export const artistSheets: ArtistSheets = { ask: AskArtistDialog, add: AddArtistDialog };
export const askShowSheet = AskShowDialog;
export const quickAskSheet = QuickAsk;
