package eventbus

import (
	"context"
	"testing"
	"time"
)

func BenchmarkEventBus_ScaleWorkers_NoChange(b *testing.B) {
	for _, pool := range []struct {
		name string
		max  int
	}{
		{name: "Fixed", max: 2},
		{name: "Dynamic", max: 8},
	} {
		for _, parallel := range []bool{false, true} {
			mode := "Serial"
			if parallel {
				mode = "Parallel"
			}
			b.Run(pool.name+"/"+mode, func(b *testing.B) {
				eb := New(&Config{MinWorkers: 2, MaxWorkers: pool.max})
				b.Cleanup(func() {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					if err := eb.Shutdown(ctx); err != nil {
						b.Fatal(err)
					}
				})
				b.ReportAllocs()
				b.ResetTimer()
				if parallel {
					b.RunParallel(func(pb *testing.PB) {
						for pb.Next() {
							eb.scaleWorkers()
						}
					})
				} else {
					for i := 0; i < b.N; i++ {
						eb.scaleWorkers()
					}
				}
				b.StopTimer()
			})
		}
	}
}
