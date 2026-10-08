<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from 'svelte-i18n';

	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import StatusBadge from '$lib/components/ui/status-badge.svelte';
	import type { DoctorCheck } from '$lib/types';

	// DoctorChecks is the daemon's own report on this machine: one status row
	// per check, the ones that need attention first, with the fix named when
	// a check is failing.

	interface Props {
		checks: DoctorCheck[];
		loading?: boolean;
		failed?: string;
		pending?: Snippet;
		onretry: () => void;
	}

	let { checks, loading = false, failed = '', pending, onretry }: Props = $props();

	const ordered = $derived(
		[...checks].sort((a, b) => {
			const rank = (check: DoctorCheck) => (check.status === 'pass' ? 1 : 0);
			return rank(a) - rank(b);
		})
	);
	const issues = $derived(checks.filter((check) => check.status !== 'pass').length);
	const tone: Record<DoctorCheck['status'], 'pass' | 'warn' | 'fail'> = {
		pass: 'pass',
		warn: 'warn',
		fail: 'fail'
	};
</script>

<div class="section-surface">
	<div class="flex items-center justify-between gap-3 px-4 pt-4 pb-3 sm:px-5">
		<div>
			<h2 class="section-heading flex items-center gap-2">
				{$t('ui.pages.dashboard.doctorTitle')}
				{#if !loading && issues > 0}
					<span
						class="inline-flex items-center gap-1 rounded-full bg-warn/10 px-2 py-0.5 text-[0.7rem] font-medium text-warn"
					>
						<Icon name="alert-triangle" size={11} />
						{$t('ui.pages.dashboard.doctorIssues', { values: { count: issues } })}
					</span>
				{/if}
			</h2>
			<p class="text-xs text-muted-foreground">
				{$t('ui.pages.dashboard.doctorDescription')}
			</p>
		</div>
	</div>
	<div class="flex flex-col gap-1 px-4 pb-4 sm:px-5">
		{#if loading}
			{@render pending?.()}
		{:else if failed}
			<div class="flex items-center justify-between gap-3">
				<p class="text-sm text-danger">{failed}</p>
				<Button variant="outline" size="sm" onclick={onretry}>
					<Icon name="refresh" size={14} />
					{$t('ui.common.retry')}
				</Button>
			</div>
		{:else if checks.length > 0}
			{#each ordered as check (check.name)}
				<div
					class="flex items-start justify-between gap-3 rounded-panel px-3 py-2 {check.status ===
					'pass'
						? ''
						: check.status === 'warn'
							? 'bg-warn/5'
							: 'bg-danger/5'}"
				>
					<div class="min-w-0">
						<p class="text-sm font-medium text-ink">{check.name}</p>
						<p class="text-xs text-muted-foreground">{check.detail}</p>
					</div>
					<StatusBadge kind={tone[check.status]} />
				</div>
			{/each}
		{/if}
	</div>
</div>
