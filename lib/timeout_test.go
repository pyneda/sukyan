package lib

import (
	"testing"
	"time"
)

func TestDoWorkWithTimeout(t *testing.T) {
	tests := []struct {
		name    string
		fn      interface{}
		params  []interface{}
		timeout time.Duration
		want    interface{}
		wantErr error
	}{
		{
			name: "Function completes before timeout",
			fn: func(a, b int) (int, error) {
				return a + b, nil
			},
			params:  []interface{}{1, 2},
			timeout: 1 * time.Second,
			want:    3,
			wantErr: nil,
		},
		{
			name: "Function completes after timeout",
			fn: func(a, b int) (int, error) {
				time.Sleep(2 * time.Second)
				return a + b, nil
			},
			params:  []interface{}{1, 2},
			timeout: 1 * time.Second,
			want:    nil,
			wantErr: TimeoutError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DoWorkWithTimeout(tt.fn, tt.params, tt.timeout)
			if got != tt.want {
				t.Errorf("DoWorkWithTimeout() got = %v, want %v", got, tt.want)
			}
			if err != tt.wantErr {
				t.Errorf("DoWorkWithTimeout() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
