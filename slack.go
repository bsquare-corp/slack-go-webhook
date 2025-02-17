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
	// Private
	statusCodeMap        = make(map[int]int)
	statusCodeLock       sync.Mutex
	statusCodeTicker     *time.Ticker
	statusCodeTickerDone = make(chan bool)
	HttpClient           = &http.Client{}
	// Public
	StatusCodeTickerInterval         = time.Hour
	StatusCodeRetryInterval          = time.Millisecond * 100
	StatusCodeRetryIntervalIncrement = time.Millisecond * 100
	StatusCodeRetryIntervalDecrement = time.Millisecond * 1
)

func Init() {
	if os.Getenv("SLACK_GO_WEBHOOK_DEBUG") != "" {
		StartTicker()
	}
}

func Exit() {
	if os.Getenv("SLACK_GO_WEBHOOK_DEBUG") != "" {
		StopTicker()
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

		} else if resp.StatusCode >= 400 {
			return []error{fmt.Errorf("Error sending msg. Status: %v", resp.StatusCode)}
		} else {
			StatusCodeRetryInterval = MaxDuration(0, StatusCodeRetryInterval-StatusCodeRetryIntervalDecrement)
			return nil
		}
	}
}

func StartTicker() {
	statusCodeLock.Lock()
	defer statusCodeLock.Unlock()

	if statusCodeTicker == nil {
		log.Printf("Initialising status code ticker (%v)\n", StatusCodeTickerInterval)
		statusCodeTicker = time.NewTicker(StatusCodeTickerInterval)
		go func() {
			for {
				select {
				case <-statusCodeTickerDone:
					log.Printf("Exiting status code ticker (%v)",StatusCodeTickerInterval)
					return
				case t := <-statusCodeTicker.C:
					reportStatusCodes(t)
					resetStatusCodes()
				}
			}
		}()
	}
}

func StopTicker() {
	log.Printf("Stopping status code ticker (%v)", StatusCodeTickerInterval)
	statusCodeTicker.Stop()
	statusCodeTickerDone <- true
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

	log.Printf("Slack HTTP response codes = %v (StatusCodeTickerInverval=%v, StatusCodeRetryInterval=%v, StatusCodeRetryIntervalIncrement=%v, StatusCodeRetryIntervalDecrement=%v)\n",
		statusCodeMap, StatusCodeTickerInterval, StatusCodeRetryInterval, StatusCodeRetryIntervalIncrement, StatusCodeRetryIntervalDecrement)
}

func resetStatusCodes() {
	statusCodeLock.Lock()
	defer statusCodeLock.Unlock()

	for code := range statusCodeMap {
		statusCodeMap[code] = 0
	}
}
