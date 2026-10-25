#!/bin/sh
# Make AppIcon.icns from AppIcon.icon (Icon Composer), for a build without
# Xcode: build.sh compiles AppIcon.icon itself when Xcode is here. Since macOS
# 26 the system draws an app's icon from an Icon Composer document; an .icns
# alone is shrunk into a frame, smaller than the other icons of the Dock (seen
# on macOS 27, 25/10). Run it when AppIcon.icon changes (edit it in Icon
# Composer). Needs Xcode (actool).
set -e
here=$(cd "$(dirname "$0")" && pwd)
out=$(mktemp -d)
xcrun actool "$here/AppIcon.icon" --compile "$out" --platform macosx --minimum-deployment-target 13.0 \
	--app-icon AppIcon --output-partial-info-plist "$out/partial.plist" >/dev/null
cp "$out/AppIcon.icns" "$here/"
rm -rf "$out"
echo "made $here/AppIcon.icns"
