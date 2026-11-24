package piezotone

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
	"time"
)

// voice renders tones as a voice-like harmonic tone (five partials). Each note sounds for 95%
// of its length. With scoop, each note starts 120 cents flat and slides up over 70 ms, as a
// singer does.
func voice(tones []tone, rate int, scoop bool) []float32 {
	var out []float32
	phase := 0.0
	for _, n := range tones {
		k := n.ms * rate / 1000
		for i := range k {
			v := 0.0
			if n.midi >= 0 && i < k*95/100 {
				m := n.midi
				if t := float64(i) / float64(rate); scoop && t < 0.07 {
					m -= 1.2 * (1 - t/0.07)
				}
				phase += 2 * math.Pi * 440 * math.Pow(2, (m-69)/12) / float64(rate)
				for h, a := range []float64{1, 0.5, 0.35, 0.25, 0.15} {
					v += 0.12 * a * math.Sin(float64(h+1)*phase)
				}
			}
			out = append(out, float32(v))
		}
	}
	return out
}

// clicks adds a 20 ms decaying noise burst at each time in seconds.
func clicks(s []float32, rate int, at []float64, amp float64) {
	rng := rand.New(rand.NewPCG(3, 4))
	for _, a := range at {
		i0 := int(a * float64(rate))
		for j := range rate / 50 {
			if i0+j < len(s) {
				s[i0+j] += float32(amp * math.Exp(-float64(j)/float64(rate/200)) * (2*rng.Float64() - 1))
			}
		}
	}
}

// onsetErrors returns, for each reference note, the absolute time from its onset to the
// nearest estimated note start with the same pitch (+-50 cents), in ms; 1000 when none.
func onsetErrors(ref []tone, est []Note) []int {
	var errs []int
	t := 0
	for _, r := range ref {
		if r.midi >= 0 {
			best := 1000
			for _, e := range est {
				if e.MIDI != Rest && math.Abs(float64(e.MIDI)-r.midi) <= 0.5 {
					best = min(best, abs(int(e.Start/time.Millisecond)-t))
				}
			}
			errs = append(errs, best)
		}
		t += r.ms
	}
	return errs
}

func pitched(notes []Note) int {
	return len(notes) - len(slices.DeleteFunc(slices.Clone(notes), func(n Note) bool { return n.MIDI != Rest }))
}

func TestFromPCMScoopOnsets(t *testing.T) {
	// A scooped note reaches its pitch late; the note must still start at its attack.
	ref := []tone{{-1, 200}, {67, 450}, {71, 450}, {69, 450}, {67, 600}, {64, 450}, {62, 450}, {67, 700}}
	notes, err := FromPCM(voice(ref, 44100, true), 44100, PCMOptions{})
	if err != nil {
		t.Fatal(err)
	}
	errs := onsetErrors(ref, notes)
	t.Logf("onset errors %v ms\n%v", errs, notes)
	if slices.Max(errs) > 30 {
		t.Errorf("onset errors %v ms, want all within 30 ms", errs)
	}
}

func TestFromPCMRepeatedNotes(t *testing.T) {
	// Twinkle at 110 BPM: repeated notes with a 27 ms gap, and hi-hat clicks on the off-beats.
	b := 545
	ref := []tone{{-1, 200}, {72, b}, {72, b}, {79, b}, {79, b}, {81, b}, {81, b}, {79, 2 * b}}
	s := voice(ref, 44100, false)
	var hats []float64
	for t := 0.2 + float64(b)/2000; t < 4.5; t += float64(b) / 1000 {
		hats = append(hats, t)
	}
	clicks(s, 44100, hats, 0.08)
	notes, err := FromPCM(s, 44100, PCMOptions{})
	if err != nil {
		t.Fatal(err)
	}
	errs := onsetErrors(ref, notes)
	t.Logf("onset errors %v ms\n%v", errs, notes)
	if n := pitched(notes); n != 7 || slices.Max(errs) > 30 {
		t.Errorf("%d notes with onset errors %v ms, want 7 notes within 30 ms", n, errs)
	}
}

func TestFixSpikes(t *testing.T) {
	// The 3rd partial of D#4 (A#5, +19) taken as a note between D#4 and D4 becomes D#4.
	in := []step{{63, 340}, {82, 640}, {Rest, 60}, {62, 250}}
	if got := fixSpikes(slices.Clone(in)); got[1].midi != 63 {
		t.Errorf("fixSpikes = %v, want the spike on 63", got)
	}
	// A real leap from a low voice up to the melody stays.
	keep := []step{{51, 300}, {63, 300}, {65, 200}}
	if got := fixSpikes(slices.Clone(keep)); !slices.Equal(got, keep) {
		t.Errorf("fixSpikes = %v, want %v", got, keep)
	}
}

func TestFromPCMDrums(t *testing.T) {
	// A held A4 from 0.2 to 1.4 s, then a rest; drum clicks inside the note and in the rest.
	ref := []tone{{-1, 200}, {69, 1200}, {-1, 800}}
	s := voice(ref, 44100, false)
	clicks(s, 44100, []float64{0.5, 0.8, 1.1, 1.6, 1.8}, 0.3)

	plain, err := FromPCM(s, 44100, PCMOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if n := pitched(plain); n != 1 {
		t.Errorf("without Drums: %d pitched notes, want 1\n%v", n, plain)
	}

	notes, err := FromPCM(s, 44100, PCMOptions{Drums: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%v", notes)
	var strikes, ticks []int
	for _, n := range notes {
		ms := int(n.Start / time.Millisecond)
		switch n.MIDI {
		case 69:
			strikes = append(strikes, ms)
		case TickMIDI:
			ticks = append(ticks, ms)
		}
	}
	near := func(got []int, want []int) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if abs(got[i]-want[i]) > 30 {
				return false
			}
		}
		return true
	}
	if !near(strikes, []int{200, 500, 800, 1100}) || !near(ticks, []int{1600, 1800}) {
		t.Errorf("A4 strikes at %v ms (want 200 500 800 1100), ticks at %v ms (want 1600 1800)", strikes, ticks)
	}
	checkTimes(t, notes)
}
