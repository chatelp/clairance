// Clairance menu bar launcher.
//
// An icon in the menu bar that says whether production runs, starts and stops
// it, and opens the page. Below, every co-atc instance running on this Mac,
// production and test alike, each with what it serves, its transcription, its
// station link, its log, and the app it depends on for macOS; and what was left
// running without an instance. It does not run production itself: it runs the
// same script a double-click runs (production_command, runs/production.command
// on the owner's station), so the sign-in, the station link and the clean stop
// stay in one place. Stopping sends that script SIGTERM, which its trap turns
// into a clean stop of co-atc, its sidecar and the station link. Nothing starts
// on its own: production runs only when asked (owner's choice, 10/10).
//
// What it starts runs in a carrier, an instance of Clairance.app of its own
// (Entry.carry), so that for macOS production depends on neither the menu nor
// whatever typed the command: `clairance --start` asks the running launcher,
// opening it first if needed (Control.swift). Processes
// are known by their PID, their exact arguments and their start time, never by
// a pattern in a command line (Processes.swift).
//
// Settings, all optional, in ~/.config/clairance/launcher.json:
//   repo                the Clairance checkout (default: where it was built)
//   production_command  relative to repo (default: runs/production.command)
//   production_config   the config that command starts co-atc with, relative
//                       to repo (default: configs/config.toml): the instance
//                       running it is production
//   production_log      the log that command writes, relative to repo (default:
//                       runs/production.log); launcher.log goes beside it
//   station_status_url  radio-ctl's /radio/etat, for the line "On air"
//   tailscale_url       copied by "Copy remote address"
//   planning_file       a markdown table of who uses the GPU; production adds
//                       its row while it runs and removes it when it stops
//   silence_minutes     how long nothing transcribed is a fault (default: 10
//                       from 8 h to 23 h, 45 at night; Checks.swift)
//
// A test bench runs a second launcher on its own settings and folder:
// CLAIRANCE_LAUNCHER_CONFIG names its launcher.json, CLAIRANCE_SUPPORT_DIR the
// folder of its socket and journals.

import AppKit
import SwiftUI

struct LauncherConfig: Decodable {
    var repo: String?
    var production_command: String?
    var production_config: String?
    var production_log: String?
    var station_status_url: String?
    var tailscale_url: String?
    var planning_file: String?
    var silence_minutes: Double?
}

/// Replaced by build.sh with the checkout the app was built from.
let builtRepo = "@REPO@"

let launcherBundleID = "dev.clairance.launcher"
let planningMarker = "(menu bar launcher)"

func loadLauncherConfig() -> LauncherConfig {
    let path = ProcessInfo.processInfo.environment["CLAIRANCE_LAUNCHER_CONFIG"].map { URL(fileURLWithPath: $0) }
        ?? FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent(".config/clairance/launcher.json")
    guard let data = try? Data(contentsOf: path),
          let c = try? JSONDecoder().decode(LauncherConfig.self, from: data) else { return LauncherConfig() }
    return c
}

@discardableResult
func run(_ path: String, _ args: [String]) -> (status: Int32, out: String) {
    let p = Process()
    p.executableURL = URL(fileURLWithPath: path)
    p.arguments = args
    let pipe = Pipe()
    p.standardOutput = pipe
    p.standardError = Pipe()
    do { try p.run() } catch { return (-1, "") }
    let data = pipe.fileHandleForReading.readDataToEndOfFile()
    p.waitUntilExit()
    return (p.terminationStatus, String(decoding: data, as: UTF8.self))
}

// MARK: - Every instance on this Mac

/// A co-atc process, production or test, and what goes with it.
struct Instance: Identifiable, Sendable {
    var id: Int32 { proc.pid }
    var pid: Int32 { proc.pid }
    let proc: Proc
    let production: Bool
    let name: String            // "Production", or the config's name ("migration-fake")
    let config: String          // as given on its command line
    let configPath: String      // resolved from its working directory
    let port: Int?
    let extraPorts: [Int]
    let staticDir: String
    let www: String             // "migration @ cad595f", or "copy" when not a checkout
    let database: String
    let log: String
    let sidecarURL: String
    let sidecar: Proc?
    let sidecarKind: String     // "real model", "fake", or the command's name
    let stationLinks: [Helper]
    let script: Proc?           // the production script it runs under, if any
    let dependence: Dependence?
    let until: Date?            // when a test instance the launcher started must stop
    let adsbSource: String?     // what it reads its ADS-B from, when the launcher can read it too
    let archiveVolume: String?  // the volume of the audio archive, "Crucial X8"
    let coatcLog: String        // co-atc's own log, before the day's suffix
    // Read from the server and the sidecar after the scan.
    var answers = false         // its /api/v1/health answered
    var lastFetch: Date?        // its last ADS-B read, from /api/v1/health
    var version = ""
    var summary = ""            // vitalsLine, empty when the server did not answer
    var sidecarHealth = ""

    var elapsed: String { shortElapsed(proc.elapsed) }
    var cwd: String { proc.cwd }
    var url: String? { port.map { "http://127.0.0.1:\($0)/" } }

    /// "Production · port 8000", "Test · port 8020 · stops at 18:00",
    /// "▲ Test · port 8020 · 2 h past its end" (design 10).
    var title: String {
        let base = (production ? "Production" : "Test") + (port.map { " · port \($0)" } ?? " · " + name)
        guard let u = until else { return base }
        let left = u.timeIntervalSinceNow
        return left > 0 ? base + " · stops at \(clockTime(u))" : "▲ " + base + " · \(shortElapsed(-left)) past its end"
    }
}

/// A helper process: a sidecar or a station link.
struct Helper: Identifiable, Sendable {
    var id: Int32 { proc.pid }
    var pid: Int32 { proc.pid }
    let proc: Proc
    let what: String
    var elapsed: String { shortElapsed(proc.elapsed) }
    var command: String { proc.commandLine }
}

/// The few keys of a co-atc TOML config the launcher reads: section -> key -> raw value.
func readConfig(_ path: String) -> [String: [String: String]] {
    guard let text = try? String(contentsOfFile: path, encoding: .utf8) else { return [:] }
    var out: [String: [String: String]] = [:]
    var section = ""
    for raw in text.components(separatedBy: "\n") {
        // Drop a comment, unless the # is inside quotes.
        var line = ""
        var quoted = false
        for ch in raw {
            if ch == "\"" { quoted.toggle() }
            if ch == "#" && !quoted { break }
            line.append(ch)
        }
        line = line.trimmingCharacters(in: .whitespaces)
        if line.hasPrefix("[") && line.hasSuffix("]") && !line.contains("=") {
            section = String(line.dropFirst().dropLast())
            continue
        }
        guard let eq = line.firstIndex(of: "=") else { continue }
        let key = line[..<eq].trimmingCharacters(in: .whitespaces)
        var value = line[line.index(after: eq)...].trimmingCharacters(in: .whitespaces)
        if value.hasPrefix("\""), value.hasSuffix("\""), value.count >= 2 { value = String(value.dropFirst().dropLast()) }
        out[section, default: [:]][key] = value
    }
    return out
}

func resolve(_ path: String, from dir: String) -> String {
    guard !path.isEmpty else { return "" }
    let full = path.hasPrefix("/") ? path : (dir as NSString).appendingPathComponent(path)
    return (full as NSString).standardizingPath
}

/// lsof's name for one of the process's descriptors ("1" for its output).
func openPath(_ pid: Int32, fd: String) -> String {
    run("/usr/sbin/lsof", ["-a", "-p", String(pid), "-d", fd, "-Fn"]).out
        .split(separator: "\n").first(where: { $0.hasPrefix("n") }).map { String($0.dropFirst()) } ?? ""
}

func listener(onPort port: Int) -> Int32? {
    run("/usr/sbin/lsof", ["-nP", "-t", "-iTCP:\(port)", "-sTCP:LISTEN"]).out
        .split(separator: "\n").compactMap { Int32($0) }.first
}

/// The branch and commit of a www folder, when it is a checkout's own.
func wwwOrigin(_ staticDir: String) -> String {
    let top = run("/usr/bin/git", ["-C", staticDir, "rev-parse", "--show-toplevel"]).out
        .trimmingCharacters(in: .whitespacesAndNewlines)
    guard !top.isEmpty, resolve("www", from: top) == staticDir else { return "copy" }
    let branch = run("/usr/bin/git", ["-C", top, "rev-parse", "--abbrev-ref", "HEAD"]).out
        .trimmingCharacters(in: .whitespacesAndNewlines)
    let commit = run("/usr/bin/git", ["-C", top, "rev-parse", "--short", "HEAD"]).out
        .trimmingCharacters(in: .whitespacesAndNewlines)
    return "\(branch) @ \(commit)"
}

