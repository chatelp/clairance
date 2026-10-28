#!/bin/sh
# Build the menu bar launcher into tools/menubar/build/Clairance.app.
# With --install, also copy it to ~/Applications.
#
# Needs the Xcode command line tools (swiftc) and macOS 13 or later.
set -e
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
app="$here/build/Clairance.app"
install=$1

rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
# Clairance.swift learns the checkout it was built from, and a stamp of the
# build, so that two builds never make the same program; the other sources go
# as they are.
stamp="$(date +%Y%m%d-%H%M%S)-$(git -C "$repo" rev-parse --short HEAD 2>/dev/null || echo nogit)"
sed -e "s|@REPO@|$repo|" -e "s|@BUILD@|$stamp|" "$here/Clairance.swift" > "$here/build/Clairance.swift"
set --
for f in "$here"/*.swift; do
	[ "$(basename "$f")" = Clairance.swift ] || set -- "$@" "$f"
done
swiftc -O -parse-as-library -target "$(uname -m)-apple-macos13.0" \
	-o "$app/Contents/MacOS/clairance" "$here/build/Clairance.swift" "$@"
# The sign's six menu bar states (docs-fr/design/10-lanceur.md): stopped,
# switching and running are template SVGs, on air, fault and down keep their
# colours.
cp "$here"/barre-menus-*.svg "$app/Contents/Resources/"
# The app's icon: AppIcon.icon compiled by Xcode's actool when Xcode is here,
# which macOS 26 and later draw at the size and in the shape of every icon;
# else the AppIcon.icns kept beside it (icon.sh), which they shrink into a frame.
if xcrun --find actool >/dev/null 2>&1; then
	xcrun actool "$here/AppIcon.icon" --compile "$app/Contents/Resources" --platform macosx \
		--minimum-deployment-target 13.0 --app-icon AppIcon --output-partial-info-plist "$here/build/icon.plist" >/dev/null
else
	cp "$here/AppIcon.icns" "$app/Contents/Resources/"
fi
cat > "$app/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>Clairance</string>
	<key>CFBundleIdentifier</key><string>dev.clairance.launcher</string>
	<key>CFBundleExecutable</key><string>clairance</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleIconFile</key><string>AppIcon</string>
	<key>CFBundleIconName</key><string>AppIcon</string>
	<key>CFBundleShortVersionString</key><string>1.0</string>
	<key>CFBundleVersion</key><string>$stamp</string>
	<key>LSMinimumSystemVersion</key><string>13.0</string>
	<key>LSUIElement</key><true/>
	<!-- Plain HTTP: the launcher only calls the addresses of its own settings,
	     all on the local network or the VPN (the server, the sidecar, radio-ctl), and
	     a name like station.lan is not among the local names ATS allows. -->
	<key>NSAppTransportSecurity</key>
	<dict><key>NSAllowsArbitraryLoads</key><true/></dict>
	<key>NSLocalNetworkUsageDescription</key>
	<string>Clairance, started from here, reads the receiving station's streams on the local network.</string>
</dict>
</plist>
EOF
# The carrier: the same program as an app of its own, which the launcher copies
# per build out of reach of any install before it runs production in it (A3).
carrier="$app/Contents/Helpers/Clairance Server.app"
mkdir -p "$carrier/Contents/MacOS"
cp "$app/Contents/MacOS/clairance" "$carrier/Contents/MacOS/clairance"
cat > "$carrier/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>Clairance Server</string>
	<key>CFBundleDisplayName</key><string>Clairance Server</string>
	<key>CFBundleIdentifier</key><string>dev.clairance.carrier</string>
	<key>CFBundleExecutable</key><string>clairance</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleShortVersionString</key><string>1.0</string>
	<key>CFBundleVersion</key><string>$stamp</string>
	<key>LSMinimumSystemVersion</key><string>13.0</string>
	<key>LSUIElement</key><true/>
	<key>NSAppTransportSecurity</key>
	<dict><key>NSAllowsArbitraryLoads</key><true/></dict>
	<key>NSLocalNetworkUsageDescription</key>
	<string>Clairance's production reads the receiving station's aircraft and streams on the local network.</string>
</dict>
</plist>
PLIST

# Signed ad hoc, unless ~/.config/clairance/codesign-identity names a signing
# identity of this Mac's keychain (outside the repository): the app then keeps
# one identity from one build to the next. Notifications also need Clairance
# allowed in System Settings > Notifications: switched off there, macOS refuses
# them without asking ("Notifications are not allowed for this application",
# 25/10).
identity=$(cat "$HOME/.config/clairance/codesign-identity" 2>/dev/null || true)
# The carrier first: an app is signed after what it contains.
if [ -n "$identity" ]; then
	codesign --force --sign "$identity" "$carrier"
	codesign --force --sign "$identity" "$app"
else
	codesign --force --sign - "$carrier" >/dev/null 2>&1 || true
	codesign --force --sign - "$app" >/dev/null 2>&1 || true
fi
echo "built $app"

if [ "$install" = "--install" ]; then
	# Never while a carrier runs from the installed app: replacing the bundle under it
	# took production's local network away at once, though the carrier ran on
	# (26/10, 10:49). Carriers are read by their exact name and arguments.
	for pid in $(pgrep -x clairance); do
		args=$(ps -o args= -p "$pid")
		case "$args" in
		"$HOME/Applications/Clairance.app/Contents/MacOS/clairance --carry"*)
			echo "not installed: a carrier runs from $HOME/Applications/Clairance.app (pid $pid);" >&2
			echo "replacing the app now would take the local network from what it carries. Stop production first." >&2
			exit 1 ;;
		esac
	done
	mkdir -p "$HOME/Applications"
	rm -rf "$HOME/Applications/Clairance.app"
	cp -R "$app" "$HOME/Applications/"
	echo "installed $HOME/Applications/Clairance.app"
fi
