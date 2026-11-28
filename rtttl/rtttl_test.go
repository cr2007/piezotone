package rtttl

import (
	"testing"
	"time"

	"github.com/cr2007/piezotone"
)

// jingleBell is the example of the specification page (see the package doc).
const jingleBell = "JingleBell:d=8,o=5,b=112:32p,a,a,4a,a,a,4a,a,c6,f.,16g,2a,a#,a#,a#.,16a#,a#,a,a.,16a,a,g,g,a,4g,4c6"

// jingleNotes are the notes of jingleBell: MIDI number and length in 32nd notes.
func jingleNotes() []piezotone.Note {
	const r = piezotone.Rest
	steps := [][2]int{{r, 1}, {81, 4}, {81, 4}, {81, 8}, {81, 4}, {81, 4}, {81, 8}, {81, 4}, {84, 4}, {77, 6},
		{79, 2}, {81, 16}, {82, 4}, {82, 4}, {82, 6}, {82, 2}, {82, 4}, {81, 4}, {81, 6}, {81, 2}, {81, 4},
		{79, 4}, {79, 4}, {81, 4}, {79, 8}, {84, 8}}
	whole := 4 * time.Minute / 112
	var out []piezotone.Note
	var t time.Duration
	for _, s := range steps {
		d := whole * time.Duration(s[1]) / 32
		out = append(out, piezotone.Note{MIDI: s[0], Start: t, Duration: d})
		t += d
	}
	return out
}

func TestEncodeSpecExample(t *testing.T) {
	for _, bpm := range []int{112, 0} { // 0: Encode must find 112
		got, warns := Encode(jingleNotes(), Options{Name: "JingleBell", BPM: bpm})
		if got != jingleBell || len(warns) != 0 {
			t.Errorf("BPM %d: Encode =\n%q, want\n%q, warnings %q", bpm, got, jingleBell, warns)
		}
	}
}

func TestEncodeFoldAndName(t *testing.T) {
	// C2 and A7 span more than four octaves: the melody stays and C2 folds up to C4.
	// At 112 BPM, 4 s is a dotted whole note and a dotted quarter.
	notes := []piezotone.Note{{MIDI: 36, Duration: 535 * time.Millisecond}, {MIDI: 105, Start: 535 * time.Millisecond, Duration: 4 * time.Second}}
	got, warns := Encode(notes, Options{Name: "a:very:long name", BPM: 112})
	if want := "averylong :d=4,o=7,b=112:c4,1a.,a."; got != want || len(warns) != 2 {
		t.Errorf("Encode = %q, want %q, warnings %q", got, want, warns)
	}
}

func TestEncodeLongNote(t *testing.T) {
	// RTTTL has no tie. At 112 BPM a dotted whole note is 3.2 s, so a 7 s note
	// becomes two dotted whole notes and a quarter: the tone starts 3 times.
	notes := []piezotone.Note{{MIDI: 81, Duration: 7 * time.Second}}
	if got, _ := Encode(notes, Options{BPM: 112}); got != "piezotone:d=1,o=5,b=112:a.,a.,4a" {
		t.Errorf("Encode = %q", got)
	}
}
