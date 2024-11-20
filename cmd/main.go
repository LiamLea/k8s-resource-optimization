package main

import (
	"res-optimize/pkg/optimization"
	"time"
)

func main() {
	duration, _ := time.ParseDuration("24h")
	FindProm := optimization.FindResFromPrometheus{PromUrl: "https://prometheus-test.tailac90.ts.net/", Duration: duration}
	FindProm.FindRes()
}
