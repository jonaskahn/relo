<script lang="ts">
	import { t } from 'svelte-i18n';

	import Icon from '$lib/components/ui/icon.svelte';
	import { Popover } from 'bits-ui';
	import PopoverPanel from '$lib/components/ui/popover-panel.svelte';
	import { cn } from '$lib/utils';
	import type { Model } from '$lib/types';
	import {
		CONTEXT_PRESETS,
		CONTEXT_SOURCE_KEY,
		contextLabel,
		contextMark,
		hasContextOverride
	} from '$lib/context-window';
	import { formatTokens } from '$lib/format';

	// ContextPicker is the Context cell of a model row, and the Max Output cell
	// beside it: the value the agent will see, and the quiet trigger under it
	// that pins one of the sizes an operator reaches for. A size of their own
	// is written in the model's own panel.
	interface Props {
		model: Model;
		onsave: (value: number | null) => Promise<void>;
		disabled?: boolean;
		field?: 'context' | 'output';
	}

	let { model, onsave, disabled = false, field = 'context' }: Props = $props();

	let open = $state(false);
	let busy = $state(false);
	let error = $state('');

	const output = $derived(field === 'output');
	const layers = $derived(
		(output ? model.max_output_layers : model.context_layers) ?? {
			override: null,
			provider: null,
			modelsdev: null
		}
	);
	const source = $derived(output ? model.max_output_source : model.context_source);
	const current = $derived(output ? model.max_output : model.context_window);
	const custom = $derived(hasContextOverride(layers));
	const label = $derived(contextLabel(current) || $t('ui.pages.providersPage.context.unset'));
	// The cell is read, not changed, on hover: one character says where the
	// value came from, and a context above the model's own maximum input is
	// marked rather than hidden, because the daemon stores it either way.
	const mark = $derived(contextMark(layers, output ? null : model.max_input));
	const markTitle = $derived.by(() => {
		if (mark === '!') {
			return $t('ui.pages.providersPage.context.overMaxTitle', {
				values: { value: formatTokens(model.max_input ?? 0) }
			});
		}
		if (mark === '*') {
			return $t('ui.pages.providersPage.context.customTitle', { values: { value: label } });
		}
		return '';
	});

	// sourceKey names the layer the effective value came from, so the popover
	// opens with what the agent will see and where it came from.
	const sourceKey = $derived.by(() => {
		const key = CONTEXT_SOURCE_KEY[source ?? ''];
		return key && !key.endsWith('sourceNone') ? key : '';
	});
	const titleKey = $derived(
		output ? 'ui.pages.providersPage.maxOutput.title' : 'ui.pages.providersPage.context.title'
	);
	const editKey = $derived(
		output ? 'ui.pages.providersPage.maxOutput.edit' : 'ui.pages.providersPage.context.edit'
	);
	const quickKey = $derived(
		output ? 'ui.pages.providersPage.maxOutput.quickSet' : 'ui.pages.providersPage.context.quickSet'
	);

	async function save(value: number | null) {
		busy = true;
		error = '';
		try {
			await onsave(value);
			open = false;
		} catch (failure) {
			// A size the daemon refused keeps the popover open with its reason,
			// so the click reads rather than disappears.
			error = failure instanceof Error ? failure.message : String(failure);
		} finally {
			busy = false;
		}
	}
</script>

<div class="flex flex-col items-start gap-0.5">
	<!-- A fixed box, so a long custom size cannot widen the column, with the
	     mark's own character reserved so the value never moves. -->
	<span class="inline-flex shrink-0 items-center font-mono text-xs {disabled ? 'opacity-60' : ''}">
		<span class="w-[7ch] min-w-0 truncate" title={label}>{label}</span>
		<span
			class="w-[1ch] shrink-0 text-center font-semibold text-accent-strong"
			title={markTitle}
			aria-hidden="true">{mark}</span
		>
	</span>
	<Popover.Root bind:open>
		<Popover.Trigger
			{disabled}
			class={cn(
				'inline-flex size-6 shrink-0 items-center justify-center rounded-md text-faint transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
				disabled ? 'cursor-not-allowed opacity-60' : 'hover:bg-muted hover:text-ink'
			)}
			aria-label={$t(editKey, { values: { value: label } })}
			title={$t(editKey, { values: { value: label } })}
		>
			<Icon name="dots" size={15} />
		</Popover.Trigger>
		<!-- Portaled: the model list scrolls on its own, so a popover opened
		     from a row near its end would be cut off inside it. -->
		<Popover.Portal>
			<PopoverPanel class="z-50 w-72 surface-pop p-3" align="start" sideOffset={6}>
				<p class="mb-2 text-xs font-semibold">{$t(titleKey)}</p>
				<p
					class="mb-2 flex items-center justify-between gap-2 rounded-md bg-muted/60 px-2 py-1.5 text-xs"
					role="status"
				>
					<span>{$t('ui.pages.providersPage.context.currentValue')}</span>
					<span class="font-mono">
						{label}{#if sourceKey}<span class="text-muted-foreground"> · {$t(sourceKey)}</span>{/if}
					</span>
				</p>

				<button
					type="button"
					data-plain
					disabled={busy}
					class="flex w-full items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left text-xs hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring {!custom
						? 'bg-accent-soft text-foreground'
						: ''}"
					onclick={() => void save(null)}
				>
					<span>{$t('ui.pages.providersPage.context.sourceDefault')}</span>
					<span class="font-mono text-muted-foreground">
						{contextLabel(layers.provider ?? layers.modelsdev) || '—'}
					</span>
				</button>

				<p class="mt-3 mb-1.5 text-xs font-semibold">
					{$t('ui.pages.providersPage.context.presets')}
				</p>
				<div class="flex flex-wrap gap-1.5">
					{#each CONTEXT_PRESETS as preset (preset)}
						<button
							type="button"
							data-plain
							disabled={busy}
							class="badge border border-border font-mono text-muted-foreground hover:border-border-strong hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring {layers.override ===
							preset
								? 'border-accent bg-accent-soft text-foreground'
								: ''}"
							aria-label={$t(quickKey, {
								values: { value: contextLabel(preset) }
							})}
							onclick={() => void save(preset)}
						>
							{contextLabel(preset)}
						</button>
					{/each}
				</div>

				{#if error}
					<p class="mt-2 flex items-start gap-1.5 text-xs text-destructive" role="alert">
						<Icon name="alert-triangle" size={13} class="mt-0.5 shrink-0" />
						<span>{error}</span>
					</p>
				{/if}
			</PopoverPanel>
		</Popover.Portal>
	</Popover.Root>
</div>
