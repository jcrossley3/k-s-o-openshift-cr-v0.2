/*
Copyright 2019 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// util has constants and helper methods useful for zipkin tracing support.

package zipkin

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/openzipkin/zipkin-go/model"
	"go.opencensus.io/trace"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/test/logging"
	"knative.dev/pkg/test/monitoring"
)

const (
	// ZipkinTraceIDHeader HTTP response header key to be used to store Zipkin Trace ID.
	ZipkinTraceIDHeader = "ZIPKIN_TRACE_ID"

	// ZipkinPort is port exposed by the Zipkin Pod
	// https://github.com/knative/serving/blob/master/config/monitoring/200-common/100-zipkin.yaml#L25 configures the Zipkin Port on the cluster.
	ZipkinPort = 9411

	// ZipkinTraceEndpoint port-forwarded zipkin endpoint
	ZipkinTraceEndpoint = "http://localhost:9411/api/v2/trace/"

	// App is the name of this component.
	// This will be used as a label selector.
	app = "zipkin"

	// istioNS is the namespace we are using for istio components.
	istioNS = "istio-system"
)

var (
	zipkinPortForwardPID int

	// ZipkinTracingEnabled variable indicating if zipkin tracing is enabled.
	ZipkinTracingEnabled = false

	// sync.Once variable to ensure we execute zipkin setup only once.
	setupOnce sync.Once

	// sync.Once variable to ensure we execute zipkin cleanup only if zipkin is setup and it is executed only once.
	teardownOnce sync.Once
)

// SetupZipkinTracing sets up zipkin tracing which involves:
// 1. Setting up port-forwarding from localhost to zipkin pod on the cluster
//    (pid of the process doing Port-Forward is stored in a global variable).
// 2. Enable AlwaysSample config for tracing for the SpoofingClient.
func SetupZipkinTracing(kubeClientset *kubernetes.Clientset, logf logging.FormatLogger) bool {
	setupOnce.Do(func() {
		if err := monitoring.CheckPortAvailability(ZipkinPort); err != nil {
			logf("Zipkin port not available on the machine: %v", err)
			return
		}

		zipkinPods, err := monitoring.GetPods(kubeClientset, app, istioNS)
		if err != nil {
			logf("Error retrieving Zipkin pod details: %v", err)
			return
		}

		zipkinPortForwardPID, err = monitoring.PortForward(logf, zipkinPods, ZipkinPort, ZipkinPort, istioNS)
		if err != nil {
			logf("Error starting kubectl port-forward command: %v", err)
			return
		}

		logf("Zipkin port-forward process started with PID: %d", zipkinPortForwardPID)

		// Applying AlwaysSample config to ensure we propagate zipkin header for every request made by this client.
		trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})
		logf("Successfully setup SpoofingClient for Zipkin Tracing")
		ZipkinTracingEnabled = true
	})
	return ZipkinTracingEnabled
}

// CleanupZipkinTracingSetup cleans up the Zipkin tracing setup on the machine. This involves killing the process performing port-forward.
// This should be called exactly once in TestMain. Likely in the form:
//
// func TestMain(m *testing.M) {
//     os.Exit(func() int {
//       // Any setup required for the tests.
//       defer zipkin.CleanupZipkinTracingSetup(logger)
//       return m.Run()
//     }())
// }
func CleanupZipkinTracingSetup(logf logging.FormatLogger) {
	teardownOnce.Do(func() {
		// Because CleanupZipkinTracingSetup only runs once, make sure that now that it has been
		// run, SetupZipkinTracing will no longer setup any port forwarding.
		setupOnce.Do(func() {})

		if !ZipkinTracingEnabled {
			return
		}

		if err := monitoring.Cleanup(zipkinPortForwardPID); err != nil {
			logf("Encountered error killing port-forward process in CleanupZipkinTracingSetup() : %v", err)
			return
		}

		ZipkinTracingEnabled = false
	})
}

// JSONTrace returns a trace for the given traceID. It will continually try to get the trace. If the
// trace it gets has the expected number of spans, then it will be returned. If not, it will try
// again. If it reaches timeout, then it returns everything it has so far with an error.
func JSONTrace(traceID string, expected int, timeout time.Duration) (trace []model.SpanModel, err error) {
	t := time.After(timeout)
	for len(trace) != expected {
		select {
		case <-t:
			return trace, &TimeoutError{
				lastErr: err,
			}
		default:
			trace, err = jsonTrace(traceID)
		}
	}
	return trace, err
}

// TimeoutError is an error returned by JSONTrace if it times out before getting the expected number
// of traces.
type TimeoutError struct{
	lastErr error
}

func (t *TimeoutError) Error() string {
	return fmt.Sprintf("timeout getting JSONTrace, most recent error: %v", t.lastErr)
}

// jsonTrace gets a trace from Zipkin and returns it. Errors returned from this function should be
// retried, as they are likely caused by random problems communicating with Zipkin, or Zipkin
// communicating with its data store.
func jsonTrace(traceID string) ([]model.SpanModel, error) {
	var empty []model.SpanModel

	resp, err := http.Get(ZipkinTraceEndpoint + traceID)
	if err != nil {
		return empty, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return empty, err
	}

	var models []model.SpanModel
	err = json.Unmarshal(body, &models)
	if err != nil {
		return empty, fmt.Errorf("got an error in unmarshalling JSON %q: %v", body, err)
	}
	return models, nil
}
