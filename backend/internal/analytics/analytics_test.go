package analytics

import "testing"

func TestAnalyzeIntervalsUnionsParallelWorkAndUnattributedTime(t *testing.T) {
	wall := Interval{StartNS: 0, EndNS: 100}
	result := AnalyzeIntervals(IntervalInput{
		WallClock:   &wall,
		LLM:         []Interval{{StartNS: 0, EndNS: 50}, {StartNS: 40, EndNS: 70}},
		Commands:    []Interval{{StartNS: 60, EndNS: 90}},
		LLMObserved: true, CommandsObserved: true,
	})
	assertDuration(t, "wall", result.WallClock, 100)
	assertDuration(t, "LLM only", result.LLMOnly, 60)
	assertDuration(t, "command only", result.CommandOnly, 20)
	assertDuration(t, "overlap", result.Overlap, 10)
	assertDuration(t, "unattributed", result.Unattributed, 10)
}

func TestAnalyzeIntervalsMarksBadTimestampsAndMissingSources(t *testing.T) {
	wall := Interval{StartNS: 10, EndNS: 30}
	result := AnalyzeIntervals(IntervalInput{
		WallClock:   &wall,
		LLM:         []Interval{{StartNS: 5, EndNS: 15}, {StartNS: 20, EndNS: 20}},
		Commands:    []Interval{{StartNS: 15, EndNS: 25}},
		LLMObserved: true, CommandsObserved: true,
	})
	assertDuration(t, "clipped LLM", result.LLMOnly, 5)
	if result.LLMOnly.Quality != QualityPartial || !result.LLMOnly.TimestampAnomalies {
		t.Fatalf("clipped metric quality = %q with anomaly=%t, want partial with anomaly", result.LLMOnly.Quality, result.LLMOnly.TimestampAnomalies)
	}

	missing := AnalyzeIntervals(IntervalInput{WallClock: &wall, LLMObserved: true})
	if missing.LLMOnly.ValueNS != nil || missing.Unattributed.ValueNS != nil || missing.LLMOnly.Quality != QualityUnavailable {
		t.Fatalf("missing category was treated as a measured zero: %+v", missing)
	}
}

func TestAnalyzeIntervalsHandlesInvalidWallClock(t *testing.T) {
	wall := Interval{StartNS: 12, EndNS: 12}
	result := AnalyzeIntervals(IntervalInput{WallClock: &wall, LLMObserved: true, CommandsObserved: true})
	if result.WallClock.ValueNS != nil || result.LLMOnly.ValueNS != nil || result.WallClock.Quality != QualityUnavailable {
		t.Fatalf("invalid wall clock should make derived durations unavailable: %+v", result)
	}
}

func TestAnalyzeTokensSeparatesRequestAndGenerationRates(t *testing.T) {
	outputTokens := int64(100)
	requestDuration := int64(10_000_000_000)
	generationDuration := int64(5_000_000_000)
	ttft := 250.0
	result := AnalyzeTokens(TokenInput{
		OutputTokens: &outputTokens, RequestDurationNS: &requestDuration,
		GenerationDurationNS: &generationDuration, TTFTMS: &ttft,
	})
	if result.RequestRate.TokensPerSecond == nil || *result.RequestRate.TokensPerSecond != 10 {
		t.Fatalf("request rate = %+v, want 10 tokens/s", result.RequestRate)
	}
	if result.GenerationRate.TokensPerSecond == nil || *result.GenerationRate.TokensPerSecond != 20 {
		t.Fatalf("generation rate = %+v, want 20 tokens/s", result.GenerationRate)
	}
	if result.TTFT.Value == nil || *result.TTFT.Value != 250 {
		t.Fatalf("TTFT = %+v, want 250 ms", result.TTFT)
	}
}

func TestAnalyzeTokensKeepsUnavailableValuesNil(t *testing.T) {
	outputTokens := int64(5)
	zeroDuration := int64(0)
	result := AnalyzeTokens(TokenInput{OutputTokens: &outputTokens, RequestDurationNS: &zeroDuration})
	if result.RequestRate.TokensPerSecond != nil || result.GenerationRate.TokensPerSecond != nil || result.TTFT.Value != nil {
		t.Fatalf("missing or invalid inputs should remain unavailable: %+v", result)
	}
}

func assertDuration(t *testing.T, name string, metric DurationMetric, want int64) {
	t.Helper()
	if metric.ValueNS == nil || *metric.ValueNS != want || metric.RuleVersion != IntervalRuleVersion {
		t.Fatalf("%s duration = %+v, want %d ns with rule %q", name, metric, want, IntervalRuleVersion)
	}
}
