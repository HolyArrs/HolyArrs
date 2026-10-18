// Additive metadata fixture for preview 18746; never seeds or edits stored titles.
export const musicLifecycleArtist = {
	id: 'a4444444-0000-4000-8000-000000000004',
	name: 'Amber Satellites', type: 'Group', country: 'GB', ended: false,
	disambiguation: 'disposable preview lifecycle sample',
	albums: [
		['b4444444-0000-4000-8000-000000000001', 'Evening Signals', 'Album', [], '2024-04-12', [['c4444444-0000-4000-8000-000000000001', '', 'XW', '2024-04-12', 'Digital Media', 3]]],
		['b4444444-0000-4000-8000-000000000002', 'Quiet Orbit', 'Album', [], '2025-02-07', [['c4444444-0000-4000-8000-000000000002', '', 'XW', '2025-02-07', 'Digital Media', 2]]]
	]
};

export function addMusicLifecycleSample(fakes) {
	if (fakes.fakeArtists.some(artist => artist.id === musicLifecycleArtist.id || artist.name === musicLifecycleArtist.name)) throw new Error('Music lifecycle sample identity already present in provider');
	fakes.fakeArtists.push(structuredClone(musicLifecycleArtist));
}
