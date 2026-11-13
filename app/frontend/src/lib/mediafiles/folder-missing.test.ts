import { describe, expect, it } from 'vitest';
import { ApiError } from '#lib/core/api.ts';
import { folderMissing } from './folder-missing.ts';

const gone = () => new ApiError('the folder does not exist', 400, { error: 'the folder does not exist' }, null);

describe('folderMissing', () => {
	it('explains mounting with the typed path and a container path for the kind', () => {
		const line = folderMissing(gone(), '/mnt/1TB/Movies/', 'movie');
		expect(line).toBe("HolyArrs can't see /mnt/1TB/Movies. It only sees folders mounted into its container. Add a volume like /mnt/1TB/Movies:/movies, then use /movies here.");
	});
	it('picks /shows and /music for those kinds', () => {
		expect(folderMissing(gone(), '/srv/tv', 'show')).toContain('/srv/tv:/shows');
		expect(folderMissing(gone(), '/srv/songs', 'music')).toContain('/srv/songs:/music');
	});
	it('leaves other errors alone', () => {
		expect(folderMissing(new ApiError('that folder, or one inside or around it, is already a Library folder', 409, {}, null), '/x', 'movie')).toBeNull();
		expect(folderMissing(new Error('boom'), '/x', 'movie')).toBeNull();
	});
});
