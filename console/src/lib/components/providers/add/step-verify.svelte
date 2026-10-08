<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { ProbeCheck, ProbeResult } from '$lib/types';
	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';
	import Button from '$lib/components/ui/button.svelte';

	// Verify reads as the path a request will take: one node per check on a
	// dashed rail, each with the state it reported and the fix when it failed.
	interface Props {
		running?: boolean;
		probe?: ProbeResult | null;
		failure?: string;
		signedInAs?: string;
		onretry?: () => void;
	}

	let { running = false, probe = null, failure = '', signedInAs = '', onretry }: Props = $props();

	type Row = {
		name: string;
		labelKey: string;
		status: 'pass' | 'fail' | 'warn' | 'skip';
		text: string;
		detail: string;
	};

	function checkOf(checks: ProbeCheck[], name: string): ProbeCheck | null {
		return checks.find((check) => check.name === name) ?? null;
	}

	function statusOf(check: ProbeCheck | null, running: boolean): 'pass' | 'fail' | 'warn' | 'skip' {
		if (running || !check) return 'skip';
		const status = check.status;
		if (status === 'pass' || status === 'fail' || status === 'warn' || status === 'skip')
			return status;
		return 'warn';
	}

	// The checklist is written in the console's own words; the daemon's detail
	// appears beside a row that needs explaining rather than as the row itself.
	const rows = $derived.by<Row[]>(() => {
		const checks = probe?.checks ?? [];
		const credential = checkOf(checks, 'credential');
		const listing = checkOf(checks, 'listing');
		const modelsdev = checkOf(checks, 'modelsdev');
		const total = probe?.counts.listed ?? 0;

		const credentialStatus = statusOf(credential, running);
		const listingStatus = statusOf(listing, running);
		const modelsdevStatus = statusOf(modelsdev, running);

		const rows: Row[] = [
			{
				name: 'credential',
				labelKey: 'ui.pages.providersPage.verify.credential',
				status: credentialStatus,
				text:
					signedInAs !== ''
						? $t('ui.pages.providersPage.verify.signedInAs', { values: { label: signedInAs } })
						: credentialStatus === 'pass'
							? $t('ui.pages.providersPage.verify.credentialAccepted')
							: credentialStatus === 'fail'
								? $t('ui.pages.providersPage.verify.credentialRejected')
								: credentialStatus === 'skip'
									? $t('ui.pages.providersPage.verify.credentialSkipped')
									: $t('ui.pages.providersPage.verify.running'),
				detail: credentialStatus === 'fail' ? (credential?.detail ?? '') : ''
			},
			{
				name: 'listing',
				labelKey: 'ui.pages.providersPage.verify.listing',
				status: listingStatus,
				text:
					listingStatus === 'pass'
						? $t('ui.pages.providersPage.verify.listingLoaded', { values: { count: total } })
						: listingStatus === 'fail'
							? $t('ui.pages.providersPage.verify.listingFailed', {
									values: { detail: listing?.detail ?? failure }
								})
							: listingStatus === 'skip'
								? $t('ui.pages.providersPage.verify.listingSkipped')
								: $t('ui.pages.providersPage.verify.running'),
				detail: ''
			},
			{
				name: 'modelsdev',
				labelKey: 'ui.pages.providersPage.verify.modelsdev',
				status: modelsdevStatus,
				text:
					modelsdevStatus === 'pass'
						? $t('ui.pages.providersPage.verify.modelsdevMatched', {
								values: { matched: probe?.counts.matched ?? 0, total }
							})
						: modelsdevStatus === 'skip'
							? $t('ui.pages.providersPage.verify.running')
							: (modelsdev?.detail ?? ''),
				detail: ''
			},
			{
				name: 'pricing',
				labelKey: 'ui.pages.providersPage.verify.pricing',
				status: running
					? 'skip'
					: total === 0
						? 'skip'
						: (probe?.counts.priced ?? 0) > 0
							? 'pass'
							: 'warn',
				text: running
					? $t('ui.pages.providersPage.verify.running')
					: $t('ui.pages.providersPage.verify.pricingPriced', {
							values: { priced: probe?.counts.priced ?? 0, total }
						}),
				detail: ''
			}
		];

		// A sign-in has no probe to check: the login verified the credential,
		// and the refresh that followed it is what reported the models, so the
		// rows describing a listing and a catalog that never ran are left out.
		if (signedInAs !== '' && probe === null) return rows.slice(0, 1);
		return rows;
	});

	const icons: Record<string, IconName> = {
		pass: 'circle-check',
		fail: 'circle-x',
		warn: 'alert-triangle',
		skip: 'info-circle'
	};

	const badgeTone: Record<string, string> = {
		pass: 'bg-accent-soft text-accent-ink',
		fail: 'bg-danger text-canvas',
		warn: 'bg-warn/15 text-warn',
		skip: 'bg-muted text-muted-foreground'
	};

	const cardTone: Record<string, string> = {
		pass: 'border-border bg-surface',
		fail: 'border-danger/40 bg-danger/5',
		warn: 'border-warn/40 bg-warn/5',
		skip: 'border-border bg-surface'
	};

	const textTone: Record<string, string> = {
		pass: 'text-ink',
		fail: 'text-danger',
		warn: 'text-warn',
		skip: 'text-muted-foreground'
	};
</script>

<div class="flex flex-col gap-3" aria-live="polite">
	<ol class="flex flex-col">
		{#each rows as row, index (row.name)}
			<li class="flex gap-3">
				<div class="flex w-6 shrink-0 flex-col items-center">
					<span
						class="flex size-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold {badgeTone[
							row.status
						]}"
					>
						{#if running}
							<Icon name="loader" size={13} spin />
						{:else}
							<Icon name={icons[row.status]} size={14} />
						{/if}
					</span>
					{#if index < rows.length - 1}
						<span
							class="w-0.5 flex-1 [background:repeating-linear-gradient(to_bottom,var(--accent-100)_0_4px,transparent_4px_8px)]"
							aria-hidden="true"
						></span>
					{/if}
				</div>
				<div class="mb-3 min-w-0 flex-1 rounded-xl border px-3 py-2.5 {cardTone[row.status]}">
					<div class="flex items-center justify-between gap-2">
						<span class="text-sm font-semibold">{$t(row.labelKey)}</span>
						<span class="flex shrink-0 items-center gap-1 text-xs {textTone[row.status]}">
							{$t('ui.pages.providersPage.verify.' + row.status)}
							<Icon name={icons[row.status]} size={13} />
						</span>
					</div>
					<p class="mt-0.5 text-sm {textTone[row.status]}">{row.text}</p>
					{#if row.detail}
						<p class="mt-1 break-words font-mono text-[0.7rem] text-muted-foreground">
							{row.detail}
						</p>
					{/if}
					{#if row.status === 'fail' && row.name === 'listing' && onretry && !running}
						<Button type="button" variant="outline" size="sm" class="mt-2" onclick={onretry}>
							<Icon name="refresh" size={14} />
							{$t('ui.pages.providersPage.verify.retry')}
						</Button>
					{/if}
				</div>
			</li>
		{/each}
	</ol>

	{#if failure && !running}
		<p class="text-xs text-destructive" role="alert">{failure}</p>
	{/if}
</div>
