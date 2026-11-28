package piezotone

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/smf"
)

// testSMF builds a type 1 file at 120 BPM, 480 ticks per quarter (1 tick = 500/480 ms).
//   - track 1, channel 0: a melody with a chord, a rest, a note off by
//     velocity 0 and a repeated note;
//   - track 2, channel 1: two bass notes;
//   - track 3, channel 9: drums, with the most notes.
func testSMF(t *testing.T) []byte {
	t.Helper()
	var tempo, lead, bass, drums smf.Track
	tempo.Add(0, smf.MetaTempo(120))
	tempo.Close(0)

	lead.Add(0, midi.NoteOn(0, 72, 100))
	lead.Add(0, midi.NoteOn(0, 79, 100)) // chord: the higher note sounds
	lead.Add(240, midi.NoteOff(0, 79))   // then C5 sounds again
	lead.Add(240, midi.NoteOff(0, 72))
	lead.Add(0, midi.NoteOn(0, 74, 100))
	lead.Add(240, midi.NoteOff(0, 74))
	lead.Add(240, midi.NoteOn(0, 76, 100)) // after a rest of 240 ticks
	lead.Add(480, midi.NoteOn(0, 76, 0))   // velocity 0 is a note off
	lead.Add(0, midi.NoteOn(0, 76, 100))   // the same note again
	lead.Add(240, midi.NoteOff(0, 76))
	lead.Close(0)

	bass.Add(0, midi.NoteOn(1, 48, 100))
	bass.Add(480, midi.NoteOff(1, 48))
	bass.Add(0, midi.NoteOn(1, 43, 100))
	bass.Add(480, midi.NoteOff(1, 43))
	bass.Close(0)

	for range 10 {
		drums.Add(0, midi.NoteOn(9, 36, 100))
		drums.Add(120, midi.NoteOff(9, 36))
	}
	drums.Close(0)

	s := smf.New()
	s.TimeFormat = smf.MetricTicks(480)
	for _, tr := range []smf.Track{tempo, lead, bass, drums} {
		if err := s.Add(tr); err != nil {
			t.Fatal(err)
		}
	}
	var b bytes.Buffer
	if _, err := s.WriteTo(&b); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// short writes notes as MIDI:MS tokens, with R for a rest.
func short(notes []Note) string {
	var b strings.Builder
	for i, n := range notes {
		if i > 0 {
			b.WriteByte(' ')
		}
		if n.MIDI == Rest {
			b.WriteString("R")
		} else {
			b.WriteString(strconv.Itoa(n.MIDI))
		}
		fmt.Fprintf(&b, ":%d", n.Duration.Milliseconds())
	}
	return b.String()
}

func TestFromMIDI(t *testing.T) {
	tests := []struct {
		name string
		o    MIDIOptions
		want string
	}{
		// G5 C5 D5 R E5 E5: the chord gives G5, and the repeated E5 stays two notes.
		{"auto picks the lead, not the drums", MIDIOptions{Track: -1, Channel: -1}, "79:250 72:250 74:250 R:250 76:500 76:250"},
		{"track flag", MIDIOptions{Track: 2, Channel: -1}, "48:500 43:500"},
		{"channel flag", MIDIOptions{Track: -1, Channel: 1}, "48:500 43:500"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			notes, err := FromMIDI(bytes.NewReader(testSMF(t)), tc.o)
			if err != nil {
				t.Fatal(err)
			}
			if got := short(notes); got != tc.want {
				t.Errorf("notes = %q, want %q", got, tc.want)
			}
			checkTimes(t, notes)
		})
	}
}

func TestFromMIDIErrors(t *testing.T) {
	if _, err := FromMIDI(bytes.NewReader(testSMF(t)), MIDIOptions{Track: 7, Channel: -1}); err == nil {
		t.Error("track 7: no error")
	}
	if _, err := FromMIDI(bytes.NewReader([]byte("MThd")), MIDIOptions{Track: -1, Channel: -1}); err == nil {
		t.Error("short file: no error")
	}
	// smf.ReadFrom panicked on these headers: SMPTE time with no tracks, and a cut division.
	for _, h := range []string{"MThd\x00\x00\x00\x06\x00\x0000\xe00", "MThd\x00\x00\x00\x06\x00\x0100\x81"} {
		if _, err := FromMIDI(strings.NewReader(h), MIDIOptions{Track: -1, Channel: -1}); err == nil {
			t.Errorf("%q: no error", h)
		}
	}
}
