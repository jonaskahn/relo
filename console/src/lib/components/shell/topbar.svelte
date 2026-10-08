<script lang="ts">
	import { t } from 'svelte-i18n';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';

	import { addConnectionHref } from '$lib/connections-view';
	import { navDrawer } from '$lib/nav-drawer.svelte';
	import AppearancePopover from '$lib/components/shell/appearance-popover.svelte';
	import HealthPill from '$lib/components/shell/health-pill.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import { DropdownMenu } from 'bits-ui';
	import MenuPanel from '$lib/components/ui/menu-panel.svelte';
	import { cn } from '$lib/utils';

	// scrolled is the shell telling the bar its content moved under it, so the
	// bar can add the one shadow that separates it from the page.
	interface Props {
		scrolled?: boolean;
	}

	let { scrolled = false }: Props = $props();

	const titleKey = $derived.by(() => {
		const p = page.url.pathname;
		if (p === '/') return 'ui.pages.dashboard.title';
		if (p.startsWith('/connections')) return 'ui.sidebar.connections';
		if (p.startsWith('/keys')) return 'ui.pages.keys.title';
		if (p.startsWith('/groups')) return 'ui.sidebar.groups';
		if (p.startsWith('/logs')) return 'ui.pages.logs.title';
		if (p.startsWith('/usage')) return 'ui.pages.usage.title';
		if (p.startsWith('/chat')) return 'ui.pages.chatPage.title';
		if (p.startsWith('/agents') || p.startsWith('/integrations')) return 'ui.sidebar.agents';
		if (p.startsWith('/settings')) return 'ui.pages.settings.title';
		return 'ui.sidebar.console';
	});

	// The one button follows the surface it controls: on a wide screen it
	// switches the docked rail between full and compact, and everywhere else
	// it opens the overlay drawer — including a page that asked for one.
	const target = $derived(navDrawer.target);
	const menuLabel = $derived(
		target === 'drawer'
			? $t('ui.topbar.menu')
			: navDrawer.compact
				? $t('ui.topbar.expandMenu')
				: $t('ui.topbar.collapseMenu')
	);
	const menuExpanded = $derived((navDrawer.pinned && !navDrawer.compact) || navDrawer.overlay);
</script>

<header
	class={cn(
		'chrome sticky top-0 z-30 flex h-[60px] shrink-0 items-center gap-3 px-[var(--gutter)] transition-shadow',
		scrolled && 'shadow-[0_8px_24px_-12px_var(--shadow)]'
	)}
>
	<Button
		bind:ref={navDrawer.trigger}
		variant="ghost"
		size="icon"
		aria-label={menuLabel}
		aria-expanded={menuExpanded}
		aria-controls="nav-drawer"
		onclick={() => navDrawer.toggle()}
	>
		<Icon name="menu-2" size={18} />
	</Button>

	<nav aria-label={$t('ui.topbar.breadcrumb')} class="flex min-w-0 items-center gap-1.5 text-sm">
		<span class="hidden text-muted-foreground sm:inline">Relo</span>
		<span class="hidden text-faint sm:inline" aria-hidden="true">/</span>
		<span class="truncate font-semibold text-ink">{$t(titleKey)}</span>
	</nav>

	<div class="flex-1"></div>

	<HealthPill class="hidden sm:inline-flex" />

	<DropdownMenu.Root>
		<DropdownMenu.Trigger>
			{#snippet child({ props })}
				<Button
					variant="outline"
					size="icon"
					{...props}
					aria-label={$t('ui.topbar.quickAdd')}
					title={$t('ui.topbar.quickAdd')}
				>
					<Icon name="plus" size={18} />
				</Button>
			{/snippet}
		</DropdownMenu.Trigger>
		<DropdownMenu.Portal>
			<MenuPanel align="end" class="surface-pop z-50 min-w-44 p-1">
				<DropdownMenu.Item
					class="flex min-h-control cursor-pointer items-center gap-2 rounded-control px-2.5 py-1.5 text-sm outline-none data-highlighted:bg-hover"
					onSelect={() => goto('/keys?create=1')}
				>
					<Icon name="key" size={15} />
					{$t('ui.topbar.quickAddKey')}
				</DropdownMenu.Item>
				<DropdownMenu.Item
					class="flex min-h-control cursor-pointer items-center gap-2 rounded-control px-2.5 py-1.5 text-sm outline-none data-highlighted:bg-hover"
					onSelect={() => goto(addConnectionHref(page.url.pathname, page.url.search))}
				>
					<Icon name="plug-connected" size={15} />
					{$t('ui.topbar.quickAddAccount')}
				</DropdownMenu.Item>
			</MenuPanel>
		</DropdownMenu.Portal>
	</DropdownMenu.Root>

	<AppearancePopover />
</header>
