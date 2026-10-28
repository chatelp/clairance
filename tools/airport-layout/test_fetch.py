"""Tests of fetch.py without the network: python3 -m unittest discover tools/airport-layout"""
import io
import json
import os
import sys
import tempfile
import unittest
import urllib.error
from unittest import mock

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fetch  # noqa: E402


def way(aeroway, coords, **tags):
    return {"type": "way", "id": 1, "tags": {"aeroway": aeroway, **tags},
            "geometry": [{"lat": lat, "lon": lon} for lon, lat in coords]}


SQUARE = [(2.0, 48.0), (2.01, 48.0), (2.01, 48.01), (2.0, 48.01), (2.0, 48.0)]


class Features(unittest.TestCase):
    def test_runway_centreline_is_a_line_with_its_tags(self):
        [f] = fetch.to_features([way("runway", [(2.1234567, 48.7654321), (2.2, 48.8)],
                                     ref="06/24", width="45", surface="asphalt", lit="yes")])
        self.assertEqual(f["geometry"]["type"], "LineString")
        self.assertEqual(f["geometry"]["coordinates"][0], [2.123457, 48.765432])
        self.assertEqual(f["properties"], {"aeroway": "runway", "ref": "06/24", "width": "45", "surface": "asphalt"})

    def test_closed_apron_is_an_area_closed_taxiway_is_not(self):
        apron, loop = fetch.to_features([way("apron", SQUARE), way("taxiway", SQUARE)])
        self.assertEqual(apron["geometry"]["type"], "Polygon")
        self.assertEqual(loop["geometry"]["type"], "LineString")

    def test_runway_drawn_as_an_area_stays_an_area(self):
        [f] = fetch.to_features([way("runway", SQUARE, area="yes")])
        self.assertEqual(f["geometry"]["type"], "Polygon")

    def test_hangars_and_degenerate_ways_are_left_out(self):
        self.assertEqual(fetch.to_features([way("hangar", SQUARE), way("taxiway", [(2.0, 48.0)])]), [])

    def test_multipolygon_joins_split_outer_ways_and_keeps_its_hole(self):
        outer_a = [(2.0, 48.0), (2.1, 48.0), (2.1, 48.1)]
        outer_b = [(2.0, 48.0), (2.0, 48.1), (2.1, 48.1)]  # reversed: ends where outer_a ends
        hole = [(2.04, 48.04), (2.06, 48.04), (2.06, 48.06), (2.04, 48.04)]
        relation = {"type": "relation", "id": 2, "tags": {"type": "multipolygon", "aeroway": "apron"},
                    "members": [
                        {"type": "way", "role": "outer", "geometry": [{"lat": y, "lon": x} for x, y in outer_a]},
                        {"type": "way", "role": "outer", "geometry": [{"lat": y, "lon": x} for x, y in outer_b]},
                        {"type": "way", "role": "inner", "geometry": [{"lat": y, "lon": x} for x, y in hole]},
                    ]}
        [f] = fetch.to_features([relation])
        self.assertEqual(f["geometry"]["type"], "Polygon")
        outer, inner = f["geometry"]["coordinates"]
        self.assertEqual(outer[0], outer[-1])
        self.assertEqual(len(outer), 5)
        self.assertEqual(inner, [list(p) for p in hole])

    def test_two_separate_outers_make_a_multipolygon(self):
        far = [(x + 1, y) for x, y in SQUARE]
        relation = {"type": "relation", "tags": {"type": "multipolygon", "aeroway": "terminal"},
                    "members": [{"type": "way", "role": "outer", "geometry": [{"lat": y, "lon": x} for x, y in ring]}
                                for ring in (SQUARE, far)]}
        [f] = fetch.to_features([relation])
        self.assertEqual(f["geometry"]["type"], "MultiPolygon")
        self.assertEqual(len(f["geometry"]["coordinates"]), 2)


class Query(unittest.TestCase):
    def test_radius_and_values(self):
        q = fetch.overpass_query(48.81, 2.05, 35 * fetch.METRES_PER_NM)
        self.assertIn("around:64820,48.810000,2.050000", q)
        self.assertIn('"aeroway"~"^(runway|taxiway|apron|terminal|helipad)$"', q)
        self.assertIn('relation["type"="multipolygon"]', q)
        self.assertIn("out geom;", q)


class Mirrors(unittest.TestCase):
    def test_the_second_mirror_answers_when_the_first_fails(self):
        calls = []

        def urlopen(request, timeout):
            calls.append((request.full_url, request.get_header("User-agent")))
            if len(calls) == 1:
                raise urllib.error.HTTPError(request.full_url, 504, "Gateway Timeout", {}, None)
            return io.BytesIO(json.dumps({"elements": []}).encode())

        with mock.patch.object(fetch.urllib.request, "urlopen", urlopen):
            answer, url = fetch.fetch("q")
        self.assertEqual(answer, {"elements": []})
        self.assertEqual([c[0] for c in calls], list(fetch.MIRRORS))
        self.assertIn("Clairance", calls[0][1])

    def test_no_mirror_is_an_error(self):
        def urlopen(request, timeout):
            raise urllib.error.URLError("unreachable")

        with mock.patch.object(fetch.urllib.request, "urlopen", urlopen):
            with self.assertRaises(RuntimeError):
                fetch.fetch("q")


class Main(unittest.TestCase):
    def test_reads_the_station_and_writes_compact_geojson(self):
        with tempfile.TemporaryDirectory() as d:
            config = os.path.join(d, "config.toml")
            with open(config, "w") as f:
                f.write("[station]\nlatitude = 48.81\nlongitude = 2.05\n")
            out = os.path.join(d, "data", "layout.geojson")
            seen = {}

            def fake_fetch(query):
                seen["query"] = query
                return {"elements": [way("runway", [(2.0, 48.0), (2.1, 48.1)], ref="09/27")]}, "mirror"

            with mock.patch.object(fetch, "fetch", fake_fetch), mock.patch("sys.stdout", io.StringIO()):
                fetch.main(["--config", config, "--radius-nm", "10", "--output", out])
            self.assertIn("around:18520,48.810000,2.050000", seen["query"])
            text = open(out).read()
            self.assertNotIn(" ", text)
            self.assertEqual(json.loads(text)["features"][0]["properties"]["ref"], "09/27")

    def test_by_default_the_layout_goes_where_the_server_serves_it(self):
        with tempfile.TemporaryDirectory() as d:
            config = os.path.join(d, "config.toml")
            with open(config, "w") as f:
                f.write("[station]\nlatitude = 48.81\nlongitude = 2.05\n")
            self.assertEqual(fetch.default_output(config), os.path.join("data", "station", "airport-layout.geojson"))
            with open(config, "a") as f:
                f.write("[storage]\nsqlite_base_path = \"/srv/clairance\"\n")
            self.assertEqual(fetch.default_output(config), "/srv/clairance/station/airport-layout.geojson")

    def test_a_config_without_station_is_refused(self):
        with tempfile.NamedTemporaryFile("w", suffix=".toml", delete=False) as f:
            f.write("[other]\n")
        try:
            with self.assertRaises(SystemExit):
                fetch.station_position(f.name)
        finally:
            os.unlink(f.name)


if __name__ == "__main__":
    unittest.main()
