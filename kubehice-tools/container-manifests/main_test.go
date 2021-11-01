package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
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

}
