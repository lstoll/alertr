package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/cenk/backoff"
	"golang.org/x/net/proxy"
)

const defaultTestTimeout = 20 * time.Second

var slackCli = &http.Client{
	Timeout: time.Second * 30,
}

func main() {
	var (
		webhook     = flag.String("webhook", os.Getenv("SLACK_WEBHOOK_URL"), "Slack webhook URL")
		channel     = flag.String("channel", os.Getenv("SLACK_CHANNEL"), "Channel to notify")
		mention     = flag.String("mention", os.Getenv("MENTION"), "Who to mention in the message")
		endpoints   = flag.String("endpoints", os.Getenv("ENDPOINTS"), "URLs to monitor")
		testVia     = flag.String("test-via", os.Getenv("TEST_VIA"), "socks5 proxy to test via")
		testTimeout = flag.String("test-timeout", os.Getenv("TEST_TIMEOUT"), "timeout for testing")
	)
	flag.Parse()

	if *webhook == "" || *endpoints == "" {
		flag.PrintDefaults()
		log.Fatal("webhook and endpoints are required flags")
	}

	tt := defaultTestTimeout
	if *testTimeout != "" {
		var err error
		tt, err = time.ParseDuration(*testTimeout)
		if err != nil {
			log.Fatalf("parsing duration %s: %v", *testTimeout, err)
		}
	}

	clientFor := func(dialAddr string) *http.Client {
		testCli := &http.Client{Timeout: tt}

		if *testVia != "" {
			d := &net.Dialer{Timeout: tt}
			sp, err := proxy.SOCKS5("tcp", *testVia, nil, d)
			if err != nil {
				log.Fatalf("creating SOCKS5 proxy to %s: %v", *testVia, err)
			}

			tr := http.DefaultTransport.(*http.Transport).Clone()
			tr.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (sp.(proxy.ContextDialer)).DialContext(ctx, network, dialAddr)
			}
			testCli.Transport = tr
		}

		return testCli
	}

	eps, err := parseEndpoints(*endpoints)
	if err != nil {
		log.Fatalf("parsing endpoints: %v", err)
	}

	for _, ep := range eps {
		log.Printf("Checking %s", ep.String())
		req, err := http.NewRequest("GET", ep.url.String(), nil)
		if err != nil {
			log.Fatalf("creating request: %v", err)
		}
		da := req.URL.Host
		if ep.addr != "" {
			p := req.URL.Port()
			if p == "" {
				if req.URL.Scheme == "https" {
					p = "443"
				} else {
					p = "80"
				}
				da = net.JoinHostPort(ep.addr, p)
			}
		}
		c := clientFor(da)

		var resp *http.Response

		op := func() error {
			r, err := c.Do(req)
			if err != nil {
				return err
			}
			resp = r
			return nil
		}

		bo := backoff.NewExponentialBackOff()
		bo.MaxElapsedTime = 30 * time.Second

		err = backoff.Retry(op, bo)
		if err == nil && resp == nil {
			err = fmt.Errorf("nil err and response")
		}
		if err != nil {
			log.Printf("Error fetching %s: %v", ep.String(), err)
			if err := slackNotify(*webhook, *channel, fmt.Sprintf("%s Error fetching %s: %v", *mention, ep.String(), err)); err != nil {
				log.Printf("Error posting webhook: %v", err)
			}
			continue
		}
		if resp.StatusCode >= 400 {
			log.Printf("Error fetching %s: got status %d", ep.String(), resp.StatusCode)
			if err := slackNotify(*webhook, *channel, fmt.Sprintf("%s Error fetching %s, got status %d", *mention, ep.String(), resp.StatusCode)); err != nil {
				log.Printf("Error posting webhook: %v", err)
			}
			continue
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
		return fmt.Errorf("couldn't marshal message: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, webhookUrl, bytes.NewBuffer(slackBody))
	if err != nil {
		return fmt.Errorf("failed creating HTTP request: %w", err)
	}

	req.Header.Add("Content-Type", "application/json")

	resp, err := slackCli.Do(req)
	if err != nil {
		return fmt.Errorf("failed posting webhook: %w", err)
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	if buf.String() != "ok" {
		return fmt.Errorf("non-ok response returned from Slack")
	}

	return nil
}

type testEndpoint struct {
	// url requested to test
	url *url.URL
	// addr to direct the request to
	addr string
}

func (t *testEndpoint) String() string {
	s := t.url.String()
	if t.addr != "" {
		s = s + " (addr: " + t.addr + ")"
	}
	return s
}

func parseEndpoints(f string) ([]testEndpoint, error) {
	var ret []testEndpoint
	for _, ep := range strings.Split(f, ",") {
		sp := strings.Split(ep, ";")
		if len(sp) < 1 || len(sp) > 2 {
			return nil, fmt.Errorf("splitting %s on ; didn't give 1 or 2 results", ep)
		}
		u, err := url.Parse(sp[0])
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %v", ep, err)
		}
		e := testEndpoint{url: u}
		if len(sp) > 1 {
			q, err := url.ParseQuery(sp[1])
			if err != nil {
				return nil, fmt.Errorf("parsing %s as query string: %v", sp[1], err)
			}
			e.addr = q.Get("addr")
		}
		ret = append(ret, e)
	}
	return ret, nil
}
