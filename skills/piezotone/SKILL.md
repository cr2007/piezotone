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
- **Audio (WAV, MP3, FLAC, Ogg) works when one voice is the loudest**: a whistle, a hum, one
  instrument, a ringtone or a jingle, also over a quieter backing chord or bass. piezotone follows
  the loudest pitch. In a full song with drums, vocals and several instruments, no one voice is
  clearly the loudest, so the result is poor. Say this to the user before they try, and suggest a
  MIDI version of the song if one exists.

## When piezotone says "found no notes"

The message gives the reason, for example:
`found no notes in 2.0 s of audio: peak -6.0 dBFS, RMS -10.8 dBFS, 0% voiced frames, best pitch contrast 2.9 (voicing 3.0)`

Read it like this:
- A very low peak or RMS means the file is nearly silent.
- A low share of voiced frames means most of the sound has no stable pitch: speech, noise, drums
  or a sound effect.
- The best pitch contrast is how many times stronger the best pitch of the clearest frame was
  than the mean pitch. A clear tone gives 5 or more; white noise stays below 3. A frame needs the
  `-voicing` value (default 3).

To accept rougher pitch, lower the voicing a little at a time: `-voicing 2.5`, then 2.
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
opendisplay buzz -melody "$(piezotone -voicing 2.5 whistle.wav)"

# An RTTTL ringtone for an ESPHome buzzer
piezotone -format rtttl -name doorbell chime.mid

# Keep only the lead line of a MIDI arrangement
piezotone -format midi -track 1 -o lead.mid arrangement.mid
```
