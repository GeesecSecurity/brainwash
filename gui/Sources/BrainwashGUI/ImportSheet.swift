import AppKit
import SwiftUI
import UniformTypeIdentifiers

struct ImportSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss

    @State private var sourceSlot: String = "pi"
    @State private var customDir: String = ""
    @State private var useCustomDir: Bool = false
    @State private var sessions: [SessionRef] = []
    @State private var selectedCWD: String = ""
    @State private var packedFiles: [String] = []
    @State private var targetSlot: String = ""
    @State private var busy: Bool = false
    @State private var error: String?
    @State private var listing: Bool = false

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack {
                Text(L10n.t("import.title")).font(.title3.weight(.semibold))
                Spacer()
                Button(L10n.t("import.close")) { dismiss() }
                    .buttonStyle(.plain)
                    .foregroundStyle(.secondary)
            }
            .padding(16)
            Divider().overlay(Palette.line)
            HStack(alignment: .top, spacing: 0) {
                sourcePane
                    .frame(minWidth: 280, maxWidth: .infinity)
                Divider().overlay(Palette.line)
                dropPane
                    .frame(minWidth: 280, maxWidth: .infinity)
            }
            .frame(minHeight: 360)
            Divider().overlay(Palette.line)
            actions
        }
        .frame(width: 780, height: 520)
        .background(Palette.bg)
        .onAppear {
            sourceSlot = model.filterSlot
            refreshSessions()
        }
    }

    var sourcePane: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(L10n.t("import.sessionRoot"))
                .font(.system(size: 10, weight: .semibold, design: .rounded))
                .foregroundStyle(.secondary)
            Picker(L10n.t("agent"), selection: $sourceSlot) {
                ForEach(model.slots) { s in Text(s.name).tag(s.name) }
            }
            .pickerStyle(.segmented)
            .onChange(of: sourceSlot) { _, _ in
                useCustomDir = false
                customDir = ""
                selectedCWD = ""
                refreshSessions()
            }
            Button {
                if let dir = pickDirectory() {
                    useCustomDir = true
                    customDir = dir
                    selectedCWD = dir
                    refreshSessions()
                }
            } label: {
                HStack {
                    Image(systemName: "folder")
                    Text(useCustomDir ? customDir : L10n.t("import.chooseDir"))
                        .lineLimit(1)
                    Spacer()
                }
                .padding(8)
                .background(Palette.field)
                .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
            }
            .buttonStyle(.plain)
            if listing {
                ProgressView().padding(.top, 8)
            }
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 6) {
                    ForEach(uniqueCWDs, id: \.self) { cwd in
                        let on = selectedCWD == cwd
                        HStack {
                            Text(cwd.isEmpty ? L10n.t("import.unknownCwd") : cwd)
                                .font(.caption.monospaced())
                                .lineLimit(2)
                            Spacer()
                        }
                        .padding(8)
                        .background(on ? Palette.selected : Palette.row)
                        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                        .onTapGesture { selectedCWD = cwd }
                    }
                }
            }
            if let error {
                Text(error).font(.caption).foregroundStyle(Palette.danger).lineLimit(3)
            }
        }
        .padding(16)
    }

    var uniqueCWDs: [String] {
        var seen = Set<String>()
        var out: [String] = []
        for s in sessions {
            if seen.insert(s.cwd).inserted {
                out.append(s.cwd)
            }
        }
        return out
    }

    var dropPane: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(L10n.t("import.packed"))
                .font(.system(size: 10, weight: .semibold, design: .rounded))
                .foregroundStyle(.secondary)
            ZStack {
                RoundedRectangle(cornerRadius: 12, style: .continuous)
                    .strokeBorder(style: StrokeStyle(lineWidth: 1, dash: [5, 4]))
                    .foregroundStyle(Palette.line)
                    .background(Palette.field.clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous)))
                VStack(spacing: 8) {
                    Image(systemName: "square.and.arrow.down")
                        .font(.title2)
                        .foregroundStyle(.secondary)
                    Text(L10n.t("import.drop"))
                        .font(.callout.weight(.medium))
                    Text(L10n.t("import.orClick"))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            .frame(height: 140)
            .contentShape(Rectangle())
            .onTapGesture { addPackedFiles(pickPackedFiles()) }
            .onDrop(of: [.fileURL], isTargeted: nil) { providers in
                handleDrop(providers)
            }
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 6) {
                    ForEach(packedFiles, id: \.self) { p in
                        HStack {
                            Text((p as NSString).lastPathComponent)
                                .font(.caption)
                                .lineLimit(1)
                            Spacer()
                            Button {
                                packedFiles.removeAll { $0 == p }
                            } label: {
                                Image(systemName: "xmark.circle.fill").foregroundStyle(.secondary)
                            }
                            .buttonStyle(.plain)
                        }
                        .padding(8)
                        .background(Palette.row)
                        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                    }
                }
            }
        }
        .padding(16)
    }

    var actions: some View {
        HStack(spacing: 10) {
            Spacer()
            Picker("", selection: $targetSlot) {
                Text(L10n.t("import.keepAgent")).tag("")
                ForEach(model.slots) { s in Text(s.label).tag(s.name) }
            }
            .frame(width: 180)
            Button(L10n.t("import.cancel")) { dismiss() }
                .buttonStyle(FlatButton(fill: Palette.chip, fg: .primary))
                .frame(width: 88)
            Button {
                confirm()
            } label: {
                Text(busy ? L10n.t("import.busy") : L10n.t("import.action"))
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(FlatButton(fill: Palette.accent2, fg: .white))
            .frame(width: 120)
            .disabled(packedFiles.isEmpty || busy)
        }
        .padding(16)
    }

    func refreshSessions() {
        listing = true
        error = nil
        let slot = sourceSlot
        let cwd = useCustomDir ? customDir : ""
        Task.detached {
            do {
                let items = try Client.list(slot: slot, cwd: cwd)
                await MainActor.run {
                    self.sessions = items
                    self.listing = false
                    if self.selectedCWD.isEmpty, let first = items.first {
                        self.selectedCWD = first.cwd
                    }
                }
            } catch {
                await MainActor.run {
                    self.listing = false
                    self.error = error.localizedDescription
                    self.model.toastError(L10n.t("toast.listFailed"), error.localizedDescription)
                }
            }
        }
    }

    func confirm() {
        guard !packedFiles.isEmpty else { return }
        busy = true
        error = nil
        let files = packedFiles
        let to = targetSlot
        let out = selectedCWD
        let tools = model.includeTools
        Task.detached {
            do {
                let res = try Client.importPacked(files: files, to: to, outCwd: out, includeTools: tools)
                await MainActor.run {
                    self.busy = false
                    self.model.imported(res)
                    self.dismiss()
                }
            } catch {
                await MainActor.run {
                    self.busy = false
                    self.error = error.localizedDescription
                    self.model.toastError(L10n.t("toast.importFailed"), error.localizedDescription)
                }
            }
        }
    }

    func addPackedFiles(_ urls: [URL]) {
        for u in urls {
            let p = u.path
            if !packedFiles.contains(p) {
                packedFiles.append(p)
            }
        }
    }

    func handleDrop(_ providers: [NSItemProvider]) -> Bool {
        var claimed = false
        for p in providers {
            claimed = true
            p.loadItem(forTypeIdentifier: UTType.fileURL.identifier, options: nil) { item, _ in
                let url: URL? = {
                    if let data = item as? Data {
                        return URL(dataRepresentation: data, relativeTo: nil)
                    }
                    if let u = item as? URL { return u }
                    if let s = item as? String { return URL(fileURLWithPath: s) }
                    return nil
                }()
                guard let url else { return }
                DispatchQueue.main.async {
                    if url.pathExtension.lowercased() == "pm" {
                        addPackedFiles([url])
                    }
                }
            }
        }
        return claimed
    }
}

@MainActor
func pickDirectory() -> String? {
    let panel = NSOpenPanel()
    panel.canChooseFiles = false
    panel.canChooseDirectories = true
    panel.allowsMultipleSelection = false
    panel.prompt = L10n.t("panel.choose")
    return panel.runModal() == .OK ? panel.url?.path : nil
}

@MainActor
func pickPackedFiles() -> [URL] {
    let panel = NSOpenPanel()
    panel.canChooseFiles = true
    panel.canChooseDirectories = false
    panel.allowsMultipleSelection = true
    panel.allowedContentTypes = [UTType(filenameExtension: "pm") ?? .data]
    panel.prompt = L10n.t("panel.add")
    guard panel.runModal() == .OK else { return [] }
    return panel.urls
}

@MainActor
func pickSavePM(suggested: String) -> String? {
    let panel = NSSavePanel()
    panel.allowedContentTypes = [UTType(filenameExtension: "pm") ?? .data]
    panel.nameFieldStringValue = suggested
    panel.canCreateDirectories = true
    panel.title = L10n.t("panel.exportTitle")
    return panel.runModal() == .OK ? panel.url?.path : nil
}
