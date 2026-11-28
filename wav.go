package piezotone

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// ReadWAV reads a RIFF WAVE file with integer PCM (8, 16, 24 or 32 bit) or
// float samples (32 or 64 bit), in any channel count. It mixes the channels
// down to mono and returns the samples and the sample rate. Integer PCM is in
// [-1, 1]; float samples are as in the file.
func ReadWAV(r io.Reader) ([]float32, int, error) {
	var hdr [12]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil || string(hdr[:4]) != "RIFF" || string(hdr[8:]) != "WAVE" {
		return nil, 0, errors.New("piezotone: not a RIFF WAVE file")
	}
	var format, channels, bits, rate int
	for {
		var ch [8]byte
		if _, err := io.ReadFull(r, ch[:]); err != nil {
			return nil, 0, errors.New("piezotone: WAV file has no data chunk")
		}
		size := int64(binary.LittleEndian.Uint32(ch[4:]))
		switch string(ch[:4]) {
		case "fmt ":
			if size < 16 || size > 1024 {
				return nil, 0, fmt.Errorf("piezotone: WAV fmt chunk has a bad size %d", size)
			}
			b := make([]byte, size+size%2)
			if _, err := io.ReadFull(r, b); err != nil {
				return nil, 0, fmt.Errorf("piezotone: WAV fmt chunk: %w", err)
			}
			format = int(binary.LittleEndian.Uint16(b))
			channels = int(binary.LittleEndian.Uint16(b[2:]))
			rate = int(binary.LittleEndian.Uint32(b[4:]))
			bits = int(binary.LittleEndian.Uint16(b[14:]))
			if format == 0xFFFE && size >= 26 { // WAVE_FORMAT_EXTENSIBLE: the subformat GUID starts with the tag
				format = int(binary.LittleEndian.Uint16(b[24:]))
			}
		case "data":
			if channels == 0 || rate == 0 {
				return nil, 0, errors.New("piezotone: WAV data chunk before a valid fmt chunk")
			}
			// A short read is fine: some writers leave the size wrong.
			data, err := io.ReadAll(io.LimitReader(r, size))
			if err != nil {
				return nil, 0, err
			}
			s, err := pcmSamples(data, format, bits)
			if err != nil {
				return nil, 0, err
			}
			return mono(s, channels), rate, nil
		default:
			if _, err := io.CopyN(io.Discard, r, size+size%2); err != nil {
				return nil, 0, errors.New("piezotone: WAV file has no data chunk")
			}
		}
	}
}

func pcmSamples(b []byte, format, bits int) ([]float32, error) {
	w := bits / 8
	if (format != 1 && format != 3) || bits%8 != 0 || w < 1 {
		return nil, fmt.Errorf("piezotone: WAV format %d with %d bits is not supported; use integer PCM or float", format, bits)
	}
	s := make([]float32, len(b)/w)
	for i := range s {
		p := b[i*w:]
		switch {
		case format == 1 && w == 1:
			s[i] = float32(int(p[0])-128) / 128
		case format == 1 && w == 2:
			s[i] = float32(int16(binary.LittleEndian.Uint16(p))) / (1 << 15)
		case format == 1 && w == 3:
			s[i] = float32(int32(uint32(p[0])<<8|uint32(p[1])<<16|uint32(p[2])<<24)>>8) / (1 << 23)
		case format == 1 && w == 4:
			s[i] = float32(float64(int32(binary.LittleEndian.Uint32(p))) / (1 << 31))
		case format == 3 && w == 4:
			s[i] = math.Float32frombits(binary.LittleEndian.Uint32(p))
		case format == 3 && w == 8:
			s[i] = float32(math.Float64frombits(binary.LittleEndian.Uint64(p)))
		default:
			return nil, fmt.Errorf("piezotone: WAV format %d with %d bits is not supported", format, bits)
		}
	}
	return s, nil
}

// mono mixes interleaved samples down to one channel.
func mono(s []float32, channels int) []float32 {
	if channels == 1 {
		return s
	}
	out := make([]float32, len(s)/channels)
	for i := range out {
		var sum float32
		for _, v := range s[i*channels : (i+1)*channels] {
			sum += v
		}
		out[i] = sum / float32(channels)
	}
	return out
}
