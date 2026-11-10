<script lang="ts" module>
	// Icon set: each name maps to one stroke path on a 24 by 24 grid.
	const paths = {
		menu: 'M4 6h16M4 12h16M4 18h16',
		cloudOff: 'M8.2 8.3A4.5 4.5 0 0 0 7 17.5h9.5M12.3 6.6a5 5 0 0 1 4.9 4.1A3.5 3.5 0 0 1 19.6 16M3.5 3.5l17 17',
		home: 'M4 10.5 12 4l8 6.5V20a1 1 0 0 1-1 1h-4.5v-6h-5v6H5a1 1 0 0 1-1-1z',
		library: 'M5 4h3v16H5zM10.5 4h3v16h-3zM16 5.2l2.9-.8 3 15.4-2.9.8z',
		search: 'M10.5 4a6.5 6.5 0 1 1 0 13 6.5 6.5 0 0 1 0-13zM15.3 15.3 20 20',
		download: 'M12 4v11M7.5 10.5 12 15l4.5-4.5M5 19.5h14',
		calendar: 'M4.5 6.5h15v13h-15zM4.5 10.5h15M8.5 4v4M15.5 4v4',
		settings:
			'M12 9a3 3 0 1 1 0 6 3 3 0 0 1 0-6zM19.4 13.5a7.6 7.6 0 0 0 0-3l2-1.6-2-3.4-2.4 1a7.5 7.5 0 0 0-2.6-1.5L14 2.5h-4l-.4 2.5A7.5 7.5 0 0 0 7 6.5l-2.4-1-2 3.4 2 1.6a7.6 7.6 0 0 0 0 3l-2 1.6 2 3.4 2.4-1a7.5 7.5 0 0 0 2.6 1.5l.4 2.5h4l.4-2.5a7.5 7.5 0 0 0 2.6-1.5l2.4 1 2-3.4z',
		sun: 'M12 8a4 4 0 1 1 0 8 4 4 0 0 1 0-8zM12 2.5v2M12 19.5v2M2.5 12h2M19.5 12h2M5.3 5.3l1.4 1.4M17.3 17.3l1.4 1.4M5.3 18.7l1.4-1.4M17.3 6.7l1.4-1.4',
		moon: 'M19.5 14.5A8 8 0 0 1 9.5 4.5a8 8 0 1 0 10 10z',
		globe: 'M12 3a9 9 0 1 1 0 18 9 9 0 0 1 0-18zM3 12h18M12 3c2.5 2.5 3.8 5.5 3.8 9s-1.3 6.5-3.8 9M12 3C9.5 5.5 8.2 8.5 8.2 12s1.3 6.5 3.8 9',
		signout: 'M14 4.5H6a1 1 0 0 0-1 1v13a1 1 0 0 0 1 1h8M16 8l4 4-4 4M20 12H10',
		plus: 'M12 5v14M5 12h14',
		trash: 'M5 7h14M10 7V4.5h4V7M7 7l1 13h8l1-13M10.5 11v5.5M13.5 11v5.5',
		check: 'M5 12.5 10 17.5 19 7',
		x: 'M6 6l12 12M18 6 6 18',
		sliders: 'M4 7h10M18 7h2M4 17h4M12 17h8M14 7a2 2 0 1 0 4 0a2 2 0 1 0-4 0M8 17a2 2 0 1 0 4 0a2 2 0 1 0-4 0',
		refresh: 'M19.5 12a7.5 7.5 0 1 1-2.2-5.3M19.5 4.5v4.5H15',
		key: 'M8.5 11a4 4 0 1 1 0 .1zM12 12.5h8.5M17.5 12.5v3M20 12.5v2.5',
		folder: 'M3.5 6.5a1 1 0 0 1 1-1h5l2 2h8a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1h-15a1 1 0 0 1-1-1z',
		source: 'M4 6.5h16M4 12h16M4 17.5h10M18 16l2 2-2 2',
		image: 'M4 5h16v14H4zM9 9.5a1.5 1.5 0 1 1-.01 0M4 17l5-5 4 4 2.5-2.5L20 17',
		plug: 'M9 3.5v4M15 3.5v4M7 7.5h10v3.5a5 5 0 0 1-10 0zM12 16v4.5',
		downloader: 'M4 14.5h16v5H4zM12 4v7M8.5 7.5 12 11l3.5-3.5M7 17h.01',
		torrent: 'M6 3.5v8.5a6 6 0 0 0 12 0V3.5M6 8h4M14 8h4',
		usenet: 'M4 5h16v5.5H4zM4 13.5h16V19H4zM7.5 7.75h.01M7.5 16.25h.01M11 7.75h5M11 16.25h5',
		archive: 'M3.5 4.5h17v4h-17zM5 8.5v11h14v-11M10 12h4',
		shield: 'M12 3.5 19 6v5.5c0 4.4-3 7.7-7 9-4-1.3-7-4.6-7-9V6z',
		film: 'M4 4.5h16v15H4zM8 4.5v15M16 4.5v15M4 9.5h4M16 9.5h4M4 14.5h4M16 14.5h4',
		tv: 'M3.5 6.5h17v11h-17zM8.5 20.5h7M12 17.5v3M8 2.5l4 4 4-4',
		info: 'M12 3.5a8.5 8.5 0 1 1 0 17 8.5 8.5 0 0 1 0-17zM12 11v5.5M12 7.8v.1',
		arrow: 'M5 12h14M13 6l6 6-6 6',
		back: 'M19 12H5M11 6l-6 6 6 6',
		user: 'M12 4a4 4 0 1 1 0 8 4 4 0 0 1 0-8zM4.5 20.5c1-3.6 4-5.5 7.5-5.5s6.5 1.9 7.5 5.5',
		upload: 'M12 15V4M7.5 8.5 12 4l4.5 4.5M5 19.5h14',
		copy: 'M8.5 8.5h11v11h-11zM15.5 8.5v-4h-11v11h4',
		wish: 'M12 20s-7.5-4.6-7.5-10A4.3 4.3 0 0 1 12 7.4 4.3 4.3 0 0 1 19.5 10c0 5.4-7.5 10-7.5 10z',
		subtitle: 'M3.5 5.5h17v13h-17zM7 11.5h4M13 11.5h4M7 14.5h6.5M16 14.5h1',
		bell: 'M12 3.5v1.25M6.75 10.75a5.25 5.25 0 0 1 10.5 0v3.4c0 .8.3 1.55.85 2.1l.6.6c.4.4.12 1.15-.45 1.15H5.75c-.57 0-.85-.75-.45-1.15l.6-.6c.55-.55.85-1.3.85-2.1zM9.9 20.1a2.25 2.25 0 0 0 4.2 0',
		bellOn: 'M12 3.5v1.25M6.75 10.75a5.25 5.25 0 0 1 10.5 0v3.4c0 .8.3 1.55.85 2.1l.6.6c.4.4.12 1.15-.45 1.15H5.75c-.57 0-.85-.75-.45-1.15l.6-.6c.55-.55.85-1.3.85-2.1zM9.9 20.1a2.25 2.25 0 0 0 4.2 0M9.6 12.1l1.65 1.65 3.15-3.3',
		music: 'M9 18V5.5l11-2V16M9 18a3 3 0 1 1-6 0 3 3 0 0 1 6 0zM20 16a3 3 0 1 1-6 0 3 3 0 0 1 6 0z',
		people: 'M9 4.5a3.5 3.5 0 1 1 0 7 3.5 3.5 0 0 1 0-7zM2.5 19.5c.8-3.2 3.3-5 6.5-5s5.7 1.8 6.5 5M16 5a3 3 0 0 1 0 6M18 14.6c1.8.6 3 2.2 3.5 4.9',
		sidebar: 'M4.5 5h15a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1h-15a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1zM9.5 5v14M6 8.5h1.5M6 11h1.5',
		compass: 'M12 3.5a8.5 8.5 0 1 1 0 17 8.5 8.5 0 0 1 0-17zM15.5 8.5l-2 5-5 2 2-5z',
		chevronLeft: 'M14.5 6 8.5 12l6 6',
		chevronRight: 'M9.5 6l6 6-6 6',
		arrowUpRight: 'M7.5 16.5 16.5 7.5M9 7.5h7.5V15',
		chevronDown: 'M6 9.5l6 6 6-6',
		chevronUp: 'M6 14.5l6-6 6 6',
		chevronUpDown: 'M8 9.5l4-4 4 4M8 14.5l4 4 4-4',
		more: 'M6 12h.01M12 12h.01M18 12h.01',
		play: 'M8 5.5v13l10.5-6.5z',
		pause: 'M8.5 5.5v13M15.5 5.5v13',
		inbox: 'M4 13.5 6.5 5h11l2.5 8.5V19a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1zM4 13.5h4.5l1 2h5l1-2H20',
		requests: 'M4 6.5l1.5 1.5L8 5.5M4 12.5l1.5 1.5L8 11.5M4.5 18h3M11 7h9M11 13h9M11 18h9',
		tools: 'M14.5 4.5a4 4 0 0 0-4.6 5.4L4.5 15.3l3.2 3.2 5.4-5.4a4 4 0 0 0 5.4-4.6l-2.6 2.6-2.4-.6-.6-2.4z',
		eye: 'M2.5 12C4.6 8 8 5.5 12 5.5S19.4 8 21.5 12C19.4 16 16 18.5 12 18.5S4.6 16 2.5 12zM12 9a3 3 0 1 1 0 6 3 3 0 0 1 0-6z',
		eyeOff: 'M2.5 12C4.6 8 8 5.5 12 5.5S19.4 8 21.5 12C19.4 16 16 18.5 12 18.5S4.6 16 2.5 12zM12 9a3 3 0 1 1 0 6 3 3 0 0 1 0-6zM4 4l16 16',
		clock: 'M12 3.5a8.5 8.5 0 1 1 0 17 8.5 8.5 0 0 1 0-17zM12 7.5V12l3 2',
		grid: 'M4.5 4.5h6v6h-6zM13.5 4.5h6v6h-6zM4.5 13.5h6v6h-6zM13.5 13.5h6v6h-6z',
		star: 'M12 4l2.4 5 5.4.6-4 3.7 1.1 5.4L12 16l-4.9 2.7 1.1-5.4-4-3.7 5.4-.6z',
		sparkle: 'M11 3.5l1.8 5 5 1.8-5 1.8-1.8 5-1.8-5-5-1.8 5-1.8zM18 14.5l.9 2.1 2.1.9-2.1.9-.9 2.1-.9-2.1-2.1-.9 2.1-.9z',
		alert: 'M12 4 21 19.5H3zM12 10v4.5M12 17.2v.1',
		appearance: 'M12 3.5a8.5 8.5 0 1 1 0 17zM12 3.5a8.5 8.5 0 1 0 0 17',
		owned: 'M5.5 12.5l4 4 9-9.5',
		dash: 'M6.5 12h11',
		gauge: 'M4.5 16a7.5 7.5 0 1 1 15 0M12 16l3.5-5.5',
		partOwned: 'M12 4a8 8 0 1 1 0 16 8 8 0 0 1 0-16zM12 4v16M12 8.5h5M12 12h6M12 15.5h5',
		waiting: 'M12 4a8 8 0 1 1 0 16 8 8 0 0 1 0-16zM12 8v4l2.5 2',
		declined: 'M12 4a8 8 0 1 1 0 16 8 8 0 0 1 0-16zM6.4 6.4l11.2 11.2',
		calendarDay: 'M5 6.5h14v12.5H5zM5 10h14M9 4.5v3M15 4.5v3',
		sound: 'M4.5 9.5h3l4.5-4v13l-4.5-4h-3zM15.5 9a4 4 0 0 1 0 6M18 6.5a7.5 7.5 0 0 1 0 11',
		soundOff: 'M4.5 9.5h3l4.5-4v13l-4.5-4h-3zM15.5 9.5l5 5M20.5 9.5l-5 5',
		replay: 'M4.5 12a7.5 7.5 0 1 0 2.2-5.3M4.5 4.5v4.5H9',
		expand: 'M14 4.5h5.5V10M19.5 4.5l-6 6M10 19.5H4.5V14M4.5 19.5l6-6'
	} as const;
	export type IconName = keyof typeof paths;
</script>

<script lang="ts">
	// Decorative unless a label is given, then it is announced as an image.
	let { name, size = 20, label, stroke = 1.75 }: { name: IconName; size?: number; label?: string; stroke?: number } = $props();
</script>

<svg
	class="icon"
	class:flip={name === 'chevronLeft' || name === 'chevronRight' || name === 'arrow' || name === 'back'}
	width={size}
	height={size}
	viewBox="0 0 24 24"
	fill="none"
	stroke="currentColor"
	stroke-width={stroke}
	stroke-linecap="round"
	stroke-linejoin="round"
	role={label ? 'img' : undefined}
	aria-label={label}
	aria-hidden={label ? undefined : 'true'}
>
	<path d={paths[name]} />
</svg>

<style>
	.icon {
		flex: none;
		display: block;
	}
	/* Direction arrows point the other way in right-to-left languages. */
	:global([dir='rtl']) .flip {
		transform: scaleX(-1);
	}
</style>
