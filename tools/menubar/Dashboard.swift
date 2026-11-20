// The launcher's window, "Dashboard" (docs-fr/design/11-lanceur-fenetre.md;
// owner's request, 20/11: "a real window", and who is connected). One column:
// the state and its cause, then Permissions when one is missing, Health,
// Connected and Instances. The menu keeps what one reaches for from the bar.
//
// A Mac window with the system's own controls, fonts and colours; Clairance
// is in the sign, the shapes of the lights, the words and the numbers, which
// are the menu's: a thing never has two names.
//
// An NSWindow rather than a SwiftUI Window scene: it must open from the Dock
// icon, from the menu and at launch, never when a command opened the launcher,
// and a scene would open by itself at every launch.

import AppKit
import Combine
import SwiftUI

// MARK: - The window's words

extension Light {
    /// The system's colours, which follow light, dark and increased contrast.
    var color: Color {
        switch self {
        case .good: return .green
        case .fault: return .orange
        case .down: return .red
        case .unknown, .off, .switching: return .secondary
        }
    }

    /// The shape's word, for VoiceOver.
    var word: String {
        switch self {
        case .good: return "OK"
        case .fault: return "fault"
        case .down: return "down"
        case .unknown: return "unknown"
        case .off: return "off"
        case .switching: return "switching"
        }
    }
}

/// Who is connected, or why the launcher cannot say: each empty state has its
/// own reason (design 11).
enum Connected {
    case accounts([HealthDetail.Present])
    case stopped        // production is stopped
    case silent         // the server does not answer
    case noAccounts     // no sign-in required, or usage not followed: nobody to name
    case tooOld         // a co-atc built before it said who is here
}

/// "Listening": the menu's initial in capitals for the window's right column.
func capitalised(_ s: String) -> String {
    s.prefix(1).uppercased() + s.dropFirst()
}

/// "2 pages open for 2 h 14 min", "1 page open for 5 min", "No page open".
func pagesLine(_ a: HealthDetail.Present, now: Date = Date()) -> String {
    guard a.pages > 0 else { return "No page open" }
    let since = a.since.map { " for " + shortElapsed(now.timeIntervalSince($0)) } ?? ""
    return "\(brandNumber(a.pages)) page\(a.pages == 1 ? "" : "s") open" + since
}

/// "Listening to 124.350 for 40 min, 126.425 for 5 min"; nil without a stream.
func listeningLine(_ a: HealthDetail.Present, _ detail: HealthDetail?, now: Date = Date()) -> String? {
    guard !a.listening.isEmpty else { return nil }
    let parts = a.listening.map { "\(frequencyName($0.freq, detail)) for \(shortElapsed(now.timeIntervalSince($0.since)))" }
    return "Listening to " + parts.joined(separator: ", ")
}

/// "124.350" for a frequency's id, as the health lines name it.
func frequencyName(_ id: String, _ detail: HealthDetail?) -> String {
    if let known = detail?.frequencies.first(where: { $0.id == id }) { return frequencyLabel(known) }
    return id.count == 6 && id.allSatisfy(\.isNumber) ? "\(id.prefix(3)).\(id.suffix(3))" : id
}

extension Production {
    var connected: Connected {
        guard let p = production else { return .stopped }
        guard p.answers else { return .silent }
        guard let d = observation.detail else { return observation.detailMissing ? .tooOld : .silent }
        guard let present = d.present, let accounts = d.accounts else { return .tooOld }
        guard accounts.sign_in, accounts.usage else { return .noAccounts }
        return .accounts(present.sorted { $0.user.localizedCaseInsensitiveCompare($1.user) == .orderedAscending })
    }

    /// "2 connected · pierre, famille", "5 connected · pierre, famille, club and
    /// 2 more": the menu's line, nil when nobody is there or it is not known.
    var connectedLine: String? {
        guard case .accounts(let a) = connected, !a.isEmpty else { return nil }
        let names = a.prefix(3).map(\.user).joined(separator: ", ")
        return "\(brandNumber(a.count)) connected · \(names)" + (a.count > 3 ? " and \(brandNumber(a.count - 3)) more" : "")
    }
}

// MARK: - The view

/// Which instances are unfolded: the window's own, kept while it lives.
@MainActor
final class Unfolded: ObservableObject {
    @Published var pids: Set<Int32> = []
}

struct Dashboard: View {
    @ObservedObject var p: Production
    @ObservedObject var unfolded: Unfolded

