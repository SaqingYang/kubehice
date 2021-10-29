/*
Author: Yang Saqing
Date: 2021-10-21
Email: yangsaqing@163.com
*/

package hicev2

import (
	"fmt"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	v1helper "k8s.io/kubernetes/pkg/apis/core/v1/helper"
	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

// preFilterState computed at PreFilter and used at Filter.
type preFilterState struct {
	framework.Resource
}

// Clone the prefilter state.
func (s *preFilterState) Clone() framework.StateData {
	return s
}

// computePodResourceRequest returns a framework.Resource that covers the largest
// width in each resource dimension. Because init-containers run sequentially, we collect
// the max in each dimension iteratively. In contrast, we sum the resource vectors for
// regular containers since they run simultaneously.
//
// If Pod Overhead is specified and the feature gate is set, the resources defined for Overhead
// are added to the calculated Resource request sum
//
// Example:
//
// Pod:
//   InitContainers
//     IC1:
//       CPU: 2
//       Memory: 1G
//     IC2:
//       CPU: 2
//       Memory: 3G
//   Containers
//     C1:
//       CPU: 2
//       Memory: 1G
//     C2:
//       CPU: 1
//       Memory: 1G
//
// Result: CPU: 3, Memory: 3G
func computePodResourceRequest(pod *v1.Pod) preFilterState {
	result := preFilterState{}
	for _, container := range pod.Spec.Containers {
		result.Add(container.Resources.Requests)
	}

	// take max_resource(sum_pod, any_init_container)
	for _, container := range pod.Spec.InitContainers {
		result.SetMaxResource(container.Resources.Requests)
	}

	// If Overhead is being utilized, add to the total requests for the pod
	if pod.Spec.Overhead != nil && utilfeature.DefaultFeatureGate.Enabled(features.PodOverhead) {
		result.Add(pod.Spec.Overhead)
	}

	return result
}

// PreFilter invoked at the prefilter extension point.
// func (f *Hice) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod) *framework.Status {
// cycleState.Write(preFilterStateKey, computePodResourceRequest(pod))
// 	return nil
// }

// func getPreFilterState(cycleState *framework.CycleState) (*preFilterState, error) {
// 	c, err := cycleState.Read(preFilterStateKey)
// 	if err != nil {
// preFilterState doesn't exist, likely PreFilter wasn't invoked.
// 		return nil, fmt.Errorf("error reading %q from cycleState: %v", preFilterStateKey, err)
// 	}

// 	s, ok := c.(*preFilterState)
// 	if !ok {
// 		return nil, fmt.Errorf("%+v  convert to NodeResourcesFit.preFilterState error", c)
// 	}
// 	return s, nil
// }

// InsufficientResource describes what kind of resource limit is hit and caused the pod to not fit the node.
type InsufficientResource struct {
	ResourceName v1.ResourceName
	// We explicitly have a parameter for reason to avoid formatting a message on the fly
	// for common resources, which is expensive for cluster autoscaler simulations.
	Reason    string
	Requested int64
	Used      int64
	Capacity  int64
}

func fitsRequest(podRequest *preFilterState, nodeInfo *framework.NodeInfo, ignoredExtendedResources, ignoredResourceGroups sets.String) []InsufficientResource {
	if nodeInfo.Node().Name == "cloud-master" {
		fmt.Println("cloud master", podRequest.MilliCPU)
	}
	insufficientResources := make([]InsufficientResource, 0, 4)

	allowedPodNumber := nodeInfo.Allocatable.AllowedPodNumber
	if len(nodeInfo.Pods)+1 > allowedPodNumber {
		insufficientResources = append(insufficientResources, InsufficientResource{
			v1.ResourcePods,
			"Too many pods",
			1,
			int64(len(nodeInfo.Pods)),
			int64(allowedPodNumber),
		})
	}

	if podRequest.MilliCPU == 0 &&
		podRequest.Memory == 0 &&
		podRequest.EphemeralStorage == 0 &&
		len(podRequest.ScalarResources) == 0 {
		return insufficientResources
	}

	// Hicev2 节点能力感知的CPU资源计算
	// 将节点上已有Pod的CPU资源单位转换为节点能力的单位
	var hiceNodeRequestedMilliCPU int64
	hiceNodeRequestedMilliCPU = 0

	// 获取节点单线程能力
	strHiceKj, ok := nodeInfo.Node().Labels["hice.kj"]
	if !ok {
		strHiceKj = "1.0"
	}
	hiceKj, err := strconv.ParseFloat(strHiceKj, 64)
	if err != nil {
		hiceKj = 1.0
	}

	// 计算节点已分配CPU资源，由于调度器缓存中的CPU资源数值并没有更新，必须根据kb和kj计算
	// 同时当调度器从Api Server将Pod同步过来后，Pod中的资源数值为真实数值
	// 从Api Server同步得到的已调度历史Pod的NodeName字段非空，从缓存中加入的NodeName字段为空
	// 后面的代码并没有对函数中的Pod指针做任何更改
	for _, pod := range nodeInfo.Pods {
		if pod.Pod.Spec.SchedulerName == "kubehice-scheduler" && pod.Pod.Spec.NodeName == "" {

			// 方法1： 下面的注释为通过Pod标签获得Pod的请求资源量，已弃用
			// strPodMilliCPU, ok := pod.Pod.Labels["hice.cpu.req"]

			// if !ok {
			// 	strPodMilliCPU = "0m"
			// }
			// strPodMilliCPU = strings.Split(strPodMilliCPU, "m")[0]
			// hicePodRequestedMilliCPU, err := strconv.ParseInt(strPodMilliCPU, 10, 64)
			// if err != nil {
			// 	hicePodRequestedMilliCPU = 0
			// }

			// 方法2： 下面代码假设没有nodeName字段的Pod也没有更新资源配置数值
			hicePodRequestedMilliCPU := computePodResourceRequest(pod.Pod).MilliCPU

			strHiceKb, ok := pod.Pod.Labels["hice.kb"]
			if !ok {
				strHiceKb = "1.0"
			}
			hiceKb, err := strconv.ParseFloat(strHiceKb, 64)
			if err != nil {
				hiceKb = 1.0
			}
			hiceNodeRequestedMilliCPU = hiceNodeRequestedMilliCPU + int64(float64(hicePodRequestedMilliCPU)*hiceKb/hiceKj)
		} else {
			hiceNodeRequestedMilliCPU = hiceNodeRequestedMilliCPU + computePodResourceRequest(pod.Pod).MilliCPU
		}

	}
	// 此处podRequest.MilliCPU已经转换为hiceCPU
	// Hice v2 CPU资源过滤
	if nodeInfo.Allocatable.MilliCPU < podRequest.MilliCPU+hiceNodeRequestedMilliCPU {
		insufficientResources = append(insufficientResources, InsufficientResource{
			v1.ResourceCPU,
			"Insufficient hice v2 cpu",
			podRequest.MilliCPU,
			hiceNodeRequestedMilliCPU,
			nodeInfo.Allocatable.MilliCPU,
		})
	}

	if nodeInfo.Allocatable.Memory < podRequest.Memory+nodeInfo.Requested.Memory {
		insufficientResources = append(insufficientResources, InsufficientResource{
			v1.ResourceMemory,
			"Insufficient memory",
			podRequest.Memory,
			nodeInfo.Requested.Memory,
			nodeInfo.Allocatable.Memory,
		})
	}
	if nodeInfo.Allocatable.EphemeralStorage < podRequest.EphemeralStorage+nodeInfo.Requested.EphemeralStorage {
		insufficientResources = append(insufficientResources, InsufficientResource{
			v1.ResourceEphemeralStorage,
			"Insufficient ephemeral-storage",
			podRequest.EphemeralStorage,
			nodeInfo.Requested.EphemeralStorage,
			nodeInfo.Allocatable.EphemeralStorage,
		})
	}

	for rName, rQuant := range podRequest.ScalarResources {
		if v1helper.IsExtendedResourceName(rName) {
			// If this resource is one of the extended resources that should be ignored, we will skip checking it.
			// rName is guaranteed to have a slash due to API validation.
			var rNamePrefix string
			if ignoredResourceGroups.Len() > 0 {
				rNamePrefix = strings.Split(string(rName), "/")[0]
			}
			if ignoredExtendedResources.Has(string(rName)) || ignoredResourceGroups.Has(rNamePrefix) {
				continue
			}
		}
		if nodeInfo.Allocatable.ScalarResources[rName] < rQuant+nodeInfo.Requested.ScalarResources[rName] {
			insufficientResources = append(insufficientResources, InsufficientResource{
				rName,
				fmt.Sprintf("Insufficient %v", rName),
				podRequest.ScalarResources[rName],
				nodeInfo.Requested.ScalarResources[rName],
				nodeInfo.Allocatable.ScalarResources[rName],
			})
		}
	}

	return insufficientResources
}
