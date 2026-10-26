// The checks: from what one refresh saw, the incidents that hold and the menu's
// health lines (docs-fr/34-lanceur.md, "L'étape 1"; the words are
// docs-fr/design/10-lanceur.md). Each threshold is the plan's, measured or
// argued there.

import Foundation

/// Production stopped without anyone asking, or a start that did not end with
/// production running.
enum Down: Sendable {
    case stoppedByItself(at: Date)
    case didNotStart(at: Date, cause: String, notify: Bool)
}

/// A helper left without an instance, and since when the launcher has seen it so.
struct Orphan: Sendable {
    let helper: Helper
    let since: Date
    let memory: Int64?
}

/// What one refresh saw, for the checks.
struct Observation: Sendable {
    var now = Date()
    var production: Instance?
    var detail: HealthDetail?
    var detailMissing = false          // the server answers 404 there: built before the route
    var secondsPerTransmission: Double?
    var station = StationRead()
    var sourceAnswersLauncher: Bool?   // the launcher's own read of production's ADS-B source, when co-atc reads nothing
    var pressure: Int32 = 1
    var coatcMemory: Int64?
    var sidecarMemory: Int64?
    var linkLostAt: Date?              // production had a station link, and it has gone
    var noFrequencySince: Date?        // the station on air, and co-atc without a frequency
    var down: Down?
    var archiveVolume: String?
    var dataDir = ""
    var silenceLimit: TimeInterval = 600
    var tests: [Instance] = []
    var orphans: [Orphan] = []
}

// Thresholds (34-lanceur.md, the plan accepted on 24/10).
let deafAfter: TimeInterval = 30          // no ADS-B read for 30 s, then held 15 s
let silentAudioAfter: TimeInterval = 60   // no byte decoded for 60 s
let diskFault: Int64 = 20_000_000_000
let diskCritical: Int64 = 5_000_000_000

/// From 8 h to 23 h, the longest silence of an hour was 83 s (03 and 16/10):
/// 10 min then; at night it reached 21 min: 45 min then.
func silenceLimit(at d: Date) -> TimeInterval {
    let h = Calendar.current.component(.hour, from: d)
    return (8..<23).contains(h) ? 600 : 2700
}

