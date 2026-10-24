# Static assets

Reference data the server reads at start: for the map, the flight phases, the runway
judge, and the phraseology grammar that reads callsigns from the radio. No file here is
sent anywhere.

`scripts/download_assets.sh` (or `.ps1` on Windows) downloads the six public files
below again; the two observation tables are this project's own and are not downloaded.

## Public data

### `aircraft.csv`
Aircraft metadata (registration, type, manufacturer), joined to the live ADS-B by hex
code, for sources that do not carry it.

* **Source**: [wiedehopf/tar1090-db](https://github.com/wiedehopf/tar1090-db)
* **Format**: semicolon-separated CSV (hex;registration;type;…)

### `airlines.dat`
Airline operators: ICAO and IATA codes, radiotelephony callsign, country. The grammar
matches the operator a pilot or controller says against it.

* **Source**: [OpenFlights](https://openflights.org/data), frozen around 2014: what it
  gets wrong or lacks is corrected by `telephony-overrides.csv` below.
* **Format**: CSV (id, name, alias, IATA, ICAO, callsign, country, active)

### `airports.csv`
Airports and their positions: the airports drawn on the map, and the reference airport
the server picks among those followed.

* **Source**: [OurAirports](https://ourairports.com/data/)

### `runways.csv`
Runway ends and thresholds: the runway judge, take-off and landing detection, the
arrival sequence and the PAR view.

* **Source**: [OurAirports](https://ourairports.com/data/)

### `airport-frequencies.csv`, `navaids.csv`
Airport radio frequencies and radio navaids (VOR, DME, NDB) for the map and the
frequency names.

* **Source**: [OurAirports](https://ourairports.com/data/)

## This project's observations

### `telephony-overrides.csv`
Radiotelephony designators that `airlines.dat` gets wrong, lacks or ignores, as heard
over the receiver. Read by the grammar (`internal/transcription/phraseology`).

### `spoken-operators.csv`
How operator names come out of the speech recognition on this station, mangled forms
included (`mazda` for Malta Air), with the evidence count behind each line. Not an
aeronautical reference: a record of observations, in French.
