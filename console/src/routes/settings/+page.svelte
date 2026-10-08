<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { t } from 'svelte-i18n';

	import { api } from '$lib/api';
	import Icon from '$lib/components/ui/icon.svelte';
	import PageHeader from '$lib/components/ui/page-header.svelte';
	import AccessCard from '$lib/components/settings/access-card.svelte';
	import ConsoleCard from '$lib/components/settings/console-card.svelte';
	import DaemonCard from '$lib/components/settings/daemon-card.svelte';
	import NetworkCard from '$lib/components/settings/network-card.svelte';
	import ProvidersCard from '$lib/components/settings/providers-card.svelte';
	import RestartBanner from '$lib/components/settings/restart-banner.svelte';
	import RetentionCard from '$lib/components/settings/retention-card.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import { consoleState } from '$lib/console-state.svelte';
	import { savedLanguage } from '$lib/i18n';
	import { stepSwap } from '$lib/modal-motion';
	import type {
		AccessSettings,
		ProvidersSettings,
		RetentionSettings,
		ServerSettings,
		Settings,
		SystemSettings
	} from '$lib/types';

	// The settings page reads every daemon choice once and hands each card
	// its own group. A card saves only its group; the page then re-reads the
	// one marker a save elsewhere can move: whether a daemon restart is
	// waiting.
	let settings = $state<Settings | null>(null);
	let restartPending = $state(false);
	let loadState = $state<'loading' | 'ready' | 'error'>('loading');
	let loadSeq = 0;
	let alive = true;

	onMount(() => {
		void loadDaemonSettings();
	});

	onDestroy(() => {
		alive = false;
	});

	async function loadDaemonSettings() {
		const seq = ++loadSeq;
		const appearanceRevision = consoleState.appearanceRevision;
		loadState = 'loading';
		try {
			const data = await api<Settings>('/settings');
			if (!alive || seq !== loadSeq) return;
			consoleState.useAppearance(data.appearance, appearanceRevision);
			savedLanguage.set(data.language ?? 'auto');
			restartPending = data.restart_pending;
			settings = data;
			loadState = 'ready';
		} catch {
			if (!alive || seq !== loadSeq) return;
			loadState = 'error';
		}
	}

	async function readRestartMarker() {
		if (!alive) return;
		try {
			const fresh = await api<Settings>('/settings');
			if (alive) restartPending = fresh.restart_pending;
		} catch {
			// The save is stored; the marker catches up on the next read.
		}
	}

	function onAccessSaved(next: AccessSettings) {
		if (!settings) return;
		settings = { ...settings, access: next };
		void readRestartMarker();
	}

	function onNetworkSaved(next: ServerSettings) {
		if (!settings) return;
		settings = { ...settings, server: next };
		void readRestartMarker();
	}

	function onProvidersSaved(next: ProvidersSettings) {
		if (!settings) return;
		settings = { ...settings, providers: next };
		void readRestartMarker();
	}

	function onSystemSaved(next: SystemSettings) {
		if (!settings) return;
		settings = { ...settings, system: next };
		void readRestartMarker();
	}

	function onRetentionSaved(next: RetentionSettings) {
		if (!settings) return;
		settings = { ...settings, retention: next };
	}
</script>

<svelte:head><title>{$t('ui.pages.settings.title')} · Relo</title></svelte:head>

<PageHeader title="ui.pages.settings.title" description="ui.pages.settings.description" />

<div class="page-frame page-stack pt-6">
	{#if loadState === 'error'}
		<div class="section-surface flat-surface flex flex-wrap items-center gap-3 p-5">
			<Icon name="circle-alert" size={16} class="shrink-0 text-destructive" />
			<p class="text-sm text-muted-foreground">{$t('ui.settingsPage.loadError')}</p>
			<Button type="button" variant="outline" size="sm" onclick={() => void loadDaemonSettings()}>
				<Icon name="refresh" size={14} />
				{$t('ui.common.retry')}
			</Button>
		</div>
	{:else if settings && loadState === 'ready'}
		{#if restartPending}
			<div in:stepSwap><RestartBanner /></div>
		{/if}
		<!-- One column below xl: the paired cards hold port and URL rows that
		     need the width. From xl the small cards pair up and the sections
		     with many settings keep a full row. -->
		<div class="grid grid-cols-1 gap-[var(--section)] xl:grid-cols-2">
			<div class="xl:col-span-2"><ConsoleCard /></div>
			<AccessCard access={settings.access} secrets={settings.secrets} onsaved={onAccessSaved} />
			<NetworkCard value={settings.server} onsaved={onNetworkSaved} />
			<DaemonCard value={settings.system} onsaved={onSystemSaved} />
			<div class="xl:col-span-2">
				<ProvidersCard value={settings.providers} onsaved={onProvidersSaved} />
			</div>
			<div class="xl:col-span-2">
				<RetentionCard value={settings.retention} onsaved={onRetentionSaved} />
			</div>
		</div>
	{:else}
		<div
			class="section-surface flat-surface flex min-h-48 items-center justify-center"
			role="status"
		>
			<Icon name="loader" size={20} spin class="text-[var(--accent)]" />
			<span class="sr-only">{$t('ui.common.loading')}</span>
		</div>
	{/if}
</div>
