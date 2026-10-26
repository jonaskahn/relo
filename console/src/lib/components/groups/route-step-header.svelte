<script lang="ts">
	import WizardStepper from '$lib/components/ui/wizard-stepper.svelte';
	import type { RouteStep } from '$lib/route-stepper';

	interface Props {
		steps: readonly RouteStep[];
		step: RouteStep;
		onjump: (step: RouteStep) => void;
	}

	let { steps, step, onjump }: Props = $props();

	const labelKeys: Record<RouteStep, string> = {
		basics: 'ui.pages.groupsPage.step.basics',
		billing: 'ui.pages.groupsPage.step.billing',
		members: 'ui.pages.groupsPage.step.members',
		review: 'ui.pages.groupsPage.step.review'
	};
</script>

<div class="px-4 pb-4 sm:px-7">
	<WizardStepper
		{steps}
		current={step}
		{labelKeys}
		{onjump}
		stepOfKey="ui.pages.groupsPage.stepOf"
	/>
</div>
