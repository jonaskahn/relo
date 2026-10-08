<script lang="ts">
	import { t } from 'svelte-i18n';

	import Icon from '$lib/components/ui/icon.svelte';
	import StatusBadge from '$lib/components/ui/status-badge.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import { formatDateTime } from '$lib/format';
	import { daemonLevelTone, type DaemonLine } from '$lib/process-logs';

	// ProcessLine is one parsed daemon log line: time, severity and message on
	// the summary row, the flattened fields behind a disclosure. Severity is
	// the daemon's own word, so every language reads the same line support
	// threads match on.
	interface Props {
		line: DaemonLine;
	}

	let { line }: Props = $props();

	const tone = $derived(daemonLevelTone(line.level));

	let copied = $state(false);
	let copyTimer: ReturnType<typeof setTimeout> | undefined;

	async function copyLine() {
		try {
			await navigator.clipboard.writeText(
				[line.message, line.detail].filter((part) => part !== '').join(' — ')
			);
			copied = true;
			clearTimeout(copyTimer);
			copyTimer = setTimeout(() => (copied = false), 1500);
		} catch {
			copied = false;
		}
	}
</script>

<details class="group rounded-panel border border-line bg-surface px-3.5 py-2">
	<summary
		class="flex cursor-pointer list-none items-baseline gap-2.5 select-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring [&::-webkit-details-marker]:hidden"
	>
		<Icon
			name="chevron-right"
			size={12}
			class="shrink-0 self-center text-muted-foreground transition-transform duration-200 group-open:rotate-90"
		/>
		<span class="mono-data shrink-0 text-muted-foreground"
			>{line.timestamp_ms ? formatDateTime(line.timestamp_ms) : '—'}</span
		>
		<StatusBadge kind={tone} label={line.level} />
		<span class="min-w-0 flex-1 truncate text-sm text-ink" title={line.message}>
			{line.message || '—'}
		</span>
	</summary>
	{#if line.detail}
		<div class="flex items-start justify-between gap-3 pt-1.5 pl-5">
			<p class="mono-data min-w-0 flex-1 text-muted-foreground wrap-break-word">{line.detail}</p>
			<Button
				variant="ghost"
				size="icon"
				aria-label={$t('ui.common.copy')}
				title={$t('ui.common.copy')}
				onclick={() => void copyLine()}
			>
				<Icon name={copied ? 'check' : 'copy'} size={14} />
			</Button>
		</div>
	{/if}
</details>
