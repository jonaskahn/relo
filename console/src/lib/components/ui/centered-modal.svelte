<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { Snippet } from 'svelte';
	import { onMount, tick } from 'svelte';
	import { fade } from 'svelte/transition';
	import { Dialog } from 'bits-ui';
	import Icon from '$lib/components/ui/icon.svelte';
	import { modalMotion, trackModalOrigin } from '$lib/modal-motion';
	import { cn } from '$lib/utils';

	type ModalSize = 'compact' | 'standard' | 'wide' | 'xl';

	interface Props {
		open?: boolean;
		onOpenChange?: (open: boolean) => void;
		size?: ModalSize;
		title: string;
		description?: string;
		dismissible?: boolean;
		children?: Snippet;
		footer?: Snippet;
		class?: string;
		// chrome lets one workspace surface draw its own title, header, footer
		// and overlay without a second modal shell. They travel together
		// because a surface sets them together.
		chrome?: { title?: string; header?: string; footer?: string; overlay?: string };
		// bodyClass is for a body that lays out its own panes: a workspace modal
		// takes the scroll away from the body and gives it to each pane.
		bodyClass?: string;
		// onClosed runs once the closing transition has finished and the content
		// is really gone, which is what a hand-off to a confirmation waits for.
		onClosed?: () => void;
	}

	let {
		open = $bindable(false),
		onOpenChange,
		size = 'standard',
		title,
		description,
		dismissible = true,
		children,
		footer,
		class: className,
		chrome = {},
		bodyClass,
		onClosed
	}: Props = $props();

	const maxWidth: Record<ModalSize, string> = {
		compact: 'max-w-lg',
		standard: 'max-w-2xl',
		wide: 'max-w-4xl',
		xl: 'max-w-6xl'
	};

	// On a phone the dialog is a bottom sheet: it slides up from the bottom
	// edge and a drag on the handle or header dismisses it. The drag tracks
	// the finger one to one, and a fast release past a third of the sheet
	// height closes it the way a flick throws a drawer home.
	let contentEl: HTMLDivElement | null = $state(null);
	let dragY = $state(0);
	let dragging = $state(false);
	let snappingBack = $state(false);

	function onSheetPointerDown(event: PointerEvent) {
		if (!dismissible || contentEl === null) return;
		if (!window.matchMedia('(max-width: 639.98px)').matches) return;
		const startY = event.clientY;
		let lastY = startY;
		let lastTime = event.timeStamp;
		let velocity = 0;
		let active = false;

		const move = (moveEvent: PointerEvent) => {
			const delta = moveEvent.clientY - startY;
			if (!active && Math.abs(delta) < 10) return;
			active = true;
			dragging = true;
			// Upward resistance keeps the sheet anchored to the top of the
			// screen while the finger pulls down freely.
			dragY = delta < 0 ? delta * 0.25 : delta;
			velocity = (moveEvent.clientY - lastY) / Math.max(1, moveEvent.timeStamp - lastTime);
			lastY = moveEvent.clientY;
			lastTime = moveEvent.timeStamp;
		};
		const finish = () => {
			window.removeEventListener('pointermove', move);
			window.removeEventListener('pointerup', finish);
			window.removeEventListener('pointercancel', finish);
			dragging = false;
			const height = contentEl?.offsetHeight ?? 0;
			const projected = dragY + velocity * 120;
			if (height > 0 && (dragY > height * 0.3 || projected > height * 0.3)) {
				open = false;
			} else if (dragY !== 0) {
				snappingBack = true;
				dragY = 0;
				setTimeout(() => (snappingBack = false), 300);
			}
		};
		window.addEventListener('pointermove', move);
		window.addEventListener('pointerup', finish);
		window.addEventListener('pointercancel', finish);
	}

	const dragStyle = $derived(
		dragging
			? 'transform: translateY(' + dragY + 'px); transition: none;'
			: snappingBack
				? 'transform: translateY(0); transition: transform 300ms cubic-bezier(0.32, 0.72, 0, 1);'
				: ''
	);

	let returnFocus: HTMLElement | null = null;
	// mounted reports that the content node is on the page. Bits UI settles its
	// open state before the leaving content is actually removed, so a hand-off to
	// a confirmation waits for this rather than for the state.
	let mounted = $state(false);

	onMount(trackModalOrigin);

	// reportGone tells the caller the modal is really off the page. It reads the
	// node rather than the open state, because the closing transition keeps both on
	// screen for its own length and a second overlay must not appear under it.
	$effect(() => {
		if (mounted || open) return;
		onClosed?.();
	});

	$effect.pre(() => {
		if (typeof document === 'undefined') return;
		if (open) {
			dragY = 0;
			returnFocus ??= document.activeElement instanceof HTMLElement ? document.activeElement : null;
		} else if (returnFocus) {
			const target = returnFocus;
			returnFocus = null;
			void tick().then(() => {
				if (target.isConnected) target.focus();
			});
		}
	});
