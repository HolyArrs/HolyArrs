<script lang="ts">
	import { theme, setTheme, type ThemeChoice } from '#lib/theme.svelte.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';

	let themeChoice = $state<ThemeChoice>(theme.choice);
	const themes: { value: ThemeChoice; label: string; icon: 'sun' | 'moon' | 'settings' }[] = [
		{ value: 'light', label: 'Light', icon: 'sun' },
		{ value: 'dark', label: 'Dark', icon: 'moon' },
		{ value: 'system', label: 'Automatic', icon: 'settings' }
	];
</script>

<svelte:head><title>Appearance · HolyArrs</title></svelte:head>

<PageHeader title="Appearance" subtitle="How HolyArrs looks in this browser. Each person and each browser picks their own." />

<SettingsCard id="theme-card" title="Theme" description="Light, dark, or follow this device. The change shows straight away and is kept in this browser." icon="appearance">
	<div id="theme" tabindex="-1">
		<SegmentedControl label="Theme" options={themes} bind:value={themeChoice} onchange={(v) => setTheme(v)} />
	</div>
	<p class="hint">Automatic turns dark when this device does, for example at night.</p>
</SettingsCard>

<SettingsCard id="motion-card" title="Motion" description="HolyArrs follows this device's own Reduce motion setting: when it is on, screens change without sliding or growing.">
	<p class="hint">Change it in your device's accessibility settings.</p>
</SettingsCard>