func assess(_ o: Observation) -> (findings: [Finding], lines: [HealthLine], facts: [String: String]) {
    var f: [Finding] = []
    var lines: [HealthLine] = []
    var facts: [String: String] = [:]
    let now = o.now
    let p = o.production
    let d = o.detail

    // Down: production stopped by itself, or did not start.
    switch o.down {
    case .stoppedByItself(let at):
        f.append(Finding(key: "production-stopped", kind: "production", down: true, began: at,
            cause: "Stopped by itself at \(clockTime(at))",
            notice: Notice(title: "Production stopped by itself",
                           body: "co-atc exited at \(clockTime(at)). Nothing is transcribed or archived until it starts again.",
                           sound: true, level: .timeSensitive, click: .log, category: "down"),
            recovery: { lasted, _ in Notice(title: "Production running again",
                                            body: "Started at \(clockTime(Date())), after \(brandDuration(lasted)) stopped.",
                                            level: .passive, click: .clairance) }))
    case .didNotStart(let at, let cause, let notify):
        f.append(Finding(key: "production-start", kind: "production", down: true, quiet: !notify, began: at,
            cause: cause,
            notice: Notice(title: "Production did not start", body: cause + ".",
                           sound: true, level: .timeSensitive, click: .log, category: "down"),
            recovery: { _, _ in Notice(title: "Production running",
                                       body: "Started at \(clockTime(Date())), after the start that failed at \(clockTime(at)).",
                                       level: .passive, click: .clairance) }))
    case nil:
        break
    }

    guard let p else { return (f, lines, facts) }

    // The server answers.
    if !p.answers {
        f.append(Finding(key: "production-silent", kind: "production", hold: 45, down: true,
            cause: "co-atc runs, its server does not answer",
            notice: Notice(title: "Production not answering",
                           body: "co-atc is running but its server has not answered for 45 s. The page and the map are frozen.",
                           sound: true, level: .timeSensitive, click: .log, category: "silent"),
            recovery: { lasted, _ in Notice(title: "Production answering again",
                                            body: "After \(brandDuration(lasted)) without an answer.",
                                            level: .passive, click: .clairance) }))
        lines.append(HealthLine(light: o.station.answered ? .good : .unknown, name: "Station",
                                fact: o.station.answered ? stationFact(o.station) : "not answering this Mac"))
        for name in ["ADS-B", "Audio", "Transcription"] { lines.append(HealthLine(light: .unknown, name: name, fact: "unknown")) }
        lines += localLines(o, &f, &facts)
        return (f, lines, facts)
    }

    // A co-atc built before /api/v1/health/detail answers 404 there: its audio,
    // transcription and disks cannot be watched, which must show rather than
    // pass for health (26/10: production had no ffmpeg, and nothing said so).
    if o.detailMissing {
        f.append(Finding(key: "unwatched", kind: "production",
            cause: "Audio not watched: this co-atc is too old for the launcher",
            notice: Notice(title: "Production not fully watched",
                           body: "This co-atc has no health detail (/api/v1/health/detail): its audio, transcription and disks are not watched. A co-atc built from local has it.",
                           click: .log)))
    }

    // The app production depends on, for its local network (measured on 24/10).
    var deafLine: HealthLine?
    if let dep = p.dependence, !dep.launcher {
        if dep.pid == p.pid {
            f.append(Finding(key: "deaf", kind: "station",
                cause: "The app that started it quit: macOS took its local network",
                notice: Notice(title: "Production deaf: no local network",
                               body: "The app that started it quit: macOS has taken the local network from it. Restarting it from Clairance gives it back.",
                               sound: true, level: .timeSensitive, click: .log, category: "deaf"),
                recovery: { lasted, _ in Notice(title: "Production hears the station again",
                                                body: "After \(brandDuration(lasted)) deaf.", level: .passive, click: .clairance) }))
            deafLine = HealthLine(light: .fault, name: "Station", fact: "no local network for co-atc")
        } else {
            let app = dep.app
            f.append(Finding(key: "outside", kind: "production",
                cause: "Started from \(app): it stops hearing if \(app) quits",
                notice: Notice(title: "Production depends on \(app)",
                               body: "It was started from \(app): if \(app) quits or updates, production loses the local network. Restart it from Clairance to keep it.",
                               category: "outside"),
                recovery: { _, _ in Notice(title: "Production runs from Clairance",
                                           body: "It no longer depends on \(app).", level: .passive) }))
        }
    }

    // The station, seen by production: its last ADS-B read.
    let lastRead = d?.adsb?.last_fetch ?? p.lastFetch
    let readAge = lastRead.map { now.timeIntervalSince($0) } ?? TimeInterval(d?.uptime_s ?? 0)
    if deafLine == nil && readAge > deafAfter {
        let why = d?.adsb?.failing_why ?? ""
        let since = lastRead ?? now.addingTimeInterval(-readAge)
        if o.sourceAnswersLauncher == true {
            f.append(Finding(key: "deaf", kind: "station", hold: 15, began: since,
                cause: "The station answers this Mac, not co-atc: macOS took the local network from co-atc",
                notice: Notice(title: "Production deaf: no local network",
                               body: "The station answers this Mac but not co-atc: macOS has taken the local network from it. Restarting it from Clairance gives it back.",
                               sound: true, level: .timeSensitive, click: .log, category: "deaf"),
                recovery: { lasted, _ in Notice(title: "Production hears the station again",
                                                body: "After \(brandDuration(lasted)) deaf.", level: .passive, click: .clairance) }))
            deafLine = HealthLine(light: .fault, name: "Station", fact: "answers this Mac, not co-atc")
        } else {
            let because = why.isEmpty ? "" : " (\(why))"
            f.append(Finding(key: "deaf", kind: "station", hold: 15, began: since,
                cause: "The station does not answer" + because,
                notice: Notice(title: "Production deaf: the station does not answer",
                               body: "Neither co-atc nor this Mac has reached the station for \(brandDuration(readAge))\(because). ADS-B and audio stop until it answers.",
                               sound: true, level: .timeSensitive, click: .log),
                recovery: { lasted, _ in Notice(title: "Production hears the station again",
                                                body: "After \(brandDuration(lasted)) deaf.", level: .passive, click: .clairance) }))
            deafLine = HealthLine(light: .fault, name: "Station", fact: "not reached for \(brandDuration(readAge))" + because)
        }
    }

    // The station link: radio-ctl-sync, while production had one.
    if let lost = o.linkLostAt {
        f.append(Finding(key: "link", kind: "station", hold: 15, began: lost,
            cause: "The station link stopped at \(clockTime(lost))",
            notice: Notice(title: "Station link stopped",
                           body: "radio-ctl-sync exited: Clairance no longer follows what the station puts on air. The current streams keep playing.",
                           click: .log),
            recovery: { lasted, _ in Notice(title: "Station link running again", body: "After \(brandDuration(lasted)).", level: .passive) }))
    }
    if let l = deafLine {
        lines.append(l)
    } else if let lost = o.linkLostAt {
        lines.append(HealthLine(light: .fault, name: "Station", fact: "link stopped \(brandDuration(now.timeIntervalSince(lost))) ago"))
    } else {
        lines.append(HealthLine(light: o.station.answered ? .good : .unknown, name: "Station",
                                fact: o.station.answered ? stationFact(o.station) : "not answering this Mac"))
    }

    // ADS-B.
    if readAge > deafAfter {
        lines.append(HealthLine(light: .fault, name: "ADS-B", fact: "nothing read for \(brandDuration(readAge))"))
    } else if let a = d?.adsb {
        lines.append(HealthLine(light: .good, name: "ADS-B",
                                fact: "\(brandNumber(a.aircraft)) aircraft, \(brandNumber(Int(a.messages_per_second.rounded()))) msg/s"))
    } else {
        lines.append(HealthLine(light: lastRead == nil ? .unknown : .good, name: "ADS-B", fact: lastRead == nil ? "unknown" : "reading"))
    }

    // Audio: bytes decoded, per frequency (16-18/10 would have shown here).
    let freqs = (d?.frequencies ?? []).filter { $0.status != "stopped" }
    func silentFor(_ fr: HealthDetail.Frequency) -> TimeInterval {
        if let b = fr.last_byte_at { return now.timeIntervalSince(b) }
        return TimeInterval(d?.uptime_s ?? 0)
    }
    let silent = freqs.filter { silentFor($0) > silentAudioAfter }
    let receiving = freqs.filter { silentFor($0) <= silentAudioAfter }
    for fr in silent {
        let label = frequencyLabel(fr)
        let age = silentFor(fr)
        let onAir = o.station.onAir.contains(label) ? " while on air at the station" : ""
        let others = receiving.isEmpty ? "" : receiving.count == 1 ? " The other frequency is receiving." : " The other \(receiving.count) frequencies are receiving."
        f.append(Finding(key: "audio-\(fr.id)", kind: "audio", began: now.addingTimeInterval(-age),
            cause: "No audio on \(label) for \(brandDuration(age))",
            notice: Notice(title: "No audio on \(label)",
                           body: "\(fr.name) has sent nothing for \(brandDuration(age))\(onAir).\(others)", click: .clairance),
            recovery: { lasted, _ in Notice(title: "Audio back on \(label)", body: "After \(brandDuration(lasted)) silent.",
                                            level: .passive, click: .clairance) }))
    }
    if d == nil {
        lines.append(HealthLine(light: o.detailMissing ? .fault : .unknown, name: "Audio",
                                fact: o.detailMissing ? "not watched: co-atc too old" : "unknown"))
    } else if freqs.isEmpty && o.station.mode != "chaine" {
        lines.append(HealthLine(light: .off, name: "Audio", fact: "nothing on air"))
    } else if freqs.isEmpty {
        // The station's frequencies reach co-atc through radio-ctl-sync: waiting
        // for it is normal at a start or a change at the station, for as long as
        // a frequency may stay silent; longer, it is a fault (design 10).
        let since = o.noFrequencySince ?? now
        let age = now.timeIntervalSince(since)
        if age <= silentAudioAfter {
            lines.append(HealthLine(light: .switching, name: "Audio", fact: "waiting for the station link"))
        } else {
            // In a sentence the list takes "and"; the menu's lines keep "124.350, 126.425" (design 10).
            let air = o.station.onAir
            let list = air.count < 2 ? air.joined() : air.dropLast().joined(separator: ", ") + " and " + air.last!
            let received = air.count == 1 ? "has not received it" : air.count == 2 ? "has received neither" : "has received none of them"
            let body = air.isEmpty
                ? "The station is on air; co-atc has received no frequency from the station link for \(brandDuration(age))."
                : "The station has \(list) on air; co-atc \(received) from the station link for \(brandDuration(age))."
            f.append(Finding(key: "audio-none", kind: "audio", began: since,
                cause: "No frequency while the station is on air",
                notice: Notice(title: "No frequency for co-atc", body: body, click: .clairance),
                recovery: { lasted, _ in Notice(title: "Frequencies received again", body: "After \(brandDuration(lasted)) without one.",
                                                level: .passive, click: .clairance) }))
            lines.append(HealthLine(light: .fault, name: "Audio", fact: "no frequency while the station is on air"))
        }
    } else if let first = silent.first {
        let names = silent.map(frequencyLabel).joined(separator: " and ")
        lines.append(HealthLine(light: .fault, name: "Audio", fact: "\(names) silent for \(brandDuration(silentFor(first)))"))
    } else {
        let all = freqs.count == 1 ? "1 frequency, receiving" : freqs.count == 2 ? "2 frequencies, both receiving" : "\(freqs.count) frequencies, all receiving"
        lines.append(HealthLine(light: .good, name: "Audio", fact: all))
    }

    // Transcription.
    var transcription: HealthLine?
    if let s = d?.sidecar {
        if s.status == "off" {
            transcription = HealthLine(light: .off, name: "Transcription", fact: "off")
        } else if !s.reachable {
            f.append(Finding(key: "sidecar", kind: "transcription", hold: 45,
                cause: "The transcription sidecar does not answer",
                notice: Notice(title: "Transcription silent", body: "The sidecar has not answered for 45 s.", click: .log),
                recovery: { lasted, _ in Notice(title: "Transcription back", body: "After \(brandDuration(lasted)) silent.",
                                                level: .passive, click: .clairance) }))
            transcription = HealthLine(light: .fault, name: "Transcription", fact: "sidecar silent")
        } else {
            for lang in s.degraded ?? [] {
                let name = languageName(lang)
                let fallback = lang == "fr" ? ": transmissions are read by the English model only." : "."
                f.append(Finding(key: "degraded-\(lang)", kind: "transcription", hold: 60,
                    cause: "\(name) model not loaded",
                    notice: Notice(title: "\(name) transcription unavailable", body: "The \(name) model is not loaded\(fallback)", click: .log),
                    recovery: { lasted, _ in Notice(title: "\(name) transcription back", body: "After \(brandDuration(lasted)).", level: .passive) }))
                transcription = HealthLine(light: .fault, name: "Transcription", fact: "\(name) model not loaded")
            }
        }
    } else if !p.sidecarURL.isEmpty && p.sidecarHealth == "not answering" {
        transcription = HealthLine(light: .fault, name: "Transcription", fact: "sidecar silent")
    }

    // Nothing cut from the audio for a long time, while it arrives and the station is on air.
    let transcribing = receiving.filter(\.transcribes)
    if o.station.mode == "chaine", !transcribing.isEmpty {
        let last = transcribing.compactMap(\.last_segment_at).max()
        let age = last.map { now.timeIntervalSince($0) } ?? TimeInterval(d?.uptime_s ?? 0)
        if age > o.silenceLimit {
            let n = transcribing.count
            let since = last.map { ", and no transmission has been cut since \(clockTime($0))" } ?? ", and no transmission has been cut since co-atc started"
            f.append(Finding(key: "silence", kind: "transcription", began: last,
                cause: "Nothing transcribed for \(brandDuration(age))",
                notice: Notice(title: "Nothing transcribed for \(brandDuration(age))",
                               body: "Audio arrives on \(n == 1 ? "1 frequency" : "\(n) frequencies") on air\(since).", click: .clairance),
                recovery: { lasted, _ in Notice(title: "Transmissions again", body: "After \(brandDuration(lasted)) without one.",
                                                level: .passive, click: .clairance) }))
            transcription = transcription ?? HealthLine(light: .fault, name: "Transcription", fact: "nothing transcribed for \(brandDuration(age))")
        }
    }
    if let t = transcription {
        lines.append(t)
    } else if let s = d?.sidecar {
        var fact = "queue \(s.queue?.first ?? 0)"
        if let spt = o.secondsPerTransmission { fact += ", \(String(format: "%.1f", spt)) s a transmission" }
        lines.append(HealthLine(light: .good, name: "Transcription", fact: fact))
    } else {
        lines.append(HealthLine(light: p.sidecarURL.isEmpty ? .off : .unknown, name: "Transcription",
                                fact: p.sidecarURL.isEmpty ? "off" : "unknown"))
    }

    // The archive disk, when the archive is meant to be on a volume (the server's
    // cap has a default, so a cap does not say an archive was configured).
    if let a = d?.audio_archive, o.archiveVolume != nil, !a.available {
        let name = o.archiveVolume.map { "“\($0)” is not mounted" } ?? "The archive disk is not mounted"
        f.append(Finding(key: "archive", kind: "disk", hold: 30,
            cause: "Archive disk missing",
            notice: Notice(title: "Archive disk missing", body: "\(name): the audio archive is not written. Transcription continues.",
                           click: .finder("/Volumes"), category: "archive"),
            recovery: { lasted, _ in Notice(title: "Archive disk back", body: "After \(brandDuration(lasted)).", level: .passive) }))
    }
    lines += localLines(o, &f, &facts)
    return (f, lines, facts)
}

