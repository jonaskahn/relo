<script lang="ts">
	import { t } from 'svelte-i18n';

	import Icon from '$lib/components/ui/icon.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import { redactHeaders } from '$lib/capture-redact';
	import { formatBytes } from '$lib/format';
	import type { UsageCapture } from '$lib/types';

	// One captured message: the line that identifies it, its headers, and the
	// body. The body is read only when an operator asks for it, because a
	// captured body can be tens of megabytes.
	interface Props {
		requestId: number;
		capture: UsageCapture;
	}

	let { requestId, capture }: Props = $props();

	const PREVIEW_BYTES = 20_000;

	// phase names where the body read stands; it is not called state, which
	// is the rune that builds one.
	let phase = $state<'idle' | 'loading' | 'ready' | 'error'>('idle');
	let body = $state('');
	let binary = $state(false);
	let failure = $state('');
	let copied = $state(false);
	let copyTimer: ReturnType<typeof setTimeout> | undefined;
	let open = $state(false);

	// A capture stored before the daemon masked credentials, or by an older
	// daemon, still holds the key a client presented: the console masks the
	// values again before it draws them.
	const headers = $derived(
		Object.entries(redactHeaders(capture.headers ?? {})).sort((a, b) => a[0].localeCompare(b[0]))
	);
	// A response is named by the status it came back with and a request by the
	// method it was sent with; the label beside them says which half of the
	// exchange the message is.
	const response = $derived(
		capture.kind === 'agent_response' || capture.kind === 'provider_response'
	);
	const label = $derived(
		capture.kind === 'agent_request'
			? $t('ui.pages.logsPage.captureAgentRequest')
			: capture.kind === 'agent_response'
				? $t('ui.pages.logsPage.captureAgentResponse')
				: capture.kind === 'provider_request'
					? $t('ui.pages.logsPage.captureProviderRequest')
					: $t('ui.pages.logsPage.captureProviderResponse')
	);
	const summary = $derived(response ? String(capture.status) : capture.method);
	const preview = $derived(body.length > PREVIEW_BYTES ? body.slice(0, PREVIEW_BYTES) : body);
	const previewClipped = $derived(body.length > PREVIEW_BYTES);

	async function load() {
		if (phase === 'loading' || phase === 'ready') return;
		phase = 'loading';
		failure = '';
		try {
			const response = await fetch(
				'/api/v1/activity/requests/' + requestId + '/captures/' + capture.id + '/body',
				{ credentials: 'same-origin', headers: { Accept: 'application/octet-stream' } }
			);
			if (!response.ok) {
				throw new Error(
					$t('ui.pages.logsPage.captureFailed', { values: { status: response.status } })
				);
			}
			const buffer = await response.arrayBuffer();
			const bytes = new Uint8Array(buffer);
			binary = looksBinary(bytes);
			body = binary ? '' : new TextDecoder().decode(bytes);
			phase = 'ready';
		} catch (error) {
			failure = error instanceof Error ? error.message : String(error);
			phase = 'error';
		}
	}

	// A NUL byte is what tells an image or a compressed frame apart from text
	// worth rendering.
	function looksBinary(bytes: Uint8Array): boolean {
		const sample = bytes.subarray(0, 4096);
		for (const byte of sample) {
			if (byte === 0) return true;
		}
		return false;
	}

	async function copy() {
		if (phase !== 'ready') return;
		try {
			await navigator.clipboard.writeText(body);
			copied = true;
			clearTimeout(copyTimer);
			copyTimer = setTimeout(() => (copied = false), 1500);
		} catch {
			copied = false;
		}
	}

	// The body is re-read as a blob, so one too large to keep in memory as text
	// still arrives whole.
	async function download() {
		try {
			const response = await fetch(
				'/api/v1/activity/requests/' + requestId + '/captures/' + capture.id + '/body',
				{ credentials: 'same-origin' }
			);
			if (!response.ok) return;
			const blob = await response.blob();
			const url = URL.createObjectURL(blob);
			const link = document.createElement('a');
			link.href = url;
			link.download = captureName();
			link.click();
			URL.revokeObjectURL(url);
		} catch {
			// A refused download needs no message: the browser reported it.
		}
	}

	function captureName(): string {
		const kind = response ? 'reply' : 'request';
		const ordinal = capture.ordinal === undefined ? '' : '-attempt-' + (capture.ordinal + 1);
		return 'relo-' + kind + ordinal + '-request-' + requestId + '.body';
	}