    var body: some View {
        // A grouped form, one column, that scrolls. The header rides the first
        // section's own header, which has no card behind it.
        Form {
            if !p.permissions.isEmpty {
                Section { permissions } header: { head("Permissions", first: true) }
            }
            Section { health } header: { head("Health", first: p.permissions.isEmpty) }
            Section { connected } header: { head("Connected", count: connectedCount) }
            Section { instances } header: { head("Instances") }
        }
        .formStyle(.grouped)
        // Nothing moves: a value changes at once, a line comes or goes.
        .transaction { $0.animation = nil }
    }

    private func head(_ title: String, count: Int? = nil, first: Bool = false) -> some View {
        VStack(alignment: .leading, spacing: 18) {
            if first { DashboardHeader(p: p) }
            (Text(title) + Text(count.map { " · \(brandNumber($0))" } ?? "").foregroundColor(.secondary))
                .font(.headline)
                .monospacedDigit()
        }
        .textCase(nil)
    }

    // MARK: Permissions

    @ViewBuilder private var permissions: some View {
        // Two of the same pane: two lines, one button, on the last.
        let last = Dictionary(p.permissions.enumerated().map { ($0.element.settings, $0.offset) }, uniquingKeysWith: { $1 })
        ForEach(Array(p.permissions.enumerated()), id: \.offset) { n, m in
            HStack(alignment: .firstTextBaseline) {
                shape(.fault)
                Text("\(m.app) · \(m.what) not allowed")
                Spacer()
                if last[m.settings] == n {
                    Button(m.settings == notificationSettings ? "Open Notification Settings…" : "Open Local Network Settings…") {
                        if let u = URL(string: m.settings) { NSWorkspace.shared.open(u) }
                    }
                }
            }
            .accessibilityElement(children: .combine)
        }
    }

    // MARK: Health

    private func shape(_ l: Light) -> some View {
        Text(l.rawValue)
            .foregroundStyle(l.color)
            .frame(width: 14, alignment: .center)
            .accessibilityLabel(l.word)
    }

    @ViewBuilder private var health: some View {
        ForEach(p.health, id: \.self) { line in
            LabeledContent {
                Text(capitalised(line.fact))
                    .monospacedDigit()
                    .multilineTextAlignment(.trailing)
            } label: {
                HStack(alignment: .firstTextBaseline, spacing: 6) {
                    shape(line.light)
                    Text(line.name)
                }
            }
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("\(line.name): \(line.fact), \(line.light.word)")
        }
    }

    // MARK: Connected

    private var connectedCount: Int? {
        if case .accounts(let a) = p.connected, !a.isEmpty { return a.count }
        return nil
    }

    private func nameWidth(_ accounts: [HealthDetail.Present]) -> CGFloat {
        let font = NSFont.systemFont(ofSize: NSFont.systemFontSize, weight: .semibold)
        let widest = accounts.map { ($0.user as NSString).size(withAttributes: [.font: font]).width }.max() ?? 0
        return min(max(widest.rounded(.up) + 4, 72), 200)
    }

