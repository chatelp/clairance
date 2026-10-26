// The guard's notifications, macOS's own (docs-fr/design/10-lanceur.md, "Les
// notifications"): one per incident when it begins, replaced by a second when
// it ends (same request identifier), grouped by kind of incident. A button
// that stops or restarts goes through the menu's confirmation: a notification
// never does anything on its own.

import AppKit
import UserNotifications

/// What a notification's buttons ask the launcher for.
enum NoticeAction: String {
    case start, log, restart, finder, stop, settings
}

@MainActor
final class Notifier: NSObject, UNUserNotificationCenterDelegate {
    /// The launcher's answer to a button or a click.
    var handle: (NoticeAction?, Notice.Click, [String: String]) -> Void = { _, _, _ in }
    private(set) var allowed: Bool?
    private(set) var refusal = ""      // macOS's reason, when it refuses

    func setUp() {
        let c = UNUserNotificationCenter.current()
        c.delegate = self
        func action(_ a: NoticeAction, _ title: String) -> UNNotificationAction {
            UNNotificationAction(identifier: a.rawValue, title: title, options: [])
        }
        func category(_ id: String, _ actions: [UNNotificationAction]) -> UNNotificationCategory {
            UNNotificationCategory(identifier: id, actions: actions, intentIdentifiers: [], options: [])
        }
        c.setNotificationCategories([
            category("down", [action(.start, "Start production"), action(.log, "Show log")]),
            category("silent", [action(.restart, "Restart production"), action(.log, "Show log")]),
            category("deaf", [action(.restart, "Restart production")]),
            category("outside", [action(.restart, "Restart from Clairance")]),
            category("archive", [action(.finder, "Show in Finder")]),
            category("instance", [action(.stop, "Stop it…")]),
            category("helper", [action(.stop, "Stop it…")]),
            category("permission", [action(.settings, "Open Local Network Settings…")]),
        ])
        c.requestAuthorization(options: [.alert, .sound]) { granted, error in
            Task { @MainActor in
                self.allowed = granted
                self.refusal = error.map { ($0 as NSError).localizedDescription + " (\(($0 as NSError).domain) \(($0 as NSError).code))" } ?? ""
            }
        }
    }

    func post(_ id: String, thread: String, _ n: Notice) {
        let content = UNMutableNotificationContent()
        content.title = n.title
        content.body = n.body
        content.threadIdentifier = thread
        content.categoryIdentifier = n.category
        if n.sound { content.sound = .default }
        switch n.level {
        case .timeSensitive: content.interruptionLevel = .timeSensitive
        case .active: content.interruptionLevel = .active
        case .passive: content.interruptionLevel = .passive
        }
        var info = n.target
        switch n.click {
        case .none: break
        case .log: info["click"] = "log"
        case .clairance: info["click"] = "clairance"
        case .finder(let path): info["click"] = "finder:" + path
        case .activityMonitor: info["click"] = "activity"
        case .settings(let pane): info["click"] = "settings:" + pane
        }
        content.userInfo = info
        UNUserNotificationCenter.current().add(UNNotificationRequest(identifier: id, content: content, trigger: nil))
    }

    // Shown even while the launcher is the active app.
    nonisolated func userNotificationCenter(_ center: UNUserNotificationCenter, willPresent notification: UNNotification,
                                            withCompletionHandler done: @escaping (UNNotificationPresentationOptions) -> Void) {
        done([.banner, .list, .sound])
    }

    nonisolated func userNotificationCenter(_ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse,
                                            withCompletionHandler done: @escaping () -> Void) {
        let info = response.notification.request.content.userInfo as? [String: String] ?? [:]
        let action = NoticeAction(rawValue: response.actionIdentifier)
        let c = info["click"] ?? ""
        let click: Notice.Click = c == "log" ? .log : c == "clairance" ? .clairance : c == "activity" ? .activityMonitor
            : c.hasPrefix("finder:") ? .finder(String(c.dropFirst("finder:".count)))
            : c.hasPrefix("settings:") ? .settings(String(c.dropFirst("settings:".count))) : .none
        Task { @MainActor in self.handle(action, click, info) }
        done()
    }
}
