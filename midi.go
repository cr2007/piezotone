package piezotone

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"

	"gitlab.com/gomidi/midi/v2/smf"
)

// drumChannel is General MIDI channel 10 (9 from 0). Its notes are drum
// sounds, not pitches, so the auto pick skips it.
const drumChannel = 9

// maxMIDIMS is the longest MIDI melody, 24 h.
const maxMIDIMS = 24 * 3600 * 1000

// MIDIOptions picks the notes to play. A value of -1 means "any". When both
// are -1, FromMIDI picks the track and channel pair with the most notes.
type MIDIOptions struct {
	Track   int // 0 is the first track
	Channel int // 0 to 15
}

type span struct {
	track, ch, key int
	on, off        int64 // absolute ticks
}

// FromMIDI reads a standard MIDI file and returns one voice: when notes
// overlap, the highest note sounds.
func FromMIDI(r io.Reader, o MIDIOptions) ([]Note, error) {
	s, err := readSMF(r)
	if err != nil {
		return nil, fmt.Errorf("piezotone: read MIDI: %w", err)
	}
	if _, ok := s.TimeFormat.(smf.MetricTicks); !ok {
		return nil, errors.New("piezotone: MIDI files with SMPTE time are not supported")
	}
	var spans []span
	for ti, tr := range s.Tracks {
		var abs int64
		open := map[[2]uint8]int{} // channel and key to the index in spans
		for _, ev := range tr {
			if abs += int64(ev.Delta); abs > math.MaxUint32 { // smf.TimeAt wraps here
				return nil, fmt.Errorf("piezotone: MIDI track %d is longer than 2^32 ticks", ti)
			}
			var ch, key, vel uint8
			switch {
			case ev.Message.GetNoteStart(&ch, &key, &vel):
				if i, ok := open[[2]uint8{ch, key}]; ok {
					spans[i].off = abs
				}
				open[[2]uint8{ch, key}] = len(spans)
				spans = append(spans, span{ti, int(ch), int(key), abs, -1})
			case ev.Message.GetNoteOff(&ch, &key, nil), ev.Message.GetNoteOn(&ch, &key, &vel): // the second is velocity 0
				if i, ok := open[[2]uint8{ch, key}]; ok {
					spans[i].off = abs
					delete(open, [2]uint8{ch, key})
				}
			}
		}
		for _, i := range open { // a note with no note off ends with its track
			spans[i].off = abs
		}
	}

	if o.Track < 0 && o.Channel < 0 {
		o.Track, o.Channel = busiest(spans)
	}
	var sel []span
	for _, sp := range spans {
		if (o.Track < 0 || sp.track == o.Track) && (o.Channel < 0 || sp.ch == o.Channel) && sp.off > sp.on {
			sel = append(sel, sp)
		}
	}
	if len(sel) == 0 {
		return nil, fmt.Errorf("piezotone: no notes in MIDI track %d, channel %d (-1 is any)", o.Track, o.Channel)
	}

	// Skyline: between two event times, the highest held note sounds.
	var times []int64
	for _, sp := range sel {
		times = append(times, sp.on, sp.off)
	}
	slices.Sort(times)
	times = slices.Compact(times)
	at := make([]int, len(times)) // event times in ms
	for i, tick := range times {
		at[i] = int(math.Round(float64(s.TimeAt(tick)) / 1000))
		if at[i] < 0 || at[i] > maxMIDIMS || i > 0 && at[i] < at[i-1] { // for example a tempo event with no data
			return nil, errors.New("piezotone: the MIDI tempo events give times that are negative, go back or pass 24 h")
		}
	}
	var notes []step
	last := -2 // index in sel of the last note, -1 for a rest
	for i, t := range times[:len(times)-1] {
		top := -1
		for j, sp := range sel {
			if sp.on <= t && t < sp.off && (top < 0 || sp.key > sel[top].key || sp.key == sel[top].key && sp.on > sel[top].on) {
				top = j
			}
		}
		d := at[i+1] - at[i]
		if d == 0 { // shorter than the ms grid: drop it, so that the note around it stays one note
			continue
		}
		if top == last {
			notes[len(notes)-1].ms += d
			continue
		}
		if top < 0 {
			notes = append(notes, step{Rest, d})
		} else {
			notes = append(notes, step{sel[top].key, d})
		}
		last = top
	}
	if len(notes) == 0 {
		return nil, errors.New("piezotone: all MIDI notes are shorter than 1 ms")
	}
	return timed(notes), nil
}

