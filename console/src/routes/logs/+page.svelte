<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { page } from '$app/state';

	import Button from '$lib/components/ui/button.svelte';
	import FilterToggle from '$lib/components/ui/filter-toggle.svelte';
	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';
	import LiveButton from '$lib/components/ui/live-button.svelte';
	import PageHeader from '$lib/components/ui/page-header.svelte';
	import Segmented from '$lib/components/ui/segmented.svelte';
	import AgentSection from '$lib/components/logs/agent-section.svelte';
	import DaemonSection from '$lib/components/logs/daemon-section.svelte';
	import RequestDetailModal from '$lib/components/logs/request-detail-modal.svelte';
	import StartupSection from '$lib/components/logs/startup-section.svelte';
	import {
		browserStorage,
		filterPanelId,
		readPanelOpen,
		writePanelOpen,
		FILTER_PANEL_KEYS
	} from '$lib/filter-panel';
	import {
		countRequestFilters,
		parseLogSection,
		type DaemonLevel,
		type LogSection,
		type RequestFilters
	} from '$lib/process-logs';
	import { prefersReducedMotion, tabDirection, tabPaneIn, tabPaneOut } from '$lib/tab-motion';
	import type { TransitionConfig } from 'svelte/transition';
	import type { UsageLogRow } from '$lib/types';

	// The page answers as three sections: the request log agents write to,
	// the daemon's own log, and the boot transcripts. The address names the
	// section, so a section link survives a reload. Each section loads its
	// own rows; the page holds the toolbar row they share and the detail the
	// request log opens.
	let section = $state<LogSection>('agent');
	// sectionOrder fixes the strip order the pane swap travels along, and
	// sectionDir names the direction of the current swap for the transition.
	const sectionOrder: LogSection[] = ['agent', 'daemon', 'startup'];
	let sectionDir = $state<1 | -1>(1);
	// sectionSwapped turns the pane travel on after the first swap, so a
	// fresh visit paints still and only a section change slides.
	let sectionSwapped = $state(false);

	// The toolbar row is fixed above the panes: the filter toggle, the
	// section strip centred, and the live or refresh control. The daemon's
	// filters and live switch live here while its fields render in the pane,
	// so the strip never remounts when the section changes.
	let agentLive = $state(true);
	let agentFiltersOpen = $state(false);
	let daemonFiltersOpen = $state(false);
	let daemonLive = $state(true);
	let daemonLevel = $state<DaemonLevel>('all');
	let daemonHideRequests = $state(true);
	let startupLoading = $state(true);
	let startupRef = $state<{ refresh: () => void } | null>(null);

	// The request log's filters are page state: the toggle above the panes
	// counts what is active while the section's fields edit the values.
	let agentFilters = $state<RequestFilters>({
		provider: '',
		status: 'any',
		origin: 'any',
		key: '',
		client: ''
	});
	const activeFilters = $derived(countRequestFilters(agentFilters));
	const daemonActiveFilters = $derived(
		(daemonLevel !== 'all' ? 1 : 0) + (daemonHideRequests ? 0 : 1)
	);

	let selected = $state<UsageLogRow | null>(null);

	function chooseSection(next: LogSection) {
		if (next === section) return;
		sectionDir = tabDirection(sectionOrder, section, next);
		sectionSwapped = true;
		section = next;
		const url = new URL(window.location.href);
		url.searchParams.set('section', next);
		window.history.replaceState(null, '', url);
	}

	// The first paint mounts still; only a section change travels.
	function paneIn(node: HTMLElement): TransitionConfig {
		if (!sectionSwapped) return { duration: 0 };
		return tabPaneIn(node, { direction: sectionDir, reducedMotion: prefersReducedMotion() });
	}

	function paneOutro(node: HTMLElement): TransitionConfig {
		if (!sectionSwapped) return { duration: 0 };
		return tabPaneOut(node, { direction: sectionDir, reducedMotion: prefersReducedMotion() });
	}

	// The section switch names the view the operator is looking at, with the
	// same marks the usage tabs use for their panes.
	const sectionOptions: { value: LogSection; label: string; icon: IconName }[] = $derived([
		{
			value: 'agent',
			label: $t('ui.pages.logsPage.tabAgent'),
			icon: 'list-details'
		},
		{
			value: 'daemon',
			label: $t('ui.pages.logsPage.tabDaemon'),
			icon: 'server'
		},
		{
			value: 'startup',
			label: $t('ui.pages.logsPage.tabStartup'),
			icon: 'power'
		}
	]);

	function toggleAgentFilters() {
		agentFiltersOpen = !agentFiltersOpen;
		writePanelOpen(browserStorage(), FILTER_PANEL_KEYS.logs, agentFiltersOpen);
	}

	function toggleDaemonFilters() {
		daemonFiltersOpen = !daemonFiltersOpen;
		writePanelOpen(browserStorage(), FILTER_PANEL_KEYS.logsDaemon, daemonFiltersOpen);
	}

	onMount(() => {
		section = parseLogSection(page.url.searchParams.get('section'));
		// A deep link names a request, which lives on the agent section.
		if (page.url.searchParams.get('request')) section = 'agent';
		agentFiltersOpen = readPanelOpen(browserStorage(), FILTER_PANEL_KEYS.logs);
		daemonFiltersOpen = readPanelOpen(browserStorage(), FILTER_PANEL_KEYS.logsDaemon);
	});
