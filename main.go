package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

var kubeconfig *string

func init() {
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()
}

func getKubeClient() (kubernetes.Interface, error) {
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

	pods := make([]string, 0)

	for _, pod := range podList.Items {
		if len(pod.OwnerReferences) > 0 {
			owner := pod.OwnerReferences[0]
			if owner.Name == sts.Name {
				pods = append(pods, pod.Name)
			}
		}
	}

	lcp := LCP(pods)
	var podPromRegex string

	if lcp == "" {
		podPromRegex = strings.Join(pods, "|")
	} else {
		podPromRegex = fmt.Sprintf("%s.*", lcp)
	}

	promQuery := fmt.Sprintf(`sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{namespace="%s", pod=~"%s", container!=""})`, ns, podPromRegex)

	val, err := getPromQueryResult(promQuery)
	if err != nil {
		return nil, err
	}
	return val, nil
}

func getPromQueryResult(promQuery string) (*float64, error) {
	client, err := promapi.NewClient(promapi.Config{
		Address: "http://127.0.0.1:9090/",
	})
	if err != nil {
		return nil, err
	}
	promClient := promv1.NewAPI(client)

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

func LCP(strs []string) string {
	minLen := math.MaxInt32
	for _, s := range strs {
		minLen = minINT(len(s), minLen)
	}

	lcpLen := 0

	for i := 0; i < minLen; i++ {
		ok := true
		for j := 1; j < len(strs); j++ {
			if strs[0][i] != strs[j][i] {
				ok = false
				break
			}
		}
		if !ok {
			break
		}
		lcpLen += 1
	}
	if lcpLen == 0 {
		return ""
	}
	return strs[0][:lcpLen]
}

func minINT(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func getPodsMemory(ns string, selector map[string]string) (*float64, error) {
	kc, err := getKubeClient()
	if err != nil {
		return nil, err
	}
	podList, err := kc.CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{
		LabelSelector: labels.Set(selector).String(),
	})
	if err != nil {
		return nil, err
	}

	pods := make([]string, 0)

	for _, pod := range podList.Items {
		pods = append(pods, pod.Name)
	}

	lcp := LCP(pods)
	var podPromRegex string

	if lcp == "" {
		podPromRegex = strings.Join(pods, "|")
	} else {
		podPromRegex = fmt.Sprintf("%s.*", lcp)
	}

	promQuery := fmt.Sprintf(`sum(container_memory_working_set_bytes{namespace="%s", pod=~"%s", container!="", image!=""})`, ns, podPromRegex)

	val, err := getPromQueryResult(promQuery)
	if err != nil {
		return nil, err
	}
	return val, nil
}

func getPodsCPU(ns string, selector map[string]string) (*float64, error) {
	kc, err := getKubeClient()
	if err != nil {
		return nil, err
	}
	podList, err := kc.CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{
		LabelSelector: labels.Set(selector).String(),
	})
	if err != nil {
		return nil, err
	}

	pods := make([]string, 0)

	for _, pod := range podList.Items {
		pods = append(pods, pod.Name)
	}

	lcp := LCP(pods)
	var podPromRegex string

	if lcp == "" {
		podPromRegex = strings.Join(pods, "|")
	} else {
		podPromRegex = fmt.Sprintf("%s.*", lcp)
	}

	promQuery := fmt.Sprintf(`sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{namespace="%s", pod=~"%s", container!=""})`, ns, podPromRegex)

	val, err := getPromQueryResult(promQuery)
	if err != nil {
		return nil, err
	}
	return val, nil
}

func getPodsStorage(ns string, selector map[string]string) (*float64, error) {
	kc, err := getKubeClient()
	if err != nil {
		return nil, err
	}
	podList, err := kc.CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{
		LabelSelector: labels.Set(selector).String(),
	})
	if err != nil {
		return nil, err
	}

	pods := make([]string, 0)

	for _, pod := range podList.Items {
		pods = append(pods, pod.Name)
	}

	lcp := LCP(pods)
	var podPromRegex string

	if lcp == "" {
		podPromRegex = strings.Join(pods, "|")
	} else {
		podPromRegex = fmt.Sprintf("%s.*", lcp)
	}

	promQuery := fmt.Sprintf(`avg(container_blkio_device_usage_total{namespace="%s", pod=~"%s"})`, ns, podPromRegex)

	val, err := getPromQueryResult(promQuery)
	if err != nil {
		return nil, err
	}
	return val, nil
}

func main() {
	val, err := getStatefulSetCPU("demo", "mg-sh-shard0")
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println("CPU Usage in core(StatefulSet): ", *val)

	selector := make(map[string]string)
	selector["app.kubernetes.io/instance"] = "mg-sh"
	selector["app.kubernetes.io/managed-by"] = "kubedb.com"
	selector["app.kubernetes.io/name"] = "mongodbs.kubedb.com"

	val, err = getPodsCPU("demo", selector)
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println("CPU Usage in core(Pods): ", *val)

	memory, err := getPodsMemory("demo", selector)
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println("Memory usage in MB(Pods): ", *memory/1024/1024)

	storage, err := getPodsStorage("demo", selector)
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println("Storage usage in MB(Pods): ", *storage/1024/1024)
}
