package interfaces

import (
	"context"

	"github.com/ezenkico/deploy-commander/runner/models"
)

type Platform interface {
	Run(ctx context.Context, config models.Configuration) error
}
