# Piezo response of the OpenDisplay buzzer

Date: 2026-11-25. Question: must piezotone prefer an octave near the piezo resonance, so that the
melody is louder? Answer: no. The data shows no clear loudness gain for the typical note. The
code stays as it is. A blind listening test is ready for a decision by ear.

## Method

- Device: an OpenDisplay display on firmware 2.x with a TDK PS1240P02BT piezo. Buzzer volume
  (duty) 30, set for this test at night. The duty changes the level of all notes, and it can
  change the harmonic balance.
- Sweep: the 59 semitone notes from G#4 (415 Hz) to F#9 (11840 Hz), 300 ms each with a 200 ms
  rest, in two halves of 30 and 29 notes. Each half starts with a B7 marker (300 ms) and a 500 ms
  rest, for time alignment. Three sweeps.
- Microphone: the laptop built-in microphone, 48 kHz mono, at a fixed position (distance not
  measured). 10 s of silence give the noise floor.
- Analysis (numpy, scipy): a 300 Hz high-pass filter, then per note a 200 ms window from 50 ms
  after the note start. "Tone" is the power in the bands of all harmonics up to 20 kHz
  (each band f +/- 1.5 %), minus the noise floor in the same bands. "dBA rel" is the same power
  with IEC 61672-1 A-weighting per harmonic, relative to the loudest note. The tables give the
  median of the 3 sweeps and the spread (maximum minus minimum).
- Datasheet: TDK piezoelectronic buzzer catalog, February 2022, page 4, PS1240P02BT, "frequency
  sound pressure characteristics, square wave drive" (3 V0-P, dB(A) at 10 cm). The curve was
  read from the graph image, 1 kHz to 10 kHz. Copy used:
  https://datasheet.lcsc.com/datasheet/pdf/b82b1254cb55646d45aaa9841e8f9db1.pdf

## Results

- The spread over the 3 sweeps is under 3 dB for most notes. It is up to 12 dB for some notes
  of octaves 4 and 5, near the noise floor.
- Below about 1.2 kHz, the fundamental is weak. The loudest part of a low note is its 3rd to 7th
  harmonic, near 2.6 to 3.1 kHz. The ear still hears the pitch of the fundamental.
- The loudest notes are E7 and F7 (2637 Hz and 2794 Hz). E6 and F6 are also loud, because their
  2nd harmonic is at the same frequency. This peak is not at the 4 kHz datasheet resonance.
- Octave medians (dBA rel): octave 4: -22.9, 5: -21.8, 6: -20.1, 7: -18.5, 8: -20.1, 9: -17.0.
  The level changes little from octave to octave. Single notes near 2.8 kHz stand out.

### Comparison with the datasheet

| Band | Measured median (dBA rel) | Datasheet median (dB rel) |
|---|---|---|
| 1 to 2 kHz | -20.1 | -17.5 |
| 2 to 3 kHz | -17.3 | -18.5 |
| 3 to 4 kHz | -18.8 | -18.7 |
| 4 to 6 kHz | -19.7 | -8.0 |
| 6 to 10 kHz | -17.0 | -12.8 |

The band medians agree within 3 dB from 1 to 4 kHz. Note by note, the curves do not agree
(Pearson r = -0.07 over 40 notes). The datasheet peak at 4.75 kHz is not in the measurement.
Possible causes: the display case and the mount change the resonance; the datasheet drives the
piezo directly at 3 V; the laptop microphone is not flat and its response is not known. This
measurement cannot separate these causes.

### Octave choice on the benchmark melodies

The 7 known-truth melodies of the melody-check benchmark (truth notes, not transcriptions), with
the current octave rule (+0) and one or two more octaves up. Level from the measured curve.

| Measure | +1 octave | +2 octaves |
|---|---|---|
| Duration-weighted median note, mean gain | +1.3 dB (min -0.7) | +2.9 dB (min +1.2) |
| Energy mean over the notes, mean gain | +6.0 dB | +10.6 dB |

The display played two melodies at both octaves (piezotone output, recorded with the same
microphone). Level of the 10 ms frames, A-weighted, above 300 Hz, B (one octave up) minus A
(current rule):

| Melody | 25th percentile | Median | 75th percentile | Energy mean |
|---|---|---|---|---|
| Phone ringtone | +4.4 dB | +1.6 dB | +0.8 dB | +10.1 dB |
| When the Saints Go Marching In | +0.8 dB | -0.1 dB | +7.7 dB | +8.5 dB |

The recordings agree with the curve: the typical moment is about as loud, and a few notes are
much louder.

The energy mean comes from a few notes near 2.8 kHz. They become louder than the notes around
them, so the melody becomes less even. The median note gains 1 to 3 dB. That is within the
uncertainty of the microphone. A higher octave also moves the melody away from the original
pitch (the octave error counts in raw pitch accuracy), and the user reported "screech" on high
notes in the blind test of 2026-11-24. Decision: no code change.

## Limits

- One display, one microphone position, one duty (30), one room with some background noise.
- The laptop microphone is not calibrated. Use the levels to compare notes, not as sound
  pressure.
- A-weighting is a simple loudness model. It does not model masking or the loudness of a
  sequence of notes.

## Blind test (prepared, not run)

