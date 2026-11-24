package main

import (
	"bytes"
	"encoding/binary"
	"math"
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

// clickWAV writes 1.2 s of A4 with two noise clicks inside it, as 16-bit mono WAV.
func clickWAV(t *testing.T) string {
	t.Helper()
	const rate = 44100
	s := make([]int16, rate*3/2)
	seed := uint32(1)
	for i := range s {
		v := 0.0
		if i < rate*6/5 {
			v = 0.3 * math.Sin(2*math.Pi*440*float64(i)/rate)
		}
		for _, at := range []int{rate / 2, rate * 4 / 5} {
			if d := i - at; d >= 0 && d < rate/50 {
				seed = seed*1664525 + 1013904223
				v += 0.4 * math.Exp(-float64(d)/(rate/200)) * (float64(seed>>8)/(1<<24)*2 - 1)
			}
		}
		s[i] = int16(v * 32767)
	}
	var b bytes.Buffer
	le := binary.LittleEndian
	b.WriteString("RIFF")
	binary.Write(&b, le, uint32(36+2*len(s)))
	b.WriteString("WAVEfmt ")
	for _, v := range []any{uint32(16), uint16(1), uint16(1), uint32(rate), uint32(2 * rate), uint16(2), uint16(16)} {
		binary.Write(&b, le, v)
	}
	b.WriteString("data")
	binary.Write(&b, le, uint32(2*len(s)))
	binary.Write(&b, le, s)
	file := filepath.Join(t.TempDir(), "click.wav")
	if err := os.WriteFile(file, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestRunDrums(t *testing.T) {
	file := clickWAV(t)
	for _, drums := range []bool{false, true} {
		args := []string{file}
		if drums {
			args = []string{"-drums", file}
		}
		var out bytes.Buffer
		if err := run(args, &out, new(bytes.Buffer)); err != nil {
			t.Fatal(err)
		}
		if got := strings.Count(out.String(), "R:25"); (got == 2) != drums {
			t.Errorf("drums %v: %q has %d strike gaps", drums, out.String(), got)
		}
	}
}
