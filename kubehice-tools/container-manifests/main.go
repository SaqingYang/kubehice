package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

func main() {
	c := exec.Command("bash", "-c", "docker manifest inspect local-registry:5000/nginx:latest --insecure")
	output, err := c.CombinedOutput()
	if err != nil {
		panic(err)
	}
	var arches []string
	if strings.Contains(string(output), "manifests") {
		reg1 := regexp.MustCompile(`"architecture": ".*"`)

		results := reg1.FindAllStringSubmatch(string(output), -1)

		for _, result := range results {
			arch := strings.Split(strings.Split(result[0], " ")[1], `"`)[1]

			arches = append(arches, arch)
		}
	}
	fmt.Println(arches)
}
