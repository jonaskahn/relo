<script lang="ts">
	import { t } from 'svelte-i18n';

	import Card from '$lib/components/ui/card.svelte';
	import CardContent from '$lib/components/ui/card-content.svelte';
	import CardDescription from '$lib/components/ui/card-description.svelte';
	import CardHeader from '$lib/components/ui/card-header.svelte';
	import CardTitle from '$lib/components/ui/card-title.svelte';
	import StatusBadge from '$lib/components/ui/status-badge.svelte';
	import { keyPillValues, keyStatusPill } from '$lib/access-keys';
	import { consoleState } from '$lib/console-state.svelte';
	import { formatDate, relativeTime } from '$lib/format';
	import { cardLayoutCard } from '$lib/theme.svelte';
	import type { AccessKey } from '$lib/types';

	// KeyCard is one client key: its name, who it was issued for, the hint
	// that identifies it without revealing it, and when it was last used. The
	// whole card opens the detail; the selection checkbox is a separate
	// control so it never takes the card's click.

	interface Props {
		accessKey: AccessKey;
		selectable?: boolean;
		selected?: boolean;
		onopen: (key: AccessKey) => void;
		onselect: (id: string, on: boolean) => void;
	}

	let { accessKey, selectable = false, selected = false, onopen, onselect }: Props = $props();

	const pill = $derived(keyStatusPill(accessKey));
</script>

<div class="relative h-full">
	{#if selectable}
		<input
			type="checkbox"
			class="absolute left-3 top-4 z-10 size-4"
			checked={selected}
			onchange={(event) => onselect(accessKey.id, event.currentTarget.checked)}
			onclick={(event) => event.stopPropagation()}
			aria-label={$t('ui.pages.keysPage.selectKey', { values: { name: accessKey.name } })}
		/>
	{/if}
	<button
		type="button"
		data-plain
		aria-label={$t('ui.common.openCard', { values: { name: accessKey.name } })}
		onclick={() => onopen(accessKey)}
		class="block h-full w-full text-left transition-transform duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring active:scale-[0.985]"
	>
		<Card class="section-surface {cardLayoutCard(consoleState.settings.cardLayout)} gap-4 py-5">
			<CardHeader class="shrink-0 gap-1.5">
				<div class="flex min-w-0 items-center justify-between gap-2 {selectable ? 'pl-6' : ''}">
					<CardTitle class="truncate text-sm">{accessKey.name}</CardTitle>
					<StatusBadge
						kind={pill.kind}
						label={$t(pill.labelKey, { values: keyPillValues(pill) })}
					/>
				</div>
				<CardDescription
					class="flex items-center gap-1.5 truncate text-xs {selectable ? 'pl-6' : ''}"
				>
					{accessKey.client || $t('ui.status.kind.' + accessKey.kind)}
					{#if accessKey.owner}
						<span class="badge border border-line text-muted-foreground">
							{$t('ui.pages.keysPage.owned')}
						</span>
					{/if}
				</CardDescription>
			</CardHeader>
			<CardContent class="flex flex-col gap-2 text-xs">
				<div class="flex items-center justify-between gap-3">
					<span class="text-muted-foreground">{$t('ui.pages.keysPage.columnHint')}</span>
					<span class="mono-data truncate">{accessKey.token_hint}</span>
				</div>
				<div class="flex items-center justify-between gap-3">
					<span class="text-muted-foreground">{$t('ui.pages.keysPage.columnCreated')}</span>
					<span>{formatDate(accessKey.created_at_ms)}</span>
				</div>
				<div class="flex items-center justify-between gap-3">
					<span class="text-muted-foreground">{$t('ui.pages.keysPage.columnLastUsed')}</span>
					<span>{relativeTime(accessKey.last_used_at_ms)}</span>
				</div>
			</CardContent>
		</Card>
	</button>
</div>
