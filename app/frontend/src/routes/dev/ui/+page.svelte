<script lang="ts">
	// The component catalog: every lib/ui part in each state, light and dark side by side. Signed-in only (the layout handles that); not in the navigation.
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Badge from '#lib/ui/Badge.svelte';
	import StatusPill, { type StatusKind } from '#lib/ui/StatusPill.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Field from '#lib/ui/Field.svelte';
	import Menu from '#lib/ui/Menu.svelte';
	import Popover from '#lib/ui/Popover.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import WideCard from '#lib/ui/WideCard.svelte';
	import PersonCircle from '#lib/ui/PersonCircle.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import ProgressBar from '#lib/ui/ProgressBar.svelte';
	import { Table } from '#lib/accounts/index.ts';
	import Toast from '../../(frame)/Toast.svelte';
	import SidebarItem from '../../(frame)/SidebarItem.svelte';
	import TabBar from '../../(frame)/TabBar.svelte';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { reveal } from '#lib/core/motion.ts';

	type Width = 'both' | '1024' | '834' | '390';
	let width = $state<Width>('both');
	const widths: { value: Width; label: string }[] = [
		{ value: 'both', label: 'Fit' },
		{ value: '1024', label: '1024' },
		{ value: '834', label: '834' },
		{ value: '390', label: '390' }
	];

	// Self-made gradient art so the catalog needs no network.
	const art = (a: string, b: string, w = 200, h = 300) =>
		`data:image/svg+xml;utf8,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="${a}"/><stop offset="1" stop-color="${b}"/></linearGradient></defs><rect width="100%" height="100%" fill="url(#g)"/><circle cx="${w * 0.7}" cy="${h * 0.3}" r="${w * 0.22}" fill="rgba(255,255,255,.18)"/></svg>`)}`;
	const posters = [
		{ title: 'Quiet Harbour', sub: '2024 · Movie', img: art('#1b3a5c', '#e07a5f'), status: 'in_library' as StatusKind, badges: ['4K'] },
		{ title: 'The Long Field', sub: '2019 · Movie', img: art('#264653', '#2a9d8f'), status: 'downloading' as StatusKind, progress: 42 },
		{ title: 'Northern Lines', sub: 'TV show · 3 seasons', img: art('#3d405b', '#f2cc8f'), status: 'requested' as StatusKind },
		{ title: 'Paper Moons', sub: '2021 · Movie', img: null, status: 'missing' as StatusKind },
		{ title: 'Glass Orchard', sub: 'Coming 12 Nov', img: art('#5f0f40', '#fb8b24'), status: 'coming_soon' as StatusKind },
		{ title: 'Salt and Iron', sub: '2016 · Movie', img: art('#0b090a', '#a4161a'), badges: ['HDR', 'Atmos'] },
		{ title: 'Kite Season', sub: 'TV show', img: art('#003049', '#fcbf49') }
	];
	const statuses: StatusKind[] = ['in_library', 'downloading', 'missing', 'requested', 'approved', 'turned_down', 'failed', 'coming_soon'];

	let seg = $state<'grid' | 'list' | 'table'>('grid');
	let segL = $state<'grid' | 'list' | 'table'>('grid');
	let chips = $state({ movies: true, shows: false, music: false });
	let sheet = $state({ light: false, dark: false });
	let progress = $state(64);
	let rows = $state([
		{ id: 1, name: 'Quiet Harbour', size: '18.2 GB', state: 'Done', speed: '' },
		{ id: 2, name: 'The Long Field', size: '6.4 GB', state: '42%', speed: '24 MB/s' },
		{ id: 3, name: 'Northern Lines S02E04', size: '1.1 GB', state: 'Waiting', speed: '' }
	]);
</script>

<svelte:head><title>Components · HolyArrs</title></svelte:head>

<PageHeader title="Components" subtitle="Every part of the HolyArrs kit, light and dark side by side.">
	{#snippet actions()}<SegmentedControl label="Preview width" options={widths} bind:value={width} />{/snippet}
</PageHeader>

{#snippet gallery(theme: 'light' | 'dark')}
	<section class="block">
		<h2 class="bh">Buttons</h2>
		<div class="line">
			<Button variant="primary" size="lg" icon="plus">Get it</Button>
			<Button variant="primary">Add</Button>
			<Button variant="primary" size="sm">Small</Button>
		</div>
		<div class="line">
			<Button variant="secondary" icon="refresh">Search now</Button>
			<Button variant="plain">Show more</Button>
			<Button variant="destructive" icon="trash">Remove</Button>
		</div>
		<div class="line">
			<Button variant="primary" busy>Saving</Button>
			<Button variant="secondary" disabled>Disabled</Button>
			<Button variant="secondary" href="#top" iconAfter="arrow">As a link</Button>
		</div>
		<div class="line">
			<IconButton icon="plus" label="Add" variant="filled" size="lg" />
			<IconButton icon="play" label="Play" variant="tinted" size="lg" />
			<IconButton icon="more" label="More actions" variant="tinted" />
			<IconButton icon="bell" label="Alerts, 3 new" badge={3} />
			<IconButton icon="x" label="Close" size="sm" variant="tinted" />
			<IconButton icon="trash" label="Remove (disabled)" disabled />
		</div>
	</section>

	<section class="block">
		<h2 class="bh">Badges and status</h2>
		<div class="line">
			{#each ['4K', 'HDR', 'Dolby Vision', 'Atmos', 'CC', 'PG-13'] as b (b)}<Badge label={b} />{/each}
		</div>
		<div class="line">
			{#each statuses as s (s)}<StatusPill kind={s} percent={s === 'downloading' ? 42 : undefined} />{/each}
		</div>
	</section>

	<section class="block">
		<h2 class="bh">Chips and segmented control</h2>
		<div class="line">
			<Chip label="Movies" icon="film" selected={chips.movies} onclick={() => (chips.movies = !chips.movies)} />
			<Chip label="TV shows" icon="tv" selected={chips.shows} onclick={() => (chips.shows = !chips.shows)} />
			<Chip label="Music" icon="music" selected={chips.music} count={12} onclick={() => (chips.music = !chips.music)} />
			<Chip label="Genre: Drama" onremove={() => toasts.push('Removed the Drama filter')} />
		</div>
		<div class="line">
			{#if theme === 'light'}
				<SegmentedControl label="View ({theme})" bind:value={segL} options={[{ value: 'grid', label: 'Posters', icon: 'grid' }, { value: 'list', label: 'List' }, { value: 'table', label: 'Table' }]} />
			{:else}
				<SegmentedControl label="View ({theme})" bind:value={seg} options={[{ value: 'grid', label: 'Posters', icon: 'grid' }, { value: 'list', label: 'List' }, { value: 'table', label: 'Table' }]} />
			{/if}
			<SegmentedControl label="Size ({theme})" size="sm" value="m" options={[{ value: 's', label: 'S' }, { value: 'm', label: 'M' }, { value: 'l', label: 'L' }]} />
		</div>
	</section>

	<section class="block">
		<h2 class="bh">Menu, popover and sheet</h2>
		<div class="line">
			<Menu label="Title actions ({theme})" items={[
				{ label: 'Search now', icon: 'search', onselect: () => toasts.push('Searching') },
				{ label: 'Keep up to date', icon: 'refresh', checked: true },
				{ kind: 'separator' },
				{ kind: 'heading', label: 'Library' },
				{ label: 'Move', icon: 'folder' },
				{ label: 'Rename files', icon: 'copy', disabled: true },
				{ kind: 'separator' },
				{ label: 'Remove', icon: 'trash', destructive: true }
			]} />
			<Menu label="Sort by ({theme})" text="Sort: Name" variant="button" align="start" items={[{ label: 'Name', checked: true }, { label: 'Date added', checked: false }, { label: 'Year', checked: false }]} />
			<Popover label="About this ({theme})" icon="info">
				{#snippet children(close)}
					<div class="pop"><p>Popovers close on Escape, an outside click, or a pick, and return focus to their button.</p><Button size="sm" variant="secondary" onclick={close}>Got it</Button></div>
				{/snippet}
			</Popover>
			<Button variant="secondary" onclick={() => (sheet[theme] = true)}>Open sheet</Button>
		</div>
		<Sheet bind:open={sheet[theme]} title="Ask for Quiet Harbour" description="Pick what you would like and we will let you know when it is ready.">
			<div class="col">
				<SettingsCard title="Quality" description="The best copy that fits.">
					<SegmentedControl label="Quality" value="hd" options={[{ value: 'sd', label: 'Smaller' }, { value: 'hd', label: 'HD' }, { value: 'uhd', label: '4K' }]} />
				</SettingsCard>
			</div>
			{#snippet footer()}
				<Button variant="plain" onclick={() => (sheet[theme] = false)}>Cancel</Button>
				<Button variant="primary" onclick={() => { sheet[theme] = false; toasts.push('Asked for Quiet Harbour', 'ok'); }}>Get it</Button>
			{/snippet}
		</Sheet>
	</section>

	<section class="block">
		<Shelf title="Recently added" href="#top" subtitle="Posters, 2:3">
			{#each posters as p, i (p.title)}
				<div use:reveal={i}>
					<PosterCard title={p.title} subtitle={p.sub} image={p.img} href="#top" status={p.status} badges={p.badges} progress={p.progress} vtName="ui-{theme}-{i}">
						{#snippet action()}<IconButton icon="plus" label="Get {p.title}" variant="glass" size="sm" tooltip={false} />{/snippet}
					</PosterCard>
				</div>
			{/each}
		</Shelf>
	</section>

	<section class="block">
		<Shelf title="Downloading now" item="280px">
			<WideCard title="The Long Field" subtitle="Movie · 6.4 GB" meta="42% · 3 min left" image={art('#264653', '#2a9d8f', 320, 180)} progress={42} href="#top" status="downloading" />
			<WideCard title="Northern Lines" subtitle="S02 E04 · Ice" meta="Finished" image={art('#3d405b', '#f2cc8f', 320, 180)} progress={100} href="#top" badges={['HDR']} />
			<WideCard title="Kite Season" subtitle="S01 E01" meta="Waiting" image={null} href="#top" status="failed" />
		</Shelf>
	</section>

	<section class="block">
		<Shelf title="Cast and crew" item="112px">
			<PersonCircle name="Ana Rivera" role="Mara" href="#top" image={art('#6d597a', '#e56b6f', 120, 120)} />
			<PersonCircle name="Tomás Lind" role="Director" href="#top" />
			<PersonCircle name="June Okafor" role="Ellis" image={art('#355070', '#eaac8b', 120, 120)} />
			<PersonCircle name="Pia Novak" role="Writer" />
		</Shelf>
	</section>

	<section class="block">
		<h2 class="bh">Table</h2>
		<Table caption="Downloads ({theme})" columns={[{ key: 'name', label: 'Title' }, { key: 'size', label: 'Size', align: 'end', hideOnPhone: true }, { key: 'speed', label: 'Speed', align: 'end', hideOnPhone: true }, { key: 'state', label: 'State', align: 'end' }]} {rows} rowKey={(r) => r.id} />
		<div class="sp"></div>
		<Table caption="Empty table ({theme})" columns={[{ key: 'name', label: 'Title' }]} rows={[] as { name: string }[]} rowKey={(r) => r.name} empty="No downloads right now." />
	</section>

	<section class="block">
		<h2 class="bh">Settings card</h2>
		<SettingsCard title="Appearance" description="Light, dark, or follow this device." icon="appearance">
			<SegmentedControl label="Appearance ({theme})" value="system" options={[{ value: 'light', label: 'Light' }, { value: 'dark', label: 'Dark' }, { value: 'system', label: 'System' }]} />
			{#snippet footer()}<Button variant="primary" size="sm">Save</Button>{/snippet}
		</SettingsCard>
	</section>

	<section class="block">
		<h2 class="bh">Fields</h2>
		<Field id="ui-name-{theme}" label="Name" hint="Shown to everyone in your household." required>
			<input id="ui-name-{theme}" class="input" value="Living room" />
		</Field>
		<Field id="ui-kind-{theme}" label="Kind">
			<select id="ui-kind-{theme}" class="input"><option>Movies</option><option>Shows</option></select>
		</Field>
		<Field id="ui-url-{theme}" label="Address" error="That address does not answer.">
			<input id="ui-url-{theme}" class="input mono" value="http://10.0.0.5:8080" aria-invalid="true" aria-describedby="ui-url-{theme}-error" />
		</Field>
	</section>

	<section class="block">
		<h2 class="bh">Progress, loading and errors</h2>
		<div class="col">
			<ProgressBar value={progress} label="Example progress ({theme})" showValue />
			<ProgressBar value={100} label="Finished example ({theme})" />
			<div class="line"><Button size="sm" variant="secondary" onclick={() => (progress = progress >= 100 ? 12 : Math.min(100, progress + 22))}>Advance</Button></div>
			<div class="skels"><Skeleton shape="poster" /><Skeleton shape="poster" /><Skeleton shape="poster" /><div class="col"><Skeleton lines={3} /><Skeleton shape="circle" /></div></div>
			<LoadError hasData message="Could not refresh downloads." onretry={() => toasts.push('Trying again')} />
			<LoadError hasData={false} message="Could not load the Library." onretry={() => toasts.push('Trying again')} />
		</div>
	</section>

	<section class="block">
		<h2 class="bh">Empty state and toasts</h2>
		<EmptyState icon="inbox" title="Nothing waiting" text="New asks from your household show up here." action={{ label: 'Open Explore', href: '#top', icon: 'compass' }} compact />
		<div class="col">
			<Toast text="Saved." tone="ok" ondismiss={() => undefined} />
			<Toast text="Could not reach the downloader." tone="fail" ondismiss={() => undefined} />
			<Toast text="Searching for Quiet Harbour." />
			<div class="line">
				<Button size="sm" variant="secondary" onclick={() => toasts.push('Saved', 'ok')}>Show a toast</Button>
				<Button size="sm" variant="secondary" onclick={() => toasts.push("Couldn't save", 'fail')}>Show a failure</Button>
			</div>
		</div>
	</section>

	<section class="block">
		<h2 class="bh">Navigation parts</h2>
		<div class="navdemo">
			<SidebarItem href="#top" label="Home" icon="home" current />
			<SidebarItem href="#top" label="Downloads" icon="download" badge={6} badgeLabel="6 need a decision" />
			<SidebarItem label="Search" icon="search" kbd="⌘K" onclick={() => undefined} />
		</div>
		<div class="tabdemo">
			<TabBar label="Tab bar example ({theme})" items={[
				{ label: 'Home', icon: 'home', href: '#top', current: true },
				{ label: 'Explore', icon: 'compass', href: '#top' },
				{ label: 'Library', icon: 'library', href: '#top' },
				{ label: 'Search', icon: 'search', href: '#top' },
				{ label: 'Manage', icon: 'inbox', dot: true, onclick: () => undefined }
			]} />
		</div>
	</section>
{/snippet}

<div class="panes" class:fit={width === 'both'} style:--pane={width === 'both' ? undefined : `${width}px`}>
	<div class="pane" data-theme="light" role="group" aria-label="Light">
		<p class="tag">Light</p>
		{@render gallery('light')}
	</div>
	<div class="pane" data-theme="dark" role="group" aria-label="Dark">
		<p class="tag">Dark</p>
		{@render gallery('dark')}
	</div>
</div>

<style>
	.panes { display: flex; gap: 20px; align-items: flex-start; overflow-x: auto; padding-bottom: 12px; }
	.pane { --gutter: 20px; flex: 0 0 var(--pane, 100%); width: var(--pane, 100%); min-width: 0; padding: 20px var(--gutter); border-radius: var(--r-panel); background: var(--bg); color: var(--text); box-shadow: 0 0 0 1px var(--line); container-type: inline-size; }
	.fit .pane { flex: 1 1 0; }
	@media (max-width: 1279px) { .fit { flex-direction: column; } .fit .pane { flex: none; width: 100%; } }
	.tag { display: inline-block; margin-bottom: 12px; padding: 2px 10px; border-radius: 999px; background: var(--surface-2); color: var(--text-muted); font: var(--t-footnote); font-weight: 600; }
	.block { padding: 18px 0; border-bottom: 1px solid var(--line); display: flex; flex-direction: column; gap: 12px; }
	.block:last-child { border-bottom: 0; }
	.bh { font: var(--t-title3); letter-spacing: var(--t-title3-track); }
	.line { display: flex; flex-wrap: wrap; gap: 10px; align-items: center; }
	.col { display: flex; flex-direction: column; gap: 12px; }
	.sp { height: 12px; }
	.pop { display: flex; flex-direction: column; gap: 10px; padding: 8px; font: var(--t-callout); }
	.skels { display: grid; grid-template-columns: repeat(3, 80px) 1fr; gap: 12px; align-items: start; }
	.navdemo { width: 240px; max-width: 100%; display: flex; flex-direction: column; gap: 2px; padding: 8px; border-radius: var(--r-card); background: var(--sidebar-solid); }
	.navdemo :global(.ui-sbi.current) { background: var(--surface-3); }
	.tabdemo { position: relative; height: 72px; border-radius: var(--r-card); overflow: hidden; box-shadow: 0 0 0 1px var(--line); }
	.tabdemo :global(.ui-tabbar) { display: flex; position: absolute; }
</style>
