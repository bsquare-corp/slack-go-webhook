package slack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"
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

var (
	HttpClient                       = &http.Client{}
	statusCodeMap                    = make(map[int]int)
	statusCodeLock                   sync.Mutex
	statusCodeTicker                 *time.Ticker
	StatusCodeTickerInterval         = time.Minute
	StatusCodeRetryInterval          = time.Millisecond * 100
	StatusCodeRetryIntervalIncrement = time.Millisecond * 100
	StatusCodeRetryIntervalDecrement = time.Millisecond * 1
)

func Init() {
	if os.Getenv("SLACK_GO_WEBHOOK_DEBUG") != "" {
		InitialiseTicker()
	}
}

func MinDuration(vars ...time.Duration) time.Duration {
	min := vars[0]

	for _, i := range vars {
		if min > i {
			min = i
		}
	}

	return min
}

func MaxDuration(vars ...time.Duration) time.Duration {
	max := vars[0]

	for _, i := range vars {
		if max < i {
			max = i
		}
	}

	return max
}

func Send(webhookUrl string, proxy string, payload Payload) []error {

	payloadJson, err := json.Marshal(payload)
	if err != nil {
		return []error{err}
	}

	if proxy != "" {
		proxyUrl, err := url.Parse(proxy)
		if err != nil {
			return []error{err}
		}
		HttpClient.Transport = &http.Transport{Proxy: http.ProxyURL(proxyUrl)}
	}

	for {
		req, err := http.NewRequest("POST", webhookUrl, bytes.NewBuffer(payloadJson))
		if err != nil {
			return []error{err}
		}

		resp, err := HttpClient.Do(req)
		if err != nil {
			return []error{err}
		}

		if os.Getenv("SLACK_GO_WEBHOOK_DEBUG") != "" {
			incrementStatusCode(resp.StatusCode)
		}

		// We alway sleep between messages, but we adapt our rate.
		time.Sleep(StatusCodeRetryInterval)

		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfterHeader := resp.Header.Get("Retry-After")
			if retryAfterHeader != "" {
				retryAfterSeconds, err := strconv.Atoi(retryAfterHeader)

				if err != nil {
					return []error{fmt.Errorf("Error parsing Retry-After header: %s", retryAfterHeader)}
				}

				StatusCodeRetryInterval = MinDuration(time.Duration(retryAfterSeconds)*time.Second, StatusCodeRetryInterval+StatusCodeRetryIntervalIncrement)
			} else {
				StatusCodeRetryInterval = MinDuration(4*time.Second, StatusCodeRetryInterval+StatusCodeRetryIntervalIncrement)
			}

			log.Printf("Slack HTTP failure response [StatusCodeRetryInterval=%v,StatusCodeRetryIntervalIncrement=%v,StatusCodeRetryIntervalDecrement=%v]\n", StatusCodeRetryInterval, StatusCodeRetryIntervalIncrement, StatusCodeRetryIntervalDecrement)

		} else if resp.StatusCode >= 400 {
			return []error{fmt.Errorf("Error sending msg. Status: %v", resp.StatusCode)}
		} else {
			StatusCodeRetryInterval = MaxDuration(0, StatusCodeRetryInterval-StatusCodeRetryIntervalDecrement)
			log.Printf("Slack HTTP success response [StatusCodeRetryInterval=%v,StatusCodeRetryIntervalIncrement=%v,StatusCodeRetryIntervalDecrement=%v]\n", StatusCodeRetryInterval, StatusCodeRetryIntervalIncrement, StatusCodeRetryIntervalDecrement)
			return nil
		}
	}
}

func InitialiseTicker() {
	statusCodeLock.Lock()
	defer statusCodeLock.Unlock()

	if statusCodeTicker == nil {
		log.Printf("Initialising status code ticker (1/min)\n")
		statusCodeTicker = time.NewTicker(StatusCodeTickerInterval)
		go func() {
			for t := range statusCodeTicker.C {
				reportStatusCodes(t)
				resetStatusCodes()
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

	log.Printf("Slack HTTP response codes / min = %v (tick %v)\n", statusCodeMap, tick)
	log.Printf("Slack HTTP response [StatusCodeRetryInterval=%v,StatusCodeRetryIntervalIncrement=%v,StatusCodeRetryIntervalDecrement=%v]\n", StatusCodeRetryInterval, StatusCodeRetryIntervalIncrement, StatusCodeRetryIntervalDecrement)
}

func resetStatusCodes() {
	statusCodeLock.Lock()
	defer statusCodeLock.Unlock()

	for code := range statusCodeMap {
		statusCodeMap[code] = 0
	}
}
