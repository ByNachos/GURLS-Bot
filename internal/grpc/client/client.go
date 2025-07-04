package client

import (
	"context"
	"fmt"
	"time"

	shortenerv1 "GURLS-Bot/gen/go/shortener/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type BackendClient struct {
	conn   *grpc.ClientConn
	client shortenerv1.ShortenerClient
	log    *zap.Logger
}

func NewBackendClient(address string, timeout time.Duration, log *zap.Logger) (*BackendClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, address, 
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to backend: %w", err)
	}

	client := shortenerv1.NewShortenerClient(conn)

	return &BackendClient{
		conn:   conn,
		client: client,
		log:    log,
	}, nil
}

func (c *BackendClient) CreateLink(ctx context.Context, req *shortenerv1.CreateLinkRequest) (*shortenerv1.CreateLinkResponse, error) {
	resp, err := c.client.CreateLink(ctx, req)
	if err != nil {
		c.log.Error("failed to create link via backend", zap.Error(err))
		return nil, err
	}
	return resp, nil
}

func (c *BackendClient) GetLinkStats(ctx context.Context, req *shortenerv1.GetLinkStatsRequest) (*shortenerv1.GetLinkStatsResponse, error) {
	resp, err := c.client.GetLinkStats(ctx, req)
	if err != nil {
		c.log.Error("failed to get link stats via backend", zap.Error(err))
		return nil, err
	}
	return resp, nil
}

func (c *BackendClient) DeleteLink(ctx context.Context, req *shortenerv1.DeleteLinkRequest) error {
	_, err := c.client.DeleteLink(ctx, req)
	if err != nil {
		c.log.Error("failed to delete link via backend", zap.Error(err))
		return err
	}
	return nil
}

func (c *BackendClient) ListUserLinks(ctx context.Context, req *shortenerv1.ListUserLinksRequest) (*shortenerv1.ListUserLinksResponse, error) {
	resp, err := c.client.ListUserLinks(ctx, req)
	if err != nil {
		c.log.Error("failed to list user links via backend", zap.Error(err))
		return nil, err
	}
	return resp, nil
}

func (c *BackendClient) Close() error {
	return c.conn.Close()
}