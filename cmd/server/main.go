package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	flags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	addrValue := flags.String("addr", defaultAddress, "回环监听地址")
	dataDir := flags.String("data", "./data", "持久化数据目录")
	selfCheck := flags.Bool("self-check", false, "运行真实 HTTP 业务自检后退出")
	_ = flags.Parse(os.Args[1:])
	addrSet := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "addr" {
			addrSet = true
		}
	})
	addr, err := resolveAddress(*addrValue, addrSet)
	if err != nil {
		log.Fatal(err)
	}
	if *selfCheck {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err = runSelfCheck(ctx, addr); err != nil {
			log.Fatal(err)
		}
		fmt.Println("自检通过：缺陷返修、连续复测、独立批准和证书校验链路完整")
		return
	}
	handler, err := buildHandler(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	log.Printf("压力测孔资格工作台监听 http://%s", addr)
	if err = server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
