package domain

import (
	"net/url"
	"strconv"
)

func BuildTaskDeepLink(taskID, turnID int64, focus Focus) (string, error) {
	return BuildTaskDeepLinkForSource(taskID, turnID, focus, "", 0)
}

func BuildTaskDeepLinkForSource(taskID, turnID int64, focus Focus, sourceType SourceType, sourceID int64) (string, error) {
	if taskID <= 0 || turnID <= 0 || !focus.Valid() {
		return "", ErrDeepLinkInvalid
	}
	query := url.Values{}
	query.Set("turn", strconv.FormatInt(turnID, 10))
	query.Set("focus", string(focus))
	// Values.Encode sorts keys alphabetically. The product contract deliberately
	// fixes turn before focus, so build the two allowlisted fields explicitly.
	result := "/workforce/tasks/" + strconv.FormatInt(taskID, 10) +
		"?turn=" + url.QueryEscape(query.Get("turn")) + "&focus=" + url.QueryEscape(query.Get("focus"))
	if focus == FocusDelivery {
		switch sourceType {
		case SourceDeliveryRequest:
			if sourceID <= 0 {
				return "", ErrDeepLinkInvalid
			}
			result += "&delivery=" + strconv.FormatInt(sourceID, 10)
		case SourceReviewOperation:
			if sourceID <= 0 {
				return "", ErrDeepLinkInvalid
			}
			result += "&review=" + strconv.FormatInt(sourceID, 10)
		}
	}
	return result, nil
}
