package context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"sync"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	dapr "github.com/dapr/go-sdk/client"
	"github.com/dapr/go-sdk/service/common"
	"k8s.io/klog/v2"
)

var (
	clientGRPCPort string
)

const (
	functionContextEnvName                    = "FUNC_CONTEXT"
	PodNameEnvName                            = "POD_NAME"
	PodNamespaceEnvName                       = "POD_NAMESPACE"
	ModeEnvName                               = "CONTEXT_MODE"
	Async                        Runtime      = "Async"
	Knative                      Runtime      = "Knative"
	OpenFuncBinding              ResourceType = "bindings"
	OpenFuncTopic                ResourceType = "pubsub"
	Success                                   = 200
	InternalError                             = 500
	defaultPort                               = "8080"
	daprSidecarGRPCPort                       = "50001"
	TracingProviderSkywalking                 = "skywalking"
	TracingProviderOpentelemetry              = "opentelemetry"
	KubernetesMode                            = "kubernetes"
	SelfHostMode                              = "self-host"
)

type Runtime string
type ResourceType string

type RuntimeContext interface {
	GetContext() *Context
	GetNativeContext() context.Context
	GetOut() *Out
	HasInputs() bool
	HasOutputs() bool
	ReturnOnSuccess() Out
	ReturnOnInternalError() Out
	InitDaprClientIfNil()
	DestroyDaprClient()
	GetPrePlugins() []string
	GetPostPlugins() []string
	GetRuntime() Runtime
	GetPort() string
	GetError() error
	GetHttpPattern() string
	SetSyncRequestMeta(w http.ResponseWriter, r *http.Request)
	SetEventMeta(inputName string, event interface{})
	GetInputs() map[string]*Input
	GetOutputs() map[string]*Output
	GetSyncRequestMeta() *SyncRequestMetadata
	GetBindingEventMeta() *common.BindingEvent
	GetTopicEventMeta() *common.TopicEvent
	GetCloudEventMeta() *cloudevents.Event
	WithOut(out *Out) RuntimeContext
	WithError(err error) RuntimeContext
	GetPodName() string
	GetPodNamespace() string
	GetPluginsTracingCfg() TracingConfig
}

type UserContext interface {
	Send(outputName string, data []byte) ([]byte, error)
}

type FunctionOut interface {
	GetOut() *Out
	GetCode() int
	GetData() []byte
	GetMetadata() map[string]string
	WithCode(code int) *Out
	WithData(data []byte) *Out
}

type TracingConfig interface {
	IsEnabled() bool
	ProviderName() string
	ProviderOapServer() string
	GetTags() map[string]string
	GetBaggage() map[string]string
}

type Context struct {
	mu              sync.Mutex
	Name            string               `json:"name"`
	Version         string               `json:"version"`
	RequestID       string               `json:"requestID,omitempty"`
	Ctx             context.Context      `json:"ctx,omitempty"`
	Inputs          map[string]*Input    `json:"inputs,omitempty"`
	Outputs         map[string]*Output   `json:"outputs,omitempty"`
	Runtime         Runtime              `json:"runtime"`
	Port            string               `json:"port,omitempty"`
	State           interface{}          `json:"state,omitempty"`
	EventMeta       *EventMetadata       `json:"event,omitempty"`
	SyncRequestMeta *SyncRequestMetadata `json:"syncRequest,omitempty"`
	PrePlugins      []string             `json:"prePlugins,omitempty"`
	PostPlugins     []string             `json:"postPlugins,omitempty"`
	PluginsTracing  *PluginsTracing      `json:"pluginsTracing,omitempty"`
	Out             *Out                 `json:"out,omitempty"`
	Error           error                `json:"error,omitempty"`
	HttpPattern     string               `json:"httpPattern,omitempty"`
	podName         string
	podNamespace    string
	daprClient      dapr.Client
	mode            string
}

