import AppKit
import SwiftUI

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationWillFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.regular)
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.regular)
        NSApp.activate(ignoringOtherApps: true)
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        true
    }
}

@main
struct BrainwashApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @State private var model = AppModel()

    var body: some Scene {
        WindowGroup(L10n.t("app.name")) {
            ContentView()
                .environment(model)
                .frame(minWidth: 980, minHeight: 640)
        }
        .windowStyle(.automatic)
        .defaultSize(width: 1180, height: 740)
        .commands {
            CommandGroup(replacing: .appInfo) {
                Button(L10n.t("menu.about")) {
                    NSApp.orderFrontStandardAboutPanel(options: [
                        .applicationName: L10n.t("app.name"),
                        .applicationVersion: "1.0",
                    ])
                }
            }
            CommandGroup(replacing: .newItem) { }
            CommandGroup(replacing: .appSettings) {
                Button(L10n.t("menu.settings")) {
                    model.showSettings = true
                }
                .keyboardShortcut(",", modifiers: [.command])
            }
            CommandMenu(L10n.t("menu.memory")) {
                Button(L10n.t("menu.import")) {
                    model.showImport = true
                }
                .keyboardShortcut("i", modifiers: [.command])
                Button(L10n.t("menu.export")) {
                    model.exportPackedInteractive()
                }
                .keyboardShortcut("e", modifiers: [.command])
                .disabled(model.selected == nil || model.busy)
                Divider()
                Button(L10n.t("menu.refresh")) {
                    model.refresh()
                }
                .keyboardShortcut("r", modifiers: [.command])
                .disabled(model.listing)
                Button(L10n.t("menu.brainwash")) {
                    model.cloneSelected()
                }
                .keyboardShortcut("b", modifiers: [.command])
                .disabled(model.selected == nil || model.busy)
            }
        }
    }
}
