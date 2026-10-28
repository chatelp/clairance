// Package aip makes the list of official names around a station from the
// French eAIP (SIA, DGAC): the significant points within a radius, with their
// coordinates (ENR 4.4); the call signs of the control positions (ENR 2.1 and
// AD 2 of each followed airport); the procedure designators the text names
// (OKIPA 1W). The matcher reads it to tell "ELCOB 7 Golf", a procedure, from a
// callsign's flight part (docs-fr/05-decisions.md, Q55).
//
// No AIP data is in the repository: each station makes its own list, for its
// own region and the cycle in force, from the settings page. The SIA's pages
// are public information; its terms ask that the source and the date of update
// be cited, which the list carries. The AIP changes every 28 days (the AIRAC
// cycle).
package aip

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yegors/co-atc/pkg/tool"
)

// A known AIRAC date; the cycle repeats every 28 days.
var airacEpoch = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

const airacDays = 28

const siaDVD = "https://www.sia.aviation-civile.gouv.fr/media/dvd"

var months = [...]string{"JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"}

// InForce is the AIRAC cycle in force on a day, as its first day (UTC).
func InForce(t time.Time) time.Time {
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	days := int(math.Floor(day.Sub(airacEpoch).Hours() / 24))
	cycles := days / airacDays
	if days < 0 && days%airacDays != 0 {
		cycles--
	}
	return airacEpoch.AddDate(0, 0, cycles*airacDays)
}

// BaseURL is where a cycle's eAIP pages are published.
func BaseURL(airac time.Time) string {
	dvd := fmt.Sprintf("eAIP_%02d_%s_%d", airac.Day(), months[airac.Month()-1], airac.Year())
	return fmt.Sprintf("%s/%s/FRANCE/AIRAC-%s/html/eAIP", siaDVD, dvd, airac.Format("2006-01-02"))
}

const (
	pagePoints  = "FR-ENR-4.4-fr-FR.html"
	pageRoutes  = "FR-ENR-2.1-fr-FR.html"
	pageAirport = "FR-AD-2.%s-fr-FR.html"
)

// PageNames are the pages read: the points, the airspace's call signs, and
// each airport's AD 2.
func PageNames(airports []string) []string {
	out := []string{pagePoints, pageRoutes}
	for _, a := range airports {
		out = append(out, fmt.Sprintf(pageAirport, strings.ToUpper(a)))
	}
	return out
}

// Point is a significant point: its five-letter name and where it is.
type Point struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
	NM   float64 `json:"nm"` // from the centre
}

// Names is the list a station keeps.
type Names struct {
	Source     string   `json:"source"`
	SourceURL  string   `json:"source_url"`
	AIRAC      string   `json:"airac"`
	NextAIRAC  string   `json:"next_airac"`
	Made       string   `json:"made"`
	RadiusNM   float64  `json:"radius_nm"`
	Airports   []string `json:"airports"`
	Points     []Point  `json:"points"`
	Callsigns  []string `json:"callsigns"`
	Procedures []string `json:"procedures"`
	// Airports whose AD 2 page could not be had (a small aerodrome may have
	// none): the list is made without them, and says so.
	Skipped []string `json:"skipped,omitempty"`

	// The points the followed airports' departures and arrivals are named
	// after (OPALE for OPALE 7A), from the SID and STAR coding tables of the
	// eAIP and the procedures the text names: the page names the procedures
	// heard against these alone (Q55, usage 3).
	ProcedurePoints []ProcedurePoint `json:"procedure_points,omitempty"`
	ChartsRead      int              `json:"charts_read"`
	ChartsNote      string           `json:"charts_note,omitempty"`

	// Airports followed outside the French eAIP: nothing is read for them.
	Outside []string `json:"outside,omitempty"`

	// The published routes of those departures and arrivals, for the map
	// (Q56, routes.go), and how their reading went.
	Routes     []Route     `json:"routes,omitempty"`
	RoutesNote *RoutesNote `json:"routes_note,omitempty"`

	allPoints map[string]Point // every point of ENR 4.4, for the procedures' coordinates
}

// ProcedurePoint is a point procedures are named after.
type ProcedurePoint struct {
	Name        string   `json:"name"`
	Lat         float64  `json:"lat"`
	Lon         float64  `json:"lon"`
	Designators []string `json:"designators"`
	// SID or STAR for each designator a coding table names; the ones only the
	// AIP text names have no kind.
	Kinds map[string]string `json:"kinds,omitempty"`
}

// Covered says whether the French eAIP holds an airport: LF, metropolitan
// France. The overseas AIPs are other publications.
func Covered(icao string) bool {
	return len(icao) == 4 && strings.HasPrefix(strings.ToUpper(icao), "LF")
}