</script>

<Dialog.Root
	bind:open
	{onOpenChange}
	onOpenChangeComplete={(value) => {
		if (!value) dragY = 0;
	}}
>
	<Dialog.Portal>
		<Dialog.Overlay forceMount class={cn('fixed inset-0 z-50 bg-[var(--scrim)]', chrome.overlay)}>
			{#snippet child({ props, open: overlayOpen })}
				{#if overlayOpen}
					<div {...props} data-reduced-motion-fade transition:fade={{ duration: 140 }}></div>
				{/if}
			{/snippet}
		</Dialog.Overlay>
		<Dialog.Content
			forceMount
			interactOutsideBehavior={dismissible ? 'close' : 'ignore'}
			class={cn(
				'fixed left-1/2 top-1/2 z-50 flex w-[calc(100%-1.5rem)] -translate-x-1/2 -translate-y-1/2 flex-col overflow-hidden rounded-card bg-surface text-ink shadow-modal outline-none',
				'max-h-[min(90dvh,800px)]',
				'max-sm:inset-x-0 max-sm:bottom-0 max-sm:top-auto max-sm:w-full max-sm:max-w-none max-sm:translate-x-0 max-sm:translate-y-0 max-sm:rounded-b-none max-sm:rounded-t-card max-sm:max-h-[92dvh]',
				maxWidth[size],
				className
			)}
			bind:ref={contentEl}
			style={dragStyle}
		>
			{#snippet child({ props, open: contentOpen })}
				{#if contentOpen}
					<div
						{...props}
						data-reduced-motion-fade
						transition:modalMotion={{ startY: () => dragY }}
						{@attach (_node) => {
							mounted = true;
							return () => {
								mounted = false;
							};
						}}
					>
						{#if dismissible}
							<div
								class="sheet-handle hidden shrink-0 items-center justify-center pt-2 max-sm:flex"
								onpointerdown={onSheetPointerDown}
								aria-hidden="true"
							>
								<span class="h-1 w-9 rounded-full bg-line-strong"></span>
							</div>
						{/if}
						<!-- Sticky header, no rule against the body; the header band
						     and the spacing separate the rows. -->
						<div
							role="presentation"
							class={cn(
								'flex shrink-0 items-start justify-between gap-3 bg-surface px-5 py-4 max-sm:px-4',
								chrome.header
							)}
							onpointerdown={onSheetPointerDown}
						>
							<div class="min-w-0">
								<Dialog.Title class={cn('text-[1.125rem] font-semibold leading-snug', chrome.title)}
									>{title}</Dialog.Title
								>
								{#if description}
									<Dialog.Description class="mt-0.5 text-sm text-muted-foreground"
										>{description}</Dialog.Description
									>
								{/if}
							</div>
							{#if dismissible}
								<Dialog.Close
									class="flex size-8 shrink-0 items-center justify-center rounded-control text-muted-foreground transition-colors hover:bg-hover hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
									aria-label={$t('ui.common.close')}
								>
									<Icon name="x" size={16} />
								</Dialog.Close>
							{/if}
						</div>

						<div class={cn('min-h-0 flex-1 overflow-y-auto px-5 py-4 max-sm:px-4', bodyClass)}>
							{@render children?.()}
						</div>

						<!-- Optional sticky footer on a canvas band, no rule above it. -->
						{#if footer}
							<div class={cn('shrink-0 bg-canvas px-5 py-3 max-sm:px-4', chrome.footer)}>
								{@render footer()}
							</div>
						{/if}
					</div>
				{/if}
			{/snippet}
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
