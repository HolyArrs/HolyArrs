// The heading and lead line of each first-run step, and of its sub-screens.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { ServerKind, Step } from './welcome-steps.ts';

type Head = { h: string; p: string };
export type HeadState = {
	firstRun: boolean;
	step: Step;
	linkOpen: ServerKind | null;
	linkSub: ServerKind | null;
	chooseSub: 'apps-a' | 'apps-b' | 'fresh' | null;
	importing: boolean;
	// The picked old app's name, while its import is open.
	importName: string;
	readOpen: boolean;
	todo: number;
	// Folders after Bring my apps: the old apps' folders, not the parts.
	bringFolders?: boolean;
};

const kindName = (k: ServerKind) => ({ plex: 'Plex', jellyfin: 'Jellyfin', emby: 'Emby' })[k];

function stepHead(step: Step): Head {
	const titles: Record<Step, Head> = {
		parts: { h: t('welcome.parts.title'), p: t('welcome.parts.lead') },
		server: { h: t('welcome.link.title'), p: t('welcome.link.lead') },
		folders: { h: t('welcome.whereSYourMedia'), p: t('welcome.folders.lead') },
		sources: { h: t('welcome.whereShouldWeSearch'), p: t('welcome.linkProwlarrOrJackett') },
		downloads: { h: t('welcome.whichAppDownloads'), p: t('welcome.connectTheOneYou') },
		subtitles: { h: t('welcome.subs.title'), p: t('welcome.subs.lead') },
		access: { h: t('welcome.access.title'), p: t('welcome.access.lead') },
		done: { h: t('welcome.youReSet'), p: t('welcome.findSomethingOnHome') }
	};
	return titles[step];
}

export function welcomeHead(s: HeadState): Head {
	const { firstRun, step, linkOpen, linkSub, chooseSub, importName } = s;
	return firstRun && step === 'server' && linkOpen && !linkSub
		? { h: t('welcome.link.signInTo', { name: kindName(linkOpen) }), p: t('accounts.login.useAdmin', { way: kindName(linkOpen) }) }
		: firstRun && step === 'folders' && (s.bringFolders || s.importing)
		? { h: t('welcome.whereSYourMedia'), p: t('welcome.folders.bringLead') }
		: firstRun && step === 'parts' && s.importing && importName && s.readOpen
		? { h: t('welcome.link.appTitle', { name: importName }), p: t('welcome.link.appLead') }
		: firstRun && step === 'parts' && chooseSub
		? chooseSub === 'apps-b'
			? { h: t('welcome.choose.tools'), p: t('welcome.choose.toolsLead') }
			: chooseSub === 'apps-a'
				? { h: t('welcome.choose.apps'), p: t('welcome.choose.appsLead') }
				: { h: t('welcome.choose.fresh'), p: t('welcome.choose.freshLead') }
		: firstRun && step === 'server' && linkSub
		? { h: t('welcome.link.serverTitle', { name: kindName(linkSub) }), p: t('welcome.link.serverLead') }
		: !firstRun
		? { h: t('welcome.whatDoYouUse'), p: t('welcome.youCanChangeThis') }
		: step === 'done' && s.todo
			? { h: t('welcome.holyarrsIsReady'), p: t('welcome.aFewThingsAre') }
			: stepHead(step);
}
