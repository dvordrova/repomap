package bootstrap

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"example.invalid/fulfilment/internal/address"
	"example.invalid/fulfilment/internal/checkout"
	"example.invalid/fulfilment/internal/notification"
	"example.invalid/fulfilment/internal/payment"
	"example.invalid/fulfilment/internal/shipping"
	"example.invalid/fulfilment/internal/tax"
	"example.invalid/fulfilment/internal/telemetry"
)

type Runtime struct {
	Port     string
	Checkout *checkout.Service
}

func AssembleFromEnvironment() (*Runtime, error) {
	recorder := telemetry.Logger{}
	attempts, err := strconv.Atoi(required("PAYMENT_ATTEMPTS"))
	if err != nil {
		return nil, fmt.Errorf("PAYMENT_ATTEMPTS: %w", err)
	}
	paymentGateway, err := payment.Open(payment.Settings{
		URL: required("PAYMENT_URL"), MerchantID: required("PAYMENT_MERCHANT"),
		APISecret: required("PAYMENT_SECRET"), MaxAttempts: attempts,
	}, recorder)
	if err != nil {
		return nil, err
	}
	taxQuote, err := tax.Connect(tax.Deployment{
		Origin: required("TAX_URL"), Credential: required("TAX_TOKEN"),
		Organization: required("TAX_TENANT"), Deadline: 900 * time.Millisecond,
	}, recorder)
	if err != nil {
		return nil, err
	}
	addressVerifier, err := address.New(address.Configuration{
		ServiceURL: required("ADDRESS_URL"), AccessKey: required("ADDRESS_KEY"), RuleSet: "shipping-strict",
	}, recorder)
	if err != nil {
		return nil, err
	}
	parcelBooker, err := shipping.Open(shipping.Environment{
		Endpoint: required("PARCEL_URL"), Key: required("PARCEL_KEY"), Depot: required("PARCEL_DEPOT"),
	}, recorder)
	if err != nil {
		return nil, err
	}
	receiptDispatcher, err := notification.NewDispatcher(notification.Credentials{
		Token: required("RECEIPT_TOKEN"), Host: required("RECEIPT_URL"),
	}, recorder)
	if err != nil {
		return nil, err
	}
	return &Runtime{
		Port:     valueOr("PORT", "8080"),
		Checkout: checkout.New(paymentGateway, taxQuote, addressVerifier, parcelBooker, receiptDispatcher),
	}, nil
}

func required(name string) string { return os.Getenv(name) }

func valueOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
