package knativeserving

import (
	"context"
	"encoding/json"
	"net/http"

	servingv1alpha1 "knative.dev/serving-operator/pkg/apis/serving/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var log = logf.Log.WithName("webhook_knativeserving")

// Add creates a new KnativeServing Webhook
func Add(mgr manager.Manager) error {
	log.Info("Setting up mutating webhook for KnativeServing")
	hookServer := mgr.GetWebhookServer()
	hookServer.Register("/mutate-v1alpha1-knativeserving", &webhook.Admission{Handler: &KnativeServingConfigurator{}})
	return nil
}

// KnativeServingConfigurator annotates Kss
type KnativeServingConfigurator struct {
	client  client.Client
	decoder *admission.Decoder
}

// Implement admission.Handler so the controller can handle admission request.
var _ admission.Handler = (*KnativeServingConfigurator)(nil)

// KnativeServingConfigurator adds an annotation to every incoming
// KnativeServing CR.
func (a *KnativeServingConfigurator) Handle(ctx context.Context, req admission.Request) admission.Response {
	ks := &servingv1alpha1.KnativeServing{}

	err := a.decoder.Decode(req, ks)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	err = a.mutate(ctx, ks)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	marshaledPod, err := json.Marshal(ks)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPod)
}

// mutate configures the given ks
func (a *KnativeServingConfigurator) mutate(ctx context.Context, ks *servingv1alpha1.KnativeServing) error {
	const (
		configmap = "network"
		key       = "istio.sidecar.includeOutboundIPRanges"
		value     = "10.0.0.1/24"
	)
	if ks.Spec.Config == nil {
		ks.Spec.Config = map[string]map[string]string{}
	}
	if len(ks.Spec.Config[configmap][key]) == 0 {
		if ks.Spec.Config[configmap] == nil {
			ks.Spec.Config[configmap] = map[string]string{}
		}
		ks.Spec.Config[configmap][key] = value
	}
	return nil
}

// InjectClient injects the client.
func (v *KnativeServingConfigurator) InjectClient(c client.Client) error {
	v.client = c
	return nil
}

// InjectDecoder injects the decoder.
func (v *KnativeServingConfigurator) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
