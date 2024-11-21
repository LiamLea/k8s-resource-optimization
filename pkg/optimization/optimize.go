package optimization

import (
	"context"
	"fmt"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"math"
	"os"
	"sort"
	"time"
)

type IFind interface {
	FindRes() map[string][]Resource
}

type IRecommend interface {
	RecommendRes(resFound map[string][]Resource) map[string][]OptimizedRes
}

type FindResFromPrometheus struct {
	PromUrl  string
	Duration time.Duration
}

func (this *FindResFromPrometheus) QueryProm(query string) model.Value {
	client, err := api.NewClient(api.Config{
		Address: this.PromUrl,
	})
	if err != nil {
		fmt.Printf("Error creating client: %v\n", err)
		os.Exit(1)
	}

	v1api := v1.NewAPI(client)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	fmt.Println(query)
	result, warnings, err := v1api.Query(ctx, query, time.Now(), v1.WithTimeout(5*time.Second))
	if err != nil {
		fmt.Printf("Error querying Prometheus: %v\n", err)
		os.Exit(1)
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}

	return result
}

func resLookup_item() map[string]float64 {
	result := map[string]float64{
		"cpuUsage":       -1,
		"cpuRequests":    -1,
		"cpuLimits":      -1,
		"memoryUsage":    -1,
		"memoryRequests": -1,
		"memoryLimits":   -1,
	}
	return result
}

func buildResLookup(queryResult model.Value, storeKey string, podInfo map[string]map[string]string, receiver map[string]map[string]float64) {

	if v, ok := queryResult.(model.Vector); ok {
		for _, i := range v {
			if i.Metric["container"] != "" {
				podName := string(i.Metric["pod"])
				if len(podInfo[podName]) != 0 && podInfo[podName]["createdByKind"] != "Job" {
					key := string(i.Metric["namespace"]) + "/" + podInfo[podName]["createdByKind"] + "/" + podInfo[podName]["createdByName"] + "/" + string(i.Metric["container"])
					if receiver[key] == nil {
						receiver[key] = resLookup_item()
					}
					receiver[key][storeKey] = float64(i.Value)
				}

			}
		}
	}
}

func (this *FindResFromPrometheus) FindRes() map[string][]Resource {

	result := map[string][]Resource{
		"candidates":      {},
		"qualified":       {},
		"memoryQualified": {},
		"cpuQualified":    {},
	}
	duration := this.Duration.Hours()
	resLookup := make(map[string]map[string]float64)
	podInfo := make(map[string]map[string]string)

	// define prometheus query string
	cpuUsageResult := this.QueryProm(fmt.Sprintf("quantile_over_time(0.95, sum by (cluster, namespace, pod, container) (irate(container_cpu_usage_seconds_total[5m]))[%.0fh:5m])", duration))
	cpuRequestsResult := this.QueryProm("sum by (cluster, namespace, pod, container) (kube_pod_container_resource_requests{resource=\"cpu\"})")
	cpuLimitsResult := this.QueryProm("sum by (cluster, namespace, pod, container) (kube_pod_container_resource_limits{resource=\"cpu\"})")

	memoryUsageResult := this.QueryProm(fmt.Sprintf("max_over_time(sum by (cluster, namespace, pod, container) (container_memory_working_set_bytes)[%.0fh:5m])", duration))
	memoryRequestsResult := this.QueryProm("sum by (cluster, namespace, pod, container) (kube_pod_container_resource_requests{resource=\"memory\"})")
	memoryLimitsResult := this.QueryProm("sum by (cluster, namespace, pod, container) (kube_pod_container_resource_limits{resource=\"memory\"})")

	podInfoQuery := this.QueryProm("kube_pod_info")

	// get pod info
	if v, ok := podInfoQuery.(model.Vector); ok {
		for _, i := range v {
			podInfo[string(i.Metric["pod"])] = map[string]string{
				"createdByKind": string(i.Metric["created_by_kind"]),
				"createdByName": string(i.Metric["created_by_name"]),
			}
		}
	}

	// fill resources lookup
	buildResLookup(cpuUsageResult, "cpuUsage", podInfo, resLookup)
	buildResLookup(cpuRequestsResult, "cpuRequests", podInfo, resLookup)
	buildResLookup(cpuLimitsResult, "cpuLimits", podInfo, resLookup)
	buildResLookup(memoryUsageResult, "memoryUsage", podInfo, resLookup)
	buildResLookup(memoryRequestsResult, "memoryRequests", podInfo, resLookup)
	buildResLookup(memoryLimitsResult, "memoryLimits", podInfo, resLookup)

	// get resources which cpu usage/request <= 60%
	for k, v := range resLookup {
		res := Resource{
			Id: k,
			ResUsage: ResUsage{
				Duration:    duration,
				Cpu:         v["cpuUsage"],
				CpuRatio:    -1,
				Memory:      v["memoryUsage"],
				MemoryRatio: -1,
			},
			ResAllocation: ResAllocation{
				Requests: ComputeRes{
					Cpu:    v["cpuRequests"],
					Memory: v["memoryRequests"],
				},
				Limits: ComputeRes{
					Cpu:    v["cpuLimits"],
					Memory: v["memoryLimits"],
				},
			},
		}
		if v["cpuRequests"] != float64(-1) {
			res.ResUsage.CpuRatio = float64(v["cpuUsage"] / v["cpuRequests"])
		}
		if v["memoryRequests"] != float64(-1) {
			res.ResUsage.MemoryRatio = float64(v["memoryUsage"] / v["memoryRequests"])
		}

		// drop pods which are down
		if v["cpuUsage"] != -1 && v["memoryUsage"] != -1 {
			if v["cpuRequests"] == float64(-1) {
				if v["memoryRequests"] == float64(-1) {
					result["candidates"] = append(result["candidates"], res)
				} else {
					result["memoryQualified"] = append(result["memoryQualified"], res)
				}
			} else {
				if v["memoryRequests"] == float64(-1) {
					result["cpuQualified"] = append(result["cpuQualified"], res)
				} else {
					result["qualified"] = append(result["qualified"], res)
				}
			}
		}
	}

	return result
}

