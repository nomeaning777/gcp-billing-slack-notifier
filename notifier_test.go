package billing_notifier

import (
	"cloud.google.com/go/firestore"
	"context"
	"os"
	"testing"
	"time"
)

func TestGenerateToken(t *testing.T) {
	cases := []struct {
		pubSubMessage  PubSubMessage
		billingMessage BillingMessage
		expected       string
	}{
		{
			pubSubMessage: PubSubMessage{
				Data: nil,
				Attributes: struct {
					BillingAccountId string `json:billingAccountId`
					BudgetId         string `json:budgetId`
					SchemaVersion    string `json:schemaVersion`
				}{
					BillingAccountId: "billing_id",
					BudgetId:         "budget_id",
					SchemaVersion:    "1.0",
				},
			},
			billingMessage: BillingMessage{
				BudgetDisplayName:      "Hogehoge",
				AlertThresholdExceeded: 0.1,
				CostAmount:             100.23,
				CostIntervalStart:      time.Unix(1567576776, 0),
				BudgetAmount:           1000,
				BudgetAmountType:       "SPECIFIED_AMOUNT",
				CurrencyCode:           "JPY",
			},
			expected: "budget_id:1567576776:0.100000",
		},
	}
	for _, cc := range cases {
		msg := GenerateToken(&cc.pubSubMessage, &cc.billingMessage)
		if msg != cc.expected {
			t.Errorf("got:%s, want:%s", msg, cc.expected)
		}
	}
}

func TestGenerateMessage(t *testing.T) {
	cases := []struct {
		billingMessage BillingMessage
		expected       string
	}{
		{
			billingMessage: BillingMessage{
				BudgetDisplayName:      "Hogehoge",
				AlertThresholdExceeded: 0.1,
				CostAmount:             100.23,
				CostIntervalStart:      time.Unix(1567576776, 0),
				BudgetAmount:           1000,
				BudgetAmountType:       "SPECIFIED_AMOUNT",
				CurrencyCode:           "JPY",
			},
			expected: "[Hogehoge] 2019年09月 予算(1000円)の10%に達しました。現在の利用額: 100円",
		},
	}
	for _, cc := range cases {
		msg := GenerateMessage(&cc.billingMessage)
		if msg != cc.expected {
			t.Errorf("got:%s, want:%s", msg, cc.expected)
		}
	}
}

func TestCheckDuplicate(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST is empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	client, err := firestore.NewClient(ctx, "test39project")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	cases := []struct {
		collection string
		token      string
		err        error
	}{
		{collection: "hoge", token: "token1", err: nil},
		{collection: "hoge", token: "token2", err: nil},
		{collection: "hoge", token: "token3", err: nil},
		{collection: "hoge", token: "token2", err: AlreadyUsedErr},
		{collection: "hoge", token: "token2", err: AlreadyUsedErr},
		{collection: "hoge", token: "token4", err: nil},
		{collection: "hoge", token: "token1", err: AlreadyUsedErr},
	}

	for _, cc := range cases {
		if err := CheckDuplicate(client, ctx, cc.collection, cc.token); err != cc.err {
			t.Errorf("Unexpected err: got:%+v, want:%+v", err, cc.err)
		}
	}
}

func TestBillingNotifier(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST is empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	os.Setenv("GCP_PROJECT", "test39project")

	err := BillingNotifier(ctx, PubSubMessage{
		Data: []byte(`{
			"budgetDisplayName": "example",
			"alertThresholdExceeded": 0.2,
			"costAmount": 100.15,
			"costIntervalStart": "2019-09-01T07:00:00Z",
			"budgetAmount": 1000.0,
			"budgetAmountType": "SPECIFIED_AMOUNT",
			"currencyCode": "JPY"
		}`),
		Attributes: struct {
			BillingAccountId string `json:billingAccountId`
			BudgetId         string `json:budgetId`
			SchemaVersion    string `json:schemaVersion`
		}{BillingAccountId: "account_id", BudgetId: "budget_id", SchemaVersion: "1.0"},
	})
	if err != nil {
		t.Errorf("Unexpected err: %+v", err)
	}
}
