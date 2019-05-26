package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

var httpcli = &http.Client{
	Timeout: time.Second * 30,
}

func main() {
	var (
		webhook   = flag.String("webhook", os.Getenv("SLACK_WEBHOOK_URL"), "Slack webhook URL")
		channel   = flag.String("channel", os.Getenv("SLACK_CHANNEL"), "Channel to notify")
		mention   = flag.String("mention", os.Getenv("MENTION"), "Who to mention in the message")
		endpoints = flag.String("endpoints", os.Getenv("ENDPOINTS"), "URLs to monitor")
	)
	flag.Parse()

	if *webhook == "" || *endpoints == "" {
		flag.PrintDefaults()
		log.Fatal("webhook and endpoints are required flags")
	}

	for _, ep := range strings.Split(*endpoints, ",") {
		log.Printf("Checking %s", ep)
		resp, err := httpcli.Get(ep)
		if err != nil {
			log.Printf("Error fetching %s: %v", ep, err)
			if err := slackNotify(*webhook, *channel, fmt.Sprintf("%s Error fetching %s: %v", *mention, ep, err)); err != nil {
				log.Printf("Error posting webhook: %v", err)
				continue
			}
		}
		if resp.StatusCode >= 400 {
			log.Printf("Error fetching %s: got status %d", ep, resp.StatusCode)
			if err := slackNotify(*webhook, *channel, fmt.Sprintf("%s Error fetching %s, got status %d", *mention, ep, resp.StatusCode)); err != nil {
				log.Printf("Error posting webhook: %v", err)
				continue
			}
		}
	}
}

// slackReq is the request for sending a slack notification.
type slackReq struct {
	Channel     string            `json:"channel,omitempty"`
	Username    string            `json:"username,omitempty"`
	IconEmoji   string            `json:"icon_emoji,omitempty"`
	IconURL     string            `json:"icon_url,omitempty"`
	LinkNames   bool              `json:"link_names,omitempty"`
	Attachments []slackAttachment `json:"attachments"`
}

// slackAttachment is used to display a richly-formatted message block.
type slackAttachment struct {
	Title      string `json:"title,omitempty"`
	TitleLink  string `json:"title_link,omitempty"`
	Pretext    string `json:"pretext,omitempty"`
	Text       string `json:"text"`
	Fallback   string `json:"fallback"`
	CallbackID string `json:"callback_id"`
	// Fields     []config.SlackField  `json:"fields,omitempty"`
	// Actions    []config.SlackAction `json:"actions,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	ThumbURL string `json:"thumb_url,omitempty"`
	Footer   string `json:"footer"`

	Color    string   `json:"color,omitempty"`
	MrkdwnIn []string `json:"mrkdwn_in,omitempty"`
}

func slackNotify(webhookUrl, channel, msg string) error {
	pl := slackReq{
		Username:  "alertr",
		IconEmoji: ":arrow_heading_down:",
		Channel:   channel,
		Attachments: []slackAttachment{
			{
				Text:  msg,
				Color: "danger",
			},
		},
		LinkNames: true,
	}

	slackBody, err := json.Marshal(&pl)
	if err != nil {
		return xerrors.Errorf("couldn't marshal message: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, webhookUrl, bytes.NewBuffer(slackBody))
	if err != nil {
		return xerrors.Errorf("failed creating HTTP request: %w", err)
	}

	req.Header.Add("Content-Type", "application/json")

	resp, err := httpcli.Do(req)
	if err != nil {
		return xerrors.Errorf("failed posting webhook: %w", err)
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	if buf.String() != "ok" {
		return xerrors.New("Non-ok response returned from Slack")
	}

	return nil
}
