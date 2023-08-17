package main

import (
	"context"
	"flag"
	"github.com/hankeyyh/a-simple-rpc/server"
	"github.com/rpcxio/rpcx-benchmark/proto"
	"runtime"
	"time"
)

type Hello int

func (t *Hello) Say(ctx context.Context, args *proto.BenchmarkMessage, reply *proto.BenchmarkMessage) error {
	args.Field1 = "OK"
	args.Field2 = 100
	*reply = *args
	if *delay > 0 {
		time.Sleep(*delay)
	} else {
		runtime.Gosched()
	}
	return nil
}

var (
	host  = flag.String("s", "127.0.0.1:8972", "listened ip and port")
	delay = flag.Duration("delay", 0, "delay to mock business processing by sleep")
)

func main() {
	svr := server.NewServer()
	svr.RegisterName("Hello", new(Hello))
	svr.Serve("tcp", *host)
}