    @ViewBuilder private var connected: some View {
        switch p.connected {
        case .accounts(let accounts) where accounts.isEmpty:
            Text("No one connected").foregroundStyle(.secondary)
        case .accounts(let accounts):
            let width = nameWidth(accounts)
            ForEach(accounts, id: \.user) { a in
                let sound = listeningLine(a, p.observation.detail, now: p.observation.now)
                HStack(alignment: .firstTextBaseline, spacing: 16) {
                    Text(a.user)
                        .fontWeight(.semibold)
                        .lineLimit(1)
                        .truncationMode(.tail)
                        .frame(width: width, alignment: .leading)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(pagesLine(a, now: p.observation.now) + (sound == nil ? " · sound off" : ""))
                        if let sound { Text(sound) }
                    }
                    .foregroundStyle(.secondary)
                    .monospacedDigit()
                    Spacer(minLength: 0)
                }
                .accessibilityElement(children: .combine)
            }
        case .stopped:
            Text("No one: production is stopped").foregroundStyle(.secondary)
        case .silent:
            Text("○ Not known: the server does not answer").foregroundStyle(.secondary)
        case .noAccounts:
            Text("Not known: Clairance opens without an account here").foregroundStyle(.secondary)
        case .tooOld:
            Text("Not known: this version of Clairance does not say").foregroundStyle(.secondary)
        }
    }

    // MARK: Instances

    private func unfolding(_ pid: Int32) -> Binding<Bool> {
        Binding(get: { unfolded.pids.contains(pid) },
                set: { if $0 { unfolded.pids.insert(pid) } else { unfolded.pids.remove(pid) } })
    }

    @ViewBuilder private var instances: some View {
        if p.instances.isEmpty && p.observation.orphans.isEmpty {
            Text("No instance running").foregroundStyle(.secondary)
        }
        ForEach(p.instances) { i in
            DisclosureGroup(isExpanded: unfolding(i.pid)) {
                // No rules between the lines: the indent, to the title's text, says
                // whose they are.
                Group {
                    ForEach(Array(instanceFacts(i).enumerated()), id: \.offset) { _, f in
                        LabeledContent(f.name) {
                            if f.path.isEmpty {
                                Text(f.value).monospacedDigit().multilineTextAlignment(.trailing)
                            } else {
                                Text(f.value)
                                    .font(.callout.monospaced())
                                    .lineLimit(1)
                                    .truncationMode(.middle)
                                    .help(f.path)
                            }
                        }
                    }
                    instanceButtons(i).padding(.top, 2)
                }
                .padding(.leading, 12)
            } label: {
                Text(i.title).monospacedDigit()
            }
        }
        ForEach(p.observation.orphans, id: \.helper.pid) { o in
            DisclosureGroup(isExpanded: unfolding(o.helper.pid)) {
                Group {
                    LabeledContent("Process") { Text("PID \(String(o.helper.pid)) · \(o.helper.elapsed)").monospacedDigit() }
                    LabeledContent("Command") {
                        Text(o.helper.command)
                            .font(.callout.monospaced())
                            .lineLimit(1)
                            .truncationMode(.middle)
                            .help(o.helper.command)
                    }
                    HStack {
                        Spacer()
                        Button("Stop It…") { p.stopHelper(o.helper) }
                    }
                }
                .padding(.leading, 12)
            } label: {
                Text(orphanTitle(o)).monospacedDigit()
            }
        }
    }

    @ViewBuilder private func instanceButtons(_ i: Instance) -> some View {
        // By meaning: its addresses, then its files; stopping stands apart, on the first row.
        VStack(alignment: .leading) {
            HStack(alignment: .firstTextBaseline) {
                browse(i)
                if i.production && p.cfg.tailscale_url != nil { remoteAddress }
                Spacer(minLength: 8)
                if !i.production { Button("Stop Instance…") { p.stopInstance(i) } }
            }
            HStack(alignment: .firstTextBaseline) { files(i) }
        }
    }

    @ViewBuilder private func browse(_ i: Instance) -> some View {
        Button("Open in Browser") { if let u = i.url { p.open(u) } }.disabled(i.url == nil || !i.answers)
        Button("Copy Address") { if let u = i.url { p.copy(u) } }.disabled(i.url == nil)
    }

    @ViewBuilder private func files(_ i: Instance) -> some View {
        Button("Show Log") { p.openFile(i.log) }.disabled(i.log.isEmpty || !i.log.hasPrefix("/"))
        Button("Show Config") { p.openFile(i.configPath) }
    }

    private var remoteAddress: some View {
        Button("Copy Remote Address") { p.copyRemoteAddress() }
    }
}

/// On top of the content, outside the sections: the sign in the state of the
/// menu bar's icon, the menu's first line as a title, its cause in full, what
/// can be done about it now, and production's version.
struct DashboardHeader: View {
    @ObservedObject var p: Production

    private var cause: String { p.header.dropFirst().joined(separator: " ") }

    var body: some View {
        // Alone, the title sits beside the middle of the sign; with its lines, on top.
        HStack(alignment: cause.isEmpty && p.build == nil && (p.note.isEmpty || p.busy) ? .center : .top, spacing: 14) {
            Image(nsImage: signImage(p.sign, size: 48))
                .foregroundStyle(.primary)
                .accessibilityLabel(p.signLabel)
            VStack(alignment: .leading, spacing: 4) {
                Text(p.header.first ?? "")
                    .font(.title2.weight(.semibold))
                    .foregroundStyle(.primary)
                    .monospacedDigit()
                Group {
                    if !cause.isEmpty { Text(cause).monospacedDigit() }
                    if !p.note.isEmpty && !p.busy { Text(p.note) }
                }
                .font(.body)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
                actions
                if let b = p.build { version(b) }
            }
            Spacer(minLength: 0)
        }
        .padding(.bottom, 4)
    }

