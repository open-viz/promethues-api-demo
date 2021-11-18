package main

import (
	"context"
	"flag"
	"fmt"
	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func getKubeClient() (kubernetes.Interface,error) {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		// fmt.Println(home)
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	//fmt.Println(*kubeconfig)
	flag.Parse()
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		return nil, err
	}
	clientset := kubernetes.NewForConfigOrDie(config)
	return clientset, nil
}

func getStatefulSetCPU(ns, name string) (*float64, error) {
	kc, err := getKubeClient()
	if err != nil {
		return nil, err
	}
	sts, err := kc.AppsV1().StatefulSets(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	podList, err := kc.CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{
		LabelSelector: labels.Set(sts.Spec.Selector.MatchLabels).String(),
	})
	if err != nil {
		return nil, err
	}

	pods := make([]string,0)

	for _, pod := range podList.Items {
		if len(pod.OwnerReferences) > 0 {
			owner := pod.OwnerReferences[0]
			if owner.Name == sts.Name {
				pods = append(pods, pod.Name)
			}
		}
	}
	podQuery := strings.Join(pods, "|")
	val, err := getPromQueryResult(ns, podQuery)
	if err != nil {
		return nil, err
	}
	return val, nil
}

func getPromQueryResult(ns, podQuery string) (*float64,error) {
	client, err := promapi.NewClient(promapi.Config{
		Address: "http://127.0.0.1:9090/",
	})
	if err != nil {
		return nil, err
	}
	promClient := promv1.NewAPI(client)

	promQuery := fmt.Sprintf(`node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{namespace="%s", pod=~"%s"}`,ns, podQuery)
	fmt.Println(promQuery)
	val, warn, err := promClient.Query(context.Background(), promQuery, time.Now())
	if err != nil {
		return nil, err
	}
	if warn != nil {
		log.Println("Warning: ", warn)
	}

	metrics := strings.Split(val.String(), "\n")

	cpu := float64(0)

	for _, m := range metrics {
		val := strings.Split(m, "=>")
		if len(val) != 2 {
			return nil, fmt.Errorf("metrics %s is invalid", m)
		}
		valStr := strings.Split(val[1], "@")
		if len(valStr) != 2 {
			return nil, fmt.Errorf("metrics %s is invalid", m)
		}
		valStr[0] = strings.Replace(valStr[0], " ", "", -1)
		metricVal, err := strconv.ParseFloat(valStr[0], 64)
		if err != nil {
			return nil, err
		}
		cpu += metricVal
	}
	return &cpu, nil
}

func main()  {
	val, err := getStatefulSetCPU("demo", "pg-demo")
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Println("CPU Usage: ", *val)
}



