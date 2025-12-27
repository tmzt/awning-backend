package services

import (
	"awning-backend/common"
	"log/slog"
)

type Processors struct {
	logger       *slog.Logger
	cfg          *common.Config
	processorMap map[string]common.Processor
}

func NewProcessors(cfg *common.Config) *Processors {
	logger := slog.With("service", "Processors")

	processorMap := make(map[string]common.Processor)

	return &Processors{
		logger:       logger,
		cfg:          cfg,
		processorMap: processorMap,
	}
}

func (p *Processors) RegisterProcessor(name string, processor common.Processor) {
	p.logger.Info("Registering processor", "name", name)
	p.processorMap[name] = processor
}

func (p *Processors) GetProcessor(name string) (common.Processor, bool) {
	processor, exists := p.processorMap[name]
	return processor, exists
}

func (p *Processors) GetEnabledProcessors() []common.Processor {
	var processors []common.Processor
	for name, processor := range p.processorMap {
		if p.cfg.IsProcessorEnabled(name) {
			processors = append(processors, processor)
		}
	}
	return processors
}