/// "on air 124.350, 126.425", or what the station is doing instead.
func stationFact(_ s: StationRead) -> String {
    if s.mode == "chaine" { return s.onAir.isEmpty ? "on air" : "on air " + s.onAir.joined(separator: ", ") }
    return "nothing on air"
}

/// Disk and memory: read on this Mac, whether the server answers or not.
private func localLines(_ o: Observation, _ f: inout [Finding], _ facts: inout [String: String]) -> [HealthLine] {
    var lines: [HealthLine] = []
    let d = o.detail
    if let free = d?.databases.disk_free_bytes {
        facts["disk_free"] = brandGB(free)
        if free < diskCritical {
            f.append(Finding(key: "disk-critical", kind: "disk",
                cause: "\(brandGB(free)) free on the internal disk",
                notice: Notice(title: "Disk nearly full: \(brandGB(free)) left", body: "Less than 5 GB free: the databases may stop being written.",
                               sound: true, level: .timeSensitive, click: .finder(o.dataDir))))
        }
        if free < diskFault {
            f.append(Finding(key: "disk", kind: "disk",
                cause: "\(brandGB(free)) free on the internal disk",
                notice: Notice(title: "Disk almost full: \(brandGB(free)) left", body: "The internal disk has less than 20 GB free.",
                               click: .finder(o.dataDir)),
                recovery: { _, facts in Notice(title: "Disk space back: \(facts["disk_free"] ?? "more") free", body: "", level: .passive) }))
            lines.append(HealthLine(light: .fault, name: "Disk", fact: "\(brandGB(free)) free"))
        } else if let a = d?.audio_archive, o.archiveVolume != nil, !a.available {
            lines.append(HealthLine(light: .fault, name: "Disk", fact: "archive disk missing"))
        } else {
            var fact = "\(brandGB(free)) free"
            if let a = d?.audio_archive, let b = a.bytes, a.cap_bytes > 0 {
                fact += " · archive \(Int((Double(b) / 1e9).rounded())) of \(Int((Double(a.cap_bytes) / 1e9).rounded())) GB"
            }
            lines.append(HealthLine(light: .good, name: "Disk", fact: fact))
        }
    }
    if o.pressure >= 2 {
        let level = o.pressure >= 4 ? "critical" : "warning"
        var uses: [String] = []
        if let s = o.sidecarMemory { uses.append("The sidecar uses \(brandGB(s))") }
        if let c = o.coatcMemory { uses.append("co-atc \(brandGB(c))") }
        let body = "macOS is short of memory." + (uses.isEmpty ? "" : " " + uses.joined(separator: ", ") + ".")
        f.append(Finding(key: "memory", kind: "memory", hold: 120,
            cause: "Memory pressure \(level)",
            notice: Notice(title: "Memory pressure: \(level)", body: body, sound: o.pressure >= 4, click: .activityMonitor),
            recovery: { lasted, _ in Notice(title: "Memory pressure normal", body: "After \(brandDuration(lasted)).", level: .passive) }))
        lines.append(HealthLine(light: .fault, name: "Memory", fact: "pressure \(level)"))
    } else {
        lines.append(HealthLine(light: .good, name: "Memory", fact: "normal"))
    }
    return lines
}

