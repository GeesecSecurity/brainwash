import Foundation
import Observation
import SwiftUI

@MainActor
@Observable
final class AppModel {
    var slots: [SlotInfo] = [
        .init(name: "pi", label: "pi"),
        .init(name: "codex", label: "Codex"),
        .init(name: "claude", label: "Claude Code"),
        .init(name: "dsh", label: "DeepSeek Harness"),
    ]
    var filterSlot: String = "pi"
    var toSlot: String = "codex"
    var query: String = ""
    var sessions: [SessionRef] = []
    var selected: SessionRef?
    var detail: NeutralSession?
    var outCwd: String = ""
    var includeTools: Bool = true
    var status: String = L10n.t("status.starting")
    var listing: Bool = false
    var loadingDetail: Bool = false
    var showImport: Bool = false
    var showSettings: Bool = false
    var localeEpoch: Int = 0
    var toasts: [ToastItem] = []
    private var listGen: Int = 0
    private var showGen: Int = 0
    private var toastTasks: [UUID: Task<Void, Never>] = [:]

    var busy: Bool { listing || loadingDetail }

    func localeChanged() {
        localeEpoch += 1
        status = L10n.t("status.ready")
    }

    func toast(_ kind: ToastKind, _ title: String, _ message: String = "", linger: TimeInterval = 4.5) {
        let item = ToastItem(kind: kind, title: title, message: message)
        withAnimation(.spring(duration: 0.28)) {
            toasts.append(item)
            if toasts.count > 4 { toasts.removeFirst(toasts.count - 4) }
        }
        toastTasks[item.id]?.cancel()
        toastTasks[item.id] = Task { @MainActor in
            try? await Task.sleep(for: .seconds(linger))
            guard !Task.isCancelled else { return }
            dismissToast(item.id)
        }
    }

    func dismissToast(_ id: UUID) {
        toastTasks[id]?.cancel()
        toastTasks[id] = nil
        withAnimation(.easeOut(duration: 0.18)) {
            toasts.removeAll { $0.id == id }
        }
    }

    func toastSuccess(_ title: String, _ message: String = "") {
        toast(.success, title, message)
    }

    func toastWarning(_ title: String, _ message: String = "") {
        toast(.warning, title, message, linger: 6)
    }

    func toastError(_ title: String, _ message: String = "") {
        toast(.error, title, message, linger: 8)
    }

    var visibleSessions: [SessionRef] {
        let q = query.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        guard !q.isEmpty else { return sessions }
        return sessions.filter {
            $0.title.lowercased().contains(q)
                || $0.id.lowercased().contains(q)
                || $0.cwd.lowercased().contains(q)
        }
    }

    func bootstrap() {
        listing = true
        Task.detached {
            do {
                _ = try Helper.binary()
                let slots = (try? Client.slots()) ?? []
                await MainActor.run {
                    if !slots.isEmpty { self.slots = slots }
                    self.status = L10n.t("status.ready")
                    self.refresh()
                }
            } catch {
                await MainActor.run {
                    self.listing = false
                    self.status = L10n.t("status.helperMissing")
                    self.toastError(L10n.t("toast.helperMissing"), error.localizedDescription)
                }
            }
        }
    }

    func refresh() {
        listGen += 1
        let gen = listGen
        listing = true
        selected = nil
        detail = nil
        status = L10n.t("status.listing", filterSlot)
        let slot = filterSlot
        Task.detached {
            do {
                let items = try Client.list(slot: slot)
                await MainActor.run {
                    guard gen == self.listGen else { return }
                    self.sessions = items
                    self.listing = false
                    let key = items.count == 1 ? "status.sessions" : "status.sessions.plural"
                    self.status = L10n.t(key, items.count, slot)
                }
            } catch {
                await MainActor.run {
                    guard gen == self.listGen else { return }
                    self.listing = false
                    self.status = L10n.t("status.listFailed")
                    self.toastError(L10n.t("toast.listFailed"), error.localizedDescription)
                }
            }
        }
    }

