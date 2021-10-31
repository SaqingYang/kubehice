package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

func main() {
	var threads int
	flag.IntVar(&threads, "t", 1, "threads")
	flag.Parse()
	var fg []chan int
	urls := []string{
		"http://192.168.10.3/index.html",
		"http://192.168.2.10/index.html",
	}
	for i := 0; i < threads; i++ {
		tfg := make(chan int)
		fg = append(fg, tfg)
		go httpReq(urls, fg[i])
	}
	for i := 0; i < threads; i++ {
		val := 0
		val, ok := <-fg[i]
		if ok {
			fmt.Printf("thread %v finished count:%v\n", i, val)
		}
	}
}
func httpReq(urls []string, fg chan int) {
	min := 10240
	max := 0
	start := time.Now()
	for {
		for i := 0; i < len(urls); i++ {
			resp, err := http.Get(urls[i])
			if err != nil {
				fmt.Println("err")
				continue
			}
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			if len(body) < min {
				min = len(body)
			}
			if len(body) > max {
				max = len(body)
			}
		}
		if time.Now().Sub(start).Seconds() > 600 {
			break
		}
	}

	fg <- max - min
}
