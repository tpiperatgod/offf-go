package main

import (
	"context"
	"log"

	"k8s.io/klog/v2"

	ofctx "github.com/tpiperatgod/offf-go/context"

	"github.com/tpiperatgod/offf-go/framework"
	"github.com/tpiperatgod/offf-go/plugin"
	"github.com/tpiperatgod/offf-go/plugin/skywalking"
)

func bindingsFunction(ctx ofctx.Context, in []byte) (ofctx.Out, error) {
	if in != nil {
		log.Printf("binding - Data: %s", in)
	} else {
		log.Print("binding - Data: Received")
	}
	return ctx.ReturnOnSuccess().WithData([]byte("hello there")), nil
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

	err = fwk.Register(ctx, bindingsFunction)
	if err != nil {
		klog.Fatal(err)
	}

	err = fwk.Start(ctx)
	if err != nil {
		klog.Fatal(err)
	}
}
