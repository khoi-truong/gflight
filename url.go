package gflight

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/khoi-truong/gflight/internal/encoding"
)

// SearchURL returns a shareable Google Flights deep link for req. The route,
// dates and trip type are encoded into the `tfs` parameter exactly as the web
// UI does it; opening the link in a browser reproduces the search.
//
// It performs no network I/O and needs no [Client], so it is useful on its own
// — e.g. to hand a user a link when the API surface is blocked.
func SearchURL(req SearchRequest) (string, error) {
	if req.Origin == "" || req.Destination == "" {
		return "", fmt.Errorf("gflight: SearchURL needs origin and destination: %w", ErrBadResponse)
	}
	if req.DepartDate.IsZero() {
		return "", fmt.Errorf("gflight: SearchURL needs a departure date: %w", ErrBadResponse)
	}

	oneWay := req.ReturnDate.IsZero()
	tfsReq := encoding.TFSRequest{
		OneWay: oneWay,
		Segments: []encoding.TFSSegment{{
			Legs: []encoding.TFSLeg{{
				Origin: req.Origin,
				Dest:   req.Destination,
				Date:   req.DepartDate,
			}},
		}},
	}
	if !oneWay {
		tfsReq.Segments = append(tfsReq.Segments, encoding.TFSSegment{
			Legs: []encoding.TFSLeg{{
				Origin: req.Destination,
				Dest:   req.Origin,
				Date:   req.ReturnDate,
			}},
		})
	}

	token, err := encoding.EncodeTFS(tfsReq)
	if err != nil {
		return "", fmt.Errorf("gflight: encode tfs: %w", err)
	}

	q := url.Values{}
	q.Set("tfs", token)
	if req.Currency != "" {
		q.Set("curr", strings.ToUpper(req.Currency))
	}
	return DefaultBaseURL + "?" + q.Encode(), nil
}
