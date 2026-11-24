# piezotone

piezotone turns MIDI and audio files into melodies for piezo buzzers. It is a Go library and a
command. It has no cgo, so it builds for any Go target with `CGO_ENABLED=0`.

```sh
piezotone song.mid                       # OpenDisplay melody notation
piezotone -format rtttl whistle.wav      # Nokia RTTTL ringtone
piezotone -format midi -o one-voice.mid song.mid
opendisplay buzz -melody "$(piezotone song.mid)"
```

## What it does

1. It reads the file and finds one voice:
   - **MIDI**: it plays the track and channel with the most notes. It skips the drums on channel
     9 (10 in General MIDI numbering). When notes overlap, the highest note sounds.
   - **Audio**: every 10 ms it sums the spectrum at the partials of each semitone (harmonic
     summation) and follows the loudest voice with a smooth path (Viterbi). This keeps the
     melody over a quieter backing chord or bass. Then it cuts the pitch track into notes:
     - a note starts at its attack, also when a singer slides up into it;
     - a repeated note starts again (the energy at its partials rises again);
     - a partial taken as a note (an octave or a twelfth too high) gets the right pitch.
   - **Drums** (`-drums`, audio only): a harmonic/percussive split finds the drum hits. A hit
     strikes a held note again, and a hit in a rest becomes a short high tick.
2. It gives note events: `piezotone.Note{MIDI, Start, Duration}`. `MIDI` is a MIDI note number
   (69 is A4, 440 Hz) or `piezotone.Rest`.
3. An output profile writes the notes for a device or a format.

## What it cannot do

- **One voice only.** A piezo buzzer plays one square-wave tone at a time. In MIDI, chords
  become their highest note. In audio, piezotone follows the loudest voice. Full songs in which
  no one voice is clearly the loudest give poor results.
- **No speech, noise or drums as a melody.** These sounds have no stable pitch (`-drums` uses
  drum hits for rhythm only). piezotone then stops with a
  "found no notes" message. The message gives the length, the loudness, the share of voiced
  frames and the best pitch contrast: how many times stronger the best pitch is than the mean
  pitch. A clear tone gives 5 or more; white noise stays below 3. For a rough pitch, try `-voicing 2.5`
  to `2`.
- **No timbre.** A piezo plays square-wave tones only. The output keeps pitch and rhythm only.

## Formats

| `-format` | Package | Output |
|-----------|---------|--------|
| `opendisplay` (default) | `opendisplay` | The melody notation of `opendisplay buzz -melody` from go-opendisplay, for example `A4:200 R:50 G4+:5`. It fits the notes to the buzzer: 400 to 12000 Hz, 5 ms steps, at most 1275 ms per note. `opendisplay buzz` plays a long melody as several frames; `-max-notes` cuts it for an older CLI that sends one frame. |
| `rtttl` | `rtttl` | A Nokia RTTTL ringtone, for example `song:d=8,o=5,b=112:a,4c6`. Octaves 4 to 7. It picks the tempo that fits the note lengths best. Spec: [mobilefish.com RTTTL specification](https://www.mobilefish.com/tutorials/rtttl/rtttl_quickguide_specification.html). |
| `midi` | `midi` | A standard MIDI file (type 0) with one voice. |

## How good is it

A benchmark renders 7 public-domain tunes from MIDI (melody, harmony, pad, bass, drums) and scores
the notes against the MIDI melody with [mir_eval](https://github.com/mir-evaluation/mir_eval)
(onset within 50 ms, pitch within 50 cents).

| Mix | Note F-measure | Pitch correct (frames) |
|-----|----------------|------------------------|
| Lead clearly on top | 0.75 | 81% |
| Lead as loud as the backing | 0.38 | 41% |

In a blind listening test of a phone ringtone (sung lead, harmony, sparse drums), the listener
rated the output 76 of 100 without `-drums` and 89 with it. For a song with a steady hi-hat, the
listener preferred the output without `-drums` (86 against 32).

## OpenDisplay notes

- The melody has no volume. The buzzer volume is the duty cycle in the device config
  (`duty_percent`; 50% is the loudest). Set it with `opendisplay volume -set PERCENT`.
- Tested on one OpenDisplay e-paper display (800 x 480, black and white, firmware 2.27.1) with
  [go-opendisplay](https://github.com/cr2007/go-opendisplay), at duty 10%.

## Decoders

piezotone finds the input format from the first bytes of the file, not from the file name.

| Input | Decoder | License |
|-------|---------|---------|
| MIDI (SMF type 0 and 1) | [gitlab.com/gomidi/midi/v2](https://gitlab.com/gomidi/midi) | MIT |
| WAV (integer PCM 8 to 32 bit, float 32 and 64 bit) | own reader, standard library only | this repository |
| MP3 (MPEG-1/2 Layer III) | [github.com/tphakala/go-mp3](https://github.com/tphakala/go-mp3) | MIT |
| FLAC | [github.com/mewkiz/flac](https://github.com/mewkiz/flac) | Unlicense |
| Ogg Vorbis | [github.com/jfreymuth/oggvorbis](https://github.com/jfreymuth/oggvorbis) | MIT |

[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) lists all linked modules.

## Install

```sh
go install github.com/cr2007/piezotone/cmd/piezotone@latest
```

## Command flags

| Flag | Default | Use |
|------|---------|-----|
| `-format` | `opendisplay` | `opendisplay`, `rtttl` or `midi` |
| `-o FILE` | stdout | write the output to FILE |
| `-track N`, `-channel N` | `-1` (pick) | MIDI: the track and channel to play |
| `-voicing V` | `3` | audio: the pitch contrast a frame needs; a lower value finds more notes |
| `-min-note MS` | `60` | audio: the shortest note |
| `-drums` | off | audio: strike held notes again on drum hits and tick on hits in rests; good for sparse drums, too busy for a steady hi-hat |
| `-max-notes N` | `0` (keep all) | opendisplay: cut at N notes and 30 s, for an opendisplay CLI that sends one frame (104 encrypted, 119 plain) |
| `-name NAME` | file name | rtttl: the tune name, at most 10 characters |

Warnings, for example "moved the melody up by 2 octave(s)", go to stderr.

## Library

```go
data, _ := os.ReadFile("whistle.flac")
samples, rate, err := piezotone.Decode(data)
notes, err := piezotone.FromPCM(samples, rate, piezotone.PCMOptions{})
var nn *piezotone.NoNotesError
if errors.As(err, &nn) {
	// nn.Seconds, nn.RMSDB, nn.Voiced, nn.BestContrast tell why
}
tune, warnings := rtttl.Encode(notes, rtttl.Options{Name: "whistle"})
```

Use `piezotone.FromMIDI` for MIDI files and `piezotone.IsMIDI` to tell the two apart.

## Tests

```sh
go test ./...
```

The audio tests synthesize a reference melody (sine and square waves, 44.1 and 48 kHz, WAV and
MP3) and want a note F-measure of 1.0. With ffmpeg installed, they also check FLAC, Ogg Vorbis
and LAME MP3 files.

## Agent skill

`skills/piezotone/SKILL.md` teaches a coding agent (for example Claude Code) how to use piezotone:
which output format to pick, which inputs work, and how to read the "found no notes" message.
Install it for Claude Code by copying the folder into your skills directory:

```sh
mkdir -p ~/.claude/skills && cp -r skills/piezotone ~/.claude/skills/
```

## License

MIT. See [LICENSE](LICENSE).
