# goxiph, Clairance's copy

A pure-Go MP3 decoder, copied here so that Clairance reads its radio streams
without ffmpeg (docs-fr/35-plan.md, step B1; question Q60 in
docs-fr/05-decisions.md). Kept frozen: changes upstream are taken by hand,
after the tests below pass again.

- **Origin**: https://github.com/faide/goxiph, commit `da6c86de389d`
  (module version `v0.0.0-20260823211529-da6c86de389d`, 23 August 2026).
- **Licence**: BSD 3-Clause, `LICENSE` and `NOTICE` copied unchanged.
- **Copied**: the packages `mp3`, `audio`, `id3`, `vorbiscomment` and
  `internal/bitio`, which `mp3` needs; nothing else (no Ogg, Vorbis, Opus,
  FLAC, WAVE, no command or test tool). Import paths rewritten to this
  directory.

## What was changed

- `mp3/live.go` (new): `FrameReader` and `NewLiveDecoder`, for a live stream.
  Upstream's `Scanner` reads its whole input before the first frame, which a
  radio stream with no end never lets it finish. `FrameReader` reads one frame
  at a time, syncs on a corroborated header, resyncs after damage, and says
  `ErrFormatChanged` when frames of another kind follow. `OnFrame` hands each
  whole frame on as read, for relaying the stream's own MP3 to browsers.
- `mp3/decode.go`: four lines, each marked `Clairance:` -- the `live` field,
  the frame source in `Next`, and `Duration` and `Tags` on a decoder without a
  file.
- Four benchmarks: `for b.Loop()` became `for range b.N`, as this module
  declares an older Go than `testing.Loop` needs.

## What was measured (26/10/2026)

- On the station's own streams (MPEG-2.5, 8 kHz, mono, variable bitrate),
  five minutes of each of two frequencies: identical to ffmpeg to within
  rounding, about 100 dB signal-to-noise in every 5-second window.
- On the synthetic recordings in `mp3/testdata/clairance` (made with ffmpeg
  and LAME, each with ffmpeg's decoding beside it): 105 dB on every mono one.
- **Known gap, stereo**: on joint stereo the left channel is 52 dB from
  ffmpeg, LAME and Apple's decoder, which agree among themselves at 79 to
  85 dB; the right channel is at 82 to 88 dB. Inaudible, and the station is
  mono, but a defect of this decoder; `TestLiveDecodesLikeFFmpeg` holds the
  level so that it cannot get worse unseen.

## The tests (`mp3/live_test.go`)

Equivalence with ffmpeg; the live reader against the whole-file reader, sample
for sample; a stream joined at any byte; bytes flipped, zeroed, inserted or
dropped mid-stream; a stream cut inside a frame; what is not MP3 (random
bytes, an HTML page, nothing); a change of format; any chunking of the
network reads; an hour of stream in flat memory; and `FuzzLiveDecoder`.