    @ViewBuilder private var actions: some View {
        let noNetwork: Bool = {
            if case .didNotStart(_, let cause, _) = p.down { return cause.hasPrefix(carrierName + " has no local network") }
            return false
        }()
        let failed = p.sign == .down && p.down != nil
        if p.offerRestartFromClairance || failed || noNetwork {
            HStack {
                if p.offerRestartFromClairance { Button("Restart from Clairance") { p.restart() }.disabled(p.busy) }
                if failed {
                    Button("Start production") { Task { await p.start() } }
                    Button("Leave stopped") { p.leaveStopped() }
                }
                if noNetwork {
                    Button("Open Local Network Settings…") { if let u = URL(string: localNetworkSettings) { NSWorkspace.shared.open(u) } }
                }
            }
            .font(.body)
            .padding(.vertical, 2)
        }
    }

    /// "Version of 20 November 2026 · 9a4b5ce · up to date": the commit in
    /// fixed width, and selectable.
    private func version(_ b: BuildInfo) -> some View {
        let what = "Version" + (b.version.isEmpty ? "" : " " + b.version) + (b.day.isEmpty ? "" : " of " + b.day) + " · "
        return (Text(what) + Text(b.commit).font(.body.monospaced()) + Text(b.behind.isEmpty ? "" : " · " + b.behind))
            .font(.body)
            .foregroundStyle(.secondary)
            .monospacedDigit()
            .textSelection(.enabled)
    }
}

// MARK: - The window and its toolbar

/// The window, opened from the menu (Show Dashboard, ⌘0), from the Dock icon,
/// and when the owner opens the app. Closing it quits nothing.
@MainActor
final class DashboardWindow: NSObject, NSToolbarDelegate, NSMenuItemValidation {
    static let shared = DashboardWindow()
    private(set) var window: NSWindow?
    private var watching: AnyCancellable?
    private var buttons: [NSToolbarItem.Identifier: NSButton] = [:]
    private let unfolded = Unfolded()

    private static let start = NSToolbarItem.Identifier("start")
    private static let stop = NSToolbarItem.Identifier("stop")
    private static let restart = NSToolbarItem.Identifier("restart")
    private static let page = NSToolbarItem.Identifier("page")

    func show() {
        guard make() else { return }
        NSApplication.shared.activate(ignoringOtherApps: true)
        window?.makeKeyAndOrderFront(nil)
    }

    /// The window as it is drawn now, in PNG, shown or not, in the given look
    /// ("light", "dark") and size or its own, its instances unfolded if asked:
    /// `clairance --snapshot`, for a review of the real window without a screen
    /// recording.
    func snapshot(look: String?, size: [Double]?, unfold: Bool) async -> Data? {
        guard make(), let w = window, let frame = w.contentView?.superview, let p = Production.shared else { return nil }
        let before = (look: w.appearance, frame: w.frame, unfolded: unfolded.pids)
        if let look { w.appearance = NSAppearance(named: look == "dark" ? .darkAqua : .aqua) }
        if let size, size.count == 2 { w.setContentSize(NSSize(width: size[0], height: size[1])) }
        if unfold { unfolded.pids = Set(p.instances.map(\.pid) + p.observation.orphans.map(\.helper.pid)) }
        defer {
            w.appearance = before.look
            w.setFrame(before.frame, display: false)
            unfolded.pids = before.unfolded
        }
        // The view takes its new state at the next turn of the run loop.
        try? await Task.sleep(nanoseconds: 300_000_000)
        sync()
        frame.layoutSubtreeIfNeeded()
        frame.displayIfNeeded()
        guard let rep = frame.bitmapImageRepForCachingDisplay(in: frame.bounds) else { return nil }
        frame.cacheDisplay(in: frame.bounds, to: rep)
        return rep.representation(using: .png, properties: [:])
    }

