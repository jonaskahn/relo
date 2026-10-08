<script lang="ts">
	import { onMount } from 'svelte';
	import { locale, t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';

	import { api } from '$lib/api';
	import AccentPicker from '$lib/components/shell/accent-picker.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import ThemePicker from '$lib/components/shell/theme-picker.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import { Popover } from 'bits-ui';
	import PopoverPanel from '$lib/components/ui/popover-panel.svelte';
	import { Select } from 'bits-ui';
	import {
		auto,
		isLanguageChoice,
		isLocale,
		languageChoices,
		languageOptionLabel,
		savedLanguage,
		setLanguage,
		type LanguageChoice
	} from '$lib/i18n';

	let open = $state(false);
	let savingLanguage = $state(false);
	let choiceEpoch = 0;

	const selectedLanguage = $derived(
		languageChoices.find((choice) => choice === $savedLanguage) ?? auto
	);
	const languageItems = $derived(
		languageChoices.map((code) => ({
			value: code,
			label: languageOptionLabel(code, $t('ui.settings.language.auto'))
		}))
	);

	onMount(() => {
		const epoch = choiceEpoch;
		void api<{ language?: string }>('/settings')
			.then((data) => {
				if (epoch !== choiceEpoch) return;
				const language = data.language;
				if (typeof language === 'string' && (language === auto || isLocale(language))) {
					savedLanguage.set(language);
				}
			})
			.catch(() => {});
	});

	async function chooseLanguage(code: LanguageChoice) {
		if (savingLanguage || $savedLanguage === code) return;
		const previousChoice = $savedLanguage;
		const previousRendered = $locale;
		choiceEpoch += 1;
		savingLanguage = true;
		savedLanguage.set(code);
		try {
			await setLanguage(code === auto ? navigator.language : code);
			await api('/settings/language', {
				method: 'PATCH',
				body: JSON.stringify({ language: code })
			});
			if (code === auto) {
				try {
					const status = await api<{ language?: string }>('/status');
					await setLanguage(status.language);
				} catch {
					// The choice was saved. The browser locale already stands in
					// until the next status read.
				}
			}
		} catch (error) {
			savedLanguage.set(previousChoice);
			await setLanguage(previousRendered);
			toast.error(error instanceof Error ? error.message : $t('ui.settings.language.failure'));
		} finally {
			savingLanguage = false;
		}
	}
</script>

<Popover.Root bind:open>
	<Popover.Trigger>
		{#snippet child({ props })}
			<Button
				variant="outline"
				size="icon"
				{...props}
				aria-haspopup="dialog"
				aria-label={$t('ui.topbar.appearance')}
				title={$t('ui.topbar.appearance')}
			>
				<Icon name="palette" size={18} />
			</Button>
		{/snippet}
	</Popover.Trigger>
	<Popover.Portal>
		<PopoverPanel align="end" class="surface-pop z-50 w-[min(360px,100vw-2rem)] p-[1.125rem]">
			<div class="space-y-5">
				<section>
					<h4 class="mb-2.5 text-[0.78125rem] font-semibold text-muted-foreground">
						{$t('ui.topbar.theme')}
					</h4>
					<ThemePicker layout="popover" />
				</section>
				<section>
					<h4 class="mb-2.5 text-[0.78125rem] font-semibold text-muted-foreground">
						{$t('ui.topbar.accent')}
					</h4>
					<AccentPicker layout="popover" />
				</section>
				<section>
					<h4 class="mb-2.5 text-[0.78125rem] font-semibold text-muted-foreground">
						{$t('ui.topbar.language')}
					</h4>
					<Select.Root
						type="single"
						value={selectedLanguage}
						items={languageItems}
						disabled={savingLanguage}
						onValueChange={(value) => {
							if (isLanguageChoice(value)) void chooseLanguage(value);
						}}
					>
						<Select.Trigger
							class="flex h-control w-full items-center justify-between gap-2 rounded-control border border-line bg-surface px-3 text-sm text-ink outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50"
							aria-label={$t('ui.topbar.language')}
						>
							<span class="truncate">
								{languageOptionLabel(selectedLanguage, $t('ui.settings.language.auto'))}
							</span>
							<Icon name="chevron-down" size={14} class="shrink-0 text-muted-foreground" />
						</Select.Trigger>
						<Select.Portal>
							<Select.Content
								class="surface-pop z-50 w-[min(320px,calc(100vw-2rem))] overflow-hidden p-1"
								sideOffset={6}
							>
								<Select.Viewport class="max-h-72 overflow-y-auto">
									{#each languageChoices as code (code)}
										<Select.Item
											value={code}
											label={languageOptionLabel(code, $t('ui.settings.language.auto'))}
											class="flex min-h-control cursor-pointer items-center rounded-control px-2.5 text-sm text-ink outline-none data-highlighted:bg-hover"
										>
											{languageOptionLabel(code, $t('ui.settings.language.auto'))}
										</Select.Item>
									{/each}
								</Select.Viewport>
							</Select.Content>
						</Select.Portal>
					</Select.Root>
				</section>
			</div>
		</PopoverPanel>
	</Popover.Portal>
</Popover.Root>
