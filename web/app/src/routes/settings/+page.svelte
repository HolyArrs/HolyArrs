<script lang="ts">
	import { to } from '#lib/paths.ts';
	import Icon from '#lib/Icon.svelte';
	import { sections as all, visible } from '#lib/settings.ts';
	import { session } from '#lib/session.svelte.ts';
	import { partsNow } from '#lib/parts.svelte.ts';
	import { offPartName } from '#lib/parts.ts';
	// Pages of a part that is off are hidden here too.
	const sections = $derived(visible(all, session.can).filter((s) => !offPartName(partsNow.answer, s.href)));
	import { theme, setTheme } from '#lib/theme.svelte.ts';

	const groups = [
		{
			label: 'Library and automation',
			hrefs: ['/settings/library-folders', '/settings/naming', '/settings/search', '/settings/subtitles', '/settings/music', '/settings/wishlist', '/settings/schedules']
		},
		{
			label: 'Linked apps',
			hrefs: ['/settings/sources', '/settings/downloaders', '/settings/outside-lists', '/settings/media-servers', '/settings/tmdb', '/settings/alerts']
		},
		{
			label: 'People and access',
			hrefs: ['/settings/users', '/settings/account', '/settings/sign-in', '/settings/api']
		},
		{
			label: 'System',
			hrefs: ['/settings/backups', '/settings/move', '/settings/parts', '/settings/this-app', '/settings/logs']
		}
	];
</script>

<svelte:head><title>Settings · HolyArrs</title></svelte:head>

<header class="page-head">
	<div>
		<h1>Settings</h1>
		<p>Everything HolyArrs needs to run, one page each.</p>
	</div>
</header>

<div class="groups">
	{#each groups as group (group.label)}
		{@const items = sections.filter((section) => group.hrefs.includes(section.href))}
		{#if items.length}
			<section class="group" aria-labelledby="group-{group.label.replaceAll(' ', '-').toLowerCase()}">
				<h2 id="group-{group.label.replaceAll(' ', '-').toLowerCase()}" class="group-title">{group.label}</h2>
				<ul class="section-list">
					{#each items as s (s.href)}
						<li>
							<a class="tile" href={to(s.href)}>
								<span class="ic"><Icon name={s.icon} size={18} /></span>
								<span class="grow">
									<span class="t">{s.label}</span>
									<span class="d small muted">{s.about}</span>
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

<section class="card appearance" aria-labelledby="h-look">
	<h2 id="h-look" class="h2">Appearance</h2>
	<div class="row" role="radiogroup" aria-label="Theme" style="margin-top: var(--s-3)">
		<label class="checkline"><input type="radio" name="theme" checked={theme.value === 'dark'} onchange={() => setTheme('dark')} /> Dark</label>
		<label class="checkline"><input type="radio" name="theme" checked={theme.value === 'light'} onchange={() => setTheme('light')} /> Light</label>
	</div>
	<p class="hint">Saved in this browser. Movement follows your system's reduce-motion setting.</p>
</section>

<style>
	.groups {
		display: grid;
		gap: var(--s-8);
	}
	.group-title {
		margin-bottom: var(--s-2);
		font-size: var(--text-sm);
		font-weight: var(--weight-semibold);
		color: var(--text-muted);
	}
	.section-list {
		border-top: 1px solid var(--line);
	}
	.tile {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		min-height: 64px;
		padding: var(--s-3) var(--s-2);
		border-bottom: 1px solid var(--line);
		transition:
			background var(--dur-fast),
			color var(--dur-fast);
	}
	.tile:hover {
		background: var(--surface-2);
	}
	.tile > :global(.icon:last-child) {
		color: var(--text-faint);
	}
	.ic {
		width: 32px;
		height: 32px;
		display: grid;
		place-items: center;
		border-radius: var(--r-btn);
		background: var(--surface-3);
		color: var(--text);
		flex: none;
	}
	.t {
		display: block;
		font-weight: var(--weight-semibold);
	}
	.d {
		display: block;
		max-width: 66ch;
		line-height: 1.45;
	}
	.appearance {
		margin-top: var(--s-8);
	}
	@media (max-width: 600px) {
		.tile {
			min-height: 72px;
			padding: var(--s-3) 0;
		}
		.ic {
			width: 36px;
			height: 36px;
		}
	}
</style>
