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
	Drums        bool    // strike held notes again on drum hits and put a tick on hits in rests
}

// TickMIDI is the pitch of a drum tick in a rest (C7).
const TickMIDI = 96

const (
	hopMS     = 10   // one pitch estimate per 10 ms
	harmonics = 8    // partials summed per candidate pitch
	decay     = 0.8  // weight of partial h is decay^(h-1)
	jumpCost  = 0.15 // log-salience cost per semitone of pitch change between frames

	snapBackMS  = 80  // a note start moves back to an attack up to this far
	splitRise   = 2.0 // summed log-magnitude rise at a note's partials that starts it again
	strikeGapMS = 25  // drums: the silence before a note is struck again
	tickMS      = 15  // drums: the length of a tick in a rest
	scoopMS     = 80  // a shorter note that slides into the next note joins it
)

// FromPCM finds the main melody of mono samples and cuts it into notes. For
// each 10 ms frame it sums the spectrum at the first partials of every
// semitone (harmonic summation, Klapuri, ISMIR 2006), then picks the
// smoothest strong path through the frames (Viterbi). This keeps the melody
// over a quieter backing chord or bass. It does not work for music in which
// no one voice is clearly the loudest. When it finds no pitched note, the
// error is a *NoNotesError. A sample that is NaN or infinite counts as 0.
func FromPCM(samples []float32, rate int, o PCMOptions) ([]Note, error) {
	if rate <= 0 || rate > 1_000_000 { // the FFT size grows with the rate; audio files use at most 768 kHz
		return nil, fmt.Errorf("piezotone: sample rate %d Hz is not in 1 Hz to 1 MHz", rate)
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
	for i, v := range padded[size : size+len(samples)] {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) { // a float WAV can hold them
			padded[size+i] = 0
		}
	}

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
	// Note starts and repeats come from a short (23 ms) spectrum: the 90 ms window of the
	// pitch analysis puts a scooped note about 50 ms late and merges repeated notes.
	L := shortLogSpec(padded, size, hop, n, rate)
	minF := max(1, o.MinNoteMS/hopMS)
	onsets := onsetPeaks(L)
	steps = splitRepeats(steps, L, rate, minF)
	steps = absorbScoops(steps)
	steps = snapStarts(steps, onsets, minF)
	steps = fixSpikes(steps)
	if o.Drums {
		steps = addDrums(steps, drumOnsets(L))
	}
	if !slices.ContainsFunc(steps, func(s step) bool { return s.midi != Rest }) {
		var e float64
		for _, v := range padded[size : size+len(samples)] {
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

// shortLogSpec returns log(1 + 100|X|/size) of a short Hann STFT (the largest power of two up to
// 25 ms) centered on each frame.
func shortLogSpec(padded []float32, size, hop, n, rate int) [][]float64 {
	ss := 1
	for ss*2 <= rate*25/1000 {
		ss *= 2
	}
	win := make([]float64, ss)
	for i := range win {
		win[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(ss))
	}
	buf := make([]complex128, ss)
	L := make([][]float64, n)
	for i := range L {
		c := size + i*hop - ss/2
		for j := range buf {
			buf[j] = complex(float64(padded[c+j])*win[j], 0)
		}
		fft(buf)
		L[i] = make([]float64, ss/2)
		for j := range L[i] {
			L[i][j] = math.Log1p(100 * cmplx.Abs(buf[j]) / float64(ss))
		}
	}
	return L
}

// onsetPeaks marks attacks: frames where the broadband spectral flux (normalized to 1) is the
// largest within 30 ms and exceeds the mean of the 200 ms around it by 0.05 (after the peak
// picking of Bock, Krebs and Schedl, ISMIR 2012).
func onsetPeaks(L [][]float64) []bool {
	n := len(L)
	f := make([]float64, n)
	top := 0.0
	for i := 1; i < n; i++ {
		for j := range L[i] {
			f[i] += math.Max(0, L[i][j]-L[i-1][j])
		}
		top = math.Max(top, f[i])
	}
	pk := make([]bool, n)
	if top == 0 {
		return pk
	}
	for i := range f {
		m, c, ok := 0.0, 0, true
		for j := max(0, i-10); j <= min(n-1, i+10); j++ {
			m += f[j]
			c++
			if j >= i-3 && j <= i+3 && f[j] > f[i] {
				ok = false
			}
		}
		pk[i] = ok && f[i]/top > m/float64(c)/top+0.05
	}
	return pk
}

// snapStarts moves the start of each pitched step back to the nearest attack up to snapBackMS
// before it, when the step before stays at least minF frames long (a rest at least one frame).
// Scoops and slow attacks make a note late, never early, so a start never moves later.
func snapStarts(steps []step, pk []bool, minF int) []step {
	f := 0
	for k := range steps {
		n := steps[k].ms / hopMS
		if k > 0 && steps[k].midi != Rest {
			best := -1
			for d := -snapBackMS / hopMS; d <= 0; d++ {
				if i := f + d; i >= 0 && i < len(pk) && pk[i] && (best < 0 || iabs(d) < iabs(best-f)) {
					best = i
				}
			}
			if best >= 0 && best != f {
				shift := best - f
				prev := steps[k-1].ms/hopMS + shift
				if n-shift >= 1 && (prev >= minF || (steps[k-1].midi == Rest && prev >= 1)) {
					steps[k-1].ms += shift * hopMS
					steps[k].ms -= shift * hopMS
					n -= shift
					f += shift
				}
			}
		}
		f += n
	}
	return steps
}

// splitRepeats starts a pitched step again where the energy at its first six partials rises,
// more than the energy between the partials rises, by splitRise in summed log magnitude: a
// repeated note. A drum hit lifts both, so it does not split a note.
func splitRepeats(steps []step, L [][]float64, rate, minF int) []step {
	ss := 2 * len(L[0])
	band := func(hz float64) [2]int {
		return [2]int{int(hz * math.Pow(2, -0.5/12) * float64(ss) / float64(rate)), int(math.Ceil(hz * math.Pow(2, 0.5/12) * float64(ss) / float64(rate)))}
	}
	rise := func(b [2]int, i int) float64 {
		x, y := 0.0, 0.0
		for j := b[0]; j <= b[1]; j++ {
			x, y = math.Max(x, L[i][j]), math.Max(y, L[i-2][j])
		}
		return math.Max(0, x-y)
	}
	var out []step
	f := 0
	for _, s := range steps {
		k := s.ms / hopMS
		if s.midi == Rest || k < 2*minF {
			out = append(out, s)
			f += k
			continue
		}
		var parts, mids [][2]int
		for h := 1; h <= 6; h++ {
			p, m := band(MIDIHz(s.midi)*float64(h)), band(MIDIHz(s.midi)*(float64(h)+0.5))
			if m[1] >= ss/2 {
				break
			}
			parts, mids = append(parts, p), append(mids, m)
		}
		pf := func(i int) float64 {
			v := 0.0
			for h := range parts {
				v += math.Max(0, rise(parts[h], i)-rise(mids[h], i))
			}
			return v
		}
		last := f
		for i := f + minF; i <= f+k-minF && i+1 < len(L); i++ {
			if i-last >= minF && i >= 2 && pf(i) > splitRise && pf(i) >= pf(i-1) && pf(i) >= pf(i+1) {
				out = append(out, step{s.midi, (i - last) * hopMS})
				last = i
			}
		}
		out = append(out, step{s.midi, s.ms - (last-f)*hopMS})
		f += k
	}
	return out
}

// absorbScoops joins a pitched step of scoopMS or less to the next step when that step is
// pitched, at least twice as long and within 2 semitones: the slide into a sung note is part of
// it (the attack state of the pYIN note model, Mauch et al. 2015).
func absorbScoops(steps []step) []step {
	var out []step
	for k := 0; k < len(steps); k++ {
		s := steps[k]
		if k+1 < len(steps) && s.midi != Rest && s.ms <= scoopMS {
			if nx := steps[k+1]; nx.midi != Rest && iabs(nx.midi-s.midi) <= 2 && nx.ms >= 2*s.ms {
				steps[k+1].ms += s.ms
				continue
			}
		}
		out = append(out, s)
	}
	return out
}

// fixSpikes gives a note the pitch of a pitched neighbor when the note lies 12 or more
// semitones above both neighbors and exactly 12, 19 or 24 above one of them: the 2nd, 3rd or
// 4th partial of that neighbor taken as a note.
func fixSpikes(steps []step) []step {
	nb := func(i, d int) int {
		for j := i + d; j >= 0 && j < len(steps); j += d {
			if steps[j].midi != Rest {
				return steps[j].midi
			}
		}
		return Rest
	}
	for i, s := range steps {
		if s.midi == Rest {
			continue
		}
		for _, d := range []int{-1, 1} {
			p, q := nb(i, d), nb(i, -d)
			if p == Rest || (q != Rest && s.midi-q < 12) {
				continue
			}
			if x := s.midi - p; x == 12 || x == 19 || x == 24 {
				steps[i].midi = p
				break
			}
		}
	}
	return steps
}

// drumOnsets finds drum hits. A harmonic/percussive split by median filtering (Fitzgerald,
// DAFx 2010) keeps the bins that stand out across frequency more than 3 times as much as
// across time; the flux of that part is peak-picked as in librosa.onset.onset_detect
// (pre_max 30 ms, pre_avg and post_avg 100 ms, delta 0.07 after normalization, wait 30 ms).
func drumOnsets(L [][]float64) []bool {
	n, nb := len(L), len(L[0])
	const half = 8 // median over 17 frames or bins
	buf := make([]float64, 0, 2*half+1)
	med := func(get func(int) float64, lo, hi int) float64 {
		buf = buf[:0]
		for i := lo; i <= hi; i++ {
			buf = append(buf, get(i))
		}
		slices.Sort(buf)
		return buf[len(buf)/2]
	}
	env := make([]float64, n)
	prev := make([]float64, nb)
	for t := range n {
		cur := make([]float64, nb)
		for k := range nb {
			// log1p is monotonic, so a median of L is L of the median magnitude.
			h := math.Expm1(med(func(i int) float64 { return L[i][k] }, max(0, t-half), min(n-1, t+half)))
			p := math.Expm1(med(func(i int) float64 { return L[t][i] }, max(0, k-half), min(nb-1, k+half)))
			m := 0.0
			if p+h > 0 {
				m = p * p / (p*p + 9*h*h) // soft mask, margin 3, power 2
			}
			cur[k] = math.Log1p(math.Expm1(L[t][k]) * m)
			if t > 0 {
				env[t] += math.Max(0, cur[k]-prev[k])
			}
		}
		prev = cur
	}
	lo, hi := slices.Min(env), slices.Max(env)
	pk := make([]bool, n)
	if hi <= lo {
		return pk
	}
	last := -100
	for t := range n {
		e := (env[t] - lo) / (hi - lo)
		m, c, ok := 0.0, 0, true
		for j := max(0, t-10); j <= min(n-1, t+10); j++ {
			m += (env[j] - lo) / (hi - lo)
			c++
			if j >= t-3 && j <= t && env[j] > env[t] {
				ok = false
			}
		}
		if ok && e >= m/float64(c)+0.07 && t-last >= 3 {
			pk[t], last = true, t
		}
	}
	return pk
}

// addDrums turns attacks into rhythm: an attack inside a held note (80 ms after its start and
// 60 ms before its end) strikes the note again after strikeGapMS of silence; an attack in a
// rest becomes a tickMS tick at TickMIDI. Short drum blips follow Shiru, "PC Speaker To
// Eleven" (2019); a blind test preferred this for a ringtone with sparse drums.
func addDrums(steps []step, pk []bool) []step {
	var out []step
	t := 0
	for _, s := range steps {
		cur := t
		for i, on := range pk {
			at := i * hopMS
			if !on {
				continue
			}
			if s.midi != Rest && at > t+80 && at < t+s.ms-60 && at-strikeGapMS > cur {
				out = append(out, step{s.midi, at - strikeGapMS - cur}, step{Rest, strikeGapMS})
				cur = at
			}
			if s.midi == Rest && at >= cur && at+tickMS < t+s.ms {
				if at > cur {
					out = append(out, step{Rest, at - cur})
				}
				out = append(out, step{TickMIDI, tickMS})
				cur = at + tickMS
			}
		}
		if t+s.ms > cur {
			out = append(out, step{s.midi, t + s.ms - cur})
		}
		t += s.ms
	}
	return out
}

func iabs(x int) int { return max(x, -x) }
