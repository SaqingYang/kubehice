package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"time"

	"go.etcd.io/etcd/client/pkg/v3/transport"
	clientv3 "go.etcd.io/etcd/client/v3"
	k8sYaml "k8s.io/apimachinery/pkg/util/yaml"
)

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

var (
	filePath = ""
	caFile   = ""
	certFile = ""
	key      = ""
	host     = ""
	port     = ""
)

func init() {
	flag.StringVar(&filePath, "file", "images.yaml", "image list yaml file path.")
	flag.StringVar(&caFile, "ca", "ca.crt", "cafile.")
	flag.StringVar(&certFile, "cert", "peer.crt", "cert file")
	flag.StringVar(&key, "key", "peer.key", "key file")
	flag.StringVar(&host, "host", "192.168.10.2", "etcd server host address.")
	flag.StringVar(&port, "port", "2379", "etcd server port.")
	flag.Parse()
}
func main() {
	cli, err := etcdClient(host+":"+port, key, certFile, caFile)
	if err != nil {
		fmt.Println(err)
		return
	}
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		fmt.Println(err)
		return
	}
	data, _ = k8sYaml.ToJSON(data)
	key := "kubehice/images"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	_, err = cli.Put(ctx, key, string(data))
	cancel()
	if err != nil {
		fmt.Println(err)
		return
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	resp, err := cli.Get(ctx, key)
	cancel()
	if err != nil {
		fmt.Println(err)
		return
	}
	imageList := &ImagesList{}
	err = json.Unmarshal(resp.Kvs[0].Value, imageList)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("struct:", imageList)

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
