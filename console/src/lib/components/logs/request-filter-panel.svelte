<script lang="ts">
	import { t } from 'svelte-i18n';

	import Field from '$lib/components/ui/field.svelte';
	import FilterPanel from '$lib/components/ui/filter-panel.svelte';
	import NativeSelect from '$lib/components/ui/native-select.svelte';
	import { FILTER_PANEL_KEYS } from '$lib/filter-panel';
	import { CLIENT_OPTIONS } from '$lib/client-setup';
	import { isRequestOrigin, isRequestStatus, type RequestFilters } from '$lib/process-logs';
	import type { AccessKey, Provider } from '$lib/types';

	// RequestFilterPanel is the fields grid behind the request log's filter
	// toggle: which provider, which class of answer, whose traffic, and which
	// agent sent it. The page owns the toggle, so which way the panel stands
	// arrives bound and only the fields render here.

	interface Props {
		filters: RequestFilters;
		onfilters: (patch: Partial<RequestFilters>) => void;
		providers: Provider[];
		keys: AccessKey[];
		open?: boolean;
	}

	let { filters, onfilters, providers, keys, open = $bindable(false) }: Props = $props();

	// The coding clients a key can be issued for, which is what a log row is
	// filtered by. The shared and free-text rows name no client of their own.
	const clientOptions = CLIENT_OPTIONS.filter(
		(option) => option.kind === 'agent' && option.id !== 'other'
	);
</script>

<FilterPanel storageKey={FILTER_PANEL_KEYS.logs} columns={3} bind:open>
	<Field id="log-provider" label={$t('ui.pages.logsPage.columnProvider')}>
		<NativeSelect
			id="log-provider"
			value={filters.provider}
			onchange={(event) => onfilters({ provider: event.currentTarget.value })}
		>
			<option value="">{$t('ui.pages.logsPage.filterAll')}</option>
			{#each providers as provider (provider.id)}
				<option value={provider.id}>{provider.label}</option>
			{/each}
		</NativeSelect>
	</Field>
	<Field id="log-status" label={$t('ui.pages.logsPage.filterStatus')}>
		<NativeSelect
			id="log-status"
			value={filters.status}
			onchange={(event) => {
				const { value } = event.currentTarget;
				if (isRequestStatus(value)) onfilters({ status: value });
			}}
		>
			<option value="any">{$t('ui.pages.logsPage.filterAny')}</option>
			<option value="ok">{$t('ui.pages.logsPage.filterOk')}</option>
			<option value="error">{$t('ui.pages.logsPage.filterError')}</option>
		</NativeSelect>
	</Field>
	<Field id="log-origin" label={$t('ui.pages.logsPage.filterOrigin')}>
		<NativeSelect
			id="log-origin"
			value={filters.origin}
			onchange={(event) => {
				const { value } = event.currentTarget;
				if (isRequestOrigin(value)) onfilters({ origin: value });
			}}
		>
			<option value="any">{$t('ui.pages.logsPage.filterAllOrigins')}</option>
			<option value="internal">{$t('ui.pages.logsPage.originInternal')}</option>
			<option value="external">{$t('ui.pages.logsPage.originExternal')}</option>
		</NativeSelect>
	</Field>
	<Field id="log-client" label={$t('ui.pages.logsPage.filterClient')}>
		<NativeSelect
			id="log-client"
			value={filters.client}
			onchange={(event) => onfilters({ client: event.currentTarget.value })}
		>
			<option value="">{$t('ui.pages.logsPage.filterAllClients')}</option>
			{#each clientOptions as option (option.id)}
				<option value={option.id}>{option.label}</option>
			{/each}
		</NativeSelect>
	</Field>
	<Field id="log-key" label={$t('ui.pages.logsPage.filterAccessKey')}>
		<NativeSelect
			id="log-key"
			value={filters.key}
			onchange={(event) => onfilters({ key: event.currentTarget.value })}
		>
			<option value="">{$t('ui.pages.logsPage.filterAllKeys')}</option>
			{#each keys as key (key.id)}
				<option value={key.id}>{key.name}</option>
			{/each}
		</NativeSelect>
	</Field>
</FilterPanel>
