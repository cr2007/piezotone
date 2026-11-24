package opendisplay

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cr2007/piezotone"
)

// parseMelody reads NAME:MS tokens with the note name rules of
// payload.NoteToIndex in go-opendisplay: a letter, an optional "#" or "b",
// an octave, and an optional "+" for a quarter tone above. Index 6 is C0.
func parseMelody(m string) ([]Step, error) {
	semis := map[byte]int{'C': 0, 'D': 2, 'E': 4, 'F': 5, 'G': 7, 'A': 9, 'B': 11}
	var out []Step
	for _, tok := range strings.Fields(m) {
		name, msText, ok := strings.Cut(tok, ":")
		ms, err := strconv.Atoi(msText)
		if !ok || err != nil || ms < UnitMS || ms > MaxNoteMS || ms%UnitMS != 0 {
			return nil, fmt.Errorf("bad length in %q", tok)
		}
		if name == "R" {
			out = append(out, Step{Rest: true, MS: ms})
			continue
		}
		semi, ok := semis[name[0]]
		if !ok {
			return nil, fmt.Errorf("bad note %q", tok)
		}
		rest := name[1:]
		switch {
		case strings.HasPrefix(rest, "#"):
			semi, rest = semi+1, rest[1:]
		case strings.HasPrefix(rest, "b"):
			semi, rest = semi-1, rest[1:]
		}
		quarter := strings.HasSuffix(rest, "+")
		oct, err := strconv.Atoi(strings.TrimSuffix(rest, "+"))
		if err != nil {
			return nil, fmt.Errorf("bad octave in %q", tok)
		}
		idx := 6 + 24*oct + 2*semi
		if quarter {
			idx++
		}
		out = append(out, Step{Index: idx, MS: ms})
	}
	return out, nil
}

// checkPlayable fails unless m parses, fits the buzzer limits and holds the
// same steps as notes.
func checkPlayable(t *testing.T, m string, notes []Step) {
	t.Helper()
	steps, err := parseMelody(m)
	if err != nil {
		t.Fatalf("parseMelody(%q): %v", m, err)
	}
	if len(steps) > MaxNotesSealed {
		t.Errorf("%d steps, more than %d", len(steps), MaxNotesSealed)
	}
	if !slices.Equal(steps, notes) {
		t.Fatalf("steps %v, want %v", steps, notes)
	}
	total := 0
	for i, s := range steps {
		if !s.Rest && (s.Index < MinIndex || s.Index > MaxIndex) {
			t.Errorf("step %d: index %d outside %d to %d", i, s.Index, MinIndex, MaxIndex)
		}
		total += s.MS
	}
	if total > MaxPlayMS {
		t.Errorf("melody plays %d ms, more than %d", total, MaxPlayMS)
	}
}

func TestMelodyNamesRoundTrip(t *testing.T) {
	for i := MinIndex; i <= MaxIndex; i++ {
		n := []Step{{Index: i, MS: 100}}
		checkPlayable(t, Melody(n), n)
	}
}

func TestMIDIIndex(t *testing.T) {
	for m := 60; m < 128; m++ {
		if got := HzIndex(piezotone.MIDIHz(m)); got != MIDIIndex(m) {
			t.Errorf("MIDI %d: HzIndex %d, MIDIIndex %d", m, got, MIDIIndex(m))
		}
	}
}

func TestFitNotes(t *testing.T) {
	// The lead of the core MIDI test: G5 C5 D5 R E5 E5. The two E5 get a gap.
	var notes []piezotone.Note
	var start time.Duration
	for _, s := range [][2]int{{79, 250}, {72, 250}, {74, 250}, {piezotone.Rest, 250}, {76, 500}, {76, 250}} {
		d := time.Duration(s[1]) * time.Millisecond
		notes = append(notes, piezotone.Note{MIDI: s[0], Start: start, Duration: d})
		start += d
	}
	got, warns := Fit(notes, FitOptions{})
	if m := Melody(got); m != "G5:250 C5:250 D5:250 R:250 E5:495 R:5 E5:250" || len(warns) != 0 {
		t.Errorf("Melody = %q, warnings %q", m, warns)
	}
	// Bass at C3 and G2 moves up three octaves, as in go-opendisplay.
	got, warns = Fit([]piezotone.Note{{MIDI: 48, Duration: 500 * time.Millisecond}, {MIDI: 43, Start: 500 * time.Millisecond, Duration: 500 * time.Millisecond}}, FitOptions{})
	if m := Melody(got); m != "C6:500 G5:500" || len(warns) != 1 {
		t.Errorf("Melody = %q, warnings %q", m, warns)
	}
}

