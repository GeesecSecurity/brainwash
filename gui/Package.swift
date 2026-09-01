// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "BrainwashGUI",
    platforms: [.macOS(.v14)],
    products: [
        .executable(name: "BrainwashGUI", targets: ["BrainwashGUI"]),
    ],
    targets: [
        .executableTarget(name: "BrainwashGUI"),
    ]
)
