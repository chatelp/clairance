// The launcher's control socket: how `clairance --start`, typed in a terminal
// or run by an agent, has the running launcher start production instead of
// starting it itself.
//
// For macOS a process depends on the app that started it, and runs on that
// app's Local Network permission. Production started by the command line from
// a Claude session depended on Claude, and when Claude updated itself and
// restarted, production kept running without reaching the station: a day and
// a half without a transcription (16-18/10). Started by the launcher, it
// depends on the launcher, an app of its own.
//
// A Unix socket in the user's Application Support, open to this user only: not
// a TCP port or a URL scheme, which a web page could reach. One request per
// connection, a line of JSON; the launcher answers with lines of JSON, notes
// while it works and a last one that says how it ended.

import Foundation

let supportDir = FileManager.default.homeDirectoryForCurrentUser
    .appendingPathComponent("Library/Application Support/Clairance").path
let socketPath = (supportDir as NSString).appendingPathComponent("launcher.sock")

struct ControlRequest: Codable {
    var cmd: String              // start, stop, start-test, stop-test, status
    var config: String?          // a test instance's config, absolute
    var binary: String?          // a test instance's co-atc, absolute
    var seconds: Double?         // how long a test instance may run
}

struct ControlReply: Codable {
    var note: String?            // a step, while the launcher works
    var done: Bool?              // the last line
    var ok: Bool?
    var message: String?

    static func step(_ s: String) -> ControlReply { ControlReply(note: s) }
    static func end(_ ok: Bool, _ s: String) -> ControlReply { ControlReply(done: true, ok: ok, message: s) }
}

private func unixAddress() -> sockaddr_un? {
    var addr = sockaddr_un()
    addr.sun_family = sa_family_t(AF_UNIX)
    addr.sun_len = UInt8(MemoryLayout<sockaddr_un>.size)
    let bytes = Array(socketPath.utf8)
    guard bytes.count < MemoryLayout.size(ofValue: addr.sun_path) else { return nil }
    withUnsafeMutableBytes(of: &addr.sun_path) { $0.copyBytes(from: bytes) }
    return addr
}

private func withSockaddr<T>(_ addr: inout sockaddr_un, _ body: (UnsafePointer<sockaddr>, socklen_t) -> T) -> T {
    withUnsafePointer(to: &addr) {
        $0.withMemoryRebound(to: sockaddr.self, capacity: 1) { body($0, socklen_t(MemoryLayout<sockaddr_un>.size)) }
    }
}

private func noSigpipe(_ fd: Int32) {
    var one: Int32 = 1
    setsockopt(fd, SOL_SOCKET, SO_NOSIGPIPE, &one, socklen_t(MemoryLayout<Int32>.size))
}

/// A connection to the running launcher, or nil when none answers.
func connectControl() -> Int32? {
    guard var addr = unixAddress() else { return nil }
    let fd = socket(AF_UNIX, SOCK_STREAM, 0)
    guard fd >= 0 else { return nil }
    if withSockaddr(&addr, { connect(fd, $0, $1) }) == 0 {
        noSigpipe(fd)
        return fd
    }
    close(fd)
    return nil
}

func writeLine<T: Encodable>(_ fd: Int32, _ value: T) {
    guard var data = try? JSONEncoder().encode(value) else { return }
    data.append(0x0A)
    data.withUnsafeBytes { raw in
        var sent = 0
        while sent < raw.count {
            let n = send(fd, raw.baseAddress! + sent, raw.count - sent, 0)
            if n <= 0 { return }
            sent += n
        }
    }
}

/// Reads lines until the other side closes; each one goes to the handler,
/// which returns false to stop reading.
func readLines(_ fd: Int32, _ handle: (String) -> Bool) {
    var pending = Data()
    var chunk = [UInt8](repeating: 0, count: 4096)
    while true {
        let n = recv(fd, &chunk, chunk.count, 0)
        if n <= 0 { return }
        pending.append(contentsOf: chunk[0..<n])
        while let nl = pending.firstIndex(of: 0x0A) {
            let line = String(decoding: pending[pending.startIndex..<nl], as: UTF8.self)
            pending.removeSubrange(pending.startIndex...nl)
            if !handle(line) { return }
        }
    }
}

/// Sends one request and hands every note to the caller; returns the last line.
func ask(_ fd: Int32, _ request: ControlRequest, onNote: (String) -> Void) -> ControlReply? {
    defer { close(fd) }
    writeLine(fd, request)
    var last: ControlReply?
    readLines(fd) { line in
        guard let r = try? JSONDecoder().decode(ControlReply.self, from: Data(line.utf8)) else { return true }
        if let n = r.note { onNote(n) }
        if r.done == true { last = r; return false }
        return true
    }
    return last
}

