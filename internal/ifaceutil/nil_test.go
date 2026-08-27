package ifaceutil

import "testing"

type testImplementation struct{}

func TestNormalizeNil(t *testing.T) {
	t.Run("nil interface", func(t *testing.T) {
		var value any
		if got := NormalizeNil(value); got != nil {
			t.Fatalf("NormalizeNil(nil) = %#v, want nil", got)
		}
	})

	t.Run("typed nil pointer", func(t *testing.T) {
		var pointer *testImplementation
		var value any = pointer
		if got := NormalizeNil(value); got != nil {
			t.Fatalf("NormalizeNil(typed nil) = %#v, want nil", got)
		}
	})

	t.Run("non-nil pointer", func(t *testing.T) {
		pointer := &testImplementation{}
		var value any = pointer
		if got := NormalizeNil(value); got != pointer {
			t.Fatalf("NormalizeNil(non-nil pointer) = %#v, want same pointer", got)
		}
	})
}
