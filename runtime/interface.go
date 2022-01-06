package runtime

import (
	"context"
	"net/http"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	ofctx "github.com/tpiperatgod/offf-go/context"
)

type Interface interface {
	Start(ctx context.Context) error
	RegisterHTTPFunction(
		ctx ofctx.Context,
		processPreHooksFunc func() error,
		processPostHooksFunc func() error,
		fn func(http.ResponseWriter, *http.Request) error,
	) error
	RegisterOpenFunction(
		ctx ofctx.Context,
		processPreHooksFunc func() error,
		processPostHooksFunc func() error,
		fn func(ofctx.Context, []byte) (ofctx.Out, error),
	) error
	RegisterCloudEventFunction(
		ctx context.Context,
		ofContext ofctx.Context,
		processPreHooksFunc func() error,
		processPostHooksFunc func() error,
		fn func(context.Context, cloudevents.Event) error,
	) error
}
