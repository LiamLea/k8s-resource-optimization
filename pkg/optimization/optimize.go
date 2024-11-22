package optimization

import (
	"context"
	"fmt"
	gh "github.com/dustin/go-humanize"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"gonum.org/v1/gonum/stat"
	"k8s-resource-optimization/pkg/utils"
	"math"
	"os"
	"slices"
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

func resLookup_item() map[string]interface{} {
	result := map[string]interface{}{
		"cpuUsage":       []float64{},
		"cpuRequests":    float64(-1),
		"cpuLimits":      float64(-1),
		"memoryUsage":    []float64{},
		"memoryRequests": float64(-1),
		"memoryLimits":   float64(-1),
	}
	return result
}

func (this *FindResFromPrometheus) buildRelationMap(receiver map[string]map[string]interface{}) map[string]string {
	podControllerMap := make(map[string]string)
	podDeploymentQuery := this.QueryProm("(kube_replicaset_owner * on(replicaset,namespace)  group_right(owner_kind,owner_name) label_replace(kube_pod_info{created_by_kind=\"ReplicaSet\"}, \"replicaset\", \"$1\", \"created_by_name\", \"(.*)\")) * on(pod,namespace) group_right(owner_kind,owner_name) kube_pod_container_info")
	podControllersQuery := this.QueryProm("label_replace(label_replace(kube_pod_info{created_by_kind!=\"ReplicaSet\"}, \"owner_name\", \"$1\", \"created_by_name\", \"(.*)\"),\"owner_kind\", \"$1\", \"created_by_kind\", \"(.*)\") * on(pod,namespace) group_right(owner_kind,owner_name) kube_pod_container_info")
	v1, ok1 := podDeploymentQuery.(model.Vector)
	v2, ok2 := podControllersQuery.(model.Vector)
	if ok1 && ok2 {
		merged := append(v1, v2...)
		for _, i := range merged {
			if i.Metric["owner_kind"] != "Job" {
				key := string(i.Metric["namespace"] + "/" + i.Metric["owner_kind"] + "/" + i.Metric["owner_name"] + "/" + i.Metric["container"])
				receiver[key] = resLookup_item()

				// fill podControllerMap
				podControllerMap[string(i.Metric["namespace"]+"/"+i.Metric["pod"])] = string(i.Metric["namespace"] + "/" + i.Metric["owner_kind"] + "/" + i.Metric["owner_name"])
			}
		}
	}

	return podControllerMap
}

func buildResLookup(queryResult model.Value, storeKey string, podInfo map[string]string, receiver map[string]map[string]interface{}) {

	if v, ok := queryResult.(model.Vector); ok {
		for _, i := range v {
			if i.Metric["container"] != "" {
				podName := string(i.Metric["namespace"] + "/" + i.Metric["pod"])
				if len(podInfo[podName]) != 0 {
					key := podInfo[podName] + "/" + string(i.Metric["container"])
					if v1, ok1 := receiver[key][storeKey].([]float64); ok1 {
						receiver[key][storeKey] = append(v1, float64(i.Value))
					} else {
						receiver[key][storeKey] = float64(i.Value)
					}
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
	resLookup := make(map[string]map[string]interface{})
	podInfo := make(map[string]string)

	// define prometheus query string
	cpuUsageResult := this.QueryProm(fmt.Sprintf("quantile_over_time(0.95, sum by (cluster, namespace, pod, container) (irate(container_cpu_usage_seconds_total[5m]))[%.0fh:5m])", duration))
	cpuRequestsResult := this.QueryProm("sum by (cluster, namespace, pod, container) (kube_pod_container_resource_requests{resource=\"cpu\"})")
	cpuLimitsResult := this.QueryProm("sum by (cluster, namespace, pod, container) (kube_pod_container_resource_limits{resource=\"cpu\"})")

	memoryUsageResult := this.QueryProm(fmt.Sprintf("max_over_time(sum by (cluster, namespace, pod, container) (container_memory_working_set_bytes)[%.0fh:5m])", duration))
	memoryRequestsResult := this.QueryProm("sum by (cluster, namespace, pod, container) (kube_pod_container_resource_requests{resource=\"memory\"})")
	memoryLimitsResult := this.QueryProm("sum by (cluster, namespace, pod, container) (kube_pod_container_resource_limits{resource=\"memory\"})")

	// build relationship map
	podInfo = this.buildRelationMap(resLookup)

	// fill resources lookup
	buildResLookup(cpuUsageResult, "cpuUsage", podInfo, resLookup)
	buildResLookup(cpuRequestsResult, "cpuRequests", podInfo, resLookup)
	buildResLookup(cpuLimitsResult, "cpuLimits", podInfo, resLookup)
	buildResLookup(memoryUsageResult, "memoryUsage", podInfo, resLookup)
	buildResLookup(memoryRequestsResult, "memoryRequests", podInfo, resLookup)
	buildResLookup(memoryLimitsResult, "memoryLimits", podInfo, resLookup)

	// get all resources
	for k, v := range resLookup {
		res := Resource{
			Id: k,
			ResUsage: ResUsage{
				Duration:           duration,
				Cpu:                v["cpuUsage"].([]float64),
				CpuRequestRatio:    -1,
				Memory:             v["memoryUsage"].([]float64),
				MemoryRequestRatio: -1,
			},
			ResAllocation: ResAllocation{
				Requests: ComputeRes{
					Cpu:    v["cpuRequests"].(float64),
					Memory: v["memoryRequests"].(float64),
				},
				Limits: ComputeRes{
					Cpu:    v["cpuLimits"].(float64),
					Memory: v["memoryLimits"].(float64),
				},
			},
		}
		if v["cpuRequests"] != float64(-1) {
			res.ResUsage.CpuRequestRatio = stat.Mean(v["cpuUsage"].([]float64), nil) / v["cpuRequests"].(float64)
		}
		if v["memoryRequests"] != float64(-1) {
			res.ResUsage.MemoryRequestRatio = stat.Mean(v["memoryUsage"].([]float64), nil) / v["memoryRequests"].(float64)
		}

		// drop pods which are down
		if len(v["cpuUsage"].([]float64)) != 0 && len(v["memoryUsage"].([]float64)) != 0 {
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
	// collect: cpu unit: 1, memory unit: bytes
	// covert:  cpu unit: m, memory unit: MB
	cpuRecommend := math.Ceil(slices.Max(res.ResUsage.Cpu)*100) * 10
	memoryRecommend := math.Ceil(slices.Max(res.ResUsage.Memory)/(1024*1024)/10) * 10

	if cpuRecommend < 10 {
		cpuRecommend = 10
	}
	if memoryRecommend < 1 {
		memoryRecommend = 1
	}

	cpuRecommend = cpuRecommend / 1000
	memoryRecommend = memoryRecommend * 1024 * 1024

	if res.ResUsage.CpuRequestRatio >= 1 {
		cpuRecommend = res.ResAllocation.Requests.Cpu
	}

	if res.ResUsage.MemoryRequestRatio >= 1 {
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

		if i.ResUsage.CpuRequestRatio < 1 || i.ResUsage.MemoryRequestRatio < 1 {

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

	// render html
	data := ReportData{
		Title: "Resource Report",
	}
	for n, i := range optimizedResult["scored"] {
		if n > 10 {
			break
		}
		item := ReportDataItem{
			Id:    i.Id,
			Score: fmt.Sprintf("%.2f", i.Score),
			Cpu: map[string]string{
				"usage":     fmt.Sprintf("%.0f m (%.1f%%)", stat.Mean(i.ResUsage.Cpu, nil)*1000, i.ResUsage.CpuRequestRatio*100),
				"requests":  fmt.Sprintf("%.0f m", i.ResAllocation.Requests.Cpu*1000),
				"recommend": fmt.Sprintf("%.0f m", i.RecommendRes.Requests.Cpu*1000),
				"limits":    fmt.Sprintf("%.0f m", i.ResAllocation.Limits.Cpu*1000),
			},
			Memory: map[string]string{
				"usage":     fmt.Sprintf("%s (%.1f%%)", gh.IBytes(uint64(int(stat.Mean(i.ResUsage.Memory, nil)))), i.ResUsage.MemoryRequestRatio*100),
				"requests":  fmt.Sprintf("%s", gh.IBytes(uint64(i.ResAllocation.Requests.Memory))),
				"recommend": fmt.Sprintf("%s", gh.IBytes(uint64(i.RecommendRes.Requests.Memory))),
				"limits":    fmt.Sprintf("%s", gh.IBytes(uint64(i.ResAllocation.Limits.Memory))),
			},
		}
		data.Scored = append(data.Scored, item)
	}

	utils.DumpHtmlTable("pkg/optimization/templates/recommend.html", data, "report.html")

	return optimizedResult
}
