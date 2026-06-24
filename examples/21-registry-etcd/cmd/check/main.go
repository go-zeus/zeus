// Command check 读取 etcd 中 /zeus/services 前缀下所有注册的实例
//
// 用法：
//
//	go run ./cmd/check
//	go run ./cmd/check -prefix /zeus/services/zeus-etcd-demo
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

func main() {
	endpoint := flag.String("endpoint", "127.0.0.1:2379", "etcd endpoint")
	prefix := flag.String("prefix", "/zeus/services", "key prefix to scan")
	flag.Parse()

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{*endpoint},
		DialTimeout: 30 * time.Second,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "dial:", err)
		os.Exit(1)
	}
	defer cli.Close()

	// 拨号成功后 Status 探活；首次 gRPC 拨号可能较慢，给 30s
	stCtx, stCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer stCancel()
	if st, err := cli.Status(stCtx, *endpoint); err != nil {
		fmt.Fprintln(os.Stderr, "status:", err)
		os.Exit(1)
	} else {
		fmt.Printf("etcd cluster: %s (version %s, leader id %x)\n", *endpoint, st.Version, st.Leader)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := cli.Get(ctx, *prefix, clientv3.WithPrefix())
	if err != nil {
		fmt.Fprintln(os.Stderr, "get:", err)
		os.Exit(1)
	}
	if len(resp.Kvs) == 0 {
		fmt.Println("(no instances registered)")
		return
	}
	fmt.Printf("Found %d instances:\n", len(resp.Kvs))
	for _, kv := range resp.Kvs {
		fmt.Printf("  KEY: %s\n  VAL: %s\n  LEASE: %d\n\n", string(kv.Key), string(kv.Value), kv.Lease)
	}
}
