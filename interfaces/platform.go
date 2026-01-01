package platforms

import (
	"github.com/ezenkico/deploy-commander/runner/models"
	"github.com/google/uuid"
)

type Platform interface {
	Run(job uuid.UUID, run uuid.UUID, metdata models.Metadata) error
}
