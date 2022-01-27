package runtime

import (
	"context"
	"encoding/json"
	"net/http"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"k8s.io/klog/v2"

	ofctx "github.com/tpiperatgod/offf-go/context"
	"github.com/tpiperatgod/offf-go/plugin"
)

type Interface interface {
	Start(ctx context.Context) error
	RegisterHTTPFunction(
		ctx ofctx.RuntimeContext,
		prePlugins []plugin.Plugin,
		postPlugins []plugin.Plugin,
		fn func(http.ResponseWriter, *http.Request) error,
	) error
	RegisterOpenFunction(
		ctx ofctx.RuntimeContext,
		prePlugins []plugin.Plugin,
		postPlugins []plugin.Plugin,
		fn func(ofctx.UserContext, []byte) (ofctx.FunctionOut, error),
	) error
	RegisterCloudEventFunction(
		ctx context.Context,
		funcContex ofctx.RuntimeContext,
		prePlugins []plugin.Plugin,
		postPlugins []plugin.Plugin,
		fn func(context.Context, cloudevents.Event) error,
	) error
	Name() ofctx.Runtime
	GetHandler() interface{}
}

type RuntimeManager struct {
	FuncContext ofctx.RuntimeContext
	FuncOut     ofctx.FunctionOut
	prePlugins  []plugin.Plugin
	postPlugins []plugin.Plugin
	pluginState map[string]plugin.Plugin
}

func NewRuntimeManager(funcContext ofctx.RuntimeContext, prePlugin []plugin.Plugin, postPlugin []plugin.Plugin) *RuntimeManager {
	ctx := funcContext
	rm := &RuntimeManager{
		FuncContext: ctx,
		prePlugins:  prePlugin,
		postPlugins: postPlugin,
	}
	if ctx.GetOut() != nil {
		rm.FuncOut = ctx.GetOut()
	} else {
		rm.FuncOut = ofctx.NewFunctionOut()
	}
	rm.init()
	return rm
}

func (rm *RuntimeManager) init() {
	rm.pluginState = map[string]plugin.Plugin{}

	var newPrePlugins []plugin.Plugin
	for _, plg := range rm.prePlugins {
		if existPlg, ok := rm.pluginState[plg.Name()]; !ok {
			p := plg.Init()
			rm.pluginState[plg.Name()] = p
			newPrePlugins = append(newPrePlugins, p)
		} else {
			newPrePlugins = append(newPrePlugins, existPlg)
		}
	}
	rm.prePlugins = newPrePlugins

	var newPostPlugins []plugin.Plugin
	for _, plg := range rm.postPlugins {
		if existPlg, ok := rm.pluginState[plg.Name()]; !ok {
			p := plg.Init()
			rm.pluginState[plg.Name()] = p
			newPostPlugins = append(newPostPlugins, p)
		} else {
			newPostPlugins = append(newPostPlugins, existPlg)
		}
	}
	rm.postPlugins = newPostPlugins
}

func (rm *RuntimeManager) ProcessPreHooks() {
	for _, plg := range rm.prePlugins {
		if err := plg.ExecPreHook(rm.FuncContext, rm.pluginState); err != nil {
			klog.Warningf("plugin %s failed in pre phase: %s", plg.Name(), err.Error())
		}
	}
}

func (rm *RuntimeManager) ProcessPostHooks() {
	for _, plg := range rm.postPlugins {
		if err := plg.ExecPostHook(rm.FuncContext, rm.pluginState); err != nil {
			klog.Warningf("plugin %s failed in post phase: %s", plg.Name(), err.Error())
		}
	}
}

func (rm *RuntimeManager) FunctionRunWrapperWithHooks(fn interface{}) {
	functionContext := rm.FuncContext.GetContext()

	rm.ProcessPreHooks()

	if function, ok := fn.(func(http.ResponseWriter, *http.Request) error); ok {
		srMeta := rm.FuncContext.GetSyncRequestMeta()
		rm.FuncContext.WithError(function(srMeta.ResponseWriter, srMeta.Request))
	} else if function, ok := fn.(func(ofctx.UserContext, []byte) (ofctx.FunctionOut, error)); ok {
		if rm.FuncContext.GetBindingEventMeta() != nil {
			out, err := function(functionContext, rm.FuncContext.GetBindingEventMeta().Data)
			rm.FuncContext.WithOut(out.GetOut())
			rm.FuncContext.WithError(err)
		} else if rm.FuncContext.GetTopicEventMeta() != nil {
			out, err := function(functionContext, convertTopicEventToByte(rm.FuncContext.GetTopicEventMeta().Data))
			rm.FuncContext.WithOut(out.GetOut())
			rm.FuncContext.WithError(err)
		} else {
			out, err := function(functionContext, nil)
			rm.FuncContext.WithOut(out.GetOut())
			rm.FuncContext.WithError(err)
		}
	} else if function, ok := fn.(func(context.Context, cloudevents.Event) error); ok {
		ce := cloudevents.Event{}
		if rm.FuncContext.GetCloudEventMeta() != nil {
			ce = *rm.FuncContext.GetCloudEventMeta()
		}
		rm.FuncContext.WithError(function(rm.FuncContext.GetNativeContext(), ce))
	}

	rm.ProcessPostHooks()
}

func convertTopicEventToByte(data interface{}) []byte {
	if d, ok := data.([]byte); ok {
		return d
	}
	if d, err := json.Marshal(data); err != nil {
		return nil
	} else {
		return d
	}
}
