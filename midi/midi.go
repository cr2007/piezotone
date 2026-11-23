// Package midi writes notes as a monophonic standard MIDI file (SMF type 0).
package midi

import (
	"io"

	gm "gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/smf"

	"github.com/cr2007/piezotone"
)

// Write writes notes on channel 0 at velocity 100. The file has 1000 ticks
// per quarter note at 60 BPM, so one tick is one millisecond.
func Write(w io.Writer, notes []piezotone.Note) error {
	var tr smf.Track
	tr.Add(0, smf.MetaTempo(60))
	var gap uint32 // ticks of rest before the next note
	for _, n := range notes {
		ms := uint32(n.Duration.Milliseconds())
		if n.MIDI == piezotone.Rest || n.MIDI < 0 || n.MIDI > 127 {
			gap += ms
			continue
		}
		tr.Add(gap, gm.NoteOn(0, uint8(n.MIDI), 100))
		tr.Add(ms, gm.NoteOff(0, uint8(n.MIDI)))
		gap = 0
	}
	tr.Close(gap)
	s := smf.New()
	s.TimeFormat = smf.MetricTicks(1000)
	if err := s.Add(tr); err != nil {
		return err
	}
	_, err := s.WriteTo(w)
	return err
}