</script>

<svelte:head><title>{$t('ui.pages.logs.title')} · Relo</title></svelte:head>

<PageHeader title="ui.pages.logs.title" description="ui.pages.logs.description" />

<div class="page-frame page-stack pt-6">
	<div class="flex flex-col gap-5">
		<div class="toolbar-row logs-toolbar">
			<div class="toolbar-start">
				{#if section === 'agent'}
					<FilterToggle
						open={agentFiltersOpen}
						activeCount={activeFilters}
						controls={filterPanelId(FILTER_PANEL_KEYS.logs)}
						onclick={toggleAgentFilters}
					/>
				{:else if section === 'daemon'}
					<FilterToggle
						open={daemonFiltersOpen}
						activeCount={daemonActiveFilters}
						controls={filterPanelId(FILTER_PANEL_KEYS.logsDaemon)}
						onclick={toggleDaemonFilters}
					/>
				{/if}
			</div>
			<div class="toolbar-center">
				<Segmented
					bind:value={section}
					options={sectionOptions}
					ariaLabel={$t('ui.pages.logsPage.sectionLabel')}
					onchange={chooseSection}
					class="w-full"
				/>
			</div>
			<div class="toolbar-end">
				{#if section === 'agent'}
					<LiveButton live={agentLive} ontoggle={() => (agentLive = !agentLive)} />
				{:else if section === 'daemon'}
					<LiveButton live={daemonLive} ontoggle={() => (daemonLive = !daemonLive)} />
				{:else}
					<Button
						variant="outline"
						size="sm"
						onclick={() => startupRef?.refresh()}
						disabled={startupLoading}
					>
						<Icon name={startupLoading ? 'loader' : 'refresh'} size={14} spin={startupLoading} />
						{$t('ui.common.retry')}
					</Button>
				{/if}
			</div>
		</div>
		{#key section}
			<div in:paneIn out:paneOutro>
				{#if section === 'agent'}
					<AgentSection
						filters={agentFilters}
						onfilters={(patch) => (agentFilters = { ...agentFilters, ...patch })}
						live={agentLive}
						bind:filtersOpen={agentFiltersOpen}
						onopen={(row) => (selected = row)}
					/>
				{:else if section === 'daemon'}
					<DaemonSection
						live={daemonLive}
						level={daemonLevel}
						onlevelchange={(next) => (daemonLevel = next)}
						hideRequests={daemonHideRequests}
						onhiderequestschange={(next) => (daemonHideRequests = next)}
						bind:filtersOpen={daemonFiltersOpen}
					/>
				{:else}
					<StartupSection
						loading={startupLoading}
						onloadingchange={(next) => (startupLoading = next)}
						bind:this={startupRef}
					/>
				{/if}
			</div>
		{/key}
	</div>

	<RequestDetailModal row={selected} onclose={() => (selected = null)} />
</div>
