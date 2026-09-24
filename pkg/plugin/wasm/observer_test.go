package wasm_test

import (
	"sync"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/plugin/wasm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// invocationObservation captures one executable plugin call observed by the runtime.
type invocationObservation struct {
	// pluginID is the manifest ID of the invoked plugin.
	pluginID string
	// moduleID is the manifest module ID of the invocation.
	moduleID string
	// stage is the SDK render stage used for the invocation.
	stage string
	// duration is the end-to-end runtime call duration.
	duration time.Duration
	// err is the invocation result reported to the observer.
	err error
}

// invocationObserverStub records executable plugin invocation observations.
type invocationObserverStub struct {
	// mu protects observations written by plugin calls.
	mu sync.Mutex
	// observations contains calls observed by the test double.
	observations []invocationObservation
}

// ObservePluginInvocation records one completed executable plugin call.
func (o *invocationObserverStub) ObservePluginInvocation(pluginID, moduleID, stage string, duration time.Duration, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.observations = append(o.observations, invocationObservation{
		pluginID: pluginID,
		moduleID: moduleID,
		stage:    stage,
		duration: duration,
		err:      err,
	})
}

func TestInvocationObserverReceivesPluginCall(t *testing.T) {
	t.Parallel()

	observer := &invocationObserverStub{}
	instance, _ := runtimeFixtureWithOptions(
		t,
		"preprocess",
		wasm.Limits{},
		wasm.WithInvocationObserver(observer),
	)

	_, err := instance.Contributions().Preprocessors[0].Preprocess(plugin.Context{}, "text")
	require.NoError(t, err)

	observer.mu.Lock()
	defer observer.mu.Unlock()
	require.Len(t, observer.observations, 1)
	assert.Equal(t, "io.example.fixture", observer.observations[0].pluginID)
	assert.Equal(t, "fixture", observer.observations[0].moduleID)
	assert.Equal(t, "preprocess", observer.observations[0].stage)
	assert.Greater(t, observer.observations[0].duration, time.Duration(0))
	assert.NoError(t, observer.observations[0].err)
}
