// Package rtttl writes notes in the Nokia RTTTL (Ring Tone Text Transfer
// Language) format, for example "JingleBell:d=8,o=5,b=112:32p,a,a,4a".
//
// Source: the RTTTL specification at
// https://www.mobilefish.com/tutorials/rtttl/rtttl_quickguide_specification.html
// It gives the three sections, the durations 1 to 32, the octaves 4 to 7
// (octave 4: A is 440 Hz), the allowed tempos, the dot after the octave and
// a name of at most 10 characters. A beat is a quarter note, as in common
// RTTTL players.
package rtttl

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/cr2007/piezotone"
)

// Tempos are the allowed beats per minute of the specification.
var Tempos = []int{25, 28, 31, 35, 40, 45, 50, 56, 63, 70, 80, 90, 100, 112, 125, 140,
	160, 180, 200, 225, 250, 285, 320, 355, 400, 450, 500, 565, 635, 715, 800, 900}

// MinOctave and MaxOctave are the octave range of the specification.
const MinOctave, MaxOctave = 4, 7

var names = [12]string{"c", "c#", "d", "d#", "e", "f", "f#", "g", "g#", "a", "a#", "b"}

// Options set the header. A zero BPM picks the tempo from Tempos that fits
// the note lengths best. An empty Name is "piezotone".
type Options struct {
	Name string
	BPM  int
}

type token struct {
	midi, dur int // midi is piezotone.Rest for a pause; dur is 1 to 32
	dot       bool
}

// Encode writes notes as RTTTL. It moves the melody by whole octaves so that
// most notes are in octaves 4 to 7, and folds the other notes into range. The warnings tell what it changed.
func Encode(notes []piezotone.Note, o Options) (string, []string) {
	var warns []string
	name := strings.ReplaceAll(o.Name, ":", "")
	if name == "" {
		name = "piezotone"
	}
	if len(name) > 10 {
		name = name[:10]
		warns = append(warns, fmt.Sprintf("cut the name to %q: the limit is 10 characters", name))
	}
	bpm := o.BPM
	if bpm <= 0 {
		best := math.Inf(1)
		for _, b := range Tempos {
			if e := tempoError(notes, b); e < best {
				bpm, best = b, e
			}
		}
	}

	// One octave shift for the melody keeps its shape; then fold what is still out.
	lo, hi := 12*(MinOctave+1), 12*(MaxOctave+2)-1
	shift, bestIn := 0, -1
	for _, k := range []int{0, 1, -1, 2, -2, 3, -3, 4, -4, 5, -5} {
		in := 0
		for _, n := range notes {
			if m := n.MIDI + 12*k; n.MIDI != piezotone.Rest && m >= lo && m <= hi {
				in++
			}
		}
		if in > bestIn {
			shift, bestIn = k, in
		}
	}
	if shift != 0 {
		warns = append(warns, fmt.Sprintf("moved the melody by %+d octave(s) into octaves %d to %d", shift, MinOctave, MaxOctave))
	}
	var toks []token
	folded := 0
	for _, n := range notes {
		m := n.MIDI
		if m != piezotone.Rest {
			m += 12 * shift
			f := m
			for ; f/12-1 < MinOctave; f += 12 {
			}
			for ; f/12-1 > MaxOctave; f -= 12 {
			}
			if f != m {
				folded++
			}
			m = f
		}
		for _, l := range lengths(n.Duration, bpm) {
			toks = append(toks, token{m, l.dur, l.dot})
		}
	}
	if folded > 0 {
		warns = append(warns, fmt.Sprintf("moved %d note(s) by octaves into octaves %d to %d", folded, MinOctave, MaxOctave))
	}

	// The defaults are the most common duration and octave.
	durs, octs := map[int]int{}, map[int]int{}
	for _, t := range toks {
		durs[t.dur]++
		if t.midi != piezotone.Rest {
			octs[t.midi/12-1]++
		}
	}
	d, oct := mostCommon(durs, 4), mostCommon(octs, 6)

	var b strings.Builder
	fmt.Fprintf(&b, "%s:d=%d,o=%d,b=%d:", name, d, oct, bpm)
	for i, t := range toks {
		if i > 0 {
			b.WriteByte(',')
		}
		if t.dur != d {
			b.WriteString(strconv.Itoa(t.dur))
		}
		if t.midi == piezotone.Rest {
			b.WriteByte('p')
		} else {
			b.WriteString(names[t.midi%12])
			if o := t.midi/12 - 1; o != oct {
				b.WriteString(strconv.Itoa(o))
			}
		}
		if t.dot {
			b.WriteByte('.')
		}
	}
	return b.String(), warns
}

type length struct {
	dur int
	dot bool
}

// lengths splits a duration into the nearest RTTTL lengths. Each piece is at
// least a 32nd note. shortcut: a note longer than a dotted whole note plays
// again for each piece; use a slower tempo if that matters.
func lengths(d time.Duration, bpm int) []length {
	whole := 4 * float64(time.Minute) / float64(bpm)
	var out []length
	for left := float64(d); ; {
		best, bestErr := length{32, false}, math.Inf(1)
		for _, dur := range []int{1, 2, 4, 8, 16, 32} {
			for _, dot := range []bool{false, true} {
				v := whole / float64(dur)
				if dot {
					v *= 1.5
				}
				if e := math.Abs(left - v); e < bestErr {
					best, bestErr = length{dur, dot}, e
				}
			}
		}
		out = append(out, best)
		if left <= whole*1.5 {
			return out
		}
		left -= whole * 1.5
	}
}

// tempoError is the total time error of the notes at bpm. Each split costs
// 100 ms, so that a fast tempo does not win by cutting long notes.
func tempoError(notes []piezotone.Note, bpm int) float64 {
	whole := 4 * float64(time.Minute) / float64(bpm)
	e := 0.0
	for _, n := range notes {
		got := 0.0
		ls := lengths(n.Duration, bpm)
		e += float64(len(ls)-1) * float64(100*time.Millisecond)
		for _, l := range ls {
			v := whole / float64(l.dur)
			if l.dot {
				v *= 1.5
			}
			got += v
		}
		e += math.Abs(got - float64(n.Duration))
	}
	return e
}

func mostCommon(count map[int]int, def int) int {
	best, n := def, 0
	for k, c := range count {
		if c > n || c == n && k < best {
			best, n = k, c
		}
	}
	return best
}
