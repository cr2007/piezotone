package piezotone

import (
	"bytes"
	"fmt"
	"math"
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

	return encodeSMF(t, 480, tempo, lead, bass, drums)
}

// encodeSMF writes tracks as a type 1 file with ppq ticks per quarter note.
func encodeSMF(t *testing.T, ppq uint16, tracks ...smf.Track) []byte {
	t.Helper()
	s := smf.New()
	s.TimeFormat = smf.MetricTicks(ppq)
	for _, tr := range tracks {
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

func TestFromMIDIZeroLengthNote(t *testing.T) {
	// At 960 ticks per quarter and 120 BPM, ticks 1 and 2 both round to 1 ms.
	// A grace note on those ticks must not become a 0 ms note.
	var tr smf.Track
	tr.Add(0, midi.NoteOn(0, 72, 100))
	tr.Add(1, midi.NoteOn(0, 79, 100))
	tr.Add(1, midi.NoteOff(0, 79))
	tr.Add(958, midi.NoteOff(0, 72))
	tr.Close(0)
	notes, err := FromMIDI(bytes.NewReader(encodeSMF(t, 960, tr)), MIDIOptions{Track: -1, Channel: -1})
	if got := short(notes); err != nil || got != "72:500" {
		t.Errorf("FromMIDI = %s, %v; want 72:500", got, err)
	}
}

func TestFromMIDIOnlyTooShort(t *testing.T) {
	// One note of 1 tick (0.05 ms at 9600 ticks per quarter) rounds to 0 ms: no notes, so an error.
	var tr smf.Track
	tr.Add(0, midi.NoteOn(0, 72, 100))
	tr.Add(1, midi.NoteOff(0, 72))
	tr.Close(0)
	if notes, err := FromMIDI(bytes.NewReader(encodeSMF(t, 9600, tr)), MIDIOptions{Track: -1, Channel: -1}); err == nil {
		t.Errorf("FromMIDI = %q, no error", short(notes))
	}
}

func TestFromMIDITooLong(t *testing.T) {
	// smf.TimeAt wraps after 2^32 ticks, so a later note got a negative length.
	var tr smf.Track
	tr.Add(0, midi.NoteOn(0, 72, 100))
	tr.Add(10, midi.NoteOff(0, 72))
	for range 17 { // 17 deltas of 0x0FFFFFFF, the longest that the SMF spec allows
		tr.Add(0x0FFFFFFF, smf.MetaText("x"))
	}
	tr.Add(0, midi.NoteOn(0, 74, 100))
	tr.Add(10, midi.NoteOff(0, 74))
	tr.Close(0)
	notes, err := FromMIDI(bytes.NewReader(encodeSMF(t, 960, tr)), MIDIOptions{Track: -1, Channel: -1})
	if err == nil {
		t.Errorf("FromMIDI = %s, no error", short(notes))
	}
}

func TestFromMIDIBadTempo(t *testing.T) {
	// A tempo event with no data gave an infinite tempo, and smf.TimeAt then
	// gave a time near -2^63 ns. FuzzFromMIDI found it.
	var tr smf.Track
	tr.Add(0, midi.NoteOn(0, 72, 100))
	tr.Add(10, smf.Message{0xFF, 0x51, 0x00})
	tr.Add(10, midi.NoteOff(0, 72))
	tr.Add(0, midi.NoteOn(0, 74, 100))
	tr.Add(10, midi.NoteOff(0, 74))
	tr.Close(0)
	if notes, err := FromMIDI(bytes.NewReader(encodeSMF(t, 960, tr)), MIDIOptions{Track: -1, Channel: -1}); err == nil {
		t.Errorf("FromMIDI = %s, no error", short(notes))
	}
}

func TestFromMIDIRunningStatusAfterMeta(t *testing.T) {
	// The note offs use running status after a lyric event. The SMF spec says a
	// meta event cancels running status, but many files do this, for example
	// karaoke files. gomidi dropped the note offs, so the notes never ended.
	trk := "\x00\x90\x3c\x64" + // C4 on
		"\x00\xff\x05\x01x" + // lyric "x"
		"\x60\x3c\x00" + // C4 off, running status
		"\x00\x90\x3e\x64" + // D4 on
		"\x00\xff\x05\x01y" +
		"\x60\x3e\x00" + // D4 off, running status
		"\x83\x00\x90\x40\x64\x60\x40\x00" + // E4 after a rest of 384 ticks
		"\x00\xff\x2f\x00"
	smf := "MThd\x00\x00\x00\x06\x00\x00\x00\x01\x00\x60MTrk\x00\x00\x00" + string(rune(len(trk))) + trk
	notes, err := FromMIDI(strings.NewReader(smf), MIDIOptions{Track: -1, Channel: -1})
	if got := short(notes); err != nil || got != "60:500 62:500 R:2000 64:500" {
		t.Errorf("FromMIDI = %s, %v", got, err)
	}
}

func TestFromMIDITempoChanges(t *testing.T) {
	// A tempo event at each quarter note, 121 to 180 BPM, as exact us values.
	// Note 72 starts after the 60 quarters, at the sum of the tempos.
	q := func(bpm int) int { return int(math.Round(6e7 / float64(bpm))) }
	tempo := func(bpm int) smf.Message {
		v := q(bpm)
		return smf.Message{0xFF, 0x51, 0x03, byte(v >> 16), byte(v >> 8), byte(v)}
	}
	var tr smf.Track
	tr.Add(0, tempo(121))
	tr.Add(0, midi.NoteOn(0, 60, 100))
	tr.Add(1, midi.NoteOff(0, 60))
	at, total := 1, q(121)
	for bpm := 122; bpm <= 180; bpm++ {
		tr.Add(uint32(480*(bpm-121)-at), tempo(bpm))
		at, total = 480*(bpm-121), total+q(bpm)
	}
	tr.Add(uint32(480*60-at), midi.NoteOn(0, 72, 100))
	tr.Add(480, midi.NoteOff(0, 72))
	tr.Close(0)
	notes, err := FromMIDI(bytes.NewReader(encodeSMF(t, 480, tr)), MIDIOptions{Track: -1, Channel: -1})
	ms := func(us int) int { return int(math.Round(float64(us) / 1000)) }
	want := fmt.Sprintf("60:1 R:%d 72:%d", ms(total)-1, ms(total+q(180))-ms(total))
	if got := short(notes); err != nil || got != want {
		t.Errorf("FromMIDI = %s, %v; want %s", got, err, want)
	}
}
