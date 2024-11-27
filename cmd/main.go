package main

import (
	"k8s-resource-optimization/pkg/optimization"
	"time"
)

func main() {
	duration, _ := time.ParseDuration("168h")
	FindProm := optimization.FindResFromPrometheus{PromUrl: "https://prometheus-test.tailac90.ts.net/", Duration: duration}
	resFound := FindProm.FindRes()
	FindProm.RecommendRes(resFound)
}
