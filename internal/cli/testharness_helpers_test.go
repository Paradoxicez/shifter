package cli

import (
	"context"
	"io"
	"log/slog"
	"math/big"
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/ingest"
	"github.com/shifter-io/shifter/internal/resolver"
	"github.com/shifter-io/shifter/internal/swap"
)

// swapDepsValue returns a swap.Deps suitable for in-test CommitSwap calls.
func (h *cliHarnessInfra) swapDepsValue() swap.Deps {
	return swap.Deps{Pool: h.pool, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

// cliPadDevEUI maps a label to a stable lowercase 16-hex dev_eui (FNV-1a-based
// to avoid the "all letters strip to same hex" pitfall a naive filter has).
// Mirrors padDevEUI in internal/testharness/scenarios_test.go.
func cliPadDevEUI(s string) string {
	var h uint64 = 0xcbf29ce484222325
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 0x100000001b3
	}
	out := make([]byte, 16)
	const hex = "0123456789abcdef"
	for i := 15; i >= 0; i-- {
		out[i] = hex[h&0xF]
		h >>= 4
	}
	return string(out)
}

// cliSqlcLoader is the resolver.Loader used by the CLI test mock ingest
// consumer. Wraps the production sqlc query.
type cliSqlcLoader struct{ pool *pgxpool.Pool }

func (l *cliSqlcLoader) LoadActive(ctx context.Context, devEUI string, at time.Time) (resolver.Binding, error) {
	q := sqlc.New(l.pool)
	row, err := q.GetActiveBindingByDevEUI(ctx, sqlc.GetActiveBindingByDevEUIParams{
		DevEui:    devEUI,
		ValidFrom: pgtype.Timestamptz{Time: at, Valid: true},
	})
	if err != nil {
		if err.Error() == "no rows in result set" {
			return resolver.Binding{}, resolver.ErrNoActiveBinding
		}
		return resolver.Binding{}, err
	}
	b := resolver.Binding{
		BindingID:       uuid.UUID(row.ID.Bytes),
		MeteringPointID: uuid.UUID(row.MeteringPointID.Bytes),
		DeviceID:        uuid.UUID(row.DeviceID.Bytes),
		DeviceProfileID: uuid.UUID(row.DeviceProfileID.Bytes),
		CounterModulus:  row.CounterModulus,
		BatteryCurve:    row.BatteryCurve,
	}
	if row.ValidFrom.Valid {
		b.ValidFrom = row.ValidFrom.Time
	}
	if row.ValidTo.Valid {
		b.ValidTo = row.ValidTo.Time
	}
	if row.ReadingOffset.Valid {
		if f, err := cliNumericToBigFloat(row.ReadingOffset); err == nil {
			b.ReadingOffset = f
		} else {
			b.ReadingOffset = big.NewFloat(0)
		}
	} else {
		b.ReadingOffset = big.NewFloat(0)
	}
	if row.LastRawValue.Valid {
		if f, err := cliNumericToBigFloat(row.LastRawValue); err == nil {
			b.LastRawValue = f
		}
	}
	return b, nil
}

func cliNumericToBigFloat(n pgtype.Numeric) (*big.Float, error) {
	if !n.Valid {
		return big.NewFloat(0), nil
	}
	b, err := n.MarshalJSON()
	if err != nil {
		return nil, err
	}
	s := string(b)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	f, _, err := big.ParseFloat(s, 10, 128, big.ToNearestEven)
	return f, err
}

// buildCLITestIngestDeps constructs an ingest.Deps the mock-serve-ingest
// goroutine uses to persist uplinks.
func buildCLITestIngestDeps(t *testing.T, pool *pgxpool.Pool) (ingest.Deps, *resolver.Resolver) {
	t.Helper()
	loader := &cliSqlcLoader{pool: pool}
	r := resolver.New(loader)
	mappings := &ingest.SQLCMappingStore{Pool: pool}
	return ingest.Deps{
		Pool:     pool,
		Resolver: r,
		Mappings: mappings,
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, r
}

// runMockMQTTConsumer subscribes to the canonical CS v4 uplink topic on
// brokerURL and invokes ingest.UplinkHandler for every received message.
// Mirrors what shifter serve does in production. Runs until ctx is canceled.
func runMockMQTTConsumer(ctx context.Context, brokerURL string, deps ingest.Deps) {
	opts := paho.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID("shifter-cli-test-mock-ingest").
		SetConnectTimeout(5 * time.Second)
	c := paho.NewClient(opts)
	if tk := c.Connect(); tk.WaitTimeout(5*time.Second) && tk.Error() == nil {
		// good
	} else {
		return
	}
	defer c.Disconnect(250)

	handler := ingest.UplinkHandler(deps)
	tk := c.Subscribe("application/+/device/+/event/up", 1, func(_ paho.Client, msg paho.Message) {
		handler(msg.Topic(), msg.Payload())
	})
	if !tk.WaitTimeout(5*time.Second) || tk.Error() != nil {
		return
	}
	<-ctx.Done()
}
