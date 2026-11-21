package api

import (
	"context"
	"time"
)

type nowKey struct{}

// withNow hands a handler the router's clock without widening ReadHandler: the
// clock is injected once, in Config, and "today" is never read from the host
// anywhere else.
func withNow(ctx context.Context, now func() time.Time) context.Context {
	return context.WithValue(ctx, nowKey{}, now)
}

func nowOf(ctx context.Context) time.Time {
	if now, ok := ctx.Value(nowKey{}).(func() time.Time); ok {
		return now()
	}
	return time.Now()
}

// brazilZone formats the time of day a person reads. The day itself comes from
// brday, which owns the zone for everything that decides which month a row is in.
var brazilZone = mustLoadBrazilZone()

func mustLoadBrazilZone() *time.Location {
	zone, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		panic("api: America/Sao_Paulo: " + err.Error())
	}
	return zone
}

func clockTimeOf(instant time.Time) string {
	return instant.In(brazilZone).Format("15:04")
}
