package mp3

import (
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/yegors/co-atc/third_party/goxiph/audio"
	"github.com/yegors/co-atc/third_party/goxiph/id3"
	"github.com/yegors/co-atc/third_party/goxiph/internal/bitio"
	"github.com/yegors/co-atc/third_party/goxiph/vorbiscomment"
)

// Decoder turns an MP3 stream into samples. All three layers are read.
//
// It holds the state a layer III stream carries between frames: the bit reservoir the main data is
// read out of, the previous granule's scale factors, the overlap each subband's transform left
// behind, and the filterbank's delay line. None of it can be rebuilt part way through, which is why
// the first frames after a seek are not playable. Layers I and II carry only the delay line.
type Decoder struct {
	stream *Stream
	live   *FrameReader // Clairance: set by NewLiveDecoder (live.go), in place of stream
	res    reservoir

	channels int
	rate     int

	previous [2]scaleFactors
	overlap  [2][576]float32
	synth    [2]synthesiser

	// spectra is one granule of each channel, reused. slots is the same for layers I and II, which
	// have no transform and hand the filterbank their subbands directly.
	spectra [2][576]float32
	slots   [2][36][32]float64

	// skip is how many samples of the encoder's own priming are still to be thrown away, and limit
	// how many samples the stream holds in all. Both come from the tag, and without one there is
	// nothing to trim: the priming stays and the file plays a fraction of a second long.
	skip    int
	limit   int64
	emitted int64

	// scratch holds a frame's samples before trimming.
	scratch *audio.Buffer
}

// NewDecoder reads a stream's first frame and prepares to decode it.
func NewDecoder(r io.Reader) (*Decoder, error) {
	s, err := NewStream(r)
	if err != nil {
		return nil, err
	}
	h := s.Header()
	d := &Decoder{stream: s, channels: h.Channels(), rate: h.SampleRate}
	if tag, ok := s.Tag(); ok {
		d.skip = tag.Skip()
		d.limit = tag.Samples(h.Samples())
	}
	scratch, err := audio.NewBuffer(d.Format(), 0)
	if err != nil {
		return nil, err
	}
	d.scratch = scratch
	return d, nil
}

// Format describes the samples Next produces.
func (d *Decoder) Format() audio.Format {
	return audio.Format{Channels: d.channels, SampleRate: d.rate}
}

// Stream is the underlying stream, for its tag and its length.
func (d *Decoder) Stream() *Stream { return d.stream }

// Next decodes one frame into out, and returns io.EOF at the end of the stream.
//
// A frame whose main data begins before the stream does is skipped rather than refused: that is what
// the first frames of a file say, and what every frame after a seek says.
func (d *Decoder) Next(out *audio.Buffer) error {
	for {
		fr, err := d.nextFrame() // Clairance: d.stream.Next(), or the live reader (live.go)
		if err != nil {
			out.Resize(0)
			return err
		}
		if fr.Header.Layer != Layer3 {
			if err := d.layer12(fr); err != nil {
				out.Resize(0)
				return err
			}
			if d.trim(out) {
				return nil
			}
			continue
		}

		si, err := ParseSideInfo(fr.Header, fr.Data)
		if err != nil {
			continue
		}
		main := fr.Data[sideInfoSize(fr.Header):]
		r, err := d.res.open(main, si.MainDataBegin)
		if err != nil {
			continue
		}

		h := fr.Header
		grs := granules(h.Version)
		d.scratch.Format = d.Format()
		d.scratch.Resize(grs * 576)

		for g := range grs {
			if err := d.granule(r, h, si, g, d.scratch, g*576); err != nil {
				out.Resize(0)
				return err
			}
		}
		if d.trim(out) {
			return nil
		}
		// The whole frame was priming or padding, so there is nothing to hand over yet.
	}
}

// trim moves a frame's samples into out, less the priming at the head of the stream and the padding
// at its tail, and reports whether anything survived.
//
// A stream has no granule position and no length of its own, so the tag is the only thing that says
// where the audio truly starts and stops. Without one nothing is trimmed and the file plays a
// fraction of a second longer than it should, which is what makes MP3 famous for gaps between tracks.
func (d *Decoder) trim(out *audio.Buffer) bool {
	frames := d.scratch.Frames()
	from := 0
	if d.skip > 0 {
		from = min(d.skip, frames)
		d.skip -= from
	}
	to := frames
	if d.limit > 0 {
		if left := d.limit - d.emitted; left < int64(to-from) {
			to = from + int(max(left, 0))
		}
	}
	if to <= from {
		out.Resize(0)
		return false
	}

	out.Format = d.Format()
	out.Resize(to - from)
	for ch := range d.channels {
		copy(out.Data[ch], d.scratch.Data[ch][from:to])
	}
	d.emitted += int64(to - from)
	return true
}

// Duration is how long the stream plays.
func (d *Decoder) Duration() time.Duration {
	if d.stream == nil { // Clairance: a live decoder has no stream
		return 0
	}
	return d.stream.Duration()
}

