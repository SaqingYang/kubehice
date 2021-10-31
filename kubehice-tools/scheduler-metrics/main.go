package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"net/http"
)

var (
	url = ""
)

func init() {
	flag.StringVar(&url, "url", "", "kule-scheduler cadvisor rest url")
	flag.Parse()
	if url == "" {
		url = "http://localhost:10251/metrics"
	}
}

func main() {
	client := &http.Client{}
	for {
		resp, err := client.Get(url)
		if err != nil {
			fmt.Println("Get error:", err)
			return
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("Read error:", err)
			return
		}
		strs := strings.Split(string(body), "\n")
		defer resp.Body.Close()
		for _, line := range strs {
			if strings.Contains(line, "scheduler_binding_duration_seconds_") {
				fmt.Println(line)
				continue
			}
			if strings.Contains(line, "scheduler_e2e_scheduling_duration_seconds_") {
				fmt.Println(line)
				continue
			}
			if strings.Contains(line, "scheduler_pod_scheduling_duration_seconds_") {
				fmt.Println(line)
				continue
			}
		}
		time.Sleep(10 * time.Second)
	}

	// fmt.Println(string(body))

}