func sidecarKind(_ p: Proc) -> String {
    if p.runs("whisper_server.py") { return "real model" }
    if p.runs("fake-sidecar.py") { return "fake" }
    return p.name
}

/// Whether a station link reports to the co-atc listening on that port: its
/// -coatc argument, read as an address, not searched as text.
func feeds(_ link: Proc, port: Int) -> Bool {
    guard let s = link.argument(after: "-coatc"), let u = URL(string: s) else { return false }
    return u.port == port && ["127.0.0.1", "localhost", "::1"].contains(u.host ?? "")
}

/// The production script co-atc runs under: its parent, a shell running
/// production_command -- started by the launcher, or by a double-click in
/// Terminal, which runs it through the same shebang.
func productionScript(of p: Proc, script: String) -> Proc? {
    guard let parent = Proc.read(p.ppid), parent.args.count >= 2,
          ["sh", "bash", "zsh"].contains((parent.args[0] as NSString).lastPathComponent),
          resolve(parent.args[1], from: parent.cwd) == script else { return nil }
    return parent
}

/// What a scan needs to know, read on the main actor and handed to the scan's thread.
struct ScanSettings: Sendable {
    let productionConfig: String
    let productionScript: String
    let started: [Started]
}

/// Every co-atc process, and the sidecars and station links left without one.
/// Runs off the main thread: lsof and git, a few tens of milliseconds each.
func scanInstances(_ s: ScanSettings) -> (instances: [Instance], orphans: [Helper]) {
    let coAtc = Proc.all { $0 == "co-atc" }
    let links = Proc.all { $0 == "radio-ctl-sync" }.map {
        Helper(proc: $0, what: $0.args.contains("-once") ? "station link, once (should run continuously)" : "station link")
    }
    let sidecars = Proc.all { $0.lowercased().hasPrefix("python") }
        .filter { $0.runs("whisper_server.py") || $0.runs("fake-sidecar.py") }
        .map { Helper(proc: $0, what: "sidecar, \(sidecarKind($0))") }
    var claimed = Set<Int32>()
    var instances: [Instance] = []
    for p in coAtc {
        let cfgArg = p.argument(after: "-config") ?? ""
        let cfgPath = resolve(cfgArg, from: p.cwd)
        let cfg = readConfig(cfgPath)
        let server = cfg["server"] ?? [:]
        let port = server["port"].flatMap { Int($0) }
        let extra = (server["additional_ports"] ?? "").split(whereSeparator: { !$0.isNumber }).compactMap { Int($0) }
        let staticDir = resolve(server["static_dir"] ?? "www", from: p.cwd)
        let local = cfg["transcription.local"] ?? [:]
        let sidecarURL = local["server_url"] ?? ""
        let sidecar = URL(string: sidecarURL)?.port.flatMap { listener(onPort: $0) }.flatMap { Proc.read($0) }
        if let sc = sidecar { claimed.insert(sc.pid) }
        let mine = links.filter { h in port.map { feeds(h.proc, port: $0) } ?? false }
        mine.forEach { claimed.insert($0.pid) }
        let production = cfgPath == s.productionConfig
        var name = "Production"
        if !production {
            name = ((cfgPath as NSString).lastPathComponent as NSString).deletingPathExtension
            if name.hasPrefix("config-") { name = String(name.dropFirst("config-".count)) }
            if name == "config" { name = ((cfgPath as NSString).deletingLastPathComponent as NSString).lastPathComponent }
            name = "Test · " + name
        }
        let mark = s.started.first { $0.role == "test" && $0.pid == p.pid && $0.start == p.start }
        let adsb = cfg["adsb"] ?? [:]
        var source: String?
        if adsb["source_type"] == "tar1090", let base = adsb["tar1090_base_url"], !base.isEmpty {
            source = base.hasSuffix("/") ? base + "aircraft.json" : base + "/aircraft.json"
        }
        // The archive's volume: its folder in the config, else the sidecar's --save-audio.
        let archive = [local["audio_archive_dir"] ?? ""] + (sidecar?.args ?? [])
        let volume = archive.first { $0.hasPrefix("/Volumes/") }.map { $0.split(separator: "/")[1] }.map(String.init)
        instances.append(Instance(
            proc: p, production: production, name: name,
            config: cfgArg, configPath: cfgPath, port: port, extraPorts: extra,
            staticDir: staticDir, www: wwwOrigin(staticDir),
            database: resolve(cfg["storage"]?["sqlite_base_path"] ?? "", from: p.cwd),
            log: openPath(p.pid, fd: "1"),
            sidecarURL: sidecarURL, sidecar: sidecar,
            sidecarKind: sidecar.map { sidecarKind($0) } ?? "",
            stationLinks: mine,
            script: production ? productionScript(of: p, script: s.productionScript) : nil,
            dependence: dependence(of: p.pid),
            until: mark?.until,
            adsbSource: source, archiveVolume: volume,
            coatcLog: resolve(cfg["logging"]?["file"] ?? "", from: p.cwd)))
    }
    instances.sort { ($0.production ? 0 : 1, $0.port ?? 0) < ($1.production ? 0 : 1, $1.port ?? 0) }
    let orphans = (sidecars + links).filter { !claimed.contains($0.pid) }
    return (instances, orphans)
}

/// /api/v1/health/detail, open to this Mac without a session.
func healthDetail(_ base: String) async -> HealthDetail? {
    guard let url = URL(string: base + "api/v1/health/detail") else { return nil }
    var req = URLRequest(url: url)
    req.timeoutInterval = 3
    guard let (data, r) = try? await URLSession.shared.data(for: req), (r as? HTTPURLResponse)?.statusCode == 200 else { return nil }
    return HealthDetail.decode(data)
}

/// Whether an address answers at all, from the launcher itself.
func reachable(_ s: String) async -> Bool {
    guard let url = URL(string: s) else { return false }
    var req = URLRequest(url: url)
    req.timeoutInterval = 3
    return (try? await URLSession.shared.data(for: req)) != nil
}

/// An RFC 3339 time, with or without fractions of a second.
func parseTime(_ s: String) -> Date? {
    let precise = ISO8601DateFormatter()
    precise.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    return precise.date(from: s) ?? ISO8601DateFormatter().date(from: s)
}

/// A field of the JSON that ends a co-atc log line ({"error": "…"}).
func jsonField(_ line: String, _ key: String) -> String? {
    guard let brace = line.firstIndex(of: "{"),
          let j = try? JSONSerialization.jsonObject(with: Data(line[brace...].utf8)) as? [String: Any] else { return nil }
    return j[key] as? String
}

/// A file's last lines, from its last 16 KB.
func lastLines(_ path: String, _ n: Int) -> [String] {
    guard let h = FileHandle(forReadingAtPath: path) else { return [] }
    defer { try? h.close() }
    let size = (try? h.seekToEnd()) ?? 0
    try? h.seek(toOffset: size > 16_384 ? size - 16_384 : 0)
    let text = String(decoding: h.readDataToEndOfFile(), as: UTF8.self)
    return Array(text.split(separator: "\n").map(String.init).suffix(n))
}

func getJSON(_ s: String, timeout: Double = 3) async -> [String: Any]? {
    guard let url = URL(string: s) else { return nil }
    var req = URLRequest(url: url)
    req.timeoutInterval = timeout
    guard let (data, _) = try? await URLSession.shared.data(for: req) else { return nil }
    return try? JSONSerialization.jsonObject(with: data) as? [String: Any]
}

/// "103 aircraft · 5 437 transmissions today · 44 % matched"
func vitalsLine(_ v: [String: Any]) -> String {
    func n(_ k: String) -> Int? { (v[k] as? NSNumber)?.intValue }
    let f = NumberFormatter()
    f.numberStyle = .decimal
    f.locale = Locale(identifier: "fr_FR")
    var parts: [String] = []
    if let a = n("aircraft") { parts.append("\(a) aircraft") }
    if let t = n("transmissions_today") { parts.append("\(f.string(from: NSNumber(value: t)) ?? String(t)) transmissions today") }
    if let m = n("matched_share") { parts.append("\(m) % matched") }
    return parts.joined(separator: " · ")
}

func sidecarHealthLine(_ h: [String: Any]) -> String {
    let status = h["status"] as? String ?? "?"
    let degraded = (h["degraded"] as? [String]) ?? []
    let q = h["queue"] as? [String: Any]
    let queue = q.map { " · queue \(($0["first"] as? NSNumber)?.intValue ?? 0)/\(($0["second"] as? NSNumber)?.intValue ?? 0)" } ?? ""
    return (degraded.isEmpty ? status : "\(status): \(degraded.joined(separator: ", "))") + queue
}

