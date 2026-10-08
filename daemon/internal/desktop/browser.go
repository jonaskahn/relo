package desktop

// Open hands url to the default browser. The browser is started detached
// from the tray; the call returns once it is launched.
func Open(url string) error {
	command, err := openCommand(url)
	if err != nil {
		return err
	}
	return command.Start()
}
