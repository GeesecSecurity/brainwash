import Foundation
import SwiftUI

enum AppLanguage: String, CaseIterable, Identifiable {
    case system
    case en
    case zhHans = "zh-Hans"
    case zhHant = "zh-Hant"

    var id: String { rawValue }

    var titleKey: String {
        switch self {
        case .system: return "settings.lang.system"
        case .en: return "settings.lang.en"
        case .zhHans: return "settings.lang.zhHans"
        case .zhHant: return "settings.lang.zhHant"
        }
    }
}

enum AppSettings {
    static let languageKey = "brainwash.language"
    static let cliPathKey = "brainwash.cliPath"
    static let extraArgsKey = "brainwash.cliExtraArgs"

    static var language: AppLanguage {
        get {
            let raw = UserDefaults.standard.string(forKey: languageKey) ?? AppLanguage.system.rawValue
            return AppLanguage(rawValue: raw) ?? .system
        }
        set { UserDefaults.standard.set(newValue.rawValue, forKey: languageKey) }
    }

    static var cliPath: String {
        get { UserDefaults.standard.string(forKey: cliPathKey) ?? "" }
        set { UserDefaults.standard.set(newValue, forKey: cliPathKey) }
    }

    static var extraArgs: String {
        get { UserDefaults.standard.string(forKey: extraArgsKey) ?? "" }
        set { UserDefaults.standard.set(newValue, forKey: extraArgsKey) }
    }

    static func extraArgList() -> [String] {
        extraArgs
            .split(whereSeparator: { $0.isWhitespace })
            .map(String.init)
            .filter { !$0.isEmpty }
    }
}
