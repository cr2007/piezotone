# Third-party notices

The `piezotone` binary links these Go modules. Each keeps its own license. The MIT, BSD and
Apache-2.0 licenses need their copyright notice in copies, so a binary release must ship the
license texts below (for example, from `go mod download -json` or the module cache).

| Module | Version | License | Copyright |
|--------|---------|---------|-----------|
| [gitlab.com/gomidi/midi/v2](https://gitlab.com/gomidi/midi) | v2.3.24 | MIT | (c) 2017 Marc Rene Arns |
| [github.com/tphakala/go-mp3](https://github.com/tphakala/go-mp3) | v0.4.0 | MIT | (c) 2026 Tomi P. Hakala |
| [github.com/tphakala/simd](https://github.com/tphakala/simd) | v1.16.0 | MIT | (c) 2025 Tomi P. Hakala |
| [github.com/jfreymuth/oggvorbis](https://github.com/jfreymuth/oggvorbis) | v1.0.6 | MIT | (c) 2016 Johann Freymuth |
| [github.com/mewkiz/flac](https://github.com/mewkiz/flac) | v1.0.14 | Unlicense (public domain) | none |
| [github.com/mewkiz/pkg](https://github.com/mewkiz/pkg) | v0.0.0-20250417130911 | Unlicense (public domain) | none |
| [github.com/mewpkg/term](https://github.com/mewpkg/term) | v0.0.0-20241026122259 | Unlicense (public domain) | none |
| [github.com/icza/bitio](https://github.com/icza/bitio) | v1.1.0 | Apache-2.0 | see its LICENSE; it has no NOTICE file |
| [golang.org/x/sys](https://cs.opensource.google/go/x/sys) | v0.50.0 | BSD-3-Clause | (c) 2009 The Go Authors |

The code of this repository came from the `tones` module of go-opendisplay, by the same author.
The audio pitch tracker follows the harmonic summation of Klapuri (ISMIR 2006) and the salience
function of Salamon and Gomez (IEEE TASLP 20(6), 2012). It is new code, not a port of an existing
implementation.
