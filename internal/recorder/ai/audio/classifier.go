package audio

import (
	"fmt"
	"math"
	"sync"
)

// Classifier runs audio classification inference on PCM audio frames.
// It wraps one or more ONNX model sessions (one per event type) and
// returns classification scores for each enabled detector.
//
// The classifier operates on mel-spectrogram features extracted from raw
// PCM audio. Each model expects a fixed-size input window (typically 1s
// of audio at 16 kHz = 16000 samples).
type Classifier struct {
	mu       sync.RWMutex
	models   map[EventType]*AudioModel
	modelDir string
}

// AudioModel represents a loaded ONNX model for a specific audio event type.
// In the real deployment this wraps an ort.AdvancedSession; for portability
// and testability we use a pluggable inference function.
type AudioModel struct {
	EventType EventType
	ModelPath string

	// InferFunc is the inference function. It accepts mel-spectrogram features
	// and returns a confidence score [0, 1]. When nil, the model is treated
	// as unloaded and returns 0.
	InferFunc func(features []float32) (float32, error)
}

// NewClassifier creates a classifier that loads models from modelDir.
func NewClassifier(modelDir string) *Classifier {
	return &Classifier{
		models:   make(map[EventType]*AudioModel),
		modelDir: modelDir,
	}
}

// LoadModel registers a model for the given event type.
func (c *Classifier) LoadModel(model *AudioModel) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.models[model.EventType] = model
}

// HasModel returns true if a model is loaded for the given event type.
func (c *Classifier) HasModel(evt EventType) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.models[evt]
	return ok
}

// Classify runs inference on the provided mel-spectrogram features for
// the specified event type. Returns the confidence score [0, 1].
func (c *Classifier) Classify(evt EventType, features []float32) (float32, error) {
	c.mu.RLock()
	model, ok := c.models[evt]
	c.mu.RUnlock()

	if !ok {
		return 0, fmt.Errorf("no model loaded for event type %s", evt)
	}
	if model.InferFunc == nil {
		return 0, fmt.Errorf("model for %s has no inference function", evt)
	}

	score, err := model.InferFunc(features)
	if err != nil {
		return 0, fmt.Errorf("inference error for %s: %w", evt, err)
	}
	return score, nil
}

// ClassifyAll runs inference across all loaded models and returns scores
// for each event type. Models that fail are skipped (score = 0).
func (c *Classifier) ClassifyAll(features []float32) map[EventType]float32 {
	c.mu.RLock()
	models := make(map[EventType]*AudioModel, len(c.models))
	for k, v := range c.models {
		models[k] = v
	}
	c.mu.RUnlock()

	results := make(map[EventType]float32, len(models))
	for evt, model := range models {
		if model.InferFunc == nil {
			continue
		}
		score, err := model.InferFunc(features)
		if err != nil {
			continue
		}
		results[evt] = score
	}
	return results
}

// Close releases all model resources.
func (c *Classifier) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Clear references; actual ONNX session cleanup is handled by the
	// InferFunc closures if needed.
	c.models = make(map[EventType]*AudioModel)
}

// ExtractMelSpectrogram converts raw PCM float32 samples into a mel-spectrogram
// feature vector suitable for the audio classifier models.
//
// Parameters:
//   - samples: mono PCM float32 in [-1, 1]
//   - sampleRate: sample rate in Hz (typically 16000)
//   - nMels: number of mel frequency bins (typically 64)
//   - fftSize: FFT window size (typically 512)
//   - hopSize: hop size between frames (typically 160)
//
// Returns a flattened mel-spectrogram of shape [nMels x numFrames].
func ExtractMelSpectrogram(samples []float32, sampleRate, nMels, fftSize, hopSize int) []float32 {
	if len(samples) == 0 {
		return nil
	}

	numFrames := (len(samples) - fftSize) / hopSize
	if numFrames <= 0 {
		numFrames = 1
	}

	// Generate mel filterbank.
	filterbank := melFilterbank(sampleRate, fftSize, nMels)

	result := make([]float32, nMels*numFrames)

	for frame := 0; frame < numFrames; frame++ {
		start := frame * hopSize
		end := start + fftSize
		if end > len(samples) {
			end = len(samples)
		}

		// Apply Hann window and compute power spectrum.
		window := make([]float64, fftSize)
		for i := 0; i < fftSize && start+i < end; i++ {
			hannCoeff := 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(fftSize-1)))
			window[i] = float64(samples[start+i]) * hannCoeff
		}

		// Simple DFT for the positive frequencies (fftSize/2 + 1 bins).
		nBins := fftSize/2 + 1
		powerSpectrum := make([]float64, nBins)
		for k := 0; k < nBins; k++ {
			var real, imag float64
			for n := 0; n < fftSize; n++ {
				angle := -2 * math.Pi * float64(k) * float64(n) / float64(fftSize)
				real += window[n] * math.Cos(angle)
				imag += window[n] * math.Sin(angle)
			}
			powerSpectrum[k] = (real*real + imag*imag) / float64(fftSize)
		}

		// Apply mel filterbank.
		for mel := 0; mel < nMels; mel++ {
			var energy float64
			for k := 0; k < nBins && k < len(filterbank[mel]); k++ {
				energy += filterbank[mel][k] * powerSpectrum[k]
			}
			// Log-mel energy (add small epsilon to avoid log(0)).
			logEnergy := math.Log(energy + 1e-10)
			result[mel*numFrames+frame] = float32(logEnergy)
		}
	}

	return result
}

// melFilterbank creates a mel-scale filterbank matrix.
func melFilterbank(sampleRate, fftSize, nMels int) [][]float64 {
	nBins := fftSize/2 + 1
	lowFreq := 0.0
	highFreq := float64(sampleRate) / 2.0

	lowMel := hzToMel(lowFreq)
	highMel := hzToMel(highFreq)

	// Equally spaced mel points.
	melPoints := make([]float64, nMels+2)
	step := (highMel - lowMel) / float64(nMels+1)
	for i := range melPoints {
		melPoints[i] = lowMel + float64(i)*step
	}

	// Convert mel points to FFT bin indices.
	binPoints := make([]int, nMels+2)
	for i, m := range melPoints {
		freq := melToHz(m)
		binPoints[i] = int(math.Floor(float64(fftSize+1) * freq / float64(sampleRate)))
	}

	filterbank := make([][]float64, nMels)
	for mel := 0; mel < nMels; mel++ {
		filterbank[mel] = make([]float64, nBins)
		left := binPoints[mel]
		center := binPoints[mel+1]
		right := binPoints[mel+2]

		for k := left; k < center && k < nBins; k++ {
			if center > left {
				filterbank[mel][k] = float64(k-left) / float64(center-left)
			}
		}
		for k := center; k < right && k < nBins; k++ {
			if right > center {
				filterbank[mel][k] = float64(right-k) / float64(right-center)
			}
		}
	}

	return filterbank
}

func hzToMel(hz float64) float64 {
	return 2595.0 * math.Log10(1.0+hz/700.0)
}

func melToHz(mel float64) float64 {
	return 700.0 * (math.Pow(10.0, mel/2595.0) - 1.0)
}
