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
# Clairance.swift learns the checkout it was built from; the other sources go as they are.
sed "s|@REPO@|$repo|" "$here/Clairance.swift" > "$here/build/Clairance.swift"
set --
for f in "$here"/*.swift; do
	[ "$(basename "$f")" = Clairance.swift ] || set -- "$@" "$f"
done
swiftc -O -parse-as-library -target "$(uname -m)-apple-macos13.0" \
	-o "$app/Contents/MacOS/clairance" "$here/build/Clairance.swift" "$@"
# The sign's three menu bar states (docs-fr/design/identite/): stopped and
# running are template SVGs, listening keeps its colours.
cp "$here/barre-menus-marche.svg" "$here/barre-menus-arret.svg" "$here/barre-menus-ecoute.svg" "$app/Contents/Resources/"
cat > "$app/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>Clairance</string>
	<key>CFBundleIdentifier</key><string>dev.clairance.launcher</string>
	<key>CFBundleExecutable</key><string>clairance</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleShortVersionString</key><string>1.0</string>
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
codesign --force --sign - "$app" >/dev/null 2>&1 || true
echo "built $app"

if [ "$install" = "--install" ]; then
	mkdir -p "$HOME/Applications"
	rm -rf "$HOME/Applications/Clairance.app"
	cp -R "$app" "$HOME/Applications/"
	echo "installed $HOME/Applications/Clairance.app"
fi
