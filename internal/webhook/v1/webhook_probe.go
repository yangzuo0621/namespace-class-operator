package v1

import (
	"crypto/tls"
	"net"
	"net/http"
	"strconv"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var healthzLog = ctrl.Log.WithName("webhook-probe")

func HealthzCheck(option webhook.Options) func(*http.Request) error {
	return func(r *http.Request) error {
		if err := dialWebhookServer(option); err != nil {
			return err
		}
		healthzLog.Info("webhook server is healthy", "host", option.Host, "port", option.Port)
		return nil
	}
}

func ReadyzCheck(option webhook.Options) func(*http.Request) error {
	return func(r *http.Request) error {
		if err := dialWebhookServer(option); err != nil {
			return err
		}
		healthzLog.Info("webhook server is ready", "host", option.Host, "port", option.Port)
		return nil
	}
}

func dialWebhookServer(option webhook.Options) error {
	cfg := &tls.Config{
		NextProtos:         []string{"h2"},
		InsecureSkipVerify: true,
	}
	for _, op := range option.TLSOpts {
		op(cfg)
	}
	if option.Port <= 0 {
		option.Port = webhook.DefaultPort
	}

	// wait for the webhook server to get ready.
	dialer := &net.Dialer{Timeout: time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(option.Host, strconv.Itoa(option.Port)), cfg)
	if err != nil {
		return err
	}
	if err := conn.Close(); err != nil {
		return err
	}
	return nil
}
