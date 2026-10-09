import AppKit
import Foundation
import Sparkle

final class Launcher: NSObject, NSApplicationDelegate {
	let controller = SPUStandardUpdaterController(
		startingUpdater: true,
		updaterDelegate: nil,
		userDriverDelegate: nil
	)
	var child: Process?
 var stopping = false
 var signalSources: [DispatchSourceSignal] = []

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
 guard !stopping else { return }
		controller.checkForUpdates(nil)
	}

	func stopChild() {
		guard !stopping else { return }
  stopping = true
  guard let child, child.isRunning else { exit(0) }
  child.terminate()
  let pid = child.processIdentifier
  DispatchQueue.global().asyncAfter(deadline: .now() + 1) {
   kill(pid, SIGKILL)
   exit(1)
  }
	}
 func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
  stopChild()
  return .terminateCancel
 }
}

// The launcher hosts updates only; the Dock presence belongs to the app it
// starts. Regular → accessory is the direction the switch always takes.
// NSApplication.shared creates the instance; the bare NSApp global is nil
// in a process that never made one, and touching it traps.
NSApplication.shared.setActivationPolicy(.accessory)

let launcher = Launcher()
NSApplication.shared.delegate = launcher

func watch(_ signalValue: Int32, handler: @escaping () -> Void) {
	signal(signalValue, SIG_IGN)
	let source = DispatchSource.makeSignalSource(signal: signalValue, queue: .main)
	source.setEventHandler(handler: handler)
	launcher.signalSources.append(source)
 source.resume()
}

watch(SIGUSR1) { launcher.checkForUpdates() }
watch(SIGINT) { launcher.stopChild() }
watch(SIGTERM) { launcher.stopChild() }

try launcher.start()
NSApplication.shared.run()
