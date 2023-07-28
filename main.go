package slack

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/parnurzeal/gorequest"
)

type Field struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

type Action struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Url   string `json:"url"`
	Style string `json:"style"`
}

type Attachment struct {
	Fallback     *string   `json:"fallback"`
	Color        *string   `json:"color"`
	PreText      *string   `json:"pretext"`
	AuthorName   *string   `json:"author_name"`
	AuthorLink   *string   `json:"author_link"`
	AuthorIcon   *string   `json:"author_icon"`
	Title        *string   `json:"title"`
	TitleLink    *string   `json:"title_link"`
	Text         *string   `json:"text"`
	ImageUrl     *string   `json:"image_url"`
	Fields       []*Field  `json:"fields"`
	Footer       *string   `json:"footer"`
	FooterIcon   *string   `json:"footer_icon"`
	Timestamp    *int64    `json:"ts"`
	MarkdownIn   *[]string `json:"mrkdwn_in"`
	Actions      []*Action `json:"actions"`
	CallbackID   *string   `json:"callback_id"`
	ThumbnailUrl *string   `json:"thumb_url"`
}

type Payload struct {
	Parse       string       `json:"parse,omitempty"`
	Username    string       `json:"username,omitempty"`
	IconUrl     string       `json:"icon_url,omitempty"`
	IconEmoji   string       `json:"icon_emoji,omitempty"`
	Channel     string       `json:"channel,omitempty"`
	Text        string       `json:"text,omitempty"`
	LinkNames   string       `json:"link_names,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
	UnfurlLinks bool         `json:"unfurl_links,omitempty"`
	UnfurlMedia bool         `json:"unfurl_media,omitempty"`
	Markdown    bool         `json:"mrkdwn,omitempty"`
}

func (attachment *Attachment) AddField(field Field) *Attachment {
	attachment.Fields = append(attachment.Fields, &field)
	return attachment
}

func (attachment *Attachment) AddAction(action Action) *Attachment {
	attachment.Actions = append(attachment.Actions, &action)
	return attachment
}

func redirectPolicyFunc(req gorequest.Request, via []gorequest.Request) error {
	return fmt.Errorf("Incorrect token (redirection)")
}

var (
	statusCodeMap    = make(map[int]int)
	statusCodeLock   sync.Mutex
	statusCodeTicker *time.Ticker
)

func Init() {
	if os.Getenv("SLACK_GO_WEBHOOK_DEBUG") != "" {
		InitialiseTicker()
	}
}

func Send(webhookUrl string, proxy string, payload Payload) []error {

	request := gorequest.New().Proxy(proxy)

	for {
		resp, _, err := request.
			Post(webhookUrl).
			RedirectPolicy(redirectPolicyFunc).
			Send(payload).
			End()

		if err != nil {
			return err
		}

		if os.Getenv("SLACK_GO_WEBHOOK_DEBUG") != "" {
			incrementStatusCode(resp.StatusCode)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfterHeader := resp.Header.Get("Retry-After")
			if retryAfterHeader != "" {
				retryAfterSeconds, err := strconv.Atoi(retryAfterHeader)
				if err != nil {
					return []error{fmt.Errorf("Error parsing Retry-After header: %s", retryAfterHeader)}
				}
				time.Sleep(time.Duration(retryAfterSeconds) * time.Second)
			} else {
				// If Retry-After header is missing or invalid, wait for 1 second before retrying.
				time.Sleep(1 * time.Second)
			}
		} else if resp.StatusCode >= 400 {
			return []error{fmt.Errorf("Error sending msg. Status: %v", resp.Status)}
		} else {
			return nil
		}
	}
}

func InitialiseTicker() {
	statusCodeLock.Lock()
	defer statusCodeLock.Unlock()

	if statusCodeTicker == nil {
		fmt.Printf("Initialising status code ticker (1/min)\n")
		statusCodeTicker = time.NewTicker(1 * time.Minute)
		go func() {
			for t := range statusCodeTicker.C {
				reportStatusCodes(t)
			}
		}()
	}
}

func incrementStatusCode(code int) {
	statusCodeLock.Lock()
	defer statusCodeLock.Unlock()

	_, ok := statusCodeMap[code]
	if !ok {
		statusCodeMap[code] = 1
	} else {
		statusCodeMap[code]++
	}
}

func reportStatusCodes(tick time.Time) {
	statusCodeLock.Lock()
	defer statusCodeLock.Unlock()
	fmt.Printf("Slack HTTP response codes / min = %v (tick %v)\n", statusCodeMap, tick)

	resetStatusCodes()
}

func resetStatusCodes() {
	statusCodeLock.Lock()
	defer statusCodeLock.Unlock()

	for code := range statusCodeMap {
		statusCodeMap[code] = 0
	}
}