/// "2h", "90m", "1h30", "45s"; a number alone is minutes.
func parseDuration(_ s: String) -> TimeInterval? {
    var total = 0.0
    var digits = ""
    for ch in s {
        if ch.isNumber { digits.append(ch); continue }
        guard let n = Double(digits) else { return nil }
        switch ch {
        case "h": total += n * 3600
        case "m": total += n * 60
        case "s": total += n
        default: return nil
        }
        digits = ""
    }
    if let n = Double(digits) { total += n * 60 }
    return total > 0 ? total : nil
}

func clockTime(_ d: Date) -> String {
    let f = DateFormatter()
    f.locale = Locale(identifier: "fr_FR")
    f.dateFormat = Calendar.current.isDateInToday(d) ? "HH:mm" : "dd/MM HH:mm"
    return f.string(from: d)
}

/// Where a start was asked for: a failed start is notified only when asked
/// outside the menu, which says it already (design 10).
enum Origin: Sendable { case menu, commandLine }

/// A start under way, followed until production answers or is gone.
struct Starting {
    let origin: Origin
    let at: Date
    let carrierLog: LogTail     // launcher.log, where the carrier says whether the station answered
    let coatcLog: LogTail?      // co-atc's own log of the day
    var coatcLines: [String] = []
    var sawCoAtc = false
    var stationSilent: Date?    // when the carrier wrote that the station did not answer

    /// co-atc wrote a fatal line since the start: it gave up, whether or not a scan saw it run.
    var fatal: String? { coatcLines.last { $0.contains("\tfatal\t") } }
}

@MainActor
final class Production: ObservableObject {
    @Published var running = false
    @Published var elapsed = ""
    @Published var onAir = "—"
    // The station's own mode word ("chaine" = the airband chain on the air).
    @Published var stationMode = ""
    @Published var sidecar = "—"
    @Published var instances: [Instance] = []
    @Published var orphans: [Helper] = []
    @Published var busy = false
    @Published var note = ""
    // The guard's view (design 10): the sign, the menu's first lines, the health lines.
    @Published var sign = SignState.stopped
    @Published var header: [String] = ["Production stopped"]
    @Published var health: [HealthLine] = []
    @Published var offerRestartFromClairance = false
    @Published var down: Down?

    let cfg: LauncherConfig
    let repo: String
    var command: String { cfg.production_command ?? "runs/production.command" }
    var script: String { resolve(command, from: repo) }
    var productionConfig: String { resolve(cfg.production_config ?? "configs/config.toml", from: repo) }
    var productionLog: String { resolve(cfg.production_log ?? "runs/production.log", from: repo) }
    var launcherLog: String { ((productionLog as NSString).deletingLastPathComponent as NSString).appendingPathComponent("launcher.log") }
    var production: Instance? { instances.first(where: \.production) }

    /// Only the launcher in the menu bar serves the control socket, starts
    /// anything, keeps what it started and guards production; the command line reads.
    let serving: Bool
    private var started: [Started]
    private var timer: Timer?
    let guardian = Guard()
    let notifier = Notifier()
    /// The launcher in the menu bar, for its app delegate (Settings.swift).
    static weak var shared: Production?
    /// A quit the launcher has confirmed, or that needs no confirmation.
    var quitConfirmed = false

    // What the guard follows between two refreshes.
    private var starting: Starting?
    private var stopRequested: Date?
    private var expected: Proc?           // the production last seen running
    private var linkSeen = false          // production had a station link
    private var linkLostAt: Date?
    private var sidecarPID: Int32?
    private var orphanSince: [Int32: Date] = [:]
    private(set) var observation = Observation()

    init(serving: Bool = false) {
        cfg = loadLauncherConfig()
        repo = cfg.repo ?? builtRepo
        self.serving = serving
        started = loadStarted()
        if serving {
            Production.shared = self
            // Asked once the app has finished launching: asked during it, macOS refused.
            Task { @MainActor in
                try? await Task.sleep(nanoseconds: 2_000_000_000)
                self.notifier.setUp()
            }
            notifier.handle = { [weak self] action, click, target in self?.answer(action, click, target) }
            guardian.post = { [weak self] key, thread, n in self?.notifier.post(key, thread: thread, n) }
            timer = Timer.scheduledTimer(withTimeInterval: 5, repeats: true) { [weak self] _ in
                Task { @MainActor in await self?.refresh() }
            }
            serveControl { [weak self] request, reply in
                guard let self else { return reply(.end(false, "the launcher is closing")) }
                await self.handle(request, reply: reply)
            }
            Task { await refresh() }
        }
    }

    private var refreshing = false

    func refresh() async {
        guard !refreshing else { return }
        refreshing = true
        defer { refreshing = false }
        let settings = ScanSettings(productionConfig: productionConfig, productionScript: script, started: started)
        let scan = await Task.detached { scanInstances(settings) }.value
        var found = scan.instances
        var o = Observation()
        for i in found.indices {
            if let u = found[i].url {
                if let h = await getJSON(u + "api/v1/health") {
                    found[i].answers = true
                    found[i].lastFetch = (h["last_fetch"] as? String).flatMap(parseTime)
                }
                if let v = await getJSON(u + "api/v1/vitals") {
                    found[i].version = v["version"] as? String ?? ""
                    found[i].summary = vitalsLine(v)
                    if found[i].production { o.secondsPerTransmission = (v["seconds_per_transmission"] as? NSNumber)?.doubleValue }
                }
                if found[i].production, found[i].answers { o.detail = await healthDetail(u) }
            }
            if !found[i].sidecarURL.isEmpty {
                found[i].sidecarHealth = await getJSON(found[i].sidecarURL + "/health").map(sidecarHealthLine) ?? "not answering"
            }
        }
        instances = found
        orphans = scan.orphans
        let prod = production
        running = prod != nil
        elapsed = prod?.elapsed ?? ""
        sidecar = prod.map { $0.sidecarURL.isEmpty ? "—" : $0.sidecarHealth } ?? "—"
        o.station = await stationRead()
        onAir = o.station.title
        stationMode = o.station.mode

        // The rest is the launcher's own: what it started, and the guard.
        o.production = prod
        o.tests = found.filter { !$0.production }
        o.pressure = memoryPressure()
        o.coatcMemory = prod.flatMap { footprint($0.pid) }
        o.sidecarMemory = (o.detail?.sidecar?.pid ?? prod?.sidecar?.pid).flatMap(footprint)
        o.archiveVolume = prod?.archiveVolume
        o.dataDir = prod?.database ?? repo
        o.silenceLimit = cfg.silence_minutes.map { $0 * 60 } ?? silenceLimit(at: o.now)
        // The launcher reads production's ADS-B source itself only when co-atc reads nothing,
        // to say whether the station or co-atc's local network is at fault.
        if let p = prod, let src = p.adsbSource,
           Date().timeIntervalSince(o.detail?.adsb?.last_fetch ?? p.lastFetch ?? .distantPast) > deafAfter {
            o.sourceAnswersLauncher = await reachable(src)
        }
        if serving {
            stopExpired()
            forgetGone()
            follow(prod, &o)
        }
        observation = o
        let (findings, lines, facts) = assess(o)
        health = lines
        if serving { guardian.update(findings + assessOthers(o), facts: facts) }
        updateSign(findings)
    }

