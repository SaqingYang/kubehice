package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"net/http"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
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
type KubeletContainerCpuUsageSecondsTotal struct {
	Inf   ContainerInf
	usage float64
	total int64
}

func main() {
	// if err != nil {
	// 	fmt.Println("Loadx509keypair err:", err)
	// 	return
	// }

	client := &http.Client{}

	var ccust KubeletContainerCpuUsageSecondsTotal
	var ccustMap map[string]KubeletContainerCpuUsageSecondsTotal
	ccustMap = make(map[string]KubeletContainerCpuUsageSecondsTotal)
	lastCcustMap := make(map[string]KubeletContainerCpuUsageSecondsTotal)
	var lastTime int64
	lastTime = -1
	startTime := time.Now()
	fmt.Println(startTime)
	for {
		resp, err := client.Get("http://localhost:10350/metrics/cadvisor")
		if err != nil {
			fmt.Println("Get error:", err)
			return
		}
		body, err := ioutil.ReadAll(resp.Body)
		strs := strings.Split(string(body), "\n")
		defer resp.Body.Close()
		for _, line := range strs {
			if strings.Contains(line, "container_cpu_usage_seconds_total{container=\"") {
				if strings.Contains(line, "container_cpu_usage_seconds_total{container=\"\"") {
					continue
				}
				r := strings.NewReader(strings.Replace(strings.Replace(line, ",", " ", -1), "\"", " ", -1))
				fmt.Fscanf(r, "container_cpu_usage_seconds_total{container=%s cpu=%v id= %v image= %v name= %v namespace= %v pod= %v }%f%d",
					&ccust.Inf.Container,
					&ccust.Inf.Cpu,
					&ccust.Inf.Id,
					&ccust.Inf.Image,
					&ccust.Inf.Name,
					&ccust.Inf.Namespace,
					&ccust.Inf.Pod,
					&ccust.usage,
					&ccust.total)
				if ccust.Inf.Container == "" {
					continue
				}
				ccustMap[ccust.Inf.Id] = ccust
			}
		}
		currentTime := int64(time.Now().Sub(startTime).Seconds())
		fmt.Println(currentTime)
		if lastTime == -1 {
			fmt.Println(lastTime)
			lastTime = currentTime
			for id, item := range ccustMap {
				lastCcustMap[id] = item
			}
		} else {
			// deltaTime := currentTime - lastTime
			lastTime = currentTime
			for id, item := range ccustMap {
				if item.total-lastCcustMap[id].total == 0 {
					continue
				}
				usage := (item.usage - lastCcustMap[id].usage) / float64((item.total-lastCcustMap[id].total)/1000) * 100
				if usage > 1 {
					fmt.Printf("%v %v cpu usage:%f\n", item.Inf.Container, id, usage)
				}

			}

			for id, item := range ccustMap {
				lastCcustMap[id] = item
			}
		}
		time.Sleep(time.Second * 10)
	}

	// fmt.Println(string(body))

}
func AllContainersCpuStats(cli *client.Client, ctx context.Context) ([]ContainerCpuStat, error) {
	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{All: false})
	if err != nil {
		return nil, err
	}
	var containersCpuStat []ContainerCpuStat
	for _, container := range containers {
		usage, err := ContainerCpuStats(cli, ctx, container.ID)
		if err != nil {
			return containersCpuStat, err
		}
		containersCpuStat = append(containersCpuStat, ContainerCpuStat{Id: container.ID, Usage: usage})
	}
	return containersCpuStat, nil
}
func ContainerCpuStats(cli *client.Client, ctx context.Context, id string) (int, error) {
	stat, err := cli.ContainerStats(ctx, id, false)
	if err != nil {
		panic(err)
	}
	var cstat containerStats
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(stat.Body)
	if err != nil {
		return 0, err
	}

	err = json.Unmarshal(buf.Bytes(), &cstat)
	if err != nil {
		return 0, err
	}
	totalUse := cstat.Cpu_stats.Cpu_usage.Total_usage - cstat.Precpu_stats.Cpu_usage.Total_usage
	sysUse := cstat.Cpu_stats.System_cpu_usage - cstat.Precpu_stats.System_cpu_usage
	return cstat.Cpu_stats.Online_cpus * 100 * totalUse / int(sysUse), nil
}

func Test(cli *client.Client, ctx context.Context, id string) {
	stat, err := cli.ContainerStats(ctx, id, true)
	if err != nil {
		panic(err)
	}
	// var cstat containerStats
	// buf := new(bytes.Buffer)
	// buf.Grow(10240)

	// fmt.Print("finish1")
	var p []byte
	p = make([]byte, 1024000)
	for {
		lt := time.Now()
		n, err := stat.Body.Read(p)
		fmt.Println(n)
		fmt.Println(time.Now().Sub(lt).Milliseconds(), "ms")
		// _, err = buf.ReadFrom(stat.Body.Read())
		// fmt.Print("finish2")
		// fmt.Print(buf.String())
		if err != nil {
			panic(err)
		}
	}

}
