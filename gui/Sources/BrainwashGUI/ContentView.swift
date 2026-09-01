import AppKit
import SwiftUI

struct ContentView: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        @Bindable var model = model
        ZStack(alignment: .topTrailing) {
            HSplitView {
                sidebar.frame(minWidth: 280, idealWidth: 320)
                detail.frame(minWidth: 420)
                inspector.frame(minWidth: 260, idealWidth: 300)
            }
            ToastStack(items: model.toasts) { id in
                model.dismissToast(id)
            }
        }
        .background(Palette.bg)
        .onAppear { model.bootstrap() }
        .sheet(isPresented: $model.showImport) {
            ImportSheet()
                .environment(model)
        }
        .sheet(isPresented: $model.showSettings) {
            SettingsView()
                .environment(model)
        }
        .id(model.localeEpoch)
    }

    var sidebar: some View {
        @Bindable var model = model
        return VStack(alignment: .leading, spacing: 12) {
            HStack(spacing: 8) {
                Circle().fill(Palette.accent).frame(width: 8, height: 8)
                Text(L10n.t("app.name")).font(.system(size: 15, weight: .semibold, design: .rounded))
                Spacer()
                Button {
                    model.showSettings = true
                } label: {
                    Image(systemName: "gearshape")
                }
                .buttonStyle(.plain)
                .help(L10n.t("sidebar.settings"))
            }
            Picker(L10n.t("agent"), selection: $model.filterSlot) {
                ForEach(model.slots) { s in Text(s.name).tag(s.name) }
            }
            .pickerStyle(.segmented)
            .onChange(of: model.filterSlot) { _, _ in
                model.refresh()
            }
            TextField(L10n.t("filter.placeholder"), text: $model.query)
                .textFieldStyle(.plain)
                .padding(8)
                .background(Palette.field)
                .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))

            Divider().overlay(Palette.line)

            ScrollView {
                LazyVStack(alignment: .leading, spacing: 6) {
                    ForEach(model.visibleSessions, id: \.identity) { s in
                        SessionRow(session: s, selected: model.selected?.identity == s.identity)
                            .onTapGesture { model.select(s) }
                    }
                }
            }
            Spacer(minLength: 0)
            Button {
                model.showImport = true
            } label: {
                Text(L10n.t("sidebar.import"))
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(FlatButton(fill: Palette.accent, fg: .white))
            Text(model.status).font(.caption).foregroundStyle(.tertiary).lineLimit(1)
        }
        .padding(14)
        .background(Palette.sidebar)
    }

    var detail: some View {
        VStack(alignment: .leading, spacing: 0) {
            if let d = model.detail {
                VStack(alignment: .leading, spacing: 6) {
                    Text(d.title).font(.title3.weight(.semibold))
                    Text("\(d.slot) · \(d.id)")
                        .font(.caption.monospaced())
                        .foregroundStyle(.secondary)
                    Text(d.sourcePath)
                        .font(.caption2.monospaced())
                        .foregroundStyle(.tertiary)
                        .lineLimit(1)
                }
                .padding(16)
                Divider().overlay(Palette.line)
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 12) {
                        ForEach(d.events) { ev in
                            ChatTurn(event: ev)
                                .frame(maxWidth: .infinity, alignment: ev.isUser ? .trailing : .leading)
                        }
                    }
                    .padding(16)
                }
            } else {
                VStack(spacing: 8) {
                    if model.loadingDetail {
                        ProgressView()
                        Text(L10n.t("empty.opening")).foregroundStyle(.secondary)
                    } else if model.listing {
                        ProgressView()
                        Text(L10n.t("empty.listing")).foregroundStyle(.secondary)
                    } else {
                        Text(L10n.t("empty.select")).font(.headline)
                        Text(L10n.t("empty.hint"))
                            .foregroundStyle(.secondary)
                    }
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .background(Palette.bg)
    }

    var inspector: some View {
        @Bindable var model = model
        return VStack(alignment: .leading, spacing: 14) {
            Text(L10n.t("inspector.title")).font(.headline)
            Text(L10n.t("inspector.from", model.selected?.slot ?? "—"))
                .font(.caption.monospaced())
                .foregroundStyle(.secondary)
            labeled(L10n.t("inspector.to")) {
                Picker("", selection: $model.toSlot) {
                    ForEach(model.slots) { s in Text(s.label).tag(s.name) }
                }
                .labelsHidden()
            }
            labeled(L10n.t("inspector.out")) {
                TextField(L10n.t("inspector.out.placeholder"), text: $model.outCwd)
                    .textFieldStyle(.plain)
                    .padding(8)
                    .background(Palette.field)
                    .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
            }
            Toggle(L10n.t("inspector.tools"), isOn: $model.includeTools)
                .toggleStyle(.switch)
            Text(L10n.t("inspector.tools.hint"))
                .font(.caption)
                .foregroundStyle(.secondary)
            Spacer()
            Button {
                model.exportPackedInteractive()
            } label: {
                Text(L10n.t("inspector.export"))
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(FlatButton(fill: Palette.chip, fg: .primary))
            .disabled(model.selected == nil || model.busy)
            Button {
                model.cloneSelected()
            } label: {
                Text(L10n.t("inspector.brainwash"))
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(FlatButton(fill: Palette.accent2, fg: .white))
            .disabled(model.selected == nil || model.busy)
        }
        .padding(16)
        .background(Palette.sidebar)
    }

}

struct SessionRow: View {
    let session: SessionRef
    let selected: Bool
    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Text(session.slot.uppercased())
                    .font(.system(size: 10, weight: .semibold, design: .rounded))
                    .padding(.horizontal, 6).padding(.vertical, 2)
                    .background(Palette.chip)
                    .clipShape(Capsule())
                Spacer()
                Text(session.updatedAt.formatted(date: .abbreviated, time: .shortened))
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
            Text(session.title.isEmpty ? session.id : session.title)
                .font(.system(size: 13, weight: .medium))
                .lineLimit(2)
            Text(session.cwd.isEmpty ? session.id : session.cwd)
                .font(.caption2.monospaced())
                .foregroundStyle(.tertiary)
                .lineLimit(1)
        }
        .padding(10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(selected ? Palette.selected : Palette.row)
        .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
    }
}

struct ChatTurn: View {
    let event: Event
    var body: some View {
        if event.isFoldedMeta {
            HStack {
                FoldRow(title: event.text.isEmpty ? (event.kind ?? event.role) : event.text, detail: event.injects?.first?.text)
                Spacer(minLength: 0)
            }
        } else {
            aligned {
                VStack(alignment: event.isUser ? .trailing : .leading, spacing: 8) {
                    if event.interrupted == true {
                        Text(L10n.t("chat.interrupted"))
                            .font(.system(size: 10, weight: .semibold, design: .rounded))
                            .foregroundStyle(.orange)
                    }
                    if let thinking = event.thinking, !thinking.isEmpty {
                        FoldRow(title: L10n.t("chat.thinking"), detail: thinking, compact: true)
                    }
                    if !event.text.isEmpty {
                        BubbleFit {
                            Text(event.text)
                                .font(.body)
                                .foregroundStyle(event.isUser ? Color.white : Palette.agentText)
                                .textSelection(.enabled)
                                .padding(.horizontal, 12)
                                .padding(.vertical, 8)
                                .background(event.isUser ? Palette.userBubble : Palette.agentBubble)
                                .clipShape(RoundedRectangle(cornerRadius: 16, style: .continuous))
                        }
                    }
                    ForEach(Array((event.images ?? []).enumerated()), id: \.offset) { _, url in
                        ChatImage(url: url)
                    }
                    ForEach(Array((event.injects ?? []).enumerated()), id: \.offset) { _, inj in
                        FoldRow(title: inj.summary, detail: inj.text, compact: true)
                    }
                    ForEach(Array((event.tools ?? []).enumerated()), id: \.offset) { _, t in
                        FoldRow(
                            title: (t.isError == true ? "⚠ " : "⌘ ") + t.name,
                            detail: toolDetail(t),
                            compact: true
                        )
                    }
                    Text(event.timestamp.formatted(date: .omitted, time: .shortened))
                        .font(.caption2)
                        .foregroundStyle(.tertiary)
                }
            }
        }
    }

    @ViewBuilder
    func aligned<Content: View>(@ViewBuilder _ content: () -> Content) -> some View {
        HStack(alignment: .top, spacing: 0) {
            if event.isUser { Spacer(minLength: 96) }
            content()
            if !event.isUser { Spacer(minLength: 96) }
        }
    }

    func toolDetail(_ t: ToolTrace) -> String {
        var parts: [String] = []
        if let a = t.arguments, !a.isEmpty { parts.append(L10n.t("chat.args") + "\n" + a) }
        if let r = t.result, !r.isEmpty { parts.append(L10n.t("chat.result") + "\n" + r) }
        return parts.joined(separator: "\n\n")
    }
}

struct BubbleFit: Layout {
    var cap: CGFloat = 420

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        guard let child = subviews.first else { return .zero }
        let maxW = min(cap, proposal.width ?? cap)
        let ideal = child.sizeThatFits(.unspecified)
        if ideal.width <= maxW {
            return ideal
        }
        return child.sizeThatFits(.init(width: maxW, height: nil))
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        guard let child = subviews.first else { return }
        child.place(at: bounds.origin, proposal: ProposedViewSize(width: bounds.width, height: bounds.height))
    }
}

