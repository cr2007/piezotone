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
   - **Audio**: it finds the pitch every 10 ms with the YIN method, then cuts the pitch track
     into notes.
2. It gives note events: `piezotone.Note{MIDI, Start, Duration}`. `MIDI` is a MIDI note number
   (69 is A4, 440 Hz) or `piezotone.Rest`.
3. An output profile writes the notes for a device or a format.

## What it cannot do

- **One voice only.** A piezo buzzer plays one square-wave tone at a time. Chords become their
  highest note. Full songs with several instruments give poor results.
- **No speech, noise or drums.** These sounds have no stable pitch. piezotone then stops with a
  "found no notes" message. The message gives the length, the loudness, the share of voiced
  frames and the best YIN dip. A YIN dip near 0 is a clear pitch; a dip near 1 is noise. For a
  rough pitch, try `-threshold 0.3` to `0.5`.
- **No timbre.** A piezo plays square-wave tones only. The output keeps pitch and rhythm only.

## Formats

| `-format` | Package | Output |
|-----------|---------|--------|
| `opendisplay` (default) | `opendisplay` | The melody notation of `opendisplay buzz -melody` from go-opendisplay, for example `A4:200 R:50 G4+:5`. It fits the notes to the buzzer: 400 to 12000 Hz, 5 ms steps, at most 1275 ms per note, 104 notes (`-max-notes`) and 30 s. |
| `rtttl` | `rtttl` | A Nokia RTTTL ringtone, for example `song:d=8,o=5,b=112:a,4c6`. Octaves 4 to 7. It picks the tempo that fits the note lengths best. Spec: [mobilefish.com RTTTL specification](https://www.mobilefish.com/tutorials/rtttl/rtttl_quickguide_specification.html). |
| `midi` | `midi` | A standard MIDI file (type 0) with one voice. |

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
| `-threshold T` | `0.15` | audio: YIN threshold; a higher value finds more notes |
| `-min-note MS` | `60` | audio: the shortest note |
| `-max-notes N` | `104` | opendisplay: the note limit (104 fits one encrypted frame, 119 a plain one) |
| `-name NAME` | file name | rtttl: the tune name, at most 10 characters |

Warnings, for example "moved the melody up by 2 octave(s)", go to stderr.

## Library

```go
data, _ := os.ReadFile("whistle.flac")
samples, rate, err := piezotone.Decode(data)
notes, err := piezotone.FromPCM(samples, rate, piezotone.PCMOptions{})
var nn *piezotone.NoNotesError
if errors.As(err, &nn) {
	// nn.Seconds, nn.RMSDB, nn.Voiced, nn.BestDip tell why
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
