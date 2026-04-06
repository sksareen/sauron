package embed

import (
	"math"
	"testing"
)

func TestVectorRoundTrip(t *testing.T) {
	original := []float32{0.1, -0.5, 0.999, 0.0, -1.0, 3.14}
	bytes := VectorToBytes(original)
	recovered := BytesToVector(bytes)

	if len(recovered) != len(original) {
		t.Fatalf("length mismatch: %d vs %d", len(recovered), len(original))
	}
	for i := range original {
		if original[i] != recovered[i] {
			t.Errorf("mismatch at %d: %f vs %f", i, original[i], recovered[i])
		}
	}
}

func TestVectorToBytesLength(t *testing.T) {
	v := make([]float32, 1536) // text-embedding-3-small dimension
	b := VectorToBytes(v)
	if len(b) != 1536*4 {
		t.Errorf("expected %d bytes, got %d", 1536*4, len(b))
	}
}

func TestBytesToVectorEmpty(t *testing.T) {
	if v := BytesToVector(nil); v != nil {
		t.Error("expected nil for nil input")
	}
	if v := BytesToVector([]byte{}); v != nil {
		t.Error("expected nil for empty input")
	}
	if v := BytesToVector([]byte{1, 2, 3}); v != nil {
		t.Error("expected nil for non-aligned input")
	}
}

func TestCosineSimilarityIdentical(t *testing.T) {
	a := []float32{1.0, 2.0, 3.0}
	sim := CosineSimilarity(a, a)
	if math.Abs(sim-1.0) > 1e-6 {
		t.Errorf("identical vectors should have similarity 1.0, got %f", sim)
	}
}

func TestCosineSimilarityOrthogonal(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{0.0, 1.0}
	sim := CosineSimilarity(a, b)
	if math.Abs(sim) > 1e-6 {
		t.Errorf("orthogonal vectors should have similarity 0, got %f", sim)
	}
}

func TestCosineSimilarityOpposite(t *testing.T) {
	a := []float32{1.0, 2.0, 3.0}
	b := []float32{-1.0, -2.0, -3.0}
	sim := CosineSimilarity(a, b)
	if math.Abs(sim-(-1.0)) > 1e-6 {
		t.Errorf("opposite vectors should have similarity -1, got %f", sim)
	}
}

func TestCosineSimilarityDifferentLengths(t *testing.T) {
	a := []float32{1.0, 2.0}
	b := []float32{1.0, 2.0, 3.0}
	sim := CosineSimilarity(a, b)
	if sim != 0 {
		t.Errorf("different length vectors should return 0, got %f", sim)
	}
}

func TestCosineSimilarityZeroVector(t *testing.T) {
	a := []float32{0, 0, 0}
	b := []float32{1, 2, 3}
	sim := CosineSimilarity(a, b)
	if sim != 0 {
		t.Errorf("zero vector should return 0, got %f", sim)
	}
}
