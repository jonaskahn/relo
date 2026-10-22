<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';

	import { api } from '$lib/api';
	import { daemonDialog } from '$lib/daemon-dialog.svelte';
	import { subscribe } from '$lib/sse';
	import { latestBetaVersion, updateAvailable, type UpdateNotice } from '$lib/updates';
	import { cn } from '$lib/utils';
	import UpdateDialog from '$lib/components/shell/update-dialog.svelte';

	interface Props {
		class?: string;
	}

	let { class: className = '' }: Props = $props();

	let up = $state<boolean | null>(null);
	let version = $state('');
	let notice = $state<UpdateNotice | null>(null);
	let updateOpen = $state(false);
	// A beta build always names the newest beta the catalog carries, so the
	// dashboard shows what the next check would offer even when the running
	// build is already the newest one.
	const latestBeta = $derived(latestBetaVersion(version, notice));

	// A poll still waiting for its answers is the same question the timer
	// would ask again, so the tick that finds one running skips it. That also
	// keeps a daemon that is not answering from stacking requests forever.
	let polling = false;

	async function poll() {
		if (polling) return;
		polling = true;
		try {
			try {
				const status = await api<{ status: string; version: string }>('/status');
				up = status.status === 'running';
				version = status.version;
			} catch {
				up = false;
			}
			try {
				notice = await api<UpdateNotice>('/updates');
			} catch {
				notice = null;
			}
		} finally {
			polling = false;
		}
	}

	onMount(() => {
		void poll();
		const timer = setInterval(poll, 30_000);
		const close = subscribe('status', () => void poll());
		return () => {
			clearInterval(timer);
			close();
		};
	});

	// The pill is one shape in every state: surface fill, a full hairline
	// border and the same corners as the buttons beside it. A stopped daemon
	// turns it into the button that opens the manage dialog, which is the
	// next action it offers.
	const pill =
		'inline-flex h-control items-center gap-2 rounded-control border border-line bg-surface px-3 text-[0.8125rem] font-medium text-ink';
</script>

{#if up === false}
	<button
		type="button"
		data-plain
		class={cn(pill, 'transition-colors hover:bg-hover', className)}
		aria-label={$t('ui.sidebar.manage')}
		title={$t('ui.sidebar.manage')}
		onclick={() => daemonDialog.show()}
	>
		<span class="size-2 shrink-0 rounded-full bg-faint" aria-hidden="true"></span>
		{$t('ui.topbar.health')}
		{$t('ui.topbar.healthDown')}
	</button>
{:else}
	<span class={cn(pill, className)} role="status">
		<span
			aria-hidden="true"
			class={cn('size-2 shrink-0 rounded-full', up === true ? 'daemon-dot bg-ok' : 'bg-faint')}
		></span>
		<span class={up === true ? 'text-ink' : 'text-muted-foreground'}>
			{$t('ui.topbar.health')}
			{up === true ? $t('ui.topbar.healthRunning') : '…'}
		</span>
		{#if version && up}
			<span class="mono-data text-muted-foreground">{version}</span>
		{/if}
		{#if latestBeta}
			<span class="mono-data text-muted-foreground">
				{$t('ui.topbar.latestBeta', { values: { version: latestBeta } })}
			</span>
		{/if}
		{#if updateAvailable(notice)}
			<button
				type="button"
				data-plain
				class="text-accent-deep underline-offset-2 hover:underline"
				aria-haspopup="dialog"
				onclick={() => (updateOpen = true)}
			>
				{$t('ui.topbar.updateAvailable')}
				{#if notice?.latest}
					<span class="mono-data opacity-80">{notice.latest}</span>
				{/if}
			</button>
		{/if}
	</span>
{/if}

<UpdateDialog bind:open={updateOpen} {notice} />

<style>
	.daemon-dot {
		animation: daemon-pulse 2.4s ease-out infinite;
	}

	@keyframes daemon-pulse {
		0% {
			box-shadow: 0 0 0 0 color-mix(in srgb, var(--ok) 45%, transparent);
		}
		70% {
			box-shadow: 0 0 0 7px transparent;
		}
		100% {
			box-shadow: 0 0 0 0 transparent;
		}
	}
</style>
