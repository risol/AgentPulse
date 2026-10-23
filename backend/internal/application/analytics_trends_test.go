package application

import (
	"testing"
	"time"
)

func TestSummaryRangeDuration(t *testing.T) {
	tests := []struct {
		name     string
		want     time.Duration
		wantOkay bool
	}{
		{name: "15m", want: 15 * time.Minute, wantOkay: true},
		{name: "1h", want: time.Hour, wantOkay: true},
		{name: "6h", want: 6 * time.Hour, wantOkay: true},
		{name: "24h", want: 24 * time.Hour, wantOkay: true},
		{name: "7d", want: 7 * 24 * time.Hour, wantOkay: true},
		{name: "unsupported", want: 0, wantOkay: false},
		{name: "", want: 0, wantOkay: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, okay := SummaryRangeDuration(test.name)
			if got != test.want || okay != test.wantOkay {
				t.Fatalf("SummaryRangeDuration(%q) = (%s, %t), want (%s, %t)", test.name, got, okay, test.want, test.wantOkay)
			}
		})
	}
}
