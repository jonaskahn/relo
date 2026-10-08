<script lang="ts">
	import WizardStepper from '$lib/components/ui/wizard-stepper.svelte';
	import { ROUTE_STEPS, type RouteStep } from '$lib/route-stepper';

	interface Props {
		step: RouteStep;
		onjump: (step: RouteStep) => void;
	}

	let { step, onjump }: Props = $props();

	const labelKeys: Record<RouteStep, string> = {
		basics: 'ui.pages.groupsPage.step.basics',
		members: 'ui.pages.groupsPage.step.members',
		review: 'ui.pages.groupsPage.step.review'
	};
</script>

<div class="px-4 pb-4 sm:px-7">
	<WizardStepper
		steps={ROUTE_STEPS}
		current={step}
		{labelKeys}
		{onjump}
		stepOfKey="ui.pages.groupsPage.stepOf"
	/>
</div>
