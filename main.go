package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/olahol/melody"
	cors "github.com/rs/cors/wrapper/gin"
	v1apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	v1meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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

	r := gin.Default()
	r.Use(cors.AllowAll())

	m := melody.New()

	createWatcherFor("pods", m, &v1.Pod{}, clientset.CoreV1().RESTClient())
	createWatcherFor("deployments", m, &v1apps.Deployment{}, clientset.AppsV1().RESTClient())
	createWatcherFor("services", m, &v1.Service{}, clientset.CoreV1().RESTClient())
	createWatcherFor("configmaps", m, &v1.ConfigMap{}, clientset.CoreV1().RESTClient())
	createWatcherFor("secrets", m, &v1.Secret{}, clientset.CoreV1().RESTClient())
	createWatcherFor("replicasets", m, &v1apps.ReplicaSet{}, clientset.AppsV1().RESTClient())
	//createWatcherFor("ds", m, &v1apps.DaemonSet{}, clientset.AppsV1().RESTClient())

	pods := clientset.CoreV1().Pods("default")
	deployments := clientset.AppsV1().Deployments("default")

	r.GET("/", func(c *gin.Context) {
		http.ServeFile(c.Writer, c.Request, "websocket.html")
	})

	r.GET("/secrets", func(c *gin.Context) {
		secrets := clientset.CoreV1().Secrets("default")

		secretList, err := secrets.List(context.Background(), v1meta.ListOptions{})

		if err != nil {
			log.Fatalf("couldn't get rs err: %v", err)
		}
		c.JSON(200, secretList.Items)
	})

	r.GET("/configmaps", func(c *gin.Context) {
		configMaps := clientset.CoreV1().ConfigMaps("default")

		configMapList, err := configMaps.List(context.Background(), v1meta.ListOptions{})

		if err != nil {
			log.Fatalf("couldn't get rs err: %v", err)
		}
		c.JSON(200, configMapList.Items)
	})

	r.GET("/services", func(c *gin.Context) {
		services := clientset.CoreV1().Services("default")

		serviceList, err := services.List(context.Background(), v1meta.ListOptions{})

		if err != nil {
			log.Fatalf("couldn't get rs err: %v", err)
		}
		c.JSON(200, serviceList.Items)
	})

	r.GET("/replicasets", func(c *gin.Context) {
		rs := clientset.AppsV1().ReplicaSets("default")

		rsList, err := rs.List(context.Background(), v1meta.ListOptions{})

		if err != nil {
			log.Fatalf("couldn't get rs err: %v", err)
		}
		c.JSON(200, rsList.Items)
	})

	r.GET("/deamonsets", func(c *gin.Context) {
		deamonSets := clientset.AppsV1().DaemonSets("default")

		dsList, err := deamonSets.List(context.Background(), v1meta.ListOptions{})

		if err != nil {
			log.Fatalf("couldn't get rs err: %v", err)
		}
		c.JSON(200, dsList.Items)
	})

	r.GET("/deployments", func(c *gin.Context) {

		deployList, err := deployments.List(context.Background(), v1meta.ListOptions{})

		if err != nil {
			log.Fatalf("couldn't get deployments err: %v", err)
		}
		c.JSON(200, deployList.Items)
	})

	type DeploymentScaleRequest struct {
		Replicas int32 `json:"replicas"`
	}

	r.POST("/deployment/scale/:name", func(c *gin.Context) {
		name := c.Param("name")

		req := DeploymentScaleRequest{}

		err := c.BindJSON(&req)
		if err != nil {
			log.Fatalf("couldn't parse json err: %v", err)
		}

		log.Printf("Hmm %s", name)

		currentScale, err := deployments.GetScale(context.Background(), name, v1meta.GetOptions{})
		if err != nil {
			log.Fatalf("couldn't get  scale err: %v scale:%v", err, req)
		}

		currentScale.Spec.Replicas = req.Replicas

		scale, err := deployments.UpdateScale(context.Background(), name, currentScale,
			v1meta.UpdateOptions{})

		if err != nil {
			log.Fatalf("couldn't update deployment scale err: %v scale:%v", err, req)
		}
		c.JSON(200, scale)
	})

	r.GET("/pods", func(c *gin.Context) {

		podList, err := pods.List(context.Background(), v1meta.ListOptions{})

		if err != nil {
			log.Fatalf("couldn't get deployments err: %v", err)
		}
		c.JSON(200, podList.Items)
	})

	r.DELETE("/pod/:name", func(c *gin.Context) {
		name := c.Param("name")
		err := pods.Delete(context.Background(), name, v1meta.DeleteOptions{})
		if err != nil {
			c.AbortWithError(500, err)
			return
		}
		c.Status(200)
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

func createWatcherFor(resource string, m *melody.Melody, obj runtime.Object, rest rest.Interface) {
	watchlist := cache.NewListWatchFromClient(rest, resource, v1meta.NamespaceDefault,
		fields.Everything())
	_, controller := cache.NewInformer(
		watchlist,
		obj,
		time.Second*0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				msgObj := wsMessage{
					Type: fmt.Sprintf("%s-added", resource),
					Data: obj,
				}

				msg, err := json.Marshal(msgObj)

				if err != nil {
					log.Fatalf("pod added json marshal err: %v", err)
				}
				log.Printf("Message sent: %v", msgObj)
				m.Broadcast(msg)
			},
			DeleteFunc: func(obj interface{}) {
				msgObj := wsMessage{
					Type: fmt.Sprintf("%s-deleted", resource),
					Data: obj,
				}

				msg, err := json.Marshal(msgObj)

				if err != nil {
					log.Fatalf("pod deleted json marshal err: %v", err)
				}
				log.Printf("Message sent: %v", msgObj)
				m.Broadcast(msg)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {

				data := struct {
					New interface{}
					Old interface{}
				}{
					New: newObj,
					Old: oldObj,
				}

				msgObj := wsMessage{
					Type: fmt.Sprintf("%s-modified", resource),
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
}
