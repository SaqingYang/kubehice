/*
Author: Yang Saqing
Date: 2021-10-21
Email: yangsaqing@163.com
*/

package mhice

import (
	"context"
	"fmt"
	"math"
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

// Name is the name of the plugin used in the plugin registry and configurations.
const Name = "MHice"

// MHice is a plugin that implements Priority based sorting.
type MHice struct {
	ServiceGraph MicroServiceGraph
	Toplogy      NetworkToplogy
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

type NetworkToplogy struct {
	Bandwidth [100][100]float64
	Latency   [100][100]float64
	Node      map[string]int
	Devices   Tree // 这是下一步工作
}

// 下一步工作
type Tree struct {
	Device    string // root表示根
	Bandwidth int64  // Mbps, 到上一级的带宽
	Latency   int64  // us， 到上一级节点的延迟
	Children  []*Tree
}

var _ framework.QueueSortPlugin = &MHice{}
var _ framework.PreFilterPlugin = &MHice{}
var _ framework.ScorePlugin = &MHice{}

func (hice *MHice) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

func (hice *MHice) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

// Name returns name of the plugin.
func (pl *MHice) Name() string {
	return Name
}

// Less 根据微服务交互图和网络拓扑结构来选择Pod优先级，
// example: p1， p2， p4, 已部署微服务p0,p3
// if p1--<1>-->p0, p1--<-1>--p3, p2--<-1>--p0, p2--<0.5>--p3， then p2<p1
// if p1--<-1>-->p0, p2--<-1>-->p3, p1--<4>-->p4, p2--<3>-->p4, then p2<p1

func (pl *MHice) Less(pInfo1, pInfo2 *framework.QueuedPodInfo) bool {

	// 获取p1和p2与集群已有负载的交互关系
	p1ExistedServiceNeighborEdgeWeight := pl.neighborExistedServiceEdgeWeight(pInfo1, &pl.ServiceGraph)
	p2ExistedServiceNeighborEdgeWeight := pl.neighborExistedServiceEdgeWeight(pInfo2, &pl.ServiceGraph)

	// 与集群已有负载有交互关系的优先级更高
	if p1ExistedServiceNeighborEdgeWeight == nil {
		if p2ExistedServiceNeighborEdgeWeight != nil {
			fmt.Println(pInfo1.Pod.Name, "与已有负载有交互 ", pInfo2.Pod.Name, "与已有负载没有交互")
			return true
		}
	} else if p2ExistedServiceNeighborEdgeWeight == nil {
		fmt.Println(pInfo1.Pod.Name, "与已有负载没有交互 ", pInfo2.Pod.Name, "与已有负载有交互")
		return false
	}

	// 如果都与集群已有负载有交互关系，取与集群已有负载的邻边最大者为高优先pod
	if p1ExistedServiceNeighborEdgeWeight != nil && p2ExistedServiceNeighborEdgeWeight != nil {
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

		fmt.Println("都与集群已有负载没有交互! ", pInfo1.Pod.Name, "max Edge=", p1Max, " ", pInfo2.Pod.Name, " max Edge=", p2Max)
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
		fmt.Println("都与集群已有负载没有交互！ ", pInfo1.Pod.Name, "sum Edge=", p1Sum, " ", pInfo2.Pod.Name, " sum Edge=", p2Sum)
		return p1Sum < p2Sum
	}
}

// PreFilter 检查pod是否符合部署要求，即在微服务交互图中，已部署微服务集合中包含微服务交互图中元素，但此微服务与已部署微服务集合中任何元素都没有交互关系时返回错误，等待重新部署。
// 若微服务交互图包含多个独立的子图，另行考虑
func (pl *MHice) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod) *framework.Status {
	// 如果与已部署微服务存在交互，满足部署条件
	callSvc := pl.neighborEdgeWeight(pod, &pl.ServiceGraph)
	fmt.Println(pod.Name, " callSvc", callSvc)
	podIndex, ok := pl.ServiceGraph.Vertex[pod.Labels["app"]]
	if !ok {
		// 没有查到app属于微服务交互图中哪一个Service，无法识别它属于哪一个Service
		return nil
	}

	// 如果与已部署微服务不存在交互，
	// 存在已部署微服务属于微服务交互图，当graph为连通图，则认为不满足部署条件，等待
	maxEdge := int64(-1)
	nodes, _ := pl.handle.SnapshotSharedLister().NodeInfos().List()
	deployedSvc := make(map[int]int)
	for _, node := range nodes {
		for _, np := range node.Pods {
			vb, ok := pl.ServiceGraph.Vertex[np.Pod.Labels["app"]]
			if ok {
				// 此pod在微服务图中
				edgeva2vb := pl.ServiceGraph.Edge[podIndex][vb]
				edgevb2va := pl.ServiceGraph.Edge[vb][podIndex]
				if maxEdge < edgeva2vb {
					maxEdge = edgeva2vb
				}
				if maxEdge < edgevb2va {
					maxEdge = edgevb2va
				}

				deployedSvc[vb] = vb

			}
		}
	}

	// 暂时只考虑单副本
	deployedSvcMaxNeighborEdge := int64(-1)

	for k := range deployedSvc {
		for i := 0; i < pl.ServiceGraph.Num; i++ {
			_, ok := deployedSvc[i]
			if !ok {
				edge := pl.ServiceGraph.Edge[k][i]
				if edge > deployedSvcMaxNeighborEdge {
					fmt.Println("k ", k, "i ", i, "edge ", edge)
					deployedSvcMaxNeighborEdge = edge
				} else {
					edge = pl.ServiceGraph.Edge[i][k]
					if edge > deployedSvcMaxNeighborEdge {
						fmt.Println("k ", k, "i ", i, "edge ", edge)
						deployedSvcMaxNeighborEdge = edge
					}
				}
			}
		}
	}
	fmt.Println("已部署微服务与未部署微服务API调用的最大边=", deployedSvcMaxNeighborEdge)

	// 如果pod与已部署微服务没有API调用关系“-1”
	if maxEdge == -1 {
		fmt.Println("pod与已部署微服务没有API调用关系“-1”")
		maxNeighborSum := int64(-1)
		for i := 0; i < pl.ServiceGraph.Num; i++ {
			neighborSum := int64(0)

			for iCall := 0; iCall < pl.ServiceGraph.Num; iCall++ {
				iCallSum := int64(0)
				callISum := int64(0)
				for j := 0; j < pl.ServiceGraph.Num; j++ {
					if pl.ServiceGraph.Edge[iCall][j] != -1 {
						iCallSum += pl.ServiceGraph.Edge[iCall][j]
					}
					if pl.ServiceGraph.Edge[j][iCall] != -1 {
						callISum += pl.ServiceGraph.Edge[j][iCall]
					}
				}
				neighborSum = iCallSum + callISum
				if neighborSum > maxNeighborSum {
					maxNeighborSum = neighborSum
				}
			}

		}

		neighborEdgeSum := int64(0)
		for _, i := range callSvc {
			neighborEdgeSum += i
		}
		if neighborEdgeSum != maxNeighborSum {
			fmt.Println("neighborEdgeSum ", neighborEdgeSum, "maxNeighborSum ", maxNeighborSum)
			return framework.NewStatus(framework.Error, "pod"+pod.Name+" svc:"+pod.Labels["app"]+" should not schedule until other svc created! ")
		}
	} else {
		fmt.Println("pod与已部署微服务有API调用关系“-1”")
		if maxEdge != deployedSvcMaxNeighborEdge {
			return framework.NewStatus(framework.Error, "pod"+pod.Name+" svc:"+pod.Labels["app"]+" should not schedule until other svc created!")
		}
	}
	return nil
}

// Score 对所有符合部署要求的节点进行排序，原则是预期延迟最低优先，若无法计算，即首次部署微服务，交互需求最多者优先
func (pl *MHice) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	podIndex, ok := pl.ServiceGraph.Vertex[pod.Labels["app"]]
	if !ok {
		// 没有查到app属于微服务交互图中哪一个Service，无法识别它属于哪一个Service
		fmt.Println("非svc: ", pod.Name)
		return 100, nil
	}
	maxEdge := int64(-1)
	nodes, _ := pl.handle.SnapshotSharedLister().NodeInfos().List()
	deployedSvc := make(map[int]string)
	callDeployedPod := make(map[string]*framework.PodInfo)
	calledByDeployedPod := make(map[string]*framework.PodInfo)
	for _, node := range nodes {
		for _, np := range node.Pods {
			vb, ok := pl.ServiceGraph.Vertex[np.Pod.Labels["app"]]
			if ok {
				// 此pod在微服务图中
				edgeva2vb := pl.ServiceGraph.Edge[podIndex][vb]
				edgevb2va := pl.ServiceGraph.Edge[vb][podIndex]
				if edgeva2vb >= 0 {
					callDeployedPod[np.Pod.Name] = np
				}
				if edgevb2va >= 0 {
					calledByDeployedPod[np.Pod.Name] = np
				}
				if maxEdge < edgeva2vb {
					maxEdge = edgeva2vb
				}
				if maxEdge < edgevb2va {
					maxEdge = edgevb2va
				}

				deployedSvc[vb] = node.Node().Name

			}
		}
	}

	nodeInfo, err := pl.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return 0, framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}
	snPerf, ok := nodeInfo.Node().Labels["hice.kj"]
	var jPerf float64
	if ok {
		jPerf, err = strconv.ParseFloat(snPerf, 64)
		if err != nil {
			jPerf = 1.0
		}
	} else {
		jPerf = 1.0
	}

	bsPerf, ok := pod.Labels["hice.kb"]
	var bPerf float64
	if ok {
		bPerf, err = strconv.ParseFloat(bsPerf, 64)
		if err != nil {
			bPerf = 1.0
		}
	} else {
		bPerf = 1.0
	}

	if maxEdge == -1 {
		// pod是首次部署，选择一个资源最丰富的节点、
		memReq := int64(0)
		cpuReq := int64(0)
		for _, container := range pod.Spec.Containers {

			memReq += container.Resources.Requests.Memory().Value()
			cpuReq += container.Resources.Requests.Cpu().MilliValue()
		}
		cpuReq = int64(float64(cpuReq) * bPerf / jPerf)

		//计算异构CPU资源
		// 计算节点已分配CPU资源，由于调度器缓存中的CPU资源数值并没有更新，必须根据kb和kj计算
		// 同时当调度器从Api Server将Pod同步过来后，Pod中的资源数值为真实数值
		// v1.18: 从Api Server同步得到的已调度历史Pod的NodeName字段非空，从缓存中加入的NodeName字段为空
		//        后面的代码并没有对函数中的Pod指针做任何更改
		// v1.22: 缓存的pod也会更新NodeName字段，所以我们使用Pod注释区分是从API Server同步过来的对象还是从缓存中得到的对象
		//        我们在Bind方法中添加了注释字段<fromEtcd, 1>，因此从API Server得到的Pod注释不为空，而从缓存中得到的Pod注释为空
		hiceNodeRequestedMilliCPU := int64(0)
		for _, npod := range nodeInfo.Pods {
			if npod.Pod.Spec.SchedulerName == "kubehice-scheduler" && npod.Pod.Annotations == nil {
				hicePodRequestedMilliCPU := computePodResourceRequest(npod.Pod).MilliCPU

				strHiceKb, ok := npod.Pod.Labels["hice.kb"]
				if !ok {
					strHiceKb = "1.0"
				}
				hiceKb, err := strconv.ParseFloat(strHiceKb, 64)
				if err != nil {
					hiceKb = 1.0
				}
				hiceNodeRequestedMilliCPU = hiceNodeRequestedMilliCPU + int64(float64(hicePodRequestedMilliCPU)*hiceKb/jPerf)
			} else {
				hiceNodeRequestedMilliCPU = hiceNodeRequestedMilliCPU + computePodResourceRequest(npod.Pod).MilliCPU
			}

		}
		// 计算百分比，pod需要的资源/节点剩余资源，越大表示节点剩余资源越少
		ratioCpu := float64(cpuReq) / float64(nodeInfo.Allocatable.MilliCPU-hiceNodeRequestedMilliCPU)
		ratioMem := float64(memReq) / float64(nodeInfo.Allocatable.Memory-nodeInfo.Requested.Memory)
		fmt.Println(nodeInfo.Allocatable.MilliCPU, " ", hiceNodeRequestedMilliCPU, " ", nodeInfo.Allocatable.Memory, " ", nodeInfo.Requested.Memory, " ", cpuReq, " ", memReq)
		// 更大的一项表示资源更紧缺
		maxRatio := math.Max(ratioCpu, ratioMem)
		// 百分比在0~100之间，范围与Score的返回值范围相当，Score分数越大表示越优先，而我们的调度是资源越紧缺优先级越低，因此使用100-百分比，将其翻转
		fmt.Println("首次部署的微服务组件", pod.Name, " 节点分数=", 100-int64(100*maxRatio))
		return 100 - int64(100*maxRatio), nil
	} else {
		// 选择一个期望延迟最低的节点
		maxCallDeployedPodDelay := float64(0)
		maxCalledByDeployedPodDelay := float64(0)
		maxCallDeployedPodNode := pl.Toplogy.Node[nodeName]
		maxCalledByDeployedPodNode := pl.Toplogy.Node[nodeName]
		for _, v := range callDeployedPod {
			svcIndex := pl.ServiceGraph.Vertex[v.Pod.Labels["app"]]
			svcNodeIndex := pl.Toplogy.Node[v.Pod.Spec.NodeName]
			dataTransDelay := float64(pl.ServiceGraph.Edge[podIndex][svcIndex]) / pl.Toplogy.Bandwidth[svcNodeIndex][pl.Toplogy.Node[nodeName]] / 100.0

			propagateDelay := pl.Toplogy.Latency[svcNodeIndex][pl.Toplogy.Node[nodeName]]
			if maxCallDeployedPodDelay < dataTransDelay+propagateDelay {
				maxCallDeployedPodNode = svcNodeIndex
			}
			maxCallDeployedPodDelay = math.Max(maxCallDeployedPodDelay, dataTransDelay+propagateDelay)
		}
		for _, v := range calledByDeployedPod {
			svcIndex := pl.ServiceGraph.Vertex[v.Pod.Labels["app"]]
			svcNodeIndex := pl.Toplogy.Node[v.Pod.Spec.NodeName]
			dataTransDelay := float64(pl.ServiceGraph.Edge[svcIndex][podIndex]) / pl.Toplogy.Bandwidth[svcNodeIndex][pl.Toplogy.Node[nodeName]] / 100.0

			propagateDelay := pl.Toplogy.Latency[svcNodeIndex][pl.Toplogy.Node[nodeName]]
			if maxCalledByDeployedPodDelay < dataTransDelay+propagateDelay {
				maxCalledByDeployedPodNode = svcNodeIndex
			}
			maxCalledByDeployedPodDelay = math.Max(maxCalledByDeployedPodDelay, dataTransDelay+propagateDelay)

		}
		fmt.Println("节点", nodeName, "期望最大延迟=", maxCallDeployedPodDelay+maxCalledByDeployedPodDelay, " 分数=", int64(100*math.Exp(-maxCallDeployedPodDelay-maxCalledByDeployedPodDelay)))
		maxCallDelay := maxCallDeployedPodDelay + maxCalledByDeployedPodDelay
		if maxCallDelay > 10 {
			return 0, nil
		}
		if maxCallDeployedPodNode == pl.Toplogy.Node[nodeName] && pl.Toplogy.Node[nodeName] == maxCalledByDeployedPodNode {
			return 100, nil
		}

		return 100 - int64(maxCallDelay/10*100), nil
		// return int64(100 * math.Exp(-maxCallDeployedPodDelay-maxCalledByDeployedPodDelay)), nil
	}
	// return 100, nil
}

