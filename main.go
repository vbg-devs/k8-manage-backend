package main

import (
	"context"
	"encoding/json"
	"flag"
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

				msgObj := wsMessage{
					Type: "pod-added",
					Data: pod,
				}

				msg, err := json.Marshal(msgObj)

				if err != nil {
					log.Fatalf("pod added json marshal err: %v", err)
				}
				log.Printf("Message sent: %v", msgObj)
				m.Broadcast(msg)
			},
			DeleteFunc: func(obj interface{}) {
				pod, ok := obj.(*v1.Pod)
				if !ok {
					log.Fatalf("list/watch returned non-pod object: %T", pod)
				}

				msgObj := wsMessage{
					Type: "pod-deleted",
					Data: pod,
				}

				msg, err := json.Marshal(msgObj)

				if err != nil {
					log.Fatalf("pod deleted json marshal err: %v", err)
				}
				log.Printf("Message sent: %v", msgObj)
				m.Broadcast(msg)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				newPod, ok := newObj.(*v1.Pod)
				if !ok {
					log.Fatalf("list/watch returned non-pod object: %T", newPod)
				}

				oldPod, ok := oldObj.(*v1.Pod)
				if !ok {
					log.Fatalf("list/watch returned non-pod object: %T", oldPod)
				}

				data := modifiedPod{
					NewPod: newPod,
					OldPod: oldPod,
				}

				msgObj := wsMessage{
					Type: "pod-modified",
					Data: data,
				}

				msg, err := json.Marshal(msgObj)

				if err != nil {
					log.Fatalf("pod added json marshal err: %v", err)
				}
				log.Printf("Message sent: %v", msgObj.Data)
				m.Broadcast(msg)
			},
		},
	)

	stop := make(chan struct{})
	go controller.Run(stop)

	r.GET("/", func(c *gin.Context) {
		http.ServeFile(c.Writer, c.Request, "websocket.html")
	})

	r.GET("/pods", func(c *gin.Context) {
		c.JSON(200, podList.Items)
	})

	r.GET("/ws", func(c *gin.Context) {
		m.HandleRequest(c.Writer, c.Request)
	})

	m.HandleMessage(func(s *melody.Session, msg []byte) {
		m.Broadcast(msg)
	})

	r.Run(":5000")

}

type wsMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type modifiedPod struct {
	NewPod *v1.Pod `json:"new_pod"`
	OldPod *v1.Pod `json:"old_pod"`
}

type wsError struct {
	Error string `json:"error"`
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
