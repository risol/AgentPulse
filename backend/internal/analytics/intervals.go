package analytics

import "sort"

const IntervalRuleVersion = "intervals.v1"

type Quality string

const (
	QualityComplete    Quality = "complete"
	QualityPartial     Quality = "partial"
	QualityUnavailable Quality = "unavailable"
)

type Interval struct {
	StartNS int64
	EndNS   int64
}

type IntervalInput struct {
	WallClock        *Interval
	WallClockQuality Quality
	LLM              []Interval
	Commands         []Interval
	LLMObserved      bool
	CommandsObserved bool
}

type DurationMetric struct {
	ValueNS            *int64  `json:"value_ns"`
	Quality            Quality `json:"quality"`
	RuleVersion        string  `json:"rule_version"`
	TimestampAnomalies bool    `json:"timestamp_anomalies"`
}

type IntervalAnalysis struct {
	WallClock    DurationMetric `json:"wall_clock"`
	LLMOnly      DurationMetric `json:"llm_only"`
	CommandOnly  DurationMetric `json:"command_only"`
	Overlap      DurationMetric `json:"overlap"`
	Unattributed DurationMetric `json:"unattributed"`
}

func AnalyzeIntervals(input IntervalInput) IntervalAnalysis {
	if input.WallClock == nil || !valid(input.WallClock.StartNS, input.WallClock.EndNS) {
		anomaly := input.WallClock != nil
		unavailable := unavailableDuration(anomaly)
		return IntervalAnalysis{
			WallClock: unavailable, LLMOnly: unavailable, CommandOnly: unavailable,
			Overlap: unavailable, Unattributed: unavailable,
		}
	}

	wall := *input.WallClock
	wallDuration := wall.EndNS - wall.StartNS
	wallQuality := input.WallClockQuality
	if wallQuality == "" {
		wallQuality = QualityComplete
	}
	result := IntervalAnalysis{
		WallClock: availableDuration(wallDuration, wallQuality, false),
		LLMOnly:   unavailableDuration(false), CommandOnly: unavailableDuration(false),
		Overlap: unavailableDuration(false), Unattributed: unavailableDuration(false),
	}
	if !input.LLMObserved || !input.CommandsObserved {
		return result
	}

	llm, llmAnomaly := clipAndMerge(input.LLM, wall)
	commands, commandAnomaly := clipAndMerge(input.Commands, wall)
	anomaly := llmAnomaly || commandAnomaly
	llmTotal := unionDuration(llm)
	commandTotal := unionDuration(commands)
	overlap := intersectionDuration(llm, commands)
	all := merge(append(append([]Interval(nil), llm...), commands...))
	unattributed := wallDuration - unionDuration(all)
	quality := wallQuality
	if anomaly || wallQuality == QualityPartial {
		quality = QualityPartial
	}
	result.LLMOnly = availableDuration(llmTotal-overlap, quality, anomaly)
	result.CommandOnly = availableDuration(commandTotal-overlap, quality, anomaly)
	result.Overlap = availableDuration(overlap, quality, anomaly)
	result.Unattributed = availableDuration(unattributed, quality, anomaly)
	return result
}

func clipAndMerge(intervals []Interval, wall Interval) ([]Interval, bool) {
	clipped := make([]Interval, 0, len(intervals))
	anomaly := false
	for _, interval := range intervals {
		if !valid(interval.StartNS, interval.EndNS) {
			anomaly = true
			continue
		}
		if interval.EndNS <= wall.StartNS || interval.StartNS >= wall.EndNS {
			anomaly = true
			continue
		}
		if interval.StartNS < wall.StartNS || interval.EndNS > wall.EndNS {
			anomaly = true
		}
		if interval.StartNS < wall.StartNS {
			interval.StartNS = wall.StartNS
		}
		if interval.EndNS > wall.EndNS {
			interval.EndNS = wall.EndNS
		}
		clipped = append(clipped, interval)
	}
	return merge(clipped), anomaly
}

func merge(intervals []Interval) []Interval {
	if len(intervals) == 0 {
		return nil
	}
	sorted := append([]Interval(nil), intervals...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].StartNS == sorted[j].StartNS {
			return sorted[i].EndNS < sorted[j].EndNS
		}
		return sorted[i].StartNS < sorted[j].StartNS
	})
	merged := make([]Interval, 0, len(sorted))
	for _, interval := range sorted {
		lastIndex := len(merged) - 1
		if lastIndex < 0 || interval.StartNS > merged[lastIndex].EndNS {
			merged = append(merged, interval)
			continue
		}
		if interval.EndNS > merged[lastIndex].EndNS {
			merged[lastIndex].EndNS = interval.EndNS
		}
	}
	return merged
}

func unionDuration(intervals []Interval) int64 {
	var total int64
	for _, interval := range intervals {
		total += interval.EndNS - interval.StartNS
	}
	return total
}

func intersectionDuration(left, right []Interval) int64 {
	var total int64
	for i, j := 0, 0; i < len(left) && j < len(right); {
		start := max(left[i].StartNS, right[j].StartNS)
		end := min(left[i].EndNS, right[j].EndNS)
		if end > start {
			total += end - start
		}
		if left[i].EndNS < right[j].EndNS {
			i++
		} else {
			j++
		}
	}
	return total
}

func valid(start, end int64) bool {
	return start >= 0 && end > start
}

func availableDuration(value int64, quality Quality, anomaly bool) DurationMetric {
	return DurationMetric{ValueNS: &value, Quality: quality, RuleVersion: IntervalRuleVersion, TimestampAnomalies: anomaly}
}

func unavailableDuration(anomaly bool) DurationMetric {
	return DurationMetric{Quality: QualityUnavailable, RuleVersion: IntervalRuleVersion, TimestampAnomalies: anomaly}
}
