<script lang="ts">
	import { t } from 'svelte-i18n';

	import Field from '$lib/components/ui/field.svelte';
	import FilterPanel from '$lib/components/ui/filter-panel.svelte';
	import Select from '$lib/components/ui/select.svelte';
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
		<Select
			id="log-provider"
			value={filters.provider}
			onValueChange={(next) => onfilters({ provider: next })}
			options={[
				{ value: '', label: $t('ui.pages.logsPage.filterAll') },
				...providers.map((provider) => ({ value: provider.id, label: provider.label }))
			]}
		/>
	</Field>
	<Field id="log-status" label={$t('ui.pages.logsPage.filterStatus')}>
		<Select
			id="log-status"
			value={filters.status}
			onValueChange={(next) => {
				if (isRequestStatus(next)) onfilters({ status: next });
			}}
			options={[
				{ value: 'any', label: $t('ui.pages.logsPage.filterAny') },
				{ value: 'ok', label: $t('ui.pages.logsPage.filterOk') },
				{ value: 'error', label: $t('ui.pages.logsPage.filterError') }
			]}
		/>
	</Field>
	<Field id="log-origin" label={$t('ui.pages.logsPage.filterOrigin')}>
		<Select
			id="log-origin"
			value={filters.origin}
			onValueChange={(next) => {
				if (isRequestOrigin(next)) onfilters({ origin: next });
			}}
			options={[
				{ value: 'any', label: $t('ui.pages.logsPage.filterAllOrigins') },
				{ value: 'internal', label: $t('ui.pages.logsPage.originInternal') },
				{ value: 'external', label: $t('ui.pages.logsPage.originExternal') }
			]}
		/>
	</Field>
	<Field id="log-client" label={$t('ui.pages.logsPage.filterClient')}>
		<Select
			id="log-client"
			value={filters.client}
			onValueChange={(next) => onfilters({ client: next })}
			options={[
				{ value: '', label: $t('ui.pages.logsPage.filterAllClients') },
				...clientOptions.map((option) => ({ value: option.id, label: option.label }))
			]}
		/>
	</Field>
	<Field id="log-key" label={$t('ui.pages.logsPage.filterAccessKey')}>
		<Select
			id="log-key"
			value={filters.key}
			onValueChange={(next) => onfilters({ key: next })}
			options={[
				{ value: '', label: $t('ui.pages.logsPage.filterAllKeys') },
				...keys.map((key) => ({ value: key.id, label: key.name }))
			]}
		/>
	</Field>
</FilterPanel>
