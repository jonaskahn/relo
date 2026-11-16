<script lang="ts">
	import { onMount } from 'svelte';
	import { locale, t } from 'svelte-i18n';

	import { formatStars, loadStars, STAR_URL } from '$lib/github-stars';
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
     be copied or middle-clicked. -->
<a
	href={STAR_URL}
	target="_blank"
	rel="noopener noreferrer"
	class={cn(buttonVariants({ variant: 'outline', size: 'sm' }), className)}
>
	<Icon name="brand-github" size={16} />
	{$t('ui.topbar.starOnGithub')}
	{#if stars !== null}
		<span class="border-l border-border pl-2 tabular-nums text-muted-foreground">
			{formatStars(stars, $locale ?? 'en')}
		</span>
	{/if}
</a>
