package admin

import (
	"context"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/adminclient"
)

// NamedDownstream adapts an adminclient to the Downstream port, attaching the
// label that lands in failed_services and, through it, in the console's
// "Media not deleted" wording.
//
// It lives here rather than in adminclient so that package stays a transport
// concern and this one owns the vocabulary the operator sees. The composition
// root is the only place that knows both the client and its label.
type NamedDownstream struct {
	Label  string
	Client interface {
		Purge(ctx context.Context, req adminclient.PurgeRequest) (map[string]int, error)
		Restore(ctx context.Context, opID string) (map[string]int, error)
		Reap(ctx context.Context, opID string) (map[string]int, error)
	}
}

func (n NamedDownstream) Name() string { return n.Label }

func (n NamedDownstream) Purge(ctx context.Context, req adminclient.PurgeRequest) (map[string]int, error) {
	return n.Client.Purge(ctx, req)
}

func (n NamedDownstream) Restore(ctx context.Context, opID string) (map[string]int, error) {
	return n.Client.Restore(ctx, opID)
}

func (n NamedDownstream) Reap(ctx context.Context, opID string) (map[string]int, error) {
	return n.Client.Reap(ctx, opID)
}

// NamedStatsSource adapts a bare count function to the StatsSource port.
type NamedStatsSource struct {
	AttrKey string
	Service string
	Fn      func(ctx context.Context) (int, error)
}

func (n NamedStatsSource) Key() string                            { return n.AttrKey }
func (n NamedStatsSource) Name() string                           { return n.Service }
func (n NamedStatsSource) Count(ctx context.Context) (int, error) { return n.Fn(ctx) }