type EventMetadata struct {
	InputName    string               `json:"inputName,omitempty"`
	BindingEvent *common.BindingEvent `json:"bindingEvent,omitempty"`
	TopicEvent   *common.TopicEvent   `json:"topicEvent,omitempty"`
	CloudEvent   *cloudevents.Event   `json:"cloudEventnt,omitempty"`
}

type SyncRequestMetadata struct {
	ResponseWriter http.ResponseWriter `json:"responseWriter,omitempty"`
	Request        *http.Request       `json:"request,omitempty"`
}

type Input struct {
	Uri       string            `json:"uri,omitempty"`
	Component string            `json:"component,omitempty"`
	Type      ResourceType      `json:"type"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type Output struct {
	Uri       string            `json:"uri,omitempty"`
	Component string            `json:"component,omitempty"`
	Type      ResourceType      `json:"type"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Operation string            `json:"operation,omitempty"`
}

type Out struct {
	mu       sync.Mutex
	Code     int               `json:"code"`
	Data     []byte            `json:"data,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type PluginsTracing struct {
	Enable   bool              `json:"enable" yaml:"enable"`
	Provider *TracingProvider  `json:"provider" yaml:"provider"`
	Tags     map[string]string `json:"tags,omitempty" yaml:"tags,omitempty"`
	Baggage  map[string]string `json:"baggage" yaml:"baggage"`
}

type TracingProvider struct {
	Name      string `json:"name" yaml:"name"`
	OapServer string `json:"oapServer" yaml:"oapServer"`
}

func (ctx *Context) Send(outputName string, data []byte) ([]byte, error) {
	if ctx.HasOutputs() {
		return nil, errors.New("no output")
	}

	var err error
	var output *Output
	var response *dapr.BindingEvent

	if v, ok := ctx.Outputs[outputName]; ok {
		output = v
	} else {
		return nil, fmt.Errorf("output %s not found", outputName)
	}

	switch output.Type {
	case OpenFuncTopic:
		err = ctx.daprClient.PublishEvent(context.Background(), output.Component, output.Uri, data)
	case OpenFuncBinding:
		in := &dapr.InvokeBindingRequest{
			Name:      output.Component,
			Operation: output.Operation,
			Data:      data,
			Metadata:  output.Metadata,
		}
		response, err = ctx.daprClient.InvokeBinding(context.Background(), in)
	}

	if err != nil {
		return nil, err
	}

	if response != nil {
		return response.Data, nil
	}
	return nil, nil
}

func (ctx *Context) HasInputs() bool {
	nilInputs := map[string]*Input{}
	if reflect.DeepEqual(ctx.Inputs, nilInputs) {
		return false
	}
	return true
}

func (ctx *Context) HasOutputs() bool {
	nilOutputs := map[string]*Output{}
	if reflect.DeepEqual(ctx.Outputs, nilOutputs) {
		return false
	}
	return true
}

func (ctx *Context) ReturnOnSuccess() Out {
	return Out{
		Code: Success,
	}
}

func (ctx *Context) ReturnOnInternalError() Out {
	return Out{
		Code: InternalError,
	}
}

func (ctx *Context) InitDaprClientIfNil() {
	if ctx.daprClient == nil {
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		c, e := dapr.NewClientWithPort(clientGRPCPort)
		if e != nil {
			panic(e)
		}
		ctx.daprClient = c
	}
}

func (ctx *Context) DestroyDaprClient() {
	if ctx.daprClient != nil {
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		ctx.daprClient.Close()
		ctx.daprClient = nil
	}
}

func (ctx *Context) GetPrePlugins() []string {
	return ctx.PrePlugins
}

func (ctx *Context) GetPostPlugins() []string {
	return ctx.PostPlugins
}

func (ctx *Context) GetRuntime() Runtime {
	return ctx.Runtime
}

func (ctx *Context) GetPort() string {
	return ctx.Port
}

func (ctx *Context) GetHttpPattern() string {
	return ctx.HttpPattern
}

func (ctx *Context) GetError() error {
	return ctx.Error
}

func (ctx *Context) GetMode() string {
	return ctx.mode
}

func (ctx *Context) GetNativeContext() context.Context {
	return ctx.Ctx
}

func (ctx *Context) SetSyncRequestMeta(w http.ResponseWriter, r *http.Request) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.SyncRequestMeta.ResponseWriter = w
	ctx.SyncRequestMeta.Request = r
}

func (ctx *Context) SetEventMeta(inputName string, event interface{}) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	switch event.(type) {
	case *common.BindingEvent:
		ctx.EventMeta.BindingEvent = event.(*common.BindingEvent)
	case *common.TopicEvent:
		ctx.EventMeta.TopicEvent = event.(*common.TopicEvent)
	case *cloudevents.Event:
		ctx.EventMeta.CloudEvent = event.(*cloudevents.Event)
	default:
		klog.Error("failed to resolve event type")
	}
	ctx.EventMeta.InputName = inputName
}

func (ctx *Context) GetContext() *Context {
	return ctx
}

func (ctx *Context) GetInputs() map[string]*Input {
	return ctx.Inputs
}

func (ctx *Context) GetOutputs() map[string]*Output {
	return ctx.Outputs
}

func (ctx *Context) GetPodName() string {
	return ctx.podName
}

func (ctx *Context) GetPodNamespace() string {
	return ctx.podNamespace
}

func (ctx *Context) GetSyncRequestMeta() *SyncRequestMetadata {
	return ctx.SyncRequestMeta
}

func (ctx *Context) GetBindingEventMeta() *common.BindingEvent {
	return ctx.EventMeta.BindingEvent
}

func (ctx *Context) GetTopicEventMeta() *common.TopicEvent {
	return ctx.EventMeta.TopicEvent
}

func (ctx *Context) GetCloudEventMeta() *cloudevents.Event {
	return ctx.EventMeta.CloudEvent
}

func (ctx *Context) GetPluginsTracingCfg() TracingConfig {
	return ctx.PluginsTracing
}

func (ctx *Context) WithOut(out *Out) RuntimeContext {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.Out = out
	return ctx
}

func (ctx *Context) WithError(err error) RuntimeContext {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.Error = err
	return ctx
}

func (ctx *Context) GetOut() *Out {
	return ctx.Out
}

func (o *Out) GetOut() *Out {
	return o
}

func (o *Out) GetCode() int {
	return o.Code
}

func (o *Out) GetData() []byte {
	return o.Data
}

func (o *Out) GetMetadata() map[string]string {
	return o.Metadata
}

func (o *Out) WithCode(code int) *Out {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.Code = code
	return o
}

func (o *Out) WithData(data []byte) *Out {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.Data = data
	return o
}

func (tracing *PluginsTracing) IsEnabled() bool {
	return tracing.Enable
}

func (tracing *PluginsTracing) ProviderName() string {
	if tracing.Provider != nil {
		return tracing.Provider.Name
	} else {
		return ""
	}
}

func (tracing *PluginsTracing) ProviderOapServer() string {
	if tracing.Provider != nil {
		return tracing.Provider.OapServer
	} else {
		return ""
	}
}

func (tracing *PluginsTracing) GetTags() map[string]string {
	return tracing.Tags
}

func (tracing *PluginsTracing) GetBaggage() map[string]string {
	return tracing.Baggage
}

func registerTracingPluginIntoPrePlugins(plugins []string, target string) []string {
	if plugins == nil {
		plugins = []string{}
	}
	if exist := hasPlugin(plugins, target); !exist {
		plugins = append(plugins, target)
	}
	return plugins
}

func registerTracingPluginIntoPostPlugins(plugins []string, target string) []string {
	if exist := hasPlugin(plugins, target); !exist {
		plugins = append(plugins[:1], plugins[:]...)
		plugins[0] = target
	}
	return plugins
}

func hasPlugin(plugins []string, target string) bool {
	for _, plg := range plugins {
		if plg == target {
			return true
		}
	}
	return false
}

func GetRuntimeContext() (RuntimeContext, error) {
	if ctx, err := parseContext(); err != nil {
		return nil, err
	} else {
		return ctx, nil
	}
}

func parseContext() (*Context, error) {
	ctx := &Context{
		Inputs:  make(map[string]*Input),
		Outputs: make(map[string]*Output),
	}

	data := os.Getenv(functionContextEnvName)
	if data == "" {
		return nil, fmt.Errorf("env %s not found", functionContextEnvName)
	}

	err := json.Unmarshal([]byte(data), ctx)
	if err != nil {
		return nil, err
	}

	switch ctx.Runtime {
	case Async, Knative:
		break
	default:
		return nil, fmt.Errorf("invalid runtime: %s", ctx.Runtime)
	}

	ctx.EventMeta = &EventMetadata{}
	ctx.SyncRequestMeta = &SyncRequestMetadata{}

	if !ctx.HasInputs() {
		for name, in := range ctx.Inputs {
			switch in.Type {
			case OpenFuncBinding, OpenFuncTopic:
				break
			default:
				return nil, fmt.Errorf("invalid input type %s: %s", name, in.Type)
			}
		}
	}

	if !ctx.HasOutputs() {
		for name, out := range ctx.Outputs {
			switch out.Type {
			case OpenFuncBinding, OpenFuncTopic:
				break
			default:
				return nil, fmt.Errorf("invalid output type %s: %s", name, out.Type)
			}
		}
	}

	switch os.Getenv(ModeEnvName) {
	case SelfHostMode:
		ctx.mode = SelfHostMode
	default:
		ctx.mode = KubernetesMode
	}

	if ctx.mode == KubernetesMode {
		podName := os.Getenv(PodNameEnvName)
		if podName == "" {
			return nil, errors.New("the name of the pod cannot be retrieved from the environment, " +
				"you need to set the POD_NAME environment variable")
		}
		ctx.podName = podName

		podNamespace := os.Getenv(PodNamespaceEnvName)
		if podNamespace == "" {
			return nil, errors.New("the namespace of the pod cannot be retrieved from the environment, " +
				"you need to set the POD_NAMESPACE environment variable")
		}
		ctx.podNamespace = podNamespace
	}

	if ctx.PluginsTracing != nil && ctx.PluginsTracing.Enable {
		if ctx.PluginsTracing.Provider != nil && ctx.PluginsTracing.Provider.Name != "" {
			switch ctx.PluginsTracing.Provider.Name {
			case TracingProviderSkywalking, TracingProviderOpentelemetry:
				ctx.PrePlugins = registerTracingPluginIntoPrePlugins(ctx.PrePlugins, ctx.PluginsTracing.Provider.Name)
				ctx.PostPlugins = registerTracingPluginIntoPostPlugins(ctx.PostPlugins, ctx.PluginsTracing.Provider.Name)
			default:
				return nil, fmt.Errorf("invalid tracing provider name: %s", ctx.PluginsTracing.Provider.Name)
			}
			if ctx.PluginsTracing.Tags != nil {
				if funcName, ok := ctx.PluginsTracing.Tags["func"]; !ok || funcName != ctx.Name {
					ctx.PluginsTracing.Tags["func"] = ctx.Name
				}
				ctx.PluginsTracing.Tags["instance"] = ctx.podName
				ctx.PluginsTracing.Tags["namespace"] = ctx.podNamespace
			}
		} else {
			return nil, errors.New("the tracing plugin is enabled, but its configuration is incorrect")
		}
	}

	if ctx.Port == "" {
		ctx.Port = defaultPort
	} else {
		if _, err := strconv.Atoi(ctx.Port); err != nil {
			return nil, fmt.Errorf("error parsing port: %s", err.Error())
		}
	}

	// When using self-hosted mode, configure the client port via env,
	// refer to https://docs.dapr.io/reference/environment/
	port := os.Getenv("DAPR_GRPC_PORT")
	if port == "" {
		clientGRPCPort = daprSidecarGRPCPort
	} else {
		clientGRPCPort = port
	}

	return ctx, nil
}
