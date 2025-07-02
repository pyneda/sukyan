package browser

import (
	"encoding/json"
	"github.com/go-rod/rod"
)

// PostMessage represents the structure of a post message
type PostMessage struct {
	Data      interface{} `json:"data"`
	Origin    string      `json:"origin"`
	Timestamp int64       `json:"timestamp"`
}

// ListenForPostMessages executes JavaScript in the page to listen for post messages
func ListenForPostMessages(page *rod.Page) error {
	jsCode := `
    function capturePostMessage(event) {
        if (event.data.listener) {
            var postMessageData = {
                data: event.data,
                origin: event.origin,
                timestamp: Date.now()
            };
            window.postMessageData = window.postMessageData || [];
            window.postMessageData.push(postMessageData);
        }
    }

    window.addEventListener('message', capturePostMessage);
    `
	_, err := page.Eval(jsCode)

	return err
}

// GetPostMessages returns all post messages that have been sent to the page.
// NOTE: This function will only return messages that have been sent after ListenForPostMessages has been called.
func GetPostMessages(page *rod.Page) ([]PostMessage, error) {
	evalResult, err := page.Eval(`JSON.stringify(window.postMessageData)`)
	if err != nil {
		return nil, err
	}
	resultStr := evalResult.Value.String()
	var postMessages []PostMessage
	if err := json.Unmarshal([]byte(resultStr), &postMessages); err != nil {
		return nil, err
	}
	return postMessages, nil
}
