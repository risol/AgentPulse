//go:build !windows

package process

import "context"

type unsupportedCollector struct{}

func NewCollector() Collector { return unsupportedCollector{} }

func (unsupportedCollector) Snapshot(context.Context, Config, []Sample) (Snapshot, error) {
	return Snapshot{}, ErrUnsupported
}