    /// Follows production between refreshes: a start under way, a stop asked
    /// for, a production that stopped by itself, its station link and sidecar.
    private func follow(_ prod: Instance?, _ o: inout Observation) {
        let now = o.now
        if var s = starting {
            if s.carrierLog.newLines().contains(where: { $0.contains("carrier: the station did not answer") }) { s.stationSilent = now }
            s.coatcLines += s.coatcLog?.newLines() ?? []
            if let p = prod {
                s.sawCoAtc = true
                if p.answers {
                    // Switching ends as soon as co-atc answers (design 10).
                    starting = nil
                    busy = false
                    note = ""
                    down = nil
                } else {
                    starting = s
                }
            } else {
                let scriptGone = started.last(where: { $0.role == "production" }).map { Proc.read($0.pid)?.start != $0.start } ?? true
                if s.sawCoAtc || scriptGone || s.fatal != nil || s.stationSilent.map({ now.timeIntervalSince($0) > 10 }) == true
                    || now.timeIntervalSince(s.at) > 300 {
                    // A new failure replaces the last one's notification, with its own cause.
                    if case .didNotStart = down { guardian.dismiss("production-start") }
                    down = .didNotStart(at: s.at, cause: failedStartCause(s), notify: s.origin == .commandLine)
                    // The failed start's script goes too: still waiting for a co-atc, it would
                    // take the next start's for its own and start a second station link
                    // (seen on the bench, 24/10).
                    if let st = started.last(where: { $0.role == "production" }), let sp = Proc.read(st.pid), sp.start == st.start {
                        terminate(sp)
                    }
                    starting = nil
                    busy = false
                    note = ""
                } else {
                    starting = s
                }
            }
        }
        if let p = prod {
            if expected?.pid != p.pid || expected?.start != p.proc.start {
                expected = p.proc
                linkSeen = false
                linkLostAt = nil
                sidecarPID = nil
                if starting == nil, case .stoppedByItself = down { down = nil }
            }
            if !p.stationLinks.isEmpty {
                linkSeen = true
                linkLostAt = nil
            } else if linkSeen && linkLostAt == nil {
                linkLostAt = now
            }
            if let sp = o.detail?.sidecar?.pid ?? p.sidecar?.pid {
                if let old = sidecarPID, old != sp {
                    guardian.event("sidecar-restarted", thread: "transcription",
                                   Notice(title: "Transcription restarted", body: "The sidecar stopped and co-atc started it again.",
                                          level: .passive, click: .log))
                }
                sidecarPID = sp
            }
            if busy && note == "Stopping…" { stopRequested = stopRequested ?? now }
        } else if expected != nil {
            // Gone: asked for (the launcher's stop, or the script's own "arret demande"), or by itself.
            expected = nil
            let asked = stopRequested.map { now.timeIntervalSince($0) < 300 } ?? false
            if !asked && !scriptSaysStopAsked() && starting == nil { down = .stoppedByItself(at: now) }
            stopRequested = nil
            if busy && note == "Stopping…" {
                busy = false
                note = ""
            }
        } else if busy && note == "Stopping…" {
            busy = false
            note = ""
        }
        o.linkLostAt = linkLostAt
        o.down = down
        let present = Set(orphans.map(\.pid))
        orphanSince = orphanSince.filter { present.contains($0.key) }
        for h in orphans where orphanSince[h.pid] == nil { orphanSince[h.pid] = now }
        o.orphans = orphans.map { Orphan(helper: $0, since: orphanSince[$0.pid] ?? now, memory: footprint($0.pid)) }
    }

    /// The production script writes "arret demande" when asked to stop, by
    /// whoever asked; its last one, after its last "co-atc pret", means production
    /// was stopped on purpose.
    private func scriptSaysStopAsked() -> Bool {
        let tail = lastLines(productionLog, 40)
        guard let stop = tail.lastIndex(where: { $0.contains("arret demande") }) else { return false }
        let ready = tail.lastIndex(where: { $0.contains("co-atc pret") }) ?? -1
        return stop > ready
    }

    /// A failed start's cause, as far as the launcher can tell it (design 10).
    // "The station does not answer" only when the station did not answer: the
    // carrier said so, or co-atc's ADS-B check failed on a connection. On 24/10
    // the bench's check failed on a missing receiver.json, which the station's
    // silence would have hidden.
    private func failedStartCause(_ s: Starting) -> String {
        let station = "The station does not answer: co-atc stopped at its ADS-B check"
        if s.stationSilent != nil { return station }
        if let line = s.coatcLines.last(where: { $0.contains("ADS-B source validation failed") }) {
            let error = jsonField(line, "error") ?? ""
            let unreached = ["no route to host", "connection refused", "i/o timeout", "Client.Timeout", "deadline exceeded", "no such host"]
            if unreached.contains(where: error.contains) { return station }
            return "co-atc stopped at its ADS-B check" + (error.isEmpty ? "" : ": " + error)
        }
        if let f = s.fatal {
            let what = f.components(separatedBy: "\tfatal\t").last?.components(separatedBy: "\t").first ?? ""
            let error = jsonField(f, "error").map { ": " + $0 } ?? ""
            return "co-atc stopped while starting: " + what + error
        }
        let last = lastLines(productionLog, 20).last { !$0.trimmingCharacters(in: .whitespaces).isEmpty } ?? ""
        return "co-atc stopped while starting" + (last.isEmpty ? "" : ": " + last)
    }

    /// The sign, and the menu's first lines (design 10): the state, since when, its cause.
    private func updateSign(_ findings: [Finding]) {
        let open = serving ? guardian.open.values.filter(\.finding.production).map(\.finding) : findings.filter { $0.hold == 0 }
        func began(_ key: String) -> Date { guardian.began(key) ?? Date() }
        offerRestartFromClairance = false
        if busy {
            sign = .switching
            header = [note == "Stopping…" ? "Production stopping…" : "Production starting…"]
        } else if let d = open.first(where: \.down) {
            sign = .down
            switch d.key {
            case "production-start": header = ["■ Production did not start"] + causeLines(d.cause)
            case "production-stopped": header = ["■ Production down · stopped by itself \(brandDuration(Date().timeIntervalSince(began(d.key)))) ago"]
            default: header = ["■ Production not answering · \(brandDuration(Date().timeIntervalSince(began(d.key))))"]
            }
        } else if let deaf = open.first(where: { $0.key == "deaf" }) {
            sign = .fault
            if deaf.cause.hasPrefix("The app that started it quit") {
                header = ["▲ Production without local network"] + causeLines(deaf.cause)
            } else {
                header = ["▲ Production deaf · \(brandDuration(Date().timeIntervalSince(began("deaf"))))"] + causeLines(deaf.cause)
            }
            offerRestartFromClairance = deaf.notice.category == "deaf"
        } else if let first = open.first {
            sign = .fault
            if let out = open.first(where: { $0.key == "outside" }) {
                header = ["Production running · \(elapsed)", "▲ " + out.cause]
                offerRestartFromClairance = true
            } else {
                header = ["▲ Production running · \(elapsed)"] + causeLines(first.cause)
            }
        } else if running {
            sign = stationMode == "chaine" ? .onAir : .running
            header = ["Production running · \(elapsed)"]
        } else {
            sign = .stopped
            header = ["Production stopped"]
        }
    }

    /// For VoiceOver: what the sign says (design 10).
    var signLabel: String {
        let cause = header.dropFirst().first ?? ""
        switch sign {
        case .stopped: return "Clairance: production stopped"
        case .switching: return note == "Stopping…" ? "Clairance: production stopping" : "Clairance: production starting"
        case .running: return "Clairance: production running"
        case .onAir: return "Clairance: production running, on air"
        case .fault: return "Clairance: production running with a fault: " + (cause.isEmpty ? header[0] : cause)
        case .down: return "Clairance: production down: " + (cause.isEmpty ? header[0] : cause)
        }
    }

    /// "Leave stopped": the failure was seen, the sign goes back to Stopped.
    func leaveStopped() {
        down = nil
        guardian.dismiss("production-start")
        guardian.dismiss("production-stopped")
        Task { await refresh() }
    }

    // MARK: Production

    // What the launcher starts runs in a carrier (Entry.carry), an instance of
    // Clairance.app of its own, and depends on it rather than on the menu: the
    // menu can then quit, crash or be updated without production losing the
    // station (Q59, measured on 24/10).
    func start(from origin: Origin = .menu) async {
        guard serving, !running, !busy else { return }
        guard FileManager.default.isExecutableFile(atPath: script) else {
            note = "Not found: \(script)"
            return
        }
        busy = true
        note = "Starting…"
        updateSign([])
        let log = launcherLog
        // co-atc's own log of the day (pkg/logger: <base>-<day><ext>), where a failed ADS-B check is written.
        let file = resolve(readConfig(productionConfig)["logging"]?["file"] ?? "", from: repo)
        let day = DateFormatter()
        day.dateFormat = "yyyy-MM-dd"
        let coatcLog = file.isEmpty ? nil : LogTail(((file as NSString).deletingPathExtension) + "-" + day.string(from: Date())
                                                     + ((file as NSString).pathExtension.isEmpty ? "" : "." + (file as NSString).pathExtension))
        starting = Starting(origin: origin, at: Date(), carrierLog: LogTail(log), coatcLog: coatcLog)
        guard let pid = await carry(["/bin/sh", script], log: log), let proc = Proc.read(pid) else {
            starting = nil
            busy = false
            note = "Could not start: " + lastLine(log)
            return
        }
        remember(Started(role: "production", pid: proc.pid, start: proc.start, config: productionConfig))
        setPlanningRow(true)
        // Every second until production answers or is gone: switching ends there, not at the next round.
        Task {
            while starting != nil {
                try? await Task.sleep(nanoseconds: 1_000_000_000)
                await refresh()
            }
        }
    }

