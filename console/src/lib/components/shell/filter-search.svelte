<script lang="ts">
	import type { Snippet } from 'svelte';

	import { consoleState } from '$lib/console-state.svelte';
	import { cardLayoutSlot } from '$lib/theme.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import NativeSelect from '$lib/components/ui/native-select.svelte';
	import { cn } from '$lib/utils';

	// FilterSearch is the filter select and search pair every card-grid page
	// narrows with. The pair spans exactly one card column of the grid below and
	// re-widths with the layout control, one third filter to two thirds search.
	// Below 640px the pair falls into the page's action row: search takes a full
	// row first, and the filter shares the next row with the page's actions.
	// Pass filterFullRowOnMobile where the filter must instead take its own
	// full row like search.
	interface Props {
		filterId: string;
		filterLabel: string;
		searchLabel: string;
		searchPlaceholder: string;
		filter?: unknown;
		search?: string;
		onsearch?: (next: string) => void;
		filterFullRowOnMobile?: boolean;
		children?: Snippet;
	}

	let {
		filterId,
		filterLabel,
		searchLabel,
		searchPlaceholder,
		filter = $bindable(),
		search = $bindable(''),
		onsearch,
		filterFullRowOnMobile = false,
		children
	}: Props = $props();
</script>

<div
	class={cn(
		'contents sm:grid sm:min-w-0 sm:grid-cols-[minmax(0,1fr)_minmax(0,2fr)] sm:items-center sm:gap-2',
		// The width eases only while the layout control moves the cards: a
		// window resize must track its container exactly, not lag behind it.
		consoleState.cardLayoutAnimating && 'motion-safe:transition-[width] motion-safe:duration-200',
		cardLayoutSlot(consoleState.settings.cardLayout)
	)}
>
	<NativeSelect
		id={filterId}
		class={filterFullRowOnMobile ? 'min-w-0 max-sm:basis-full' : 'min-w-0 max-sm:flex-1'}
		bind:value={filter}
		aria-label={filterLabel}
	>
		{@render children?.()}
	</NativeSelect>
	<div class="relative min-w-0 max-sm:order-first max-sm:basis-full">
		<span
			class="pointer-events-none absolute inset-y-0 start-2 flex items-center text-muted-foreground"
		>
			<Icon name="search" size={14} />
		</span>
		<Input
			class="ps-7"
			value={search}
			oninput={(event) => {
				search = event.currentTarget.value;
				onsearch?.(search);
			}}
			placeholder={searchPlaceholder}
			aria-label={searchLabel}
		/>
	</div>
</div>
