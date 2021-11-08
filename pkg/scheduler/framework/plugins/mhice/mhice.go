/*
Author: Yang Saqing
Date: 2021-10-21
Email: yangsaqing@163.com
*/

package mhice

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

// Name is the name of the plugin used in the plugin registry and configurations.
const Name = "MHice"

// MHice is a plugin that implements Priority based sorting.
type MHice struct {
	ServiceGraph MicroServiceGraph
	handle       framework.Handle
}

// MicroServiceGraph 微服务交互关系图，Edge 有向边表示两个微服务之间的API调用方向，若有API调用，则此值总为非负数
// 大小数据传输大小，单位kbit，四舍五入，若没有交互，则用-1表示
// Vertex 顶点表示微服务，使用 <key, value>表示，key一般为Service编排文件中的匹配标签，value为微服务索引号，
// 与Edge的数组下标对应
type MicroServiceGraph struct {
	Edge   [100][100]int64
	Vertex map[string]int
	Num    int
}

var _ framework.QueueSortPlugin = &MHice{}

// Name returns name of the plugin.
func (pl *MHice) Name() string {
	return Name
}

// Less is the function used by the activeQ heap algorithm to sort pods.
// It sorts pods based on their priority. When priorities are equal, it uses
// PodQueueInfo.timestamp.
func (pl *MHice) Less(pInfo1, pInfo2 *framework.QueuedPodInfo) bool {

	// 获取p1和p2与集群已有负载的交互关系
	p1ExistedServiceNeighborEdgeWeight := pl.neighborExistedServiceEdgeWeight(pInfo1, &pl.ServiceGraph)
	p2ExistedServiceNeighborEdgeWeight := pl.neighborExistedServiceEdgeWeight(pInfo2, &pl.ServiceGraph)

	// 与集群已有负载有交互关系的优先级更高
	if p1ExistedServiceNeighborEdgeWeight == nil {
		if p2ExistedServiceNeighborEdgeWeight != nil {
			return true
		}
	} else if p2ExistedServiceNeighborEdgeWeight == nil {
		return false
	}

	// 如果都与集群已有负载有交互关系，取与集群已有负载的邻边最大者为高优先pod
	if p1ExistedServiceNeighborEdgeWeight == nil && p2ExistedServiceNeighborEdgeWeight == nil {
		p1Max := int64(0)
		p2Max := int64(0)
		for _, e := range p1ExistedServiceNeighborEdgeWeight {
			if p1Max < e {
				p1Max = e
			}
		}

		for _, e := range p2ExistedServiceNeighborEdgeWeight {
			if p2Max < e {
				p2Max = e
			}
		}

		if p1Max > p2Max {
			return false
		} else {
			return true
		}
	} else {
		// 如果都与集群已有负载没有交互关系，取邻边总和最大者为高优先pod
		p1Sum := int64(0)
		p2Sum := int64(0)
		p1Neighbor := pl.neighborEdgeWeight(pInfo1, &pl.ServiceGraph)
		p2Neighbor := pl.neighborEdgeWeight(pInfo2, &pl.ServiceGraph)

		for _, e := range p1Neighbor {
			p1Sum = p1Sum + e
		}
		for _, e := range p2Neighbor {
			p2Sum = p2Sum + e
		}

		return p1Sum < p2Sum
	}
}

// New initializes a new plugin and returns it.
func New(_ runtime.Object, h framework.Handle) (framework.Plugin, error) {
	return &MHice{ServiceGraph: MicroServiceGraph{}, handle: h}, nil
}

func (pl *MHice) neighborEdgeWeight(p *framework.QueuedPodInfo, graph *MicroServiceGraph) []int64 {

	return nil
}

func (pl *MHice) neighborExistedServiceEdgeWeight(p *framework.QueuedPodInfo, graph *MicroServiceGraph) []int64 {

	return nil
}
