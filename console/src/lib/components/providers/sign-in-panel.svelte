<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api';
	import type { LoginMethod, OAuthOperation, OAuthPortStatus } from '$lib/types';
	import Button from '$lib/components/ui/button.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import Label from '$lib/components/ui/label.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import ConfirmDialog from '$lib/components/ui/confirm-dialog.svelte';

	interface Props {
		methods: LoginMethod[];
		label?: string;
		lockLabel?: boolean;
		ondone: (operation: OAuthOperation) => void;
		oncancel?: () => void;
	}

	let {
		methods,
		label: initialLabel = '',
		// lockLabel keeps the name this panel was opened with. A sign-in for an
		// account that already exists is not a place to rename it.
		lockLabel = false,
		ondone,
		oncancel
	}: Props = $props();

	// The form opens from the methods and the label it was given, and holds
	// the operator's own edits from then on.
	let flow = $state(untrack(() => methods[0]?.flow ?? ''));
	let accountLabel = $state(untrack(() => initialLabel));
	let operation = $state<OAuthOperation | null>(null);
	let working = $state(false);
	let failed = $state('');
	let copied = $state(false);
	// portBusy is the sign-in port another process holds. A provider that
	// pinned its redirect address has no port to fall back to, so the panel
	// offers the choice to end that process before a sign-in starts.
	let portBusy = $state<OAuthPortStatus | null>(null);
	let confirmKill = $state(false);
	let killing = $state(false);

	const running = $derived(operation?.state === 'running');
	// The conflict text names the process the operator would end, read from
	// the daemon rather than guessed in the browser.
	const portConflictText = $derived(
		$t('ui.pages.providersPage.add.signInPortBusy', {
			values: { process: portBusy?.process ?? String(portBusy?.pid ?? '') }
		})
	);

	// A fixed port another process holds clears on its own when that process
	// exits, so the panel asks again while the conflict is showing.
	$effect(() => {
		if (!portBusy) return;
		const timer = setInterval(() => void checkPort(), 2000);
		return () => clearInterval(timer);
	});

	// The panel opens on the flow it was given, so the port is asked then,
	// and again when the operator picks another way in.
	$effect(() => {
		if (running) return;
		void checkPort();
	});

	// The login finishes in a browser or on another device, so the panel
	// follows the operation rather than waiting on one request.
	$effect(() => {
		if (!running) return;
		const timer = setInterval(() => void poll(), 2000);
		return () => clearInterval(timer);
	});

	async function start() {
		working = true;
		failed = '';
		try {
			const query =
				accountLabel.trim() === '' ? '' : '?label=' + encodeURIComponent(accountLabel.trim());
			const started = await api<OAuthOperation>(
				'/oauth/' + encodeURIComponent(flow) + '/start' + query,
				{ method: 'POST' }
			);
			operation = started;
			if (started.state === 'done') ondone(started);
		} catch (error) {
			failed = error instanceof Error ? error.message : String(error);
		} finally {
			working = false;
		}
	}

	async function checkPort() {
		if (flow === '') return;
		try {
			const status = await api<OAuthPortStatus>(
				'/oauth/' + encodeURIComponent(flow) + '/port-status'
			);
			portBusy = status.busy ? status : null;
		} catch {
			// A probe that fails is not a conflict; the sign-in reports its own
			// port error when it runs, so this stays silent.
			portBusy = null;
		}
	}

	async function forceSignIn() {
		if (!portBusy) return;
		killing = true;
		failed = '';
		try {
			await api('/oauth/' + encodeURIComponent(flow) + '/free-port', { method: 'POST' });
			confirmKill = false;
			portBusy = null;
			await start();
		} catch (error) {
			failed = error instanceof Error ? error.message : String(error);
			confirmKill = false;
		} finally {
			killing = false;
		}
	}

	function later() {
		portBusy = null;
		operation = null;
		oncancel?.();
	}

	async function poll() {
		if (!operation) return;
		try {
			const next = await api<OAuthOperation>(
				'/oauth/operations/' + encodeURIComponent(operation.operation_id)
			);
			operation = next;
			if (next.state === 'done') ondone(next);
			if (next.state === 'failed') failed = next.error ?? '';
		} catch (error) {
			failed = error instanceof Error ? error.message : String(error);
		}
	}

	async function copy() {
		if (!operation?.device_code) return;
		try {
			await navigator.clipboard.writeText(operation.device_code);
			copied = true;
		} catch {
			copied = false;
		}
	}
</script>

