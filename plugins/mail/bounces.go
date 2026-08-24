package mail

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

type bounceEvent struct {
	Email      string
	Event      string // "bounce" or "complaint"
	BounceType string // "Permanent", "Transient", "Undetermined"
	Details    string
}

// isAWSSNSURL reports whether u is a legitimate AWS SNS confirmation URL: https
// to an sns.<region>.amazonaws.com host. This blocks the SubscribeURL (which is
// attacker-influenced) from pointing the server at arbitrary/internal hosts.
func isAWSSNSURL(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme != "https" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return strings.HasPrefix(host, "sns.") && strings.HasSuffix(host, ".amazonaws.com")
}

func extractBounceEvents(body []byte) []bounceEvent {
	var events []bounceEvent

	parseMap := func(m map[string]any) []bounceEvent {
		var results []bounceEvent

		// 1. SES Format
		if nType, ok := m["notificationType"].(string); ok {
			switch nType {
			case "Bounce":
				if bMap, ok := m["bounce"].(map[string]any); ok {
					bType, _ := bMap["bounceType"].(string)
					bSubType, _ := bMap["bounceSubType"].(string)
					details := fmt.Sprintf("Bounce Type: %s, SubType: %s", bType, bSubType)
					if recs, ok := bMap["bouncedRecipients"].([]any); ok {
						for _, rVal := range recs {
							if rMap, ok := rVal.(map[string]any); ok {
								if email, ok := rMap["emailAddress"].(string); ok {
									results = append(results, bounceEvent{
										Email:      email,
										Event:      "bounce",
										BounceType: bType,
										Details:    details,
									})
								}
							}
						}
					}
				}
			case "Complaint":
				if cMap, ok := m["complaint"].(map[string]any); ok {
					feed, _ := cMap["complaintFeedbackType"].(string)
					details := fmt.Sprintf("Complaint Feedback Type: %s", feed)
					if recs, ok := cMap["complainedRecipients"].([]any); ok {
						for _, rVal := range recs {
							if rMap, ok := rVal.(map[string]any); ok {
								if email, ok := rMap["emailAddress"].(string); ok {
									results = append(results, bounceEvent{
										Email:   email,
										Event:   "complaint",
										Details: details,
									})
								}
							}
						}
					}
				}
			}
			if len(results) > 0 {
				return results
			}
		}

		// 2. Mailgun Format
		if edVal, ok := m["event-data"].(map[string]any); ok {
			ev, _ := edVal["event"].(string)
			recipient, _ := edVal["recipient"].(string)
			var details string
			if dsVal, ok := edVal["delivery-status"].(map[string]any); ok {
				if desc, ok := dsVal["description"].(string); ok {
					details = desc
				} else if msg, ok := dsVal["message"].(string); ok {
					details = msg
				}
			}
			if ev == "failed" || ev == "complained" {
				var finalEv string
				if ev == "failed" {
					finalEv = "bounce"
				} else {
					finalEv = "complaint"
				}
				var bType string
				if sev, ok := edVal["severity"].(string); ok {
					if strings.EqualFold(sev, "permanent") {
						bType = "Permanent"
					} else if strings.EqualFold(sev, "temporary") {
						bType = "Transient"
					}
				}
				if recipient != "" {
					results = append(results, bounceEvent{
						Email:      recipient,
						Event:      finalEv,
						BounceType: bType,
						Details:    details,
					})
					return results
				}
			}
		}

		// 3. SendGrid / Generic Format
		var email, event, details, bType string
		for _, key := range []string{"email", "recipient", "address", "rcpt"} {
			if eVal, ok := m[key].(string); ok && eVal != "" {
				email = eVal
				break
			}
		}
		for _, key := range []string{"event", "eventType"} {
			if eVal, ok := m[key].(string); ok && eVal != "" {
				event = eVal
				break
			}
		}
		for _, key := range []string{"reason", "description", "status"} {
			if eVal, ok := m[key].(string); ok && eVal != "" {
				details = eVal
				break
			}
		}
		for _, key := range []string{"bounceType", "bounce_type", "type", "severity"} {
			if btVal, ok := m[key].(string); ok && btVal != "" {
				if strings.EqualFold(btVal, "permanent") {
					bType = "Permanent"
				} else if strings.EqualFold(btVal, "transient") || strings.EqualFold(btVal, "temporary") {
					bType = "Transient"
				} else {
					bType = btVal
				}
				break
			}
		}
		if (bType == "" || strings.EqualFold(bType, "bounce")) && (strings.HasPrefix(details, "5.") || strings.Contains(details, "550")) {
			bType = "Permanent"
		}
		event = strings.ToLower(event)
		if strings.Contains(event, "bounce") || event == "dropped" || event == "failed" {
			event = "bounce"
		} else if strings.Contains(event, "complaint") || event == "spamreport" {
			event = "complaint"
		}

		if email != "" && event != "" {
			results = append(results, bounceEvent{
				Email:      email,
				Event:      event,
				BounceType: bType,
				Details:    details,
			})
		}
		return results
	}

	var list []map[string]any
	if err := json.Unmarshal(body, &list); err == nil {
		for _, m := range list {
			events = append(events, parseMap(m)...)
		}
		return events
	}

	var m map[string]any
	if err := json.Unmarshal(body, &m); err == nil {
		events = append(events, parseMap(m)...)
	}

	return events
}
