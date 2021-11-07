/*
Author: Yang Saqing
Date: 2021-10-21
Email: yangsaqing@163.com
*/
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Shopify/sarama"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	//
)

func main() {

	clientset, err := GetConfig()
	if err != nil {
		panic(err.Error())
	}
	// ReadNodesStat(clientset)
	// stat, index := GetControlledContainers(clientset)
	// workLoadInfo(stat)
	// for id, p := range index {
	// 	fmt.Println(id, p.Name, p.ControlledBy.Name, p.Node)
	// }
	ReadNodesStat(clientset)
}

// The data structure that holds the state of the cluster
type WorkLoad struct {
	NameSpaces map[string]*NameSpace
}
type NameSpace struct {
	Name            string
	ControlledBy    *WorkLoad
	ControllerTypes map[string]*ControllerType
}
type ControllerType struct {
	Name         string
	ControlledBy *NameSpace
	Controllers  map[string]*Controller
}
type Controller struct {
	Name         string
	ControlledBy *ControllerType
	Pods         map[string]*Pod
}
type Pod struct {
	Name         string
	ControlledBy *Controller
	Containers   map[string]*Container
	Node         string
}
type Container struct {
	Name         string
	ControlledBy *Pod
	Id           string
	CpuReq       int64
	CpuLmt       int64
	CpuUsage     int64
	Node         string
}

func GetPodsBelongToController(clientset *kubernetes.Clientset) map[string]v1.Pod {
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	// var items []v1.Pod
	controlldPods := make(map[string]v1.Pod)
	for _, pod := range pods.Items {
		//fmt.Println(index, len(pod.OwnerReferences))
		if pod.Status.Phase != "Running" {
			continue
		}
		for ior := 0; ior < len(pod.OwnerReferences); ior++ {
			if *pod.OwnerReferences[ior].Controller {
				orcn := pod.OwnerReferences[ior].Kind
				if orcn == "DaemonSet" || orcn == "Deployment" || orcn == "ReplicaSet" || orcn == "StatefulSet" || orcn == "ReplicationController" {
					for ics := 0; ics < len(pod.Status.ContainerStatuses); ics++ {
						// items = append(items, pod)
						// fmt.Println(orcn, "/", pod.Namespace, "/", pod.OwnerReferences[ior].Name, "/", pod.Name, "/", pod.Status.ContainerStatuses[ics].ContainerID[9:], "/", pod.Spec.NodeName)
						controlldPods[pod.Status.ContainerStatuses[ics].ContainerID[9:]] = pod

					}
				}
			}
		}

	}
	return controlldPods
}

func GetControlledContainers(clientset *kubernetes.Clientset) (*WorkLoad, map[string]*Container) {
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	// var items []v1.Pod
	var workload WorkLoad
	workload.NameSpaces = make(map[string]*NameSpace)

	var runningContainersMap map[string]*Container
	runningContainersMap = make(map[string]*Container)

	for _, pod := range pods.Items {
		//fmt.Println(index, len(pod.OwnerReferences))
		if pod.Status.Phase != "Running" {
			continue
		}

		// Check the namespace, or create it if it does not exist
		pns, ok := workload.NameSpaces[pod.Namespace]
		if !ok {
			var newNs NameSpace
			newNs.ControllerTypes = make(map[string]*ControllerType)
			newNs.Name = pod.Namespace
			pns = &newNs
			pns.ControlledBy = &workload
			workload.NameSpaces[pod.Namespace] = pns
		}

		for ior := 0; ior < len(pod.OwnerReferences); ior++ {
			if *pod.OwnerReferences[ior].Controller && pod.Status.Phase == "Running" {
				// Check the controllertype, or create it if it does not exist
				pControllerType, ok := pns.ControllerTypes[pod.OwnerReferences[ior].Kind]
				if !ok {
					var newCtype ControllerType
					newCtype.Controllers = make(map[string]*Controller)
					newCtype.Name = pod.OwnerReferences[ior].Kind
					pControllerType = &newCtype
					newCtype.ControlledBy = pns
					pns.ControllerTypes[pod.OwnerReferences[ior].Kind] = pControllerType
				}
				// Check the controller, or create it if it does not exist
				pController, ok := pControllerType.Controllers[pod.OwnerReferences[ior].Name]
				if !ok {
					var newController Controller
					newController.Pods = make(map[string]*Pod)
					newController.Name = pod.OwnerReferences[ior].Name
					pController = &newController
					pController.ControlledBy = pControllerType
					pControllerType.Controllers[pod.OwnerReferences[ior].Name] = pController
				}
				// Check the pod, or create it if it does not exist
				cPod, ok := pController.Pods[pod.Name]
				if !ok {
					var newPod Pod
					newPod.Name = pod.Name
					newPod.Containers = make(map[string]*Container)
					cPod = &newPod
					cPod.ControlledBy = pController
					newPod.Node = pod.Spec.NodeName
					pController.Pods[pod.Name] = cPod
				}

				for ics := 0; ics < len(pod.Status.ContainerStatuses); ics++ {

					var newContainer Container
					containerName := pod.Status.ContainerStatuses[ics].Name
					cPod.Containers[containerName] = &newContainer
					newContainer.ControlledBy = cPod
					newContainer.Name = containerName
					newContainer.Id = pod.Status.ContainerStatuses[ics].ContainerID[9:]
					newContainer.Node = pod.Spec.NodeName

					for _, specContainer := range pod.Spec.Containers {
						if specContainer.Name == containerName {
							runningContainersMap[newContainer.Id] = &newContainer
							newContainer.CpuReq = specContainer.Resources.Requests.Cpu().MilliValue()
							newContainer.CpuLmt = specContainer.Resources.Limits.Cpu().MilliValue()
							break
						}
					}

				}
			}
		}

	}
	return &workload, runningContainersMap
}