<div class="flex flex-col gap-3 rounded-lg border border-border p-3">
	{#if methods.length > 1 && !running}
		<div class="flex flex-col gap-1">
			<span class="text-xs text-muted-foreground"
				>{$t('ui.pages.providersPage.add.signInMethod')}</span
			>
			<div class="flex flex-wrap gap-2">
				{#each methods as method (method.flow)}
					<Button
						type="button"
						variant="chip"
						size="sm"
						aria-pressed={flow === method.flow}
						onclick={() => (flow = method.flow)}
					>
						{method.kind}
					</Button>
				{/each}
			</div>
		</div>
	{/if}

	{#if !running && !lockLabel}
		<div class="flex flex-col gap-1">
			<Label for="signin-account-label">{$t('ui.pages.providersPage.add.accountLabel')}</Label>
			<Input id="signin-account-label" bind:value={accountLabel} />
			<p class="text-[0.7rem] text-muted-foreground">
				{$t('ui.pages.providersPage.add.accountLabelHint')}
			</p>
		</div>
	{/if}

	{#if running}
		<div class="flex flex-col gap-2" aria-live="polite">
			<p class="text-sm text-muted-foreground">{$t('ui.pages.providersPage.add.signInRunning')}</p>
			{#if operation?.url}
				<div class="flex flex-wrap items-center gap-2">
					<a
						class="min-w-0 break-all font-mono text-xs underline"
						href={operation.url}
						target="_blank"
						rel="external noreferrer"
					>
						{operation.url}
					</a>
					<a
						class="inline-flex items-center gap-1.5 text-sm font-medium text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
						href={operation.url}
						target="_blank"
						rel="external noreferrer"
					>
						<Icon name="external-link" size={14} />
						{$t('ui.pages.providersPage.add.openLink')}
					</a>
				</div>
			{/if}
			{#if operation?.device_code}
				<div class="flex flex-wrap items-center gap-2">
					<code
						class="rounded-md border border-border px-3 py-1.5 font-mono text-lg tracking-widest"
						>{operation.device_code}</code
					>
					<Button variant="outline" size="sm" onclick={copy}>
						<Icon name="copy" size={14} />
						{copied
							? $t('ui.pages.providersPage.add.copied')
							: $t('ui.pages.providersPage.add.copyCode')}
					</Button>
				</div>
			{/if}
			{#if operation?.instructions}
				<p class="text-xs text-muted-foreground">{operation.instructions}</p>
			{/if}
			<Button variant="ghost" size="sm" class="self-start" onclick={() => (operation = null)}>
				<Icon name="x" size={14} />
				{$t('ui.pages.providersPage.add.cancelSignIn')}
			</Button>
		</div>
	{:else}
		<div class="flex flex-wrap gap-2">
			{#if portBusy}
				<div class="flex w-full flex-col gap-2">
					<p class="text-xs text-amber-600 dark:text-amber-400">{portConflictText}</p>
					<div class="flex flex-wrap gap-2">
						<Button size="sm" disabled={killing} onclick={() => (confirmKill = true)}>
							<Icon name={killing ? 'loader' : 'player-play'} size={14} spin={killing} />
							{$t('ui.pages.providersPage.add.forceSignIn')}
						</Button>
						<Button variant="ghost" size="sm" onclick={later}>
							<Icon name="x" size={14} />
							{$t('ui.pages.providersPage.add.signInLater')}
						</Button>
					</div>
				</div>
			{:else}
				<Button size="sm" disabled={working || flow === ''} onclick={start}>
					<Icon name={working ? 'loader' : 'player-play'} size={14} spin={working} />
					{$t('ui.pages.providersPage.add.startSignIn')}
				</Button>
				{#if oncancel}
					<Button variant="ghost" size="sm" onclick={oncancel}>
						<Icon name="x" size={14} />
						{$t('ui.common.cancel')}
					</Button>
				{/if}
			{/if}
		</div>
	{/if}

	{#if failed}
		<p class="text-xs text-destructive" role="alert">{failed}</p>
	{/if}
</div>

<ConfirmDialog
	bind:open={confirmKill}
	title={$t('ui.pages.providersPage.add.forceSignInTitle')}
	body={$t('ui.pages.providersPage.add.forceSignInBody', {
		values: { process: portBusy?.process ?? String(portBusy?.pid ?? ''), pid: portBusy?.pid ?? 0 }
	})}
	confirmLabel={$t('ui.pages.providersPage.add.forceSignIn')}
	tone="destructive"
	busy={killing}
	onconfirm={forceSignIn}
/>
