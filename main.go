package main

import (
	"context"
	"fmt"
	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"log"
	"time"
)

func main()  {
	client, err := promapi.NewClient(promapi.Config{
		Address:      "http://127.0.0.1:9090/",
	})
	if err != nil {
		log.Fatalln(err)
	}
	promClient := promv1.NewAPI(client)

	val,warn, err := promClient.Query(context.Background(), `node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate`, time.Now())
	if err != nil {
		log.Fatalln(err)
	}
	if warn != nil {
		log.Println("Warning: ", warn)
	}

	fmt.Println("Value type: ", val.Type())
	fmt.Println(val)
}
