package billing_notifier

import (
	"cloud.google.com/go/firestore"
	"context"
	"encoding/json"
	"fmt"
	"github.com/nlopes/slack"
	"golang.org/x/xerrors"
	"log"
	"os"
	"time"
)

var AlreadyUsedErr = xerrors.New("Already used token")

type BillingMessage struct {
	BudgetDisplayName      string    `json:budgetDisplayName`
	AlertThresholdExceeded float64   `json:alertThresholdExceeded`
	CostAmount             float64   `json:costAmount`
	CostIntervalStart      time.Time `json:costIntervalStart`
	BudgetAmount           float64   `json:budgetAmount`
	BudgetAmountType       string    `json:budgetAmountType`
	CurrencyCode           string    `json:CurrencyCode`
}

type PubSubMessage struct {
	Data       []byte `json:"data"`
	Attributes struct {
		BillingAccountId string `json:billingAccountId`
		BudgetId         string `json:budgetId`
		SchemaVersion    string `json:schemaVersion`
	} `json:"attributes"`
}

func GenerateToken(m *PubSubMessage, b *BillingMessage) string {
	return fmt.Sprintf("%s:%d:%f", m.Attributes.BudgetId, b.CostIntervalStart.Unix(), b.AlertThresholdExceeded)
}

func GenerateMessage(b *BillingMessage) string {
	return fmt.Sprintf("[%s] %s 予算(%.0f円)の%.0f%%に達しました。現在の利用額: %.0f円",
		b.BudgetDisplayName,
		b.CostIntervalStart.Format("2006年01月"),
		b.BudgetAmount,
		b.AlertThresholdExceeded*100,
		b.CostAmount)
}

func CheckDuplicate(client *firestore.Client, ctx context.Context, collection string, token string) error {
	// 既に利用されたToken
	var docRef = client.Collection(collection).Doc(token)
	var newUsed int64
	err := client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		var used interface{}
		doc, err := tx.Get(docRef)
		if err != nil {
			used = int64(0)
		} else {
			var err error
			used, err = doc.DataAt("used")
			if err != nil {
				return err
			}
		}
		var ok bool
		newUsed, ok = used.(int64)
		if !ok {
			return xerrors.New("Invalid Field")
		}
		newUsed += 1
		return tx.Set(docRef, map[string]interface{}{
			"used":       newUsed,
			"updated_at": time.Now().Unix(),
		}, firestore.MergeAll)
	})
	if err != nil {
		return err
	}
	if newUsed != 1 {
		return AlreadyUsedErr
	}
	return nil
}

func BillingNotifier(ctx context.Context, m PubSubMessage) error {
	log.Printf("%+v", m)

	client, err := firestore.NewClient(ctx, os.Getenv("GCP_PROJECT"))
	if err != nil {
		return xerrors.Errorf("Failed to create firestore client: %w", err)
	}
	defer client.Close()
	collection := os.Getenv("FIRESTORE_COLLECTION")
	if collection == "" {
		collection = "billing_notifier"
	}

	var message BillingMessage
	if err := json.Unmarshal(m.Data, &message); err != nil {
		return xerrors.Errorf("Failed to unmarshal json: %w", err)
	}

	token := GenerateToken(&m, &message)
	if err := CheckDuplicate(client, ctx, collection, token); err == AlreadyUsedErr {
		return nil
	} else if err != nil {
		return xerrors.Errorf("Failed to check token: %w", err)
	}

	slackMsg := &slack.WebhookMessage{Text: GenerateMessage(&message)}
	if err := slack.PostWebhook(os.Getenv("SLACK_WEBHOOK_URL"), slackMsg); err != nil {
		return xerrors.Errorf("Failed to post slack: %w", err)
	}
	return nil
}
