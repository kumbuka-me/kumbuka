package plugincap

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk"
)

// PublicUserSource contains persistence lookups required by the runtime-wide safe directory.
type PublicUserSource interface {
	SearchPublicUsers(context.Context, string, int) ([]domain.User, error)
	UserByUsername(context.Context, string) (domain.User, error)
}

// PublicUsers adapts core users to the runtime's plugin-safe user contract.
type PublicUsers struct {
	// Source provides privacy-safe directory queries.
	Source PublicUserSource
}

// Search returns bounded enabled users without contact or authorization data.
func (p PublicUsers) Search(ctx context.Context, query sdk.UserQuery) ([]sdk.User, error) {
	if p.Source == nil || !validUserQuery(query) {
		return nil, errors.New("invalid user query")
	}
	users, err := p.Source.SearchPublicUsers(ctx, query.Query, query.Limit)
	return userValues(users), err
}

// ResolveMention resolves one enabled account by canonical mention.
func (p PublicUsers) ResolveMention(ctx context.Context, request sdk.UserMention) (sdk.User, error) {
	if p.Source == nil || !validUserMention(request.Mention) {
		return sdk.User{}, errors.New("invalid user mention")
	}
	username := strings.TrimPrefix(strings.TrimSpace(request.Mention), "@")
	user, err := p.Source.UserByUsername(ctx, username)
	if err != nil {
		return sdk.User{}, err
	}
	if !user.Enabled {
		return sdk.User{}, domain.ErrNotFound
	}
	return userValue(user), nil
}

// NotificationSender creates attributed core-owned notifications.
type NotificationSender interface {
	SendPlugin(context.Context, int64, string, string, sdk.NotificationInput) (sdk.Notification, error)
}

// NotificationCapabilities returns mutation-only notification creation capabilities.
func NotificationCapabilities(sender NotificationSender, actorID int64, pluginID, pluginName string) map[string]plugin.Capability {
	if sender == nil || actorID <= 0 || pluginID == "" || pluginName == "" {
		return nil
	}
	return map[string]plugin.Capability{
		"notifications.send": func(ctx context.Context, data json.RawMessage) (any, error) {
			var input sdk.NotificationInput
			if err := json.Unmarshal(data, &input); err != nil {
				return nil, errors.New("invalid notification")
			}
			return sender.SendPlugin(ctx, actorID, pluginID, pluginName, input)
		},
	}
}

// userValues converts domain accounts to plugin-safe identities.
func userValues(users []domain.User) []sdk.User {
	values := make([]sdk.User, 0, len(users))
	for _, user := range users {
		if user.Enabled {
			values = append(values, userValue(user))
		}
	}
	return values
}

// userValue converts one account without exposing contact or authorization data.
func userValue(user domain.User) sdk.User {
	return sdk.User{ID: user.ID, Mention: "@" + user.Username, DisplayName: user.DisplayName}
}

// validUserQuery reports whether a directory search stays within host bounds.
func validUserQuery(query sdk.UserQuery) bool {
	return len(strings.TrimSpace(query.Query)) <= 128 && query.Limit >= 1 && query.Limit <= 50
}

// validUserMention reports whether a mention is bounded and @-prefixed.
func validUserMention(mention string) bool {
	mention = strings.TrimSpace(mention)
	username, found := strings.CutPrefix(mention, "@")
	return found && username != "" && len(username) <= 128 && !strings.Contains(username, "@")
}
