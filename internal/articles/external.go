package articles

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// webhookClient is used for outbound webhook calls. A bounded timeout keeps
// a worker goroutine from hanging indefinitely if the webhook endpoint is
// slow or unresponsive.
var webhookClient = &http.Client{Timeout: 10 * time.Second}

// ErrWebhookPermanent indicates the webhook rejected the request in a way that
// retrying with the same payload cannot fix (4xx status code).
var ErrWebhookPermanent = errors.New("webhook rejected request permanently")

func NotifyArticlePublishedWebhook(ctx context.Context, webhookURL, articleID string) error {
	if webhookURL == "" {
		return nil
	}

	body, err := json.Marshal(map[string]string{"article_id": articleID})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := webhookClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return fmt.Errorf("webhook returned status %d: %w", resp.StatusCode, ErrWebhookPermanent)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}
