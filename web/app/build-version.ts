import { execFileSync } from 'node:child_process';

// The name SvelteKit gives each build (/_app/version.json). It must change
// whenever the pages change, or a tab opened before an update never learns of
// the new build. HOLYARRS_BUILD_ID wins when set; else the git commit, with the
// build time added when the tree has edits not yet committed; else (no git, as
// in the Docker build) the build time. The same clean commit builds the same.
export function buildVersion(): string {
	const given = process.env.HOLYARRS_BUILD_ID?.trim();
	if (given) return given;
	const time = Date.now().toString(36);
	try {
		const git = (...args: string[]) => execFileSync('git', args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
		const sha = git('rev-parse', '--short', 'HEAD');
		const dirty = git('status', '--porcelain', '--untracked-files=no', '--', '.') !== '';
		return dirty ? `${sha}-${time}` : sha;
	} catch {
		return time;
	}
}
