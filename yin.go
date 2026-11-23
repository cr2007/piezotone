package piezotone

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

// PCMOptions tunes FromPCM. Zero values use the defaults in brackets.
type PCMOptions struct {
	MinHz, MaxHz float64 // pitch search range [80, 5000]
	Threshold    float64 // YIN absolute threshold [0.15]; a frame with no dip below it is unvoiced
	SilenceDB    float64 // a frame this far below the loudest frame is silent [-30]
	MinNoteMS    int     // shorter segments join the segment before them [60]
}

const hopMS = 10 // one pitch estimate per 10 ms

// FromPCM tracks the pitch of mono samples with YIN (de Cheveigne and
// Kawahara, JASA 111(4), 2002) and cuts the track into notes. It works for
// one voice at a time: a whistle, a single instrument or a ringtone. It does
// not work for music with chords or several instruments. When it finds no
// pitched note, the error is a *NoNotesError.
func FromPCM(samples []float32, rate int, o PCMOptions) ([]Note, error) {
	if rate <= 0 {
		return nil, errors.New("piezotone: sample rate must be positive")
	}
	o.MinHz = cmp0(o.MinHz, 80)
	o.MaxHz = cmp0(o.MaxHz, 5000)
	o.Threshold = cmp0(o.Threshold, 0.15)
	o.SilenceDB = cmp0(o.SilenceDB, -30)
	o.MinNoteMS = int(cmp0(float64(o.MinNoteMS), 60))

	tauMin := max(2, int(float64(rate)/o.MaxHz))
	tauMax := int(float64(rate) / o.MinHz)
	w := 2 * tauMax // integration window: two periods of the lowest pitch
	hop := rate * hopMS / 1000
	if tauMin >= tauMax {
		return nil, errors.New("piezotone: the pitch range is empty at this sample rate")
	}

	// Frame energy first, to find the loudest frame.
	n := 0
	if len(samples) >= w+tauMax+1 {
		n = (len(samples)-w-tauMax-1)/hop + 1
	}
	rms := make([]float64, n)
	peak := 0.0
	for i := range rms {
		e := 0.0
		for _, v := range samples[i*hop : i*hop+w] {
			e += float64(v) * float64(v)
		}
		rms[i] = math.Sqrt(e / float64(w))
		peak = max(peak, rms[i])
	}
	floor := peak * math.Pow(10, o.SilenceDB/20)

	labels := make([]int, n) // pitch per frame in quarter tones, 0 for no pitch
	d := make([]float64, tauMax+2)
	nn := &NoNotesError{Seconds: float64(len(samples)) / float64(rate), Frames: n, BestDip: math.Inf(1), Threshold: o.Threshold}
	for i := range labels {
		if rms[i] <= floor || peak == 0 {
			continue
		}
		hz, dip := yin(samples[i*hop:], w, tauMin, tauMax, o.Threshold, d)
		nn.BestDip = min(nn.BestDip, dip)
		if hz > 0 {
			labels[i] = int(math.Round(24 * math.Log2(hz*float64(rate)/13.75)))
			nn.Voiced++
		}
	}
	// Frame i looks at the audio around i*hop + w/2.
	segs := segment(median(labels, 5), max(1, o.MinNoteMS/hopMS), w*500/rate, len(samples)*1000/rate)
	var steps []step
	for _, s := range segs {
		m := Rest
		if s.midi > 0 {
			m = int(math.Round(float64(s.midi)/2)) + 9 // 13.75 Hz is MIDI 9
		}
		if len(steps) > 0 && steps[len(steps)-1].midi == m { // two quarter tones round to one note
			steps[len(steps)-1].ms += s.ms
			continue
		}
		steps = append(steps, step{m, s.ms})
	}
	if !slices.ContainsFunc(steps, func(s step) bool { return s.midi != Rest }) {
		var e float64
		for _, v := range samples {
			e += float64(v) * float64(v)
			nn.PeakDB = max(nn.PeakDB, math.Abs(float64(v)))
		}
		nn.RMSDB = 20 * math.Log10(math.Sqrt(e/float64(max(1, len(samples)))))
		nn.PeakDB = 20 * math.Log10(nn.PeakDB)
		return nil, nn
	}
	return timed(steps), nil
}