func computePodResourceRequest(pod *v1.Pod) preFilterState {
	result := preFilterState{}
	for _, container := range pod.Spec.Containers {
		result.Add(container.Resources.Requests)
	}

	// take max_resource(sum_pod, any_init_container)
	for _, container := range pod.Spec.InitContainers {
		result.SetMaxResource(container.Resources.Requests)
	}
	return result
}

// preFilterState computed at PreFilter and used at Filter.
type preFilterState struct {
	framework.Resource
}

// New initializes a new plugin and returns it.
func New(_ runtime.Object, h framework.Handle) (framework.Plugin, error) {
	return &MHice{ServiceGraph: *GenerateServiceGraph(), Toplogy: *GenerateNetworkToplogy(), handle: h}, nil
}

// neighborEdgeWeight 根据微服务交互图得到与此Pod相连的所有的微服务调用边
func (pl *MHice) neighborEdgeWeight(p *v1.Pod, graph *MicroServiceGraph) []int64 {

	var edges []int64
	pSvc, ok := p.Labels["app"]
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
			edges = append(edges, graph.Edge[pIndex][v])
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
	pSvc, ok := p.Pod.Labels["app"]
	if !ok {
		return nil
	}
	pIndex, ok := graph.Vertex[pSvc]
	if !ok {
		return nil
	}
	for _, node := range nodes {
		for _, ep := range node.Pods {
			svc, ok := ep.Pod.Labels["app"]
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
	graph.Vertex["adservice"] = 0 //无
	graph.Vertex["cartservice"] = 1
	graph.Vertex["checkoutservice"] = 2
	graph.Vertex["currencyservice"] = 3 //无
	graph.Vertex["emailservice"] = 4    //无
	graph.Vertex["frontend"] = 5
	graph.Vertex["paymentservice"] = 6        //无
	graph.Vertex["productcatalogservice"] = 7 //无
	graph.Vertex["recommendationservice"] = 8
	graph.Vertex["redis-cart"] = 9       //无
	graph.Vertex["shippingservice"] = 10 //无
	for i := 0; i < graph.Num; i++ {
		for j := 0; j < graph.Num; j++ {
			graph.Edge[i][j] = -1
		}
	}

	SetAPICall("frontend", "cartservice", 1, graph)
	SetAPICall("frontend", "recommendationservice", 1, graph)
	SetAPICall("frontend", "productcatalogservice", 100, graph)
	SetAPICall("frontend", "shippingservice", 1, graph)
	SetAPICall("frontend", "checkoutservice", 1, graph)
	SetAPICall("frontend", "adservice", 1, graph)

	SetAPICall("cartservice", "redis-cart", 1, graph)

	SetAPICall("recommendationservice", "productcatalogservice", 10, graph)

	SetAPICall("checkoutservice", "productcatalogservice", 10, graph)
	SetAPICall("checkoutservice", "currencyservice", 1, graph)
	SetAPICall("checkoutservice", "shippingservice", 1, graph)
	SetAPICall("checkoutservice", "paymentservice", 1, graph)
	SetAPICall("checkoutservice", "emailservice", 1, graph)
	SetAPICall("checkoutservice", "cartservice", 1, graph)

	SetAPICall("checkoutservice", "cartservice", 1, graph)
	return graph
}

func SetAPICall(src, dst string, value int, g *MicroServiceGraph) {
	srcIndex := g.Vertex[src]
	dstIndex := g.Vertex[dst]
	g.Edge[srcIndex][dstIndex] = int64(value)
}

func GenerateNetworkToplogy() *NetworkToplogy {
	topology := &NetworkToplogy{}
	topology.Node = make(map[string]int)
	topology.Node["cloud-master"] = 0
	topology.Node["cloud-worker1"] = 1
	topology.Node["cloud-worker3"] = 2
	topology.Node["ubuntu"] = 3
	topology.Node["edge-arm64-1"] = 4
	topology.Node["edge3-amd64-1"] = 5

	b := [6][6]float64{
		{70e3, 40e3, 40e3, 1e3, 1e3, 1e3},
		{40e3, 70e3, 40e3, 1e3, 1e3, 1e3},
		{40e3, 40e3, 70e3, 1e3, 1e3, 1e3},
		{1e3, 1e3, 1e3, 6e3, 1e3, 1e3},
		{1e3, 1e3, 1e3, 1e3, 6e3, 1e3},
		{1e3, 1e3, 1e3, 1e3, 1e3, 60e3}}
	for i := 0; i < 6; i++ {
		for j := 0; j < 6; j++ {
			topology.Bandwidth[i][j] = b[i][j]
		}
	}

	l := [6][6]float64{ //目前的实验环境，延迟全部小于1ms
		{0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0}}
	for i := 0; i < 6; i++ {
		for j := 0; j < 6; j++ {
			topology.Latency[i][j] = l[i][j]
		}
	}
	return topology
}
