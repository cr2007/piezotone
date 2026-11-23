---
name: piezotone
description: Turn a MIDI, WAV, MP3, FLAC or Ogg Vorbis file into a one-voice melody for a piezo buzzer with the piezotone CLI, as OpenDisplay buzzer notation, a Nokia RTTTL ringtone or a one-voice MIDI file. Use this skill whenever the user wants a song, jingle, whistle, ringtone or sound file to play on a buzzer, piezo, beeper, Arduino, ESP32, ESPHome or OpenDisplay e-paper device, wants RTTTL from a MIDI or audio file, asks why a file "found no notes", or pipes audio into `opendisplay buzz -melody`, even if they do not name piezotone.
---

# piezotone

piezotone reads an audio or MIDI file and writes a melody with one voice: one note at a time, as a
piezo buzzer plays it. A buzzer plays square-wave tones only, so the goal is the tune, not the sound.

## Install

```sh
go install github.com/cr2007/piezotone/cmd/piezotone@latest
```

It is pure Go with no cgo, so the same command works on Linux, macOS and Windows.

## Pick the output format

| The user wants | Format | Command |
|---|---|---|
| Play it on an OpenDisplay buzzer | `opendisplay` (default) | `opendisplay buzz -melody "$(piezotone song.mid)"` |
| A ringtone for Arduino, ESP32 or ESPHome | `rtttl` | `piezotone -format rtttl -name tune song.mid` |
| A clean one-voice MIDI file | `midi` | `piezotone -format midi -o lead.mid song.mid` |

`-o FILE` writes to a file instead of stdout. With `-format rtttl`, `-name` sets the tune name
(10 characters at most); without it the file name is used.

## Choose the input well

The input decides the result more than any flag, so check it first.

- **MIDI gives the best result.** The notes are exact. piezotone takes the track and channel with
  the most notes and skips the drums (channel 9). When notes overlap, the highest note plays, which
  usually keeps the melody. If the wrong part plays, pick it with `-track N` or `-channel N`.
- **Audio (WAV, MP3, FLAC, Ogg) works for one voice only**: a whistle, a hum, one instrument, a
  ringtone, a simple jingle. piezotone finds the pitch with the YIN method. A full song with chords,
  drums or several instruments has no single pitch, so the result is noise or nothing. Say this to
  the user before they try, and suggest a MIDI version of the song if one exists.

## When piezotone says "found no notes"

The message gives the reason, for example:
`found no notes in 2.5 s of audio: peak -16.2 dBFS, RMS -34.4 dBFS, 4% voiced frames, best YIN dip 0.20`

Read it like this:
- A very low peak or RMS means the file is nearly silent.
- A low share of voiced frames means most of the sound has no stable pitch: speech, noise, drums
  or a sound effect.
- The best YIN dip is how close the clearest frame came to a pitch. The default threshold is 0.15;
  a dip above it is rejected.

To accept rougher pitch, raise the threshold a little at a time: `-threshold 0.3`, then 0.4 or 0.5.
Higher values find more notes but also more wrong ones, so listen to the result. For a sound effect
or speech, one or two notes is often the best possible result. That is the buzzer's limit, not a bug.

`-min-note MS` (default 60) drops notes shorter than MS. Raise it to remove short glitches.

## OpenDisplay limits

The OpenDisplay buzzer plays 400 to 12000 Hz, in 5 ms steps, for at most 30 s and 104 notes (119
without encryption). piezotone moves notes by octaves into range and cuts what does not fit, and
it prints what it changed to stderr. `-max-notes N` sets the cut. A longer tune needs several
buzz commands.

The volume is not part of the melody. On OpenDisplay it is the buzzer duty cycle in the device
config (`duty_percent`; 50% is the loudest). Change it with `opendisplay export-config` and
`write-config`, not with piezotone.

## Examples

```sh
# Play a MIDI file on a display
opendisplay buzz -melody "$(piezotone ode.mid)"

# A whistled tune recorded on a phone
opendisplay buzz -melody "$(piezotone -threshold 0.25 whistle.wav)"

# An RTTTL ringtone for an ESPHome buzzer
piezotone -format rtttl -name doorbell chime.mid

# Keep only the lead line of a MIDI arrangement
piezotone -format midi -track 1 -o lead.mid arrangement.mid
```
