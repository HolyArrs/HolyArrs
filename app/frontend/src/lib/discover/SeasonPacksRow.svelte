<script lang="ts">
	// Season packs row on a show's Library tab: the whole season, or a few
	// episodes at a time from a pack (qBittorrent only).
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';

	let { value = 0, busy = false, onpick }: { value?: number; busy?: boolean; onpick: (n: number) => void } = $props();

	const choices = [0, 1, 2, 3, 5];
	const words = (n: number) => (n === 1 ? t('discover.titleLibrary.packOneAtATime') : n > 0 ? t('discover.titleLibrary.packAtATime', { n }) : t('discover.titleLibrary.packWhole'));
	const menu = $derived<MenuItem[]>(choices.map((n) => ({ label: words(n), checked: value === n, onselect: () => onpick(n) })));
</script>

<div class="lib-row static" data-testid="season-packs-row">
	<span><b>{t('discover.titleLibrary.seasonPacks')}</b><small class="wrap">{t('discover.titleLibrary.packHint')}</small></span>
	<div class="lib-pick"><Menu label={t('discover.titleLibrary.seasonPacks')} variant="button" items={menu} width={220}>{#snippet trigger()}<span>{busy ? t('discover.titleLibrary.saving') : value ? t('discover.titleLibrary.packShort', { n: value }) : t('discover.titleLibrary.packWhole')}</span><Icon name="chevronDown" size={14} />{/snippet}</Menu></div>
</div>
