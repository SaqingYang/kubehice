/*
Author: Yang Saqing
Date: 2021-10-21
Email: yangsaqing@163.com
*/

package mhice

import (
	"context"

	v1 "k8s.io/api/core/v1"
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
		p1Neighbor := pl.neighborEdgeWeight(pInfo1.Pod, &pl.ServiceGraph)
		p2Neighbor := pl.neighborEdgeWeight(pInfo2.Pod, &pl.ServiceGraph)

		for _, e := range p1Neighbor {
			p1Sum = p1Sum + e
		}
		for _, e := range p2Neighbor {
			p2Sum = p2Sum + e
		}

		return p1Sum < p2Sum
	}
}

// PreFilter 检查pod是否符合部署要求，即在微服务交互图中，已部署微服务集合中包含微服务交互图中元素，但此微服务与已部署微服务集合中任何元素都没有交互关系时返回错误，等待重新部署。
// 若微服务交互图包含多个独立的子图，另行考虑
func (pl *MHice) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod) *framework.Status {
	// 如果与已部署微服务存在交互，满足部署条件
	callExistedSvc := pl.neighborEdgeWeight(pod, &pl.ServiceGraph)
	if len(callExistedSvc) != 0 {
		return nil
	}

	// 如果与已部署微服务不存在交互，
	// 存在已部署微服务属于微服务交互图，当graph为连通图，则认为不满足部署条件，等待
	nodes, _ := pl.handle.SnapshotSharedLister().NodeInfos().List()
	for _, node := range nodes {
		for _, np := range node.Pods {
			_, ok := pl.ServiceGraph.Vertex[np.Pod.Labels["svc"]]
			if ok {
				// 当graph为连通图，则认为不满足部署条件，等待
				return framework.NewStatus(framework.Wait, "pod"+pod.Name+" svc:"+pod.Labels["svc"]+" should not schedule until other more import svc deployed!")
				// 此此pod属于另一个子图，此子图与集群已有微服务无交互，待优化
			}
		}
	}
	return nil
}

// Score 对所有符合部署要求的节点进行排序，原则是预期延迟最低优先，若无法计算，即首次部署微服务，交互需求最多者优先
func (pl *MHice) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	return 100, nil
}

// NormalizeScore将不同的打分插件所得的分数合并
func (pl *MHice) NormalizeScore(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod, scores framework.NodeScoreList) *framework.Status {
	return nil
}

// New initializes a new plugin and returns it.
func New(_ runtime.Object, h framework.Handle) (framework.Plugin, error) {
	return &MHice{ServiceGraph: *GenerateServiceGraph(), handle: h}, nil
}

// neighborEdgeWeight 根据微服务交互图得到与此Pod相连的所有的微服务调用边
func (pl *MHice) neighborEdgeWeight(p *v1.Pod, graph *MicroServiceGraph) []int64 {

	var edges []int64
	pSvc, ok := p.Labels["svc"]
	if !ok {
		return nil
	}
	pIndex, ok := graph.Vertex[pSvc]
	if !ok {
		return nil
	}

	for _, v := range graph.Vertex {
		if graph.Edge[v][pIndex] >= 0 {
			edges = append(edges, graph.Edge[v][pIndex])
			continue
		}
		if graph.Edge[pIndex][v] >= 0 {
			edges = append(edges, graph.Edge[v][pIndex])
		}

	}
	return edges
}

// neighborExistedServiceEdgeWeight根据集群已有状态和微服务交互图提取与已有
// 微服务实例交互的微服务边
func (pl *MHice) neighborExistedServiceEdgeWeight(p *framework.QueuedPodInfo, graph *MicroServiceGraph) []int64 {
	nodes, err := pl.handle.SnapshotSharedLister().NodeInfos().List()
	if err != nil {
		return nil
	}
	var edges []int64
	pSvc, ok := p.Pod.Labels["svc"]
	if !ok {
		return nil
	}
	pIndex, ok := graph.Vertex[pSvc]
	if !ok {
		return nil
	}
	for _, node := range nodes {
		for _, ep := range node.Pods {
			svc, ok := ep.Pod.Labels["svc"]
			if !ok {
				continue
			}
			out := graph.Edge[pIndex][graph.Vertex[svc]]
			in := graph.Edge[graph.Vertex[svc]][pIndex]
			if in >= 0 {
				edges = append(edges, in)
			}
			if out >= 0 {
				edges = append(edges, out)
			}
		}
	}
	return edges
}

// GenerateServiceGraph, 前期开发过程使用，用于生成微服务交互图
func GenerateServiceGraph() *MicroServiceGraph {
	graph := &MicroServiceGraph{Num: 11}

	graph.Vertex = make(map[string]int)
	graph.Vertex["ad"] = 0 //无
	graph.Vertex["cart"] = 1
	graph.Vertex["checkout"] = 2
	graph.Vertex["currency"] = 3 //无
	graph.Vertex["email"] = 4    //无
	graph.Vertex["frontend"] = 5
	graph.Vertex["payment"] = 6        //无
	graph.Vertex["productcatalog"] = 7 //无
	graph.Vertex["recommendation"] = 8
	graph.Vertex["redis-cart"] = 9 //无
	graph.Vertex["shipping"] = 10  //无
	for i := 0; i < graph.Num; i++ {
		for j := 0; j < graph.Num; j++ {
			graph.Edge[i][j] = -1
		}
	}

	SetAPICall("frontend", "cart", 1, graph)
	SetAPICall("frontend", "recommendation", 1, graph)
	SetAPICall("frontend", "productcatalog", 100, graph)
	SetAPICall("frontend", "shipping", 1, graph)
	SetAPICall("frontend", "checkout", 1, graph)
	SetAPICall("frontend", "ad", 1, graph)

	SetAPICall("cart", "redis-cart", 1, graph)

	SetAPICall("recommendation", "productcatalog", 10, graph)

	SetAPICall("checkout", "productcatalog", 10, graph)
	SetAPICall("checkout", "currency", 1, graph)
	SetAPICall("checkout", "shipping", 1, graph)
	SetAPICall("checkout", "payment", 1, graph)
	SetAPICall("checkout", "email", 1, graph)
	SetAPICall("checkout", "cart", 1, graph)

	SetAPICall("checkout", "cart", 1, graph)
	return graph
}

func SetAPICall(src, dst string, value int, g *MicroServiceGraph) {
	srcIndex := g.Vertex[src]
	dstIndex := g.Vertex[dst]
	g.Edge[srcIndex][dstIndex] = int64(value)
}
