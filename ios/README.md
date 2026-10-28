# opendesk for iPhone

The SwiftUI app uses the same API as the web app. Its bundle ID is `sh.opendesk.app`.
See [the native contract](../contracts/native.md) for authentication, push, and client behaviour.

## Build checks

The required CI status builds the app for the simulator in both Debug and
Release configurations. The **iOS** workflow can also run these checks manually.
These builds do not use Apple signing credentials.

Device distribution is managed separately from the server and desktop release.
Release builds use production push and App Attest entitlements.