var (
	reRow     = regexp.MustCompile(`(?s)<tr[^>]*>(.*?)</tr>`)
	reCell    = regexp.MustCompile(`(?s)<td[^>]*>(.*?)</td>`)
	reTag     = regexp.MustCompile(`<[^>]+>`)
	reScripts = regexp.MustCompile(`(?s)<script.*?</script>|<style.*?</style>`)
	// A value amended this cycle is printed twice: the old one struck out
	// (<del>), the new one inserted (<ins>). Only the new one holds.
	reDel   = regexp.MustCompile(`(?s)<del\b[^>]*>.*?</del>`)
	reSpace = regexp.MustCompile(`\s+`)
	reName5 = regexp.MustCompile(`^[A-Z]{5}$`)
	reDMS   = regexp.MustCompile(`^(\d+)°(\d+)'([\d.]+)"([NSEW])`)
	reCall  = regexp.MustCompile(`\b((?:[A-Z][A-Z'\-]+ ){0,2}[A-Z][A-Z'\-]+) (Approche|Départ|Tour|Sol|Prévol|Contrôle|Info|Information|Approach|Departure|Tower|Ground|Delivery|Control|Radar|Trafic|Prevol)\b`)
	// Table words and abbreviations caught before a role (NIL Départ, UAC ACS
	// PARIS Contrôle...) are not call signs.
	reNotAName = regexp.MustCompile(`\b(FREQ|HOR|MHZ|ATS|APP|TWR|FIS|ACC|SID|STAR|RWY|AD|FIR|UIR|TMA|CTR|NIL|PENTE|VPE|DEP|ATC|UAC|ACS|ASFC|FIC|INFO|CONTROLE)\b`)
	reProc     = regexp.MustCompile(`\b([A-Z]{5} ?\d[A-Z])\b`)
)

func plain(s string) string {
	return strings.TrimSpace(reSpace.ReplaceAllString(html.UnescapeString(reTag.ReplaceAllString(s, " ")), " "))
}

func pageText(raw string) string {
	raw = reDel.ReplaceAllString(reScripts.ReplaceAllString(raw, ""), "")
	return reSpace.ReplaceAllString(html.UnescapeString(reTag.ReplaceAllString(raw, " ")), " ")
}

func dms(s string) (float64, bool) {
	m := reDMS.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	d, _ := strconv.Atoi(m[1])
	mi, _ := strconv.Atoi(m[2])
	se, _ := strconv.ParseFloat(m[3], 64)
	v := float64(d) + float64(mi)/60 + se/3600
	if m[4] == "S" || m[4] == "W" {
		v = -v
	}
	return v, true
}

func distanceNM(lat1, lon1, lat2, lon2 float64) float64 {
	r := math.Pi / 180
	h := math.Pow(math.Sin((lat2-lat1)*r/2), 2) + math.Cos(lat1*r)*math.Cos(lat2*r)*math.Pow(math.Sin((lon2-lon1)*r/2), 2)
	return 2 * 3440.065 * math.Asin(math.Sqrt(h))
}

// points reads ENR 4.4's table: a five-letter name, then its latitude and
// longitude in degrees, minutes and seconds.
func points(raw string) []Point {
	var out []Point
	raw = reDel.ReplaceAllString(raw, "")
	for _, row := range reRow.FindAllStringSubmatch(raw, -1) {
		cells := reCell.FindAllStringSubmatch(row[1], -1)
		if len(cells) < 2 {
			continue
		}
		name := plain(cells[0][1])
		if !reName5.MatchString(name) {
			continue
		}
		parts := strings.Fields(plain(cells[1][1]))
		if len(parts) < 2 {
			continue
		}
		lat, ok1 := dms(parts[0])
		lon, ok2 := dms(parts[1])
		if ok1 && ok2 {
			out = append(out, Point{Name: name, Lat: lat, Lon: lon})
		}
	}
	return out
}

