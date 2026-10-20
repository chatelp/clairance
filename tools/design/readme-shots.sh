#!/bin/sh
# The README's images: build the screens (readme.py), capture them at twice
# the size (capture.mjs), crop and compress them into .github/readme/.
#   sh tools/design/readme-shots.sh [work-dir]
# Needs node, python3 and ImageMagick (magick). 256 colours, no dither: on
# these flat screens it cannot be told from the full PNG and weighs a third.
set -e
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
W=${1:-$ROOT/runs/design/readme}
mkdir -p "$W" "$ROOT/.github/readme"
python3 "$ROOT/tools/design/readme.py" "$W"
for n in hero follow map-routes par; do
  node "$ROOT/tools/design/capture.mjs" "file://$W/$n.html" "$W/$n-2x.png" --size 2880x1800 --wait 1200
done
node "$ROOT/tools/design/capture.mjs" "file://$W/phone.html" "$W/phone-2x.png" --size 1760x1800 --wait 1200
png8() { magick "$1" $2 -strip -dither None -colors 256 "PNG8:$ROOT/.github/readme/$3.png"; }
png8 "$W/hero-2x.png" "-resize 1600x" hero                                   # the whole screen
png8 "$W/hero-2x.png" "-crop 880x1704+0+96 +repage" thread                   # the radio thread
png8 "$W/follow-2x.png" "-crop 2000x1704+880+96 +repage -resize 1400x" follow  # the sky and the story
png8 "$W/map-routes-2x.png" "-crop 1240x1344+880+96 +repage" map-routes       # the sky, without the legend
png8 "$W/par-2x.png" "-crop 1240x1064+880+96 +repage" par                     # the PAR view, its first runway
png8 "$W/phone-2x.png" "-crop 1650x1704+40+40 +repage -resize 1100x" phone    # two panes
ls -l "$ROOT/.github/readme"
