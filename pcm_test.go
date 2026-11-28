package piezotone

import (
	"errors"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"
	"time"
)

// tone is a reference note. midi can be a quarter tone; -1 is a rest.
type tone struct {
	midi float64
	ms   int
}

// refNotes is the melody "A4:300 C5:250 E5:250 R:150 A5:400 G5:200 F5+:200
// E5:300 R:200 D5:250 F5:250 B5:300 C6:350 R:100 C6:250 E7:200 G7:250 R:120
// A5:500": 440 Hz to 3136 Hz, one quarter tone (F5+), rests of 100 ms and
// more, and a note that repeats after a rest.
func refNotes(t *testing.T) []tone {
	t.Helper()
	return []tone{{69, 300}, {72, 250}, {76, 250}, {-1, 150}, {81, 400}, {79, 200}, {77.5, 200},
		{76, 300}, {-1, 200}, {74, 250}, {77, 250}, {83, 300}, {84, 350}, {-1, 100}, {84, 250},
		{100, 200}, {103, 250}, {-1, 120}, {81, 500}}
}

// synth renders tones as a sine or a square wave at amplitude 0.5.
func synth(tones []tone, rate int, square bool) []float32 {
	var out []float32
	phase := 0.0
	for _, n := range tones {
		for range n.ms * rate / 1000 {
			v := 0.0
			if n.midi >= 0 {
				phase += 2 * math.Pi * 440 * math.Pow(2, (n.midi-69)/12) / float64(rate)
				v = 0.5 * math.Sin(phase)
				if square {
					v = 0.5 * math.Copysign(1, v)
				}
			}
			out = append(out, float32(v))
		}
	}
	return out
}

// fMeasure is the note-level F-measure of mir_eval.transcription without
// offsets: a note matches when its onset is within 50 ms and its pitch within
// 50 cents. The reference starts with a note. shortcut: greedy matching by nearest onset, not the maximum
// bipartite matching of mir_eval; the two agree when the notes are far apart,
// as in these tests.
func fMeasure(ref []tone, est []Note) (p, r, f float64) {
	type on struct {
		t    int
		midi float64
	}
	var ro, eo []on
	t := 0
	for _, n := range ref {
		if n.midi >= 0 {
			ro = append(ro, on{t, n.midi})
		}
		t += n.ms
	}
	// Count from the first note, as the output profiles do: an encoder can add silence at the start.
	for _, n := range est {
		if n.MIDI != Rest {
			if len(eo) == 0 {
				t = int(n.Start / time.Millisecond)
			}
			eo = append(eo, on{int(n.Start/time.Millisecond) - t, float64(n.MIDI)})
		}
	}
	used := make([]bool, len(eo))
	match := 0
	for _, a := range ro {
		best := -1
		for j, b := range eo {
			d := abs(a.t - b.t)
			if !used[j] && d <= 50 && math.Abs(a.midi-b.midi) <= 0.5 && (best < 0 || d < abs(a.t-eo[best].t)) {
				best = j
			}
		}
		if best >= 0 {
			used[best] = true
			match++
		}
	}
	if match == 0 {
		return 0, 0, 0
	}
	p, r = float64(match)/float64(len(eo)), float64(match)/float64(len(ro))
	return p, r, 2 * p * r / (p + r)
}

func abs(x int) int { return max(x, -x) }

func TestFromPCMSynthetic(t *testing.T) {
	ref := refNotes(t)
	for _, rate := range []int{44100, 48000} {
		for _, square := range []bool{false, true} {
			wave := map[bool]string{false: "sine", true: "square"}[square]
			t.Run(wave+"/"+strconv.Itoa(rate), func(t *testing.T) {
				samples, gotRate, err := Decode(writeWAV(synth(ref, rate, square), rate, 1, 16, 1, false))
				if err != nil || gotRate != rate {
					t.Fatalf("Decode: %d Hz, %v", gotRate, err)
				}
				checkTranscription(t, ref, samples, rate)
			})
		}
	}
}

// checkTranscription runs FromPCM and wants an F-measure of 1.
func checkTranscription(t *testing.T, ref []tone, samples []float32, rate int) {
	t.Helper()
	notes, err := FromPCM(samples, rate, PCMOptions{})
	if err != nil {
		t.Fatal(err)
	}
	checkTimes(t, notes)
	p, r, f := fMeasure(ref, notes)
	t.Logf("P %.3f R %.3f F %.3f\n%v", p, r, f, notes)
	if f < 1 {
		t.Errorf("F-measure %.3f, want 1", f)
	}
}

// checkTimes fails unless each note starts where the one before it ends.
func checkTimes(t *testing.T, notes []Note) {
	t.Helper()
	var end time.Duration
	for i, n := range notes {
		if n.Start != end || n.Duration <= 0 {
			t.Errorf("note %d = %+v, want start %v and a positive duration", i, n, end)
		}
		end = n.Start + n.Duration
	}
}