Trials "ring" (phone ringtone) and "saints" (benchmark song). Clips: the original, A = current
octave rule, B = one octave up. Both are recorded from the display. The clips are loudness
matched, as the blind-listening-test skill requires. So the test asks which octave sounds more
like the original, not which one is louder.

## Curve

| Note | Hz | dBA rel | dBA spread | Tone dB above floor | Dominant Hz |
|---|---|---|---|---|---|
| G#4 | 415 | -25.8 | 6.2 | 14.8 | 2905 |
| A4 | 440 | -23.1 | 2.5 | 16.7 | 2640 |
| A#4 | 466 | -19.5 | 0.3 | 20.9 | 2795 |
| B4 | 494 | -22.7 | 1.3 | 17.6 | 2965 |
| C5 | 523 | -18.8 | 0.9 | 22.5 | 2615 |
| C#5 | 554 | -14.4 | 1.7 | 25.7 | 2770 |
| D5 | 587 | -24.4 | 10.5 | 17.1 | 2935 |
| D#5 | 622 | -21.7 | 4.0 | 21.1 | 3110 |
| E5 | 659 | -20.0 | 1.3 | 21.3 | 2635 |
| F5 | 698 | -15.6 | 2.2 | 26.7 | 2790 |
| F#5 | 740 | -21.8 | 12.6 | 21.2 | 2960 |
| G5 | 784 | -24.6 | 6.9 | 19.5 | 9410 |
| G#5 | 831 | -24.9 | 4.7 | 19.8 | 9140 |
| A5 | 880 | -22.4 | 1.3 | 21.4 | 2640 |
| A#5 | 932 | -18.6 | 1.5 | 24.7 | 2795 |
| B5 | 988 | -23.6 | 2.9 | 19.4 | 2965 |
| C6 | 1047 | -24.4 | 1.4 | 22.0 | 9425 |
| C#6 | 1109 | -27.1 | 3.4 | 18.9 | 9980 |
| D6 | 1175 | -20.9 | 5.6 | 23.2 | 9400 |
| D#6 | 1245 | -20.9 | 0.6 | 25.7 | 9960 |
| E6 | 1319 | -11.0 | 0.3 | 31.8 | 2640 |
| F6 | 1397 | -3.2 | 1.4 | 41.7 | 2795 |
| F#6 | 1480 | -14.9 | 4.1 | 30.6 | 2960 |
| G6 | 1568 | -19.2 | 1.0 | 28.2 | 3135 |
| G#6 | 1661 | -19.7 | 0.6 | 26.4 | 3320 |
| A6 | 1760 | -22.1 | 1.9 | 25.9 | 8800 |
| A#6 | 1865 | -19.8 | 1.7 | 29.4 | 9325 |
| B6 | 1976 | -20.5 | 0.6 | 26.9 | 9880 |
| C7 | 2093 | -24.2 | 1.8 | 24.4 | 10465 |
| C#7 | 2217 | -22.8 | 2.1 | 27.1 | 11085 |
| D7 | 2349 | -18.3 | 1.0 | 30.1 | 2350 |
| D#7 | 2489 | -17.3 | 0.5 | 30.8 | 2490 |
| E7 | 2637 | -4.3 | 0.2 | 44.4 | 2635 |
| F7 | 2794 | 0.0 | 1.1 | 48.1 | 2795 |
| F#7 | 2960 | -12.9 | 3.8 | 35.5 | 2960 |
| G7 | 3136 | -18.7 | 0.9 | 31.3 | 3135 |
| G#7 | 3322 | -16.2 | 0.5 | 33.2 | 3320 |
| A7 | 3520 | -18.8 | 0.2 | 32.5 | 7040 |
| A#7 | 3729 | -19.1 | 1.6 | 32.3 | 7455 |
| B7 | 3951 | -26.1 | 3.1 | 21.4 | 7900 |
| C8 | 4186 | -23.4 | 0.3 | 27.1 | 4185 |
| C#8 | 4435 | -19.7 | 1.2 | 32.0 | 8870 |
| D8 | 4699 | -14.8 | 0.5 | 37.9 | 9400 |
| D#8 | 4978 | -12.4 | 3.7 | 41.7 | 9955 |
| E8 | 5274 | -21.0 | 1.7 | 35.2 | 10550 |
| F8 | 5588 | -18.0 | 0.2 | 39.3 | 11175 |
| F#8 | 5920 | -25.0 | 0.8 | 29.6 | 5920 |
| G8 | 6272 | -20.6 | 0.8 | 34.8 | 6275 |
| G#8 | 6645 | -21.8 | 1.4 | 34.0 | 6645 |
| A8 | 7040 | -16.9 | 0.3 | 38.9 | 7040 |
| A#8 | 7459 | -16.6 | 1.5 | 38.4 | 7455 |
| B8 | 7902 | -22.3 | 3.1 | 34.0 | 7900 |
| C9 | 8372 | -26.8 | 2.2 | 28.0 | 8375 |
| C#9 | 8870 | -17.0 | 2.7 | 38.2 | 8875 |
| D9 | 9397 | -11.6 | 1.0 | 43.7 | 9400 |
| D#9 | 9956 | -8.7 | 2.8 | 48.3 | 9960 |
| E9 | 10548 | -17.7 | 0.8 | 42.3 | 10550 |
| F9 | 11175 | -14.5 | 0.5 | 46.4 | 11175 |
| F#9 | 11840 | -32.4 | 7.0 | 28.6 | 11835 |
