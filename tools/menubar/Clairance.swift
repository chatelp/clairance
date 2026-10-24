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
// What it starts, it starts as itself, so that for macOS production depends on
// the launcher and not on whatever typed the command: `clairance --start` asks
// the running launcher, opening it first if needed (Control.swift). Processes
// are known by their PID, their exact arguments and their start time, never by
// a pattern in a command line (Processes.swift).
//
// Settings, all optional, in ~/.config/clairance/launcher.json:
//   repo                the Clairance checkout (default: where it was built)
//   production_command  relative to repo (default: runs/production.command)
//   production_config   the config that command starts co-atc with, relative
//                       to repo (default: configs/config.toml): the instance
//                       running it is production
//   station_status_url  radio-ctl's /radio/etat, for the line "On air"
//   tailscale_url       copied by "Copy remote address"
//   planning_file       a markdown table of who uses the GPU; production adds
//                       its row while it runs and removes it when it stops

import AppKit
import SwiftUI

struct LauncherConfig: Decodable {
    var repo: String?
    var production_command: String?
    var production_config: String?
    var station_status_url: String?
    var tailscale_url: String?
    var planning_file: String?
}

/// Replaced by build.sh with the checkout the app was built from.
let builtRepo = "@REPO@"

let launcherBundleID = "dev.clairance.launcher"
let planningMarker = "(menu bar launcher)"

func loadLauncherConfig() -> LauncherConfig {
    let path = FileManager.default.homeDirectoryForCurrentUser
        .appendingPathComponent(".config/clairance/launcher.json")
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
    // Read from the server and the sidecar after the scan.
    var answers = false         // its /api/v1/health answered
    var version = ""
    var summary = ""            // vitalsLine, empty when the server did not answer
    var sidecarHealth = ""

    var elapsed: String { shortElapsed(proc.elapsed) }
    var cwd: String { proc.cwd }
    var url: String? { port.map { "http://127.0.0.1:\($0)/" } }

    var title: String {
        var t = name
        if let p = port { t += " · :\(p)" }
        t += " · \(elapsed)"
        return t
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
            until: mark?.until))
    }
    instances.sort { ($0.production ? 0 : 1, $0.port ?? 0) < ($1.production ? 0 : 1, $1.port ?? 0) }
    let orphans = (sidecars + links).filter { !claimed.contains($0.pid) }
    return (instances, orphans)
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

@MainActor
final class Production: ObservableObject {
    @Published var running = false
    @Published var elapsed = ""
    @Published var onAir = "—"
    // The station's own mode word ("chaine" = the airband chain on the air).
    @Published var stationMode = ""
    // The third state of the sign (05-identite.md): production runs and the
    // antenna is on the air.
    var listening: Bool { running && stationMode == "chaine" }
    @Published var sidecar = "—"
    @Published var instances: [Instance] = []
    @Published var orphans: [Helper] = []
    @Published var busy = false
    @Published var note = ""

    let cfg: LauncherConfig
    let repo: String
    var command: String { cfg.production_command ?? "runs/production.command" }
    var script: String { resolve(command, from: repo) }
    var productionConfig: String { resolve(cfg.production_config ?? "configs/config.toml", from: repo) }
    var productionLog: String { (repo as NSString).appendingPathComponent("runs/production.log") }
    var production: Instance? { instances.first(where: \.production) }

    /// Only the launcher in the menu bar serves the control socket, starts
    /// anything and keeps what it started; the command line reads.
    let serving: Bool
    private var started: [Started]
    private var timer: Timer?