    /// Opens a carrier that runs the command, from the checkout, its output
    /// appended to the log; returns the command's PID once the carrier has
    /// started it. The carrier first waits for the station to answer it.
    private func carry(_ command: [String], log: String) async -> Int32? {
        let bundle = Bundle.main.bundleURL
        guard bundle.pathExtension == "app" else { return nil }
        let pidfile = (supportDir as NSString).appendingPathComponent("carried-\(UUID().uuidString).pid")
        defer { unlink(pidfile) }
        var args = ["-n", "-g"]
        // A test bench's carrier answers to the bench's launcher, not the owner's.
        if let dir = ProcessInfo.processInfo.environment["CLAIRANCE_SUPPORT_DIR"] { args += ["--env", "CLAIRANCE_SUPPORT_DIR=" + dir] }
        args += [bundle.path, "--args", "--carry", "--cwd", repo, "--log", log, "--pidfile", pidfile]
        if let probe = cfg.station_status_url { args += ["--probe", probe] }
        guard run("/usr/bin/open", args + ["--"] + command).status == 0 else { return nil }
        for _ in 0..<120 { // 30 s, the carrier's wait for the station included
            if let s = try? String(contentsOfFile: pidfile, encoding: .utf8),
               let pid = Int32(s.trimmingCharacters(in: .whitespacesAndNewlines)) { return pid }
            try? await Task.sleep(nanoseconds: 250_000_000)
        }
        return nil
    }

    // Production stops through its script, whose trap stops co-atc, its sidecar
    // and the station link, whoever started it: the launcher or a double-click
    // in Terminal. The script is co-atc's parent, recognised by its arguments,
    // and is signalled only if it is still the process the scan read.
    func stop() {
        guard let prod = production else {
            note = "Production is not running"
            return
        }
        if let s = prod.script, terminate(s) {
            // its trap does the rest
        } else {
            // No script above it: co-atc, and the station links that feed it.
            terminate(prod.proc)
            prod.stationLinks.forEach { terminate($0.proc) }
        }
        stopRequested = Date()
        busy = true
        note = "Stopping…"
        setPlanningRow(false)
        updateSign([])
    }

    // MARK: Test instances

    /// Starts a test instance in a carrier, for a given time: co-atc with its
    /// config, the co-atc beside that config if there is one, else the
    /// checkout's. Its output goes to co-atc.log beside the config.
    func startTest(config: String, binary: String?, seconds: TimeInterval) async -> (proc: Proc?, message: String) {
        guard serving else { return (nil, "only the launcher starts instances") }
        let config = (config as NSString).standardizingPath
        guard FileManager.default.fileExists(atPath: config) else { return (nil, "Not found: \(config)") }
        guard config != productionConfig else { return (nil, "That is production's config: start production instead") }
        if let i = instances.first(where: { $0.configPath == config }) {
            return (nil, "\(i.name) already runs with that config (pid \(i.pid))")
        }
        let dir = (config as NSString).deletingLastPathComponent
        let beside = (dir as NSString).appendingPathComponent("co-atc")
        let exe = binary ?? (FileManager.default.isExecutableFile(atPath: beside) ? beside : resolve("bin/co-atc", from: repo))
        guard FileManager.default.isExecutableFile(atPath: exe) else { return (nil, "Not found: \(exe)") }
        let log = (dir as NSString).appendingPathComponent("co-atc.log")
        guard let pid = await carry([exe, "-config", config], log: log) else { return (nil, "Could not start: " + lastLine(log)) }
        guard let proc = Proc.read(pid) else { return (nil, "co-atc stopped at once: " + lastLine(log)) }
        let until = Date().addingTimeInterval(seconds)
        remember(Started(role: "test", pid: proc.pid, start: proc.start, config: config, until: until))
        return (proc, "started \(exe) -config \(config) in a carrier, pid \(proc.pid), until \(clockTime(until)); log \(log)")
    }

    /// What stopping an instance stops: co-atc, the station links that feed it,
    /// and its sidecar when co-atc did not start it.
    func targets(_ i: Instance) -> [(proc: Proc, what: String)] {
        var t: [(Proc, String)] = [(i.proc, "co-atc")]
        t += i.stationLinks.map { ($0.proc, "station link") }
        if let s = i.sidecar, s.ppid != i.pid { t.append((s, "sidecar")) }
        return t
    }

    // A test instance stops by its own PIDs, never by a pattern (on 14/10 a
    // pattern also stopped the night's measurement).
    func stopInstance(_ i: Instance) {
        let t = targets(i)
        let alert = NSAlert()
        alert.messageText = "Stop \(i.name)?"
        alert.informativeText = t.map { "\($0.what) (\($0.proc.pid))" }.joined(separator: ", ")
        alert.addButton(withTitle: "Stop")
        alert.addButton(withTitle: "Cancel")
        NSApplication.shared.activate(ignoringOtherApps: true)
        guard alert.runModal() == .alertFirstButtonReturn else { return }
        t.forEach { terminate($0.proc) }
        note = "Stopped \(i.name)"
        Task { await refresh() }
    }

    func stopHelper(_ h: Helper) {
        let alert = NSAlert()
        alert.messageText = "Stop the \(h.what) (\(h.pid))?"
        alert.informativeText = h.command
        alert.addButton(withTitle: "Stop")
        alert.addButton(withTitle: "Cancel")
        NSApplication.shared.activate(ignoringOtherApps: true)
        guard alert.runModal() == .alertFirstButtonReturn else { return }
        terminate(h.proc)
        Task { await refresh() }
    }

    /// A test instance the launcher started stops when its time is up.
    private func stopExpired() {
        for s in started where s.role == "test" {
            guard let until = s.until, until < Date(),
                  let i = instances.first(where: { $0.pid == s.pid && $0.proc.start == s.start }) else { continue }
            targets(i).forEach { terminate($0.proc) }
            note = "Stopped \(i.name): its time was up"
        }
    }

    private func remember(_ s: Started) {
        started.append(s)
        saveStarted(started)
    }

    /// Forgets what has stopped.
    private func forgetGone() {
        let before = started.count
        started.removeAll { Proc.read($0.pid)?.start != $0.start }
        if started.count != before { saveStarted(started) }
    }

    // MARK: Requests from the command line

    func handle(_ r: ControlRequest, reply: @escaping (ControlReply) -> Void) async {
        await refresh()
        switch r.cmd {
        case "status":
            // What the menu says on top, for the command line.
            let notices = notifier.allowed.map { $0 ? "notifications allowed" : "notifications not allowed by macOS" + (notifier.refusal.isEmpty ? "" : ": " + notifier.refusal) } ?? "notifications not answered yet"
            reply(.end(true, (["launcher running, pid \(getpid()), \(notices)"] + header).joined(separator: "\n")))
        case "start":
            await startForCommandLine(reply)
        case "stop":
            guard running else { return reply(.end(true, "production was not running")) }
            let tail = LogTail(productionLog)
            stop()
            reply(.step("stopping production (co-atc pid \(production?.pid ?? 0))"))
            for _ in 0..<60 where running {
                try? await Task.sleep(nanoseconds: 2_000_000_000)
                tail.newLines().forEach { reply(.step($0)) }
                await refresh()
            }
            tail.newLines().forEach { reply(.step($0)) }
            reply(running ? .end(false, "production still running after 2 min") : .end(true, "production stopped"))
        case "start-test":
            guard let config = r.config else { return reply(.end(false, "no config given")) }
            let seconds = min(r.seconds ?? 7200, 86_400)
            let (proc, message) = await startTest(config: config, binary: r.binary, seconds: seconds)
            guard let proc else { return reply(.end(false, message)) }
            reply(.step(message))
            await waitForServer(proc, config: config, reply)
        case "stop-test":
            guard let config = r.config.map({ ($0 as NSString).standardizingPath }),
                  let i = instances.first(where: { $0.configPath == config && !$0.production }) else {
                return reply(.end(false, "no test instance runs with that config"))
            }
            let t = targets(i)
            t.forEach { terminate($0.proc) }
            reply(.step("stopping " + t.map { "\($0.what) (\($0.proc.pid))" }.joined(separator: ", ")))
            for _ in 0..<30 where t.contains(where: { $0.proc.alive }) {
                try? await Task.sleep(nanoseconds: 1_000_000_000)
            }
            await refresh()
            reply(t.contains(where: { $0.proc.alive }) ? .end(false, "still running after 30 s") : .end(true, "\(i.name) stopped"))
        default:
            reply(.end(false, "unknown command \(r.cmd)"))
        }
    }

