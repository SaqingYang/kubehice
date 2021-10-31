/*
Author: Yang Saqing
Date: 2021-10-21
Email: yangsaqing@163.com
*/
package hicev2

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"time"

	"go.etcd.io/etcd/client/pkg/v3/transport"
	clientv3 "go.etcd.io/etcd/client/v3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/protobuf"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	etcdscheme "k8s.io/kubectl/pkg/scheme"
	framework "k8s.io/kubernetes/pkg/scheduler/framework"
	hicev1 "k8s.io/kubernetes/pkg/scheduler/framework/plugins/hicev1"
)

// Hice is a plugin that checks if a node's arch equal to pod and score node based on performance.
type Hice struct {
	handle                framework.Handle
	ignoredResources      sets.String
	ignoredResourceGroups sets.String
}

// var _ framework.PreFilterPlugin = &Hice{}
var _ framework.FilterPlugin = &Hice{}
var _ framework.BindPlugin = &Hice{}

// var _ framework.ScorePlugin = &Hice{}

const (
	// Name is the name of the plugin used in the plugin registry and configurations.
	Name = "Hicev2"

	// Using the name of the plugin will likely help us avoid collisions with other plugins.
	preFilterStateKey = "PreFilter" + Name
	// ErrReason when node ports aren't available.
	ErrReasonPerf = "node(s)'s performace is too low"
)

// ScoreExtention
func (hice *Hice) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

// ImageInf represents information about architecture and os of a image.
type ImageInf struct {
	Name string `json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`
	Arch string `json:"arch,omitempty" protobuf:"bytes,2,opt,name=arch"`
	Os   string `json:"os,omitempty" protobuf:"bytes,3,opt,name=os"`
}

// Images represents information about multi-architecture version images of a image.
type Images struct {
	Name      string     `json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`
	ImageInfs []ImageInf `json:"images,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,2,rep,name=images"`
}

// ImagesList represents information about kubhc multi-architecture version images list
type ImagesList struct {
	List []Images `json:"list,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=list"`
}

// GetMultiArchImages 返回可用版本的多架构镜像名
func GetMultiArchImages(imageName string, imagedata []byte) []ImageInf {

	imageList := &ImagesList{}
	err := json.Unmarshal(imagedata, imageList)
	if err != nil {
		log.Println(err)
		return nil
	}

	containers := imageList.List
	for _, item := range containers {
		if imageName == item.Name {
			return item.ImageInfs
		}
	}
	// 若无多架构镜像信息，返回空对象
	return nil
}

// Name returns name of the plugin. It is used in logs, etc.
func (pl *Hice) Name() string {
	return Name
}

// PreFilterExtensions do not exist for this plugin.
func (pl *Hice) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