// Comments are the stream's tags, in the shape the rest of this module uses.
//
// MP3 carries no tags of its own. What a file has is an ID3 or APE tag wrapped around the audio, read
// by the id3 package and named here the way a Vorbis comment would be, so that a caller handling
// several formats has one thing to read. Tags on the stream gives the same tag unflattened.
func (d *Decoder) Comments() vorbiscomment.Tags {
	t := d.Tags()
	if t.Empty() {
		return vorbiscomment.Tags{}
	}
	out := vorbiscomment.Tags{Vendor: t.Version}
	for _, f := range []struct{ name, value string }{
		{"TITLE", t.Title}, {"ARTIST", t.Artist}, {"ALBUM", t.Album},
		{"ALBUMARTIST", t.AlbumArtist}, {"COMPOSER", t.Composer},
		{"GENRE", t.Genre}, {"DATE", t.Date}, {"COMMENT", t.Comment},
	} {
		if f.value != "" {
			out.Add(f.name, f.value)
		}
	}
	if t.Track != 0 {
		out.Add("TRACKNUMBER", strconv.Itoa(t.Track))
	}
	if t.Disc != 0 {
		out.Add("DISCNUMBER", strconv.Itoa(t.Disc))
	}
	return out
}

// Tags are what the file says about itself, unflattened.
func (d *Decoder) Tags() id3.Tags {
	if d.stream == nil { // Clairance: a live decoder has no stream
		return id3.Tags{}
	}
	return d.stream.Tags()
}

// Duration is how long an MP3 stream plays, without decoding it.
//
// From the tag where there is one, which is exact; otherwise from the bitrate and the bytes, which is
// right for a constant bitrate stream and an estimate for a variable one. The reader is left where it
// started.
func Duration(rs io.ReadSeeker) (time.Duration, error) {
	start, err := rs.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	defer func() { _, _ = rs.Seek(start, io.SeekStart) }()

	s, err := NewStream(rs)
	if err != nil {
		return 0, err
	}
	return s.Duration(), nil
}

// granule decodes one granule of every channel and writes its samples into out at the given offset.
func (d *Decoder) granule(r *bitio.MSBReader, h Header, si SideInfo, g int, out *audio.Buffer, at int) error {
	midSide := h.Mode == JointStereo && h.ModeExtension&2 != 0
	channels := h.Channels()

	var sfs [2]scaleFactors
	for ch := range channels {
		gi := si.Granule[g][ch]
		start := r.BitsRead()
		sfs[ch] = readScaleFactors(r, h, gi, ch, &d.previous[ch])
		d.previous[ch] = sfs[ch]

		sp, _, err := readSpectrum(r, h, gi, start+gi.Part23Length)
		if err != nil && !errors.Is(err, errTruncated) {
			return err
		}
		requantise(&sp, &sfs[ch], layoutFor(h, gi), gi, midSide && channels == 2, &d.spectra[ch])

		// The granule ends where its length said, whatever was read of it.
		for r.BitsRead() < start+gi.Part23Length {
			if _, err := r.Read(min(uint(start+gi.Part23Length-r.BitsRead()), 32)); err != nil {
				return err
			}
		}
	}

	if channels == 2 {
		gi := si.Granule[g][0]
		applyStereo(&d.spectra[0], &d.spectra[1], h, gi, &sfs[1],
			layoutFor(h, gi), si.Granule[g][1].ScalefacCompress)
	}

	// Once the couplings are undone the channels share nothing: their own spectrum, overlap,
	// filterbank history and output. A stereo pair is therefore transformed on two goroutines, which
	// is worth the hand-off because the transform and the filterbank are most of a granule's cost.
	// The channels share nothing from here — their own spectrum, overlap, filterbank history and
	// output — so a stereo granule could run on two goroutines. Measured, it does not pay: the two
	// arrangements interleaved in one process came to 401 and 404 ms, and the fan-out cost five
	// thousand allocations a file. A granule is a hundred and fifty microseconds of work and the
	// hand-off is a fair share of that.
	for ch := range channels {
		d.channel(h, si.Granule[g][ch], ch, out, at)
	}
	return nil
}

// channel turns one granule of one channel into samples: the transform, then the filterbank.
func (d *Decoder) channel(h Header, gi GranuleInfo, ch int, out *audio.Buffer, at int) {
	long := longBandsOf(h, gi)
	bands := 31
	if gi.BlockType == BlockShort {
		reorder(d.spectra[ch][:], layoutFor(h, gi), long*18)
		bands = long - 1
	}
	if bands > 0 {
		antialias(&d.spectra[ch], bands)
	}
	imdctGranule(&d.spectra[ch], &d.overlap[ch], gi, long)
	invertFrequencies(&d.spectra[ch])

	if dbgSubbands != nil {
		dbgSubbands(d.spectra[ch][:])
	}
	// Eighteen time slots of thirty-two subbands, each becoming thirty-two samples.
	var sub, pcm [32]float64
	for slot := range 18 {
		for b := range 32 {
			sub[b] = float64(d.spectra[ch][b*18+slot])
		}
		d.synth[ch].synthesise(&sub, &pcm)
		for i := range 32 {
			out.Data[ch][at+slot*32+i] = float32(pcm[i])
		}
	}
}

// dbgSubbands is a hook for comparing the decoder against the reference at the filterbank.
var dbgSubbands func([]float32)