    /// Starts production for the command line and follows it until its server
    /// answers, its station link runs and its sidecar is ready, relaying what
    /// the script writes to its log on the way.
    private func startForCommandLine(_ reply: @escaping (ControlReply) -> Void) async {
        if let p = production {
            return reply(.end(true, "production already running for \(p.elapsed), pid \(p.pid)"))
        }
        let tail = LogTail(productionLog)
        await start(from: .commandLine)
        guard busy, let s = started.last(where: { $0.role == "production" }) else { return reply(.end(false, note)) }
        reply(.step("production script started in a carrier, pid \(s.pid)"))
        var answeredAt: Date?
        for _ in 0..<120 {
            try? await Task.sleep(nanoseconds: 2_000_000_000)
            tail.newLines().forEach { reply(.step($0)) }
            await refresh()
            if case .didNotStart(_, let cause, _) = down {
                tail.newLines().forEach { reply(.step($0)) }
                let short = cause.hasPrefix("The station does not answer") ? "the station does not answer"
                    : cause.components(separatedBy: ": ").first ?? cause
                return reply(.end(false, "Production did not start: \(short) (see the log)"))
            }
            guard let p = production, p.answers else { continue }
            answeredAt = answeredAt ?? Date()
            // Then its station link, which the script starts once co-atc answers
            // (20 s at most: a bench has none), and its sidecar.
            let linked = !p.stationLinks.isEmpty || Date().timeIntervalSince(answeredAt!) > 20
            if linked && (p.sidecarURL.isEmpty || p.sidecarHealth.hasPrefix("ok")) {
                return reply(.end(true, "production running, pid \(p.pid)"))
            }
        }
        reply(.end(false, "still starting after 4 min"))
    }

    /// Follows a test instance until its server answers.
    private func waitForServer(_ proc: Proc, config: String, _ reply: @escaping (ControlReply) -> Void) async {
        let log = ((config as NSString).deletingLastPathComponent as NSString).appendingPathComponent("co-atc.log")
        for _ in 0..<60 {
            try? await Task.sleep(nanoseconds: 2_000_000_000)
            await refresh()
            if let i = instances.first(where: { $0.pid == proc.pid }), i.answers {
                return reply(.end(true, "\(i.name) answers on \(i.url ?? "?"), depends on \(i.dependence?.app ?? "?")"))
            }
            if !proc.alive { return reply(.end(false, "co-atc stopped: " + lastLine(log))) }
        }
        reply(.end(false, "no answer after 2 min"))
    }

    // MARK: Menu actions

    /// The instances that depend on this launcher for macOS.
    var carried: [Instance] { instances.filter { $0.dependence?.pid == getpid() } }

    // What the launcher started depends on it: measured on 24/10, a test
    // instance kept running when the launcher quit, but lost the local network
    // at once and for good (its last ADS-B read was the one before the quit).
    // Quitting says so first.
    // Texts and buttons: docs-fr/design/10-lanceur.md.
    func quit() {
        let c = carried
        guard !c.isEmpty else {
            quitConfirmed = true
            return NSApplication.shared.terminate(nil)
        }
        let names = c.map { i in i.production ? "production" : i.port.map { "the test instance on port \($0)" } ?? "the test instance \(i.name)" }
        var who = names.count == 1 ? names[0] : names.dropLast().joined(separator: ", ") + " and " + names.last!
        who = who.prefix(1).uppercased() + who.dropFirst()
        let subject = c.count > 1 ? "they keep" : c[0].production ? "production keeps" : "it keeps"
        let alert = NSAlert()
        alert.alertStyle = .warning
        alert.messageText = "Quit the launcher?"
        alert.informativeText = "\(who) \(c.count > 1 ? "run" : "runs") from the launcher. If it quits, \(subject) running without the local network: no station, nothing transcribed."
        alert.addButton(withTitle: "Cancel") // the default: Return and Escape
        alert.addButton(withTitle: c.count > 1 ? "Stop All and Quit" : c[0].production ? "Stop Production and Quit" : "Stop Instance and Quit")
        alert.addButton(withTitle: "Quit Anyway")
        NSApplication.shared.activate(ignoringOtherApps: true)
        switch alert.runModal() {
        case .alertSecondButtonReturn: stopAndQuit(c)
        case .alertThirdButtonReturn:
            quitConfirmed = true
            NSApplication.shared.terminate(nil)
        default: break
        }
    }

    /// Stops cleanly what depends on the launcher, then quits; stays open if
    /// something has not stopped after two minutes.
    private func stopAndQuit(_ c: [Instance]) {
        for i in c {
            if i.production { stop() } else { targets(i).forEach { terminate($0.proc) } }
        }
        Task {
            for _ in 0..<60 where c.contains(where: { $0.proc.alive }) {
                try? await Task.sleep(nanoseconds: 2_000_000_000)
            }
            if c.contains(where: { $0.proc.alive }) {
                note = "Not everything stopped: the launcher stays open"
                await refresh()
            } else {
                quitConfirmed = true
                NSApplication.shared.terminate(nil)
            }
        }
    }

    /// Production stops, then starts again from the launcher, which it then
    /// depends on: the way out for a production started from elsewhere.
    func restart() {
        guard production != nil, !busy else { return }
        let alert = NSAlert()
        alert.messageText = "Restart production from the launcher?"
        alert.informativeText = "It stops, then starts again from the launcher. Nothing is transcribed in between."
        alert.addButton(withTitle: "Restart") // the default: it was just asked for
        alert.addButton(withTitle: "Cancel")  // Escape
        NSApplication.shared.activate(ignoringOtherApps: true)
        guard alert.runModal() == .alertFirstButtonReturn else { return }
        stop()
        Task {
            for _ in 0..<60 where running {
                try? await Task.sleep(nanoseconds: 2_000_000_000)
                await refresh()
            }
            guard !running else {
                note = "Production did not stop: not restarted"
                return
            }
            await start()
        }
    }

    func open(_ url: String) {
        if let u = URL(string: url) { NSWorkspace.shared.open(u) }
    }

    func openFile(_ path: String) {
        guard !path.isEmpty else { return }
        NSWorkspace.shared.open(URL(fileURLWithPath: path))
    }

    func copy(_ text: String) {
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(text, forType: .string)
        note = "Copied \(text)"
    }

    /// The production log, at its end: Console opens a log where it ends.
    func openLog() {
        run("/usr/bin/open", ["-a", "Console", productionLog])
    }

    func copyRemoteAddress() {
        guard let url = cfg.tailscale_url else { return }
        copy(url)
    }

    /// A notification's button or click (design 10): what stops or restarts
    /// goes through the menu's own confirmation.
    func answer(_ action: NoticeAction?, _ click: Notice.Click, _ target: [String: String]) {
        let pid = target["pid"].flatMap { Int32($0) }
        let start = target["start"].flatMap { Double($0) }
        switch action {
        case .start: Task { await self.start() }
        case .log: openLog()
        case .restart: restart()
        case .finder: run("/usr/bin/open", ["/Volumes"])
        case .stop:
            if let i = instances.first(where: { $0.pid == pid && $0.proc.start == start }) { stopInstance(i) }
            else if let h = orphans.first(where: { $0.pid == pid && $0.proc.start == start }) { stopHelper(h) }
        case nil:
            switch click {
            case .none: break
            case .log: openLog()
            case .clairance: if let p = production, p.answers, let u = p.url { open(u) }
            case .finder(let path): NSWorkspace.shared.selectFile(nil, inFileViewerRootedAtPath: path)
            case .activityMonitor: run("/usr/bin/open", ["-a", "Activity Monitor"])
            }
        }
    }

    private func stationRead() async -> StationRead {
        var s = StationRead()
        guard let a = cfg.station_status_url, let url = URL(string: a) else { return s }
        var req = URLRequest(url: url)
        req.timeoutInterval = 4
        guard let (data, _) = try? await URLSession.shared.data(for: req),
              let j = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let sel = j["selection"] as? [String: Any],
              let title = sel["titre"] as? String else {
            s.title = "station not answering"
            return s
        }
        s.answered = true
        s.title = title
        s.mode = (j["mode"] as? String) ?? ""
        s.onAir = (sel["frequences"] as? [[String: Any]] ?? []).compactMap { $0["designation"] as? String }
        return s
    }

    // The GPU is shared with the lab: production says so while it runs, in the
    // row a person or an agent would otherwise have written by hand.
    private func setPlanningRow(_ on: Bool) {
        guard let path = cfg.planning_file,
              var text = try? String(contentsOfFile: path, encoding: .utf8) else { return }
        var lines = text.components(separatedBy: "\n").filter { !$0.contains(planningMarker) }
        if on, let sep = lines.firstIndex(where: { $0.hasPrefix("|---") }) {
            let f = DateFormatter()
            f.locale = Locale(identifier: "fr_FR")
            f.dateFormat = "dd/MM 'à' HH:mm"
            lines.insert("| depuis le \(f.string(from: Date())), durée ouverte \(planningMarker) | Clairance | **co-atc en production** | oui, en continu (trois modèles, ~8 Go) |", at: sep + 1)
        }
        text = lines.joined(separator: "\n")
        try? text.write(toFile: path, atomically: true, encoding: .utf8)
    }
}

