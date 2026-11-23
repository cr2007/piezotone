package midi

import (
	"bytes"
	"slices"
	"testing"
	"time"

	"github.com/cr2007/piezotone"
)

func TestWriteRoundTrip(t *testing.T) {
	ms := time.Millisecond
	in := []piezotone.Note{
		{MIDI: 69, Duration: 250 * ms},
		{MIDI: piezotone.Rest, Start: 250 * ms, Duration: 100 * ms},
		{MIDI: 72, Start: 350 * ms, Duration: 400 * ms},
		{MIDI: 72, Start: 750 * ms, Duration: 5 * ms}, // a repeated note stays two notes
	}
	var b bytes.Buffer
	if err := Write(&b, in); err != nil {
		t.Fatal(err)
	}
	got, err := piezotone.FromMIDI(&b, piezotone.MIDIOptions{Track: -1, Channel: -1})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, in) {
		t.Errorf("read back %v, want %v", got, in)
	}
}
