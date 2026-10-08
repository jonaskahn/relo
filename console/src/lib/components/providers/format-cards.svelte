<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { TemplateFormatOption } from '$lib/types';

	interface Props {
		options: TemplateFormatOption[];
		value?: string;
		disabled?: boolean;
		onchange: (option: TemplateFormatOption) => void;
	}

	let { options, value = '', disabled = false, onchange }: Props = $props();

	// Every format carries one line of prose, so a choice is made on what the
	// format does rather than on its name.
	const explain: Record<string, string> = {
		'openai-chat': 'ui.pages.providersPage.format.openaiChat',
		'openai-responses': 'ui.pages.providersPage.format.openaiResponses',
		anthropic: 'ui.pages.providersPage.format.anthropic',
		'vertex-anthropic': 'ui.pages.providersPage.format.vertexAnthropic',
		gemini: 'ui.pages.providersPage.format.gemini',
		vertex: 'ui.pages.providersPage.format.vertex',
		'bedrock-converse': 'ui.pages.providersPage.format.bedrockConverse',
		kiro: 'ui.pages.providersPage.format.kiro',
		'cloud-code-assist': 'ui.pages.providersPage.format.cloudCode'
	};
</script>

<div class="grid gap-2 sm:grid-cols-2">
	{#each options as option (option.format)}
		<button
			type="button"
			{disabled}
			aria-pressed={value === option.format}
			class="flex flex-col gap-1 rounded-xl border px-3 py-2 text-left text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60 {value ===
			option.format
				? 'border-accent bg-accent-soft'
				: 'border-border hover:border-border-strong'}"
			onclick={() => onchange(option)}
		>
			<span class="font-mono text-xs font-medium">{option.format}</span>
			<span class="text-xs text-muted-foreground">
				{explain[option.format]
					? $t(explain[option.format])
					: $t('ui.pages.providersPage.format.unsupported')}
			</span>
		</button>
	{/each}
</div>
