package piezotone

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// writeWAV encodes interleaved samples in [-1, 1] as a RIFF WAVE file.
// format is 1 (integer PCM) or 3 (float). ext writes WAVE_FORMAT_EXTENSIBLE.
func writeWAV(samples []float32, rate, channels, bits, format int, ext bool) []byte {
	var data bytes.Buffer
	for _, s := range samples {
		switch {
		case format == 3:
			binary.Write(&data, binary.LittleEndian, s)
		case bits == 8:
			data.WriteByte(byte(int(math.Round(float64(s)*127)) + 128))
		case bits == 16:
			binary.Write(&data, binary.LittleEndian, int16(math.Round(float64(s)*32767)))
		case bits == 24:
			v := int32(math.Round(float64(s) * 8388607))
			data.Write([]byte{byte(v), byte(v >> 8), byte(v >> 16)})
		case bits == 32:
			binary.Write(&data, binary.LittleEndian, int32(math.Round(float64(s)*2147483647)))
		}
	}
	align := channels * bits / 8
	fmtc := new(bytes.Buffer)
	tag := format
	if ext {
		tag = 0xFFFE
	}
	binary.Write(fmtc, binary.LittleEndian, []uint16{uint16(tag), uint16(channels)})
	binary.Write(fmtc, binary.LittleEndian, []uint32{uint32(rate), uint32(rate * align)})
	binary.Write(fmtc, binary.LittleEndian, []uint16{uint16(align), uint16(bits)})
	if ext {
		binary.Write(fmtc, binary.LittleEndian, []uint16{22, uint16(bits), 0, 0, uint16(format)})
		fmtc.Write(make([]byte, 14)) // rest of the subformat GUID
	}
	var b bytes.Buffer
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(4+8+fmtc.Len()+8+3+1+8+data.Len()))
	b.WriteString("WAVEfmt ")
	binary.Write(&b, binary.LittleEndian, uint32(fmtc.Len()))
	b.Write(fmtc.Bytes())
	b.WriteString("JUNK") // an odd-sized chunk that the reader must skip, with its pad byte
	binary.Write(&b, binary.LittleEndian, uint32(3))
	b.Write([]byte{1, 2, 3, 0})
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint32(data.Len()))
	b.Write(data.Bytes())
	return b.Bytes()
}

func TestReadWAV(t *testing.T) {
	// Stereo frames: the reader mixes each one down to (l+r)/2.
	in := []float32{0.5, 0.5, -0.5, -0.25, 1, 0, 0, -1}
	want := []float32{0.5, -0.375, 0.5, -0.5}
	tests := []struct {
		name         string
		bits, format int
		ext          bool
		tol          float64
	}{
		{"8-bit", 8, 1, false, 1.0 / 100},
		{"16-bit", 16, 1, false, 1.0 / 30000},
		{"24-bit", 24, 1, false, 1e-6},
		{"24-bit extensible", 24, 1, true, 1e-6},
		{"32-bit int", 32, 1, false, 1e-6},
		{"32-bit float", 32, 3, false, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, rate, err := ReadWAV(bytes.NewReader(writeWAV(in, 48000, 2, tc.bits, tc.format, tc.ext)))
			if err != nil || rate != 48000 || len(got) != len(want) {
				t.Fatalf("ReadWAV = %v, %d, %v", got, rate, err)
			}
			for i := range want {
				if math.Abs(float64(got[i]-want[i])) > tc.tol {
					t.Errorf("sample %d = %v, want %v", i, got[i], want[i])
				}
			}
		})
	}
}

func TestReadWAVMono(t *testing.T) {
	got, rate, err := ReadWAV(bytes.NewReader(writeWAV([]float32{0.25, -0.5}, 44100, 1, 32, 3, false)))
	if err != nil || rate != 44100 || len(got) != 2 || got[0] != 0.25 || got[1] != -0.5 {
		t.Errorf("ReadWAV = %v, %d, %v", got, rate, err)
	}
}

func TestReadWAVErrors(t *testing.T) {
	good := writeWAV([]float32{0.1, 0.2}, 44100, 1, 16, 1, false)
	adpcm := bytes.Clone(good)
	adpcm[20] = 2 // format tag 2, ADPCM
	for name, b := range map[string][]byte{
		"not RIFF":        []byte("RIFX0000WAVE"),
		"short":           good[:10],
		"no data chunk":   good[:36],
		"ADPCM":           adpcm,
		"data before fmt": append([]byte("RIFF\x00\x00\x00\x00WAVEdata\x02\x00\x00\x00\x00\x00"), good[12:]...),
	} {
		if _, _, err := ReadWAV(bytes.NewReader(b)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
