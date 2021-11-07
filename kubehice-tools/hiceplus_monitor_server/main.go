package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"

	"github.com/Shopify/sarama"
	"github.com/containerd/containerd/log"
)

type ContainerCpuStat struct {
	Id    string
	Usage int
}
type ContainerInf struct {
	Time      int64
	Container string
	Cpu       string
	Id        string
	Image     string
	Name      string
	Namespace string
	Pod       string
}

var (
	port    = "0"
	brokers = ""
	topics  = ""
)

func init() {
	flag.StringVar(&port, "port", "12345", "kubehice server port")
	flag.StringVar(&brokers, "brokers", "192.168.10.12:9092", "Kafka bootstrap brokers to connect to, as a comma separated list")
	flag.StringVar(&topics, "topics", "nodesstats", "Kafka topics to be consumed, as a comma separated list")
	flag.Parse()
}
func main() {
	udpAddr, _ := net.ResolveUDPAddr("udp4", "0.0.0.0:"+port)

	//监听端口
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		fmt.Println(err)
	}
	defer udpConn.Close()

	fmt.Println("udp listening ... ")

	//udp不需要Accept
	for {
		handleConnection(udpConn)
	}
}

type NodeStat struct {
	NodeName string
	Stats    []ContainerCpuStat
}

// 读取消息
func handleConnection(udpConn *net.UDPConn) {

	// 外部数据库选择Kafka，此处发布节点状态消息
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.NoResponse
	config.Producer.Partitioner = sarama.NewRandomPartitioner
	config.Producer.Return.Successes = true
	config.Producer.Return.Errors = true
	config.Version = sarama.MaxVersion
	producer, err := sarama.NewAsyncProducer([]string{brokers}, config)
	if err != nil {
		panic(err)
	}
	defer producer.AsyncClose()

	// 读取数据
	buf := make([]byte, 10240)
	for {
		len, _, err := udpConn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		var containerCpuStat NodeStat
		err = json.Unmarshal(buf[0:len], &containerCpuStat)
		if err != nil {
			log.L.Error("Received error data: ", string(buf))
		} else {
			log.L.Println(containerCpuStat)
			msg := &sarama.ProducerMessage{
				Topic: topics,
				Key:   sarama.StringEncoder(containerCpuStat.NodeName),
				Value: sarama.ByteEncoder(buf[0:len]),
			}
			producer.Input() <- msg
		}
	}
}
