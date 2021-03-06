/*
Author: Yang Saqing
Date: 2021-10-21
Email: yangsaqing@163.com
*/

package hicev1

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"go.etcd.io/etcd/client/pkg/v3/transport"
	clientv3 "go.etcd.io/etcd/client/v3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/protobuf"
	"k8s.io/klog/v2"
	etcdscheme "k8s.io/kubectl/pkg/scheme"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

// Hice is a plugin that checks if a node's arch equal to pod and score node based on performance.
type Hice struct {
	handle framework.Handle
}

var _ framework.PreFilterPlugin = &Hice{}
var _ framework.FilterPlugin = &Hice{}
var _ framework.BindPlugin = &Hice{}

// var _ framework.ScorePlugin = &Hice{}

const (
	// Name is the name of the plugin used in the plugin registry and configurations.
	Name = "Hicev1"

	// preFilterStateKey is the key in CycleState to Hice pre-computed data.
	// Using the name of the plugin will likely help us avoid collisions with other plugins.
	preFilterStateKey = "PreFilter" + Name
	imageStateKey     = "images"
	// ErrReason when node ports aren't available.
	ErrReasonArch = "node(s)'s arch isn't included in pods arches"
	ErrReasonPerf = "node(s)'s performace is too low"
)

type preFilterState []string

type ImageState []byte

// ScoreExtention
func (hice *Hice) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

// Clone the prefilter state.
func (s preFilterState) Clone() framework.StateData {
	// The state is not impacted by adding/removing existing pods, hence we don't need to make a deep copy.
	return s
}

func (s ImageState) Clone() framework.StateData {
	return s
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

type UnAvailableImages struct {
	Images []string `json:"images,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=images"`
}

// GetMultiArchImages ???????????????????????????????????????
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
	// fmt.Println("iamge name", imageName)
	// ?????????????????????????????????????????????
	return nil
}

// PodAvailableArch ??????etcd????????????Pod???????????????
func PodAvailableArch(pod *v1.Pod, imagedata []byte) []string {
	containers := pod.Spec.Containers
	availableArch := make(map[string]string)

	// ???Etcd??????????????????????????????

	for _, item := range GetMultiArchImages(containers[0].Image, imagedata) {
		availableArch[item.Arch] = item.Arch
	}
	// fmt.Println("containers ", len(containers))
	for i := 1; i < len(containers); i++ {
		temp := make(map[string]string)
		for _, item := range GetMultiArchImages(containers[i].Image, imagedata) {
			_, ok := availableArch[item.Arch]
			// fmt.Println("container i", item.Arch, ok)
			if ok {
				temp[item.Arch] = item.Arch
			}
		}
		availableArch = temp
	}
	var arches []string
	for _, arch := range availableArch {
		arches = append(arches, arch)
	}
	return arches
}

// Name returns name of the plugin. It is used in logs, etc.
func (pl *Hice) Name() string {
	return Name
}

// PreFilter invoked at the prefilter extension point.
func (pl *Hice) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod) *framework.Status {
	// ??????etcd??????????????????????????????
	etcdServer := os.Getenv("ETCD_SERVER")
	etcdPeerKey := os.Getenv("PEER_KEY")
	etcdPeerCrt := os.Getenv("PEER_CRT")
	etcdCaCrt := os.Getenv("CA_CRT")
	etcdCli, err := etcdClient(etcdServer, etcdPeerKey, etcdPeerCrt, etcdCaCrt)
	if err != nil {
		return nil
	}
	defer etcdCli.Close()
	key := "kubehice/images"
	resp, err := etcdCli.Get(context.Background(), key)
	if err != nil {
		fmt.Println("get resp error")
		return framework.NewStatus(framework.Error, err.Error())
	}

	data := resp.Kvs[0].Value
	unAvailableImages := GetUnAvailableImages(pod, data)
	// ?????????Pod????????????????????????????????????????????????Etcd??? ???kubehice/unavailableimages??? ???
	if len(unAvailableImages.Images) != 0 {
		resp, _ = etcdCli.Get(ctx, "kubehice/unavailableimages")
		var oldEtcdData []byte
		if len(resp.Kvs) != 0 {
			oldEtcdData = resp.Kvs[0].Value
		}
		unAvailableImageData := UpdateUnAvailableImageData(oldEtcdData, unAvailableImages)
		etcdCli.Put(ctx, "kubehice/unavailableimages", string(unAvailableImageData))
		framework.NewStatus(framework.Error, "Can't find \""+unAvailableImages.Images[0]+"\" in etcd!")

	}

	s := PodAvailableArch(pod, data)
	klog.V(3).Infof("Attempting to prefilte node by arches %v", s)
	cycleState.Write(preFilterStateKey, preFilterState(s))
	cycleState.Write(imageStateKey, ImageState(data))
	return nil
}

// GetUnAvailableImages ??????pod???????????????????????????????????????Etcd???MultArchImages???
func GetUnAvailableImages(pod *v1.Pod, data []byte) UnAvailableImages {
	unAvailableImages := UnAvailableImages{}
	for _, c := range pod.Spec.Containers {
		if len(GetMultiArchImages(c.Image, data)) == 0 {
			unAvailableImages.Images = append(unAvailableImages.Images, c.Image)
		}
	}
	return unAvailableImages
}