    private func make() -> Bool {
        guard let p = Production.shared else { return false }
        if window == nil {
            let host = NSHostingController(rootView: Dashboard(p: p, unfolded: unfolded))
            host.sizingOptions = []   // the window keeps its own size, and the frame macOS remembers
            let w = NSWindow(contentViewController: host)
            w.title = "Dashboard"
            w.styleMask = [.titled, .closable, .miniaturizable, .resizable]
            w.isReleasedWhenClosed = false
            // 600 wide: at the design's 560 the toolbar's third button did not fit
            // beside the title, and went into the toolbar's own menu (measured, 20/11).
            w.setContentSize(NSSize(width: 600, height: 680))
            w.contentMinSize = NSSize(width: 480, height: 420)
            w.center()
            w.setFrameAutosaveName("Dashboard")
            let bar = NSToolbar(identifier: "dashboard")
            bar.delegate = self
            bar.displayMode = .iconOnly   // each button carries its own title
            bar.allowsUserCustomization = false
            w.toolbar = bar
            w.toolbarStyle = .unified
            window = w
            // The toolbar follows production: after each change is in.
            watching = p.objectWillChange.receive(on: RunLoop.main).sink { [weak self] _ in self?.sync() }
            sync()
        }
        return true
    }

    /// Stopped: Start. Running: Stop and Restart. While it switches: the same,
    /// disabled. Always: Open Clairance, disabled when the server does not answer.
    private func sync() {
        guard let p = Production.shared, let bar = window?.toolbar else { return }
        let wanted: [NSToolbarItem.Identifier] = (p.running ? [Self.stop, Self.restart] : [Self.start]) + [Self.page]
        if bar.items.map(\.itemIdentifier) != wanted {
            while !bar.items.isEmpty { bar.removeItem(at: 0) }
            for (n, id) in wanted.enumerated() { bar.insertItem(withItemIdentifier: id, at: n) }
        }
        for id in [Self.start, Self.stop, Self.restart] { buttons[id]?.isEnabled = !p.busy }
        buttons[Self.page]?.isEnabled = p.production?.answers ?? false
    }

    func toolbarDefaultItemIdentifiers(_ toolbar: NSToolbar) -> [NSToolbarItem.Identifier] {
        [Self.start, Self.page]
    }

    func toolbarAllowedItemIdentifiers(_ toolbar: NSToolbar) -> [NSToolbarItem.Identifier] {
        [Self.start, Self.stop, Self.restart, Self.page]
    }

    func toolbar(_ toolbar: NSToolbar, itemForItemIdentifier id: NSToolbarItem.Identifier,
                 willBeInsertedIntoToolbar flag: Bool) -> NSToolbarItem? {
        let (title, symbol, action): (String, String, Selector)
        switch id {
        case Self.start: (title, symbol, action) = ("Start", "play.fill", #selector(start))
        case Self.stop: (title, symbol, action) = ("Stop", "stop.fill", #selector(stop))
        case Self.restart: (title, symbol, action) = ("Restart", "arrow.clockwise", #selector(restart))
        case Self.page: (title, symbol, action) = ("Open Clairance", "arrow.up.forward.app", #selector(openPage))
        default: return nil
        }
        let image = NSImage(systemSymbolName: symbol, accessibilityDescription: nil)?
            .withSymbolConfiguration(NSImage.SymbolConfiguration(pointSize: 11, weight: .medium)) ?? NSImage()
        let b = NSButton(title: title, image: image, target: self, action: action)
        if #available(macOS 14.0, *) { b.bezelStyle = .toolbar } else { b.bezelStyle = .texturedRounded }
        b.imagePosition = .imageLeading
        let item = NSToolbarItem(itemIdentifier: id)
        item.label = title
        item.toolTip = id == Self.page ? "Open Clairance's page in the browser" : "\(title) production"
        item.view = b
        // In a window too narrow for them all, the toolbar's own menu takes over:
        // the rarest action first, the everyday one last.
        item.visibilityPriority = id == Self.page ? .high : id == Self.restart ? .low : .standard
        let inMenu = NSMenuItem(title: title, action: action, keyEquivalent: "")
        inMenu.target = self
        item.menuFormRepresentation = inMenu
        buttons[id] = b
        return item
    }

    func validateMenuItem(_ item: NSMenuItem) -> Bool {
        guard let p = Production.shared else { return false }
        return item.action == #selector(openPage) ? p.production?.answers ?? false : !p.busy
    }

    @objc private func start() { Task { await Production.shared?.start() } }
    @objc private func stop() { Production.shared?.askStop() }
    @objc private func restart() { Production.shared?.restart() }
    @objc private func openPage() {
        guard let p = Production.shared, let u = p.production?.url else { return }
        p.open(u)
    }
}
