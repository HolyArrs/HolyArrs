<script lang="ts">
	import { to } from '#lib/paths.ts';
	import Icon from '#lib/Icon.svelte';
	import { sections as all, visible } from '#lib/settings.ts';
	import { session } from '#lib/session.svelte.ts';
	import { partsNow } from '#lib/parts.svelte.ts';
	import { offPartName } from '#lib/parts.ts';
	import { theme, setTheme, type ThemeChoice } from '#lib/theme.svelte.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';

	const sections = $derived(visible(all, session.can).filter((section) => !offPartName(partsNow.answer, section.href)));
	const groups = [
		{
			label: 'Library',
			description: 'Choose how HolyArrs organizes, finds and maintains your media.',
			hrefs: ['/settings/library-folders', '/settings/naming', '/settings/search', '/settings/subtitles', '/settings/music', '/settings/wishlist', '/settings/schedules']
		},
		{
			label: 'Linked apps',
			description: 'Connect the services that find, download and publish media.',
			hrefs: ['/settings/sources', '/settings/downloaders', '/settings/outside-lists', '/settings/media-servers', '/settings/tmdb', '/settings/alerts']
		},
		{
			label: 'Access',
			description: 'Manage people, sign-in and access for other apps.',
			hrefs: ['/settings/users', '/settings/account', '/settings/sign-in', '/settings/api']
		},
		{
			label: 'System',
			description: 'Maintain this installation and control which parts are running.',
			hrefs: ['/settings/backups', '/settings/move', '/settings/parts', '/settings/this-app', '/settings/logs']
		}
	];

	let query = $state('');
	const words = $derived(query.trim().toLowerCase().split(/\s+/).filter(Boolean));
	const matches = $derived(sections.filter((section) => words.every((w) => `${section.label} ${section.about}`.toLowerCase().includes(w))));
	let themeChoice = $state<ThemeChoice>(theme.choice);
	const themes: { value: ThemeChoice; label: string; icon: 'sun' | 'moon' | 'settings' }[] = [
		{ value: 'light', label: 'Light', icon: 'sun' },
		{ value: 'dark', label: 'Dark', icon: 'moon' },
		{ value: 'system', label: 'Automatic', icon: 'settings' }
	];
</script>

<svelte:head><title>Settings · HolyArrs</title></svelte:head>

<PageHeader title="Settings" subtitle="Manage your library, connections, access and this installation." />

<div class="find">
	<Icon name="search" size={17} />
	<label class="sr-only" for="settings-find">Search settings</label>
	<input id="settings-find" class="input" type="search" placeholder="Search settings" autocomplete="off" bind:value={query} />
</div>

{#if words.length}
	{#if matches.length}
		<ul class="results" aria-label="Matching settings">
			{#each matches as section (section.href)}
				<li>
					<a class="row-link" href={to(section.href)}>
						<span class="setting-icon"><Icon name={section.icon} size={17} /></span>
						<span class="setting-copy"><strong>{section.label}</strong><span>{section.about}</span></span>
						<Icon name="chevronRight" size={16} />
					</a>
				</li>
			{/each}
		</ul>
	{:else}
		<EmptyState compact icon="search" title="No settings match" text="Try another word, like folder, key or password." action={{ label: 'Clear the search', onclick: () => (query = '') }} />
	{/if}
{:else}
	<div class="settings-overview">
		<div class="wide">
			<SettingsCard title="Appearance" description="Choose how HolyArrs looks in this browser." icon="sun">
				<SegmentedControl label="Theme" options={themes} bind:value={themeChoice} onchange={(v) => setTheme(v)} />
			</SettingsCard>
		</div>

		{#each groups as group (group.label)}
			{@const items = sections.filter((section) => group.hrefs.includes(section.href))}
			{#if items.length}
				<SettingsCard title={group.label} description={group.description}>
					<ul class="links">
						{#each items as section (section.href)}
							<li>
								<a class="row-link" href={to(section.href)}>
									<span class="setting-icon"><Icon name={section.icon} size={17} /></span>
									<span class="setting-copy"><strong>{section.label}</strong><span>{section.about}</span></span>
									<Icon name="chevronRight" size={16} />
								</a>
							</li>
						{/each}
					</ul>
				</SettingsCard>
			{/if}
		{/each}
	</div>
{/if}

<style>
	.find { position: relative; display: flex; align-items: center; max-width: 520px; margin: -8px 0 24px; }
	.find > :global(.icon) { position: absolute; left: 12px; color: var(--text-muted); pointer-events: none; }
	.find .input { padding-left: 38px; border-radius: var(--r-pill); background: var(--surface-2); border-color: transparent; }
	.settings-overview { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 340px), 1fr)); gap: 16px; align-items: start; }
	.wide { grid-column: 1 / -1; }
	.links, .results { margin: 0 -12px -8px; }
	.results { margin: 0; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); padding: 6px; }
	li + li { border-top: 1px solid var(--line); }
	.row-link {
		display: flex;
		align-items: center;
		gap: 12px;
		min-height: 56px;
		padding: 8px 12px;
		border-radius: var(--r-btn);
		transition: background var(--dur-fast) var(--ease-out);
	}
	.row-link:hover { background: var(--surface-2); }
	.setting-icon { width: 30px; height: 30px; flex: none; display: grid; place-items: center; border-radius: 8px; background: var(--surface-2); color: var(--text); }
	.setting-copy { display: block; min-width: 0; flex: 1; }
	.setting-copy strong, .setting-copy span { display: block; }
	.setting-copy strong { font: var(--t-headline); font-size: 15px; }
	.setting-copy span { margin-top: 2px; color: var(--text-muted); font: var(--t-subhead); font-weight: 400; display: -webkit-box; overflow: hidden; line-clamp: 2; -webkit-line-clamp: 2; -webkit-box-orient: vertical; }
	.row-link > :global(.icon:last-child) { flex: none; color: var(--text-muted); }
</style>
