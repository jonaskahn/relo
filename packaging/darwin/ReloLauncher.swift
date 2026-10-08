import AppKit
import Foundation
import Sparkle

final class Launcher {
	let controller = SPUStandardUpdaterController(
		startingUpdater: true,
		updaterDelegate: nil,
		userDriverDelegate: nil
	)
	var child: Process?

	func start() throws {
		let directory = (Bundle.main.executableURL ?? URL(fileURLWithPath: CommandLine.arguments[0]))
			.deletingLastPathComponent()
		let process = Process()
		process.executableURL = directory.appendingPathComponent("Relo")
		process.arguments = ["daemon", "run"] + Array(CommandLine.arguments.dropFirst())
		process.terminationHandler = { proc in
			exit(proc.terminationStatus)
		}
		try process.run()
		child = process
	}

	func checkForUpdates() {
		controller.checkForUpdates(nil)
	}

	func stopChild() {
		child?.terminate()
	}
}

// The launcher hosts updates only; the Dock presence belongs to the app it
// starts. Regular → accessory is the direction the switch always takes.
// NSApplication.shared creates the instance; the bare NSApp global is nil
// in a process that never made one, and touching it traps.
NSApplication.shared.setActivationPolicy(.accessory)

let launcher = Launcher()

func watch(_ signalValue: Int32, handler: @escaping () -> Void) {
	signal(signalValue, SIG_IGN)
	let source = DispatchSource.makeSignalSource(signal: signalValue, queue: .main)
	source.setEventHandler(handler: handler)
	source.resume()
}

watch(SIGUSR1) { launcher.checkForUpdates() }
watch(SIGINT) { launcher.stopChild() }
watch(SIGTERM) { launcher.stopChild() }

try launcher.start()
RunLoop.main.run()
