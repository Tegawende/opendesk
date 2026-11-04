# Task 1 report: Persist and expose workspace product settings

## What changed

- Added `tickets_enabled` and `mail_enabled` to the existing workspace settings model and patch validator. Missing values deserialize as enabled, preserving the behavior of old workspace rows without a migration. Newly created workspaces explicitly store both as enabled.
- Added the effective Mail fields `mail_available` and `mail_disabled_reason` to the Workspace response. The reason is selected in precedence order: instance Collab disabled, missing workspace Collab entitlement, then the workspace Mail setting disabled.
- Kept the existing PATCH flow and added a workspace row lock around the merged product state check. It rejects a resulting state with both products off as `422 at_least_one_product_required`, so concurrent writes cannot bypass the invariant.
- Product flag changes use the existing JSONB merge, audit `workspace.updated`, and `workspace.updated` event publication in the same transaction.
- Updated `contracts/api.md`, the web API types, and the workspace settings unit test.

## Tests and verification

- `go test ./internal/core` — passed (`ok`, 1.092s).
- `go build ./...` — passed.
- `go vet ./...` — passed.

## Files changed

- `internal/core/workspaces.go`
- `internal/core/workspace_settings_test.go`
- `contracts/api.md`
- `web/src/lib/api/types.ts`

## Self-review

- Confirmed existing rows with no product keys map to both enabled in the compatibility helper and response model.
- Confirmed product validation reads and checks the merged settings while holding the workspace row lock.
- No data migration is required. No unresolved concerns.

## Review fixes

- Allocated independent defaults for `tickets_enabled` and `mail_enabled`, with regression coverage proving disabling either flag leaves the other enabled and old JSON missing either key retains the enabled default.
- Centralized effective Mail application in the service's Workspace loader and membership loader. `/v1/workspace`, workspace creation/updates, invitation responses, workspace image responses, `/v1/me` memberships, workspace overviews, and workspace image/settings events now carry instance and entitlement precedence. Image hooks receive the service so event payloads use the same calculation.
- Added `TestEffectiveMailStateOnWorkspaceMembershipAndImageEvent` for `/v1/workspace`, `/v1/me`, instance/plan/workspace reason precedence, and image event payloads.

### Verification after review fixes

- `go test ./internal/core` — passed; database-backed tests are skipped unless `OPENDESK_TEST_DATABASE_URL` is set.
- `go test ./internal/core -run 'TestWorkspaceProductSettingsDefaultAndPatch|TestEffectiveMailStateOnWorkspaceMembershipAndImageEvent' -v` — pointer/default regression passed; the integration test was skipped because `OPENDESK_TEST_DATABASE_URL` is not configured.
- `go build ./...` — passed.
- `go vet ./...` — passed.
