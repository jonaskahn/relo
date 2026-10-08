<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from 'svelte-i18n';

	import { headerRecord, headerRows, type HeaderRow } from '$lib/header-rows';
	import Button from '$lib/components/ui/button.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import Icon from '$lib/components/ui/icon.svelte';

	interface Props {
		headers: Record<string, string>;
		onchange: (next: Record<string, string>) => void;
	}

	let { headers = {}, onchange }: Props = $props();

	// A name and a value are edited together, so the rows open from what the
	// provider already sends and only complete pairs leave this component. Each
	// row holds an id of its own, so a removal takes the row the operator
	// pointed at rather than the one that moved into its place.
	let nextRowId = 0;
	const takeRowId = () => nextRowId++;
	let rows = $state<HeaderRow[]>(untrack(() => headerRows(headers, takeRowId)));

	// A name and a value are kept together while they are typed, and only the
	// complete pairs leave this component.
	function emit(next: HeaderRow[]) {
		rows = next;
		onchange(headerRecord(next));
	}
</script>

<div class="flex flex-col gap-2">
	{#each rows as row (row.id)}
		<div class="flex items-center gap-2">
			<Input
				class="font-mono text-xs"
				placeholder={$t('ui.pages.providersPage.settings.headerName')}
				value={row.name}
				oninput={(event) => {
					row.name = event.currentTarget.value;
					emit([...rows]);
				}}
			/>
			<Input
				class="font-mono text-xs"
				placeholder={$t('ui.pages.providersPage.settings.headerValue')}
				value={row.value}
				oninput={(event) => {
					row.value = event.currentTarget.value;
					emit([...rows]);
				}}
			/>
			<Button
				variant="ghost"
				size="icon"
				aria-label={$t('ui.pages.providersPage.settings.removeHeader')}
				onclick={() => emit(rows.filter((other) => other.id !== row.id))}
			>
				<Icon name="trash" size={14} />
			</Button>
		</div>
	{/each}
	<Button
		variant="outline"
		size="sm"
		onclick={() => emit([...rows, { id: takeRowId(), name: '', value: '' }])}
	>
		<Icon name="plus" size={14} />
		{$t('ui.pages.providersPage.settings.addHeader')}
	</Button>
</div>
