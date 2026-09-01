import AppKit
import SwiftUI
import UniformTypeIdentifiers

struct SettingsView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @State private var language: AppLanguage = AppSettings.language
    @State private var cliPath: String = AppSettings.cliPath
    @State private var extraArgs: String = AppSettings.extraArgs
    @State private var resolved: String = ""

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack {
                Text(L10n.t("settings.title")).font(.title3.weight(.semibold))
                Spacer()
                Button(L10n.t("settings.done")) { dismiss() }
                    .buttonStyle(.plain)
                    .foregroundStyle(.secondary)
            }
            .padding(16)
            Divider().overlay(Palette.line)
            ScrollView {
                VStack(alignment: .leading, spacing: 22) {
                    languageSection
                    cliSection
                }
                .padding(16)
            }
        }
        .frame(width: 520, height: 420)
        .background(Palette.bg)
        .onAppear { refreshResolved() }
    }

    var languageSection: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(L10n.t("settings.language"))
                .font(.headline)
            Picker("", selection: $language) {
                ForEach(AppLanguage.allCases) { lang in
                    Text(L10n.t(lang.titleKey)).tag(lang)
                }
            }
            .pickerStyle(.radioGroup)
            .onChange(of: language) { _, new in
                AppSettings.language = new
                model.localeChanged()
            }
        }
    }

    var cliSection: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(L10n.t("settings.cli")).font(.headline)
            Text(L10n.t("settings.cli.hint"))
                .font(.caption)
                .foregroundStyle(.secondary)
            labeled(L10n.t("settings.cli.path")) {
                HStack(spacing: 8) {
                    TextField(L10n.t("settings.cli.path.placeholder"), text: $cliPath)
                        .textFieldStyle(.plain)
                        .padding(8)
                        .background(Palette.field)
                        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                        .onChange(of: cliPath) { _, new in
                            AppSettings.cliPath = new
                            Helper.resetCache()
                            refreshResolved()
                        }
                    Button(L10n.t("settings.cli.browse")) { browseCLI() }
                        .buttonStyle(FlatButton(fill: Palette.chip, fg: .primary))
                        .frame(width: 88)
                }
            }
            HStack(spacing: 8) {
                Button(L10n.t("settings.cli.reset")) {
                    cliPath = ""
                    AppSettings.cliPath = ""
                    Helper.resetCache()
                    refreshResolved()
                }
                .buttonStyle(FlatButton(fill: Palette.chip, fg: .primary))
                Spacer()
            }
            labeled(L10n.t("settings.cli.args")) {
                TextField(L10n.t("settings.cli.args.placeholder"), text: $extraArgs)
                    .textFieldStyle(.plain)
                    .padding(8)
                    .background(Palette.field)
                    .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                    .onChange(of: extraArgs) { _, new in
                        AppSettings.extraArgs = new
                    }
            }
            labeled(L10n.t("settings.cli.resolved")) {
                Text(resolved.isEmpty ? L10n.t("settings.cli.missing") : resolved)
                    .font(.caption.monospaced())
                    .foregroundStyle(resolved.isEmpty ? Palette.danger : .secondary)
                    .textSelection(.enabled)
            }
        }
    }

    func browseCLI() {
        let panel = NSOpenPanel()
        panel.canChooseFiles = true
        panel.canChooseDirectories = false
        panel.allowsMultipleSelection = false
        panel.prompt = L10n.t("panel.chooseCLI")
        if panel.runModal() == .OK, let url = panel.url {
            cliPath = url.path
            AppSettings.cliPath = url.path
            Helper.resetCache()
            refreshResolved()
        }
    }

    func refreshResolved() {
        resolved = (try? Helper.binary().path) ?? ""
    }
}
