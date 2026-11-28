package rtttl

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cr2007/piezotone"
)

// fuzzNotes reads 3 bytes per note: the MIDI number (128 is a rest) and the length in ms.
func fuzzNotes(b []byte) []piezotone.Note {
	var out []piezotone.Note
	var t time.Duration
	for ; len(b) >= 3; b = b[3:] {
		m := int(b[0] % 129)
		if m == 128 {
			m = piezotone.Rest
		}
		d := time.Duration(int(b[1])<<8|int(b[2])) * time.Millisecond
		out = append(out, piezotone.Note{MIDI: m, Start: t, Duration: d})
		t += d
	}
	return out
}

func FuzzEncode(f *testing.F) {
	f.Add([]byte{69, 0, 200, 128, 0, 50, 0, 1, 0}, 0)
	f.Add([]byte{127, 255, 255}, 25)
	f.Fuzz(func(t *testing.T, b []byte, bpm int) {
		notes := fuzzNotes(b)
		tune, _ := Encode(notes, Options{BPM: bpm % 1000})
		head := strings.Split(tune, ":")
		if len(head) != 3 || len(head[0]) > 10 {
			t.Fatalf("bad sections in %q", tune)
		}
		for _, tok := range strings.Split(head[2], ",") {
			if tok == "" && len(notes) == 0 {
				continue
			}
			tok = strings.TrimSuffix(tok, ".")
			i := strings.IndexAny(tok, "abcdefgp")
			if i < 0 {
				t.Fatalf("token %q in %q has no note", tok, tune)
			}
			if d := tok[:i]; d != "" && !slices.Contains([]string{"1", "2", "4", "8", "16", "32"}, d) {
				t.Fatalf("token %q in %q has a bad length", tok, tune)
			}
			name := strings.TrimLeft(tok[i:], "abcdefgp#")
			if o, err := strconv.Atoi(name); name != "" && (err != nil || o < MinOctave || o > MaxOctave) {
				t.Fatalf("token %q in %q has a bad octave", tok, tune)
			}
		}
	})
}
