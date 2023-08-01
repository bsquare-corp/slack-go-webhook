package slack

import (
	"log"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v6"
	"github.com/h2non/gock"
)

func TestSend(t *testing.T) {
	defer gock.Off()

	gock.New("http://test.com").
		Post("/200").
		Persist().
		Reply(200)

	gock.DisableNetworking()

	// Set debug env var
	os.Setenv("SLACK_GO_WEBHOOK_DEBUG", "true")

	StatusCodeTickerInterval = 4 * time.Second
	StatusCodeRetryInterval = 1000 * time.Microsecond
	StatusCodeRetryIntervalDecrement = 1 * time.Microsecond
	StatusCodeRetryIntervalIncrement = 100 * time.Microsecond

	// Initialize ticker
	Init()

	// Send messages
	for i := 0; i < 100; i++ {
		var url string

		gock.New("http://test.com").
			Post("/429").
			Times(8).
			Reply(429)
		gock.New("http://test.com").
			Post("/429").
			Reply(200)

		log.Printf("Test %v", i)

		// Generate first and last names
		firstName := gofakeit.FirstName()
		lastName := gofakeit.LastName()

		fullName := firstName + " " + lastName

		if rand.Intn(100) < 80 {
			url = "http://test.com/200"
		} else {
			url = "http://test.com/429"
		}

		payload := Payload{
			Text: "Hello " + fullName,
		}

		Send(url, "", payload)
	}

	time.Sleep(3 * StatusCodeTickerInterval)

}
