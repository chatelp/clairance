// Processes read by their PID, from the kernel: what runs, with which exact
// arguments, since when, and which app macOS holds responsible for it.
//
// Nothing here looks for a pattern in a command line. On 16/10 a command that
// merely contained production's pattern made the launcher believe production
// ran, twice; on 14/10 a kill by pattern stopped the night's measurement. A
// process is known by its own name, its exact arguments and its start time,
// and the start time also tells it apart from a later process the system has
// given the same PID.

import AppKit
import Darwin

/// One process of this user, as the kernel describes it.
struct Proc: Sendable {
    let pid: Int32
    let ppid: Int32
    let name: String      // the executable's own name, 16 characters at most
    let start: Double     // seconds since 1970, to the microsecond
    let args: [String]    // exact, argv[0] first
    let cwd: String

    /// Still the same process: its PID runs, and started at the same moment.
    var alive: Bool { Proc.read(pid)?.start == start }

    var elapsed: TimeInterval { Date().timeIntervalSince1970 - start }

    /// The argument that follows a flag ("-config" -> its path).
    func argument(after flag: String) -> String? {
        guard let i = args.firstIndex(of: flag), i + 1 < args.count else { return nil }
        return args[i + 1]
    }

    /// Whether one of its arguments is a file of that name, in any folder.
    func runs(_ file: String) -> Bool {
        args.contains { ($0 as NSString).lastPathComponent == file }
    }

    var commandLine: String { args.joined(separator: " ") }

    static func read(_ pid: Int32) -> Proc? {
        guard let info = bsdInfo(pid) else { return nil }
        var vpi = proc_vnodepathinfo()
        let size = Int32(MemoryLayout<proc_vnodepathinfo>.size)
        let cwd = proc_pidinfo(pid, PROC_PIDVNODEPATHINFO, 0, &vpi, size) == size ? cString(vpi.pvi_cdir.vip_path) : ""
        return Proc(pid: pid, ppid: Int32(info.pbi_ppid), name: cString(info.pbi_comm),
                    start: Double(info.pbi_start_tvsec) + Double(info.pbi_start_tvusec) / 1_000_000,
                    args: arguments(pid), cwd: cwd)
    }

    /// This user's processes whose executable's name passes the test. Only
    /// those are read in full: arguments cost a copy of up to a megabyte each.
    static func all(named test: (String) -> Bool) -> [Proc] {
        allPIDs().filter { pid in bsdInfo(pid).map { test(cString($0.pbi_comm)) } ?? false }
            .compactMap(read)
    }
}

/// SIGTERM to a process read earlier, only if it is still that process.
@discardableResult
func terminate(_ p: Proc) -> Bool {
    guard p.alive else { return false }
    return kill(p.pid, SIGTERM) == 0
}

/// "3 min", "2 h 14 min", "1 d 2 h" (docs-fr/design/10-lanceur.md).
func shortElapsed(_ seconds: TimeInterval) -> String {
    let s = Int(max(0, seconds))
    let d = s / 86_400, h = s % 86_400 / 3600, m = s % 3600 / 60
    if d > 0 { return "\(d) d \(h) h" }
    if h > 0 { return "\(h) h \(m) min" }
    return "\(m) min"
}

private func cString<T>(_ tuple: T) -> String {
    withUnsafeBytes(of: tuple) { String(decoding: $0.prefix { $0 != 0 }, as: UTF8.self) }
}

/// A process's description, if it is this user's.
private func bsdInfo(_ pid: Int32) -> proc_bsdinfo? {
    var info = proc_bsdinfo()
    let size = Int32(MemoryLayout<proc_bsdinfo>.size)
    guard pid > 0, proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &info, size) == size, info.pbi_uid == getuid() else { return nil }
    return info
}

private func allPIDs() -> [Int32] {
    let count = proc_listallpids(nil, 0)
    guard count > 0 else { return [] }
    var pids = [Int32](repeating: 0, count: Int(count) + 64)
    let got = pids.withUnsafeMutableBytes { proc_listallpids($0.baseAddress, Int32($0.count)) }
    return pids.prefix(Int(max(0, got))).filter { $0 > 0 }
}

/// A process's exact arguments. The kernel gives the argument count, the
/// executable's path, some padding, then the arguments, each ended by a zero.
private func arguments(_ pid: Int32) -> [String] {
    var argmax: Int32 = 0
    var len = MemoryLayout<Int32>.size
    var mib: [Int32] = [CTL_KERN, KERN_ARGMAX]
    guard sysctl(&mib, 2, &argmax, &len, nil, 0) == 0, argmax > 0 else { return [] }
    var buf = [UInt8](repeating: 0, count: Int(argmax))
    var size = buf.count
    mib = [CTL_KERN, KERN_PROCARGS2, pid]
    guard sysctl(&mib, 3, &buf, &size, nil, 0) == 0, size > MemoryLayout<Int32>.size else { return [] }
    let argc = buf.withUnsafeBytes { $0.load(as: Int32.self) }
    var i = MemoryLayout<Int32>.size
    while i < size && buf[i] != 0 { i += 1 }
    while i < size && buf[i] == 0 { i += 1 }
    var args: [String] = []
    while args.count < Int(argc) && i < size {
        let from = i
        while i < size && buf[i] != 0 { i += 1 }
        args.append(String(decoding: buf[from..<i], as: UTF8.self))
        i += 1
    }
    return args
}

// MARK: - Who a process depends on

/// The process macOS holds responsible for another: the app whose
/// permissions it runs on, Local Network among them. Production started by a
/// command typed in a terminal, or by an agent, depends on that app, and lost
/// the station when it restarted (16-18/10); started by the launcher, it
/// depends on the launcher. A private function of the system library, looked
/// up when the launcher starts: nil if it is ever gone.
private let responsibleFor: (@convention(c) (pid_t) -> pid_t)? = {
    guard let sym = dlsym(dlopen(nil, RTLD_NOW), "responsibility_get_pid_responsible_for_pid") else { return nil }
    return unsafeBitCast(sym, to: (@convention(c) (pid_t) -> pid_t).self)
}()

/// The app a process depends on, by name, and whether that app is a Clairance launcher.
struct Dependence: Sendable {
    let app: String
    let pid: Int32
    let launcher: Bool
}

func dependence(of pid: Int32) -> Dependence? {
    guard let f = responsibleFor else { return nil }
    let r = f(pid)
    guard r > 0 else { return nil }
    let app = NSRunningApplication(processIdentifier: r)
    if app?.bundleIdentifier == launcherBundleID { return Dependence(app: "Clairance", pid: r, launcher: true) }
    if let name = app?.localizedName { return Dependence(app: name, pid: r, launcher: false) }
    // No app registered under that PID: the .app its executable sits in, else its name.
    var path = [CChar](repeating: 0, count: 4 * Int(MAXPATHLEN))
    guard proc_pidpath(r, &path, UInt32(path.count)) > 0 else { return Dependence(app: "pid \(r)", pid: r, launcher: false) }
    let exe = String(cString: path)
    let bundle = exe.split(separator: "/").first { $0.hasSuffix(".app") }.map { String($0.dropLast(4)) }
    return Dependence(app: bundle ?? (exe as NSString).lastPathComponent, pid: r, launcher: false)
}
