package logmon

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hashicorp/nomad/client/logmon/proto"
	"github.com/hashicorp/nomad/helper/pluginutils/grpcutils"
)

type logmonClient struct {
	client proto.LogMonClient

	// doneCtx is closed when the plugin exits
	doneCtx context.Context
}

const logmonRPCTimeout = 1 * time.Minute

func (c *logmonClient) Start(cfg *LogConfig) error {
	bc, _ := json.Marshal(cfg.Config)
	bd, _ := json.Marshal(cfg.Data)

	req := &proto.StartRequest{
		LogDir:         cfg.LogDir,
		StdoutFileName: cfg.StdoutLogFile,
		StderrFileName: cfg.StderrLogFile,
		MaxFiles:       uint32(cfg.MaxFiles),
		MaxFileSizeMb:  uint32(cfg.MaxFileSizeMB),
		StdoutFifo:     cfg.StdoutFifo,
		StderrFifo:     cfg.StderrFifo,
		Driver:         cfg.DriverName,
		Config:         bc,
		Data:           bd,
	}
	ctx, cancel := context.WithTimeout(context.Background(), logmonRPCTimeout)
	defer cancel()

	_, err := c.client.Start(ctx, req)
	return grpcutils.HandleGrpcErr(err, c.doneCtx)
}

func (c *logmonClient) Stop() error {
	req := &proto.StopRequest{}
	ctx, cancel := context.WithTimeout(context.Background(), logmonRPCTimeout)
	defer cancel()

	_, err := c.client.Stop(ctx, req)
	return grpcutils.HandleGrpcErr(err, c.doneCtx)
}
