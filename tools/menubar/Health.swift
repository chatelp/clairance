// The guard: production's health at every refresh, and the incidents that
// open and close on it (docs-fr/34-lanceur.md, step 1b). The forms, the words
// and the notifications are docs-fr/design/10-lanceur.md.
//
// Every check says a measured fact: a duration read from the server's own
// clocks (/api/v1/health/detail), a byte counter, a process that is there or
// not. An incident opens once its condition has held for the check's time,
// and closes once it has been gone for 10 s; its notification is sent when it
// opens and replaced when it closes. Each is written to events.jsonl beside
// the launcher's socket, which is how detection times are measured.

import AppKit
import Darwin

// MARK: - What the server says

/// /api/v1/health/detail, the part the guard reads
/// (internal/api/health_detail_handlers.go, internal/frequencies/health.go).
struct HealthDetail: Decodable, Sendable {
    struct ADSB: Decodable, Sendable {
        let ok: Bool
        let last_fetch: Date?
        let failing_since: Date?
        let failing_why: String?
        let aircraft: Int
        let messages_per_second: Double
    }
    struct Frequency: Decodable, Sendable {
        let id: String
        let name: String
        let status: String
        let transcribes: Bool
        let bytes_last_60s: Int64
        let last_byte_at: Date?
        let last_segment_at: Date?
        let last_transcribed_at: Date?
    }
    struct Sidecar: Decodable, Sendable {
        struct Queue: Decodable, Sendable {
            let first: Int
            let second: Int
        }
        let reachable: Bool
        let status: String
        let degraded: [String]?
        let pid: Int32?
        let queue: Queue?
    }
    struct Archive: Decodable, Sendable {
        let available: Bool
        let bytes: Int64?
        let cap_bytes: Int64
    }
    struct Databases: Decodable, Sendable {
        let disk_free_bytes: Int64?
    }
    let pid: Int32
    let uptime_s: Int64
    let adsb: ADSB?
    let frequencies: [Frequency]
    let sidecar: Sidecar?
    let audio_archive: Archive
    let databases: Databases

    /// The server writes RFC 3339 times, with or without fractions of a second.
    static func decode(_ data: Data) -> HealthDetail? {
        let d = JSONDecoder()
        let precise = ISO8601DateFormatter()
        precise.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let plain = ISO8601DateFormatter()
        d.dateDecodingStrategy = .custom { decoder in
            let s = try decoder.singleValueContainer().decode(String.self)
            if let t = precise.date(from: s) ?? plain.date(from: s) { return t }
            throw DecodingError.dataCorrupted(.init(codingPath: decoder.codingPath, debugDescription: "not a time: \(s)"))
        }
        return try? d.decode(HealthDetail.self, from: data)
    }
}

/// What the station says of itself (radio-ctl's /radio/etat), read by the launcher.
struct StationRead: Sendable {
    var answered = false
    var mode = ""                 // "chaine" when the airband chain is on the air
    var title = "—"
    var onAir: [String] = []      // "124.350", as the station designates them
}

// MARK: - The brand's words and numbers (design 10)

/// ● good, ▲ fault, ■ down, ○ unknown, – off by choice, ◐ switching: a shape
/// and a word, never a colour alone.
enum Light: String, Sendable {
    case good = "●", fault = "▲", down = "■", unknown = "○", off = "–", switching = "◐"
}

struct HealthLine: Hashable, Sendable {
    let light: Light
    let name: String
    let fact: String
    var text: String { "\(light.rawValue) \(name) · \(fact)" }
}

/// The sign's six states, most urgent first.
enum SignState: Sendable {
    case switching, down, fault, onAir, running, stopped
}

/// "1 200": a narrow no-break space between thousands.
func brandNumber(_ n: Int) -> String {
    let f = NumberFormatter()
    f.numberStyle = .decimal
    f.groupingSeparator = "\u{202F}"
    f.usesGroupingSeparator = true
    return f.string(from: NSNumber(value: n)) ?? String(n)
}

/// "212 GB", "9.4 GB" (decimal gigabytes, as the Finder counts them).
func brandGB(_ bytes: Int64) -> String {
    let gb = Double(bytes) / 1e9
    if gb < 10 { return String(format: "%.1f GB", gb).replacingOccurrences(of: ".0 GB", with: " GB") }
    return "\(brandNumber(Int(gb.rounded()))) GB"
}

/// "45 s" below a minute, then shortElapsed.
func brandDuration(_ seconds: TimeInterval) -> String {
    shortElapsed(seconds)
}

/// "124.350" from the station's id "124350"; otherwise the frequency's name.
func frequencyLabel(_ f: HealthDetail.Frequency) -> String {
    if f.id.count == 6, f.id.allSatisfy(\.isNumber) { return "\(f.id.prefix(3)).\(f.id.suffix(3))" }
    return f.name
}

func languageName(_ code: String) -> String {
    ["fr": "French", "en": "English"][code] ?? code
}

/// A cause of 60 characters at most per line, never with an ellipsis: cut
/// after a colon when both halves fit, else at the word that best balances them.
func causeLines(_ s: String) -> [String] {
    guard s.count > 60 else { return [s] }
    let chars = Array(s)
    var best: (score: Int, at: Int)?
    for i in chars.indices where chars[i] == " " {
        let first = i, second = chars.count - i - 1
        guard first <= 60, second <= 60 else { continue }
        let score = (i > 0 && chars[i - 1] == ":" ? 0 : 1000) + abs(first - second)
        if best == nil || score < best!.score { best = (score, i) }
    }
    guard let at = best?.at else { return [String(chars.prefix(60)), String(chars.dropFirst(60))] }
    return [String(chars[..<at]), String(chars[(at + 1)...])]
}