func TestMelodyString(t *testing.T) {
	got := Melody([]Step{{Index: 120, MS: 200}, {Rest: true, MS: 50}, {Index: 117, MS: 5}, {Index: 128, MS: 1275}})
	want := "A4:200 R:50 G4+:5 C#5:1275"
	if got != want {
		t.Errorf("Melody = %q, want %q", got, want)
	}
}

func TestFit(t *testing.T) {
	r := func(ms int) Step { return Step{Rest: true, MS: ms} }
	n := func(i, ms int) Step { return Step{Index: i, MS: ms} }
	tests := []struct {
		name  string
		in    []Step
		o     FitOptions
		want  []Step
		warns int
	}{
		{"unchanged", []Step{n(120, 200), r(100), n(144, 300)}, FitOptions{}, []Step{n(120, 200), r(100), n(144, 300)}, 0},
		{"trim rests at the ends", []Step{r(300), n(120, 200), r(100)}, FitOptions{}, []Step{n(120, 200)}, 0},
		{"melody moves up one octave", []Step{n(96, 100), n(100, 100), n(140, 100)}, FitOptions{}, []Step{n(120, 100), n(124, 100), n(164, 100)}, 1},
		{"one note folds", []Step{n(120, 100), n(124, 100), n(240, 100)}, FitOptions{}, []Step{n(120, 100), n(124, 100), n(216, 100)}, 1},
		// The shift puts the duration-weighted median note in range; one short low note folds
		// instead of moving the whole melody up two more octaves.
		{"a short low note folds", []Step{n(112, 400), n(114, 400), n(116, 400), n(68, 100)}, FitOptions{}, []Step{n(136, 400), n(138, 400), n(140, 395), r(5), n(140, 100)}, 2},
		// Boundaries 0, 12, 13, 31 round to 10, 15, 30 on the grid.
		{"quantize to 5 ms", []Step{n(120, 12), n(122, 1), n(124, 18)}, FitOptions{}, []Step{n(120, 10), n(122, 5), n(124, 15)}, 0},
		{"drop a note shorter than the grid", []Step{n(120, 11), n(122, 1), n(124, 18)}, FitOptions{}, []Step{n(120, 10), n(124, 20)}, 1},
		{"merge rests", []Step{n(120, 100), r(50), r(50), n(122, 100)}, FitOptions{}, []Step{n(120, 100), r(100), n(122, 100)}, 0},
		{"split a long note", []Step{n(120, 3000)}, FitOptions{}, []Step{n(120, 1275), n(120, 1275), n(120, 450)}, 0},
		{"repeated note gets a gap", []Step{n(120, 100), n(120, 100)}, FitOptions{}, []Step{n(120, 95), r(5), n(120, 100)}, 0},
		{"cut at max notes", []Step{n(120, 100), r(100), n(122, 100), n(124, 100)}, FitOptions{MaxNotes: 2}, []Step{n(120, 100)}, 1},
		{"cut at max time", []Step{n(120, 1000), n(122, 1000)}, FitOptions{MaxMS: 1500}, []Step{n(120, 1000), n(122, 500)}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, warns := fit(tc.in, tc.o)
			if !slices.Equal(got, tc.want) {
				t.Errorf("Fit = %v, want %v", got, tc.want)
			}
			if len(warns) != tc.warns {
				t.Errorf("warnings %q, want %d", warns, tc.warns)
			}
			checkPlayable(t, Melody(got), got)
		})
	}
}

func TestFitLimits(t *testing.T) {
	// 300 notes of 150 ms: 45 s, too long and too many.
	var in []Step
	for i := range 300 {
		in = append(in, Step{Index: 60 + i%200, MS: 150})
	}
	got, warns := fit(in, FitOptions{})
	if len(got) != MaxNotesSealed || len(warns) == 0 {
		t.Errorf("%d notes, warnings %q", len(got), warns)
	}
	checkPlayable(t, Melody(got), got)
}
