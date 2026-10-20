<script lang="ts">
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';

	import { installGuides, type InstallGuide, type InstallMethod } from '$lib/update-install';
	import type { UpdateInstaller, UpdateNotice } from '$lib/updates';
	import Button from '$lib/components/ui/button.svelte';
	import CenteredModal from '$lib/components/ui/centered-modal.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import IconAction from '$lib/components/ui/icon-action.svelte';

	interface Props {
		open?: boolean;
		notice: UpdateNotice | null;
	}

	let { open = $bindable(false), notice }: Props = $props();

	const methodKeys: Partial<Record<UpdateInstaller, string>> = {
		shell: 'ui.updateDialog.methodShell',
		powershell: 'ui.updateDialog.methodPowerShell',
		npm: 'ui.updateDialog.methodNpm',
		brew: 'ui.updateDialog.methodBrew'
	};

	const guides = $derived(notice ? installGuides(notice.installer, notice.latest) : []);
	const methodKey = $derived(notice?.installer ? (methodKeys[notice.installer] ?? '') : '');

	let copied = $state<InstallMethod | null>(null);
	let timer: ReturnType<typeof setTimeout> | undefined;

	$effect(() => () => clearTimeout(timer));

	async function copy(guide: InstallGuide) {
		try {
			await navigator.clipboard.writeText(guide.command);
			copied = guide.method;
			clearTimeout(timer);
			timer = setTimeout(() => (copied = null), 1500);
		} catch {
			toast.error($t('ui.common.error'));
		}
	}
</script>

<CenteredModal
	bind:open
	size="compact"
	title={$t('ui.updateDialog.title')}
	description={$t('ui.updateDialog.body', {
		values: { current: notice?.current ?? '', latest: notice?.latest ?? '' }
	})}
>
	<div class="flex flex-col gap-4">
		{#if notice}
			{#if guides.length === 0}
				<p class="text-sm text-muted-foreground">
					{$t(
						notice.installer === 'sparkle' ? 'ui.updateDialog.sparkle' : 'ui.updateDialog.installer'
					)}
				</p>
			{:else}
				<p class="text-sm text-muted-foreground">
					{methodKey
						? $t('ui.updateDialog.detected', { values: { method: $t(methodKey) } })
						: $t('ui.updateDialog.unknown')}
				</p>
				{#each guides as guide (guide.method)}
					<div class="overflow-hidden rounded-panel bg-sunken">
						<div class="flex items-center gap-2 px-3 pt-1.5">
							<span class="mono-data min-w-0 flex-1 truncate text-faint">
								{$t(guide.labelKey)}
							</span>
							<IconAction
								icon={copied === guide.method ? 'check' : 'copy'}
								label={copied === guide.method ? $t('ui.common.copied') : $t('ui.common.copy')}
								onclick={() => copy(guide)}
							/>
						</div>
						<pre class="overflow-x-auto px-3 pt-1 pb-3 text-[13px] leading-6"><code
								class="font-mono">{guide.command}</code
							></pre>
					</div>
				{/each}
			{/if}
			<a
				href={notice.url}
				target="_blank"
				rel="external noreferrer"
				class="inline-flex items-center gap-1.5 text-sm text-accent-deep underline-offset-2 hover:underline"
			>
				<Icon name="external-link" size={14} />
				{$t('ui.updateDialog.releasePage')}
			</a>
		{/if}
	</div>
	{#snippet footer()}
		<div class="modal-footer">
			<Button variant="outline" onclick={() => (open = false)}>
				<Icon name="x" size={14} />
				{$t('ui.common.close')}
			</Button>
		</div>
	{/snippet}
</CenteredModal>
