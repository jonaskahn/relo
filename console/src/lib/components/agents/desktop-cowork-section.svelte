<script lang="ts">
	import { t } from 'svelte-i18n';

	import { api, ApiError } from '$lib/api';
	import {
		buildAgentPrompt,
		COWORK_REASONS,
		coworkManualMode,
		coworkReasonKey
	} from '$lib/cowork-trust';
	import { DESKTOP_COWORK_PATH, DESKTOP_COWORK_TRUST_PATH } from '$lib/integration-actions';
	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import type { DesktopPicker } from '$lib/types';

	// Route Cowork: hands Cowork's virtual machine's api.anthropic.com traffic to Relo's
	// intercept. It needs the operating system to trust Relo's certificate system-wide, so
	// turning it on shows the risk first, the trust stays until the operator removes it, and
	// when Relo cannot install or remove it the operator gets the steps to do it themselves.
	interface Props {
		picker: DesktopPicker;
		onupdate: (next: DesktopPicker) => void;
		onreload: () => Promise<void>;
	}

	let { picker, onupdate, onreload }: Props = $props();

	let busy = $state(false);
	let failure = $state('');
	let confirming = $state(false);
	let removeFailed = $state(false);
	let copied = $state('');

	const cowork = $derived(picker.cowork);
	const canRoute = $derived(picker.enabled && picker.armed);
	const trustPresent = $derived(cowork.trust === 'trusted');
	const manualMode = $derived(coworkManualMode(cowork.reason, removeFailed));
	// How many connections to the vendor API the picker's proxy has been asked for, which is
	// what tells an operator whether Cowork's traffic reaches Relo at all.
	const vendorConnections = $derived(
		picker.connections.hosts.find((entry) => entry.host === 'api.anthropic.com')?.count ?? 0
	);

	async function run(request: () => Promise<unknown>, onrefused?: (code: string) => void) {
		busy = true;
		failure = '';
		try {
			onupdate((await request()) as DesktopPicker);
			confirming = false;
		} catch (error) {
			failure = refusalText(error);
			if (error instanceof ApiError) onrefused?.(error.code);
			// A refusal carries a reason, and where the picker stands has moved.
			await onreload();
		} finally {
			busy = false;
		}
	}

	// A refusal the daemon names by reason reads in the console's language.
	function refusalText(error: unknown): string {
		if (error instanceof ApiError && COWORK_REASONS.has(error.code)) {
			return $t(coworkReasonKey(error.code));
		}
		return error instanceof Error ? error.message : String(error);
	}

	function turn(enabled: boolean) {
		void run(() => api(DESKTOP_COWORK_PATH, { method: 'PUT', body: JSON.stringify({ enabled }) }));
	}

	function removeTrust() {
		void run(
			() => api(DESKTOP_COWORK_TRUST_PATH, { method: 'DELETE' }),
			(code) => (removeFailed = code === 'cowork_trust_remains')
		);
	}

	// Reading again changes nothing. It is how Relo notices a certificate the operator installed
	// or removed by hand.
	async function checkAgain() {
		busy = true;
		await onreload();
		if (picker.cowork.trust !== 'trusted') removeFailed = false;
		busy = false;
	}

	async function copy(text: string, id: string) {
		try {
			await navigator.clipboard.writeText(text);
			copied = id;
			setTimeout(() => (copied = copied === id ? '' : copied), 2000);
		} catch {
			// The text is on screen to select by hand.
		}
	}

	const format = (key: string, values: Record<string, string>) => $t(key, { values });
</script>

