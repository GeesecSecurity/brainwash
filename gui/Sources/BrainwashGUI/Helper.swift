import Foundation

enum Helper {
    static let binaryName = "brainwash-cli"
    private static let lock = NSLock()
    nonisolated(unsafe) private static var cached: URL?

    static func resetCache() {
        lock.lock()
        cached = nil
        lock.unlock()
    }

    static func binary() throws -> URL {
        lock.lock()
        defer { lock.unlock() }
        if let cached { return cached }
        if let custom = UserDefaults.standard.string(forKey: "brainwash.cliPath"), !custom.isEmpty,
           FileManager.default.isExecutableFile(atPath: custom) {
            cached = URL(fileURLWithPath: custom)
            return cached!
        }
        if let env = ProcessInfo.processInfo.environment["BRAINWASH_BIN"], FileManager.default.isExecutableFile(atPath: env) {
            cached = URL(fileURLWithPath: env)
            return cached!
        }
        for url in candidates() where FileManager.default.isExecutableFile(atPath: url.path) {
            cached = url
            return url
        }
        if let built = try? buildFromRepo() {
            cached = built
            return built
        }
        throw BrainwashError.binaryMissing("brainwash-cli not found. Build with `go build -o dist/brainwash-cli ./cmd/brainwash-cli` or set BRAINWASH_BIN.")
    }

    private static func candidates() -> [URL] {
        var urls: [URL] = []
        let fm = FileManager.default
        let cwd = URL(fileURLWithPath: fm.currentDirectoryPath)
        if let exeDir = Bundle.main.executableURL?.deletingLastPathComponent() {
            urls.append(exeDir.appendingPathComponent(binaryName))
            urls.append(exeDir.appendingPathComponent("brainwash"))
        }
        urls += [
            cwd.appendingPathComponent("dist/\(binaryName)"),
            cwd.appendingPathComponent("dist/brainwash"),
            cwd.appendingPathComponent("../dist/\(binaryName)"),
            Bundle.main.bundleURL.appendingPathComponent("Contents/MacOS/\(binaryName)"),
            Bundle.main.bundleURL.appendingPathComponent("Contents/MacOS/brainwash"),
            Bundle.main.bundleURL.deletingLastPathComponent().appendingPathComponent("dist/\(binaryName)"),
        ]
        var dir = cwd
        for _ in 0..<6 {
            urls.append(dir.appendingPathComponent("dist/\(binaryName)"))
            urls.append(dir.appendingPathComponent("dist/brainwash"))
            dir = dir.deletingLastPathComponent()
        }
        let homeRepo = fm.homeDirectoryForCurrentUser.appendingPathComponent("work/research/agent_mem_transfer/dist")
        urls.append(homeRepo.appendingPathComponent(binaryName))
        urls.append(homeRepo.appendingPathComponent("brainwash"))
        if let path = ProcessInfo.processInfo.environment["PATH"] {
            for part in path.split(separator: ":") {
                let base = URL(fileURLWithPath: String(part))
                urls.append(base.appendingPathComponent(binaryName))
                urls.append(base.appendingPathComponent("brainwash"))
            }
        }
        return urls
    }

    private static func buildFromRepo() throws -> URL {
        guard let root = findRepoRoot() else {
            throw BrainwashError.binaryMissing("could not find brainwash go.mod")
        }
        let out = root.appendingPathComponent("dist/\(binaryName)")
        try FileManager.default.createDirectory(at: root.appendingPathComponent("dist"), withIntermediateDirectories: true)
        let proc = Process()
        proc.currentDirectoryURL = root
        proc.executableURL = URL(fileURLWithPath: "/usr/bin/env")
        proc.arguments = ["go", "build", "-o", out.path, "./cmd/brainwash-cli"]
        let err = Pipe()
        proc.standardError = err
        proc.standardOutput = Pipe()
        try proc.run()
        proc.waitUntilExit()
        if proc.terminationStatus != 0 || !FileManager.default.isExecutableFile(atPath: out.path) {
            let msg = String(data: err.fileHandleForReading.readDataToEndOfFile(), encoding: .utf8) ?? ""
            throw BrainwashError.commandFailed("auto-build helper failed: \(msg)")
        }
        return out
    }

    private static func findRepoRoot() -> URL? {
        var dir = URL(fileURLWithPath: FileManager.default.currentDirectoryPath)
        for _ in 0..<8 {
            let mod = dir.appendingPathComponent("go.mod")
            if let data = try? String(contentsOf: mod, encoding: .utf8), data.contains("module brainwash") {
                return dir
            }
            dir = dir.deletingLastPathComponent()
        }
        let fallback = FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("work/research/agent_mem_transfer")
        if FileManager.default.fileExists(atPath: fallback.appendingPathComponent("go.mod").path) {
            return fallback
        }
        return nil
    }
}
