// Whether the Downloads page is on push updates now (the server sent revs with its snapshot). The layout opens /live for people who do not manage the Library only while this is on, so they receive their own patches.
export const dlPush = $state({ on: false });
