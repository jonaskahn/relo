<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { fade } from 'svelte/transition';
	import { afterNavigate } from '$app/navigation';
	import { page } from '$app/state';
	import { agentAttention } from '$lib/agent-attention.svelte';
	import { daemonDialog } from '$lib/daemon-dialog.svelte';
	import { isNavLinkActive, NAV_SECTIONS } from '$lib/nav';
	import { navDrawer } from '$lib/nav-drawer.svelte';
	import { drawerEnter, drawerLeave } from '$lib/modal-motion';
	import Icon from '$lib/components/ui/icon.svelte';
	import LogoMark from '$lib/components/ui/logo-mark.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import IconAction from '$lib/components/ui/icon-action.svelte';
	import { cn } from '$lib/utils';

	const sections = NAV_SECTIONS;

	// The rail is docked beside the page; the drawer is the overlay the top bar
	// opens. Only the drawer is modal, moves focus, and closes on Escape.
	// Compact is the icon column. The overlay always opens at full width.
	const pinned = $derived(navDrawer.pinned);
	const overlay = $derived(navDrawer.overlay);
	const compact = $derived(navDrawer.compact);
	const compactIcon = 22;
	let reducedMotion = $state(false);
	const railEase = $derived(reducedMotion ? 'duration-0' : 'duration-200 ease-out');
	const labelEase = $derived(reducedMotion ? 'duration-0' : 'duration-150 ease-out');

	const isActive = (href: string): boolean => isNavLinkActive(href, page.url.pathname);

	// The drawer is a touch surface: it follows the finger while a swipe
	// drags it left and closes past a third of its width, the way the sheets
	// on the same screen dismiss.
	let dragX = $state(0);
	let dragging = $state(false);
	// releaseX is where the finger left the drawer on a drag-dismiss, so the
	// outro continues from there instead of snapping back to fully open. It is
	// cleared on every open, so a button or Escape close always leaves from 0.
	let releaseX = $state(0);
	let panel = $state<HTMLElement | null>(null);

	// The drawer is a transient surface, so a navigation that did not come from a
	// link inside it — a redirect, a command-palette jump — must not leave it
	// hanging open over the new page. A docked rail stays where it is.
	afterNavigate(() => {
		navDrawer.close();
		void agentAttention.refresh();
	});

	onMount(() => {
		void agentAttention.refresh();
		const stopWatchingAgentCatalog = agentAttention.watch();
		const motion = window.matchMedia('(prefers-reduced-motion: reduce)');
		const update = () => (reducedMotion = motion.matches);
		update();
		motion.addEventListener('change', update);
		return () => {
			stopWatchingAgentCatalog();
			motion.removeEventListener('change', update);
		};
	});

	// Opening moves focus into the drawer so a keyboard lands on the menu
	// rather than behind the scrim; closing hands it back to the button that
	// opened it. The first run is left alone: on load the drawer was never
	// open, and pulling focus would take it from the page for no reason.
	let wasOpen = false;
	$effect(() => {
		if (overlay) {
			wasOpen = true;
			releaseX = 0;
			panel?.focus();
			return;
		}
		if (wasOpen) {
			wasOpen = false;
			navDrawer.trigger?.focus();
		}
	});

	function onKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') navDrawer.close();
	}

	function onDrawerPointerDown(event: PointerEvent) {
		if (!overlay) return;
		if (event.target instanceof Element && event.target.closest('a, button, input')) return;
		const startX = event.clientX;
		const move = (moveEvent: PointerEvent) => {
			const dx = Math.min(0, moveEvent.clientX - startX);
			if (Math.abs(dx) < 8) return;
			dragging = true;
			dragX = dx;
		};
		const finish = () => {
			window.removeEventListener('pointermove', move);
			window.removeEventListener('pointerup', finish);
			window.removeEventListener('pointercancel', finish);
			if (dragX < -64) {
				releaseX = dragX;
				navDrawer.close();
			}
			dragging = false;
			dragX = 0;
		};
		window.addEventListener('pointermove', move);
		window.addEventListener('pointerup', finish);
		window.addEventListener('pointercancel', finish);
	}
</script>

