package piezotone

import (
	"bytes"
	"math"
	"testing"
)

// checkMelody fails unless notes have no gaps, positive lengths and MIDI
// numbers in 0 to 127 or Rest.
func checkMelody(t *testing.T, notes []Note) {
	t.Helper()
	var end int64
	for i, n := range notes {
		if n.Start.Nanoseconds() != end || n.Duration <= 0 || n.MIDI != Rest && (n.MIDI < 0 || n.MIDI > 127) {
			t.Fatalf("note %d is bad: %+v (previous note ends at %d ns)", i, n, end)
		}
		end += n.Duration.Nanoseconds()
	}
}

func FuzzFromMIDI(f *testing.F) {
	s := testSMF(&testing.T{})
	f.Add(s)
	f.Add(s[:len(s)/2])
	f.Add([]byte("MThd\x00\x00\x00\x06\x00\x00\x00\x01\x00\x60MTrk\x00\x00\x00\x04\x00\xff\x2f\x00"))
	f.Fuzz(func(t *testing.T, b []byte) {
		notes, err := FromMIDI(bytes.NewReader(b), MIDIOptions{Track: -1, Channel: -1})
		if err == nil {
			if len(notes) == 0 {
				t.Fatal("no notes and no error")
			}
			checkMelody(t, notes)
		}
	})
}

func FuzzDecode(f *testing.F) {
	tone := make([]float32, 4410)
	for i := range tone {
		tone[i] = float32(math.Sin(2 * math.Pi * 440 * float64(i) / 44100))
	}
	f.Add(writeWAV(tone, 44100, 1, 16, 1, false))
	f.Add(writeWAV(tone, 8000, 2, 24, 1, true))
	f.Add(writeWAV(tone, 22050, 1, 32, 3, false))
	f.Add(encodeMP3(&testing.T{}, tone, 44100))
	f.Add([]byte("fLaC"))
	f.Add([]byte("OggS"))
	f.Fuzz(func(t *testing.T, b []byte) {
		s, rate, err := Decode(b)
		if err != nil {
			return
		}
		if rate <= 0 {
			t.Fatalf("rate %d with no error", rate)
		}
		if len(s) > 1<<13 {
			return // keep each run fast
		}
		notes, err := FromPCM(s, rate, PCMOptions{Drums: len(b)%2 == 0})
		if err == nil {
			checkMelody(t, notes)
		}
	})
}

func FuzzLameTag(f *testing.F) {
	tag := append(make([]byte, 21), "Info\x00\x00\x00\x0f"...) // mono MPEG-1: the tag starts at byte 21
	tag = append(append(tag, make([]byte, 112)...), "Lavc63.24\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x24\x04\xa4"...)
	f.Add(tag)
	f.Fuzz(func(t *testing.T, frame []byte) {
		if skip, pad, _ := lameTag(frame); skip < 0 || pad < 0 || skip > 4095+529 || pad > 4095 {
			t.Fatalf("skip %d, pad %d", skip, pad)
		}
	})
}
