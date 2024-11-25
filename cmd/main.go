package main

import (
	"k8s-resource-optimization/pkg/optimization"
	"k8s-resource-optimization/pkg/utils"
	"time"
)

func main() {
	duration, _ := time.ParseDuration("168h")
	FindProm := optimization.FindResFromPrometheus{PromUrl: "https://prometheus-test.tailac90.ts.net/", Duration: duration}
	resFound := FindProm.FindRes()
	result := FindProm.RecommendRes(resFound)
	utils.DumpToJsonFile(result, "./output.json")
}