{#if overlay}
	<button
		type="button"
		data-plain
		data-reduced-motion-fade
		aria-label={$t('ui.sidebar.closeMenu')}
		class="fixed inset-0 z-40 bg-black/40 backdrop-blur-sm"
		transition:fade={{ duration: 140 }}
		onclick={() => navDrawer.close()}
	></button>
{/if}

<!-- The overlay is a dialog, not a landmark: it is transient and modal, so
	     the panel carries the role. A docked rail is neither, and the nav inside
	     it stays the landmark that names the console. -->
{#snippet panelBody()}
	<div
		class={cn(
			'relative flex h-[60px] shrink-0 items-center transition-[padding,gap]',
			railEase,
			compact ? 'justify-center px-1' : 'gap-2.5 px-4'
		)}
	>
		<a
			href="/"
			aria-label={$t('ui.sidebar.dashboard')}
			onclick={() => navDrawer.close()}
			class={cn('flex min-w-0 items-center gap-2.5 rounded-control', compact && 'justify-center')}
		>
			<LogoMark
				class={cn(
					'shrink-0 text-accent-strong transition-[width,height]',
					railEase,
					compact ? 'size-[22px]' : 'size-5'
				)}
			/>
			<span
				aria-hidden={compact}
				class={cn(
					'text-sm font-semibold tracking-tight transition-[opacity,transform]',
					labelEase,
					compact
						? 'pointer-events-none absolute w-0 overflow-hidden opacity-0'
						: 'translate-x-0 opacity-100'
				)}
			>
				Relo
			</span>
		</a>
		{#if overlay}
			<Button
				variant="ghost"
				size="icon"
				class="ml-auto"
				aria-label={$t('ui.sidebar.closeMenu')}
				onclick={() => navDrawer.close()}
			>
				<Icon name="x" size={16} />
			</Button>
		{/if}
	</div>

	<nav
		class={cn(
			'flex-1 overflow-y-auto overscroll-contain py-3 transition-[padding]',
			railEase,
			compact ? 'space-y-4 px-2' : 'space-y-5 px-2.5'
		)}
		aria-label={$t('ui.sidebar.console')}
	>
		{#each sections as section (section.id)}
			<div class="space-y-1">
				{#if section.labelKey}
					<p
						aria-hidden={compact}
						class={cn(
							'overflow-hidden px-2 text-[0.78125rem] font-semibold text-muted-foreground transition-[opacity,max-height,padding]',
							labelEase,
							compact ? 'max-h-0 pb-0 opacity-0' : 'max-h-5 pb-1 opacity-100'
						)}
					>
						{$t(section.labelKey)}
					</p>
				{/if}
				{#each section.links as link (link.id)}
					<a
						href={link.href}
						class={cn(
							'relative flex min-h-control items-center overflow-hidden text-sm font-medium whitespace-nowrap transition-[background-color,color,box-shadow,padding,gap]',
							railEase,
							compact ? 'justify-center rounded-control px-0' : 'gap-2.5 rounded-control px-2.5',
							isActive(link.href)
								? 'bg-accent-soft text-accent-ink shadow-[0_4px_14px_var(--accent-glow)]'
								: 'text-muted-foreground hover:bg-hover hover:text-ink'
						)}
						aria-current={isActive(link.href) ? 'page' : undefined}
						title={compact ? $t(link.labelKey) : undefined}
						onclick={() => navDrawer.close()}
					>
						<span
							class={cn(
								'inline-flex shrink-0 items-center justify-center transition-[width,height]',
								railEase,
								compact ? 'size-[22px]' : 'size-[17px]'
							)}
						>
							<Icon name={link.icon} size={compact ? compactIcon : 17} class="size-full" />
						</span>
						<span
							aria-hidden={compact}
							class={cn(
								'truncate transition-[opacity,transform]',
								labelEase,
								compact
									? 'pointer-events-none absolute left-0 w-0 -translate-x-1 overflow-hidden opacity-0'
									: 'min-w-0 flex-1 translate-x-0 opacity-100'
							)}
						>
							{$t(link.labelKey)}
						</span>
						{#if link.href === '/agents' && agentAttention.count > 0}
							<span
								role="img"
								aria-label={$t('ui.sidebar.agentsAttention')}
								title={$t('ui.sidebar.agentsAttention')}
								class={cn('text-warn', compact ? 'absolute right-0.5 top-0.5' : 'shrink-0')}
							>
								<Icon name="alert-triangle" size={compact ? 11 : 14} />
							</span>
						{/if}
					</a>
				{/each}
			</div>
		{/each}
	</nav>

	<div
		class={cn(
			'shrink-0 pb-[max(0.5rem,env(safe-area-inset-bottom))] transition-[padding]',
			railEase,
			compact ? 'flex flex-col items-center gap-1 p-2' : 'flex items-center gap-1 p-2'
		)}
		role="group"
		aria-label={$t('ui.sidebar.actions')}
	>
		<IconAction
			icon="brand-github"
			label={$t('ui.sidebar.github')}
			iconSize={compact ? compactIcon : 16}
			onclick={() =>
				window.open('https://github.com/jonaskahn/relo', '_blank', 'noopener,noreferrer')}
		/>
		<IconAction
			icon="coffee"
			label={$t('ui.sidebar.coffee')}
			iconSize={compact ? compactIcon : 16}
			onclick={() =>
				window.open('https://buymeacoffee.com/jonaskahn', '_blank', 'noopener,noreferrer')}
		/>
		{#if !compact}
			<!-- Spacer keeps Manage daemon at the far end. -->
			<div class="flex-1" aria-hidden="true"></div>
		{/if}
		<IconAction
			icon="power"
			label={$t('ui.sidebar.manage')}
			iconSize={compact ? compactIcon : 16}
			onclick={() => daemonDialog.show()}
		/>
	</div>
{/snippet}

{#if pinned}
	<div
		bind:this={panel}
		id="nav-drawer"
		aria-label={$t('ui.sidebar.console')}
		tabindex="-1"
		class={cn(
			'chrome fixed inset-y-0 left-0 z-40 flex shrink-0 flex-col overflow-hidden shadow-edge transition-[transform,width]',
			railEase,
			compact ? 'w-16' : 'w-[224px]'
		)}
	>
		{@render panelBody()}
	</div>
{:else if overlay}
	<!-- The drawer owns its entrance: a Svelte slide owns the transform, so
		     no CSS transition class may share it. The rail above keeps its CSS
		     width morph and never animates on mount. -->
	<div
		bind:this={panel}
		id="nav-drawer"
		role="dialog"
		aria-modal="true"
		aria-label={$t('ui.sidebar.console')}
		tabindex="-1"
		class="chrome fixed inset-y-0 left-0 z-40 flex w-[224px] shrink-0 flex-col overflow-hidden shadow-edge"
		style={dragging ? 'transform: translateX(' + dragX + 'px); transition: none;' : ''}
		in:drawerEnter
		out:drawerLeave={{ startX: () => releaseX }}
		onpointerdown={onDrawerPointerDown}
		onkeydown={onKeydown}
	>
		{@render panelBody()}
	</div>
{/if}
