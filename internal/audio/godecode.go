package audio

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/yegors/co-atc/pkg/logger"
	"github.com/yegors/co-atc/third_party/goxiph/audio"
	"github.com/yegors/co-atc/third_party/goxiph/mp3"
)

// Decoding an MP3 stream over HTTP in Go, in place of ffmpeg (docs-fr/35-plan.md,
// B1; Q60): the stream is read, decoded by the copy of goxiph in third_party,
// mixed to mono and brought to the configured rate, and written as the same
// signed 16-bit little-endian PCM ffmpeg writes, so that everything after it
// is unchanged. Nothing outside the binary is needed.

// goStallAfter is how long a stream may send nothing before it is taken as
// dead. The station's streams send frames through silence too (a frame every
// 72 ms), so a quiet minute is a broken connection, not a quiet frequency.
var goStallAfter = 15 * time.Second

// goResamplerTaps is the half-width of the resampler's kernel.
const goResamplerTaps = 32

var goHTTPClient = &http.Client{Transport: &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	ResponseHeaderTimeout: 15 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
}}

// decodeHTTP reads the MP3 stream at url and writes it to w as 16-bit mono PCM
// at rate, until the stream ends, fails, or ctx is done. It returns why it
// stopped; the caller reconnects.
func decodeHTTP(ctx context.Context, url string, rate int, w io.Writer, log *logger.Logger) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Clairance")
	resp, err := goHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("stream answered HTTP %d", resp.StatusCode)
	}

	// A stream that stops sending without closing is cut here, rather than
	// left to hold the decoder for ever.
	body := &stallReader{r: resp.Body}
	stall := time.AfterFunc(goStallAfter, cancel)
	defer stall.Stop()
	body.onRead = func() { stall.Reset(goStallAfter) }

	return decodeMP3(body, rate, w, log)
}

// decodeMP3 decodes a live MP3 stream from r to w, as decodeHTTP describes.
func decodeMP3(r io.Reader, rate int, w io.Writer, log *logger.Logger) error {
	d, fr, err := mp3.NewLiveDecoder(r)
	if err != nil {
		return fmt.Errorf("not an MP3 stream: %w", err)
	}
	format := d.Format()
	log.Info("Decoding the stream in Go",
		Int("stream_rate", format.SampleRate), Int("channels", format.Channels), Int("rate", rate))

	buf, err := audio.NewBuffer(format, 0)
	if err != nil {
		return err
	}
	rs := newResampler(format.SampleRate, rate, goResamplerTaps)
	var mono []float64
	var pcm []int16
	var bytesOut []byte
	resyncs := 0
	for {
		if err := d.Next(buf); err != nil {
			if fr.Resyncs > 0 {
				log.Info("Stream resynced on damaged frames", Int("resyncs", fr.Resyncs))
			}
			if errors.Is(err, io.EOF) {
				return io.ErrUnexpectedEOF // a live stream has no end
			}
			return err
		}
		if fr.Resyncs != resyncs {
			log.Debug("Stream resynced", Int("resyncs", fr.Resyncs))
			resyncs = fr.Resyncs
		}
		n := len(buf.Data[0])
		mono = mono[:0]
		for i := 0; i < n; i++ {
			var v float64
			for c := range buf.Data {
				v += float64(buf.Data[c][i])
			}
			mono = append(mono, v/float64(len(buf.Data)))
		}
		pcm = rs.process(mono, pcm[:0])
		bytesOut = bytesOut[:0]
		for _, s := range pcm {
			bytesOut = binary.LittleEndian.AppendUint16(bytesOut, uint16(s))
		}
		if _, err := w.Write(bytesOut); err != nil {
			return err
		}
	}
}

// stallReader calls onRead after every read that brought bytes.
type stallReader struct {
	r      io.Reader
	onRead func()
}

func (s *stallReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if n > 0 && s.onRead != nil {
		s.onRead()
	}
	return n, err
}

// goDecodes says whether a source is decoded in Go: an HTTP stream, with no
// ffmpeg input options (those describe raw PCM or a capture device), to mono
// 16-bit PCM.
func goDecodes(url string, inputOptions []string, channels int, format string) bool {
	return isHTTP(url) && len(inputOptions) == 0 && channels == 1 && strings.EqualFold(format, "s16le")
}

// startGoDecoder starts decoding in Go in place of ffmpeg: what it writes
// arrives where ffmpeg's output did, so processFFmpegOutput and the
// reconnection that follows a read error are unchanged.
func (p *CentralAudioProcessor) startGoDecoder() error {
	ctx, cancel := context.WithCancel(p.ctx)
	pr, pw := io.Pipe()
	p.goCancel = cancel
	p.ffmpegCmd = nil
	p.ffmpegStdout = pr
	go func() {
		err := decodeHTTP(ctx, p.audioURL, p.sampleRate, pw, p.logger)
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		pw.CloseWithError(err)
	}()
	go p.processFFmpegOutput()
	return nil
}

// stopGoDecoder stops the Go decoder, if it runs.
func (p *CentralAudioProcessor) stopGoDecoder() {
	if p.goCancel == nil {
		return
	}
	p.goCancel()
	p.goCancel = nil
	if p.ffmpegStdout != nil {
		_ = p.ffmpegStdout.Close()
	}
}
