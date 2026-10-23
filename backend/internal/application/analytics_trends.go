package application

import (
	"context"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/telemetry"
)

type AnalyticsTrendRepository interface {
	LoadAnalyticsTrends(context.Context, int64, int64, int64) ([]telemetry.AnalyticsTrendBucket, error)
}

type AnalyticsTrendView struct {
	Range                 string                           `json:"range"`
	BucketIntervalSeconds int64                            `json:"bucket_interval_seconds"`
	Points                []telemetry.AnalyticsTrendBucket `json:"points"`
}

type AnalyticsTrendService struct {
	repository AnalyticsTrendRepository
	now        func() time.Time
}

func NewAnalyticsTrendService(repository AnalyticsTrendRepository) *AnalyticsTrendService {
	return &AnalyticsTrendService{repository: repository, now: time.Now}
}

func (s *AnalyticsTrendService) Query(ctx context.Context, rangeName string) (AnalyticsTrendView, error) {
	duration, okay := SummaryRangeDuration(rangeName)
	if !okay {
		return AnalyticsTrendView{}, ErrInvalidSessionQueryRange
	}
	interval := trendBucketInterval(rangeName)
	until := s.now().UnixNano()
	points, err := s.repository.LoadAnalyticsTrends(ctx, until-duration.Nanoseconds(), until, interval.Nanoseconds())
	if err != nil {
		return AnalyticsTrendView{}, err
	}
	return AnalyticsTrendView{Range: rangeName, BucketIntervalSeconds: int64(interval.Seconds()), Points: points}, nil
}

func trendBucketInterval(rangeName string) time.Duration {
	switch rangeName {
	case "15m":
		return 15 * time.Second
	case "1h":
		return time.Minute
	case "6h":
		return 5 * time.Minute
	case "24h":
		return 15 * time.Minute
	case "7d":
		return time.Hour
	default:
		return 0
	}
}