<div class="space-y-2 rounded-lg border border-border p-3">
	<h4 class="text-sm font-semibold">
		{$t('ui.pages.integrationsPage.desktopPicker.cowork.title')}
	</h4>
	<p class="text-xs text-muted-foreground">
		{$t('ui.pages.integrationsPage.desktopPicker.cowork.hint')}
	</p>
	<p class="flex items-start gap-2 text-xs text-muted-foreground" role="status">
		<Icon name={cowork.enabled ? 'circle-check' : 'circle-off'} size={14} />
		<span>{$t(coworkReasonKey(cowork.reason))}</span>
	</p>
	{#if canRoute}
		<p class="text-xs text-muted-foreground">
			{$t('ui.pages.integrationsPage.desktopPicker.cowork.seen', {
				values: { count: vendorConnections }
			})}
		</p>
	{/if}
	{#if cowork.trust}
		<dl class="grid gap-x-4 gap-y-1 text-xs sm:grid-cols-[auto_1fr]">
			<dt class="text-muted-foreground">
				{$t('ui.pages.integrationsPage.desktopPicker.cowork.systemTrust')}
			</dt>
			<dd>
				{$t(
					trustPresent
						? 'ui.pages.integrationsPage.desktopPicker.trustTrusted'
						: cowork.trust === 'untrusted'
							? 'ui.pages.integrationsPage.desktopPicker.trustUntrusted'
							: 'ui.pages.integrationsPage.desktopPicker.trustUnknown'
				)}
			</dd>
		</dl>
	{/if}

	{#if confirming}
		<div class="rounded-lg border border-warning/40 bg-warning/5 p-3">
			<p class="text-xs text-warning">
				{$t('ui.pages.integrationsPage.desktopPicker.cowork.warning')}
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
			{#if cowork.enabled}
				<Button size="sm" variant="outline" disabled={busy} onclick={() => turn(false)}>
					{$t('ui.pages.integrationsPage.desktopPicker.disable')}
				</Button>
			{:else if canRoute}
				<Button size="sm" disabled={busy} onclick={() => (confirming = true)}>
					{$t('ui.pages.integrationsPage.desktopPicker.cowork.enable')}
				</Button>
			{/if}
			{#if trustPresent && !cowork.enabled}
				<Button size="sm" variant="outline" disabled={busy} onclick={removeTrust}>
					{$t('ui.pages.integrationsPage.desktopPicker.cowork.removeTrust')}
				</Button>
			{/if}
		</div>
	{/if}

	{#if manualMode && cowork.manual}
		{@const manual = cowork.manual}
		<div class="space-y-2 rounded-lg border border-border bg-sunken p-3">
			<h5 class="text-xs font-semibold">
				{$t('ui.pages.integrationsPage.desktopPicker.cowork.manualTitle')}
			</h5>
			<p class="text-xs text-muted-foreground">
				{$t(
					manualMode === 'install'
						? 'ui.pages.integrationsPage.desktopPicker.cowork.manualInstallIntro'
						: 'ui.pages.integrationsPage.desktopPicker.cowork.manualRemoveIntro'
				)}
			</p>
			<dl class="grid gap-x-4 gap-y-1 text-xs sm:grid-cols-[auto_1fr]">
				<dt class="text-muted-foreground">
					{$t('ui.pages.integrationsPage.desktopPicker.cowork.manualPath')}
				</dt>
				<dd class="mono-data break-all">{manual.cert_path}</dd>
				<dt class="text-muted-foreground">
					{$t('ui.pages.integrationsPage.desktopPicker.fingerprint')}
				</dt>
				<dd class="mono-data break-all">{manual.sha256}</dd>
			</dl>
			<pre class="mono-data overflow-x-auto whitespace-pre-wrap break-all text-xs">{manualMode ===
				'install'
					? manual.install
					: manual.remove}</pre>
			<div class="flex flex-wrap items-center gap-2">
				<Button
					size="sm"
					variant="outline"
					onclick={() =>
						void copy(manualMode === 'install' ? manual.install : manual.remove, 'commands')}
				>
					{copied === 'commands' ? $t('ui.common.copied') : $t('ui.common.copy')}
				</Button>
				<Button
					size="sm"
					variant="outline"
					onclick={() => void copy(buildAgentPrompt(format, manual, manualMode), 'prompt')}
				>
					{copied === 'prompt'
						? $t('ui.common.copied')
						: $t('ui.pages.integrationsPage.desktopPicker.cowork.manualCopyPrompt')}
				</Button>
				<a
					class="text-xs text-accent-strong underline"
					href={manual.docs_url}
					target="_blank"
					rel="noopener noreferrer"
				>
					{$t('ui.pages.integrationsPage.desktopPicker.cowork.manualGuide')}
				</a>
				<Button size="sm" variant="ghost" disabled={busy} onclick={() => void checkAgain()}>
					{$t('ui.pages.integrationsPage.desktopPicker.cowork.manualCheckAgain')}
				</Button>
			</div>
		</div>
	{/if}

	{#if failure}
		<p class="text-sm text-danger" role="alert">{failure}</p>
	{/if}
</div>
