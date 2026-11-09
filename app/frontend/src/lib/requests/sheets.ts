import AddArtistDialog from './AddArtistDialog.svelte';
import AskArtistDialog from './AskArtistDialog.svelte';
import AskShowDialog from './AskShowDialog.svelte';
import QuickAsk from './QuickAsk.svelte';
import type { ArtistSheets } from '#lib/music/index.ts';

/** The requests dialogs that music, show and title parts show, passed in because those modules sit below requests. */
export const artistSheets: ArtistSheets = { ask: AskArtistDialog, add: AddArtistDialog };
export const askShowSheet = AskShowDialog;
export const quickAskSheet = QuickAsk;
