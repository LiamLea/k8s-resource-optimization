# Resource Optimization


<!-- @import "[TOC]" {cmd="toc" depthFrom=1 depthTo=6 orderedList=false} -->

<!-- code_chunk_output -->

- [Resource Optimization](#resource-optimization)
    - [Introduction](#introduction)
      - [1.Find Resources](#1find-resources)
        - [(1) Identify Resources](#1-identify-resources)
        - [(2) Conditions](#2-conditions)
      - [2.Give Recommendations](#2give-recommendations)

<!-- /code_chunk_output -->


### Introduction

#### 1.Find Resources

##### (1) Identify Resources
* resource id: `<namespace>/<controller_type>/<controller_name>/<container_name>`
    * why this form:
        * use controller can trace the utilization of an application rather than a pod

* resource categories
    * common controllers: Deployment, StatefulSet, DaemonSet
    * cron job (TODO)
        * jobs run periodically 

##### (2) Conditions
* set different conditions for different envs to find qualified resources

* test env
    * cpu: 95 quantile < requests
    * memory: max < requests
    * duration: 1 week

#### 2.Give Recommendations
* resource recommendation
    * cpu minimum: 10m
    * memory minimum: 10M
* resource score
    * `score= (cpu_reserved + memory_reserved / (1024*1024*1024*2)) * replicas`
        * cpu weight is `1`, memory weight is `1/(1024*1024*1024*2)` (refer to 1C/2G)