/// Opens the control socket and serves it on a thread of its own; each request
/// is handled on the main actor, its replies written as they come. Returns
/// false when another launcher already answers there.
@discardableResult
func serveControl(_ handle: @escaping @MainActor (ControlRequest, @escaping (ControlReply) -> Void) async -> Void) -> Bool {
    // A peer that leaves before the answer must not take the launcher with it:
    // SO_NOSIGPIPE on the connection did not spare it a SIGPIPE (measured on
    // 24/10, a connection opened and closed at once killed the launcher).
    signal(SIGPIPE, SIG_IGN)
    try? FileManager.default.createDirectory(atPath: supportDir, withIntermediateDirectories: true,
                                             attributes: [.posixPermissions: 0o700])
    if let other = connectControl() {
        close(other)
        return false
    }
    unlink(socketPath) // left by a launcher that did not close it
    guard var addr = unixAddress() else { return false }
    let fd = socket(AF_UNIX, SOCK_STREAM, 0)
    guard fd >= 0, withSockaddr(&addr, { bind(fd, $0, $1) }) == 0, listen(fd, 8) == 0 else {
        if fd >= 0 { close(fd) }
        return false
    }
    chmod(socketPath, 0o600)
    Thread.detachNewThread {
        while true {
            let c = accept(fd, nil, nil)
            if c < 0 { continue }
            noSigpipe(c)
            // The folder is this user's alone; the peer's identity is checked all the same.
            var uid: uid_t = 0, gid: gid_t = 0
            guard getpeereid(c, &uid, &gid) == 0, uid == getuid() else { close(c); continue }
            var request: ControlRequest?
            readLines(c) { line in
                request = try? JSONDecoder().decode(ControlRequest.self, from: Data(line.utf8))
                return false
            }
            // Nothing readable, often a launcher checking that this one answers.
            guard let r = request else {
                close(c)
                continue
            }
            Task { @MainActor in
                await handle(r) { writeLine(c, $0) }
                close(c)
            }
        }
    }
    return true
}

// MARK: - What the launcher started

/// What the launcher started, kept across its own restarts: production's
/// script and the test instances, each by PID and start time; a test instance
/// with the time it must stop.
struct Started: Codable, Sendable {
    var role: String       // "production" (its script) or "test" (co-atc)
    var pid: Int32
    var start: Double
    var config: String
    var until: Date?
}

private let startedPath = (supportDir as NSString).appendingPathComponent("started.json")

func loadStarted() -> [Started] {
    let d = JSONDecoder()
    d.dateDecodingStrategy = .iso8601
    guard let data = FileManager.default.contents(atPath: startedPath),
          let s = try? d.decode([Started].self, from: data) else { return [] }
    return s
}

func saveStarted(_ s: [Started]) {
    let e = JSONEncoder()
    e.dateEncodingStrategy = .iso8601
    e.outputFormatting = [.prettyPrinted, .sortedKeys]
    try? FileManager.default.createDirectory(atPath: supportDir, withIntermediateDirectories: true,
                                             attributes: [.posixPermissions: 0o700])
    try? e.encode(s).write(to: URL(fileURLWithPath: startedPath), options: .atomic)
}

// MARK: - A log's new lines

/// The lines a log gains from now on, for the command line to show while the
/// launcher works: the production script narrates its own start.
final class LogTail {
    let path: String
    private var offset: UInt64

    init(_ path: String) {
        self.path = path
        offset = (try? FileManager.default.attributesOfItem(atPath: path)[.size] as? UInt64) ?? 0
    }

    func newLines() -> [String] {
        guard let h = FileHandle(forReadingAtPath: path) else { return [] }
        defer { try? h.close() }
        try? h.seek(toOffset: offset)
        let data = h.readDataToEndOfFile()
        guard let nl = data.lastIndex(of: 0x0A) else { return [] }
        offset += UInt64(nl - data.startIndex + 1)
        return String(decoding: data[data.startIndex..<nl], as: UTF8.self)
            .split(separator: "\n").map(String.init)
    }
}

/// The last line of a file that says something, for a failure's message.
func lastLine(_ path: String) -> String {
    guard let text = try? String(contentsOfFile: path, encoding: .utf8) else { return "" }
    return text.split(separator: "\n").last { !$0.trimmingCharacters(in: .whitespaces).isEmpty }.map(String.init) ?? ""
}
