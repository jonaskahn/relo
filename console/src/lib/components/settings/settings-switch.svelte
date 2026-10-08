<script lang="ts">
	import { Switch } from 'bits-ui';

	import SettingsRow from './settings-row.svelte';

	// SettingsSwitch is a boolean settings row: the label and its hint in the
	// label column, the switch at the start of the value column, so a switch
	// lines up with the inputs and read-only values of the rows above and
	// below it. The FieldSwitch primitive puts the switch on the far edge of
	// its container, which is the shape other pages want and this page does
	// not.
	interface Props {
		id: string;
		checked?: boolean;
		label: string;
		hint?: string;
		disabled?: boolean;
		onCheckedChange?: (value: boolean) => void;
	}

	let {
		id,
		checked = $bindable(false),
		label,
		hint = '',
		disabled = false,
		onCheckedChange
	}: Props = $props();
</script>

<SettingsRow {label} {hint} labelFor={id}>
	<Switch.Root {id} bind:checked {disabled} {onCheckedChange} aria-label={label} />
</SettingsRow>