// ??????unavailable imaes???Etcd???????????????????????????????????????????????????
func UpdateUnAvailableImageData(old []byte, images UnAvailableImages) []byte {
	var unAvailableImageData []byte
	if len(images.Images) != 0 {
		if len(old) == 0 {
			unAvailableImageData, _ = json.Marshal(images)
			return unAvailableImageData
		} else {
			existedUnAvailableImages := &UnAvailableImages{}
			err := json.Unmarshal(old, existedUnAvailableImages)
			if err != nil {
				panic(err)
			}
			upgradeAble := false
			for _, image := range images.Images {
				isOld := false
				for _, existedImage := range existedUnAvailableImages.Images {
					if image == existedImage {
						isOld = true
						break
					}
				}
				if !isOld {
					upgradeAble = true
					existedUnAvailableImages.Images = append(existedUnAvailableImages.Images, image)
				}
			}
			if upgradeAble {
				unAvailableImageData, _ = json.Marshal(existedUnAvailableImages)
				return unAvailableImageData
			} else {
				return old
			}

		}
	}
	return old
}

// PreFilterExtensions do not exist for this plugin.
func (pl *Hice) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

func getPreFilterState(cycleState *framework.CycleState) (preFilterState, error) {
	c, err := cycleState.Read(preFilterStateKey)
	if err != nil {
		// preFilterState doesn't exist, likely PreFilter wasn't invoked.
		return nil, fmt.Errorf("error reading %q from cycleState: %v", preFilterStateKey, err)
	}

	s, ok := c.(preFilterState)
	if !ok {
		return nil, fmt.Errorf("%+v  convert to hicev1.preFilterState error", c)
	}
	return s, nil
}

func getImageState(cycleState *framework.CycleState) (ImageState, error) {
	c, err := cycleState.Read(imageStateKey)
	if err != nil {
		// preFilterState doesn't exist, likely PreFilter wasn't invoked.
		return nil, fmt.Errorf("error reading %q from cycleState: %v", preFilterStateKey, err)
	}

	s, ok := c.(ImageState)
	if !ok {
		return nil, fmt.Errorf("%+v  convert to hice.imageState error", c)
	}
	return s, nil
}

// Filter invoked at the filter extension point.
func (pl *Hice) Filter(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {

	availableArches, err := getPreFilterState(cycleState)
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}
	fits := fitsArches(availableArches, nodeInfo)
	if !fits {
		return framework.NewStatus(framework.Unschedulable, ErrReasonArch)
	}
	return nil
}

func fitsArches(availableArches []string, nodeInfo *framework.NodeInfo) bool {
	nodeArch := nodeInfo.Node().Labels["kubernetes.io/arch"]
	for _, arch := range availableArches {
		if nodeArch == arch {
			return true
		}
	}
	return false
}

// New initializes a new plugin and returns it.
func New(_ runtime.Object, h framework.Handle) (framework.Plugin, error) {
	return &Hice{handle: h}, nil
}

// Bind binds pods to nodes using the k8s client.
// ?????????????????????nil?????????????????????????????????bind??????
func (b Hice) Bind(ctx context.Context, state *framework.CycleState, p *v1.Pod, nodeName string) *framework.Status {
	klog.V(3).Infof("Attempting to bind %v/%v to %v", p.Namespace, p.Name, nodeName)
	// binding := &v1.Binding{
	// 	ObjectMeta: metav1.ObjectMeta{Namespace: p.Namespace, Name: p.Name, UID: p.UID},
	// 	Target:     v1.ObjectReference{Kind: "Node", Name: nodeName},
	// }

	nodeInfo, err := b.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)

	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}
	node := nodeInfo.Node()
	tempPod := p.DeepCopy()

	// ????????????????????????????????????????????????
	imageData, _ := getImageState(state)
	ConvertPodImages(tempPod, node.Labels["kubernetes.io/arch"], imageData)

	// AddHiceSchedulerFlag(&tempPod)
	etcdServer := os.Getenv("ETCD_SERVER")
	etcdPeerKey := os.Getenv("PEER_KEY")
	etcdPeerCrt := os.Getenv("PEER_CRT")
	etcdCaCrt := os.Getenv("CA_CRT")
	etcdCli, err := etcdClient(etcdServer, etcdPeerKey, etcdPeerCrt, etcdCaCrt)

	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}
	defer etcdCli.Close()
	err = changePodInEtcd(tempPod, etcdCli)
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}

	// ?????????bind?????????
	// err = b.handle.ClientSet().CoreV1().Pods(binding.Namespace).Bind(ctx, binding, metav1.CreateOptions{})

	// if err != nil {
	// 	changePodInEtcd(p, etcdCli)
	// 	return framework.NewStatus(framework.Error, err.Error())
	// }
	return nil

}

// ConvertPodImages ??????????????????Prefilter????????????Pod??????????????????
// ????????????Pod????????????????????????deployment???replicaset???job???????????????????????????????????????????????????
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
	// ????????????etcdContainer??????????????????
	for index, container := range etcdPod.Spec.Containers {
		for _, pContainer := range p.Spec.Containers {
			if container.Name == pContainer.Name {
				etcdPod.Spec.Containers[index].Image = pContainer.Image
				// etcdPod.Spec.Containers[index].Resources = pContainer.Resources
				break
			}
		}
	}
	// ??????bind??????
	etcdPod.Spec.NodeName = p.Spec.NodeName

	etcdPod.Status.Conditions = append(etcdPod.Status.Conditions, v1.PodCondition{
		Type:   v1.PodScheduled,
		Status: v1.ConditionTrue,
	})

	// ?????????etcdPod??????
	protoSerializer := protobuf.NewSerializer(etcdscheme.Scheme, etcdscheme.Scheme)
	newObj := new(bytes.Buffer)
	protoSerializer.Encode(obj, newObj)

	_, err = clientv3.NewKV(cli).Put(context.Background(), key, newObj.String())
	return err

}
