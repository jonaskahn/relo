<script lang="ts">
	import { onMount } from 'svelte';
	import { locale, t } from 'svelte-i18n';

	import { formatStars, isSad, loadStars, STAR_URL } from '$lib/github-stars';
	import { buttonVariants } from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import { cn } from '$lib/utils';

	interface Props {
		class?: string;
	}

	let { class: className = '' }: Props = $props();

	let stars = $state<number | null>(null);

	onMount(() => {
		void loadStars().then((count) => (stars = count));
	});
</script>

<!-- A real link, not a button: it navigates, so it opens in a new tab and can
     be copied or middle-clicked. It takes the accent fill on purpose: it is the
     one call to action in the topbar, so it stands out from the outline controls. -->
<a
	href={STAR_URL}
	target="_blank"
	rel="noopener noreferrer"
	class={cn(buttonVariants({ variant: 'default', size: 'sm' }), className)}
>
	<Icon name="brand-github" size={16} />
	{$t('ui.topbar.starOnGithub')}
	{#if stars !== null}
		<span class="border-l border-primary-foreground/30 pl-2 tabular-nums">
			{formatStars(stars, $locale ?? 'en')}
			{#if isSad(stars)}<span aria-hidden="true">😢</span>{/if}
		</span>
	{/if}
</a>
