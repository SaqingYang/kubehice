package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"testing"

	k8sYaml "k8s.io/apimachinery/pkg/util/yaml"
)

func TestCheckKeywords(t *testing.T) {
	str := "nginx-arm64:latest"
	if CheckKeywords(str) != "arm64" {
		t.Error(CheckKeywords(str))
	}
	if CheckKeywords("nginx-arm:latest") != "arm" {
		t.Error(CheckKeywords("nginx-arm:latest"))
	}
}

func TestUpdateMultiArchImagesData(t *testing.T) {
	data, err := ioutil.ReadFile("./images.yaml")
	if err != nil {
		fmt.Println(err)
		return
	}
	jsonData, _ := k8sYaml.ToJSON(data)
	images := &ImagesList{}
	images.List = append(images.List, Images{})
	images.List[0].Name = "xx"
	images.List[0].ImageInfs = append(images.List[0].ImageInfs, ImageInf{Name: "xx", Arch: "amd64", Os: "linux"})
	images.List[0].ImageInfs = append(images.List[0].ImageInfs, ImageInf{Name: "xx-arm64", Arch: "arm64", Os: "linux"})

	images.List = append(images.List, Images{})
	images.List[1].Name = "xx2"
	images.List[1].ImageInfs = append(images.List[1].ImageInfs, ImageInf{Name: "xx2", Arch: "amd64", Os: "linux"})
	newData := UpdateMultiArchImagesData(jsonData, images)

	allImages := &ImagesList{}
	json.Unmarshal(newData, allImages)
	for _, image := range allImages.List {
		if image.Name == "xx" || image.Name == "xx2" {
			fmt.Println(image)
		}
	}

}

func TestUpdateUnAvailableImagesData(t *testing.T) {
	oldImages := &UnAvailableImages{Images: []string{"xx1", "xx2"}}
	oldData, err := json.Marshal(oldImages)
	if err != nil {
		t.Error(err)
	}
	newImages := &UnAvailableImages{Images: []string{""}}
	newData := UpdateUnAvailableImagesData(oldData, *newImages)
	fmt.Println(string(newData))
}

func TestInspect(t *testing.T) {
	image := "local-registry:5000/nginx:latest"
	insecureRegistrys := "local-registry:5000"
	fmt.Println(Inspect(image, insecureRegistrys))
}

func TestMain(t *testing.T) {
	images := &UnAvailableImages{Images: []string{"x"}}
	stillUnAvailableImages := &UnAvailableImages{}
	availableImageList := &ImagesList{}
	for _, image := range images.Images {
		if image == "" {
			continue
		}
		multi_archImages, err := Inspect(image, "local-registry")
		if err != nil {
			stillUnAvailableImages.Images = append(stillUnAvailableImages.Images, image)
			// time.Sleep(time.Minute * 10)
			continue
		}
		if len(multi_archImages.ImageInfs) == 0 {
			// 通过关键字查找支持的镜像
			multi_archImages.Name = image
			arch := CheckKeywords(image)
			if arch == "" {
				arch = "amd64"
			}
			multi_archImages.ImageInfs = append(multi_archImages.ImageInfs, ImageInf{Name: image,
				Os:   "linux",
				Arch: arch})
		}
		// 成功处理的加入availableImageList
		availableImageList.List = append(availableImageList.List, *multi_archImages)
	}

	//如果有成功处理的镜像名，更新Etcd中的多架构信息和待处理镜像
	if len(availableImageList.List) != 0 {
		// err = UpdateMultiArchImagesDataInEtcd(etcdCli, availableImageList)
		log.Println("UpdateMultiArchImagesDataInEtcd->", availableImageList)
		log.Println("UpdateUnAvailableImagesDataInEtcd->", stillUnAvailableImages)
	}
}

func TestUnavailable(t *testing.T) {
	etcdServer := "192.168.10.2:2379"
	etcdPeerKey := "./peer.key"
	etcdPeerCrt := "./peer.crt"
	etcdCaCrt := "./ca.crt"
	etcdCli, err := etcdClient(etcdServer, etcdPeerKey, etcdPeerCrt, etcdCaCrt)
	if err != nil {
		panic(err)
	}
	defer etcdCli.Close()
	images := &UnAvailableImages{Images: []string{"centos:latest"}}
	data, err := json.Marshal(images)
	if err != nil {
		t.Error(err)
	}
	etcdCli.Put(context.Background(), "kubehice/unavailableimages", string(data))

}

func TestGetUnavailable(t *testing.T) {
	etcdServer := "192.168.10.2:2379"
	etcdPeerKey := "./peer.key"
	etcdPeerCrt := "./peer.crt"
	etcdCaCrt := "./ca.crt"
	etcdCli, err := etcdClient(etcdServer, etcdPeerKey, etcdPeerCrt, etcdCaCrt)
	if err != nil {
		panic(err)
	}
	defer etcdCli.Close()

	data, _ := etcdCli.Get(context.Background(), "kubehice/unavailableimages")
	fmt.Println(string(data.Kvs[0].Value))
}
func TestGetMultiArchImages(t *testing.T) {
	etcdServer := "192.168.10.2:2379"
	etcdPeerKey := "./peer.key"
	etcdPeerCrt := "./peer.crt"
	etcdCaCrt := "./ca.crt"
	etcdCli, err := etcdClient(etcdServer, etcdPeerKey, etcdPeerCrt, etcdCaCrt)
	if err != nil {
		panic(err)
	}
	defer etcdCli.Close()

	data, _ := etcdCli.Get(context.Background(), "kubehice/images")
	fmt.Println(string(data.Kvs[0].Value))
}
