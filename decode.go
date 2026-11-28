package piezotone

import (
	"bytes"
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
	d := mp3.NewDecoder()
	pcm := make([]float32, 1152*2)
	var out []float32
	rate := 0
	for pos := 0; pos < len(b); {
		n, info, err := d.DecodeFrame(b[pos:], pcm)
		if err != nil && !errors.Is(err, mp3.ErrUnsupported) {
			return nil, 0, fmt.Errorf("piezotone: MP3: %w", err)
		}
		if info.FrameBytes == 0 {
			break
		}
		pos += info.FrameBytes
		if n > 0 {
			out = append(out, mono(pcm[:n*info.Channels], info.Channels)...)
			rate = info.SampleRate
		}
	}
	if rate == 0 {
		return nil, 0, errors.New("piezotone: MP3: no Layer III frames")
	}
	return out, rate, nil
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
