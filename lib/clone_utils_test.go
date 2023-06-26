package lib

import (
	"reflect"
	"testing"
)

type TestStruct struct {
	A int
	B string
	C []float64
	D map[string]int
	E *int
}

func TestDeepCopy(t *testing.T) {
	testCases := []struct {
		name string
		src  TestStruct
		dest TestStruct
	}{
		{
			name: "basic test",
			src: TestStruct{
				A: 1,
				B: "test",
				C: []float64{1.1, 2.2, 3.3},
				D: map[string]int{"one": 1, "two": 2},
				E: func() *int { i := 1; return &i }(),
			},
			dest: TestStruct{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := DeepCopy(tc.src, &tc.dest); err != nil {
				t.Errorf("unexpected error during deep copy: %v", err)
				return
			}

			// Use reflect.DeepEqual to compare src and dest
			if !reflect.DeepEqual(tc.src, tc.dest) {
				t.Errorf("DeepCopy = %v, want %v", tc.dest, tc.src)
			}
		})
	}
}
