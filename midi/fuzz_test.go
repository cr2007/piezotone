package midi

import (
	"bytes"
	"slices"
	"testing"
	"time"

	"github.com/cr2007/piezotone"
)

// FuzzWriteRoundTrip writes notes and reads them back with FromMIDI. Rests at the
// ends do not come back, so the check trims them first.
func FuzzWriteRoundTrip(f *testing.F) {
	f.Add([]byte{69, 0, 200, 128, 0, 50, 69, 0, 7})
	f.Fuzz(func(t *testing.T, b []byte) {
		var in []piezotone.Note
		for ; len(b) >= 3; b = b[3:] {
			m := int(b[0] % 129)
			if m == 128 {
				m = piezotone.Rest
			}
			if ms := int(b[1])<<8 | int(b[2]); ms > 0 {
				in = append(in, piezotone.Note{MIDI: m, Duration: time.Duration(ms) * time.Millisecond})
			}
		}
		var w bytes.Buffer
		if err := Write(&w, in); err != nil {
			t.Fatal(err)
		}
		for len(in) > 0 && in[0].MIDI == piezotone.Rest {
			in = in[1:]
		}
		for len(in) > 0 && in[len(in)-1].MIDI == piezotone.Rest {
			in = in[:len(in)-1]
		}
		var want []piezotone.Note // rests in a row join
		var at time.Duration
		for _, n := range in {
			if len(want) > 0 && n.MIDI == piezotone.Rest && want[len(want)-1].MIDI == piezotone.Rest {
				want[len(want)-1].Duration += n.Duration
			} else {
				want = append(want, piezotone.Note{MIDI: n.MIDI, Start: at, Duration: n.Duration})
			}
			at += n.Duration
		}
		got, err := piezotone.FromMIDI(&w, piezotone.MIDIOptions{Track: -1, Channel: -1})
		if len(want) == 0 {
			if err == nil {
				t.Fatalf("no notes written, read %v", got)
			}
			return
		}
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("read back %v, %v; want %v", got, err, want)
		}
	})
}
