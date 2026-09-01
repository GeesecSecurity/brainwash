import SwiftUI

enum ToastKind: String {
    case success
    case warning
    case error
    case info

    var tint: Color {
        switch self {
        case .success: return Palette.accent2
        case .warning: return Palette.warning
        case .error: return Palette.danger
        case .info: return Palette.accent
        }
    }

    var icon: String {
        switch self {
        case .success: return "checkmark.circle.fill"
        case .warning: return "exclamationmark.triangle.fill"
        case .error: return "xmark.octagon.fill"
        case .info: return "info.circle.fill"
        }
    }
}

struct ToastItem: Identifiable, Equatable {
    let id: UUID
    var kind: ToastKind
    var title: String
    var message: String
    var createdAt: Date

    init(kind: ToastKind, title: String, message: String = "") {
        self.id = UUID()
        self.kind = kind
        self.title = title
        self.message = message
        self.createdAt = Date()
    }
}

struct ToastStack: View {
    let items: [ToastItem]
    var onDismiss: (UUID) -> Void

    var body: some View {
        VStack(alignment: .trailing, spacing: 8) {
            ForEach(items) { item in
                ToastCard(item: item) { onDismiss(item.id) }
                    .transition(.move(edge: .top).combined(with: .opacity))
            }
        }
        .padding(16)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topTrailing)
        .allowsHitTesting(true)
    }
}

struct ToastCard: View {
    let item: ToastItem
    var onDismiss: () -> Void

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: item.kind.icon)
                .foregroundStyle(item.kind.tint)
                .font(.system(size: 16, weight: .semibold))
                .padding(.top, 1)
            VStack(alignment: .leading, spacing: 3) {
                Text(item.title)
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundStyle(.primary)
                if !item.message.isEmpty {
                    Text(item.message)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .textSelection(.enabled)
                        .lineLimit(6)
                }
            }
            Spacer(minLength: 8)
            Button(action: onDismiss) {
                Image(systemName: "xmark")
                    .font(.system(size: 10, weight: .bold))
                    .foregroundStyle(.secondary)
            }
            .buttonStyle(.plain)
        }
        .padding(12)
        .frame(width: 340, alignment: .leading)
        .background(.regularMaterial)
        .overlay(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .stroke(item.kind.tint.opacity(0.45), lineWidth: 1)
        )
        .overlay(alignment: .leading) {
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .fill(item.kind.tint)
                .frame(width: 4)
        }
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .shadow(color: .black.opacity(0.12), radius: 12, y: 4)
    }
}
