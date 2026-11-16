<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';

	import { api, ApiError } from '$lib/api';
	import { DESKTOP_COWORK_PATH, DESKTOP_PICKER_PATH } from '$lib/integration-actions';
	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import type { DesktopPicker } from '$lib/types';

	// Claude Desktop's own model picker. It is off by default and the one control
	// that asks the operating system to trust a certificate, so turning it on shows
	// the risk first and writes nothing until the operator confirms.
	let picker = $state<DesktopPicker | null>(null);
	let busy = $state(false);
	let failure = $state('');
	let confirming = $state(false);
	let coworkConfirming = $state(false);

	const path = DESKTOP_PICKER_PATH;

	onMount(() => {
		void load();
	});

	async function load() {
		try {
			picker = (await api(path)) as DesktopPicker;
		} catch (error) {
			failure = error instanceof Error ? error.message : String(error);
		}
	}

	async function run(request: () => Promise<unknown>) {
		busy = true;
		failure = '';
		try {
			picker = (await request()) as DesktopPicker;
			confirming = false;
		} catch (error) {
			failure = error instanceof Error ? error.message : String(error);
			// A refusal carries a reason, and where the picker stands has moved.
			await load();
		} finally {
			busy = false;
		}
	}

	function turn(enabled: boolean) {
		void run(() => api(path, { method: 'PUT', body: JSON.stringify({ enabled }) }));
	}

	function turnCowork(enabled: boolean) {
		void run(async () => {
			try {
				return await api(DESKTOP_COWORK_PATH, { method: 'PUT', body: JSON.stringify({ enabled }) });
			} catch (error) {
				// A refusal the daemon names by reason reads in the console's language.
				if (error instanceof ApiError && coworkReasons.has(error.code)) {
					throw new Error($t(coworkReasonKey(error.code)), { cause: error });
				}
				throw error;
			} finally {
				coworkConfirming = false;
			}
		});
	}

	const coworkReasons = new Set([
		'cowork_needs_picker',
		'cowork_trust_missing',
		'cowork_restart_required',
		'cowork_active'
	]);

	function coworkReasonKey(reason: string): string {
		switch (reason) {
			case 'cowork_active':
				return 'ui.pages.integrationsPage.desktopPicker.cowork.reasonActive';
			case 'cowork_restart_required':
				return 'ui.pages.integrationsPage.desktopPicker.cowork.reasonRestart';
			case 'cowork_needs_picker':
				return 'ui.pages.integrationsPage.desktopPicker.cowork.reasonNeedsPicker';
			case 'cowork_trust_missing':
				return 'ui.pages.integrationsPage.desktopPicker.cowork.reasonTrustMissing';
			default:
				return 'ui.pages.integrationsPage.desktopPicker.cowork.reasonOff';
		}
	}

	// How many connections to the vendor API the picker's proxy has been asked for, which is
	// what tells an operator whether Cowork's traffic reaches Relo at all.
	const vendorConnections = $derived(
		picker?.connections.hosts.find((entry) => entry.host === 'api.anthropic.com')?.count ?? 0
	);

	function removeTrust() {
		void run(() => api(`${path}/trust`, { method: 'DELETE' }));
	}

	function reasonKey(reason: string): string {
		switch (reason) {
			case 'active':
				return 'ui.pages.integrationsPage.desktopPicker.reasonActive';
			case 'restart_required':
				return 'ui.pages.integrationsPage.desktopPicker.reasonRestart';
			case 'unsupported_platform':
				return 'ui.pages.integrationsPage.desktopPicker.reasonUnsupported';
			case 'not_installed':
				return 'ui.pages.integrationsPage.desktopPicker.reasonNotInstalled';
			case 'needs_desktop_proxy':
			case 'gateway_selected':
				return 'ui.pages.integrationsPage.desktopPicker.reasonNeedsProxy';
			case 'trust_pending':
				return 'ui.pages.integrationsPage.desktopPicker.reasonTrustPending';
			case 'trust_declined':
				return 'ui.pages.integrationsPage.desktopPicker.reasonTrustDeclined';
			case 'trust_remains':
				return 'ui.pages.integrationsPage.desktopPicker.reasonTrustRemains';
			case 'ca_unverified':
				return 'ui.pages.integrationsPage.desktopPicker.reasonCaUnverified';
			case 'off':
				return 'ui.pages.integrationsPage.desktopPicker.reasonOff';
			default:
				return 'ui.pages.integrationsPage.desktopPicker.reasonFailed';
		}
	}

	function trustKey(trust: string): string {
		switch (trust) {
			case 'trusted':
				return 'ui.pages.integrationsPage.desktopPicker.trustTrusted';
			case 'untrusted':
				return 'ui.pages.integrationsPage.desktopPicker.trustUntrusted';
			default:
				return 'ui.pages.integrationsPage.desktopPicker.trustUnknown';
		}
	}
</script>

