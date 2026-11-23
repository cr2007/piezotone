// Command piezotone turns a MIDI or audio file into a one-voice melody for a
// piezo buzzer.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cr2007/piezotone"
	"github.com/cr2007/piezotone/midi"
	"github.com/cr2007/piezotone/opendisplay"
	"github.com/cr2007/piezotone/rtttl"
)

const usage = `Usage: piezotone [-format opendisplay|rtttl|midi] [-o FILE] [flags] FILE

piezotone reads a MIDI, WAV, MP3, FLAC or Ogg Vorbis file and writes a
melody with one voice for a piezo buzzer.

Formats:
  opendisplay  the melody notation of "opendisplay buzz -melody" (default):
                 opendisplay buzz -melody "$(piezotone song.mid)"
  rtttl        a Nokia RTTTL ringtone, for example for Arduino or ESPHome
  midi         a standard MIDI file with one voice

MIDI: piezotone plays the track and channel with the most notes (not the
drums on channel 9). When notes overlap, the highest note plays.

Audio: piezotone follows the loudest voice. It works for a whistle, a
single instrument or a ringtone, also over a quieter backing chord or bass.
It does not work when no one voice is clearly the loudest.

opendisplay: the buzzer plays 400 to 12000 Hz, in steps of 5 ms, for at most
30 s and 104 notes. piezotone moves notes by octaves into this range and
cuts what does not fit. It writes what it changed to stderr.

Flags:
`

const hint = `Speech, noise and drums have no stable pitch.
For a rough pitch, try -voicing 2.5 to 2.
A piezo plays square-wave tones only.`

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stderr, "piezotone:", strings.TrimPrefix(err.Error(), "piezotone: "))
		}
		var nn *piezotone.NoNotesError
		if errors.As(err, &nn) {
			fmt.Fprintln(os.Stderr, hint)
		}
		os.Exit(2)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("piezotone", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage); fs.PrintDefaults() }
	format := fs.String("format", "opendisplay", "output `FORMAT`: opendisplay, rtttl or midi")
	outFile := fs.String("o", "", "write to `FILE`, not to stdout")
	track := fs.Int("track", -1, "MIDI: play track `N` (0 is the first); -1 picks one")
	channel := fs.Int("channel", -1, "MIDI: play channel `N`, 0 to 15; -1 picks one")
	voicing := fs.Float64("voicing", 3, "audio: a frame has a pitch when its best pitch is `V` times the mean; a lower value finds more notes")
	minNote := fs.Int("min-note", 60, "audio: shortest note in `MS`")
	maxNotes := fs.Int("max-notes", opendisplay.MaxNotesSealed, "opendisplay: cut the melody at `N` notes (104 fits one encrypted frame, 119 a plain one)")
	name := fs.String("name", "", "rtttl: tune `NAME`, at most 10 characters (default: the file name)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("give one FILE")
	}
	if *format != "opendisplay" && *format != "rtttl" && *format != "midi" {
		return fmt.Errorf("unknown format %q; use opendisplay, rtttl or midi", *format)
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	var notes []piezotone.Note
	if piezotone.IsMIDI(data) {
		notes, err = piezotone.FromMIDI(bytes.NewReader(data), piezotone.MIDIOptions{Track: *track, Channel: *channel})
	} else {
		var s []float32
		var rate int
		if s, rate, err = piezotone.Decode(data); err == nil {
			notes, err = piezotone.FromPCM(s, rate, piezotone.PCMOptions{Voicing: *voicing, MinNoteMS: *minNote})
		}
	}
	if err != nil {
		return err
	}

	var out bytes.Buffer
	var warns []string
	switch *format {
	case "opendisplay":
		var steps []opendisplay.Step
		steps, warns = opendisplay.Fit(notes, opendisplay.FitOptions{MaxNotes: *maxNotes})
		if len(steps) == 0 {
			return errors.New("found no notes that the buzzer can play")
		}
		fmt.Fprintln(&out, opendisplay.Melody(steps))
	case "rtttl":
		if *name == "" {
			*name = strings.TrimSuffix(filepath.Base(fs.Arg(0)), filepath.Ext(fs.Arg(0)))
		}
		var s string
		s, warns = rtttl.Encode(notes, rtttl.Options{Name: *name})
		fmt.Fprintln(&out, s)
	case "midi":
		err = midi.Write(&out, notes)
	}
	for _, w := range warns {
		fmt.Fprintln(stderr, "piezotone:", w)
	}
	if err != nil {
		return err
	}
	if *outFile != "" {
		return os.WriteFile(*outFile, out.Bytes(), 0o644)
	}
	_, err = stdout.Write(out.Bytes())
	return err
}