/// Test instances and helpers: their own notifications, never the sign.
func assessOthers(_ o: Observation) -> [Finding] {
    var f: [Finding] = []
    for t in o.tests where t.until == nil && t.proc.elapsed > 12 * 3600 {
        let port = t.port.map { "Port \($0)" } ?? t.name
        f.append(Finding(key: "test-\(t.pid)", kind: "instances", production: false,
            cause: "\(t.name) has run for \(t.elapsed)",
            notice: Notice(title: "Test instance still running", body: "\(port) has run for \(t.elapsed).",
                           category: "instance", target: ["pid": String(t.pid), "start": String(t.proc.start)])))
    }
    for o in o.orphans where Date().timeIntervalSince(o.since) > 600 {
        let h = o.helper
        let sidecar = h.what.hasPrefix("sidecar")
        let uses = o.memory.map { " and uses \(brandGB($0))" } ?? ""
        f.append(Finding(key: "orphan-\(h.pid)", kind: "instances", production: false,
            cause: (sidecar ? "Sidecar" : "Station link") + " without an instance",
            notice: Notice(title: sidecar ? "Sidecar left without an instance" : "Station link left without an instance",
                           body: sidecar ? "A transcription sidecar (PID \(h.pid)) runs with no co-atc\(uses)."
                                         : "radio-ctl-sync (PID \(h.pid)) runs with no co-atc.",
                           category: "helper", target: ["pid": String(h.pid), "start": String(h.proc.start)])))
    }
    return f
}
