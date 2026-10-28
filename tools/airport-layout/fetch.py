#!/usr/bin/env python3
"""Fetch the airports around the station from OpenStreetMap, for the map's
"Airport layout" layer.

Runways, taxiways, aprons, terminals and helipads (not hangars) within a radius
of the station, as ways and multipolygon relations, from the Overpass API; written
as a compact GeoJSON file that the web interface loads from
/data/airport-layout.geojson. The server serves it from station/ in its data
directory ([storage] sqlite_base_path), where this writes it by default: the
page is built into the server, and a file that belongs to one station cannot be.
It is not committed: each installation generates its own, so the repository
redistributes no OpenStreetMap data (ODbL).

Usage, from the repository root (Python 3.11+, standard library only):

    python3 tools/airport-layout/fetch.py --config configs/config.toml
    python3 tools/airport-layout/fetch.py --config configs/config.toml --radius-nm 20
"""
import argparse
import json
import os
import sys
import time
import tomllib
import urllib.error
import urllib.parse
import urllib.request

MIRRORS = (
    "https://overpass.openstreetmap.fr/api/interpreter",
    "https://overpass-api.de/api/interpreter",
)
USER_AGENT = "Clairance airport-layout/1.0 (+https://github.com/chatelp/clairance)"
AEROWAYS = ("runway", "taxiway", "apron", "terminal", "helipad")
AREA_AEROWAYS = ("apron", "terminal", "helipad")  # a closed way of these is an area
PROPERTIES = ("aeroway", "ref", "name", "width", "surface")
METRES_PER_NM = 1852


def overpass_query(lat, lon, radius_m):
    """Ways, and multipolygon relations, of the wanted aeroway values, with geometry."""
    pattern = "^(" + "|".join(AEROWAYS) + ")$"
    around = f"around:{radius_m:.0f},{lat:.6f},{lon:.6f}"
    return (
        "[out:json][timeout:180];\n"
        "(\n"
        f'  way["aeroway"~"{pattern}"]({around});\n'
        f'  relation["type"="multipolygon"]["aeroway"~"{pattern}"]({around});\n'
        ");\n"
        "out geom;\n"
    )


def fetch(query, mirrors=MIRRORS, timeout=200):
    """POST the query to each mirror in turn; the first answer wins."""
    data = urllib.parse.urlencode({"data": query}).encode()
    errors = []
    for url in mirrors:
        request = urllib.request.Request(url, data=data, headers={"User-Agent": USER_AGENT})
        try:
            with urllib.request.urlopen(request, timeout=timeout) as response:
                return json.load(response), url
        except (urllib.error.URLError, TimeoutError, json.JSONDecodeError) as e:
            errors.append(f"{url}: {e}")
    raise RuntimeError("no Overpass mirror answered:\n  " + "\n  ".join(errors))


def point(node):
    return [round(node["lon"], 6), round(node["lat"], 6)]


def line(geometry):
    coords = [point(n) for n in geometry if n]
    # Consecutive duplicates add bytes and nothing else.
    return [c for i, c in enumerate(coords) if i == 0 or c != coords[i - 1]]


def join_rings(segments):
    """Join way segments end to end into closed rings; leftovers are dropped."""
    segments = [s for s in segments if len(s) >= 2]
    rings = []
    while segments:
        ring = segments.pop(0)
        while ring[0] != ring[-1]:
            for i, s in enumerate(segments):
                if s[0] == ring[-1]:
                    ring = ring + s[1:]
                elif s[-1] == ring[-1]:
                    ring = ring + s[-2::-1]
                elif s[-1] == ring[0]:
                    ring = s[:-1] + ring
                elif s[0] == ring[0]:
                    ring = s[:0:-1] + ring
                else:
                    continue
                segments.pop(i)
                break
            else:
                break  # open ring: nothing continues it
        if ring[0] == ring[-1] and len(ring) >= 4:
            rings.append(ring)
    return rings


