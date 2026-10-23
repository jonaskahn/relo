<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { page } from '$app/state';

	import Button from '$lib/components/ui/button.svelte';
	import Input from '$lib/components/ui/input.svelte';
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
	import { parseLogSection, type DaemonLevel, type LogSection } from '$lib/process-logs';
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

	// The toolbar stays mounted above the panes: the section switch, the
	// open section's controls, and the live or refresh control. The daemon's
	// filters live here while its fields render in the pane.
	let agentLive = $state(true);
	let daemonFiltersOpen = $state(false);
	let daemonLive = $state(true);
	let daemonLevel = $state<DaemonLevel>('all');
	let daemonHideRequests = $state(true);
	let startupLoading = $state(true);
	let startupRef = $state<{ refresh: () => void } | null>(null);

	// The request log narrows by status from the toolbar. Provider, origin
	// and key stay unset: the search box covers them over the loaded rows.
	let agentStatus = $state<'any' | 'ok' | 'error'>('any');
	let agentQuery = $state('');
	const daemonActiveFilters = $derived(
		(daemonLevel !== 'all' ? 1 : 0) + (daemonHideRequests ? 0 : 1)
	);

	let selected = $state<UsageLogRow | null>(null);
	let agentRows = $state<UsageLogRow[]>([]);

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

	function toggleDaemonFilters() {
		daemonFiltersOpen = !daemonFiltersOpen;
		writePanelOpen(browserStorage(), FILTER_PANEL_KEYS.logsDaemon, daemonFiltersOpen);
	}

	onMount(() => {
		section = parseLogSection(page.url.searchParams.get('section'));
		// A deep link names a request, which lives on the agent section.
		if (page.url.searchParams.get('request')) section = 'agent';
		daemonFiltersOpen = readPanelOpen(browserStorage(), FILTER_PANEL_KEYS.logsDaemon);
	});
</script>

<svelte:head><title>{$t('ui.pages.logs.title')} · Relo</title></svelte:head>

<PageHeader title="ui.pages.logs.title" description="ui.pages.logs.description">
	{#snippet actions()}
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
	{/snippet}
</PageHeader>

<div class="page-frame page-stack pt-6">
	<div class="flex flex-col gap-5">
		<div class="logs-toolbar flex items-center gap-2.5 max-sm:flex-wrap">
			<Segmented
				bind:value={section}
				options={sectionOptions}
				ariaLabel={$t('ui.pages.logsPage.sectionLabel')}
				onchange={chooseSection}
			/>
			<div
				class="ms-auto flex min-w-0 items-center justify-end gap-2.5 max-sm:w-full max-sm:flex-wrap"
			>
				{#if section === 'agent'}
					<div class="relative w-56 min-w-0 max-w-sm shrink max-sm:w-full">
						<span
							class="pointer-events-none absolute inset-y-0 start-3 flex items-center text-muted-foreground"
						>
							<Icon name="search" size={15} />
						</span>
						<Input
							class="h-[var(--control-h)] ps-9"
							type="search"
							bind:value={agentQuery}
							placeholder={$t('ui.pages.logsPage.searchPlaceholder')}
							aria-label={$t('ui.pages.logsPage.searchPlaceholder')}
						/>
					</div>
					<Segmented
						bind:value={agentStatus}
						options={[
							{ value: 'any', label: $t('ui.pages.logsPage.filterAllShort') },
							{ value: 'ok', label: $t('ui.pages.logsPage.filterSuccess') },
							{ value: 'error', label: $t('ui.pages.logsPage.filterErrors') }
						]}
						ariaLabel={$t('ui.pages.logsPage.filterStatus')}
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
		</div>
		{#key section}
			<div in:paneIn out:paneOutro>
				{#if section === 'agent'}
					<AgentSection
						status={agentStatus}
						query={agentQuery}
						live={agentLive}
						onopen={(row) => (selected = row)}
						onrows={(rows) => (agentRows = rows)}
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

	<RequestDetailModal
		row={selected}
		rows={agentRows}
		onclose={() => (selected = null)}
		onselect={(row) => (selected = row)}
	/>
</div>