// Parse makes the list from the pages, keyed by their file names.
func Parse(pages map[string]string, lat, lon, radiusNM float64) Names {
	var n Names
	all := points(pages[pagePoints])
	n.allPoints = map[string]Point{}
	for _, p := range all {
		n.allPoints[p.Name] = p
	}
	for _, p := range all {
		d := distanceNM(lat, lon, p.Lat, p.Lon)
		if d <= radiusNM {
			p.Lat = math.Round(p.Lat*1e5) / 1e5
			p.Lon = math.Round(p.Lon*1e5) / 1e5
			p.NM = math.Round(d)
			n.Points = append(n.Points, p)
		}
	}
	sort.Slice(n.Points, func(i, j int) bool { return n.Points[i].Name < n.Points[j].Name })

	calls, procs := map[string]bool{}, map[string]bool{}
	for name, raw := range pages {
		if name == pagePoints {
			continue
		}
		t := pageText(raw)
		for _, m := range reCall.FindAllStringSubmatch(t, -1) {
			who := strings.TrimSpace(m[1])
			if len(who) > 2 && len(who) < 30 && !reNotAName.MatchString(who) {
				calls[who+" "+m[2]] = true
			}
		}
		for _, m := range reProc.FindAllStringSubmatch(t, -1) {
			procs[m[1]] = true
		}
	}
	n.Callsigns = sortedKeys(calls)
	n.Procedures = sortedKeys(procs)
	n.RadiusNM = radiusNM
	return n
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// pageLabel is how the source names a page: "ENR 4.4", "AD 2 LFPG".
func pageLabel(name string) string {
	label := strings.TrimSuffix(strings.TrimPrefix(name, "FR-"), "-fr-FR.html")
	return strings.ReplaceAll(strings.Replace(label, "AD-2.", "AD-2 ", 1), "-", " ")
}

// Load reads a list made earlier.
func Load(path string) (*Names, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var n Names
	if err := json.Unmarshal(raw, &n); err != nil {
		return nil, err
	}
	return &n, nil
}

// Maker makes the list on demand, one at a time, and says how it went.
type Maker struct {
	OutPath  string // the list
	CacheDir string // the pages, one folder per cycle
	Client   *http.Client
	Now      func() time.Time

	mu         sync.Mutex
	refreshing bool
	lastError  string
	lastTried  time.Time
}

// State is what the settings page shows.
type State struct {
	Refreshing bool
	LastError  string
	LastTried  time.Time
}

func (m *Maker) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return State{Refreshing: m.refreshing, LastError: m.lastError, LastTried: m.lastTried}
}

// ErrBusy says a list is being made already.
var ErrBusy = errors.New("the list is being made already")

// Request is what a list is made for.
type Request struct {
	Lat, Lon float64
	RadiusNM float64
	Airports []string
	AIRAC    time.Time // zero: the cycle in force
}

// Make downloads the pages the cache lacks and writes the list. One at a time.
func (m *Maker) Make(ctx context.Context, req Request) (*Names, error) {
	m.mu.Lock()
	if m.refreshing {
		m.mu.Unlock()
		return nil, ErrBusy
	}
	m.refreshing = true
	m.lastTried = m.now()
	m.mu.Unlock()

	n, err := m.make(ctx, req)

	m.mu.Lock()
	m.refreshing = false
	m.lastError = ""
	if err != nil {
		m.lastError = err.Error()
	}
	m.mu.Unlock()
	return n, err
}

func (m *Maker) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Maker) make(ctx context.Context, req Request) (*Names, error) {
	var outside []string
	var covered []string
	for _, a := range req.Airports {
		if Covered(a) {
			covered = append(covered, strings.ToUpper(a))
		} else {
			outside = append(outside, strings.ToUpper(a))
		}
	}
	if len(req.Airports) > 0 && len(covered) == 0 {
		return nil, fmt.Errorf("the French eAIP covers aerodromes in France (LF…); this station follows %s", strings.Join(outside, ", "))
	}
	req.Airports = covered
	airac := req.AIRAC
	if airac.IsZero() {
		airac = InForce(m.now())
	}
	base := BaseURL(airac)
	folder := filepath.Join(m.CacheDir, airac.Format("2006-01-02"))
	pages := map[string]string{}
	var read, skipped []string
	for i, name := range PageNames(req.Airports) {
		path := filepath.Join(folder, name)
		if _, err := os.Stat(path); err != nil {
			if err := m.fetch(ctx, base+"/"+name, path); err != nil {
				// The two ENR pages are the list; an airport's AD 2 adds to it.
				if i < 2 {
					return nil, fmt.Errorf("%s: %w", name, err)
				}
				skipped = append(skipped, fmt.Sprintf("%s: %v", req.Airports[i-2], err))
				continue
			}
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		pages[name] = string(raw)
		read = append(read, pageLabel(name))
	}
	n := Parse(pages, req.Lat, req.Lon, req.RadiusNM)
	n.Skipped = skipped
	n.Outside = outside
	m.procedurePoints(ctx, &n, base, folder, req.Airports, pages)
	if len(n.Points) == 0 {
		return nil, fmt.Errorf("no point found in %s: the page's layout may have changed", pagePoints)
	}
	n.Source = fmt.Sprintf("SIA (DGAC), eAIP France, AIRAC cycle of %s: %s", airac.Format("2006-01-02"), strings.Join(read, ", "))
	n.SourceURL = base
	n.AIRAC = airac.Format("2006-01-02")
	n.NextAIRAC = airac.AddDate(0, 0, airacDays).Format("2006-01-02")
	n.Made = m.now().Format(time.RFC3339)
	n.Airports = req.Airports

	raw, err := json.MarshalIndent(n, "", " ")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(m.OutPath), 0o755); err != nil {
		return nil, err
	}
	tmp := m.OutPath + ".part"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return nil, err
	}
	return &n, os.Rename(tmp, m.OutPath)
}

