package optimization

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"os"
	"time"
)

type IFind interface {
	FindRes() []Resource
}

type IRecommend interface {
	RecommendRes(resource Resource) []OptimizedRes
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

func (this *FindResFromPrometheus) FindRes() []Resource {
	var result []Resource
	duration := this.Duration.Hours()
	resLookup := make(map[string]map[string]float64)

	// define prometheus query string
	cpuUsageResult := this.QueryProm(fmt.Sprintf("quantile_over_time(0.95, sum by (cluster, namespace, pod, container) (irate(container_cpu_usage_seconds_total[5m]))[%.0fh:5m])", duration))
	cpuRequestsResult := this.QueryProm("sum by (cluster, namespace, pod, container) (kube_pod_container_resource_requests{resource=\"cpu\"})")
	cpuLimitsResult := this.QueryProm("sum by (cluster, namespace, pod, container) (kube_pod_container_resource_limits{resource=\"cpu\"})")

	memoryUsageResult := this.QueryProm(fmt.Sprintf("max_over_time(sum by (cluster, namespace, pod, container) (container_memory_working_set_bytes)[%.0fh:5m])", duration))
	memoryRequestsResult := this.QueryProm("sum by (cluster, namespace, pod, container) (kube_pod_container_resource_requests{resource=\"memory\"})")
	memoryLimitsResult := this.QueryProm("sum by (cluster, namespace, pod, container) (kube_pod_container_resource_limits{resource=\"memory\"})")

	// fill resources lookup
	if v, ok := cpuUsageResult.(model.Vector); ok {
		for _, i := range v {
			if i.Metric["container"] != "" {
				key := string(i.Metric["namespace"] + "/" + i.Metric["pod"] + "/" + i.Metric["container"])
				if resLookup[key] == nil {
					resLookup[key] = make(map[string]float64)
				}
				resLookup[key]["cpuUsage"] = float64(i.Value)
			}
		}
	}

	if v, ok := cpuRequestsResult.(model.Vector); ok {
		for _, i := range v {
			if i.Metric["container"] != "" {
				key := string(i.Metric["namespace"] + "/" + i.Metric["pod"] + "/" + i.Metric["container"])
				if resLookup[key] == nil {
					resLookup[key] = make(map[string]float64)
				}
				resLookup[key]["cpuRequests"] = float64(i.Value)
			}
		}
	}

	if v, ok := cpuLimitsResult.(model.Vector); ok {
		for _, i := range v {
			if i.Metric["container"] != "" {
				key := string(i.Metric["namespace"] + "/" + i.Metric["pod"] + "/" + i.Metric["container"])
				if resLookup[key] == nil {
					resLookup[key] = make(map[string]float64)
				}
				resLookup[key]["cpuLimits"] = float64(i.Value)
			}
		}
	}

	if v, ok := memoryUsageResult.(model.Vector); ok {
		for _, i := range v {
			if i.Metric["container"] != "" {
				key := string(i.Metric["namespace"] + "/" + i.Metric["pod"] + "/" + i.Metric["container"])
				if resLookup[key] == nil {
					resLookup[key] = make(map[string]float64)
				}
				resLookup[key]["memoryUsage"] = float64(i.Value)
			}
		}
	}

	if v, ok := memoryRequestsResult.(model.Vector); ok {
		for _, i := range v {
			if i.Metric["container"] != "" {
				key := string(i.Metric["namespace"] + "/" + i.Metric["pod"] + "/" + i.Metric["container"])
				if resLookup[key] == nil {
					resLookup[key] = make(map[string]float64)
				}
				resLookup[key]["memoryRequests"] = float64(i.Value)
			}
		}
	}

	if v, ok := memoryLimitsResult.(model.Vector); ok {
		for _, i := range v {
			if i.Metric["container"] != "" {
				key := string(i.Metric["namespace"] + "/" + i.Metric["pod"] + "/" + i.Metric["container"])
				if resLookup[key] == nil {
					resLookup[key] = make(map[string]float64)
				}
				resLookup[key]["memoryLimits"] = float64(i.Value)
			}
		}
	}

	// get resources which cpu usage/request <= 60%
	for k, v := range resLookup {
		res := Resource{
			Id: k,
			ResUsage: ResUsage{
				Duration:    duration,
				Cpu:         v["cpuUsage"],
				CpuRatio:    float64(v["cpuUsage"] / v["cpuRequests"]),
				Memory:      v["memoryUsage"],
				MemoryRatio: float64(v["memoryUsage"] / v["memoryRequests"]),
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
		result = append(result, res)
	}

	for _, i := range result {
		data, err := json.Marshal(i)
		if err != nil {
			println("json dump failed:", err.Error())
		}
		fmt.Println(string(data))
	}
	return result
}
