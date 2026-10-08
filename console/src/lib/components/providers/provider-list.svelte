<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { Provider } from '$lib/types';
	import Icon from '$lib/components/ui/icon.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import ProviderCard from './provider-card.svelte';
	import { SECTION_LABEL_KEY, groupProviders, matchesQuery } from '$lib/provider-sections';

	interface Props {
		providers: Provider[];
		selectedId?: string;
		allModelsSelected?: boolean;
		totalModels?: number;
		onselect: (id: string) => void;
		onselectAll: () => void;
		ontoggle?: (provider: Provider, enabled: boolean) => void;
	}

	let {
		providers,
		selectedId = '',
		allModelsSelected = false,
		totalModels = 0,
		onselect,
		onselectAll,
		ontoggle
	}: Props = $props();

	let query = $state('');

	// The search appears only once the list is long enough to need it, which
	// keeps a short list free of chrome.
	const showSearch = $derived(providers.length > 8);
	const groups = $derived(groupProviders(providers, showSearch ? query : ''));
	const visible = $derived(
		providers.filter((p) => matchesQuery(showSearch ? query : '', p.id, p.label))
	);

	// Roving focus keeps arrow keys inside the list, which is what makes the
	// list a listbox rather than a column of buttons.
	let cursor = $state(0);

	function move(delta: number) {
		if (visible.length === 0) return;
		cursor = Math.min(visible.length - 1, Math.max(-1, cursor + delta));
		// Cursor -1 is the All models row, so Up from the first provider
		// climbs back to it the way the list reads top to bottom.
		if (cursor === -1) {
			onselectAll();
			requestAnimationFrame(() => {
				document.querySelector<HTMLElement>('[data-provider-row="__all__"]')?.focus();
			});
			return;
		}
		const target = visible[cursor];
		if (!target) return;
		onselect(target.id);
		// Roving focus follows the cursor so the next arrow press lands on
		// the row the operator just moved to.
		requestAnimationFrame(() => {
			document
				.querySelector<HTMLElement>('[data-provider-row="' + CSS.escape(target.id) + '"]')
				?.focus();
		});
	}

	function onKeydown(event: KeyboardEvent) {
		switch (event.key) {
			case 'ArrowDown':
				event.preventDefault();
				move(1);
				return;
			case 'ArrowUp':
				event.preventDefault();
				move(-1);
				return;
			case 'Home':
				event.preventDefault();
				move(-visible.length);
				return;
			case 'End':
				event.preventDefault();
				move(visible.length);
				return;
		}
	}
</script>

<div class="flex h-full min-h-0 flex-col border-e border-border">
	<div class="flex flex-col gap-2 px-3 py-4">
		{#if showSearch}
			<div class="relative">
				<span
					class="pointer-events-none absolute inset-y-0 start-2 flex items-center text-muted-foreground"
				>
					<Icon name="search" size={14} />
				</span>
				<Input
					class="ps-7"
					placeholder={$t('ui.pages.providersPage.list.search')}
					aria-label={$t('ui.pages.providersPage.list.search')}
					bind:value={query}
				/>
			</div>
		{/if}
	</div>
	<div
		class="min-h-0 flex-1 overflow-y-auto px-3 pb-4"
		aria-label={$t('ui.pages.providersPage.header.title')}
	>
		<button
			type="button"
			data-provider-row="__all__"
			aria-current={allModelsSelected ? 'true' : undefined}
			onkeydown={onKeydown}
			class="mb-4 flex w-full items-center gap-2.5 rounded-lg border px-3 py-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring {allModelsSelected
				? 'border-accent bg-accent-soft'
				: 'border-border bg-card hover:border-border-strong'}"
			onclick={onselectAll}
		>
			<span
				class="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border text-muted-foreground"
			>
				<Icon name="layers" size={16} />
			</span>
			<span class="min-w-0 flex-1">
				<span class="block truncate text-sm font-medium leading-5"
					>{$t('ui.pages.providersPage.list.allModels')}</span
				>
				<span class="block text-[0.7rem] text-muted-foreground">
					{$t('ui.pages.providersPage.list.allModelsMeta', {
						values: {
							models: $t('ui.pages.providersPage.list.modelCount', {
								values: { count: totalModels }
							}),
							providers: $t('ui.pages.providersPage.list.providerCount', {
								values: { count: providers.length }
							})
						}
					})}
				</span>
			</span>
		</button>

		{#each groups as group (group.section)}
			<div class="mb-3">
				<div class="flex items-baseline justify-between px-1 pb-1.5">
					<span
						class="text-[0.7rem] font-semibold uppercase tracking-[0.06em] text-muted-foreground"
						>{$t(SECTION_LABEL_KEY[group.section])}</span
					>
					<span class="text-[0.7rem] text-muted-foreground">{group.providers.length}</span>
				</div>
				<div class="flex flex-col gap-1">
					{#each group.providers as provider (provider.id)}
						<ProviderCard
							{provider}
							selected={provider.id === selectedId}
							{ontoggle}
							onrowkey={onKeydown}
							onselect={(id) => {
								cursor = Math.max(
									0,
									visible.findIndex((entry) => entry.id === id)
								);
								onselect(id);
							}}
						/>
					{/each}
				</div>
			</div>
		{/each}

		{#if groups.length === 0}
			<p class="px-2 py-6 text-center text-sm text-muted-foreground">
				{$t('ui.pages.providersPage.list.emptyBody')}
			</p>
		{/if}
	</div>
</div>