    init(serving: Bool = false) {
        cfg = loadLauncherConfig()
        repo = cfg.repo ?? builtRepo
        self.serving = serving
        started = loadStarted()
        if serving {
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

    func refresh() async {
        let settings = ScanSettings(productionConfig: productionConfig, productionScript: script, started: started)
        let scan = await Task.detached { scanInstances(settings) }.value
        var found = scan.instances
        for i in found.indices {
            if let u = found[i].url {
                found[i].answers = await getJSON(u + "api/v1/health") != nil
                if let v = await getJSON(u + "api/v1/vitals") {
                    found[i].version = v["version"] as? String ?? ""
                    found[i].summary = vitalsLine(v)
                }
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
        let station = await stationOnAir()
        onAir = station.title
        stationMode = station.mode
        if serving {
            stopExpired()
            forgetGone()
        }
        // A start ends when co-atc runs, or when its script is gone; a stop when co-atc is gone.
        if busy && note == "Starting…" {
            if running {
                busy = false
                note = ""
            } else if let s = started.last(where: { $0.role == "production" }), !(Proc.read(s.pid)?.start == s.start) {
                busy = false
                note = "Start failed: " + lastLine(productionLog)
            }
        } else if busy && note == "Stopping…" && !running {
            busy = false
            note = ""
        }
    }

    // MARK: Production

    func start() {
        guard serving, !running, !busy else { return }
        guard FileManager.default.isExecutableFile(atPath: script) else {
            note = "Not found: \(script)"
            return
        }
        let log = (repo as NSString).appendingPathComponent("runs/launcher.log")
        FileManager.default.createFile(atPath: log, contents: nil)
        guard let out = FileHandle(forWritingAtPath: log) else { return }
        out.seekToEndOfFile()
        let p = Process()
        p.executableURL = URL(fileURLWithPath: "/bin/sh")
        p.arguments = [script]
        p.currentDirectoryURL = URL(fileURLWithPath: repo)
        p.standardOutput = out
        p.standardError = out
        p.standardInput = FileHandle.nullDevice // no terminal: the password comes from its file
        do {
            try p.run()
            if let proc = Proc.read(p.processIdentifier) {
                remember(Started(role: "production", pid: proc.pid, start: proc.start, config: productionConfig))
            }
            busy = true
            note = "Starting…"
            setPlanningRow(true)
        } catch {
            note = "Could not start: \(error.localizedDescription)"
        }
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
        busy = true
        note = "Stopping…"
        setPlanningRow(false)
    }

    // MARK: Test instances

    /// Starts a test instance as the launcher, for a given time: co-atc with its
    /// config, the co-atc beside that config if there is one, else the
    /// checkout's. Its output goes to co-atc.log beside the config.
    func startTest(config: String, binary: String?, seconds: TimeInterval) -> (proc: Proc?, message: String) {
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
        FileManager.default.createFile(atPath: log, contents: nil)
        guard let out = FileHandle(forWritingAtPath: log) else { return (nil, "Cannot write \(log)") }
        out.seekToEndOfFile()
        let p = Process()
        p.executableURL = URL(fileURLWithPath: exe)
        p.arguments = ["-config", config]
        p.currentDirectoryURL = URL(fileURLWithPath: repo)
        p.standardOutput = out
        p.standardError = out
        p.standardInput = FileHandle.nullDevice
        do { try p.run() } catch { return (nil, "Could not start: \(error.localizedDescription)") }
        guard let proc = Proc.read(p.processIdentifier) else { return (nil, "co-atc stopped at once: " + lastLine(log)) }
        let until = Date().addingTimeInterval(seconds)
        remember(Started(role: "test", pid: proc.pid, start: proc.start, config: config, until: until))
        return (proc, "started \(exe) -config \(config), pid \(proc.pid), until \(clockTime(until)); log \(log)")
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
            reply(.end(true, "launcher running, pid \(getpid())"))
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
            let (proc, message) = startTest(config: config, binary: r.binary, seconds: seconds)
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
        start()
        guard busy, let s = started.last(where: { $0.role == "production" }) else { return reply(.end(false, note)) }
        reply(.step("production script started by the launcher, pid \(s.pid)"))
        for _ in 0..<120 {
            try? await Task.sleep(nanoseconds: 2_000_000_000)
            tail.newLines().forEach { reply(.step($0)) }
            await refresh()
            if let p = production, p.answers, !p.stationLinks.isEmpty,
               p.sidecarURL.isEmpty || p.sidecarHealth.hasPrefix("ok") {
                return reply(.end(true, "production running, pid \(p.pid)"))
            }
            if production == nil && Proc.read(s.pid)?.start != s.start {
                tail.newLines().forEach { reply(.step($0)) }
                return reply(.end(false, "the production script stopped: " + lastLine(productionLog)))
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
        guard !c.isEmpty else { return NSApplication.shared.terminate(nil) }
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
        case .alertThirdButtonReturn: NSApplication.shared.terminate(nil)
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
            start()
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

    func openLog() {
        NSWorkspace.shared.open(URL(fileURLWithPath: productionLog))
    }

    func copyRemoteAddress() {
        guard let url = cfg.tailscale_url else { return }
        copy(url)
    }

    private func stationOnAir() async -> (title: String, mode: String) {
        guard let s = cfg.station_status_url, let url = URL(string: s) else { return ("—", "") }
        var req = URLRequest(url: url)
        req.timeoutInterval = 4
        guard let (data, _) = try? await URLSession.shared.data(for: req),
              let j = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let sel = j["selection"] as? [String: Any],
              let title = sel["titre"] as? String else { return ("station not answering", "") }
        return (title, (j["mode"] as? String) ?? "")
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
        run("/usr/bin/open", ["-g", bundle.path])
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
            let pid = ask(fd, ControlRequest(cmd: "status")) { _ in }?.message ?? ""
            print(pid.isEmpty ? "launcher running" : pid)
        } else {
            print("launcher not running")
        }
        print(p.running ? "production running for \(p.elapsed)" : "production stopped")
        if p.running { print("sidecar: \(p.sidecar)") }
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

// menuBarIcon loads the sign's three states from the bundle's Resources
// (build.sh copies them there), 18 pt: stopped and running are template
// images, tinted by macOS for a light or dark bar; listening keeps its own
// colours, on purpose (05-identite.md). Without the bundle (the command-line
// form), the system symbols stand in.
func menuBarIcon(running: Bool, listening: Bool) -> NSImage {
    let name = listening ? "barre-menus-ecoute" : running ? "barre-menus-marche" : "barre-menus-arret"
    if let url = Bundle.main.url(forResource: name, withExtension: "svg"),
       let image = NSImage(contentsOf: url) {
        image.isTemplate = !listening
        image.size = NSSize(width: 18, height: 18)
        return image
    }
    let fallback = running ? "dot.radiowaves.left.and.right" : "antenna.radiowaves.left.and.right.slash"
    return NSImage(systemSymbolName: fallback, accessibilityDescription: "Clairance") ?? NSImage()
}

struct ClairanceLauncher: App {
    @StateObject private var p = Production(serving: true)

    var body: some Scene {
        MenuBarExtra {
            Text(p.running ? "Production running · \(p.elapsed)" : "Production stopped")
            if let prod = p.production, let d = prod.dependence, !d.launcher {
                if d.pid == prod.pid {
                    Text("▲ Production without local network")
                    Text("The app that started it quit: macOS took its local network")
                } else {
                    Text("▲ Started from \(d.app): it stops hearing if \(d.app) quits")
                }
                Button("Restart from Clairance") { p.restart() }.disabled(p.busy)
            }
            if p.running { Text("Sidecar: \(p.sidecar)") }
            Text("On air: \(p.onAir)")
            if !p.note.isEmpty { Text(p.note) }
            Divider()
            Button("Start production") { p.start() }.disabled(p.running || p.busy)
            Button("Stop production") { p.stop() }.disabled(!p.running)
            Divider()
            Button("Open Clairance") { p.open(p.production?.url ?? "http://127.0.0.1:8000/") }.disabled(!p.running)
            if p.cfg.tailscale_url != nil { Button("Copy remote address") { p.copyRemoteAddress() } }
            Button("Production log") { p.openLog() }
            Divider()
            Text(p.instances.isEmpty ? "No instance running" : "Instances running: \(p.instances.count)")
            ForEach(p.instances) { i in
                Menu(i.title) {
                    ForEach(instanceLines(i), id: \.self) { Text($0) }
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
            if !p.orphans.isEmpty {
                Menu("Left without an instance: \(p.orphans.count)") {
                    ForEach(p.orphans) { h in
                        Button("Stop \(h.what) · \(h.pid) · \(h.elapsed)…") { p.stopHelper(h) }
                    }
                }
            }
            Divider()
            Button(p.carried.isEmpty ? "Quit launcher" : "Quit launcher…") { p.quit() }
                .keyboardShortcut("q")
        } label: {
            // The brand's two states (docs-fr/design/05-identite.md): the scope with
            // its sweep and plot while production runs, the empty screen when it is
            // stopped. Template images, so macOS tints them for a light or dark bar.
            Image(nsImage: menuBarIcon(running: p.running, listening: p.listening))
        }
        .menuBarExtraStyle(.menu)
    }
}
