package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sync"

	"github.com/SkyAPM/go2sky"
	go2skyHTTP "github.com/SkyAPM/go2sky/plugins/http"
	"k8s.io/klog/v2"

	"github.com/tpiperatgod/offf-go/framework"
	"github.com/tpiperatgod/offf-go/plugin"
	"github.com/tpiperatgod/offf-go/plugin/skywalking"
)

var (
	initHttpClientOnce sync.Once
	client             *http.Client
)

func initHTTPClient() {
	initHttpClientOnce.Do(func() {
		client, _ = go2skyHTTP.NewClient(go2sky.GetGlobalTracer())
	})
}

func HelloWorldWithHttp(w http.ResponseWriter, r *http.Request) {
	initHTTPClient()
	// call end service
	request, err := http.NewRequest("GET", fmt.Sprintf("%s/helloserver", os.Getenv("PROVIDER_ADDRESS")), nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		klog.Errorf("unable to create http request: %+v\n", err)
		return
	}

	request = request.WithContext(r.Context())
	res, err := client.Do(request)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(body)
}

func main() {
	ctx := context.Background()
	fwk, err := framework.NewFramework()
	if err != nil {
		klog.Fatal(err)
	}
	fwk.RegisterPlugins(map[string]plugin.Plugin{
		"skywalking": &skywalking.PluginSkywalking{},
	})

	err = fwk.Register(ctx, HelloWorldWithHttp)
	if err != nil {
		klog.Fatal(err)
	}

	err = fwk.Start(ctx)
	if err != nil {
		klog.Fatal(err)
	}
}
