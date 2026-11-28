package opendisplay

import (
	"testing"
	"time"

	"github.com/cr2007/piezotone"
)

func FuzzFit(f *testing.F) {
	f.Add([]byte{69, 0, 200, 128, 0, 50, 69, 0, 7}, 0)
	f.Add([]byte{0, 255, 255, 127, 0, 1}, 3)
	f.Fuzz(func(t *testing.T, b []byte, maxNotes int) {
		var notes []piezotone.Note
		var at time.Duration
		for ; len(b) >= 3; b = b[3:] {
			m := int(b[0] % 129)
			if m == 128 {
				m = piezotone.Rest
			}
			d := time.Duration(int(b[1])<<8|int(b[2])) * time.Millisecond
			notes = append(notes, piezotone.Note{MIDI: m, Start: at, Duration: d})
			at += d
		}
		steps, _ := Fit(notes, FitOptions{MaxNotes: maxNotes % 200})
		if len(steps) > 0 && (steps[0].Rest || steps[len(steps)-1].Rest) {
			t.Fatalf("rest at an end: %v", steps)
		}
		checkPlayable(t, Melody(steps), steps)
	})
}
