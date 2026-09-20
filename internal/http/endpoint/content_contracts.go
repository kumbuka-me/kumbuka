package endpoint

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// navigationService supplies navigation projections and icon administration.
type navigationService interface {
	NavigationPages(context.Context) ([]domain.Page, error)
	VisiblePages(context.Context, domain.User) ([]domain.Page, error)
	NavigationItems(context.Context) ([]domain.NavigationItem, error)
	NavigationIcons(context.Context) (map[string]string, error)
	SetNavigationIcon(context.Context, string, string) error
}

// knowledgeGraphService loads the page relationship graph for the current actor.
type knowledgeGraphService interface {
	KnowledgeGraphFor(context.Context, domain.User, int) (domain.KnowledgeGraph, error)
}
