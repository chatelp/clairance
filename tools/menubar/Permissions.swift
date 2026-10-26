// The permissions the launcher needs, asked for at its first launch and shown
// while one is missing (owner's request, 26/10): notifications, and the local
// network for the launcher itself (the station's page) and for Clairance
// Carrier (production). macOS does not say whether an app may reach the local
// network; the launcher tells it from what reaches the station: itself, or a
// carrier it opens for that alone.

import AppKit
import UserNotifications

struct Permission: Hashable, Sendable {
    let app: String        // "Clairance", "Clairance Server"
    let what: String       // "notifications", "local network"
    let settings: String   // the System Settings pane that grants it

    /// "▲ Clairance Server · local network not allowed" (design 10): who, then what.
    var line: String { "▲ \(app) · \(what) not allowed" }
}

let localNetworkSettings = "x-apple.systempreferences:com.apple.preference.security?Privacy_LocalNetwork"
let notificationSettings = "x-apple.systempreferences:com.apple.Notifications-Settings.extension"

extension Notifier {
    /// Whether macOS lets the launcher notify, asking the first time.
    func authorization() async -> Bool? {
        let settings = await UNUserNotificationCenter.current().notificationSettings()
        switch settings.authorizationStatus {
        case .authorized, .provisional, .ephemeral: return true
        case .denied: return false
        case .notDetermined:
            return try? await UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound])
        @unknown default: return nil
        }
    }
}

extension Production {
    /// Opens a carrier that only probes the station, and reads its answer: the
    /// same way a production's carrier begins, the same app, without a production.
    func probeCarrier() async -> Bool? {
        guard let station = cfg.station_status_url, let carrier = carrierCopy() else { return nil }
        let log = (supportDir as NSString).appendingPathComponent("probe.log")
        if !FileManager.default.fileExists(atPath: log) { FileManager.default.createFile(atPath: log, contents: nil) }
        let tail = LogTail(log)
        var args = ["-n", "-g"]
        if let dir = ProcessInfo.processInfo.environment["CLAIRANCE_SUPPORT_DIR"] { args += ["--env", "CLAIRANCE_SUPPORT_DIR=" + dir] }
        args += [carrier, "--args", "--carry", "--cwd", "/", "--log", log, "--probe", station, "--", "/usr/bin/true"]
        guard run("/usr/bin/open", args).status == 0 else { return nil }
        for _ in 0..<120 { // 30 s: the carrier's probe gives up after 20
            try? await Task.sleep(nanoseconds: 250_000_000)
            for line in tail.newLines() {
                if line.contains("carrier: the station answered") { return true }
                if line.contains("carrier: the station did not answer") { return false }
            }
        }
        return nil
    }

    /// What is missing, now: asked for when macOS has not been asked yet.
    func checkPermissions() async {
        guard serving, !checkingPermissions else { return }
        checkingPermissions = true
        defer {
            checkingPermissions = false
            permissionsCheckedAt = Date()
        }
        var missing: [Permission] = []
        if await notifier.authorization() == false {
            missing.append(Permission(app: "Clairance", what: "notifications", settings: notificationSettings))
        }
        // The station answering one and not the other says which lacks the local network;
        // neither answering says nothing about permissions.
        if cfg.station_status_url != nil {
            let carrier = await probeCarrier()
            let launcher = observation.station.answered
            if carrier == false && launcher {
                missing.append(Permission(app: carrierName, what: "local network", settings: localNetworkSettings))
            } else if carrier == true && !launcher {
                missing.append(Permission(app: "Clairance", what: "local network", settings: localNetworkSettings))
            }
        }
        permissions = missing
    }

    /// Checked again every 2 min while something is missing, every 10 min otherwise.
    func permissionsDue() -> Bool {
        let wait: TimeInterval = permissions.isEmpty ? 600 : 120
        return Date().timeIntervalSince(permissionsCheckedAt ?? .distantPast) > wait
    }
}
