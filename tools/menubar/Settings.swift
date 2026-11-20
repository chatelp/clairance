// The launcher's own settings, in a window of its own (owner's request,
// 24/10): whether Clairance also shows in the Dock, by default yes. They are
// the app's preferences (UserDefaults, dev.clairance.launcher); the
// installation's paths stay in ~/.config/clairance/launcher.json.

import AppKit
import SwiftUI

let showInDockKey = "showInDock"

/// The kinds of notifications, each with its switch (owner's request, 20/11):
/// the guard's threads (Checks.swift) and who connects. All on by default.
let noticeKinds: [(kind: String, label: String)] = [
    ("production", "Production stops, does not start or stops answering"),
    ("station", "Production stops hearing the station"),
    ("audio", "A frequency goes silent"),
    ("transcription", "Transcription stops or degrades"),
    ("disk", "A disk is missing or almost full"),
    ("memory", "macOS is short of memory"),
    ("instances", "A test instance or a helper is left running"),
    ("connections", "Someone connects"),
]

func notifies(_ kind: String) -> Bool {
    UserDefaults.standard.object(forKey: "notify." + kind) as? Bool ?? true
}

struct NoticeSwitch: View {
    let label: String
    @AppStorage private var on: Bool

    init(kind: String, label: String) {
        self.label = label
        _on = AppStorage(wrappedValue: true, "notify." + kind)
    }

    var body: some View { Toggle(label, isOn: $on) }
}

/// Whether the launcher shows in the Dock: yes unless switched off; never a
/// test bench's, which would stand beside the owner's own.
var showsInDock: Bool {
    ProcessInfo.processInfo.environment["CLAIRANCE_SUPPORT_DIR"] == nil
        && UserDefaults.standard.object(forKey: showInDockKey) as? Bool ?? true
}

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
            Section {
                ForEach(noticeKinds, id: \.kind) { NoticeSwitch(kind: $0.kind, label: $0.label) }
            } header: {
                Text("Notify when")
            } footer: {
                Text("A fault shows on the icon, in the menu and in the window either way. Your own visits as an owner are not notified.")
                    .foregroundStyle(.secondary)
            }
        }
        .formStyle(.grouped)
        .frame(width: 460)
        // A grouped form scrolls, so it asks for no height of its own: without
        // this the window opened as a title bar alone (seen on macOS 27, 20/11).
        .fixedSize(horizontal: false, vertical: true)
    }
}

/// The settings window, opened from the menu or with ⌘,.
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

/// The launcher's app delegate: the Dock policy and the window at launch, the
/// Dock icon's click, and a quit from the Dock that goes through the menu's
/// confirmation.
final class LauncherDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        // The window opens when its owner opens the app (Finder, Dock, Spotlight):
        // not when a command opened the launcher, nor at login (design 11).
        let event = NSAppleEventManager.shared().currentAppleEvent
        let atLogin = event?.eventID == kAEOpenApplication
            && event?.paramDescriptor(forKeyword: keyAEPropData)?.enumCodeValue == keyAELaunchedAsLogInItem
        MainActor.assumeIsolated {
            applyDockPolicy(showsInDock)
            if !openedInBackground && !atLogin { DashboardWindow.shared.show() }
        }
    }

    // A click on the Dock icon brings the window back, as any app's main window.
    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        MainActor.assumeIsolated { DashboardWindow.shared.show() }
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