// 过滤节点单线程能力过低节点
func (pl *Hice) Filter(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {

	// 获取Pod中对CPU资源最高的容器的资源值
	var maxCpuReq int64
	maxCpuReq = 0
	for _, c := range pod.Spec.Containers {
		if c.Resources.Requests.Cpu().MilliValue() > maxCpuReq {
			maxCpuReq = c.Resources.Requests.Cpu().MilliValue()
		}
	}

	var kb float64

	// 得到Pod资源配置的参考节点的单线程能力
	strkb, ok := pod.Labels["hice.kb"]
	if !ok {
		kb = 1.0
	} else {
		var err error
		kb, err = strconv.ParseFloat(strkb, 64)
		if err != nil {
			kb = 1.0
		}
	}

	// 得到Node的单线程能力
	var kj float64
	strkj, ok := nodeInfo.Node().Labels["hice.kj"]
	if !ok {
		kj = 1.0
	} else {
		var err error
		kj, err = strconv.ParseFloat(strkj, 64)
		if err != nil {
			kj = 1.0
		}
	}
	// 对CPU核心数的需求一致，如在参考节点上需要0.8核，则此容器不能分配超过1核（0.8向上取整）
	if float64(maxCpuReq)/1000.0/kj*kb > math.Ceil(float64(maxCpuReq)/1000.0) {
		return framework.NewStatus(framework.Unschedulable, ErrReasonPerf)
	}

	// 资源过滤器
	// ts, err := getPreFilterState(cycleState)

	// 注意！！！
	// 此处使用指针变量存在问题，在执行到fitsRequest时，preFilter会修改指针指向的内容，导致数据错误，原因未知
	// 解决方法是创建一个新的对象并将内容复制出来
	// s := preFilterState{}
	// s = *ts
	s := computePodResourceRequest(pod)

	// 获取节点单线程能力
	strHiceKj, ok := nodeInfo.Node().Labels["hice.kj"]
	if !ok {
		strHiceKj = "1.0"
	}
	hiceKj, err := strconv.ParseFloat(strHiceKj, 64)
	if err != nil {
		hiceKj = 1.0
	}
	// 获取Pod的参考节点能力
	strHiceKb, ok := pod.Labels["hice.kb"]
	if !ok {
		strHiceKb = "1.0"
	}
	hiceKb, err := strconv.ParseFloat(strHiceKb, 64)
	if err != nil {
		hiceKb = 1.0
	}

	s.MilliCPU = int64(float64(s.MilliCPU) * hiceKb / hiceKj)
	// if nodeInfo.Node().Name == "cloud-worker2" {
	// 	fmt.Println("node", nodeInfo.Node().Name, " pod: ", pod.Name, "kj", hiceKj, "req", s.MilliCPU, "node allocatable: ", nodeInfo.Allocatable.MilliCPU, nodeInfo.Requested.MilliCPU)
	// }

	insufficientResources := fitsRequest(&s, nodeInfo, pl.ignoredResources, pl.ignoredResourceGroups)

	if len(insufficientResources) != 0 {
		// We will keep all failure reasons.
		failureReasons := make([]string, 0, len(insufficientResources))
		for _, r := range insufficientResources {
			failureReasons = append(failureReasons, r.Reason)
		}
		return framework.NewStatus(framework.Unschedulable, failureReasons...)
	}
	return nil
}

// New initializes a new plugin and returns it.
func New(plArgs runtime.Object, h framework.Handle) (framework.Plugin, error) {
	return &Hice{handle: h}, nil
}

// Bind binds pods to nodes using the k8s client.
// 其中一个返回了nil，将不再执行其它插件的bind函数
func (b Hice) Bind(ctx context.Context, state *framework.CycleState, p *v1.Pod, nodeName string) *framework.Status {
	klog.V(3).Infof("Attempting to bind %v/%v to %v", p.Namespace, p.Name, nodeName)

	c, _ := state.Read("images")
	images, _ := c.(hicev1.ImageState)
	node, err := b.handle.ClientSet().CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}
	tempPod := p.DeepCopy()

	// 替换镜像名为目标节点架构版本镜像
	etcdServer := os.Getenv("ETCD_SERVER")
	etcdPeerKey := os.Getenv("PEER_KEY")
	etcdPeerCrt := os.Getenv("PEER_CRT")
	etcdCaCrt := os.Getenv("CA_CRT")
	etcdCli, err := etcdClient(etcdServer, etcdPeerKey, etcdPeerCrt, etcdCaCrt)
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}
	defer etcdCli.Close()

	ConvertPodImages(tempPod, node.Labels["kubernetes.io/arch"], images)

	// 获取节点单线程能力
	strHiceKj, ok := node.Labels["hice.kj"]
	if !ok {
		strHiceKj = "1.0"
	}
	hiceKj, err := strconv.ParseFloat(strHiceKj, 64)
	if err != nil {
		hiceKj = 1.0
	}
	// 获取Pod的参考节点能力
	strHiceKb, ok := tempPod.Labels["hice.kb"]
	if !ok {
		strHiceKb = "1.0"
	}
	hiceKb, err := strconv.ParseFloat(strHiceKb, 64)
	if err != nil {
		hiceKb = 1.0
	}
	ConvertPodCpu(tempPod, hiceKb, hiceKj)

	defer etcdCli.Close()
	tempPod.Spec.NodeName = nodeName

	err = changePodInEtcd(tempPod, etcdCli)
	if err != nil {
		fmt.Printf("bind error %v\n", p.Name)
		return framework.NewStatus(framework.Error, err.Error())
	}
	return nil

}

// ConvertPodImages 节点已经经过Prefilter，必定为Pod可部署节点。
// 直接部署Pod存在此问题，利用deployment、replicaset、job等上层封装不存在此问题，暂时不改进
func ConvertPodImages(p *v1.Pod, arch string, imagedata []byte) {
	for index, container := range p.Spec.Containers {
		images := GetMultiArchImages(container.Image, imagedata)
		for _, image := range images {
			if arch == image.Arch {
				p.Spec.Containers[index].Image = image.Name
				break
			}
		}
	}
}

// 修改Pod中的CPU资源配置
func ConvertPodCpu(p *v1.Pod, kb, kj float64) {
	for index, container := range p.Spec.Containers {
		if container.Resources.Requests.Cpu().MilliValue() != 0 {
			p.Spec.Containers[index].Resources.Requests["cpu"] = resource.MustParse(strconv.Itoa(int(float64(container.Resources.Requests.Cpu().MilliValue())*kb/kj)) + "m")
		}

		if container.Resources.Limits.Cpu().MilliValue() != 0 {
			p.Spec.Containers[index].Resources.Limits["cpu"] = resource.MustParse(strconv.Itoa(int(float64(container.Resources.Limits.Cpu().MilliValue())*kb/kj)) + "m")
		}

	}
}
func etcdClient(endpoint, keyFile, certFile, caFile string) (*clientv3.Client, error) {
	var tlsConfig *tls.Config
	if len(certFile) != 0 || len(keyFile) != 0 || len(caFile) != 0 {
		tlsInfo := transport.TLSInfo{
			CertFile:      certFile,
			KeyFile:       keyFile,
			TrustedCAFile: caFile,
		}
		var err error
		tlsConfig, err = tlsInfo.ClientConfig()
		if err != nil {
			return nil, err
		}
	}
	config := clientv3.Config{
		Endpoints:   []string{endpoint},
		TLS:         tlsConfig,
		DialTimeout: 5 * time.Second,
	}
	return clientv3.New(config)
}

