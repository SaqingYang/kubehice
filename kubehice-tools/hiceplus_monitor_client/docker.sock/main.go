package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"time"

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

func main() {
	var server string
	flag.StringVar(&server, "s", "127.0.0.1:12345", "kubehice's server ip")
	flag.Parse()

	udpAddr, _ := net.ResolveUDPAddr("udp4", server)
	udpConn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	for {
		stats, err := AllContainersCpuStats(cli, ctx)
		if err != nil {
			panic(err)
		}
		fmt.Println(stats)
		jsonStats, err := json.Marshal(stats)
		if err != nil {
			panic(err)
		}
		_, err = udpConn.Write(jsonStats)
		if err != nil {
			time.Sleep(10 * time.Second)
		}
		time.Sleep(1 * time.Second)
	}

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