func computeRecommend(res Resource) (float64, float64) {
	// cpu unit: m, memory unit: M
	cpuRecommend := math.Round(res.ResUsage.Cpu * 1000)
	memoryRecommend := math.Round(res.ResUsage.Memory / (1024 * 1024))
	if cpuRecommend < 1 {
		cpuRecommend = 1
	}
	if memoryRecommend < 1 {
		memoryRecommend = 1
	}

	cpuRecommend = cpuRecommend / 1000
	memoryRecommend = memoryRecommend * 1024 * 1024

	if res.ResUsage.CpuRatio >= 1 {
		cpuRecommend = res.ResAllocation.Requests.Cpu
	}

	if res.ResUsage.MemoryRatio >= 1 {
		memoryRecommend = res.ResAllocation.Requests.Memory
	}

	return cpuRecommend, memoryRecommend
}

func computeScore(originAllocation ResAllocation, recommendAllocation ResAllocation) float64 {
	var score float64
	if originAllocation.Requests.Cpu == -1 {
		score = (originAllocation.Requests.Memory - recommendAllocation.Requests.Memory) / (1024 * 1024 * 1024 * 2)

	} else if originAllocation.Requests.Memory == -1 {
		score = originAllocation.Requests.Cpu - recommendAllocation.Requests.Cpu

	} else {
		score = (originAllocation.Requests.Cpu - recommendAllocation.Requests.Cpu) + (originAllocation.Requests.Memory-recommendAllocation.Requests.Memory)/(1024*1024*1024*2)
	}
	return score
}

func (this *FindResFromPrometheus) RecommendRes(resFound map[string][]Resource) map[string][]OptimizedRes {
	optimizedResult := map[string][]OptimizedRes{
		"scored":   {},
		"unscored": {},
	}

	merged := append(resFound["qualified"], resFound["memoryQualified"]...)
	merged = append(merged, resFound["cpuQualified"]...)
	for _, i := range merged {

		if i.ResUsage.CpuRatio < 1 || i.ResUsage.MemoryRatio < 1 {

			cpuRecommend, memoryRecommend := computeRecommend(i)

			resAllocation := ResAllocation{
				Requests: ComputeRes{
					Cpu:    cpuRecommend,
					Memory: memoryRecommend,
				},
			}

			score := computeScore(i.ResAllocation, resAllocation)
			optimizedRes := OptimizedRes{
				Resource:     i,
				RecommendRes: resAllocation,
				Score:        score,
			}

			optimizedResult["scored"] = append(optimizedResult["scored"], optimizedRes)
		}
	}

	for _, i := range resFound["candidates"] {

		cpuRecommend, memoryRecommend := computeRecommend(i)

		resAllocation := ResAllocation{
			Requests: ComputeRes{
				Cpu:    cpuRecommend,
				Memory: memoryRecommend,
			},
		}

		score := float64(-1)
		optimizedRes := OptimizedRes{
			Resource:     i,
			RecommendRes: resAllocation,
			Score:        score,
		}

		optimizedResult["unscored"] = append(optimizedResult["unscored"], optimizedRes)
	}

	sort.Slice(optimizedResult["scored"], func(i, j int) bool {
		return optimizedResult["scored"][i].Score > optimizedResult["scored"][j].Score
	})

	return optimizedResult
}
