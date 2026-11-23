package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cr2007/piezotone"
	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/smf"
)

// songFile writes three low notes at 120 BPM: A2, B2, C#3.
func songFile(t *testing.T) string {
	t.Helper()
	var tr smf.Track
	for _, k := range []uint8{45, 47, 49} {
		tr.Add(0, midi.NoteOn(0, k, 100))
		tr.Add(240, midi.NoteOff(0, k))
	}
	tr.Close(0)
	s := smf.New()
	s.TimeFormat = smf.MetricTicks(480)
	s.Add(tr)
	file := filepath.Join(t.TempDir(), "song.mid")
	var b bytes.Buffer
	if _, err := s.WriteTo(&b); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestRunOpenDisplay(t *testing.T) {
	// The buzzer needs 400 Hz or more: piezotone moves the notes up two octaves.
	file := songFile(t)
	var out, errOut bytes.Buffer
	if err := run([]string{"-max-notes", "2", file}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	m := strings.TrimSpace(out.String())
	if want := "A4:250 B4:250"; m != want {
		t.Errorf("stdout %q, want %q", m, want)
	}
	if !strings.Contains(errOut.String(), "moved the melody up by 2 octave") || !strings.Contains(errOut.String(), "(1 note(s))") {
		t.Errorf("stderr %q", errOut.String())
	}
}

func TestRunRTTTL(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"-format", "rtttl", songFile(t)}, &out, new(bytes.Buffer)); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(out.String()), "song:d=32,o=4,b=45:a.,b.,c#5."; got != want {
		t.Errorf("stdout %q, want %q", got, want)
	}
}

func TestRunMIDIFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.mid")
	if err := run([]string{"-format", "midi", "-o", out, songFile(t)}, new(bytes.Buffer), new(bytes.Buffer)); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	notes, err := piezotone.FromMIDI(f, piezotone.MIDIOptions{Track: -1, Channel: -1})
	if err != nil || len(notes) != 3 || notes[0].MIDI != 45 || notes[2].Duration.Milliseconds() != 250 {
		t.Errorf("read back %v, %v", notes, err)
	}
}

func TestRunErrors(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "x.txt")
	os.WriteFile(bad, []byte("not audio"), 0o600)
	for _, args := range [][]string{{}, {"a", "b"}, {bad}, {"/no/such/file"}, {"-format", "wav", bad}} {
		if err := run(args, new(bytes.Buffer), new(bytes.Buffer)); err == nil {
			t.Errorf("run(%q): no error", args)
		}
	}
}