// readSMF is smf.ReadFrom, but a panic in it (found by FuzzFromMIDI, for example
// on SMPTE time or a cut header) becomes an error.
func readSMF(r io.Reader) (s *smf.SMF, err error) {
	defer func() {
		if p := recover(); p != nil {
			s, err = nil, fmt.Errorf("bad file: %v", p)
		}
	}()
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return smf.ReadFrom(bytes.NewReader(keepRunningStatus(b)))
}

// keepRunningStatus writes the status byte again in each channel event that
// uses running status after a meta or sysex event. The SMF 1.0 spec says that
// these events cancel running status, but many files (for example karaoke
// files with lyric events) keep it, as most players do. gomidi drops such
// events, so their notes never end. A track that it cannot walk stays as it is.
func keepRunningStatus(b []byte) []byte {
	var out []byte
	i := 0
	for i+8 <= len(b) {
		end := i + 8 + min(int(binary.BigEndian.Uint32(b[i+4:])), len(b)-i-8)
		trk, changed := statusAfterMeta(b[i+8 : end])
		if string(b[i:i+4]) != "MTrk" || !changed {
			out = append(out, b[i:end]...)
		} else {
			out = binary.BigEndian.AppendUint32(append(out, "MTrk"...), uint32(len(trk)))
			out = append(out, trk...)
		}
		i = end
	}
	return append(out, b[i:]...)
}

// statusAfterMeta does keepRunningStatus for one track. changed is false when
// the track needs no change or cannot be walked.
func statusAfterMeta(t []byte) (out []byte, changed bool) {
	// vlq reads a variable-length number of at most 4 bytes at t[i:].
	vlq := func(i int) (v, next int) {
		for k := i; k < len(t) && k < i+4; k++ {
			if v = v<<7 | int(t[k]&0x7F); t[k] < 0x80 {
				return v, k + 1
			}
		}
		return 0, -1
	}
	data := func(s byte) int { // program change (Cx) and channel pressure (Dx) have 1 data byte
		if s>>4 == 0xC || s>>4 == 0xD {
			return 1
		}
		return 2
	}
	var status byte
	cleared := false
	for j := 0; j < len(t); {
		_, k := vlq(j)
		if k < 0 || k >= len(t) {
			return nil, false
		}
		n, c := 0, t[k]
		switch {
		case c == 0xFF || c == 0xF0 || c == 0xF7:
			at := k + 1
			if c == 0xFF {
				at++ // the meta type
			}
			ln, next := vlq(at)
			if next < 0 {
				return nil, false
			}
			n, cleared = next-k+ln, true
		case c >= 0x80:
			status, cleared, n = c, false, 1+data(c)
		case status == 0:
			return nil, false
		default:
			n = data(status)
			if cleared {
				out, changed, cleared = append(append(out, t[j:k]...), status), true, false
				j = k
			}
		}
		if k+n > len(t) {
			return nil, false
		}
		out = append(out, t[j:k+n]...)
		j = k + n
	}
	return out, changed
}

// busiest returns the track and channel with the most notes, without drums
// unless the file has only drums.
func busiest(spans []span) (track, ch int) {
	count := map[[2]int]int{}
	for _, sp := range spans {
		count[[2]int{sp.track, sp.ch}]++
	}
	best, n := [2]int{-1, -1}, 0
	for _, drums := range []bool{false, true} {
		for k, c := range count {
			if (k[1] == drumChannel) == drums && (c > n || c == n && (k[0] < best[0] || k[0] == best[0] && k[1] < best[1])) {
				best, n = k, c
			}
		}
		if n > 0 {
			break
		}
	}
	return best[0], best[1]
}
