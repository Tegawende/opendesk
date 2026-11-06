import SwiftUI

/// Settings › General: the workspace's starter Space (contracts/spaces.md "The starter Space").
/// Admins publish, edit and remove it with the Space editor; everyone in the workspace can start
/// a Space from it under New Space in the switcher.
struct WSStarterSpaceSection: View {
    @Environment(AppEnvironment.self) private var env
    @State private var editing: SpaceDraft?
    @State private var confirm: AccountConfirm?
    @State private var loaded = false

    private var store: SpacesStore { .shared }
    private var workspaceID: String? { env.session.workspace?.id }

    var body: some View {
        let starter = workspaceID.flatMap(store.starter(of:))
        AccountSection(
            title: "Starter Space",
            footer: "A sidebar for everyone in this workspace. Anyone here can start a Space from it under New Space in the workspace switcher."
        ) {
            if !loaded {
                AccountRow(title: "Loading")
                    .redacted(reason: .placeholder)
            } else if let starter {
                HStack(spacing: 0) {
                    SpaceMark(name: starter.name, icon: starter.icon, color: starter.color, size: 30)
                        .padding(.leading, 14)
                    AccountRow(title: starter.name, subtitle: summary(starter)) {
                        HStack(spacing: 8) {
                            Button("Edit") { edit(starter) }
                                .buttonStyle(OutlineButtonStyle(fullWidth: false, height: 32))
                            Button {
                                confirm = AccountConfirm(
                                    title: "Remove the starter Space?",
                                    message: "New members get the workspace's own sections instead. Copies people already made stay theirs.",
                                    confirmTitle: "Remove starter"
                                ) { await remove() }
                            } label: {
                                Image(systemName: "trash")
                            }
                            .buttonStyle(OutlineButtonStyle(fullWidth: false, height: 32, tint: Tone.red.text))
                            .accessibilityLabel("Remove the starter Space")
                        }
                    }
                }
            } else {
                AccountRow(title: "No starter Space", subtitle: "Members start with the workspace's own sections.") {
                    Button("Set up") { edit(nil) }
                        .buttonStyle(OutlineButtonStyle(fullWidth: false, height: 32))
                }
            }
        }
        .task(id: workspaceID) {
            guard let workspaceID else { return }
            await store.reloadStarter(workspaceID)
            loaded = true
        }
        .sheet(item: $editing) { draft in
            SpaceEditorSheet(draft: draft)
                .presentationDetents([.large])
                .presentationCornerRadius(Theme.Radius.sheet)
        }
        .accountConfirm($confirm)
    }

    private func summary(_ space: Space) -> String {
        let rows = space.sections.reduce(0) { $0 + $1.items.count }
        let sections = space.sections.count == 1 ? "1 section" : "\(space.sections.count) sections"
        return "\(sections), " + (rows == 1 ? "1 row" : "\(rows) rows")
    }

    private func edit(_ starter: Space?) {
        guard let workspaceID else { return }
        Haptics.tap()
        let space = starter ?? Space(
            name: env.session.workspace?.workspace.name ?? "Starter",
            color: SpaceRules.nextColor([]),
            sections: [SpaceSection(id: SpaceRules.newSectionID(in: nil))]
        )
        editing = SpaceDraft(space: space, isNew: starter == nil, starterOf: workspaceID)
    }

    private func remove() async {
        guard let workspaceID else { return }
        if let failure = await store.removeStarter(in: workspaceID) {
            env.toasts.show(failure, kind: .error)
        } else {
            env.toasts.show("Starter Space removed", kind: .success)
        }
    }
}
