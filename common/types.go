package common

import "context"

type Processor interface {
	Name() string
	Process(ctx context.Context, input []byte) ([]byte, error)
}
