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
	Duration    float64
	Cpu         float64
	CpuRatio    float64
	Memory      float64
	MemoryRatio float64
}

type Resource struct {
	Id            string
	ResUsage      ResUsage
	ResAllocation ResAllocation
}

type OptimizedRes struct {
	currentRes   Resource
	recommendRes Resource
}
