<script lang="ts">
	import { Switch } from 'bits-ui';

	// FieldSwitch pairs a label and its description with the switch on the
	// right, which is how a boolean setting stays scannable in a form.
	interface Props {
		id: string;
		checked?: boolean;
		label: string;
		description?: string;
		disabled?: boolean;
		onCheckedChange?: (value: boolean) => void;
	}

	let {
		id,
		checked = $bindable(false),
		label,
		description = '',
		disabled = false,
		onCheckedChange
	}: Props = $props();
</script>

<div class="field-switch">
	<div class="min-w-0">
		<label class="field-label" for={id}>{label}</label>
		{#if description}<p class="field-hint">{description}</p>{/if}
	</div>
	<Switch.Root {id} bind:checked {disabled} {onCheckedChange} aria-label={label} />
</div>