    func select(_ session: SessionRef) {
        selected = session
        if outCwd.isEmpty { outCwd = session.cwd }
        loadSelected()
    }

    func loadSelected() {
        guard let selected else {
            detail = nil
            return
        }
        showGen += 1
        let gen = showGen
        loadingDetail = true
        status = L10n.t("status.opening")
        let slot = selected.slot
        let id = selected.id
        let path = selected.path
        Task.detached {
            do {
                let sess = try Client.show(slot: slot, session: id, path: path)
                await MainActor.run {
                    guard gen == self.showGen else { return }
                    self.detail = sess
                    self.loadingDetail = false
                    self.status = L10n.t("status.events", sess.events.count)
                    if self.outCwd.isEmpty { self.outCwd = sess.cwd }
                }
            } catch {
                await MainActor.run {
                    guard gen == self.showGen else { return }
                    self.loadingDetail = false
                    self.detail = nil
                    self.status = L10n.t("status.openFailed")
                    self.toastError(L10n.t("toast.openFailed"), error.localizedDescription)
                }
            }
        }
    }

    func cloneSelected() {
        guard let selected else { return }
        loadingDetail = true
        status = L10n.t("status.brainwashing")
        let to = toSlot
        let out = outCwd.isEmpty ? selected.cwd : outCwd
        let tools = includeTools
        let from = selected.slot
        let id = selected.id
        let path = selected.path
        Task.detached {
            do {
                let res = try Client.clone(from: from, to: to, session: id, path: path, outCwd: out, includeTools: tools)
                await MainActor.run {
                    self.loadingDetail = false
                    if let first = res.first {
                        self.status = L10n.t("status.wrote", first.destPath)
                        self.toastSuccess(L10n.t("toast.cloneOk"), first.destPath)
                    } else {
                        self.status = L10n.t("status.cloneFinished")
                        self.toastWarning(L10n.t("toast.cloneEmpty"), L10n.t("toast.cloneEmpty.body"))
                    }
                }
            } catch {
                await MainActor.run {
                    self.loadingDetail = false
                    self.status = L10n.t("status.cloneFailed")
                    self.toastError(L10n.t("toast.cloneFailed"), error.localizedDescription)
                }
            }
        }
    }

    func exportPackedInteractive() {
        guard selected != nil else {
            toastWarning(L10n.t("toast.nothingSelected"), L10n.t("toast.nothingSelected.body"))
            return
        }
        let suggested = UUID().uuidString.lowercased() + ".pm"
        if let dest = pickSavePM(suggested: suggested) {
            exportSelected(to: dest)
        }
    }

    func exportSelected(to dest: String) {
        guard let selected else { return }
        loadingDetail = true
        status = L10n.t("status.exporting")
        let slot = selected.slot
        let id = selected.id
        let path = selected.path
        let tools = includeTools
        Task.detached {
            do {
                let res = try Client.exportPacked(slot: slot, session: id, path: path, out: dest, includeTools: tools)
                await MainActor.run {
                    self.loadingDetail = false
                    self.status = L10n.t("status.exported", res.path)
                    self.toastSuccess(L10n.t("toast.exportOk"), res.path)
                }
            } catch {
                await MainActor.run {
                    self.loadingDetail = false
                    self.status = L10n.t("status.exportFailed")
                    self.toastError(L10n.t("toast.exportFailed"), error.localizedDescription)
                }
            }
        }
    }

    func imported(_ results: [ImportResult]) {
        guard let last = results.last else {
            status = L10n.t("status.importFinished")
            toastWarning(L10n.t("toast.importEmpty"), L10n.t("toast.importEmpty.body"))
            return
        }
        filterSlot = last.destSlot.isEmpty ? filterSlot : last.destSlot
        let key = results.count == 1 ? "status.imported" : "status.imported.plural"
        let toastKey = results.count == 1 ? "toast.importOk" : "toast.importOk.plural"
        status = L10n.t(key, results.count)
        toastSuccess(L10n.t(toastKey, results.count), last.destPath)
        refresh()
    }
}