def inside(pt, ring):
    """Ray casting: is pt inside ring (lon/lat, good enough at airport scale)."""
    x, y = pt
    result = False
    for (x1, y1), (x2, y2) in zip(ring, ring[1:]):
        if (y1 > y) != (y2 > y) and x < (x2 - x1) * (y - y1) / (y2 - y1) + x1:
            result = not result
    return result


def properties(tags):
    return {k: tags[k] for k in PROPERTIES if k in tags}


def to_features(elements):
    """Overpass elements (out geom) to GeoJSON features."""
    features = []
    for e in elements:
        tags = e.get("tags", {})
        if tags.get("aeroway") not in AEROWAYS:
            continue
        if e["type"] == "way":
            coords = line(e.get("geometry", []))
            if len(coords) < 2:
                continue
            closed = coords[0] == coords[-1] and len(coords) >= 4
            is_area = closed and (tags["aeroway"] in AREA_AEROWAYS or tags.get("area") == "yes")
            geometry = {"type": "Polygon", "coordinates": [coords]} if is_area \
                else {"type": "LineString", "coordinates": coords}
        elif e["type"] == "relation":
            outers = join_rings([line(m.get("geometry", [])) for m in e.get("members", [])
                                 if m.get("type") == "way" and m.get("role") in ("outer", "")])
            inners = join_rings([line(m.get("geometry", [])) for m in e.get("members", [])
                                 if m.get("type") == "way" and m.get("role") == "inner"])
            if not outers:
                continue
            polygons = [[outer] + [r for r in inners if inside(r[0], outer)] for outer in outers]
            geometry = {"type": "Polygon", "coordinates": polygons[0]} if len(polygons) == 1 \
                else {"type": "MultiPolygon", "coordinates": polygons}
        else:
            continue
        features.append({"type": "Feature", "properties": properties(tags), "geometry": geometry})
    return features


def station_position(config_path):
    with open(config_path, "rb") as f:
        station = tomllib.load(f).get("station", {})
    try:
        return float(station["latitude"]), float(station["longitude"])
    except (KeyError, TypeError, ValueError):
        sys.exit(f"{config_path}: [station] latitude and longitude are required")


def default_output(config_path):
    """Where the server serves the layout from: station/ in its data directory,
    [storage] sqlite_base_path ("data" when unset), relative to the directory the
    server runs in, as the server reads it."""
    with open(config_path, "rb") as f:
        storage = tomllib.load(f).get("storage", {})
    return os.path.join(storage.get("sqlite_base_path") or "data", "station", "airport-layout.geojson")


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    parser.add_argument("--config", required=True, help="co-atc config.toml (its [station] section)")
    parser.add_argument("--radius-nm", type=float, default=35.0, help="radius around the station (default 35)")
    parser.add_argument("--output", help="GeoJSON file to write (default: station/airport-layout.geojson "
                                          "in the configuration's sqlite_base_path, where the server serves it)")
    args = parser.parse_args(argv)
    if not args.output:
        args.output = default_output(args.config)

    lat, lon = station_position(args.config)
    started = time.monotonic()
    try:
        answer, mirror = fetch(overpass_query(lat, lon, args.radius_nm * METRES_PER_NM))
    except RuntimeError as e:
        sys.exit(str(e))
    elapsed = time.monotonic() - started
    features = to_features(answer.get("elements", []))

    os.makedirs(os.path.dirname(os.path.abspath(args.output)), exist_ok=True)
    collection = {"type": "FeatureCollection", "features": features}
    with open(args.output, "w", encoding="utf-8") as f:
        json.dump(collection, f, separators=(",", ":"), ensure_ascii=False)

    counts = {}
    for feature in features:
        counts[feature["properties"]["aeroway"]] = counts.get(feature["properties"]["aeroway"], 0) + 1
    print(f"{mirror}: {elapsed:.1f} s, {len(answer.get('elements', []))} elements")
    print("  " + ", ".join(f"{counts.get(a, 0)} {a}" for a in AEROWAYS))
    print(f"wrote {args.output} ({os.path.getsize(args.output)} bytes)")


if __name__ == "__main__":
    main()
