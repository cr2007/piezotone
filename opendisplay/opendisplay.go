// Package opendisplay fits notes to the buzzer of an OpenDisplay device and
// writes the melody notation that "opendisplay buzz -melody" of
// github.com/cr2007/go-opendisplay accepts.
package opendisplay

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/cr2007/piezotone"
)

// Buzzer limits of OpenDisplay firmware 2.x. Source: go-opendisplay
// docs/research.md section 5 (frequency table 5.2, duration 5.3, steps per
// frame 5.4 and 5.9, waveform 5.7), from firmware src/buzzer_control.cpp.
const (
	MinIndex, MaxIndex = 117, 234 // 403 Hz to 11840 Hz
	UnitMS             = 5        // time unit of one step
	MaxNoteMS          = 1275     // 255 units
	MaxPlayMS          = 30000    // the firmware stops one play after 30 s
	MaxNotesSealed     = 104      // steps in one frame of an encrypted session
)

// Step is one buzzer step. Index is the firmware frequency index:
// 13.75 Hz * 2^(Index/24), 24 steps per octave (50 cents). A rest has no pitch.
type Step struct {
	Index int
	Rest  bool
	MS    int
}

// IndexHz is the frequency of a firmware index.
func IndexHz(i int) float64 { return 13.75 * math.Pow(2, float64(i)/24) }

// HzIndex is the nearest firmware index to hz.
func HzIndex(hz float64) int { return int(math.Round(24 * math.Log2(hz/13.75))) }

// MIDIIndex is the firmware index of a MIDI note number: MIDI 69 (A4) is index 120.
func MIDIIndex(m int) int { return 2*m - 18 }

var noteNames = [12]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

// Melody writes notes as NAME:MS tokens, for example "A4:200 R:50 G4+:5".
// "+" marks a quarter tone above the note.
func Melody(notes []Step) string {
	tok := make([]string, len(notes))
	for i, n := range notes {
		tok[i] = noteName(n) + ":" + strconv.Itoa(n.MS)
	}
	return strings.Join(tok, " ")
}

func noteName(n Step) string {
	switch {
	case n.Rest:
		return "R"
	case n.Index < 6: // below C0: the parser also takes a raw index
		return strconv.Itoa(n.Index)
	}
	r := n.Index - 6 // index 6 is C0
	s := noteNames[r%24/2] + strconv.Itoa(r/24)
	if r%2 == 1 {
		s += "+"
	}
	return s
}

// FitOptions sets the limits of Fit. Zero values use MaxNotesSealed and MaxPlayMS.
type FitOptions struct {
	MaxNotes int
	MaxMS    int
}

// Fit makes notes playable on the buzzer. It moves the melody by whole
// octaves so that most notes are in range and folds the other notes into
// range. It puts the note boundaries on the 5 ms grid, merges rests, removes
// rests at the start and the end, and puts a 5 ms rest between two equal
// notes so that both sound. It splits notes longer than 1275 ms and cuts the
// melody at MaxNotes or MaxMS. The warnings tell what Fit changed or dropped.
func Fit(notes []piezotone.Note, o FitOptions) ([]Step, []string) {
	steps := make([]Step, len(notes))
	for i, n := range notes {
		steps[i] = Step{Rest: true, MS: int(n.Duration.Milliseconds())}
		if n.MIDI != piezotone.Rest {
			steps[i] = Step{Index: MIDIIndex(n.MIDI), MS: steps[i].MS}
		}
	}
	return fit(steps, o)
}

func fit(notes []Step, o FitOptions) ([]Step, []string) {
	if o.MaxNotes <= 0 {
		o.MaxNotes = MaxNotesSealed
	}
	if o.MaxMS <= 0 {
		o.MaxMS = MaxPlayMS
	}
	var warns []string

	// Pitch: one octave shift for the melody, then fold what is still out.
	best, bestIn := 0, -1
	for _, k := range []int{0, 1, -1, 2, -2, 3, -3, 4, -4, 5, -5, 6, -6} {
		in := 0
		for _, n := range notes {
			if i := n.Index + 24*k; !n.Rest && i >= MinIndex && i <= MaxIndex {
				in++
			}
		}
		if in > bestIn {
			best, bestIn = k, in
		}
	}
	if best != 0 {
		dir := "up"
		if best < 0 {
			dir = "down"
		}
		warns = append(warns, fmt.Sprintf("moved the melody %s by %d octave(s) into 400 to 12000 Hz", dir, abs(best)))
	}
	folded := 0
	var out []Step
	for _, n := range notes {
		if !n.Rest {
			n.Index += 24 * best
			shifted := n.Index
			for ; n.Index < MinIndex; n.Index += 24 {
			}
			for ; n.Index > MaxIndex; n.Index -= 24 {
			}
			if n.Index != shifted {
				folded++
			}
		}
		out = append(out, n)
	}
	if folded > 0 {
		warns = append(warns, fmt.Sprintf("moved %d note(s) by octaves into 400 to 12000 Hz", folded))
	}

	// Time: round each boundary to the grid, so that errors do not add up.
	t, end, short := 0, 0, 0
	var q []Step
	for _, n := range out {
		t += n.MS
		e := int(math.Round(float64(t)/UnitMS)) * UnitMS
		n.MS, end = e-end, e
		switch {
		case n.MS == 0:
			if !n.Rest {
				short++
			}
		case len(q) > 0 && n.Rest && q[len(q)-1].Rest:
			q[len(q)-1].MS += n.MS
		case len(q) > 0 && !n.Rest && !q[len(q)-1].Rest && q[len(q)-1].Index == n.Index && q[len(q)-1].MS > UnitMS:
			q[len(q)-1].MS -= UnitMS
			q = append(q, Step{Rest: true, MS: UnitMS}, n)
		default:
			q = append(q, n)
		}
	}
	if short > 0 {
		warns = append(warns, fmt.Sprintf("dropped %d note(s) shorter than %d ms", short, UnitMS))
	}
	q = trimRests(q)

	// Split long notes, then cut at the limits.
	var res []Step
	total, cutNotes, cutMS := 0, 0, 0
	for _, n := range q {
		for n.MS > 0 {
			piece := n
			piece.MS = min(n.MS, MaxNoteMS)
			n.MS -= piece.MS
			if len(res) >= o.MaxNotes || total >= o.MaxMS {
				cutNotes++
				cutMS += piece.MS
				continue
			}
			if total+piece.MS > o.MaxMS {
				cutMS += total + piece.MS - o.MaxMS
				piece.MS = o.MaxMS - total
			}
			total += piece.MS
			res = append(res, piece)
		}
	}
	if cutMS > 0 {
		warns = append(warns, fmt.Sprintf("dropped the last %.1f s (%d note(s)): one play holds at most %d notes and %d s",
			float64(cutMS)/1000, cutNotes, o.MaxNotes, o.MaxMS/1000))
	}
	return trimRests(res), warns
}

func trimRests(n []Step) []Step {
	for len(n) > 0 && n[0].Rest {
		n = n[1:]
	}
	for len(n) > 0 && n[len(n)-1].Rest {
		n = n[:len(n)-1]
	}
	return n
}

func abs(x int) int { return max(x, -x) }
