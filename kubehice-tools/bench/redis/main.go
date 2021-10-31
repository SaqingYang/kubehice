package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
)

var ctx = context.Background()

func main() {
	var threads int
	flag.IntVar(&threads, "t", 1, "threads")
	flag.Parse()
	file, err := os.OpenFile("T-2-VM-1-7-600s.jmx", os.O_RDONLY, 600)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	var data []byte
	byte, err := ioutil.ReadAll(file)
	sdata := string(data)

	// fmt.Println(sdata)

	fmt.Printf("data size:%v\n", len(byte))
	var fg []chan int

	for i := 0; i < threads; i++ {
		tfg := make(chan int)
		fg = append(fg, tfg)
		k := fmt.Sprintf("test%v", i)
		go Test(sdata, k, fg[i])
	}
	for i := 0; i < threads; i++ {
		val := 0
		val, ok := <-fg[i]
		if ok {
			fmt.Printf("thread %v finished count:%v\n", i, val)
		}
	}

}

func ReadAndSet(rdb *redis.Client, key, data string) {
	err := rdb.Set(ctx, key, data, 0).Err()
	if err != nil {
		return
	}
	val, err := rdb.Get(ctx, key).Result()
	if err != nil {
		panic(err)
	}
	_ = val
}
func Test(key, data string, fg chan int) {
	rdb_cloud_worker1 := redis.NewClient(&redis.Options{
		Addr:     "cloud-worker1:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	rdb_cloud_worker2 := redis.NewClient(&redis.Options{
		Addr:     "cloud-worker2:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	rdb_cloud_worker3 := redis.NewClient(&redis.Options{
		Addr:     "cloud-worker3:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	rdb_edge1 := redis.NewClient(&redis.Options{
		Addr:     "192.168.2.4:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	rdb_edge2 := redis.NewClient(&redis.Options{
		Addr:     "192.168.2.5:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	rdb_edge3 := redis.NewClient(&redis.Options{
		Addr:     "192.168.2.6:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	start := time.Now()
	count := 0
	for {
		ReadAndSet(rdb_cloud_worker1, key, data)
		ReadAndSet(rdb_cloud_worker2, key, data)
		ReadAndSet(rdb_cloud_worker3, key, data)
		ReadAndSet(rdb_edge1, key, data)
		ReadAndSet(rdb_edge2, key, data)
		ReadAndSet(rdb_edge3, key, data)
		count++
		if time.Now().Second()-start.Second() > 600 {
			break
		}
	}
	// fmt.Printf("coubt:%v\n", count)
	fg <- count
}