func workLoadInfo(workload *WorkLoad) {
	fmt.Printf("workload:%p\n", workload)
	for nsName, ns := range workload.NameSpaces {
		fmt.Printf("  %v:%p controlledby %p\n", nsName, ns, ns.ControlledBy)
		for controlleTypeName, controllerType := range ns.ControllerTypes {
			fmt.Printf("    %v:%p controlledby %p\n", controlleTypeName, controllerType, controllerType.ControlledBy)
			for controllerName, controller := range controllerType.Controllers {
				fmt.Printf("      %v:%p controlledby %p\n", controllerName, controller, controller.ControlledBy)
				for pName, p := range controller.Pods {
					fmt.Printf("        %v:%p controlledby %p\n", pName, p, p.ControlledBy)
					for cName, c := range p.Containers {
						fmt.Printf("          %v:%p controlledby %p :", cName, c, c.ControlledBy)
						fmt.Printf("          %v\n", *c)
					}
				}
			}
		}

	}
}
func GetConfig() (*kubernetes.Clientset, error) {

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(err)
	}
	return kubernetes.NewForConfig(config)
}

// Sarama configuration options
var (
	brokers    = ""
	version    = ""
	group      = ""
	topics     = ""
	assignor   = ""
	oldest     = true
	verbose    = false
	batch      = 1
	kubeconfig = ""
)

type pidStats struct {
	Current int `json:"current"`
	Limit   int `json:"limit"`
}
type cpuUsage struct {
	Total_usage         int   `json:"total_usage"`
	Percpu_usage        []int `json:"percpu_usage"`
	Usage_in_kernelmode int   `json:"usage_in_kernelmode"`
	Usage_in_usermode   int   `json:"usage_in_usermode"`
}
type cpuStats struct {
	Cpu_usage        cpuUsage       `json:"cpu_usage"`
	System_cpu_usage int64          `json:"system_cpu_usage"`
	Online_cpus      int            `json:"online_cpus"`
	Throttling_data  map[string]int `json:"throttling_data"`
}
type containerStats struct {
	Read          string      `json:"read"`
	Preread       string      `json:"preread"`
	Pids_stats    interface{} `json:"pids_stats"`
	Blkio_stats   interface{} `json:"blkio_stats"`
	Num_procs     int         `json:"num_procs"`
	Storage_stats interface{} `json:"storage_stats"`
	Cpu_stats     cpuStats    `json:"cpu_stats"`
	Precpu_stats  cpuStats    `json:"precpu_stats"`
	Memory_stats  interface{} `json:"memory_stats"`
	Name          string      `json:"name"`
	Id            string      `json:"id"`
	Networks      interface{} `json:"networks"`
}

type ContainerCpuStat struct {
	Id    string
	Usage int
}
type NodeStat struct {
	NodeName          string
	ContainersCpuStat []ContainerCpuStat
}

