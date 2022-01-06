package plugin_a

import (
	"context"
	"fmt"

	"github.com/fatih/structs"
	"k8s.io/klog/v2"

	ofctx "github.com/tpiperatgod/offf-go/context"
	"github.com/tpiperatgod/offf-go/plugin"
)

const (
	Name    = "plugin-a"
	Version = "v1"
)

type PluginA struct {
	PluginName    string
	PluginVersion string
	stateA        int64
	stateB        context.Context
}

var _ plugin.Plugin = &PluginA{}

func New() *PluginA {
	return &PluginA{}
}

func (p *PluginA) Name() string {
	return Name
}

func (p *PluginA) Version() string {
	return Version
}

func (p *PluginA) ExecPreHook(ctx ofctx.Context, plugins map[string]plugin.Plugin) error {
	r := preHookLogic(ctx.Ctx)
	p.stateA = 1
	p.stateB = r
	return nil
}

func (p *PluginA) ExecPostHook(ctx ofctx.Context, plugins map[string]plugin.Plugin) error {
	// Get data from another plugin via Plugin.Get()
	plgName := "plugin-b"
	keyName := "StateC"
	plg, ok := plugins[plgName]
	if ok && plg != nil {
		v, exist := plg.Get(keyName)
		if exist {
			stateC := v.(int64)
			postHookLogic(p.stateA, stateC)
			return nil
		}
	}
	return fmt.Errorf("failed to get %s from plugin %s", keyName, plgName)
}

func (p *PluginA) Get(fieldName string) (interface{}, bool) {
	plgMap := structs.Map(p)
	value, ok := plgMap[fieldName]
	return value, ok
}

func preHookLogic(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	} else {
		return context.Background()
	}
}

func postHookLogic(numA int64, numB int64) int64 {
	sum := numA + numB
	klog.Infof("the sum is: %d", sum)
	return sum
}
