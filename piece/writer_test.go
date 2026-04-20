package piece

import (
	"bytes"
	"errors"
	"io"
	"math/rand"
	"testing"
)

func TestWriter_MatchesCalculate(t *testing.T) {
	sizes := []int{1, 127, 1024, 1 << 20, 16 << 20}
	for _, size := range sizes {
		size := size
		t.Run("", func(t *testing.T) {
			data := make([]byte, size)
			rng := rand.New(rand.NewSource(int64(size)))
			if _, err := rng.Read(data); err != nil {
				t.Fatalf("rng: %v", err)
			}

			want, err := Calculate(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("Calculate: %v", err)
			}

			w := NewWriter()
			if _, err := io.Copy(w, bytes.NewReader(data)); err != nil {
				t.Fatalf("io.Copy: %v", err)
			}
			got, err := w.Sum()
			if err != nil {
				t.Fatalf("Sum: %v", err)
			}

			if got.CIDv1 != want.CIDv1 {
				t.Errorf("CIDv1 mismatch: got %s want %s", got.CIDv1, want.CIDv1)
			}
			if got.CIDv2 != want.CIDv2 {
				t.Errorf("CIDv2 mismatch: got %s want %s", got.CIDv2, want.CIDv2)
			}
			if got.RawSize != want.RawSize {
				t.Errorf("RawSize mismatch: got %d want %d", got.RawSize, want.RawSize)
			}
		})
	}
}

func TestWriter_MultipleWrites_EqualsSingle(t *testing.T) {
	data := bytes.Repeat([]byte("abcdefgh"), 2048) // 16 KiB

	single := NewWriter()
	if _, err := single.Write(data); err != nil {
		t.Fatalf("single write: %v", err)
	}
	infoSingle, err := single.Sum()
	if err != nil {
		t.Fatalf("single Sum: %v", err)
	}

	chunked := NewWriter()
	for i := 0; i < len(data); i += 337 {
		end := i + 337
		if end > len(data) {
			end = len(data)
		}
		if _, err := chunked.Write(data[i:end]); err != nil {
			t.Fatalf("chunked write: %v", err)
		}
	}
	infoChunked, err := chunked.Sum()
	if err != nil {
		t.Fatalf("chunked Sum: %v", err)
	}

	if infoSingle != infoChunked {
		t.Errorf("chunked != single: %+v vs %+v", infoChunked, infoSingle)
	}
}

func TestWriter_EmptySumReturnsErrEmptyInput(t *testing.T) {
	w := NewWriter()
	_, err := w.Sum()
	if !errors.Is(err, ErrEmptyInput) {
		t.Errorf("expected ErrEmptyInput, got %v", err)
	}
}

func TestWriter_WriteAfterSum_Errors(t *testing.T) {
	w := NewWriter()
	if _, err := w.Write([]byte("hello world")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := w.Sum(); err != nil {
		t.Fatalf("sum: %v", err)
	}
	if _, err := w.Write([]byte("x")); err == nil {
		t.Error("expected error on Write after Sum")
	}
	if _, err := w.Sum(); err == nil {
		t.Error("expected error on Sum called twice")
	}
}

func TestWriter_Written(t *testing.T) {
	w := NewWriter()
	if w.Written() != 0 {
		t.Errorf("initial Written: got %d want 0", w.Written())
	}
	data := []byte("hello, world!")
	if _, err := w.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := w.Written(); got != int64(len(data)) {
		t.Errorf("Written: got %d want %d", got, len(data))
	}
}

func TestWriter_BelowMinPayload_V2Undef(t *testing.T) {
	w := NewWriter()
	if _, err := w.Write([]byte("short")); err != nil {
		t.Fatalf("write: %v", err)
	}
	info, err := w.Sum()
	if err != nil {
		t.Fatalf("Sum: %v", err)
	}
	if info.CIDv2.Defined() {
		t.Errorf("expected undefined CIDv2 for input below %d bytes, got %s", minPayloadSizeForV2, info.CIDv2)
	}
	if !info.CIDv1.Defined() {
		t.Error("expected defined CIDv1")
	}
}
