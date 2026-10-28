# opendesk for iPhone

The SwiftUI app uses the same API as the web app. Its bundle ID is `sh.opendesk.app`.
See [the native contract](../contracts/native.md) for authentication, push, and client behaviour.

## Release to TestFlight

Push a version tag such as `v0.4.5` to release the server, desktop apps, and iOS
build together. The release workflow waits for the TestFlight upload before
publishing the GitHub release. Apple processes the uploaded build before testers
can install it. External testing follows Apple's beta review rules.

The iOS marketing version is the tag's numeric version. For example,
`v0.4.5-beta.1` builds version `0.4.5`. The build number combines the release
workflow's run number and attempt, so a rerun produces a new build number.

Configure these repository Actions settings:

- Variable `IOS_TEAM_ID`: the Apple Developer team that owns `sh.opendesk.app`.
- Secrets `APPLE_API_KEY`, `APPLE_API_KEY_ID`, and `APPLE_API_ISSUER`: the same
  App Store Connect team API key used for desktop notarization. `APPLE_API_KEY`
  holds the base64-encoded `.p8` file. The key needs access to the app and
  automatic signing resources.

The app record and bundle ID belong to that team in App Store Connect and Apple
Developer. Xcode uses automatic provisioning and an isolated keychain on the
runner. Exported apps must pass code-signature verification and use production
push and App Attest entitlements before validation or upload.

To validate a signed build without uploading, run the **iOS** workflow on a branch
with `version=0.4.5` and `upload=false`. Leave `version` empty to build only for
the simulator. The **CI** workflow also accepts `ios_version=0.4.5` for signed
validation. Uploads require the matching `v*` tag.
