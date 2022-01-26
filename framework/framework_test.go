package framework

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	ofctx "github.com/tpiperatgod/offf-go/context"
)

func fakeHTTPFunction(w http.ResponseWriter, r *http.Request) error {
	fmt.Fprint(w, "Hello World!")
	return nil
}

func fakeCloudEventsFunction(ctx context.Context, ce cloudevents.Event) error {
	fmt.Println(string(ce.Data()))
	return nil
}

func TestHTTPFunction(t *testing.T) {
	env := `{
  "name": "function-demo",
  "version": "v1.0.0",
  "port": "8080",
  "runtime": "Knative",
  "httpPattern": "/http"
}`
	ctx := context.Background()
	fwk, err := createFramework(env)
	if err != nil {
		t.Fatalf("failed to create framework: %v", err)
	}

	fwk.RegisterPlugins(nil)

	if err := fwk.Register(ctx, fakeHTTPFunction); err != nil {
		t.Fatalf("failed to register HTTP function: %v\n", err)
	}

	if fwk.GetRuntime() == nil {
		t.Fatal("failed to create runtime")
	}
	srv := httptest.NewServer(fwk.GetRuntime().GetHTTPHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/http")
	if err != nil {
		t.Fatalf("http.Get: %v", err)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ioutil.ReadAll: %v", err)
	}

	if got, want := string(body), "Hello World!"; got != want {
		t.Fatalf("TestHTTPFunction: got %v; want %v", got, want)
	}
}

func TestCloudEventFunction(t *testing.T) {
	env := `{
  "name": "function-demo",
  "version": "v1.0.0",
  "port": "8080",
  "runtime": "Knative",
  "httpPattern": "/ce"
}`
	var ceDemo = struct {
		message map[string]string
		headers map[string]string
	}{
		message: map[string]string{
			"msg": "Hello World!",
		},
		headers: map[string]string{
			"Ce-Specversion": "1.0",
			"Ce-Type":        "cloudevents.openfunction.samples.helloworld",
			"Ce-Source":      "cloudevents.openfunction.samples/helloworldsource",
			"Ce-Id":          "536808d3-88be-4077-9d7a-a3f162705f79",
		},
	}

	ctx := context.Background()
	fwk, err := createFramework(env)
	if err != nil {
		t.Fatalf("failed to create framework: %v", err)
	}

	fwk.RegisterPlugins(nil)

	if err := fwk.Register(ctx, fakeCloudEventsFunction); err != nil {
		t.Fatalf("failed to register CloudEvents function: %v\n", err)
	}

	if fwk.GetRuntime() == nil {
		t.Fatal("failed to create runtime\n")
	}
	srv := httptest.NewServer(fwk.GetRuntime().GetHTTPHandler())
	defer srv.Close()

	messageByte, err := json.Marshal(ceDemo.message)
	if err != nil {
		t.Fatalf("failed to marshal message: %v", err)
	}

	req, err := http.NewRequest("POST", srv.URL+"/ce", bytes.NewBuffer(messageByte))
	if err != nil {
		t.Fatalf("error creating HTTP request for test: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range ceDemo.headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to do client.Do: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("failed to test cloudevents function: response status = %v, want %v", resp.StatusCode, http.StatusOK)
	}
}

func createFramework(env string) (Framework, error) {
	os.Setenv(ofctx.ModeEnvName, ofctx.SelfHostMode)
	os.Setenv(ofctx.FunctionContextEnvName, env)
	fwk, err := NewFramework()
	if err != nil {
		return nil, err
	} else {
		return fwk, nil
	}
}