func TestFromPCMSilence(t *testing.T) {
	_, err := FromPCM(make([]float32, 44100), 44100, PCMOptions{})
	var nn *NoNotesError
	if !errors.As(err, &nn) || nn.Voiced != 0 || nn.Seconds != 1 {
		t.Errorf("silence: %v", err)
	}
}

func TestFromPCMNoise(t *testing.T) {
	// White noise has no stable pitch: FromPCM finds no note and says why.
	rng := rand.New(rand.NewPCG(1, 2))
	s := make([]float32, 2*44100)
	for i := range s {
		s[i] = float32(rng.Float64() - 0.5)
	}
	_, err := FromPCM(s, 44100, PCMOptions{})
	var nn *NoNotesError
	if !errors.As(err, &nn) {
		t.Fatalf("noise: %v", err)
	}
	t.Log(err)
	if nn.Seconds != 2 || nn.Frames == 0 || nn.BestContrast >= nn.Voicing || nn.RMSDB > -10 || nn.RMSDB < -12 {
		t.Errorf("noise: %+v", nn)
	}
}

func TestFromPCMLowVoice(t *testing.T) {
	// A melody at 110 to 220 Hz keeps its pitch; the output profile moves it.
	low := []tone{{45, 400}, {49, 400}, {57, 400}}
	notes, err := FromPCM(synth(low, 44100, false), 44100, PCMOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, f := fMeasure(low, notes); f < 1 {
		t.Errorf("FromPCM = %v, want %v", notes, low)
	}
}

func TestMIDIHz(t *testing.T) {
	for m := 0; m <= 127; m++ {
		if got := HzMIDI(MIDIHz(m)); got != m {
			t.Errorf("HzMIDI(MIDIHz(%d)) = %d", m, got)
		}
	}
	if MIDIHz(69) != 440 || HzMIDI(452) != 69 || HzMIDI(453.5) != 70 {
		t.Error("A4 is not 440 Hz")
	}
}

// ringtone renders a melody whose third harmonic is stronger than its
// fundamental, over a quieter held chord, as in a real phone ringtone.
func ringtone(melody []tone, rate int) []float32 {
	var out []float32
	phase := 0.0
	for _, n := range melody {
		for range n.ms * rate / 1000 {
			v := 0.0
			if n.midi >= 0 {
				phase += 2 * math.Pi * 440 * math.Pow(2, (n.midi-69)/12) / float64(rate)
				v = 0.15*math.Sin(phase) + 0.1*math.Sin(2*phase) + 0.25*math.Sin(3*phase)
			}
			out = append(out, float32(v))
		}
	}
	for i := range out {
		t := float64(i) / float64(rate)
		out[i] += float32(0.12*math.Sin(2*math.Pi*155.56*t) + 0.08*math.Sin(2*math.Pi*233.08*t)) // D#3 and A#3
	}
	return out
}

func TestFromPCMRingtoneOverChord(t *testing.T) {
	// A user report: YIN gave long rests and octave errors for a ringtone with a backing chord.
	melody := []tone{{65, 200}, {67, 200}, {68, 350}, {70, 250}, {68, 200}, {67, 200}, {68, 350},
		{67, 200}, {65, 350}, {63, 400}, {65, 200}, {67, 500}}
	notes, err := FromPCM(ringtone(melody, 44100), 44100, PCMOptions{})
	if err != nil {
		t.Fatal(err)
	}
	checkTimes(t, notes)
	p, r, f := fMeasure(melody, notes)
	t.Logf("P %.3f R %.3f F %.3f\n%v", p, r, f, notes)
	if f < 1 {
		t.Errorf("F-measure %.3f, want 1", f)
	}
}

func TestFromPCMNonFinite(t *testing.T) {
	// A float WAV can hold NaN or Inf. One bad sample must not change the note.
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		s := synth([]tone{{69, 1000}}, 44100, false)
		s[100] = float32(bad)
		notes, err := FromPCM(s, 44100, PCMOptions{})
		if err != nil || pitched(notes) != 1 || notes[0].MIDI != 69 {
			t.Errorf("sample %v: %s, %v", bad, short(notes), err)
		}
	}
}

func TestFromPCMHugeRate(t *testing.T) {
	// A WAV header can give up to 4.29 GHz. The FFT size grows with the rate, so FromPCM must refuse it.
	if _, err := FromPCM(make([]float32, 100), 1<<31, PCMOptions{}); err == nil || !strings.Contains(err.Error(), "sample rate") {
		t.Errorf("rate 2^31: %v", err)
	}
}

func TestFromPCMContrastNoPartials(t *testing.T) {
	// A 4 kHz tone has no partial of a pitch up to 200 Hz in its first 8: the contrast is 0, not NaN.
	_, err := FromPCM(synth([]tone{{69 + 12*math.Log2(4000.0/440), 500}}, 44100, false), 44100, PCMOptions{MaxHz: 200})
	var nn *NoNotesError
	if !errors.As(err, &nn) || nn.BestContrast != 0 {
		t.Errorf("err %v", err)
	}
}
