#!/usr/bin/env bash
# Signs and notarises the macOS build.
#
# An unsigned app on macOS shows "cannot be opened because the developer cannot
# be verified", and a large share of people stop there — not because they can't
# work around it, but because a file converter is not worth the friction.
#
# Skips cleanly when credentials are absent, so a fork's CI is not broken by
# secrets it does not have.
set -euo pipefail

: "${MACOS_CERTIFICATE:?}"
: "${MACOS_CERTIFICATE_PASSWORD:?}"
: "${MACOS_NOTARY_USER:?}"
: "${MACOS_NOTARY_PASSWORD:?}"
: "${MACOS_NOTARY_TEAM_ID:?}"

app=$(find build/bin -maxdepth 1 -name '*.app' | head -1)
if [ -z "$app" ]; then
  echo "no .app bundle to sign" >&2
  exit 1
fi

# A throwaway keychain, so nothing is left behind on the runner.
keychain="$RUNNER_TEMP/lathe-signing.keychain-db"
password=$(openssl rand -hex 24)

security create-keychain -p "$password" "$keychain"
security set-keychain-settings -lut 900 "$keychain"
security unlock-keychain -p "$password" "$keychain"

echo "$MACOS_CERTIFICATE" | base64 --decode > "$RUNNER_TEMP/certificate.p12"
security import "$RUNNER_TEMP/certificate.p12" -k "$keychain" \
  -P "$MACOS_CERTIFICATE_PASSWORD" -T /usr/bin/codesign
security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$password" "$keychain"
security list-keychain -d user -s "$keychain"
rm -f "$RUNNER_TEMP/certificate.p12"

identity=$(security find-identity -v -p codesigning "$keychain" | awk 'NR==1 {print $2}')

# The hardened runtime is required for notarisation.
codesign --force --deep --options runtime --timestamp \
  --entitlements build/darwin/entitlements.plist \
  --sign "$identity" "$app"
codesign --verify --strict --verbose=2 "$app"

# Notarisation takes a zip, not the bundle directory.
ditto -c -k --sequesterRsrc --keepParent "$app" "$RUNNER_TEMP/notarize.zip"
xcrun notarytool submit "$RUNNER_TEMP/notarize.zip" \
  --apple-id "$MACOS_NOTARY_USER" \
  --password "$MACOS_NOTARY_PASSWORD" \
  --team-id "$MACOS_NOTARY_TEAM_ID" \
  --wait

# Stapling lets the app open on a machine that is offline the first time.
xcrun stapler staple "$app"
xcrun stapler validate "$app"

security delete-keychain "$keychain"
echo "signed and notarised: $app"
