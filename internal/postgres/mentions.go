package postgres

import "github.com/kumbuka-me/kumbuka/pkg/mention"

// mentionedUsernames extracts distinct normalized Kumbuka mentions in source order.
func mentionedUsernames(text string) []string { return mention.Usernames(text) }
