// The launcher's own settings, in a window of its own (owner's request,
// 24/10): whether Clairance also shows in the Dock, by default yes. They are
// the app's preferences (UserDefaults, dev.clairance.launcher); the
// installation's paths stay in ~/.config/clairance/launcher.json.

import AppKit
import SwiftUI

let showInDockKey = "showInDock"

/// Whether the launcher shows in the Dock: yes unless switched off.
var showsInDock: Bool { UserDefaults.standard.object(forKey: showInDockKey) as? Bool ?? true }

/// In the Dock, or only in the menu bar. Info.plist says LSUIElement, so the
/// app starts out of the Dock and comes in when this says so.
@MainActor
func applyDockPolicy(_ show: Bool) {
    NSApplication.shared.setActivationPolicy(show ? .regular : .accessory)
}

struct LauncherSettings: View {
    @AppStorage(showInDockKey) private var showInDock = true

    var body: some View {
        // A grouped form, the toggle and its footnote (design 10).
        Form {
            Section {
                Toggle("Show Clairance in the Dock", isOn: $showInDock)
                    .onChange(of: showInDock) { show in
                        applyDockPolicy(show)
                        // Leaving the Dock takes the window's app out of the foreground; bring it back.
                        NSApplication.shared.activate(ignoringOtherApps: true)
                        SettingsWindow.shared.window?.makeKeyAndOrderFront(nil)
                    }
            } footer: {
                Text("Clairance stays in the menu bar either way.")
                    .foregroundStyle(.secondary)
            }
        }
        .formStyle(.grouped)
        .frame(width: 420)
    }
}

/// The settings window, opened from the menu, from the Dock icon, or with ⌘,.
@MainActor
final class SettingsWindow: NSObject, NSWindowDelegate {
    static let shared = SettingsWindow()
    private(set) var window: NSWindow?

    func show() {
        if window == nil {
            let w = NSWindow(contentViewController: NSHostingController(rootView: LauncherSettings()))
            w.title = "Clairance Settings"
            w.styleMask = [.titled, .closable]
            w.isReleasedWhenClosed = false
            w.delegate = self
            w.center()
            window = w
        }
        NSApplication.shared.activate(ignoringOtherApps: true)
        window?.makeKeyAndOrderFront(nil)
    }
}

/// The launcher's app delegate: the Dock policy at launch, the Dock icon's
/// click, and a quit from the Dock that goes through the menu's confirmation.
final class LauncherDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        MainActor.assumeIsolated { applyDockPolicy(showsInDock) }
    }

    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        MainActor.assumeIsolated { SettingsWindow.shared.show() }
        return false
    }

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        MainActor.assumeIsolated {
            guard let p = Production.shared, !p.quitConfirmed, !p.carried.isEmpty else { return .terminateNow }
            DispatchQueue.main.async { p.quit() }
            return .terminateCancel
        }
    }
}
