package main

import (
	"res-optimize/pkg/optimization"
	"res-optimize/pkg/utils"
	"time"
)

func main() {
	duration, _ := time.ParseDuration("24h")
	FindProm := optimization.FindResFromPrometheus{PromUrl: "https://prometheus-test.tailac90.ts.net/", Duration: duration}
	resFound := FindProm.FindRes()
	result := FindProm.RecommendRes(resFound)
	utils.DumpToJsonFile(result, "output.json")
}