// MARK: - The command line

// The same actions without the menu, for a terminal, a Shortcut or an agent:
//   clairance --status
//   clairance --start | --stop
//   clairance --start-test <config> [--for 2h] [--binary <co-atc>]
//   clairance --stop-test <config>
// Starting goes through the launcher in the menu bar, opened by the system if
// it does not run, so that what starts depends on the launcher and not on the
// terminal or the agent that typed the command. The command line never starts
// anything itself.
@main
enum Entry {
    static func main() {
        // A launcher that closes mid-answer ends the command, not by a signal.
        signal(SIGPIPE, SIG_IGN)
        let args = Array(CommandLine.arguments.dropFirst())
        if args.first == "--carry" { carry(Array(args.dropFirst())) }
        if let cmd = args.first, cmd.hasPrefix("--") {
            MainActor.assumeIsolated { cli(cmd, Array(args.dropFirst())) }
            return
        }
        // The launcher is an app of its own for macOS only when the system
        // started it. Run from a terminal, it would depend on that terminal's
        // app, and so would the production it starts: it has the system start
        // it, and leaves. A second launcher leaves too.
        let bundle = Bundle.main.bundleURL
        if getppid() != 1, bundle.pathExtension == "app" {
            run("/usr/bin/open", ["-n", "-g", bundle.path])
            exit(0)
        }
        if let other = connectControl() {
            close(other)
            exit(0)
        }
        ClairanceLauncher.main()
    }

    // The carrier: an instance of Clairance.app of its own, opened by the
    // system, that runs one program and leaves with it. What it runs depends on
    // it for macOS, not on the launcher in the menu bar, which can then quit,
    // crash or be updated without the program losing the local network (Q59,
    // 24/10). A SIGTERM to the carrier goes to what it carries.
    //   clairance --carry [--cwd <dir>] [--log <file>] [--pidfile <file>] [--probe <url>] -- <program> <args>…
    static func carry(_ args: [String]) -> Never {
        var cwd = FileManager.default.currentDirectoryPath
        var log = "/dev/null"
        var pidfile: String?
        var probe: URL?
        var i = 0
        while i + 1 < args.count, args[i] != "--" {
            switch args[i] {
            case "--cwd": cwd = args[i + 1]
            case "--log": log = args[i + 1]
            case "--pidfile": pidfile = args[i + 1]
            case "--probe": probe = URL(string: args[i + 1])
            default: exit(2)
            }
            i += 2
        }
        let command = Array(args.dropFirst(i + 1))
        guard i < args.count, args[i] == "--", let exe = command.first else { exit(2) }
        if !FileManager.default.fileExists(atPath: log) { FileManager.default.createFile(atPath: log, contents: nil) }
        guard let out = FileHandle(forWritingAtPath: log) else { exit(1) }
        out.seekToEndOfFile()
        func say(_ s: String) {
            let f = DateFormatter()
            f.dateFormat = "dd/MM HH:mm:ss"
            out.write(Data("\(f.string(from: Date())) carrier: \(s)\n".utf8))
        }
        // Started as an app, so that macOS knows it as Clairance.
        let app = NSApplication.shared
        app.setActivationPolicy(.accessory)
        let delegate = CarrierDelegate()
        app.delegate = delegate
        app.finishLaunching()
        // Twice on 24/10, the first connection to the station from a build of
        // the launcher macOS had not seen yet failed ("no route to host"), and
        // co-atc, which checks its ADS-B source when it starts, gave up. The
        // carrier waits for the station to answer it first, 20 s at most.
        if let url = probe {
            var attempts = 0
            var answered = false
            while attempts < 40 && !answered {
                attempts += 1
                answered = answers(url)
                if !answered { Thread.sleep(forTimeInterval: 0.5) }
            }
            say(answered ? "the station answered at attempt \(attempts)" : "the station did not answer in \(attempts) attempts")
        }
        let p = Process()
        p.executableURL = URL(fileURLWithPath: exe)
        p.arguments = Array(command.dropFirst())
        p.currentDirectoryURL = URL(fileURLWithPath: cwd)
        p.standardOutput = out
        p.standardError = out
        p.standardInput = FileHandle.nullDevice
        p.terminationHandler = { exit($0.terminationStatus) }
        do { try p.run() } catch { exit(1) }
        if let f = pidfile { try? "\(p.processIdentifier)\n".write(toFile: f, atomically: true, encoding: .utf8) }
        signal(SIGTERM, SIG_IGN)
        let term = DispatchSource.makeSignalSource(signal: SIGTERM, queue: .main)
        term.setEventHandler { kill(p.processIdentifier, SIGTERM) }
        term.resume()
        app.run()
        exit(0)
    }

    // While a carrier runs, macOS takes Clairance for open: a double-click,
    // Spotlight or `open` only brings the carrier forward, and the menu would
    // never come back (measured on 24/10). The carrier opens it then, unless
    // a launcher already answers.
    final class CarrierDelegate: NSObject, NSApplicationDelegate {
        func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
            if let fd = connectControl() {
                close(fd)
            } else {
                run("/usr/bin/open", ["-n", "-g", Bundle.main.bundleURL.path])
            }
            return false
        }
    }

    /// Whether an address answers at all, within half a second.
    static func answers(_ url: URL) -> Bool {
        var req = URLRequest(url: url)
        req.timeoutInterval = 0.5
        let done = DispatchSemaphore(value: 0)
        var ok = false
        URLSession.shared.dataTask(with: req) { _, response, _ in
            ok = response != nil
            done.signal()
        }.resume()
        done.wait()
        return ok
    }

    static let usage = """
        usage: clairance --status
               clairance --start | --stop
               clairance --start-test <config> [--for 2h] [--binary <co-atc>]
               clairance --stop-test <config>
        """

    /// Runs async work from the command line's synchronous code, turning the
    /// main run loop until it ends.
    @MainActor static func block(_ work: @escaping @MainActor () async -> Void) {
        var done = false
        Task { @MainActor in
            await work()
            done = true
        }
        while !done { RunLoop.main.run(until: Date().addingTimeInterval(0.05)) }
    }

    /// The running launcher, opened by the system first when asked and needed.
    static func launcher(open: Bool) -> Int32? {
        if let fd = connectControl() { return fd }
        guard open else { return nil }
        let bundle = Bundle.main.bundleURL
        guard bundle.pathExtension == "app" else {
            print("this command is not inside Clairance.app: it cannot open the launcher")
            return nil
        }
        print("opening the launcher (\(bundle.path))")
        // A new instance: while a carrier runs, macOS takes Clairance for open
        // and would only bring that carrier forward.
        run("/usr/bin/open", ["-n", "-g", bundle.path])
        for _ in 0..<60 {
            Thread.sleep(forTimeInterval: 0.25)
            if let fd = connectControl() { return fd }
        }
        print("the launcher did not answer within 15 s; an older launcher may be running: quit it and try again")
        return nil
    }

    /// Sends a request to the launcher and prints what it says; true when it ended well.
    static func send(_ request: ControlRequest, open: Bool) -> Bool {
        guard let fd = launcher(open: open) else { return false }
        let last = ask(fd, request) { print("  " + $0) }
        guard let last else {
            print("the launcher closed the connection without an answer")
            return false
        }
        print(last.message ?? "")
        return last.ok == true
    }

    /// A config path from the command line, absolute.
    static func absolute(_ path: String) -> String {
        resolve(path, from: FileManager.default.currentDirectoryPath)
    }

    @MainActor static func cli(_ cmd: String, _ rest: [String]) {
        var ok = true
        switch cmd {
        case "--status":
            break
        case "--start":
            ok = send(ControlRequest(cmd: "start"), open: true)
        case "--stop":
            if connectControl().map({ close($0) }) != nil {
                ok = send(ControlRequest(cmd: "stop"), open: false)
            } else {
                // No launcher: stopping depends on no app, the command line does it.
                let p = Production()
                block { await p.refresh() }
                p.stop()
                for _ in 0..<60 where p.running {
                    Thread.sleep(forTimeInterval: 2)
                    block { await p.refresh() }
                }
                ok = !p.running
            }
        case "--start-test", "--stop-test":
            guard let config = rest.first, !config.hasPrefix("--") else {
                print(usage)
                exit(2)
            }
            var request = ControlRequest(cmd: String(cmd.dropFirst(2)), config: absolute(config))
            var i = 1
            while i + 1 < rest.count {
                switch rest[i] {
                case "--for":
                    guard let s = parseDuration(rest[i + 1]) else { print(usage); exit(2) }
                    request.seconds = s
                case "--binary":
                    request.binary = absolute(rest[i + 1])
                default:
                    print(usage)
                    exit(2)
                }
                i += 2
            }
            if cmd == "--start-test" || connectControl().map({ close($0) }) != nil {
                ok = send(request, open: cmd == "--start-test")
            } else if let config = request.config {
                // No launcher: like --stop, the command line stops it by its own PIDs.
                let p = Production()
                block { await p.refresh() }
                guard let i = p.instances.first(where: { $0.configPath == config && !$0.production }) else {
                    print("no test instance runs with that config")
                    exit(1)
                }
                let t = p.targets(i)
                t.forEach { terminate($0.proc) }
                for _ in 0..<30 where t.contains(where: { $0.proc.alive }) { Thread.sleep(forTimeInterval: 1) }
                ok = !t.contains(where: { $0.proc.alive })
                print(ok ? "\(i.name) stopped" : "still running after 30 s")
            }
        default:
            print(usage)
            exit(2)
        }
        print("")
        report()
        exit(ok ? 0 : 1)
    }

    @MainActor static func report() {
        let p = Production()
        block { await p.refresh() }
        if let fd = connectControl() {
            // The launcher's own view: it follows starts, stops and incidents over time.
            let view = ask(fd, ControlRequest(cmd: "status")) { _ in }?.message ?? ""
            print(view.isEmpty ? "launcher running" : view)
        } else {
            print("launcher not running")
            p.header.forEach { print($0) }
        }
        if !p.health.isEmpty {
            print("")
            print("health:")
            p.health.forEach { print("  " + $0.text) }
        }
        print("on air: \(p.onAir)")
        for i in p.instances {
            print("")
            print("\(i.title)  (pid \(i.pid))")
            for line in instanceLines(i) { print("  \(line)") }
        }
        if !p.orphans.isEmpty {
            print("")
            print("left without an instance:")
            for h in p.orphans { print("  \(h.what) (pid \(h.pid), \(h.elapsed)): \(h.command)") }
        }
    }
}

