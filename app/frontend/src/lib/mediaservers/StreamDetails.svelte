<script lang="ts">
	import { duration } from '#lib/core/format.ts';
	import { lang, t } from '#lib/core/i18n/i18n.svelte.ts';

	type Connection = 'lan' | 'wan' | 'relay';
	type Details = {
		source_resolution?: string;
		source_video?: string;
		source_audio?: string;
		output_height?: number;
		output_video?: string;
		output_audio?: string;
		video_decision?: string;
		audio_decision?: string;
		subtitle_decision?: string;
		connection?: Connection;
		bandwidth_kbps?: number;
		hardware?: string;
		buffer_ms: number;
		buffer_events: number;
	};
	let { details }: { details: Details } = $props();

	const ratio = (source?: string, output?: string) => {
		const a = source?.toUpperCase();
		const b = output?.toUpperCase();
		return a && b && a !== b ? a + ' → ' + b : a || b || '';
	};
	const resolution = $derived(ratio(details.source_resolution, details.output_height ? details.output_height + 'p' : undefined).replaceAll('P', 'p'));
	const audio = $derived(ratio(details.source_audio, details.output_audio));
	const video = $derived(ratio(details.source_video, details.output_video));
	const bandwidth = $derived(details.bandwidth_kbps ? '≈ ' + (details.bandwidth_kbps / 1000).toLocaleString(lang.value, { maximumFractionDigits: 2 }) + ' Mb/s' : '');

	function decision(value?: string): string {
		switch (value?.toLowerCase()) {
			case 'directplay': return t('settings.mediaServers.playback.method.direct_play');
			case 'copy':
			case 'directstream': return t('settings.mediaServers.playback.method.direct_stream');
			case 'transcode': return t('settings.mediaServers.playback.method.transcode');
			default: return value || '';
		}
	}
</script>

<details>
	<summary>{t('mediafiles.copy.details')}</summary>
	<dl>
		{#if resolution || video || details.video_decision}
			<dt>{t('music.video')}</dt><dd>{[resolution, video, decision(details.video_decision), details.hardware ? details.hardware === '1' ? 'HW' : details.hardware.toUpperCase() : ''].filter(Boolean).join(' · ')}</dd>
		{/if}
		{#if audio || details.audio_decision}
			<dt>{t('mediafiles.col.audio')}</dt><dd>{[audio, decision(details.audio_decision)].filter(Boolean).join(' · ')}</dd>
		{/if}
		{#if details.subtitle_decision}
			<dt>{t('mediafiles.sheet.subtitles')}</dt><dd>{decision(details.subtitle_decision)}</dd>
		{/if}
		{#if details.connection || bandwidth}
			<dt>{t('downloads.bar.network')}</dt><dd>{[details.connection?.toUpperCase(), bandwidth].filter(Boolean).join(' · ')}</dd>
		{/if}
		<dt>{t('settings.mediaServers.playback.state.buffering')}</dt>
		<dd data-testid="buffering-observed">≈ {duration(details.buffer_ms / 1000)} · {details.buffer_events.toLocaleString(lang.value)}×</dd>
	</dl>
</details>

<style>
	details { border-top: 1px solid var(--line); padding-top: var(--s-2); font: var(--t-footnote); }
	summary { cursor: pointer; color: var(--accent-text); width: fit-content; padding-block: var(--s-1); }
	dl { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: var(--s-1) var(--s-3); margin: var(--s-2) 0 0; }
	dt { color: var(--text-muted); }
	dd { margin: 0; overflow-wrap: anywhere; }
</style>
