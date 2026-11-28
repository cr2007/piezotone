package piezotone

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	mp3 "github.com/tphakala/go-mp3"
)

// encodeMP3 encodes mono samples with the go-mp3 encoder at 128 kb/s.
func encodeMP3(t *testing.T, s []float32, rate int) []byte {
	t.Helper()
	e, err := mp3.NewEncoder(mp3.EncoderConfig{SampleRate: rate, Channels: 1})
	if err != nil {
		t.Fatal(err)
	}
	var out []byte
	for i := 0; i < len(s); i += mp3.FrameSize {
		if out, err = e.EncodeFrame(out, [][]float32{s[i:min(len(s), i+mp3.FrameSize)]}); err != nil {
			t.Fatal(err)
		}
	}
	if out, err = e.EncodeFrame(out, nil); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDecodeMP3(t *testing.T) {
	ref := refNotes(t)
	for _, rate := range []int{44100, 48000} {
		samples, got, err := Decode(encodeMP3(t, synth(ref, rate, true), rate))
		if err != nil || got != rate {
			t.Fatalf("Decode: %d Hz, %v", got, err)
		}
		checkTranscription(t, ref, samples, rate)
	}
}

// TestDecodeFFmpeg checks the FLAC, Ogg Vorbis and MP3 decoders on files
// from other encoders. It needs ffmpeg with libmp3lame.
func TestDecodeFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	ref := refNotes(t)
	dir := t.TempDir()
	wav := filepath.Join(dir, "in.wav")
	// Stereo 24-bit input, so that the FLAC decoder must undo channel decorrelation.
	s := synth(ref, 44100, false)
	st := make([]float32, 0, 2*len(s))
	for _, v := range s {
		st = append(st, v, v)
	}
	if err := os.WriteFile(wav, writeWAV(st, 44100, 2, 24, 1, false), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, ext, codec string
		extra            []string
	}{
		{"flac", "flac", "flac", nil}, {"ogg", "ogg", "vorbis", nil}, {"mp3", "mp3", "libmp3lame", nil},
		{"mp3 ID3v1", "mp3", "libmp3lame", []string{"-write_id3v1", "1", "-metadata", "title=x"}}, // a tag after the last frame
	} {
		t.Run(c.name, func(t *testing.T) {
			out := filepath.Join(dir, "out."+c.ext)
			args := append([]string{"-v", "error", "-y", "-i", wav, "-c:a", c.codec, "-strict", "-2"}, c.extra...)
			if b, err := exec.Command("ffmpeg", append(args, out)...).CombinedOutput(); err != nil {
				if strings.Contains(string(b), "Unknown encoder") {
					t.Skipf("ffmpeg: %s", b)
				}
				t.Fatalf("ffmpeg: %v: %s", err, b)
			}
			b, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			samples, rate, err := Decode(b)
			if err != nil || rate != 44100 {
				t.Fatalf("Decode: %d Hz, %v", rate, err)
			}
			checkTranscription(t, ref, samples, rate)
			// The LAME tag gives the MP3 encoder delay and padding, so all three are gapless.
			if len(samples) != len(s) {
				t.Errorf("%d samples, want %d", len(samples), len(s))
			}
		})
	}
}

func TestDecodeUnknown(t *testing.T) {
	if _, _, err := Decode([]byte("hello, world")); err == nil {
		t.Error("no error")
	}
}
