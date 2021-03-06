package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"log"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"go.etcd.io/etcd/client/pkg/v3/transport"
	clientv3 "go.etcd.io/etcd/client/v3"
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

type UnAvailableImages struct {
	Images []string `json:"images,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=images"`
}

func main() {
	etcdServer := "192.168.10.2:2379"
	etcdPeerKey := "./peer.key"
	etcdPeerCrt := "./peer.crt"
	etcdCaCrt := "./ca.crt"
	etcdCli, err := etcdClient(etcdServer, etcdPeerKey, etcdPeerCrt, etcdCaCrt)
	if err != nil {
		panic(err)
	}
	defer etcdCli.Close()
	for {
		resp, err := etcdCli.Get(context.Background(), "kubehice/unavailableimages")
		if err != nil {
			time.Sleep(time.Minute * 1)
			log.Println(err)
			continue
		}

		if len(resp.Kvs) == 0 {
			log.Println("No unavailable images!")
			time.Sleep(time.Minute * 1)
			continue
		}
		images := &UnAvailableImages{}
		err = json.Unmarshal(resp.Kvs[0].Value, images)
		if err != nil {
			log.Println(err)
			time.Sleep(time.Minute * 1)
			continue
		}
		if len(images.Images) == 0 {
			time.Sleep(time.Minute * 1)
			continue
		}
		stillUnAvailableImages := &UnAvailableImages{}
		availableImageList := &ImagesList{}
		for _, image := range images.Images {
			if image == "" {
				continue
			}
			multi_archImages, err := Inspect(image, "local-registry")
			if err != nil {
				stillUnAvailableImages.Images = append(stillUnAvailableImages.Images, image)
				log.Println(err)
				continue
			}
			if len(multi_archImages.ImageInfs) == 0 {
				// ????????????????????????????????????
				multi_archImages.Name = image
				arch := CheckKeywords(image)
				if arch == "" {
					arch = "amd64"
				}
				multi_archImages.ImageInfs = append(multi_archImages.ImageInfs, ImageInf{Name: image,
					Os:   "linux",
					Arch: arch})
			}
			// ?????????????????????availableImageList
			availableImageList.List = append(availableImageList.List, *multi_archImages)
		}

		//??????????????????????????????????????????Etcd???????????????????????????????????????
		if len(availableImageList.List) != 0 {
			err = UpdateMultiArchImagesDataInEtcd(etcdCli, availableImageList)
			if err != nil {
				log.Print(err)
			}
			UpdateUnAvailableImagesDataInEtcd(etcdCli, *stillUnAvailableImages)
		}

	}
}

// UpdateUnavailableImagesInEtcd ??????Etcd??????"kubehice/unavailableimages"??????
func UpdateUnAvailableImagesDataInEtcd(cli *clientv3.Client, images UnAvailableImages) error {
	// old, _ := cli.Get(context.Background(), "kubehice/unavailableimages")
	newData, _ := json.Marshal(images)
	_, err := cli.Put(context.Background(), "kubehice/unavailableimages", string(newData))
	return err
}

// ??????unavailable imaes???Etcd???????????????????????????????????????????????????
func UpdateUnAvailableImagesData(old []byte, images UnAvailableImages) []byte {
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
				if image == "" {
					continue
				}
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

// UpdateMultiArchImages?????????Etcd???????????????????????????????????????
func UpdateMultiArchImagesData(old []byte, images *ImagesList) []byte {
	oldImageList := &ImagesList{}
	json.Unmarshal(old, oldImageList)
	oldImageList.List = append(oldImageList.List, images.List...)
	new, _ := json.Marshal(oldImageList)
	return new
}

// ??????Etcd????????????????????????
func UpdateMultiArchImagesDataInEtcd(cli *clientv3.Client, images *ImagesList) error {
	old, err := cli.Get(context.Background(), "kubehice/images")
	if err != nil {
		return err
	}
	new := UpdateMultiArchImagesData(old.Kvs[0].Value, images)
	_, err = cli.Put(context.Background(), "kubehice/images", string(new))
	return err

}

// ????????????????????????????????????
func CheckKeywords(image string) string {
	reg := regexp.MustCompile("386|amd64|arm64|arm|ppc64le|s390x|mips64le|riscv64")
	arch := reg.FindString(image)
	// arm?????????????????????????????????armv5/v6/v7/v8
	if arch == "arm" {
		// ?????????????????????arm???armv8/arm64
		reg := regexp.MustCompile("arm64")
		arch = reg.FindString(image)
		if arch == "" {
			arch = "arm"
		}
	}
	return arch
}

// Inspect ????????????image??????????????????????????????????????????????????????Windows?????????
func Inspect(image string, insecureRegistrys string) (*Images, error) {
	if image == "" {
		return nil, nil
	}
	cmd := "docker manifest inspect " + image
	if insecureRegistrys != "" && strings.Contains(image, insecureRegistrys) {
		cmd = cmd + " --insecure"
	}
	c := exec.Command("bash", "-c", cmd)
	output, err := c.CombinedOutput()
	if err != nil {
		log.Println(err)
		return nil, err
	}
	images := &Images{}
	if strings.Contains(string(output), "manifests") {
		reg1 := regexp.MustCompile(`"architecture": ".*"`)

		results := reg1.FindAllStringSubmatch(string(output), -1)
		existed := make(map[string]string)
		for _, result := range results {
			arch := strings.Split(strings.Split(result[0], " ")[1], `"`)[1]
			_, ok := existed[arch]
			if ok {
				continue
			} else {
				existed[arch] = "arch"
			}
			images.Name = image
			images.ImageInfs = append(images.ImageInfs, ImageInf{Name: image,
				Os:   "linux", // ?????????Linux????????????
				Arch: arch})
		}
	}
	return images, nil
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
