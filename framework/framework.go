package framework

import (
	"context"
	"errors"
	"net/http"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"k8s.io/klog/v2"

	ofctx "github.com/tpiperatgod/offf-go/context"
	"github.com/tpiperatgod/offf-go/plugin"
	plugin_a "github.com/tpiperatgod/offf-go/plugin/plugin-a"
	"github.com/tpiperatgod/offf-go/runtime"
	"github.com/tpiperatgod/offf-go/runtime/async"
	"github.com/tpiperatgod/offf-go/runtime/knative"
)

type functionsFrameworkImpl struct {
	ofContext   ofctx.Context
	prePlugins  []plugin.Plugin
	postPlugins []plugin.Plugin
	pluginMap   map[string]plugin.Plugin
	runtime     runtime.Interface
}

// Framework is the interface for the function conversion.
type Framework interface {
	Register(ctx context.Context, fn interface{}) error
	RegisterPlugins(customPlugins map[string]plugin.Plugin)
	Start(ctx context.Context) error
}

func NewFramework() (*functionsFrameworkImpl, error) {
	fwk := &functionsFrameworkImpl{}

	// Parse OpenFunction Context
	if err := parseOpenFunctionContext(fwk); err != nil {
		klog.Errorf("failed to get OpenFunction Context: %v\n", err)
		return nil, err
	}

	// Scan the local directory and register the plugins if exist
	// Register the framework default plugins under `plugin` directory
	fwk.pluginMap = map[string]plugin.Plugin{}

	// Create runtime
	if err := createRuntime(fwk); err != nil {
		klog.Errorf("failed to create runtime: %v\n", err)
		return nil, err
	}

	return fwk, nil
}

func (fwk *functionsFrameworkImpl) Register(ctx context.Context, fn interface{}) error {
	if fnHTTP, ok := fn.(func(http.ResponseWriter, *http.Request) error); ok {
		if err := fwk.runtime.RegisterHTTPFunction(fwk.ofContext, fwk.processPreHooks, fwk.processPostHooks, fnHTTP); err != nil {
			klog.Errorf("failed to register function: %v", err)
			return err
		}
	} else if fnOpenFunction, ok := fn.(func(ofctx.Context, []byte) (ofctx.Out, error)); ok {
		if err := fwk.runtime.RegisterOpenFunction(fwk.ofContext, fwk.processPreHooks, fwk.processPostHooks, fnOpenFunction); err != nil {
			klog.Errorf("failed to register function: %v", err)
			return err
		}
	} else if fnCloudEvent, ok := fn.(func(context.Context, cloudevents.Event) error); ok {
		if err := fwk.runtime.RegisterCloudEventFunction(ctx, fwk.ofContext, fwk.processPreHooks, fwk.processPostHooks, fnCloudEvent); err != nil {
			klog.Errorf("failed to register function: %v", err)
			return err
		}
	} else {
		err := errors.New("unrecognized function")
		klog.Errorf("failed to register function: %v", err)
		return err
	}
	return nil
}

func (fwk *functionsFrameworkImpl) processPreHooks() error {
	plugins := fwk.pluginMap
	for _, plg := range fwk.prePlugins {
		klog.Infof("exec pre hooks: %s of version %s", plg.Name(), plg.Version())
		if err := plg.ExecPreHook(fwk.ofContext, plugins); err != nil {
			klog.Warningf("failed to exec pre hooks %s: %s", plg.Name(), err.Error())
		}
	}
	return nil
}

func (fwk *functionsFrameworkImpl) processPostHooks() error {
	plugins := fwk.pluginMap
	for _, plg := range fwk.postPlugins {
		klog.Infof("exec post hooks: %s of version %s", plg.Name(), plg.Version())
		if err := plg.ExecPostHook(fwk.ofContext, plugins); err != nil {
			klog.Warningf("failed to exec post hooks %s: %s", plg.Name(), err.Error())
		}
	}
	return nil
}

func (fwk *functionsFrameworkImpl) Start(ctx context.Context) error {
	err := fwk.runtime.Start(ctx)
	if err != nil {
		klog.Error("failed to start runtime service")
		return err
	}
	return nil
}

func (fwk *functionsFrameworkImpl) RegisterPlugins(customPlugins map[string]plugin.Plugin) {
	// Register default plugins
	fwk.pluginMap = map[string]plugin.Plugin{
		plugin_a.Name: plugin_a.New(),
	}

	// Register custom plugins
	if customPlugins != nil {
		for name, plg := range customPlugins {
			if _, ok := fwk.pluginMap[name]; !ok {
				fwk.pluginMap[name] = plg
			} else {
				// Skip the registration of plugin with name that already exist
				continue
			}
		}
	}

	for _, plgName := range fwk.ofContext.PrePlugins {
		if plg, ok := fwk.pluginMap[plgName]; ok {
			fwk.prePlugins = append(fwk.prePlugins, plg)
		}
	}

	for _, plgName := range fwk.ofContext.PostPlugins {
		if plg, ok := fwk.pluginMap[plgName]; ok {
			fwk.postPlugins = append(fwk.postPlugins, plg)
		}
	}
}

func createRuntime(fwk *functionsFrameworkImpl) error {
	var err error

	rt := fwk.ofContext.Runtime
	port := fwk.ofContext.Port
	pattern := fwk.ofContext.HttpPattern

	switch rt {
	case ofctx.Knative:
		fwk.runtime = knative.NewKnativeRuntime(port, pattern)
		return nil
	case ofctx.Async:
		fwk.runtime, err = async.NewAsyncRuntime(port)
		if err != nil {
			return err
		}
	}

	if fwk.runtime == nil {
		errMsg := "runtime is nil"
		klog.Error(errMsg)
		return errors.New(errMsg)
	}

	return nil
}

func parseOpenFunctionContext(fwk *functionsFrameworkImpl) error {
	c, err := ofctx.GetOpenFunctionContext()
	if err != nil {
		return err
	}
	fwk.ofContext = *c
	return nil
}
