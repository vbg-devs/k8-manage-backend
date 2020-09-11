package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/olahol/melody"
	v1 "k8s.io/api/core/v1"
	v1meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	log.Println("Start k8-manage 🔥")

	var kubeconfig *string
	if home := homeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}

	log.Printf("KubeCOnfig %s", *kubeconfig)

	// use the current context in kubeconfig
	var err error
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	deployments := clientset.AppsV1().Deployments("default")

	deployList, err := deployments.List(context.Background(), v1meta.ListOptions{})

	if err != nil {
		log.Fatalf("couldn't get deployments err: %v", err)
	}

	for _, d := range deployList.Items {
		log.Printf("got deployment %s in namespace %s", d.GetName(), d.GetNamespace())
	}

	pods := clientset.CoreV1().Pods("default")

	podList, err := pods.List(context.Background(), v1meta.ListOptions{})

	if err != nil {
		log.Fatalf("couldn't get deployments err: %v", err)
	}

	for _, p := range podList.Items {
		log.Printf("got pods %s in namespace %s", p.GetName(), p.GetNamespace())
		for k, v := range p.GetLabels() {
			log.Printf("pod %s labels %s : %s", p.GetName(), k, v)
		}
	}

	r := gin.Default()
	m := melody.New()

	watchlist := cache.NewListWatchFromClient(clientset.CoreV1().RESTClient(), "pods", v1meta.NamespaceDefault,
		fields.Everything())
	_, controller := cache.NewInformer(
		watchlist,
		&v1.Pod{},
		time.Second*0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				pod, ok := obj.(*v1.Pod)
				if !ok {
					log.Fatalf("list/watch returned non-pod object: %T", pod)
				}
				msg := fmt.Sprintf("pod added: %s", pod.GetName())
				fmt.Println(msg)
				m.Broadcast([]byte(msg))
			},
			DeleteFunc: func(obj interface{}) {
				pod, ok := obj.(*v1.Pod)
				if !ok {
					log.Fatalf("list/watch returned non-pod object: %T", pod)
				}
				msg := fmt.Sprintf("pod deleted: %s", pod.GetName())
				fmt.Println(msg)
				m.Broadcast([]byte(msg))
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				pod, ok := newObj.(*v1.Pod)
				if !ok {
					log.Fatalf("list/watch returned non-pod object: %T", pod)
				}
				msg := fmt.Sprintf("pod changed: %s", pod.GetName())
				fmt.Println(msg)
				m.Broadcast([]byte(msg))
			},
		},
	)

	stop := make(chan struct{})
	go controller.Run(stop)

	r.GET("/", func(c *gin.Context) {
		http.ServeFile(c.Writer, c.Request, "websocket.html")
	})

	r.GET("/pods", func(c *gin.Context) {

		pods := []v1.Pod{}

		for _, p := range podList.Items {
			pods = append(pods, p)
		}
		c.JSON(200, pods)
	})

	r.GET("/ws", func(c *gin.Context) {
		m.HandleRequest(c.Writer, c.Request)
	})

	m.HandleMessage(func(s *melody.Session, msg []byte) {
		m.Broadcast(msg)
	})

	r.Run(":5000")

}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
