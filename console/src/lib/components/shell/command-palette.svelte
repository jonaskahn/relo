<script lang="ts">
	import { t } from 'svelte-i18n';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';

	import { goto } from '$app/navigation';
	import { page } from '$app/state';

	import { addConnectionHref } from '$lib/connections-view';
	import { consoleState } from '$lib/console-state.svelte';
	import { paletteOpen } from '$lib/palette-state.svelte';
	import { NAV_SECTIONS } from '$lib/nav';
	import { ACCENTS } from '$lib/theme.svelte';
	import { Dialog } from 'bits-ui';
	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';
	import { modalMotion, trackModalOrigin } from '$lib/modal-motion';
	import { cn } from '$lib/utils';

	type Item = { id: string; label: string; group: string; run: () => void; icon?: IconName };

	let query = $state('');
	let active = $state(0);

	const open = $derived(paletteOpen.value);
	onMount(trackModalOrigin);

	// Palette actions run without arguments; each one closes the dialog
	// first so navigation or a settings jump never renders under it.
	const items = $derived.by<Item[]>(() => {
		const list: Item[] = [];
		for (const section of NAV_SECTIONS) {
			for (const link of section.links) {
				list.push({
					id: 'page:' + link.id,
					label: $t(link.labelKey),
					group: $t('ui.palette.pages'),
					icon: link.icon,
					run: () => goto(link.href)
				});
			}
		}
		list.push(
			{
				id: 'action:key',
				label: $t('ui.palette.actionCreateKey'),
				group: $t('ui.palette.actions'),
				run: () => goto('/keys?create=1')
			},
			{
				id: 'action:account',
				label: $t('ui.palette.actionAddAccount'),
				group: $t('ui.palette.actions'),
				run: () => goto(addConnectionHref(page.url.pathname, page.url.search))
			},
			{
				id: 'action:theme',
				label: $t('ui.palette.actionTheme'),
				group: $t('ui.palette.actions'),
				run: () => goto('/settings')
			},
			{
				id: 'action:accent',
				label: $t('ui.palette.actionAccent'),
				group: $t('ui.palette.actions'),
				run: () => goto('/settings')
			},
			{
				id: 'action:signout',
				label: $t('ui.palette.actionSignOut'),
				group: $t('ui.palette.actions'),
				run: () => goto('/settings')
			},
			{
				id: 'set:language',
				label: $t('ui.palette.actionLanguage'),
				group: $t('ui.palette.settings'),
				run: () => goto('/settings')
			},
			{
				id: 'set:retention',
				label: $t('ui.palette.actionRetention'),
				group: $t('ui.palette.settings'),
				run: () => goto('/settings')
			}
		);
		for (const theme of ['system', 'light', 'dark'] as const) {
			list.push({
				id: 'theme:' + theme,
				label:
					$t('ui.palette.actionTheme') +
					' · ' +
					$t(
						'ui.topbar.' +
							(theme === 'system' ? 'themeSystem' : theme === 'light' ? 'themeLight' : 'themeDark')
					),
				group: $t('ui.palette.actions'),
				run: () => consoleState.setTheme(theme)
			});
		}
		for (const accent of ACCENTS) {
			list.push({
				id: 'accent:' + accent,
				label: $t('ui.palette.actionAccent') + ' · ' + $t('ui.accent.' + accent),
				group: $t('ui.palette.actions'),
				run: () => consoleState.setAccent(accent)
			});
		}
		return list;
	});

	const filtered = $derived.by(() => {
		const q = query.trim().toLowerCase();
		if (!q) return items;
		return items.filter(
			(item) => item.label.toLowerCase().includes(q) || item.group.toLowerCase().includes(q)
		);
	});

	const grouped = $derived.by(() => {
		const groups: Array<{ name: string; items: Item[] }> = [];
		for (const item of filtered) {
			const last = groups[groups.length - 1];
			if (last && last.name === item.group) last.items.push(item);
			else groups.push({ name: item.group, items: [item] });
		}
		return groups;
	});

	// The highlighted row follows the list shrinking under the query, which is
	// what keeps Enter on the row the operator can see.
	const cursor = $derived(Math.max(0, Math.min(active, filtered.length - 1)));

	function setOpen(next: boolean) {
		paletteOpen.value = next;
		// A palette opens on the whole list, whatever was typed into it last.
		if (!next) return;
		query = '';
		active = 0;
	}

	function runItem(item: Item) {
		paletteOpen.value = false;
		item.run();
	}

	function onkeydown(event: KeyboardEvent) {
		if (event.key === 'ArrowDown') {
			event.preventDefault();
			active = Math.min(cursor + 1, filtered.length - 1);
		} else if (event.key === 'ArrowUp') {
			event.preventDefault();
			active = Math.max(cursor - 1, 0);
		} else if (event.key === 'Enter' && filtered[cursor]) {
			event.preventDefault();
			runItem(filtered[cursor]);
		}
	}
</script>

<svelte:window
	onkeydown={(event) => {
		if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
			event.preventDefault();
			setOpen(!paletteOpen.value);
		}
	}}
/>

<Dialog.Root {open} onOpenChange={setOpen}>
	<Dialog.Portal>
		<Dialog.Overlay forceMount class="fixed inset-0 z-50 bg-[var(--scrim)]">
			{#snippet child({ props, open: overlayOpen })}
				{#if overlayOpen}
					<div {...props} data-reduced-motion-fade transition:fade={{ duration: 140 }}></div>
				{/if}
			{/snippet}
		</Dialog.Overlay>
		<Dialog.Content
			forceMount
			{...{ 'aria-label': $t('ui.topbar.palette') }}
			class="fixed left-1/2 top-24 z-50 w-full max-w-lg -translate-x-1/2 surface-pop p-0 outline-none"
			{onkeydown}
		>
			{#snippet child({ props, open: contentOpen })}
				{#if contentOpen}
					<div {...props} data-reduced-motion-fade transition:modalMotion>
						<Dialog.Title class="sr-only">{$t('ui.topbar.palette')}</Dialog.Title>
						<input
							bind:value={query}
							class="w-full bg-transparent px-4 py-3 text-sm outline-none"
							placeholder={$t('ui.palette.placeholder')}
							autocomplete="off"
							spellcheck="false"
						/>
						<div class="max-h-80 overflow-y-auto p-2">
							{#if grouped.length === 0}
								<p class="text-muted-foreground px-3 py-6 text-center text-sm">
									{$t('ui.palette.empty')}
								</p>
							{/if}
							{#each grouped as group (group.name)}
								<p class="text-muted-foreground px-2 pt-2 pb-1 text-[0.78125rem] font-semibold">
									{group.name}
								</p>
								{#each group.items as item (item.id)}
									{@const flatIndex = filtered.indexOf(item)}
									<button
										type="button"
										data-plain
										class={cn(
											'flex w-full cursor-pointer items-center gap-2 rounded-control px-3 py-2 text-left text-sm',
											flatIndex === cursor
												? 'bg-accent-soft font-medium text-accent-ink'
												: 'hover:bg-hover'
										)}
										onmouseenter={() => (active = flatIndex)}
										onclick={() => runItem(item)}
									>
										{#if item.icon}
											<Icon name={item.icon} size={15} class="shrink-0 text-muted-foreground" />
										{/if}
										{item.label}
									</button>
								{/each}
							{/each}
						</div>
					</div>
				{/if}
			{/snippet}
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
