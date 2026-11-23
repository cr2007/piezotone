package piezotone

import (
	"errors"
	"fmt"
	"math"
	"math/cmplx"
	"slices"
)

// PCMOptions tunes FromPCM. Zero values use the defaults in brackets.
type PCMOptions struct {
	MinHz, MaxHz float64 // pitch search range [80, 5000]
	Voicing      float64 // a frame whose best pitch is less than this many times the mean is unvoiced [3]
	SilenceDB    float64 // a frame this far below the loudest frame is silent [-30]
	MinNoteMS    int     // shorter segments join the segment before them [60]
}

const (
	hopMS     = 10   // one pitch estimate per 10 ms
	harmonics = 8    // partials summed per candidate pitch
	decay     = 0.8  // weight of partial h is decay^(h-1)
	jumpCost  = 0.15 // log-salience cost per semitone of pitch change between frames
)

// FromPCM finds the main melody of mono samples and cuts it into notes. For
// each 10 ms frame it sums the spectrum at the first partials of every
// semitone (harmonic summation, Klapuri, ISMIR 2006), then picks the
// smoothest strong path through the frames (Viterbi). This keeps the melody
// over a quieter backing chord or bass. It does not work for music in which
// no one voice is clearly the loudest. When it finds no pitched note, the
// error is a *NoNotesError.
func FromPCM(samples []float32, rate int, o PCMOptions) ([]Note, error) {
	if rate <= 0 {
		return nil, errors.New("piezotone: sample rate must be positive")
	}
	o.MinHz = cmp0(o.MinHz, 80)
	o.MaxHz = cmp0(o.MaxHz, 5000)
	o.Voicing = cmp0(o.Voicing, 3)
	o.SilenceDB = cmp0(o.SilenceDB, -30)
	o.MinNoteMS = int(cmp0(float64(o.MinNoteMS), 60))

	lo, hi := max(1, int(math.Ceil(69+12*math.Log2(o.MinHz/440)))), int(math.Floor(69+12*math.Log2(o.MaxHz/440)))
	if lo > hi || MIDIHz(lo) >= float64(rate)/2 {
		return nil, errors.New("piezotone: the pitch range is empty at this sample rate")
	}
	size := 1 // the largest power of two up to 100 ms: 4096 at 44.1 and 48 kHz
	for size*2 <= rate/10 {
		size *= 2
	}
	hop := rate * hopMS / 1000
	// Frame i is centered on sample i*hop; zeros pad both ends.
	n := (len(samples) + hop - 1) / hop
	padded := make([]float32, size+len(samples)+size)
	copy(padded[size:], samples)

	// Frame energy over 25 ms at the frame center, to find silence and short rests.
	ew := rate * 25 / 1000
	rms := make([]float64, n)
	peak := 0.0
	for i := range rms {
		c := size + i*hop
		e := 0.0
		for _, v := range padded[c-ew/2 : c+ew/2] {
			e += float64(v) * float64(v)
		}
		rms[i] = math.Sqrt(e / float64(ew))
		peak = max(peak, rms[i])
	}
	floor := peak * math.Pow(10, o.SilenceDB/20)

	// Salience of each candidate semitone in each frame (Salamon and Gomez,
	// IEEE TASLP 20(6), 2012): each spectral peak adds to the candidates of
	// which it can be partial h, weighted by decay^(h-1) and by cos^2 of the
	// distance in semitones, up to one semitone.
	nc := hi - lo + 1
	win := make([]float64, size)
	for i := range win {
		win[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(size)) // Hann
	}
	nn := &NoNotesError{Seconds: float64(len(samples)) / float64(rate), Frames: n, Voicing: o.Voicing}
	logSal := make([][]float64, n) // nil for a frame that is silent or unvoiced
	buf := make([]complex128, size)
	mag := make([]float64, size/2)
	sal := make([]float64, nc)
	for i := range logSal {
		if rms[i] <= floor || peak == 0 {
			continue
		}
		for j := range buf {
			buf[j] = complex(float64(padded[size/2+i*hop+j])*win[j], 0)
		}
		fft(buf)
		top := 0.0
		for j := range mag {
			mag[j] = cmplx.Abs(buf[j])
			top = max(top, mag[j])
		}
		clear(sal)
		for j := 1; j < len(mag)-1; j++ {
			// A local maximum within 40 dB of the highest peak is a partial.
			if mag[j] <= mag[j-1] || mag[j] < mag[j+1] || mag[j] < top/100 {
				continue
			}
			// Parabolic interpolation on the log magnitude.
			a, b, c := math.Log(mag[j-1]+1e-12), math.Log(mag[j]+1e-12), math.Log(mag[j+1]+1e-12)
			d := 0.0
			if den := a - 2*b + c; den < 0 {
				d = 0.5 * (a - c) / den
			}
			pitch := 69 + 12*math.Log2((float64(j)+d)*float64(rate)/float64(size)/440)
			amp, w := math.Exp(b-0.25*(a-c)*d), 1.0
			for h := 1; h <= harmonics; h++ {
				f0 := pitch - 12*math.Log2(float64(h))
				for k := int(math.Floor(f0)); k <= int(math.Ceil(f0)); k++ {
					if dist := math.Abs(f0 - float64(k)); k >= lo && k <= hi && dist < 1 {
						cw := math.Cos(math.Pi / 2 * dist)
						sal[k-lo] += w * cw * cw * amp
					}
				}
				w *= decay
			}
		}
		best, sum := 0.0, 0.0
		for _, v := range sal {
			best, sum = max(best, v), sum+v
		}
		contrast := best / (sum / float64(nc))
		nn.BestContrast = max(nn.BestContrast, contrast)
		if best == 0 || contrast < o.Voicing {
			continue
		}
		nn.Voiced++
		logSal[i] = make([]float64, nc)
		for c, s := range sal {
			logSal[i][c] = math.Log(s/best + 1e-6)
		}
	}

	labels := make([]int, n) // MIDI note per frame, 0 for no pitch
	for _, run := range voicedRuns(logSal) {
		for k, c := range viterbi(logSal[run[0]:run[1]]) {
			labels[run[0]+k] = lo + c
		}
	}
	segs := segment(median(labels, 5), max(1, o.MinNoteMS/hopMS), len(samples)*1000/rate)
	var steps []step
	for _, s := range segs {
		m := Rest
		if s.midi > 0 {
			m = s.midi
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
	BestContrast  float64 // highest ratio of best pitch to mean pitch of all frames that are not silent; 0 if all are silent
	Voicing       float64 // voicing threshold in use
}

func (e *NoNotesError) Error() string {
	pct := 0.0
	if e.Frames > 0 {
		pct = 100 * float64(e.Voiced) / float64(e.Frames)
	}
	return fmt.Sprintf("piezotone: found no notes in %.1f s of audio: peak %.1f dBFS, RMS %.1f dBFS, %.0f%% voiced frames, best pitch contrast %.1f (voicing %.1f)",
		e.Seconds, e.PeakDB, e.RMSDB, pct, e.BestContrast, e.Voicing)
}

// voicedRuns returns the [start, end) ranges of frames that are not nil.
func voicedRuns(frames [][]float64) [][2]int {
	var runs [][2]int
	for i := 0; i < len(frames); i++ {
		if frames[i] == nil {
			continue
		}
		j := i
		for j < len(frames) && frames[j] != nil {
			j++
		}
		runs = append(runs, [2]int{i, j})
		i = j
	}
	return runs
}

// viterbi returns the candidate per frame that maximizes the summed log
// salience minus jumpCost per semitone of change between frames.
func viterbi(frames [][]float64) []int {
	nc := len(frames[0])
	score := slices.Clone(frames[0])
	next := make([]float64, nc)
	back := make([][]int, len(frames))
	for t := 1; t < len(frames); t++ {
		back[t] = make([]int, nc)
		for c := range nc {
			best, arg := math.Inf(-1), 0
			for p := range nc {
				if s := score[p] - jumpCost*math.Abs(float64(c-p)); s > best {
					best, arg = s, p
				}
			}
			next[c], back[t][c] = best+frames[t][c], arg
		}
		score, next = next, score
	}
	path := make([]int, len(frames))
	path[len(path)-1] = slices.Index(score, slices.Max(score))
	for t := len(frames) - 1; t > 0; t-- {
		path[t-1] = back[t][path[t]]
	}
	return path
}

// fft is an in-place radix-2 Cooley-Tukey FFT; len(x) is a power of two.
func fft(x []complex128) {
	n := len(x)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j |= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		w := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			wk := complex(1, 0)
			for k := range size / 2 {
				a, b := x[start+k], x[start+k+size/2]*wk
				x[start+k], x[start+k+size/2] = a+b, a-b
				wk *= w
			}
		}
	}
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
// joins the segment before it. totalMS is the length of the audio.
func segment(labels []int, minFrames, totalMS int) []step {
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
	used := 0
	for _, s := range out {
		ms := s.frames * hopMS
		notes = append(notes, step{s.label, ms})
		used += ms
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