func init() {
	flag.StringVar(&brokers, "brokers", "192.168.10.12:9092", "Kafka bootstrap brokers to connect to, as a comma separated list")
	flag.StringVar(&group, "group", "ysq", "Kafka consumer group definition")
	flag.StringVar(&version, "version", "2.1.1", "Kafka cluster version")
	flag.StringVar(&topics, "topics", "nodesstats", "Kafka topics to be consumed, as a comma separated list")
	flag.StringVar(&assignor, "assignor", "range", "Consumer group partition assignment strategy (range, roundrobin, sticky)")
	flag.BoolVar(&oldest, "oldest", true, "Kafka consumer consume initial offset from oldest")
	flag.BoolVar(&verbose, "verbose", false, "Sarama logging")
	flag.IntVar(&batch, "batch", 10, "print inf batch records per second")
	if home := homedir.HomeDir(); home != "" {
		flag.StringVar(&kubeconfig, "kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	if len(brokers) == 0 {
		panic("no Kafka bootstrap brokers defined, please set the -brokers flag")
	}

	if len(topics) == 0 {
		panic("no topics given to be consumed, please set the -topics flag")
	}

	if len(group) == 0 {
		panic("no Kafka consumer group defined, please set the -group flag")
	}
	if len(kubeconfig) == 0 {
		panic("no kubeconfig given, please set the kubeconfig flag")
	}
}

func ReadNodesStat(clientset *kubernetes.Clientset) {

	wg := &sync.WaitGroup{}
	wg.Add(1)
	consumer2, err := sarama.NewConsumer(strings.Split(brokers, ","), nil)
	if err != nil {
		log.Printf("Failed to start consumer:%s\n", err)
	}

	partitionList, err := consumer2.Partitions(topics)
	if err != nil {
		log.Println("Failed to get the list of partitions: ", err)
	}

	for partition := range partitionList {
		pc, err := consumer2.ConsumePartition(topics, int32(partition), sarama.OffsetNewest)
		if err != nil {
			log.Printf("Failed to start consumer for partition %d: %s\n", partition, err)
		}
		defer pc.AsyncClose()

		wg.Add(1)

		//      fmt.Println("abc")
		go func(sarama.PartitionConsumer) {

			defer wg.Done()
			lastUpdateTime := time.Now()
			_, containerIndex := GetControlledContainers(clientset)
			nodesPerf := GetNodePerf(clientset)

			for msg := range pc.Messages() {
				if time.Now().Sub(lastUpdateTime) > 120*time.Second {
					lastUpdateTime = time.Now()
					_, containerIndex = GetControlledContainers(clientset)
					nodesPerf = GetNodePerf(clientset)
				}
				var nodeStat NodeStat
				err = json.Unmarshal(msg.Value, &nodeStat)
				if err != nil {
					panic(err)
				}
				for _, c := range nodeStat.ContainersCpuStat {
					tc, ok := containerIndex[c.Id]
					if ok {
						pod := tc.ControlledBy
						controller := pod.ControlledBy
						tc.CpuUsage = int64(c.Usage)

						if controller.ControlledBy.Name == "Node" {
							continue
						}
						fmt.Printf("%s:%v replicas\n", controller.Name, len(controller.Pods))
						for cpName, cp := range controller.Pods {
							fmt.Printf(" %s ->Node:%s ", cpName, cp.Node)
							for cpcName, cpc := range cp.Containers {
								if cpc.CpuUsage != 0 {
									fmt.Printf("%s Req->%v Lmt->%v Use->%v\n", cpcName, cpc.CpuReq, cpc.CpuLmt, cpc.CpuUsage)
								} else {
									fmt.Println()
								}
							}

						}
						fmt.Println(computePerf(controller, nodesPerf))

					}
				}
			}
		}(pc)
	}
	wg.Wait()
	log.Println("Done consuming topic hello")
	consumer2.Close()
}

// NewProducerClient create a kafka client
func NewProducerClient(brokers []string) (sarama.Client, error) {
	config := sarama.NewConfig()                              //实例化个sarama的Config
	config.Producer.Return.Successes = true                   //是否开启消息发送成功后通知 successes channel
	config.Producer.Partitioner = sarama.NewRandomPartitioner //随机分区器
	return sarama.NewClient(brokers, config)                  //初始化客户端
}

func GetNodePerf(clientset *kubernetes.Clientset) map[string]float64 {
	nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil
	}
	nodesPerf := make(map[string]float64)
	for _, node := range nodes.Items {
		sperf, ok := node.Labels["kubhc/performance"]
		perf := 1.0
		if ok {
			perf, err = strconv.ParseFloat(sperf, 64)
			if err != nil {
				perf = 1.0
			}
		}
		nodesPerf[node.Name] = perf
	}
	return nodesPerf
}

func computePerf(controller *Controller, nodesPerf map[string]float64) map[string]float64 {
	newNodesPerf := make(map[string]float64)
	for _, pod := range controller.Pods {
		perf := -1.0
		for _, c := range pod.Containers {
			if c.CpuUsage == 0 {
				continue
			}
			perf = 1.0 / float64(c.CpuUsage)
		}
		if perf > 0 {
			newNodesPerf[pod.Node] = perf
		}
	}
	k := 0.0

	for nodeName, perf := range newNodesPerf {
		k = k + math.Log(nodesPerf[nodeName]/perf)
	}
	k = k / float64(len(newNodesPerf))
	// k = math.Exp(k)

	for nodeName, perf := range newNodesPerf {
		newNodesPerf[nodeName] = math.Log(nodesPerf[nodeName]/perf) - k
	}
	return newNodesPerf
}
