package subscribers

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"
)

// RSSFeed generates an RSS 2.0 XML feed of status events for a tenant.
func (s *Service) RSSFeed(ctx context.Context, tenantID string, baseURL string, limit int) ([]byte, error) {
	events, err := s.ListEvents(ctx, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("rss feed: %w", err)
	}

	items := make([]rssItem, 0, len(events))
	for _, e := range events {
		items = append(items, rssItem{
			Title:       e.Title,
			Description: e.Description,
			PubDate:     e.CreatedAt.Format(time.RFC1123Z),
			GUID:        rssGUID{Value: e.EventID, IsPermaLink: false},
			Category:    string(e.EventType),
		})
	}

	feed := rssFeed{
		Version: "2.0",
		Channel: rssChannel{
			Title:         fmt.Sprintf("Status Updates — %s", tenantID),
			Link:          fmt.Sprintf("%s/status/%s", baseURL, tenantID),
			Description:   "Service status updates and incident notifications.",
			LastBuildDate: time.Now().UTC().Format(time.RFC1123Z),
			Items:         items,
		},
	}

	out, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal rss: %w", err)
	}
	return append([]byte(xml.Header), out...), nil
}

// RSS XML structures.

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	LastBuildDate string    `xml:"lastBuildDate"`
	Items         []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string  `xml:"title"`
	Description string  `xml:"description"`
	PubDate     string  `xml:"pubDate"`
	GUID        rssGUID `xml:"guid"`
	Category    string  `xml:"category"`
}

type rssGUID struct {
	Value       string `xml:",chardata"`
	IsPermaLink bool   `xml:"isPermaLink,attr"`
}