// NoNotesError tells why FromPCM found no note.
type NoNotesError struct {
	Seconds       float64 // length of the audio
	PeakDB, RMSDB float64 // loudness in dBFS
	Frames        int     // 10 ms analysis frames
	Voiced        int     // frames with a pitch
	BestDip       float64 // lowest YIN dip of all frames that are not silent; +Inf if all are silent
	Threshold     float64 // YIN threshold in use
}

func (e *NoNotesError) Error() string {
	pct := 0.0
	if e.Frames > 0 {
		pct = 100 * float64(e.Voiced) / float64(e.Frames)
	}
	return fmt.Sprintf("piezotone: found no notes in %.1f s of audio: peak %.1f dBFS, RMS %.1f dBFS, %.0f%% voiced frames, best YIN dip %.2f (threshold %.2f)",
		e.Seconds, e.PeakDB, e.RMSDB, pct, e.BestDip, e.Threshold)
}

// yin returns the pitch of x[:w] in cycles per sample, or 0 when the frame
// is aperiodic, and the lowest value of the normalized difference function
// in the search range. d is scratch space of tauMax+2 values.
func yin(x []float32, w, tauMin, tauMax int, threshold float64, d []float64) (float64, float64) {
	// Steps 2 and 3: difference function and cumulative mean normalization.
	d[0] = 1
	sum := 0.0
	for tau := 1; tau <= tauMax+1; tau++ {
		s := 0.0
		for j := range w {
			v := float64(x[j]) - float64(x[j+tau])
			s += v * v
		}
		sum += s
		d[tau] = 1
		if sum > 0 {
			d[tau] = s * float64(tau) / sum
		}
	}
	dip := slices.Min(d[tauMin : tauMax+1])
	// Step 4: the first dip below the threshold, down to its local minimum.
	for tau := tauMin; tau <= tauMax; tau++ {
		if d[tau] >= threshold {
			continue
		}
		for tau < tauMax && d[tau+1] < d[tau] {
			tau++
		}
		// Step 5: parabolic interpolation.
		t := float64(tau)
		if den := d[tau-1] - 2*d[tau] + d[tau+1]; den > 0 {
			t += (d[tau-1] - d[tau+1]) / (2 * den)
		}
		return 1 / t, dip
	}
	return 0, dip
}

// median filters labels with an odd window, to remove one-frame glitches.
func median(labels []int, win int) []int {
	out := make([]int, len(labels))
	buf := make([]int, 0, win)
	for i := range labels {
		buf = append(buf[:0], labels[max(0, i-win/2):min(len(labels), i+win/2+1)]...)
		slices.Sort(buf)
		out[i] = buf[len(buf)/2]
	}
	return out
}

// segment turns frame labels into steps; label 0 is a rest. A segment shorter than minFrames
// joins the segment before it. padMS is the time before the first frame,
// and totalMS is the length of the audio.
func segment(labels []int, minFrames, padMS, totalMS int) []step {
	type seg struct{ label, frames int }
	var segs []seg
	for _, l := range labels {
		switch {
		case len(segs) > 0 && segs[len(segs)-1].label == l:
			segs[len(segs)-1].frames++
		default:
			segs = append(segs, seg{l, 1})
		}
	}
	var out []seg
	for i, s := range segs {
		switch {
		case s.frames < minFrames && len(out) > 0:
			out[len(out)-1].frames += s.frames
		case s.frames < minFrames && i+1 < len(segs) && s.label != 0: // a short rest at the start stays
			segs[i+1].frames += s.frames
		case len(out) > 0 && out[len(out)-1].label == s.label:
			out[len(out)-1].frames += s.frames
		default:
			out = append(out, s)
		}
	}
	var notes []step
	used := padMS
	for _, s := range out {
		ms := s.frames * hopMS
		notes = append(notes, step{s.label, ms})
		used += ms
	}
	if len(notes) > 0 {
		notes[0].ms += padMS
	}
	if len(notes) > 0 && totalMS > used { // frames do not cover the tail of the audio
		notes[len(notes)-1].ms += totalMS - used
	}
	return notes
}

func cmp0(v, def float64) float64 {
	if v == 0 {
		return def
	}
	return v
}