// MARK: - Memory, read by PID

/// What a process costs in memory, as Activity Monitor counts it.
func footprint(_ pid: Int32) -> Int64? {
    var info = rusage_info_v2()
    let r = withUnsafeMutablePointer(to: &info) {
        $0.withMemoryRebound(to: rusage_info_t?.self, capacity: 1) { proc_pid_rusage(pid, RUSAGE_INFO_V2, $0) }
    }
    return r == 0 ? Int64(info.ri_phys_footprint) : nil
}

/// macOS's memory pressure: 1 normal, 2 warning, 4 critical.
func memoryPressure() -> Int32 {
    var level: Int32 = 0
    var size = MemoryLayout<Int32>.size
    return sysctlbyname("kern.memorystatus_vm_pressure_level", &level, &size, nil, 0) == 0 ? level : 0
}

// MARK: - Incidents

/// What a notification says and does (design 10, "Les notifications").
struct Notice: Sendable {
    enum Level: Sendable { case timeSensitive, active, passive }
    enum Click: Sendable { case none, log, clairance, finder(String), activityMonitor }
    var title: String
    var body: String
    var sound = false
    var level = Level.active
    var click = Click.none
    var category = ""           // its buttons, see Notify.swift
    var target: [String: String] = [:]
}

/// One check's verdict this round: its condition holds, and what it says.
struct Finding: Sendable {
    let key: String             // the incident's identity, and its notification's
    let kind: String            // its thread in the notification centre
    var hold: TimeInterval = 0  // how long the condition must hold before it opens
    var down = false            // ■ rather than ▲
    var production = true       // only production's incidents change the sign
    var quiet = false           // shown in the menu, not notified
    var began: Date? = nil      // when the condition began, if measured; else when the guard first saw it
    let cause: String           // the menu's cause, one sentence
    let notice: Notice
    /// What replaces the notification when it ends, from how long it lasted
    /// and the facts of the round in which it ended.
    var recovery: (@Sendable (TimeInterval, [String: String]) -> Notice)? = nil
}

struct Incident: Sendable {
    let finding: Finding
    let began: Date
    let opened: Date
}

/// Opens and closes incidents from each round's findings.
@MainActor
final class Guard {
    private(set) var open: [String: Incident] = [:]
    private var since: [String: Date] = [:]
    private var clearing: [String: Date] = [:]
    var post: (String, String, Notice) -> Void = { _, _, _ in } // key, thread, notice
    private let journal = (supportDir as NSString).appendingPathComponent("events.jsonl")

    /// When an open incident's condition began.
    func began(_ key: String) -> Date? { open[key]?.began }

    func update(_ findings: [Finding], facts: [String: String] = [:], now: Date = Date()) {
        let present = Set(findings.map(\.key))
        for f in findings {
            clearing[f.key] = nil
            let begun = since[f.key] ?? now
            since[f.key] = begun
            if open[f.key] == nil {
                guard now.timeIntervalSince(begun) >= f.hold else { continue }
                let i = Incident(finding: f, began: f.began ?? begun, opened: now)
                open[f.key] = i
                write("open", f.key, f.notice, began: i.began)
                if !f.quiet { post(f.key, f.kind, f.notice) }
            } else if let i = open[f.key] {
                open[f.key] = Incident(finding: f, began: i.began, opened: i.opened) // keep the menu's cause current
            }
        }
        for key in since.keys where !present.contains(key) { since[key] = nil }
        for (key, i) in open where !present.contains(key) {
            let c = clearing[key] ?? now
            clearing[key] = c
            guard now.timeIntervalSince(c) >= 10 else { continue }
            open[key] = nil
            clearing[key] = nil
            let lasted = c.timeIntervalSince(i.began)
            if let r = i.finding.recovery, !i.finding.quiet {
                let n = r(lasted, facts)
                write("close", key, n, began: i.began)
                post(key, i.finding.kind, n)
            } else {
                write("close", key, nil, began: i.began)
            }
        }
    }

    /// Ends an incident without a word: "Leave stopped".
    func dismiss(_ key: String) {
        guard open[key] != nil else { return }
        open[key] = nil
        since[key] = nil
        clearing[key] = nil
        write("dismiss", key, nil, began: nil)
    }

    /// A notification that is no incident: "Transcription restarted".
    func event(_ key: String, thread: String, _ n: Notice) {
        write("event", key, n, began: nil)
        post(key + "." + String(Int(Date().timeIntervalSince1970)), thread, n)
    }

    private func write(_ event: String, _ key: String, _ n: Notice?, began: Date?) {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        var line: [String: String] = ["at": f.string(from: Date()), "event": event, "key": key]
        if let b = began { line["began"] = f.string(from: b) }
        if let n {
            line["title"] = n.title
            line["body"] = n.body
        }
        guard var data = try? JSONSerialization.data(withJSONObject: line, options: [.sortedKeys]) else { return }
        data.append(0x0A)
        if !FileManager.default.fileExists(atPath: journal) { FileManager.default.createFile(atPath: journal, contents: nil) }
        guard let h = FileHandle(forWritingAtPath: journal) else { return }
        h.seekToEndOfFile()
        h.write(data)
        try? h.close()
    }
}