func changePodInEtcd(p *v1.Pod, cli *clientv3.Client) error {
	key := "/registry/pods/" + p.Namespace + "/" + p.Name
	resp, err := clientv3.NewKV(cli).Get(context.Background(), key)
	if err != nil {
		fmt.Println("get resp error")
		return err
	}

	decoder := etcdscheme.Codecs.UniversalDeserializer()

	obj, _, _ := decoder.Decode(resp.Kvs[0].Value, nil, nil)
	etcdPod := obj.(*v1.Pod)
	// 遍历修改etcdContainer中的容器配置
	for index, container := range etcdPod.Spec.Containers {
		for _, pContainer := range p.Spec.Containers {
			if container.Name == pContainer.Name {
				etcdPod.Spec.Containers[index].Image = pContainer.Image
				etcdPod.Spec.Containers[index].Resources = pContainer.Resources
				break
			}
		}
	}

	// 添加bind信息
	etcdPod.Spec.NodeName = p.Spec.NodeName

	etcdPod.Status.Conditions = append(etcdPod.Status.Conditions, v1.PodCondition{
		Type:   v1.PodScheduled,
		Status: v1.ConditionTrue,
	})
	if etcdPod.Annotations == nil {
		etcdPod.Annotations = make(map[string]string)
		etcdPod.Annotations["fromEtcd"] = "1"
	}

	// 序列化etcdPod对象
	protoSerializer := protobuf.NewSerializer(etcdscheme.Scheme, etcdscheme.Scheme)
	newObj := new(bytes.Buffer)
	protoSerializer.Encode(obj, newObj)

	_, err = clientv3.NewKV(cli).Put(context.Background(), key, newObj.String())
	return err

}

// Score a plugin of get nodes' priority for heterogeneous single-thread performance cluster
func (hice *Hice) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	nodeInfo, err := hice.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return 0, framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}
	snPerf, ok := nodeInfo.Node().Labels["hice.kj"]
	var nPerf float64
	if ok {
		nPerf, err = strconv.ParseFloat(snPerf, 64)
		if err != nil {
			nPerf = 1.0
		}
	} else {
		nPerf = 1.0
	}

	bsPerf, ok := pod.Labels["hice.kb"]
	var bPerf float64
	if ok {
		bPerf, err = strconv.ParseFloat(bsPerf, 64)
		if err != nil {
			bPerf = 1.0
		}
	} else {
		return 100, nil
	}
	pLevel := 0
	// nLevel1 := 0
	// nLevel2 := 0
	for _, container := range pod.Spec.Containers {
		req := container.Resources.Requests.Cpu().MilliValue()
		lmt := container.Resources.Limits.Cpu().MilliValue()
		level := hiceLevel(req, lmt, bPerf, nPerf)
		if pLevel < level {
			pLevel = level
		}
	}
	switch pLevel {
	case 0:
		// All containers are working properly
		// fmt.Printf("hice score of %v->%v\n", nodeName, 100)
		return 100, nil
	case 1:
		// Some containers work, but there may be performance degradation
		// fmt.Printf("hice score of %v->%v\n", nodeName, int64(51+49*nPerf/bPerf))
		return int64(51 + 49*nPerf/bPerf), nil
	case 2:
		// fmt.Printf("hice score of %v->%v\n", nodeName, 1+49*nPerf/bPerf)
		return int64(1 + 49*nPerf/bPerf), nil
	default:
		// The CPU performance of the node cannot meet the container running requirements
		// fmt.Printf("hice score of %v->%v\n", nodeName, 0)
		return 0, nil
	}
}

// hiceLevel 按照节点能力、Pod内容器的CPU资源请求，100=L0>L1>L2>L3=0，L3已经被过滤
func hiceLevel(req, lmt int64, bPerf, nPerf float64) int {
	if bPerf <= nPerf {
		return 0
	}
	if req == 0 && lmt == 0 {
		return 0
	}
	if req == 0 && lmt > 0 {
		req = lmt
	}
	hiceReq := int64(float64(req) / nPerf * bPerf)
	hiceLmt := int64(float64(lmt) / nPerf * bPerf)
	var level int

	if hiceReq <= mCeil(req) {
		if hiceLmt <= mCeil(lmt) {
			level = 1
		} else {
			level = 2
		}
	} else {
		level = 3
	}
	return level
}

func mCeil(x int64) int64 {
	if x/1000*1000-x == 0 {
		return x
	}
	return (x/1000 + 1) * 1000
}