// What an instance's menu says, and the command line's --status.
func instanceLines(_ i: Instance) -> [String] {
    var lines: [String] = []
    lines.append("Page: \(i.www)")
    if !i.version.isEmpty { lines.append("Server: \(i.version)") }
    lines.append(i.summary.isEmpty ? (i.answers ? "Server answering" : "Server not answering") : i.summary)
    if !i.sidecarURL.isEmpty {
        let kind = i.sidecarKind.isEmpty ? "" : "\(i.sidecarKind), "
        lines.append("Transcription: \(kind)\(i.sidecarHealth)" + (i.sidecarKind == "real model" ? " · uses the GPU" : ""))
    }
    if i.stationLinks.isEmpty {
        lines.append("Station link: none")
    } else {
        for h in i.stationLinks { lines.append("Station link: \(h.what == "station link" ? "running" : h.what), \(h.elapsed)") }
    }
    // Measured on 24/10: when the app a process depends on quits, macOS makes the
    // process depend on itself, and a bare program has no Local Network access.
    if let d = i.dependence {
        lines.append(d.pid == i.pid ? "Depends on: no app · no local network" : "Depends on: \(d.app)")
    }
    if let u = i.until {
        let left = u.timeIntervalSinceNow
        lines.append(left > 0 ? "Stops at \(clockTime(u)) (in \(shortElapsed(left)))"
                              : "Was to stop at \(clockTime(u)) (\(shortElapsed(-left)) ago)")
    }
    let ports = ([i.port].compactMap { $0 } + i.extraPorts).map(String.init).joined(separator: ", ")
    if !ports.isEmpty { lines.append("Ports: \(ports)") }
    lines.append("Config: \(i.config)")
    if !i.database.isEmpty { lines.append("Databases: \(i.database)") }
    return lines
}

// The sign's six states (docs-fr/design/10-lanceur.md), from the bundle's
// Resources (build.sh copies them there), 18 pt. Stopped, Switching and
// Running are template images, tinted by macOS for a light or dark bar; On
// air, Fault and Down keep their colours on a dark disc, on purpose
// (05-identite.md). Without the bundle (the command-line form), the system
// symbols stand in.
func menuBarIcon(_ s: SignState) -> NSImage {
    let (name, template): (String, Bool) = {
        switch s {
        case .stopped: return ("barre-menus-arret", true)
        case .switching: return ("barre-menus-bascule", true)
        case .running: return ("barre-menus-marche", true)
        case .onAir: return ("barre-menus-ecoute", false)
        case .fault: return ("barre-menus-defaut", false)
        case .down: return ("barre-menus-panne", false)
        }
    }()
    if let url = Bundle.main.url(forResource: name, withExtension: "svg"),
       let image = NSImage(contentsOf: url) {
        image.isTemplate = template
        image.size = NSSize(width: 18, height: 18)
        return image
    }
    let fallback = s == .stopped ? "antenna.radiowaves.left.and.right.slash" : "dot.radiowaves.left.and.right"
    return NSImage(systemSymbolName: fallback, accessibilityDescription: "Clairance") ?? NSImage()
}

/// "▲ Sidecar without an instance · 9 GB" (design 10).
func orphanTitle(_ o: Orphan) -> String {
    o.helper.what.hasPrefix("sidecar")
        ? "▲ Sidecar without an instance" + (o.memory.map { " · " + brandGB($0) } ?? "")
        : "▲ Station link without an instance"
}

// The menu of docs-fr/design/10-lanceur.md: the state and its cause, the
// health lines, the actions the state allows, the instances.
struct ClairanceLauncher: App {
    @NSApplicationDelegateAdaptor(LauncherDelegate.self) private var delegate
    @StateObject private var p = Production(serving: true)

    var body: some Scene {
        MenuBarExtra {
            ForEach(Array(p.header.enumerated()), id: \.offset) { Text($0.element).monospacedDigit() }
            if p.offerRestartFromClairance {
                Button("Restart from Clairance") { p.restart() }.disabled(p.busy)
            }
            if !p.note.isEmpty && !p.busy { Text(p.note) }
            if !p.health.isEmpty {
                Divider()
                Section("Health") {
                    ForEach(p.health, id: \.self) { Text($0.text).monospacedDigit() }
                }
            }
            Divider()
            switch p.sign {
            case .switching:
                EmptyView()
            case .stopped:
                Button("Start production") { Task { await p.start() } }
            case .down where p.down != nil:
                Button("Start production") { Task { await p.start() } }
                Button("Leave stopped") { p.leaveStopped() }
            default:
                Button("Stop production") { p.stop() }
                Button("Restart production") { p.restart() }
            }
            Button("Open Clairance") { if let u = p.production?.url { p.open(u) } }
                .keyboardShortcut("o")
                .disabled(!(p.production?.answers ?? false))
            if p.cfg.tailscale_url != nil { Button("Copy remote address") { p.copyRemoteAddress() } }
            Button("Show production log") { p.openLog() }
                .keyboardShortcut("l")
            Divider()
            Section("Instances") {
                if p.instances.isEmpty && p.orphans.isEmpty { Text("No instance running") }
                ForEach(p.instances) { i in
                    Menu(i.title) {
                        ForEach(instanceLines(i), id: \.self) { Text($0).monospacedDigit() }
                        Divider()
                        if let u = i.url {
                            Button("Open \(u)") { p.open(u) }
                            Button("Copy address") { p.copy(u) }
                        }
                        Button("Open log") { p.openFile(i.log) }.disabled(i.log.isEmpty || !i.log.hasPrefix("/"))
                        Button("Show config") { p.openFile(i.configPath) }
                        if !i.production {
                            Divider()
                            Button("Stop this instance…") { p.stopInstance(i) }
                        }
                    }
                }
                ForEach(p.observation.orphans, id: \.helper.pid) { o in
                    Menu(orphanTitle(o)) {
                        Text("PID \(o.helper.pid) · \(o.helper.elapsed)").monospacedDigit()
                        Text(o.helper.command)
                        Divider()
                        Button("Stop it…") { p.stopHelper(o.helper) }
                    }
                }
            }
            Divider()
            Button("Settings…") { SettingsWindow.shared.show() }
                .keyboardShortcut(",")
            Button(p.carried.isEmpty ? "Quit launcher" : "Quit launcher…") { p.quit() }
                .keyboardShortcut("q")
        } label: {
            Image(nsImage: menuBarIcon(p.sign))
                .accessibilityLabel(p.signLabel)
        }
        .menuBarExtraStyle(.menu)
        .commands {
            // In the Dock, the app's own menu: its settings at ⌘, as every Mac app.
            CommandGroup(replacing: .appSettings) {
                Button("Settings…") { SettingsWindow.shared.show() }.keyboardShortcut(",")
            }
        }
    }
}
