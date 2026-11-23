// Package piezotone turns MIDI and one-voice audio into note events for a
// piezo buzzer. The core knows no device. The packages rtttl, midi and
// opendisplay write the notes in an output format.
package piezotone

import (
	"math"
	"time"
)

// Rest is the MIDI value of a note that plays no tone.
const Rest = -1

// Note is one step of a melody: a MIDI note number (69 is A4, 440 Hz) or
// Rest, its start time and its duration. A melody has no gaps: each note
// starts where the note before it ends.
type Note struct {
	MIDI     int
	Start    time.Duration
	Duration time.Duration
}

// MIDIHz is the frequency of a MIDI note number in equal temperament.
func MIDIHz(m int) float64 { return 440 * math.Pow(2, float64(m-69)/12) }

// HzMIDI is the nearest MIDI note number to hz.
func HzMIDI(hz float64) int { return int(math.Round(69 + 12*math.Log2(hz/440))) }

// step is one note in whole milliseconds, before the start times are set.
type step struct{ midi, ms int }

// timed sets the start times.
func timed(steps []step) []Note {
	out := make([]Note, len(steps))
	var t time.Duration
	for i, s := range steps {
		d := time.Duration(s.ms) * time.Millisecond
		out[i] = Note{MIDI: s.midi, Start: t, Duration: d}
		t += d
	}
	return out
}
