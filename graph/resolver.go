package graph

import (
	"github.com/bbernstein/flowebb-go/graph/generated"
	"github.com/bbernstein/flowebb-go/internal/models"
	"github.com/bbernstein/flowebb-go/internal/tide"
)

type Resolver struct {
	TideService   tide.TideService
	StationFinder models.StationFinder
}

// Ensure Resolver implements the ResolverRoot interface
var _ generated.ResolverRoot = &Resolver{}
