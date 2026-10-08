<script lang="ts">
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';

	import IconAction from '$lib/components/ui/icon-action.svelte';

	// ChatCodeBlock is one fenced answer: the text exactly as it was written,
	// under the language the fence named, with a copy that takes the whole
	// block rather than the whole reply.
	interface Props {
		code: string;
		language: string;
	}

	let { code, language }: Props = $props();

	let copied = $state(false);
	let timer: ReturnType<typeof setTimeout> | undefined;

	$effect(() => () => clearTimeout(timer));

	async function copy() {
		try {
			await navigator.clipboard.writeText(code);
			copied = true;
			clearTimeout(timer);
			timer = setTimeout(() => (copied = false), 1500);
		} catch {
			toast.error($t('ui.common.error'));
		}
	}
</script>

<div class="overflow-hidden rounded-panel bg-sunken">
	<div class="flex items-center gap-2 px-3 pt-1.5">
		<span class="mono-data min-w-0 flex-1 truncate text-faint">
			{language}
		</span>
		<IconAction
			icon={copied ? 'check' : 'copy'}
			label={copied ? $t('ui.common.copied') : $t('ui.common.copy')}
			onclick={copy}
		/>
	</div>
	<pre class="overflow-x-auto px-3 pt-1 pb-3 text-[13px] leading-6"><code class="font-mono"
			>{code}</code
		></pre>
</div>
