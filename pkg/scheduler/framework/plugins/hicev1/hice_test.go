package hicev1

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
	k8sYaml "k8s.io/apimachinery/pkg/util/yaml"
)

func TestGetUnAvailableImages(t *testing.T) {
	yamlPod := `
apiVersion: v1
kind: Pod
metadata:
  name: nginx-hc-pod
  labels: 
    app: nginx-hc
spec:
  schedulerName: kubehice-scheduler
  containers:
  - name: nginx
    image: nginx1:latest
    imagePullPolicy: IfNotPresent
    resources:
      requests:
        cpu: 100m
      limits:
        cpu: 200m
  - name: nginx
    image: local-registry:5000/redis:latest
    imagePullPolicy: IfNotPresent
    resources:
      requests:
        cpu: 100m
      limits:
        cpu: 200m
`
	jsonPod, err := k8sYaml.ToJSON([]byte(yamlPod))
	if err != nil {
		t.Error(err)
	}
	pod := &v1.Pod{}
	err = json.Unmarshal([]byte(jsonPod), pod)
	if err != nil {
		t.Error(err)
	}

	yamlMultiArchImages := `
list:
  - name: local-registry:5000/redis:latest
    images:
    - name: local-registry:5000/redis:latest
      arch: amd64
      os: linux
    - name: local-registry:5000/redis:latest-arm64
      arch: arm64
      os: linux
  - name: local-registry:5000/hice-client:latest
    images:
    - name: local-registry:5000/hice-client:latest
      arch: amd64
      os: linux
    - name: local-registry:5000/hice-client:latest-arm64
      arch: arm64
      os: linux`
	jsonMultiArchImages, err := k8sYaml.ToJSON([]byte(yamlMultiArchImages))
	if err != nil {
		t.Error(err)
	}
	unAvailableImages := GetUnAvailableImages(pod, jsonMultiArchImages)

	testData1 := []byte(`{"images":["nginx1:latest"]}`)
	// var testdata []byte
	if string(UpdateUnAvailableImageData(testData1, unAvailableImages)) != string(testData1) {

		t.Error("update error")
	}

	testData2 := []byte(`{"images":["resis:latest"]}`)
	temp := &UnAvailableImages{}
	json.Unmarshal(UpdateUnAvailableImageData(testData2, unAvailableImages), temp)
	for _, i := range unAvailableImages.Images {
		exist := false
		for _, j := range temp.Images {
			if i == j {
				exist = true
			}
		}
		if !exist {
			// fmt.Println(i, unAvailableImages)
			t.Error("add unavailable images error")
		}
	}

	etcdServer := "192.168.10.2:2379"
	etcdPeerKey := "./peer.key"
	etcdPeerCrt := "./peer.crt"
	etcdCaCrt := "./ca.crt"
	etcdCli, err := etcdClient(etcdServer, etcdPeerKey, etcdPeerCrt, etcdCaCrt)
	if err != nil {
		panic(err)
	}
	defer etcdCli.Close()
	key := "kubehice/images"
	resp, err := etcdCli.Get(context.Background(), key)
	if err != nil {
		panic(err)
	}
	data := resp.Kvs[0].Value
	unAvailableImages = GetUnAvailableImages(pod, data)
	if len(unAvailableImages.Images) != 0 {
		resp, _ = etcdCli.Get(context.Background(), "kubehice/unavailableimages")
		var oldEtcdData []byte
		if len(resp.Kvs) != 0 {
			oldEtcdData = resp.Kvs[0].Value
			t.Log(string(oldEtcdData))
		}
		unAvailableImageData := UpdateUnAvailableImageData(oldEtcdData, unAvailableImages)
		if string(oldEtcdData) == string(unAvailableImageData) {
			t.Log("unavailable image existed!")
		} else {
			t.Log("Update Etcd data", string(oldEtcdData))
			etcdCli.Put(context.Background(), "kubehice/unavailableimages", string(unAvailableImageData))
		}

		fmt.Println("Can't find \"" + unAvailableImages.Images[0] + "\" in etcd!")
	}
	t.Log("测试结束，清除Etcd测试数据")
	t.Log(etcdCli.Delete(context.Background(), "kubehice/unavailableimages"))

}
