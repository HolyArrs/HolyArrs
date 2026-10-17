<script lang="ts">
	import { to } from '#lib/paths.ts';
	import Icon from '#lib/Icon.svelte';
	import { sections as all, visible } from '#lib/settings.ts';
	import { session } from '#lib/session.svelte.ts';
	import { partsNow } from '#lib/parts.svelte.ts';
	import { offPartName } from '#lib/parts.ts';
	import { theme, setTheme } from '#lib/theme.svelte.ts';

	const sections = $derived(visible(all, session.can).filter((section) => !offPartName(partsNow.answer, section.href)));
	const groups = [
		{
			label: 'Library',
			description: 'Choose how HolyArrs organizes, finds and maintains your media.',
			hrefs: ['/settings/library-folders', '/settings/naming', '/settings/search', '/settings/subtitles', '/settings/music', '/settings/wishlist', '/settings/schedules']
		},
		{
			label: 'Connections',
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
</script>

<svelte:head><title>Settings · HolyArrs</title></svelte:head>

<header class="settings-head">
	<div>
		<h1>Settings</h1>
		<p>Manage your library, connections, access and this installation.</p>
	</div>
</header>

<div class="settings-overview">
	<section class="appearance" aria-labelledby="appearance-heading">
		<div class="appearance-copy">
			<span class="setting-icon"><Icon name="sun" size={19} /></span>
			<div>
				<h2 id="appearance-heading">Appearance</h2>
				<p>Choose how HolyArrs looks in this browser.</p>
			</div>
		</div>
		<div class="theme-control" role="radiogroup" aria-label="Theme">
			<label class:active={theme.value === 'light'}>
				<input type="radio" name="theme" checked={theme.value === 'light'} onchange={() => setTheme('light')} />
				Light
			</label>
			<label class:active={theme.value === 'dark'}>
				<input type="radio" name="theme" checked={theme.value === 'dark'} onchange={() => setTheme('dark')} />
				Dark
			</label>
		</div>
	</section>

	{#each groups as group (group.label)}
		{@const items = sections.filter((section) => group.hrefs.includes(section.href))}
		{#if items.length}
			<section class="settings-group" aria-labelledby="group-{group.label.toLowerCase()}">
				<header>
					<h2 id="group-{group.label.toLowerCase()}">{group.label}</h2>
					<p>{group.description}</p>
				</header>
				<ul>
					{#each items as section (section.href)}
						<li>
							<a href={to(section.href)}>
								<span class="setting-icon"><Icon name={section.icon} size={19} /></span>
								<span class="setting-copy">
									<strong>{section.label}</strong>
									<span>{section.about}</span>
								</span>
								<Icon name="arrow" size={16} />
							</a>
						</li>
					{/each}
				</ul>
			</section>
		{/if}
	{/each}
</div>

<style>
	.settings-head {
		display: flex;
		align-items: center;
		min-height: 78px;
		padding: var(--s-4) var(--s-6);
		border-bottom: 1px solid var(--line);
	}
	.settings-head h1 {
		font-size: var(--text-xl);
		line-height: 1.2;
		letter-spacing: -0.025em;
	}
	.settings-head p {
		margin-top: var(--s-1);
		color: var(--text-muted);
		font-size: var(--text-sm);
	}
	.settings-overview { max-width: 960px; padding: var(--s-6); }
	.appearance {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--s-5);
		padding: var(--s-4) 0 var(--s-6);
		border-bottom: 1px solid var(--line);
	}
	.appearance-copy {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		min-width: 0;
	}
	.appearance h2,
	.settings-group h2 { font-size: var(--text-md); line-height: 1.35; }
	.appearance p,
	.settings-group header p {
		margin-top: 2px;
		color: var(--text-muted);
		font-size: var(--text-sm);
	}
	.theme-control {
		display: grid;
		grid-template-columns: repeat(2, minmax(74px, 1fr));
		flex: none;
		padding: 3px;
		border: 1px solid var(--line);
		border-radius: var(--r-pill);
		background: var(--surface-3);
	}
	.theme-control label {
		min-height: 34px;
		display: grid;
		place-items: center;
		padding: 0 var(--s-3);
		border-radius: var(--r-pill);
		font-size: var(--text-sm);
		font-weight: var(--weight-medium);
		color: var(--text-muted);
		cursor: pointer;
	}
	.theme-control label.active {
		background: var(--surface-1);
		color: var(--text);
		box-shadow: 0 1px 3px rgb(24 24 21 / 10%);
	}
	.theme-control input {
		position: absolute;
		width: 1px;
		height: 1px;
		opacity: 0;
		pointer-events: none;
	}
	.theme-control label:has(input:focus-visible) { box-shadow: var(--focus-ring); }
	.settings-group { padding-top: var(--s-8); }
	.settings-group > header { padding-bottom: var(--s-3); }
	.settings-group ul { border-top: 1px solid var(--line); }
	.settings-group li { border-bottom: 1px solid var(--line); }
	.settings-group a {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		min-height: 72px;
		padding: var(--s-3) var(--s-2);
		transition: background var(--dur-fast);
	}
	.settings-group a:hover { background: var(--surface-2); }
	.setting-icon {
		width: 38px;
		height: 38px;
		display: grid;
		place-items: center;
		flex: none;
		border: 1px solid var(--line);
		border-radius: var(--r-btn);
		background: var(--surface-2);
		color: var(--text);
	}
	.setting-copy { display: block; min-width: 0; flex: 1; }
	.setting-copy strong,
	.setting-copy span { display: block; }
	.setting-copy strong { font-size: var(--text-sm); font-weight: var(--weight-semibold); }
	.setting-copy span {
		margin-top: 2px;
		max-width: 72ch;
		color: var(--text-muted);
		font-size: var(--text-sm);
		line-height: 1.4;
	}
	.settings-group a > :global(.icon:last-child) { flex: none; color: var(--text-faint); }
	@media (max-width: 700px) {
		.settings-head { min-height: 70px; padding: var(--s-4); }
		.settings-overview { padding: var(--s-4); }
		.appearance { align-items: stretch; flex-direction: column; }
		.theme-control { width: 100%; }
		.settings-group { padding-top: var(--s-6); }
		.settings-group a { min-height: 78px; padding-inline: 0; }
		.setting-copy span {
			display: -webkit-box;
			overflow: hidden;
			line-clamp: 2;
			-webkit-line-clamp: 2;
			-webkit-box-orient: vertical;
		}
	}
</style>