// fetch downloads one page into the cache, whole or not at all.
func (m *Maker) fetch(ctx context.Context, url, path string) error {
	client := m.Client
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Clairance (one request per page per AIRAC cycle)")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the SIA answered %s for %s", resp.Status, url)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	// The largest page is about 3 MB; 32 MB bounds a wrong answer.
	if _, err := io.Copy(f, io.LimitReader(resp.Body, 32<<20)); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

var reChart = regexp.MustCompile(`href="(Cartes/(LF[A-Z]{2})/AD_2_LF[A-Z]{2}_DATA_(?:SID|STAR)[^"]*\.pdf)"`)
var reDesignator = regexp.MustCompile(`^\s*([A-Z]{5}) (\d[A-Z])\s*$`)

// procedurePoints reads the SID and STAR coding tables of the followed
// airports (linked from their AD 2 pages) and the procedures the text names,
// and keeps the points they are named after, with their coordinates.
func (m *Maker) procedurePoints(ctx context.Context, n *Names, base, folder string, airports []string, pages map[string]string) {
	designators := map[string]map[string]bool{}
	kinds := map[string]string{}
	add := func(point, designator string) {
		if designators[point] == nil {
			designators[point] = map[string]bool{}
		}
		designators[point][designator] = true
	}
	for _, p := range n.Procedures {
		if f := strings.Fields(p); len(f) == 2 {
			add(f[0], p)
		} else if len(p) == 7 {
			add(p[:5], p[:5]+" "+p[5:])
		}
	}
	tool := pdfToText()
	if tool == "" {
		n.ChartsNote = "pdftotext (poppler) is not installed: only the procedures the AIP text names are known"
	}
	var note RoutesNote
	var routes []Route
	for _, a := range airports {
		page := pages[fmt.Sprintf(pageAirport, a)]
		seen := map[string]bool{}
		for _, mm := range reChart.FindAllStringSubmatch(page, -1) {
			rel := mm[1]
			if seen[rel] || tool == "" {
				continue
			}
			seen[rel] = true
			path := filepath.Join(folder, filepath.FromSlash(rel))
			if _, err := os.Stat(path); err != nil {
				if err := m.fetch(ctx, base+"/"+rel, path); err != nil {
					n.ChartsNote = fmt.Sprintf("some coding tables could not be had (%v)", err)
					continue
				}
			}
			out, err := exec.CommandContext(ctx, tool, "-layout", path, "-").Output()
			if err != nil {
				n.ChartsNote = fmt.Sprintf("a coding table could not be read (%v)", err)
				continue
			}
			n.ChartsRead++
			kind := "STAR"
			if strings.Contains(rel, "_DATA_SID_") {
				kind = "SID"
			}
			for _, line := range strings.Split(string(out), "\n") {
				if d := reDesignator.FindStringSubmatch(line); d != nil {
					add(d[1], d[1]+" "+d[2])
					kinds[d[1]+" "+d[2]] = kind
				}
			}
			table := strings.TrimSuffix(rel[strings.LastIndex(rel, "/")+1:], ".pdf")
			routes = append(routes, parseCodingTable(string(out), a, kind, table, &note)...)
		}
	}
	if len(routes) > 0 {
		m.placeRoutes(ctx, n, routes, &note, base, folder, airports, pages, tool)
	}
	for name, ds := range designators {
		p, ok := n.allPoints[name]
		if !ok {
			continue // a name the points table does not hold: no place to show
		}
		pp := ProcedurePoint{Name: name, Lat: math.Round(p.Lat*1e5) / 1e5, Lon: math.Round(p.Lon*1e5) / 1e5}
		for d := range ds {
			pp.Designators = append(pp.Designators, d)
			if k := kinds[d]; k != "" {
				if pp.Kinds == nil {
					pp.Kinds = map[string]string{}
				}
				pp.Kinds[d] = k
			}
		}
		sort.Strings(pp.Designators)
		n.ProcedurePoints = append(n.ProcedurePoints, pp)
	}
	sort.Slice(n.ProcedurePoints, func(i, j int) bool { return n.ProcedurePoints[i].Name < n.ProcedurePoints[j].Name })
}

// pdfToText finds poppler's pdftotext: on the PATH, or where Homebrew puts it
// (a launcher's PATH may not hold it; pkg/tool).
func pdfToText() string {
	p, ok := tool.Find("pdftotext")
	if !ok {
		return ""
	}
	return p
}
