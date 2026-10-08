// The manage-daemon dialog is opened from the sidebar footer and from the
// daemon pill when it is not running, so its open state lives outside both.

class DaemonDialog {
	/** The shell's daemon-management surface. */
	open = $state(false);

	/** show opens the dialog on the daemon the operator asked about. */
	show() {
		this.open = true;
	}
}

/** The shell's daemon-management surface. */
export const daemonDialog = new DaemonDialog();
