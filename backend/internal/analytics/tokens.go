package analytics

const TokenRuleVersion = "tokens.v1"

type TokenInput struct {
	OutputTokens         *int64
	RequestDurationNS    *int64
	GenerationDurationNS *int64
	TTFTMS               *float64
	RateQuality          Quality
	TTFTQuality          Quality
}

type RateMetric struct {
	TokensPerSecond *float64 `json:"tokens_per_second"`
	Quality         Quality  `json:"quality"`
	RuleVersion     string   `json:"rule_version"`
}

type ScalarMetric struct {
	Value       *float64 `json:"value"`
	Quality     Quality  `json:"quality"`
	RuleVersion string   `json:"rule_version"`
}

type TokenAnalysis struct {
	RequestRate    RateMetric   `json:"request_rate"`
	GenerationRate RateMetric   `json:"generation_rate"`
	TTFT           ScalarMetric `json:"ttft_ms"`
}

func AnalyzeTokens(input TokenInput) TokenAnalysis {
	result := TokenAnalysis{
		RequestRate:    RateMetric{Quality: QualityUnavailable, RuleVersion: TokenRuleVersion},
		GenerationRate: RateMetric{Quality: QualityUnavailable, RuleVersion: TokenRuleVersion},
		TTFT:           ScalarMetric{Quality: QualityUnavailable, RuleVersion: TokenRuleVersion},
	}
	if input.OutputTokens != nil && *input.OutputTokens >= 0 && input.RequestDurationNS != nil && *input.RequestDurationNS > 0 {
		value := float64(*input.OutputTokens) / (float64(*input.RequestDurationNS) / 1e9)
		result.RequestRate = RateMetric{TokensPerSecond: &value, Quality: requestedQuality(input.RateQuality), RuleVersion: TokenRuleVersion}
	}
	if input.OutputTokens != nil && *input.OutputTokens >= 0 && input.GenerationDurationNS != nil && *input.GenerationDurationNS > 0 {
		value := float64(*input.OutputTokens) / (float64(*input.GenerationDurationNS) / 1e9)
		result.GenerationRate = RateMetric{TokensPerSecond: &value, Quality: requestedQuality(input.RateQuality), RuleVersion: TokenRuleVersion}
	}
	if input.TTFTMS != nil && *input.TTFTMS >= 0 {
		value := *input.TTFTMS
		result.TTFT = ScalarMetric{Value: &value, Quality: requestedQuality(input.TTFTQuality), RuleVersion: TokenRuleVersion}
	}
	return result
}

func requestedQuality(quality Quality) Quality {
	if quality == "" {
		return QualityComplete
	}
	return quality
}
