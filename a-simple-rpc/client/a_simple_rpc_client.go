package main

import (
	"context"
	"flag"
	"github.com/hankeyyh/a-simple-rpc/client"
	"github.com/hankeyyh/a-simple-rpc/protocol"
	"github.com/montanaflynn/stats"
	"github.com/rpcxio/rpcx-benchmark/proto"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

var (
	concurrency = flag.Int("c", 500, "concurrency")
	total       = flag.Int("n", 1000000, "total requests for all clients")
	host        = flag.String("s", "127.0.0.1:8972", "server ip and port")
	pool        = flag.Int("pool", 10, "shared rpcx clients")
	rate        = flag.Int("r", 0, "throughputs")
)

func main() {
	// 并发goroutine数.模拟客户端
	n := *concurrency
	// 每个客户端需要发送的请求数
	m := *total / n
	log.Printf("concurrency: %d\nrequests per client: %d\n\n", n, m)

	// 创建服务端的信息
	log.Printf("Servers: %+v\n\n", *host)

	servicePath := "Hello"
	serviceMethod := "Say"

	// 准备好参数
	args := proto.PrepareArgs()

	// 参数的大小
	b := make([]byte, 1024)
	i, _ := args.MarshalTo(b)
	log.Printf("message size: %d bytes\n\n", i)

	// 等待所有测试完成
	var wg sync.WaitGroup
	wg.Add(n * m)

	// 创建客户端连接池
	var clientIndex uint64
	poolClients := make([]client.XClient, 0, *pool)
	dis, _ := client.NewPeer2PeerDiscovery(*host, "")
	for i := 0; i < *pool; i++ {
		option := client.DefaultOption
		option.SerializeType = protocol.ProtoBuffer
		xclient := client.NewXClient(servicePath, client.FailTry, client.RoundRobin, dis, option)
		defer xclient.Close()

		// warmup
		var reply proto.BenchmarkMessage
		for j := 0; j < 5; j++ {
			xclient.Call(context.Background(), serviceMethod, args, &reply)
		}

		poolClients = append(poolClients, xclient)
	}

	// 栅栏，控制客户端同时开始测试
	var startWg sync.WaitGroup
	startWg.Add(n + 1) // +1 是因为有一个goroutine用来记录开始时间

	// 总请求数
	var trans uint64
	// 返回正常的总请求数
	var transOK uint64

	// 每个goroutine的耗时记录
	d := make([][]int64, n, n)

	// 创建客户端 goroutine 并进行测试
	startTime := time.Now().UnixNano()
	go func() {
		startWg.Done()
		startWg.Wait()
		startTime = time.Now().UnixNano()
	}()
	for i := 0; i < n; i++ {
		dt := make([]int64, 0, m)
		d = append(d, dt)

		go func(i int) {
			var reply proto.BenchmarkMessage

			startWg.Done()
			startWg.Wait()

			for j := 0; j < m; j++ {
				t := time.Now().UnixNano()
				ci := atomic.AddUint64(&clientIndex, 1)
				ci = ci % uint64(*pool)
				xclient := poolClients[int(ci)]

				err := xclient.Call(context.Background(), serviceMethod, args, &reply)
				t = time.Now().UnixNano() - t // 等待时间+服务时间，等待时间是客户端调度的等待时间以及服务端读取请求、调度的时间，服务时间是请求被服务处理的实际时间

				d[i] = append(d[i], t)

				if err == nil && reply.Field1 == "OK" {
					atomic.AddUint64(&transOK, 1)
				}

				atomic.AddUint64(&trans, 1)
				wg.Done()
			}
		}(i)

	}

	// 等待测试完成
	wg.Wait()

	// 统计
	Stats(startTime, *total, d, trans, transOK)
}

// Stats 统计结果.
func Stats(startTime int64, totalRequests int, tookTimes [][]int64, trans, transOK uint64) {
	// 测试总耗时
	totalTInNano := time.Now().UnixNano() - startTime
	totalT := totalTInNano / 1000000
	log.Printf("took %d ms for %d requests", totalT, totalRequests)

	// 汇总每个请求的耗时
	totalD := make([]int64, 0, totalRequests)
	for _, k := range tookTimes {
		totalD = append(totalD, k...)
	}
	// 将int64数组转换成float64数组，以便分析
	totalD2 := make([]float64, 0, totalRequests)
	for _, k := range totalD {
		totalD2 = append(totalD2, float64(k))
	}

	// 计算各个指标
	mean, _ := stats.Mean(totalD2)
	median, _ := stats.Median(totalD2)
	max, _ := stats.Max(totalD2)
	min, _ := stats.Min(totalD2)
	p999, _ := stats.Percentile(totalD2, 99.9)

	// 输出结果
	log.Printf("sent     requests    : %d\n", totalRequests)
	log.Printf("received requests    : %d\n", trans)
	log.Printf("received requests_OK : %d\n", transOK)
	if totalT == 0 {
		log.Printf("throughput  (TPS)    : %d\n", int64(totalRequests)*1000*1000000/totalTInNano)
	} else {
		log.Printf("throughput  (TPS)    : %d\n\n", int64(totalRequests)*1000/totalT)
	}

	log.Printf("mean: %.f ns, median: %.f ns, max: %.f ns, min: %.f ns, p99.9: %.f ns\n", mean, median, max, min, p999)
	log.Printf("mean: %d ms, median: %d ms, max: %d ms, min: %d ms, p99.9: %d ms\n", int64(mean/1000000), int64(median/1000000), int64(max/1000000), int64(min/1000000), int64(p999/1000000))
}
