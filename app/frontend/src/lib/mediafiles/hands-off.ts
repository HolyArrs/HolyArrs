// Hands-off mode on the add sheets and the title page: HolyArrs only sends to the Downloader and leaves the files there.

// A Slot whose download finished in hands-off mode: in the Library with no file, since the file stays in the Downloader.
export const leftInDownloader = (slot: { state?: string; file?: unknown }) => slot.state === 'in_library' && !slot.file;

// The stand-in folder a hands-off title with no Library folder points to; it is not a real folder.
export const standInFolder = (path: string | null | undefined) => String(path ?? '').startsWith('hands-off:');

// An add sheet stops for a missing Library folder only when hands-off mode is off.
export const needsFolder = (folderCount: number, handsOff: boolean) => folderCount === 0 && !handsOff;
