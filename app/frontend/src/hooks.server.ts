import type { Handle } from '@sveltejs/kit/hooks';
import geistLatin from '@fontsource-variable/geist/files/geist-latin-wght-normal.woff2?url';

// The app's text font starts loading with the page instead of after its
// styles: app.html gets a preload for the built (hashed) file the styles name.
// The page is made once at build time (the index.html fallback), so this runs
// then, not per visit.
const fontPreload = `<link rel="preload" href="${geistLatin}" as="font" type="font/woff2" crossorigin>`;

export const handle: Handle = ({ event, resolve }) => resolve(event, { transformPageChunk: ({ html }) => html.replace('<!--holyarrs-font-preload-->', fontPreload) });
