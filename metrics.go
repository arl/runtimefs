package runtimefs

import (
	"runtime/metrics"
)

func tree() {
	descs := metrics.All()
	samples := make([]metrics.Sample, len(descs))
	for i := range samples {
		samples[i].Name = descs[i].Name
	}
	metrics.Read(samples)

	for _, s := range samples {
		println(s.Name, s.Value.Kind())
	}
}
