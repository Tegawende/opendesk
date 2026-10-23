-- A member's own choices for one Collab mail account (contracts/collab.md "Notifications"):
-- new_mail off mutes its collab_new_mail notifications for them. No row follows the
-- defaults. Rows go with the account and with the membership.
CREATE TABLE collab_account_prefs (
    workspace_id uuid NOT NULL,
    account_id   uuid NOT NULL REFERENCES collab_accounts(id) ON DELETE CASCADE,
    user_id      uuid NOT NULL,
    new_mail     boolean NOT NULL DEFAULT true,
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, user_id),
    FOREIGN KEY (workspace_id, user_id) REFERENCES members(workspace_id, user_id) ON DELETE CASCADE
);
CREATE INDEX collab_account_prefs_member_idx ON collab_account_prefs(workspace_id, user_id);
