import Foundation

struct SlotInfo: Codable, Hashable, Identifiable {
    var name: String
    var label: String
    var id: String { name }
}

struct SessionRef: Codable, Hashable, Identifiable {
    var slot: String
    var id: String
    var cwd: String
    var title: String
    var path: String
    var createdAt: Date
    var updatedAt: Date
    var bytes: Int64?

    var identity: String { "\(slot):\(path)" }
}

struct ToolTrace: Codable, Hashable {
    var callId: String?
    var name: String
    var arguments: String?
    var result: String?
    var isError: Bool?
}

struct Inject: Codable, Hashable {
    var kind: String
    var summary: String
    var text: String?
}

struct Event: Codable, Hashable, Identifiable {
    var timestamp: Date
    var role: String
    var kind: String?
    var text: String
    var thinking: String?
    var tools: [ToolTrace]?
    var images: [String]?
    var injects: [Inject]?
    var interrupted: Bool?
    var sourceKind: String?

    var id: String { "\(sourceKind ?? "")-\(timestamp.timeIntervalSince1970)-\(role)-\(text.hashValue)" }
    var isUser: Bool { role == "user" && kind != "inject" }
    var isAssistant: Bool { role == "assistant" }
    var isFoldedMeta: Bool { role == "system" || role == "summary" || kind == "inject" }
}

struct NeutralSession: Codable {
    var id: String
    var slot: String
    var cwd: String
    var title: String
    var sourcePath: String
    var createdAt: Date
    var updatedAt: Date
    var events: [Event]
    var notes: [String]?
}

struct CloneResult: Codable, Hashable {
    var sourceId: String
    var sourcePath: String
    var destPath: String
    var events: Int
    var title: String
}

struct ExportResult: Codable, Hashable {
    var path: String
    var title: String
    var slot: String
    var events: Int
}

struct ImportResult: Codable, Hashable {
    var packedPath: String
    var sourceSlot: String
    var destSlot: String
    var destPath: String
    var title: String
    var events: Int
    var cwd: String
}

enum BrainwashError: Error, LocalizedError {
    case binaryMissing(String)
    case commandFailed(String)
    case decode(String)

    var errorDescription: String? {
        switch self {
        case .binaryMissing(let s): return s
        case .commandFailed(let s): return s
        case .decode(let s): return s
        }
    }
}

enum Client {
    static let decoder: JSONDecoder = {
        let d = JSONDecoder()
        d.dateDecodingStrategy = .custom { dec in
            let c = try dec.singleValueContainer()
            if let s = try? c.decode(String.self) {
                if let t = ISO8601DateFormatter.frac.date(from: s) { return t }
                if let t = ISO8601DateFormatter.plain.date(from: s) { return t }
                if let t = ISO8601DateFormatter.local.date(from: s) { return t }
            }
            if let n = try? c.decode(Double.self) {
                if n > 1e12 { return Date(timeIntervalSince1970: n / 1000) }
                return Date(timeIntervalSince1970: n)
            }
            return Date.distantPast
        }
        return d
    }()

    static func run(_ args: [String]) throws -> Data {
        let bin = try Helper.binary()
        let proc = Process()
        let out = Pipe()
        let err = Pipe()
        proc.standardOutput = out
        proc.standardError = err
        proc.executableURL = bin
        proc.arguments = AppSettings.extraArgList() + args

        final class Box: @unchecked Sendable {
            var data = Data()
        }
        let stdout = Box()
        let stderr = Box()
        let group = DispatchGroup()
        group.enter()
        out.fileHandleForReading.readabilityHandler = { h in
            stdout.data.append(h.availableData)
        }
        err.fileHandleForReading.readabilityHandler = { h in
            stderr.data.append(h.availableData)
        }
        proc.terminationHandler = { _ in
            out.fileHandleForReading.readabilityHandler = nil
            err.fileHandleForReading.readabilityHandler = nil
            stdout.data.append(out.fileHandleForReading.readDataToEndOfFile())
            stderr.data.append(err.fileHandleForReading.readDataToEndOfFile())
            group.leave()
        }
        try proc.run()
        group.wait()

        if proc.terminationStatus != 0 {
            let msg = String(data: stderr.data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            throw BrainwashError.commandFailed(msg.isEmpty ? "brainwash-cli exited \(proc.terminationStatus)" : msg)
        }
        return stdout.data
    }

    static func slots() throws -> [SlotInfo] {
        struct Wrap: Codable { var slots: [SlotInfo] }
        return try decoder.decode(Wrap.self, from: try run(["slots"])).slots
    }

    static func list(slot: String, cwd: String = "") throws -> [SessionRef] {
        var args = ["list"]
        if !slot.isEmpty, slot != "all" {
            args += ["--slot", slot]
        }
        if !cwd.isEmpty {
            args += ["--cwd", cwd]
        }
        return try decoder.decode([SessionRef].self, from: try run(args))
    }

    static func show(slot: String, session: String, path: String) throws -> NeutralSession {
        var args = ["show", "--slot", slot]
        if !path.isEmpty {
            args += ["--path", path]
        }
        if !session.isEmpty {
            args += ["--session", session]
        }
        do {
            return try decoder.decode(NeutralSession.self, from: try run(args))
        } catch {
            throw BrainwashError.decode("show decode failed: \(error.localizedDescription)")
        }
    }

    static func clone(from: String, to: String, session: String, path: String, outCwd: String, includeTools: Bool) throws -> [CloneResult] {
        var args = ["clone", "--from", from, "--to", to]
        if !path.isEmpty { args += ["--path", path] }
        if !session.isEmpty { args += ["--session", session] }
        if !outCwd.isEmpty { args += ["--out-cwd", outCwd] }
        if !includeTools { args.append("--no-tools") }
        return try decoder.decode([CloneResult].self, from: try run(args))
    }

    static func exportPacked(slot: String, session: String, path: String, out: String, includeTools: Bool) throws -> ExportResult {
        var args = ["export", "--slot", slot, "--out", out]
        if !path.isEmpty { args += ["--path", path] }
        if !session.isEmpty { args += ["--session", session] }
        if !includeTools { args.append("--no-tools") }
        return try decoder.decode(ExportResult.self, from: try run(args))
    }

    static func importPacked(files: [String], to: String, outCwd: String, includeTools: Bool) throws -> [ImportResult] {
        var args = ["import"]
        for f in files { args += ["--file", f] }
        if !to.isEmpty { args += ["--to", to] }
        if !outCwd.isEmpty { args += ["--out-cwd", outCwd] }
        if !includeTools { args.append("--no-tools") }
        return try decoder.decode([ImportResult].self, from: try run(args))
    }
}

extension ISO8601DateFormatter {
    nonisolated(unsafe) static let frac: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()
    nonisolated(unsafe) static let plain: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime]
        return f
    }()
    nonisolated(unsafe) static let local: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds, .withTimeZone]
        return f
    }()
}
