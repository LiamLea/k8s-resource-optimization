package optimization

type ComputeRes struct {
	Cpu    float64
	Memory float64
}

type ResAllocation struct {
	Requests ComputeRes
	Limits   ComputeRes
}

type ResUsage struct {
	Duration           float64
	Cpu                []float64
	CpuRequestRatio    float64
	Memory             []float64
	MemoryRequestRatio float64
}

type Resource struct {
	Id            string
	ResUsage      ResUsage
	ResAllocation ResAllocation
}

type OptimizedRes struct {
	Resource
	RecommendRes ResAllocation
	Score        float64
}

type ReportDataItem struct {
	Id     string
	Score  string
	Cpu    map[string]string
	Memory map[string]string
}
type ReportData struct {
	Title    string
	Duration string
	PromUrl  string
	Scored   []ReportDataItem
	Unscored []ReportDataItem
}
