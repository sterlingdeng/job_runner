package jobs

import (
	"bytes"
	"context"
	"io/ioutil"
	"job_runner/pkg/cgroupz"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// these tests must be run in a linux vm

func Test_Job_SimpleStartAndStream(t *testing.T) {
	job := New(context.Background(), []string{"echo", "hello"}, cgroupz.ResourceLimit{CpuWeight: 50, MaxMem: 1e8})
	err := job.Start()
	require.NoError(t, err)

	err = job.Wait()
	require.NoError(t, err)

	code, status := job.Result()
	require.Equal(t, 0, code)
	require.Equal(t, StatusExited, status)

	var buf bytes.Buffer
	err = job.Stream(&buf)
	require.NoError(t, err)
	// echo will append a newline
	require.Equal(t, "hello\n", buf.String())
}

func Test_JobStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	job := New(ctx, []string{"sleep", "5"}, cgroupz.ResourceLimit{CpuWeight: 50, MaxMem: 1e8})
	err := job.Start()
	require.NoError(t, err)

	cancel()
	err = job.Wait()
	require.NoError(t, err)

	code, status := job.Result()
	require.NotEqual(t, 0, code)
	require.Equal(t, StatusStopped, status)
}

func Test_Job_MultipleStreamers(t *testing.T) {
	// useful if -race flag is used
	cmd := []string{"sh", "-c", "for i in {1..50}; do echo ${RANDOM}; sleep 0.05; done"}
	job := New(context.Background(), cmd, cgroupz.ResourceLimit{CpuWeight: 50, MaxMem: 1e8})

	var wg sync.WaitGroup
	n := 20
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			err := job.Stream(ioutil.Discard)
			require.NoError(t, err)
		}()
	}

	err := job.Start()
	require.NoError(t, err)
	err = job.Wait()
	require.NoError(t, err)
	wg.Wait()
}