<section class="space-y-3 border-t border-border pt-4">
	<div class="min-w-0">
		<h3 class="text-sm font-semibold">{$t('ui.pages.integrationsPage.desktopPicker.title')}</h3>
		<p class="text-xs text-muted-foreground">
			{$t('ui.pages.integrationsPage.desktopPicker.hint')}
		</p>
	</div>

	{#if picker}
		<p class="flex items-start gap-2 text-xs text-muted-foreground" role="status">
			<Icon name={picker.enabled ? 'circle-check' : 'circle-off'} size={14} />
			<span>{$t(reasonKey(picker.reason))}</span>
		</p>

		{#if picker.supported}
			<dl class="grid gap-x-4 gap-y-1 text-xs sm:grid-cols-[auto_1fr]">
				<dt class="text-muted-foreground">
					{$t('ui.pages.integrationsPage.desktopPicker.trust')}
				</dt>
				<dd>{$t(trustKey(picker.trust))}</dd>
				{#if picker.fingerprint}
					<dt class="text-muted-foreground">
						{$t('ui.pages.integrationsPage.desktopPicker.fingerprint')}
					</dt>
					<dd class="mono-data break-all">{picker.fingerprint}</dd>
				{/if}
			</dl>

			{#if confirming}
				<div class="rounded-lg border border-warning/40 bg-warning/5 p-3">
					<p class="text-xs text-warning">
						{$t('ui.pages.integrationsPage.desktopPicker.warning')}
					</p>
					<div class="mt-2 flex flex-wrap items-center gap-2">
						<Button size="sm" disabled={busy} onclick={() => turn(true)}>
							{$t('ui.pages.integrationsPage.desktopPicker.confirm')}
						</Button>
						<Button size="sm" variant="ghost" disabled={busy} onclick={() => (confirming = false)}>
							{$t('ui.common.cancel')}
						</Button>
					</div>
				</div>
			{:else}
				<div class="flex flex-wrap items-center gap-2">
					{#if picker.enabled}
						<Button size="sm" variant="outline" disabled={busy} onclick={() => turn(false)}>
							{$t('ui.pages.integrationsPage.desktopPicker.disable')}
						</Button>
					{:else}
						<Button size="sm" disabled={busy} onclick={() => (confirming = true)}>
							{$t('ui.pages.integrationsPage.desktopPicker.enable')}
						</Button>
						{#if picker.trust === 'trusted'}
							<Button size="sm" variant="outline" disabled={busy} onclick={removeTrust}>
								{$t('ui.pages.integrationsPage.desktopPicker.removeTrust')}
							</Button>
						{/if}
					{/if}
				</div>
			{/if}
		{/if}
	{/if}

	{#if picker?.supported && picker.enabled && picker.armed}
		<div class="space-y-2 border-t border-border pt-3">
			<h4 class="text-sm font-semibold">
				{$t('ui.pages.integrationsPage.desktopPicker.cowork.title')}
			</h4>
			<p class="text-xs text-muted-foreground">
				{$t('ui.pages.integrationsPage.desktopPicker.cowork.hint')}
			</p>
			<p class="flex items-start gap-2 text-xs text-muted-foreground" role="status">
				<Icon name={picker.cowork.enabled ? 'circle-check' : 'circle-off'} size={14} />
				<span>{$t(coworkReasonKey(picker.cowork.reason))}</span>
			</p>
			<p class="text-xs text-muted-foreground">
				{$t('ui.pages.integrationsPage.desktopPicker.cowork.seen', {
					values: { count: vendorConnections }
				})}
			</p>
			{#if coworkConfirming}
				<div class="rounded-lg border border-warning/40 bg-warning/5 p-3">
					<p class="text-xs text-warning">
						{$t('ui.pages.integrationsPage.desktopPicker.cowork.warning')}
					</p>
					<div class="mt-2 flex flex-wrap items-center gap-2">
						<Button size="sm" disabled={busy} onclick={() => turnCowork(true)}>
							{$t('ui.pages.integrationsPage.desktopPicker.confirm')}
						</Button>
						<Button
							size="sm"
							variant="ghost"
							disabled={busy}
							onclick={() => (coworkConfirming = false)}
						>
							{$t('ui.common.cancel')}
						</Button>
					</div>
				</div>
			{:else if picker.cowork.enabled}
				<Button size="sm" variant="outline" disabled={busy} onclick={() => turnCowork(false)}>
					{$t('ui.pages.integrationsPage.desktopPicker.disable')}
				</Button>
			{:else}
				<Button size="sm" disabled={busy} onclick={() => (coworkConfirming = true)}>
					{$t('ui.pages.integrationsPage.desktopPicker.cowork.enable')}
				</Button>
			{/if}
		</div>
	{/if}

	{#if busy}
		<p class="flex items-center gap-2 text-xs text-muted-foreground">
			<Icon name="loader" size={14} spin />
			{$t('ui.common.saving')}
		</p>
	{:else if failure}
		<p class="text-sm text-danger" role="alert">{failure}</p>
	{/if}
</section>