</script>

<div class="rounded-lg border border-border">
	<button
		type="button"
		class="flex w-full items-center gap-2 px-3 py-2 text-left"
		aria-expanded={open}
		onclick={() => {
			open = !open;
			if (open) void load();
		}}
	>
		<Icon
			name="chevron-right"
			size={13}
			class="shrink-0 transition-transform {open ? 'rotate-90' : ''}"
		/>
		<span class="text-xs font-medium">{label}</span>
		<span class="mono-data text-muted-foreground shrink-0 text-[11px]">{summary}</span>
		<span class="mono-data min-w-0 flex-1 truncate text-xs text-muted-foreground"
			>{capture.url}</span
		>
		<span class="text-muted-foreground shrink-0 text-[11px] tabular-nums"
			>{formatBytes(capture.body_bytes)}</span
		>
	</button>

	{#if open}
		<div class="space-y-3 border-t border-border px-3 py-3">
			{#if capture.truncated}
				<p class="rounded-md border border-amber-500/30 bg-amber-500/10 px-2 py-1 text-xs">
					{$t('ui.pages.logsPage.captureTruncated')}
				</p>
			{/if}

			{#if headers.length > 0}
				<div>
					<p
						class="pb-1 text-[11px] font-semibold uppercase tracking-[0.08em] text-muted-foreground"
					>
						{$t('ui.pages.logsPage.captureHeaders')}
					</p>
					<dl class="mono-data max-h-40 space-y-0.5 overflow-y-auto text-xs">
						{#each headers as [name, values] (name)}
							<div class="flex gap-2">
								<dt class="text-muted-foreground shrink-0">{name}</dt>
								<dd class="min-w-0 break-all">{values.join(', ')}</dd>
							</div>
						{/each}
					</dl>
				</div>
			{/if}

			<div class="flex flex-wrap items-center justify-between gap-2">
				<p class="text-[11px] font-semibold uppercase tracking-[0.08em] text-muted-foreground">
					{$t('ui.pages.logsPage.captureBody')}
				</p>
				<div class="flex items-center gap-1">
					<Button
						variant="ghost"
						size="sm"
						onclick={() => void copy()}
						disabled={phase !== 'ready' || binary}
					>
						<Icon name={copied ? 'check' : 'copy'} size={14} />
						<span class="ml-1">{$t('ui.pages.logsPage.captureCopy')}</span>
					</Button>
					<Button
						variant="ghost"
						size="sm"
						onclick={() => void download()}
						disabled={capture.body_bytes === 0}
					>
						<Icon name="arrow-down" size={14} />
						<span class="ml-1">{$t('ui.pages.logsPage.captureDownload')}</span>
					</Button>
				</div>
			</div>

			{#if phase === 'loading'}
				<p class="text-muted-foreground text-sm">{$t('ui.common.loading')}</p>
			{:else if phase === 'error'}
				<p class="text-destructive text-sm" role="alert">{failure}</p>
			{:else if capture.body_bytes === 0}
				<p class="text-muted-foreground text-sm">{$t('ui.pages.logsPage.captureEmptyBody')}</p>
			{:else if binary}
				<p class="text-muted-foreground text-sm">
					{$t('ui.pages.logsPage.captureBinary', {
						values: { size: formatBytes(capture.body_bytes) }
					})}
				</p>
			{:else}
				<pre
					class="mono-data max-h-80 overflow-auto rounded-lg bg-muted p-3 text-xs leading-relaxed"><code
						>{preview}</code
					></pre>
				{#if previewClipped}
					<p class="text-muted-foreground text-xs">
						{$t('ui.pages.logsPage.capturePreviewClipped', {
							values: { shown: formatBytes(PREVIEW_BYTES), size: formatBytes(capture.body_bytes) }
						})}
					</p>
				{/if}
			{/if}
		</div>
	{/if}
</div>
