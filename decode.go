package piezotone

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/jfreymuth/oggvorbis"
	"github.com/mewkiz/flac"
	mp3 "github.com/tphakala/go-mp3"
)

// IsMIDI reports whether b starts like a standard MIDI file.
func IsMIDI(b []byte) bool { return bytes.HasPrefix(b, []byte("MThd")) }

// Decode reads a WAV, MP3, FLAC or Ogg Vorbis file. It finds the format from
// the first bytes, mixes the channels down to mono and returns the samples
// and the sample rate. Integer PCM is in [-1, 1]; float WAV and lossy data
// can go past it.
func Decode(b []byte) ([]float32, int, error) {
	switch {
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WAVE":
		return ReadWAV(bytes.NewReader(b))
	case bytes.HasPrefix(b, []byte("fLaC")):
		return decodeFLAC(b)
	case bytes.HasPrefix(b, []byte("OggS")):
		s, f, err := oggvorbis.ReadAll(bytes.NewReader(b))
		if err != nil {
			return nil, 0, fmt.Errorf("piezotone: Ogg Vorbis: %w", err)
		}
		return mono(s, f.Channels), f.SampleRate, nil
	case bytes.HasPrefix(b, []byte("ID3")), len(b) >= 2 && b[0] == 0xFF && b[1]&0xE0 == 0xE0:
		return decodeMP3(b)
	}
	return nil, 0, errors.New("piezotone: unknown file format; use MIDI, WAV, MP3, FLAC or Ogg Vorbis")
}

func decodeMP3(b []byte) ([]float32, int, error) {
	// An ID3v1 tag is the last 128 bytes and starts with "TAG" (id3.org/ID3v1).
	// go-mp3 skips the last frame when the tag follows it.
	if len(b) >= 128 && string(b[len(b)-128:len(b)-125]) == "TAG" {
		b = b[:len(b)-128]
	}
	d := mp3.NewDecoder()
	pcm := make([]float32, 1152*2)
	var out []float32
	rate, skip, pad := 0, 0, 0
	for pos := 0; pos < len(b); {
		n, info, err := d.DecodeFrame(b[pos:], pcm)
		if err != nil && !errors.Is(err, mp3.ErrUnsupported) {
			return nil, 0, fmt.Errorf("piezotone: MP3: %w", err)
		}
		if info.FrameBytes == 0 {
			break
		}
		frame := b[pos+info.FrameOffset : pos+info.FrameBytes]
		pos += info.FrameBytes
		if n == 0 {
			continue
		}
		first := rate == 0
		rate = info.SampleRate
		if first {
			var tag bool
			if skip, pad, tag = lameTag(frame); tag {
				continue // the tag frame holds no audio
			}
		}
		out = append(out, mono(pcm[:n*info.Channels], info.Channels)...)
	}
	if rate == 0 {
		return nil, 0, errors.New("piezotone: MP3: no Layer III frames")
	}
	out = out[min(skip, len(out)):]
	return out[:len(out)-min(pad, len(out))], rate, nil
}

// lameTag reads the Xing or Info frame at the start of an MP3. When it has a
// LAME tag, skip and pad are the samples to cut at the start and the end, as
// in ffmpeg: the encoder delay plus the 529-sample decoder delay, and the
// padding minus it. Source: the LAME Mp3 Info Tag rev 1 specification,
// http://gabriel.mp3-tech.org/mp3infotag.html
func lameTag(frame []byte) (skip, pad int, ok bool) {
	i := bytes.Index(frame[:min(len(frame), 4+2+32+4)], []byte("Xing")) // after the header, CRC and side info
	if i < 0 {
		i = bytes.Index(frame[:min(len(frame), 4+2+32+4)], []byte("Info"))
	}
	if i < 0 || len(frame) < i+8 {
		return 0, 0, false
	}
	flags, off := binary.BigEndian.Uint32(frame[i+4:]), i+8
	for bit, size := range []int{4, 4, 100, 4} { // frames, bytes, TOC, quality
		if flags&(1<<bit) != 0 {
			off += size
		}
	}
	if len(frame) < off+24 {
		return 0, 0, true
	}
	if v := string(frame[off : off+4]); v != "LAME" && v != "Lavf" && v != "Lavc" {
		return 0, 0, true
	}
	x := frame[off+21:]
	delay, padding := int(x[0])<<4|int(x[1])>>4, int(x[1]&0x0F)<<8|int(x[2])
	return delay + 529, max(0, padding-529), true
}

func decodeFLAC(b []byte) ([]float32, int, error) {
	st, err := flac.New(bytes.NewReader(b))
	if err != nil {
		return nil, 0, fmt.Errorf("piezotone: FLAC: %w", err)
	}
	if st.Info.BitsPerSample < 4 || st.Info.BitsPerSample > 32 {
		return nil, 0, fmt.Errorf("piezotone: FLAC: %d bits per sample", st.Info.BitsPerSample)
	}
	scale := 1 / float32(int64(1)<<(st.Info.BitsPerSample-1))
	var out []float32
	for {
		f, err := st.ParseNext()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, fmt.Errorf("piezotone: FLAC: %w", err)
		}
		for i := range f.Subframes[0].Samples {
			var sum int64
			for _, sub := range f.Subframes {
				sum += int64(sub.Samples[i])
			}
			out = append(out, float32(sum)*scale/float32(len(f.Subframes)))
		}
	}
	return out, int(st.Info.SampleRate), nil
}