struct FoldRow: View {
    let title: String
    var detail: String?
    var compact: Bool = false
    @State private var open = false
    var body: some View {
        DisclosureGroup(isExpanded: $open) {
            if let detail, !detail.isEmpty {
                Text(detail)
                    .font(.caption.monospaced())
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
        } label: {
            Text(title)
                .font(compact ? .caption.weight(.medium) : .callout.weight(.medium))
                .foregroundStyle(.secondary)
                .lineLimit(1)
        }
        .padding(compact ? 6 : 8)
        .background(Palette.chip)
        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
    }
}

struct ChatImage: View {
    let url: String
    var body: some View {
        if let img = loadImage(url) {
            Image(nsImage: img)
                .resizable()
                .scaledToFit()
                .frame(maxWidth: 280, maxHeight: 180)
                .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
        }
    }
}

func loadImage(_ s: String) -> NSImage? {
    if s.hasPrefix("data:image") { return decodeDataURL(s) }
    if s.hasPrefix("/") { return NSImage(contentsOfFile: s) }
    if s.hasPrefix("file://"), let u = URL(string: s) {
        return NSImage(contentsOf: u)
    }
    if let u = URL(string: s), !u.isFileURL {
        return NSImage(contentsOf: u)
    }
    return NSImage(contentsOfFile: s)
}

func decodeDataURL(_ s: String) -> NSImage? {
    guard let comma = s.firstIndex(of: ",") else { return nil }
    let b64 = String(s[s.index(after: comma)...])
    guard let data = Data(base64Encoded: b64) else { return nil }
    return NSImage(data: data)
}

func labeled<Content: View>(_ title: String, @ViewBuilder _ content: () -> Content) -> some View {
    VStack(alignment: .leading, spacing: 6) {
        Text(title.uppercased())
            .font(.system(size: 10, weight: .semibold, design: .rounded))
            .foregroundStyle(.secondary)
        content()
    }
}

struct FlatButton: ButtonStyle {
    var fill: Color
    var fg: Color
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: 13, weight: .semibold))
            .padding(.vertical, 8)
            .background(fill.opacity(configuration.isPressed ? 0.8 : 1))
            .foregroundStyle(fg)
            .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
    }
}

enum Palette {
    static let bg = Color(nsColor: .windowBackgroundColor)
    static let sidebar = Color(nsColor: .underPageBackgroundColor)
    static let row = Color.primary.opacity(0.04)
    static let field = Color.primary.opacity(0.05)
    static let chip = Color.primary.opacity(0.06)
    static let selected = Color.accentColor.opacity(0.16)
    static let line = Color.primary.opacity(0.08)
    static let accent = Color(red: 0.13, green: 0.55, blue: 0.83)
    static let accent2 = Color(red: 0.23, green: 0.72, blue: 0.51)
    static let danger = Color(red: 0.86, green: 0.28, blue: 0.33)
    static let warning = Color(red: 0.93, green: 0.62, blue: 0.16)
    static let userBubble = Color(red: 0.13, green: 0.55, blue: 0.83)
    static let agentBubble = Color.primary.opacity(0.06)
    static let agentText = Color.primary
}
