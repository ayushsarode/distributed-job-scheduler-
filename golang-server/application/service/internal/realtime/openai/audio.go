package openai

import "github.com/zeozeozeo/gomplerate"

const (
	bytesPerSample = 2
	bitsPerByte    = 8
)

func resampleAudio(audio []byte, fromRate, toRate int) []byte {
	if fromRate == toRate || len(audio) == 0 {
		return audio
	}

	r, err := gomplerate.NewResampler(1, fromRate, toRate)
	if err != nil {
		return audio
	}

	samples := make([]int16, len(audio)/bytesPerSample)
	for i := range samples {
		lo := int16(audio[i*bytesPerSample])
		hi := int16(audio[i*bytesPerSample+1])
		samples[i] = lo | (hi << bitsPerByte)
	}

	resampled := r.ResampleInt16(samples)

	out := make([]byte, len(resampled)*bytesPerSample)
	for i, s := range resampled {
		out[i*bytesPerSample] = byte(s)
		out[i*bytesPerSample+1] = byte(s >> bitsPerByte)
	}

	return out
}