function freezeProfile(profile) {
	for (const candidate of Object.values(profile.references)) {
		if (candidate.seasons) Object.freeze(candidate.seasons);
		if (candidate.albums) Object.freeze(candidate.albums);
		Object.freeze(candidate);
	}
	Object.freeze(profile.references);
	return Object.freeze(profile);
}

const candidateProfiles = Object.freeze({
	default: freezeProfile({
		name: 'default',
		references: {
			movie: { kind: 'movie', reference: 'tmdb:900008', name: 'Copper Lighthouse' },
			show: { kind: 'show', reference: 'tmdb:800003', name: 'Moss Parade', seasons: [1] },
			music: {
				kind: 'artist',
				reference: 'mbid:a3333333-0000-4000-8000-000000000003',
				name: 'Copper Wren',
				albums: ['b3333333-0000-4000-8000-000000000001'],
				albumName: 'Low Orbit',
				unselectedAlbumName: 'Wire'
			},
			denied: { kind: 'movie', reference: 'tmdb:900009', name: 'Velvet Tide' },
			withdrawn: { kind: 'movie', reference: 'tmdb:900007', name: 'Northern Kettle' }
		}
	}),
	audit20261017: freezeProfile({
		name: 'audit20261017',
		references: {
			movie: { kind: 'movie', reference: 'tmdb:900002', name: 'Paper Harbour' },
			show: { kind: 'show', reference: 'tmdb:800004', name: 'Quarry Bells', seasons: [1] },
			music: {
				kind: 'artist',
				reference: 'mbid:a2222222-0000-4000-8000-000000000002',
				name: 'Moss Choir',
				albums: ['b2222222-0000-4000-8000-000000000001'],
				albumName: 'Slow Green',
				unselectedAlbumName: 'Fern Hymns'
			},
			denied: { kind: 'movie', reference: 'tmdb:900005', name: 'Salt Choir' },
			withdrawn: { kind: 'movie', reference: 'tmdb:900007', name: 'Northern Kettle' }
		}
	})
});

function selectCandidateProfile(name = process.env.HOLYARRS_REAL_SAVE_PROFILE || 'default') {
	const profile = candidateProfiles[name];
	if (!profile) throw new Error(`Unknown real-save candidate profile: ${name}`);
	return profile;
}

function exactKeys(value, expected) {
	return Boolean(value && typeof value === 'object' && !Array.isArray(value)
		&& Object.keys(value).sort().join('\0') === [...expected].sort().join('\0'));
}

function isAllowedRequestBody(profile, body) {
	if (!profile?.references || typeof profile.references !== 'object') return false;
	const candidate = Object.values(profile.references).find(item => item.reference === body?.id);
	if (!candidate) return false;
	if (candidate.kind === 'movie') return exactKeys(body, ['id']);
	if (candidate.kind === 'show') {
		return exactKeys(body, ['id', 'kind', 'seasons']) && body.kind === 'show'
			&& JSON.stringify(body.seasons) === JSON.stringify(candidate.seasons);
	}
	if (candidate.kind === 'artist') {
		return exactKeys(body, ['id', 'kind', 'albums']) && body.kind === 'artist'
			&& JSON.stringify(body.albums) === JSON.stringify(candidate.albums);
	}
	return false;
}

module.exports = { candidateProfiles, selectCandidateProfile, isAllowedRequestBody };
